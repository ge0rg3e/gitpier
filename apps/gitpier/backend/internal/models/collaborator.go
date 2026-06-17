package models

import "time"

// Permission levels
const (
	PermissionRead  = "read"
	PermissionWrite = "write"
	PermissionAdmin = "admin"
)

type Collaborator struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	RepoID     string `gorm:"not null;uniqueIndex:idx_repo_user" json:"repo_id"`
	UserID     string `gorm:"not null;uniqueIndex:idx_repo_user" json:"user_id"`
	Permission string `gorm:"not null;default:read" json:"permission"` // read, write, admin

	User User       `json:"user"`
	Repo Repository `gorm:"foreignKey:RepoID" json:"-"`
}
