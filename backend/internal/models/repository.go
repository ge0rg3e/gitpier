package models

import (
	"time"
)

type Repository struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Name           string     `gorm:"size:100;not null" json:"name"`
	Description    string     `json:"description"`
	IsPrivate      bool       `gorm:"default:false" json:"is_private"`
	IsArchived     bool       `gorm:"not null;default:false" json:"is_archived"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
	DefaultBranch  string     `gorm:"default:main" json:"default_branch"`
	Website        string     `gorm:"size:500;default:''" json:"website"`
	Language       string     `gorm:"size:100;default:''" json:"language,omitempty"`
	Size           int64      `gorm:"default:0" json:"size"` // Repository size in bytes
	SizeLimitBytes int64      `gorm:"-" json:"size_limit_bytes,omitempty"`
	IsSuspended    bool       `gorm:"not null;default:false" json:"is_suspended"`

	OwnerID string        `gorm:"not null;index" json:"owner_id"`
	Owner   User          `json:"owner"`
	OrgID   *string       `gorm:"index" json:"org_id,omitempty"`
	Org     *Organization `gorm:"foreignKey:OrgID" json:"org,omitempty"`

	ForkedFromRepoID *string     `gorm:"index" json:"forked_from_repo_id,omitempty"`
	ForkedFromRepo   *Repository `gorm:"foreignKey:ForkedFromRepoID" json:"forked_from_repo,omitempty"`
	ForkCount        int64       `gorm:"-" json:"fork_count"`
	ActivitySeries   []int       `gorm:"-" json:"activity_series,omitempty"`

	Collaborators []Collaborator `gorm:"foreignKey:RepoID" json:"-"`
}
