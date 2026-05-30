package models

import "time"

const (
	FeedbackStatusNew         = "new"
	FeedbackStatusInReview    = "in_review"
	FeedbackStatusImplemented = "implemented"
	FeedbackStatusDismissed   = "dismissed"
)

type Feedback struct {
	ID         string     `gorm:"primarykey" json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	Category   string     `gorm:"size:20;not null" json:"category"`
	Message    string     `gorm:"type:text;not null" json:"message"`
	Status     string     `gorm:"size:20;not null;default:new;index" json:"status"`
	AdminNote  string     `gorm:"type:text" json:"admin_note"`
	UserID     *string    `gorm:"index" json:"user_id,omitempty"`
	User       *User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	ReviewedBy *string    `gorm:"index" json:"reviewed_by_user_id,omitempty"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
}
