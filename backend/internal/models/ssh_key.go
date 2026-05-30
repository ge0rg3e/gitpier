package models

import "time"

type SSHKey struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	UserID      string `gorm:"not null;index" json:"user_id"`
	Title       string `gorm:"not null" json:"title"`
	Key         string `gorm:"not null;type:text" json:"key"`
	Fingerprint string `gorm:"not null;uniqueIndex" json:"fingerprint"`
}
