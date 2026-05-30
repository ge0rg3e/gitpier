package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

// validateWebhookURL ensures the payload URL uses http/https and does not target
// private/loopback addresses (SSRF prevention).
func validateWebhookURL(rawURL string) error {
	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL must contain a host")
	}
	// Reject well-known loopback/internal hostnames.
	lower := strings.ToLower(host)
	blocked := []string{"localhost", "metadata.google.internal"}
	for _, b := range blocked {
		if lower == b {
			return fmt.Errorf("URL host is not allowed")
		}
	}
	// Reject if the host resolves to a private/loopback IP.
	ips, err := net.LookupHost(host)
	if err != nil {
		// If DNS resolution fails, reject to be safe.
		return fmt.Errorf("could not resolve URL host")
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("URL must not target a private or loopback address")
		}
	}
	return nil
}

type WebhookHandler struct {
	webhookSvc *services.WebhookService
	repoSvc    *services.RepoService
}

func NewWebhookHandler(webhookSvc *services.WebhookService, repoSvc *services.RepoService) *WebhookHandler {
	return &WebhookHandler{webhookSvc: webhookSvc, repoSvc: repoSvc}
}

// resolveRepo returns the repo and verifies the caller is an admin/owner.
func (h *WebhookHandler) resolveAdminRepo(c echo.Context) (*models.Repository, error) {
	username := c.Param("username")
	repoName := c.Param("repo")
	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	currentUser := c.Get("user").(*models.User)
	if !h.repoSvc.IsAdminAccess(repo, currentUser.ID) {
		return nil, echo.NewHTTPError(http.StatusForbidden, "admin access required")
	}
	return repo, nil
}

func parseHookID(c echo.Context) (string, error) {
	id := c.Param("id")
	if id == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid webhook id")
	}
	return id, nil
}

func (h *WebhookHandler) List(c echo.Context) error {
	repo, err := h.resolveAdminRepo(c)
	if err != nil {
		return err
	}
	hooks, err := h.webhookSvc.List(c.Request().Context(), repo.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list webhooks")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"webhooks": hooks})
}

func (h *WebhookHandler) Create(c echo.Context) error {
	repo, err := h.resolveAdminRepo(c)
	if err != nil {
		return err
	}

	var req struct {
		PayloadURL  string   `json:"payload_url"`
		ContentType string   `json:"content_type"`
		Secret      string   `json:"secret"`
		Active      *bool    `json:"active"`
		Events      []string `json:"events"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.PayloadURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "payload_url is required")
	}
	if err := validateWebhookURL(req.PayloadURL); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if len(req.Events) == 0 {
		req.Events = []string{"push"}
	}
	if req.ContentType == "" {
		req.ContentType = "application/json"
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}

	hook, err := h.webhookSvc.Create(c.Request().Context(), services.CreateWebhookInput{
		RepoID:      repo.ID,
		PayloadURL:  req.PayloadURL,
		ContentType: req.ContentType,
		Secret:      req.Secret,
		Active:      active,
		Events:      req.Events,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create webhook")
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"webhook": withParsedEvents(hook)})
}

func (h *WebhookHandler) Get(c echo.Context) error {
	repo, err := h.resolveAdminRepo(c)
	if err != nil {
		return err
	}
	id, err := parseHookID(c)
	if err != nil {
		return err
	}
	hook, err := h.webhookSvc.GetByID(c.Request().Context(), repo.ID, id)
	if err != nil {
		if errors.Is(err, services.ErrWebhookNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "webhook not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get webhook")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"webhook": withParsedEvents(hook)})
}

func (h *WebhookHandler) Update(c echo.Context) error {
	repo, err := h.resolveAdminRepo(c)
	if err != nil {
		return err
	}
	id, err := parseHookID(c)
	if err != nil {
		return err
	}

	var req struct {
		PayloadURL  *string  `json:"payload_url"`
		ContentType *string  `json:"content_type"`
		Secret      *string  `json:"secret"`
		Active      *bool    `json:"active"`
		Events      []string `json:"events"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.PayloadURL != nil {
		if err := validateWebhookURL(*req.PayloadURL); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}

	hook, err := h.webhookSvc.Update(c.Request().Context(), repo.ID, id, services.UpdateWebhookInput{
		PayloadURL:  req.PayloadURL,
		ContentType: req.ContentType,
		Secret:      req.Secret,
		Active:      req.Active,
		Events:      req.Events,
	})
	if err != nil {
		if errors.Is(err, services.ErrWebhookNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "webhook not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update webhook")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"webhook": withParsedEvents(hook)})
}

func (h *WebhookHandler) Delete(c echo.Context) error {
	repo, err := h.resolveAdminRepo(c)
	if err != nil {
		return err
	}
	id, err := parseHookID(c)
	if err != nil {
		return err
	}
	if err := h.webhookSvc.Delete(c.Request().Context(), repo.ID, id); err != nil {
		if errors.Is(err, services.ErrWebhookNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "webhook not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete webhook")
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *WebhookHandler) ListDeliveries(c echo.Context) error {
	repo, err := h.resolveAdminRepo(c)
	if err != nil {
		return err
	}
	id, err := parseHookID(c)
	if err != nil {
		return err
	}
	// Verify hook belongs to repo
	if _, err := h.webhookSvc.GetByID(c.Request().Context(), repo.ID, id); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "webhook not found")
	}
	deliveries, err := h.webhookSvc.ListDeliveries(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list deliveries")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"deliveries": deliveries})
}

func (h *WebhookHandler) GetDelivery(c echo.Context) error {
	repo, err := h.resolveAdminRepo(c)
	if err != nil {
		return err
	}
	id, err := parseHookID(c)
	if err != nil {
		return err
	}
	deliveryID := c.Param("deliveryID")
	if deliveryID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid delivery id")
	}
	if _, err := h.webhookSvc.GetByID(c.Request().Context(), repo.ID, id); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "webhook not found")
	}
	delivery, err := h.webhookSvc.GetDelivery(c.Request().Context(), id, deliveryID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "delivery not found")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"delivery": delivery})
}

func (h *WebhookHandler) Redeliver(c echo.Context) error {
	repo, err := h.resolveAdminRepo(c)
	if err != nil {
		return err
	}
	id, err := parseHookID(c)
	if err != nil {
		return err
	}
	deliveryID := c.Param("deliveryID")
	if deliveryID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid delivery id")
	}
	if err := h.webhookSvc.Redeliver(c.Request().Context(), repo.ID, id, deliveryID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to redeliver")
	}
	return c.NoContent(http.StatusNoContent)
}

// webhookResponse is a view of Webhook with secret masked and events as []string.
type webhookResponse struct {
	ID          string   `json:"id"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	RepoID      string   `json:"repo_id"`
	PayloadURL  string   `json:"payload_url"`
	ContentType string   `json:"content_type"`
	HasSecret   bool     `json:"has_secret"`
	InsecureSSL bool     `json:"insecure_ssl"`
	Active      bool     `json:"active"`
	Events      []string `json:"events"`
}

func withParsedEvents(hook *models.Webhook) webhookResponse {
	var events []string
	json.Unmarshal([]byte(hook.Events), &events) // #nosec ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â safe stored JSON
	return webhookResponse{
		ID:          hook.ID,
		CreatedAt:   hook.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   hook.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		RepoID:      hook.RepoID,
		PayloadURL:  hook.PayloadURL,
		ContentType: hook.ContentType,
		HasSecret:   hook.Secret != "",
		InsecureSSL: hook.InsecureSSL,
		Active:      hook.Active,
		Events:      events,
	}
}
