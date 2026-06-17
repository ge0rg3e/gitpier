package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"gitpier/internal/models"

	"gorm.io/gorm"
)

var (
	ErrWebhookNotFound = errors.New("webhook not found")
)

// AllWebhookEvents is the full set of supported event names.
var AllWebhookEvents = []string{
	"push",
	"issues",
	"issue_comment",
	"pull_request",
	"pull_request_review",
	"release",
	"create",
	"delete",
	"*",
}

type WebhookService struct {
	db *gorm.DB
}

func NewWebhookService(db *gorm.DB) *WebhookService {
	return &WebhookService{db: db}
}

type CreateWebhookInput struct {
	RepoID      string
	PayloadURL  string
	ContentType string
	Secret      string
	Active      bool
	Events      []string
}

func (s *WebhookService) Create(ctx context.Context, in CreateWebhookInput) (*models.Webhook, error) {
	eventsJSON, err := json.Marshal(in.Events)
	if err != nil {
		return nil, fmt.Errorf("invalid events: %w", err)
	}
	hook := &models.Webhook{
		RepoID:      in.RepoID,
		PayloadURL:  in.PayloadURL,
		ContentType: in.ContentType,
		Secret:      in.Secret,
		InsecureSSL: false, // always disabled â€” TLS verification is mandatory
		Active:      in.Active,
		Events:      string(eventsJSON),
	}
	if err := s.db.WithContext(ctx).Create(hook).Error; err != nil {
		return nil, fmt.Errorf("failed to create webhook: %w", err)
	}
	return hook, nil
}

func (s *WebhookService) List(ctx context.Context, repoID string) ([]models.Webhook, error) {
	var hooks []models.Webhook
	if err := s.db.WithContext(ctx).Where("repo_id = ?", repoID).Find(&hooks).Error; err != nil {
		return nil, err
	}
	return hooks, nil
}

func (s *WebhookService) GetByID(ctx context.Context, repoID, id string) (*models.Webhook, error) {
	var hook models.Webhook
	err := s.db.WithContext(ctx).Where("id = ? AND repo_id = ?", id, repoID).First(&hook).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrWebhookNotFound
	}
	return &hook, err
}

type UpdateWebhookInput struct {
	PayloadURL  *string
	ContentType *string
	Secret      *string
	Active      *bool
	Events      []string
}

func (s *WebhookService) Update(ctx context.Context, repoID, id string, in UpdateWebhookInput) (*models.Webhook, error) {
	hook, err := s.GetByID(ctx, repoID, id)
	if err != nil {
		return nil, err
	}
	if in.PayloadURL != nil {
		hook.PayloadURL = *in.PayloadURL
	}
	if in.ContentType != nil {
		hook.ContentType = *in.ContentType
	}
	if in.Secret != nil {
		hook.Secret = *in.Secret
	}
	if in.Active != nil {
		hook.Active = *in.Active
	}
	if in.Events != nil {
		eventsJSON, err := json.Marshal(in.Events)
		if err != nil {
			return nil, fmt.Errorf("invalid events: %w", err)
		}
		hook.Events = string(eventsJSON)
	}
	if err := s.db.WithContext(ctx).Save(hook).Error; err != nil {
		return nil, fmt.Errorf("failed to update webhook: %w", err)
	}
	return hook, nil
}

func (s *WebhookService) Delete(ctx context.Context, repoID, id string) error {
	result := s.db.WithContext(ctx).Where("id = ? AND repo_id = ?", id, repoID).Delete(&models.Webhook{})
	if result.RowsAffected == 0 {
		return ErrWebhookNotFound
	}
	return result.Error
}

func (s *WebhookService) ListDeliveries(ctx context.Context, webhookID string) ([]models.WebhookDelivery, error) {
	var deliveries []models.WebhookDelivery
	if err := s.db.WithContext(ctx).
		Where("webhook_id = ?", webhookID).
		Order("created_at DESC").
		Limit(100).
		Find(&deliveries).Error; err != nil {
		return nil, err
	}
	return deliveries, nil
}

