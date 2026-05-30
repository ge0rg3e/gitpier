package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type RegistrationMailer interface {
	SendRegistrationOTP(ctx context.Context, toEmail, toUsername, otpCode string, expiresAt time.Time) error
}

type MailerooEmailService struct {
	apiKey   string
	baseURL  string
	from     string
	fromName string
	client   *http.Client
}

type MailerooEmailConfig struct {
	APIKey   string
	BaseURL  string
	From     string
	FromName string
	Timeout  time.Duration
}

func NewMailerooEmailService(cfg MailerooEmailConfig) *MailerooEmailService {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &MailerooEmailService{
		apiKey:   strings.TrimSpace(cfg.APIKey),
		baseURL:  strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		from:     strings.TrimSpace(cfg.From),
		fromName: strings.TrimSpace(cfg.FromName),
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (s *MailerooEmailService) IsConfigured() bool {
	return s != nil && s.apiKey != "" && s.baseURL != "" && s.from != ""
}

func (s *MailerooEmailService) SendRegistrationOTP(ctx context.Context, toEmail, toUsername, otpCode string, expiresAt time.Time) error {
	if !s.IsConfigured() {
		return fmt.Errorf("maileroo email service is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	minutes := int(time.Until(expiresAt).Minutes())
	if minutes < 1 {
		minutes = 1
	}

	subject := "Your GitPier verification code"
	plain := strings.Join([]string{
		fmt.Sprintf("Hi %s,", toUsername),
		"",
		"Use this one-time code to finish creating your GitPier account:",
		"",
		fmt.Sprintf("  %s", otpCode),
		"",
		fmt.Sprintf("This code expires in about %d minute(s).", minutes),
		"If you did not request this, you can ignore this email.",
	}, "\n")

	payload := map[string]interface{}{
		"from": map[string]string{
			"address":      s.from,
			"display_name": s.fromName,
		},
		"to": map[string]string{
			"address":      strings.TrimSpace(toEmail),
			"display_name": strings.TrimSpace(toUsername),
		},
		"subject": subject,
		"plain":   plain,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal maileroo payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build maileroo request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("maileroo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("maileroo returned status %d", resp.StatusCode)
	}
	return nil
}
