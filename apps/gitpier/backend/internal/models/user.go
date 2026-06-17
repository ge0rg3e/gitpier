package models

import (
	"time"
)

const (
	UserRoleUser  = "user"
	UserRoleAdmin = "admin"
)

type User struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Username           string `gorm:"uniqueIndex;size:39;not null" json:"username"`
	DisplayName        string `json:"display_name"`
	Email              string `gorm:"uniqueIndex;not null" json:"email"`
	Password           string `gorm:"not null" json:"-"`
	Bio                string `json:"bio"`
	AvatarURL          string `json:"avatar_url"`
	Location           string `json:"location"`
	Website            string `json:"website"`
	Role               string `gorm:"size:20;not null;default:user" json:"role"`
	IsSuspended        bool   `gorm:"not null;default:false" json:"is_suspended"`
	TwoFAEnabled       bool   `gorm:"not null;default:false" json:"-"`
	TwoFASecret        string `gorm:"type:text" json:"-"`
	TwoFARecoveryCodes string `gorm:"type:text" json:"-"`

	// GDPR fields
	GDPRConsentAt *time.Time `json:"gdpr_consent_at"`
	GDPRConsentIP string     `gorm:"size:45" json:"-"`

	// Token revocation: increment to invalidate all previously issued JWTs.
	TokenVersion int `gorm:"not null;default:0" json:"-"`

	// Brute-force protection: track consecutive login failures.
	FailedLoginAttempts int        `gorm:"not null;default:0" json:"-"`
	LockedUntil         *time.Time `json:"-"`

	// Password reset: token is single-use and expires after 1 hour.
	PasswordResetToken     string     `gorm:"size:64" json:"-"`
	PasswordResetExpiresAt *time.Time `json:"-"`

	// Profile customization
	ProfileWidgets string `gorm:"type:text;default:'{}'" json:"profile_widgets"`

	Repos   []Repository `gorm:"foreignKey:OwnerID" json:"-"`
	SSHKeys []SSHKey     `gorm:"foreignKey:UserID" json:"-"`
}
