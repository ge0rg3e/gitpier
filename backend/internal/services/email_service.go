package services

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

type RegistrationMailer interface {
	SendRegistrationOTP(ctx context.Context, toEmail, toUsername, otpCode string, expiresAt time.Time) error
}

type SMTPEmailService struct {
	host     string
	port     int
	username string
	password string
	from     string
	fromName string
}

type SMTPEmailConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	FromName string
}

func NewSMTPEmailService(cfg SMTPEmailConfig) *SMTPEmailService {
	return &SMTPEmailService{
		host:     strings.TrimSpace(cfg.Host),
		port:     cfg.Port,
		username: strings.TrimSpace(cfg.Username),
		password: strings.TrimSpace(cfg.Password),
		from:     strings.TrimSpace(cfg.From),
		fromName: strings.TrimSpace(cfg.FromName),
	}
}

func (s *SMTPEmailService) IsConfigured() bool {
	return s != nil && s.host != "" && s.port > 0 && s.from != ""
}

func (s *SMTPEmailService) SendRegistrationOTP(ctx context.Context, toEmail, toUsername, otpCode string, expiresAt time.Time) error {
	if !s.IsConfigured() {
		return fmt.Errorf("smtp email service is not configured")
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

	to := strings.TrimSpace(toEmail)
	if to == "" {
		return fmt.Errorf("recipient email is empty")
	}

	fromHeader := s.from
	if s.fromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", s.fromName, s.from)
	}

	msg := strings.Join([]string{
		fmt.Sprintf("From: %s", fromHeader),
		fmt.Sprintf("To: %s", to),
		fmt.Sprintf("Subject: %s", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		plain,
	}, "\r\n")

	var auth smtp.Auth
	if s.username != "" || s.password != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	if err := smtp.SendMail(addr, auth, s.from, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("smtp send failed: %w", err)
	}
	return nil
}
