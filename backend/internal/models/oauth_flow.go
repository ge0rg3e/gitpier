package models

import "time"

// OAuthCode is a short-lived single-use authorization code issued after the user
// approves an OAuth app. Matches GitHub's authorization_code grant type.
// Expires after 10 minutes.
type OAuthCode struct {
	ID        string `gorm:"primarykey"`
	CreatedAt time.Time

	Code            string    `gorm:"uniqueIndex;size:40;not null"`
	AppID           string    `gorm:"not null;index"`
	UserID          string    `gorm:"not null"`
	Scopes          string    `gorm:"type:text;not null"` // space-delimited
	RedirectURI     string    `gorm:"size:500"`
	CodeChallenge   string    `gorm:"size:128"` // PKCE
	ChallengeMethod string    `gorm:"size:10"`  // "S256"
	ExpiresAt       time.Time `gorm:"not null"`
	Used            bool      `gorm:"default:false"`
}

// OAuthToken is an issued access token that third-party apps use to call the API
// on behalf of a user. The actual token string is never stored ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â only its SHA-256
// hash is kept so that a DB breach doesn't expose live tokens.
// Token format: "glo_<40 hex chars>" where glo = gitpier OAuth.
type OAuthToken struct {
	ID        string `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	TokenHash string `gorm:"uniqueIndex;size:64;not null"` // SHA-256 hex of plaintext token
	AppID     string `gorm:"not null;index"`
	UserID    string `gorm:"not null;index"`
	Scopes    string `gorm:"type:text;not null"` // space-delimited

	App  OAuthApp `gorm:"foreignKey:AppID"`
	User User     `gorm:"foreignKey:UserID"`
}

// OAuthDeviceCode supports the Device Authorization Grant (RFC 8628).
// Used by headless apps (CLI tools, etc.) that cannot open a browser.
type OAuthDeviceCode struct {
	ID        string `gorm:"primarykey"`
	CreatedAt time.Time

	DeviceCode string    `gorm:"uniqueIndex;size:40;not null"`
	UserCode   string    `gorm:"uniqueIndex;size:9;not null"` // "XXXX-XXXX"
	AppID      string    `gorm:"not null;index"`
	Scopes     string    `gorm:"type:text;not null"` // space-delimited
	ExpiresAt  time.Time `gorm:"not null"`

	// Polling interval tracking to enforce slow-down
	LastPolledAt *time.Time
	Interval     int `gorm:"default:5"` // minimum seconds between polls

	// Set when the user has approved or denied on the browser
	ApprovedByUserID *string
	Denied           bool `gorm:"default:false"`
}
