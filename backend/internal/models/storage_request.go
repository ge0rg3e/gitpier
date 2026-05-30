package models

import "time"

const (
	StorageRequestStatusPending  = "pending"
	StorageRequestStatusApproved = "approved"
	StorageRequestStatusRejected = "rejected"
)

type StorageIncreaseRequest struct {
	ID                string     `gorm:"primarykey" json:"id"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	RepoID            string     `gorm:"not null;index" json:"repo_id"`
	RequestedByUserID string     `gorm:"not null;index" json:"requested_by_user_id"`
	RequestedLimit    int64      `gorm:"not null;default:0" json:"requested_limit_bytes"`
	Message           string     `gorm:"type:text" json:"message"`
	Status            string     `gorm:"size:20;not null;default:pending;index" json:"status"`
	ReviewNote        string     `gorm:"type:text" json:"review_note"`
	ReviewedByUserID  *string    `gorm:"index" json:"reviewed_by_user_id,omitempty"`
	ReviewedAt        *time.Time `json:"reviewed_at,omitempty"`

	Repo            Repository `gorm:"foreignKey:RepoID" json:"repo"`
	RequestedByUser User       `gorm:"foreignKey:RequestedByUserID" json:"requested_by_user"`
	ReviewedByUser  *User      `gorm:"foreignKey:ReviewedByUserID" json:"reviewed_by_user,omitempty"`
}
