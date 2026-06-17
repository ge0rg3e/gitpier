package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gitpier/internal/models"

	"gorm.io/gorm"
)

type AntiSpamService struct {
	db                 *gorm.DB
	turnstileSecretKey string
	turnstileEndpoint  string
	httpClient         *http.Client
	enableTurnstile    bool
	enableRateLimiting bool
}

// TurnstileVerifyRequest is the request sent to Cloudflare Turnstile
type TurnstileVerifyRequest struct {
	Secret   string `json:"secret"`
	Response string `json:"response"`
	RemoteIP string `json:"remoteip,omitempty"`
}

// TurnstileVerifyResponse is the response from Cloudflare Turnstile
type TurnstileVerifyResponse struct {
	Success       bool      `json:"success"`
	ChallengeTS   time.Time `json:"challenge_ts"`
	Hostname      string    `json:"hostname"`
	ErrorCodes    []string  `json:"error-codes"`
	ErrorMessages []string  `json:"error_messages"`
}

type AntiSpamConfig struct {
	TurnstileSecretKey string
	EnableTurnstile    bool
	EnableRateLimiting bool
}

func NewAntiSpamService(db *gorm.DB, config AntiSpamConfig) (*AntiSpamService, error) {
	return &AntiSpamService{
		db:                 db,
		turnstileSecretKey: config.TurnstileSecretKey,
		turnstileEndpoint:  "https://challenges.cloudflare.com/turnstile/v0/siteverify",
		httpClient:         &http.Client{Timeout: 10 * time.Second},
		enableTurnstile:    config.EnableTurnstile,
		enableRateLimiting: config.EnableRateLimiting,
	}, nil
}

// VerifyTurnstileToken verifies a Turnstile CAPTCHA token with Cloudflare
func (s *AntiSpamService) VerifyTurnstileToken(ctx context.Context, token string, remoteIP string) error {
	if !s.enableTurnstile {
		return nil
	}

	if token == "" {
		return fmt.Errorf("turnstile token is required")
	}

	req := TurnstileVerifyRequest{
		Secret:   s.turnstileSecretKey,
		Response: token,
		RemoteIP: remoteIP,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal turnstile request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", s.turnstileEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create turnstile request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("turnstile verification failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read turnstile response: %w", err)
	}

	var verifyResp TurnstileVerifyResponse
	if err := json.Unmarshal(respBody, &verifyResp); err != nil {
		return fmt.Errorf("failed to parse turnstile response: %w", err)
	}

	if !verifyResp.Success {
		if len(verifyResp.ErrorCodes) > 0 {
			return fmt.Errorf("turnstile verification failed: %s", strings.Join(verifyResp.ErrorCodes, ", "))
		}
		return fmt.Errorf("turnstile verification failed")
	}

	return nil
}

// CheckAccountCreationRate limits the number of accounts created from the same IP
func (s *AntiSpamService) CheckAccountCreationRate(ctx context.Context, ipAddress string) error {
	if !s.enableRateLimiting {
		return nil
	}

	// Check 24-hour limit
	var count24h int64
	now := time.Now().UTC()
	oneDayAgo := now.Add(-24 * time.Hour)

	if err := s.db.WithContext(ctx).
		Model(&models.AccountCreationAttempt{}).
		Where("ip_address = ? AND success = true AND created_at > ?", ipAddress, oneDayAgo).
		Count(&count24h).Error; err != nil {
		return fmt.Errorf("failed to check creation rate: %w", err)
	}

	if count24h >= int64(models.AccountsPerIPPer24Hours) {
		return fmt.Errorf("too many accounts created from this IP address in the last 24 hours")
	}

	// Check 7-day limit
	var count7d int64
	sevenDaysAgo := now.Add(-7 * 24 * time.Hour)

	if err := s.db.WithContext(ctx).
		Model(&models.AccountCreationAttempt{}).
		Where("ip_address = ? AND success = true AND created_at > ?", ipAddress, sevenDaysAgo).
		Count(&count7d).Error; err != nil {
		return fmt.Errorf("failed to check creation rate: %w", err)
	}

	if count7d >= int64(models.AccountsPerIPPer7Days) {
		return fmt.Errorf("too many accounts created from this IP address in the last 7 days")
	}

	return nil
}

// RecordAccountCreationAttempt logs a registration attempt
func (s *AntiSpamService) RecordAccountCreationAttempt(ctx context.Context, ipAddress, email, userAgent string, success bool) error {
	attempt := models.AccountCreationAttempt{
		IPAddress: ipAddress,
		Email:     email,
		UserAgent: userAgent,
		Success:   success,
	}

	if err := s.db.WithContext(ctx).Create(&attempt).Error; err != nil {
		return fmt.Errorf("failed to record account creation attempt: %w", err)
	}

	return nil
}

// Close closes the disposable checker if it was initialized
func (s *AntiSpamService) Close() error {
	return nil
}
