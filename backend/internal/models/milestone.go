package models

import "time"

type Milestone struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Title       string     `gorm:"size:255;not null" json:"title"`
	Description string     `json:"description"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	Status      string     `gorm:"size:20;not null;default:'open'" json:"status"`

	RepoID string     `gorm:"not null;index" json:"repo_id"`
	Repo   Repository `gorm:"foreignKey:RepoID" json:"-"`
}
