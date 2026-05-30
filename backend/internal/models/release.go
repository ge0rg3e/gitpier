package models

import "time"

// Release represents a tagged release of a repository.
type Release struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	RepoID       string     `gorm:"not null;index" json:"repo_id"`
	Repo         Repository `gorm:"foreignKey:RepoID" json:"repo,omitempty"`
	TagName      string     `gorm:"not null" json:"tag_name"`
	TargetCommit string     `gorm:"not null;default:''" json:"target_commit"` // branch or commit SHA
	Name         string     `gorm:"not null;default:''" json:"name"`
	Body         string     `gorm:"type:text;default:''" json:"body"` // markdown release notes
	IsDraft      bool       `gorm:"not null;default:false" json:"is_draft"`
	IsPrerelease bool       `gorm:"not null;default:false" json:"is_prerelease"`
	PublishedAt  *time.Time `json:"published_at"`
	CreatedByID  string     `gorm:"not null" json:"created_by_id"`
	CreatedBy    User       `gorm:"foreignKey:CreatedByID" json:"created_by,omitempty"`

	Assets []ReleaseAsset `gorm:"foreignKey:ReleaseID" json:"assets,omitempty"`
}

// ReleaseAsset is a binary file attached to a release.
type ReleaseAsset struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	ReleaseID     string `gorm:"not null;index" json:"release_id"`
	Name          string `gorm:"not null" json:"name"`
	Size          int64  `gorm:"not null;default:0" json:"size"`
	ContentType   string `gorm:"not null;default:'application/octet-stream'" json:"content_type"`
	StoragePath   string `gorm:"not null" json:"-"` // never expose filesystem path to clients
	DownloadCount int    `gorm:"not null;default:0" json:"download_count"`
}
