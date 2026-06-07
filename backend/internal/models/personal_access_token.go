package models

import "time"

type PersonalAccessToken struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	UserID string `gorm:"not null;index" json:"user_id"`
	Name   string `gorm:"size:255;not null" json:"name"`

	TokenHash string `gorm:"size:64;not null;uniqueIndex" json:"-"`
	TokenLast string `gorm:"size:8;not null" json:"token_last"`
	Scopes    string `gorm:"type:text;not null" json:"scopes"`

	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}
