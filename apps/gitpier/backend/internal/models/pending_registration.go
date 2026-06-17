package models

import "time"

// PendingRegistration stores unverified signup intents until the email OTP is confirmed.
type PendingRegistration struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	RegistrationToken    string    `gorm:"uniqueIndex;size:128;not null" json:"-"`
	Username             string    `gorm:"index;size:39;not null" json:"-"`
	Email                string    `gorm:"index;size:254;not null" json:"-"`
	PasswordHash         string    `gorm:"type:text;not null" json:"-"`
	GDPRConsentIP        string    `gorm:"size:45;not null" json:"-"`
	OTPHash              string    `gorm:"type:text;not null" json:"-"`
	OTPExpiresAt         time.Time `gorm:"index;not null" json:"-"`
	VerificationAttempts int       `gorm:"not null;default:0" json:"-"`
	RequestIPAddress     string    `gorm:"size:45" json:"-"`
	RequestUserAgent     string    `gorm:"size:512" json:"-"`
}

func (PendingRegistration) TableName() string {
	return "pending_registrations"
}
