package models

import "time"

// Session tracks an authenticated user session. A row is created on every
// successful login/register and deleted when the user explicitly logs out or
// when all sessions are revoked (e.g. password change).
type Session struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	UserID string `gorm:"not null;index" json:"user_id"`

	// TokenID is the JWT `jti` claim Ã¢â‚¬â€ a unique identifier per token so we can
	// target a single session for revocation without touching token_version.
	TokenID string `gorm:"size:36;not null;uniqueIndex" json:"token_id"`

	// IP address of the client at login time.
	IPAddress string `gorm:"size:45" json:"ip_address"`

	// Raw User-Agent string for display purposes.
	UserAgent string `gorm:"type:text" json:"user_agent"`

	// Friendly parsed fields for the UI.
	Browser  string `gorm:"size:64" json:"browser"`
	OS       string `gorm:"size:64" json:"os"`
	IsMobile bool   `gorm:"not null;default:false" json:"is_mobile"`

	// LastSeenAt is refreshed on each authenticated request.
	LastSeenAt time.Time `json:"last_seen_at"`

	// RevokedAt is set when a session is individually revoked (not nil = invalid).
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}
