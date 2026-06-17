package models

import "time"

type Star struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	UserID string     `gorm:"not null;uniqueIndex:idx_star_user_repo" json:"user_id"`
	RepoID string     `gorm:"not null;uniqueIndex:idx_star_user_repo" json:"repo_id"`
	User   User       `json:"-"`
	Repo   Repository `gorm:"foreignKey:RepoID;OnDelete:CASCADE" json:"repo"`
}
