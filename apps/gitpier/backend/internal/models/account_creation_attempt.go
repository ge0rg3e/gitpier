package models

import "time"

// AccountCreationAttempt tracks registration attempts to prevent spam and abuse.
// Used for rate limiting account creation from a single IP address.
type AccountCreationAttempt struct {
	ID        string    `gorm:"primarykey"`
	CreatedAt time.Time `gorm:"index:idx_ip_time;index:idx_email_time"`
	UpdatedAt time.Time

	// IPAddress is the registration attempt IP
	IPAddress string `gorm:"size:45;not null;index:idx_ip_time"`

	// Email attempted to be registered
	Email string `gorm:"size:254;index:idx_email_time"`

	// Success indicates if the account was created or just attempted
	Success bool `gorm:"default:false"`

	// UserAgent of the registration attempt
	UserAgent string `gorm:"type:text"`
}

// TableName overrides the default table name
func (AccountCreationAttempt) TableName() string {
	return "account_creation_attempts"
}

// AccountCreationLimit contains configured limits for account creation
// This helps with rate limiting and abuse prevention
const (
	// AccountsPerIPPer24Hours is the max accounts that can be created from one IP in 24 hours
	AccountsPerIPPer24Hours = 3

	// AccountsPerIPPer7Days is the max accounts that can be created from one IP in 7 days
	AccountsPerIPPer7Days = 5

	// AccountsPerFingerprintPer24Hours is the max accounts from one device fingerprint in 24 hours
	AccountsPerFingerprintPer24Hours = 2
)