func (s *WebhookService) GetDelivery(ctx context.Context, webhookID string, deliveryID string) (*models.WebhookDelivery, error) {
	var d models.WebhookDelivery
	err := s.db.WithContext(ctx).Where("id = ? AND webhook_id = ?", deliveryID, webhookID).First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.New("delivery not found")
	}
	return &d, err
}

// Deliver sends the given payload to all active webhooks for repoID that
// subscribe to the given event (or "*").  It runs asynchronouslyâ€”errors are
// logged to the delivery log but never returned to the caller.
func (s *WebhookService) Deliver(ctx context.Context, repoID string, event string, payload interface{}) {
	hooks, err := s.List(ctx, repoID)
	if err != nil || len(hooks) == 0 {
		return
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return
	}

	for _, hook := range hooks {
		if !hook.Active {
			continue
		}
		if !hookSubscribesTo(hook.Events, event) {
			continue
		}
		h := hook // capture
		go s.deliver(h, event, payloadBytes)
	}
}

func hookSubscribesTo(eventsJSON, event string) bool {
	var events []string
	if err := json.Unmarshal([]byte(eventsJSON), &events); err != nil {
		return false
	}
	for _, e := range events {
		if e == "*" || e == event {
			return true
		}
	}
	return false
}

func (s *WebhookService) deliver(hook models.Webhook, event string, payloadBytes []byte) {
	guid := newGUID()
	start := time.Now()

	body := payloadBytes
	if hook.ContentType == "application/x-www-form-urlencoded" {
		body = []byte("payload=" + string(payloadBytes))
	}

	req, err := http.NewRequest("POST", hook.PayloadURL, bytes.NewReader(body))
	if err != nil {
		s.saveDelivery(hook.ID, guid, event, string(payloadBytes), 0, err.Error(), time.Since(start).Milliseconds(), false)
		return
	}
	req.Header.Set("Content-Type", hook.ContentType)
	req.Header.Set("X-GitPier-Event", event)
	req.Header.Set("X-GitPier-Delivery", guid)
	req.Header.Set("User-Agent", "GitPier-Hookshot/1.0")

	if hook.Secret != "" {
		mac := hmac.New(sha256.New, []byte(hook.Secret))
		mac.Write(payloadBytes)
		sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Hub-Signature-256", sig)
	}

	tlsCfg := secureWebhookTransport()
	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: tlsCfg,
	}

	resp, err := client.Do(req)
	dur := time.Since(start).Milliseconds()
	if err != nil {
		s.saveDelivery(hook.ID, guid, event, string(payloadBytes), 0, err.Error(), dur, false)
		return
	}
	defer resp.Body.Close()

	var respBuf bytes.Buffer
	respBuf.ReadFrom(resp.Body)
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	s.saveDelivery(hook.ID, guid, event, string(payloadBytes), resp.StatusCode, respBuf.String(), dur, success)
}

func (s *WebhookService) saveDelivery(hookID string, guid, event, payload string, code int, respBody string, durMS int64, success bool) {
	d := &models.WebhookDelivery{
		WebhookID:    hookID,
		GUID:         guid,
		Event:        event,
		Payload:      payload,
		ResponseCode: code,
		ResponseBody: respBody,
		DurationMS:   durMS,
		Success:      success,
	}
	s.db.Create(d)
}

// Redeliver re-sends an existing delivery's payload.
func (s *WebhookService) Redeliver(ctx context.Context, repoID, hookID, deliveryID string) error {
	hook, err := s.GetByID(ctx, repoID, hookID)
	if err != nil {
		return err
	}
	delivery, err := s.GetDelivery(ctx, hookID, deliveryID)
	if err != nil {
		return err
	}
	go s.deliver(*hook, delivery.Event, []byte(delivery.Payload))
	return nil
}

// newGUID generates a random UUID v4 string without external dependencies.
func newGUID() string {
	var buf [16]byte
	rand.Read(buf[:])               // #nosec G104 â€” crypto/rand.Read never errors on supported platforms
	buf[6] = (buf[6] & 0x0f) | 0x40 // version 4
	buf[8] = (buf[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}
