package models

import "time"

// OAuthApp represents a registered OAuth application owned by a user or organization.
type OAuthApp struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	ClientID         string `gorm:"uniqueIndex;size:40;not null" json:"client_id"`
	ClientSecretHash string `gorm:"size:100;not null" json:"-"` // bcrypt hash, never exposed
	Name             string `gorm:"size:255;not null" json:"name"`
	Description      string `gorm:"size:1000" json:"description"`
	HomepageURL      string `gorm:"size:500;not null" json:"homepage_url"`
	CallbackURL      string `gorm:"size:500;not null" json:"callback_url"`
	LogoURL          string `gorm:"size:500" json:"logo_url"`
	EnableDeviceFlow bool   `gorm:"default:false" json:"enable_device_flow"`

	// OwnerID is either a User.ID or Organization.ID depending on OwnerType.
	OwnerID   string `gorm:"not null;index" json:"owner_id"`
	OwnerType string `gorm:"size:10;not null;default:'user'" json:"owner_type"` // "user" or "org"

	// Populated on demand, not stored.
	AuthorizationCount int `gorm:"-" json:"authorization_count,omitempty"`
}

// OAuthAuthorization records that a user has authorized an OAuth app.
// There is at most one authorization per (app, user) pair.
type OAuthAuthorization struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	AppID  string `gorm:"not null;uniqueIndex:idx_oauth_auth_app_user" json:"app_id"`
	UserID string `gorm:"not null;uniqueIndex:idx_oauth_auth_app_user" json:"user_id"`
	Scopes string `gorm:"type:text;not null;default:'[]'" json:"scopes"` // JSON array of scope strings

	App  OAuthApp `gorm:"foreignKey:AppID" json:"app"`
	User User     `gorm:"foreignKey:UserID" json:"-"`
}
