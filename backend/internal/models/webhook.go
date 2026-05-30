package models

import "time"

// Webhook represents a repository webhook configuration.
type Webhook struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	RepoID      string `gorm:"not null;index" json:"repo_id"`
	PayloadURL  string `gorm:"size:500;not null" json:"payload_url"`
	ContentType string `gorm:"size:50;not null;default:'application/json'" json:"content_type"`
	Secret      string `gorm:"size:255" json:"-"` // never returned in responses
	InsecureSSL bool   `gorm:"default:false" json:"insecure_ssl"`
	Active      bool   `gorm:"default:true" json:"active"`
	// JSON array of event names e.g. ["push","issues","pull_request"]
	Events string `gorm:"type:text;not null;default:'[\"push\"]'" json:"events"`
}

// WebhookDelivery records a single delivery attempt for a webhook.
type WebhookDelivery struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	WebhookID    string `gorm:"not null;index" json:"webhook_id"`
	GUID         string `gorm:"size:36;not null;uniqueIndex" json:"guid"`
	Event        string `gorm:"size:50;not null" json:"event"`
	Payload      string `gorm:"type:text" json:"payload"`
	ResponseCode int    `json:"response_code"`
	ResponseBody string `gorm:"type:text" json:"response_body"`
	DurationMS   int64  `json:"duration_ms"`
	Success      bool   `json:"success"`
}
