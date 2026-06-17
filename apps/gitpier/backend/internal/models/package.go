package models

import "time"

// ContainerRepository is an OCI image namespace: <namespace>/<name>
// where namespace is a username or org name.
type ContainerRepository struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Namespace string `gorm:"uniqueIndex:idx_container_repo;size:200;not null" json:"namespace"`
	Name      string `gorm:"uniqueIndex:idx_container_repo;size:200;not null" json:"name"`
	IsPublic  bool   `gorm:"not null;default:true" json:"is_public"`
	OwnerID   string `gorm:"not null;index" json:"owner_id"`
	OwnerType string `gorm:"size:10;not null" json:"owner_type"` // "user" or "org"
}

// ContainerBlob is a content-addressable blob (layer or config).
// Blobs are shared across repositories (de-duplicated by digest).
type ContainerBlob struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	Digest string `gorm:"uniqueIndex;not null" json:"digest"` // e.g. "sha256:abc123..."
	Size   int64  `gorm:"not null" json:"size"`
	Path   string `gorm:"not null" json:"-"` // filesystem path, never exposed
}

// ContainerUpload tracks an in-progress chunked blob upload session.
type ContainerUpload struct {
	ID        string `gorm:"primarykey"`
	CreatedAt time.Time

	UUID      string `gorm:"uniqueIndex;not null"`
	Namespace string `gorm:"not null"`
	ImageName string `gorm:"not null"`
	Offset    int64  `gorm:"not null;default:0"`
	Path      string `gorm:"not null"` // temp file path
}

// ContainerManifest stores an OCI/Docker manifest (by digest).
// Each (namespace, image name) pair can have many manifests.
type ContainerManifest struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	Namespace string `gorm:"index:idx_manifest_repo;not null" json:"namespace"`
	ImageName string `gorm:"index:idx_manifest_repo;not null" json:"image_name"`
	Digest    string `gorm:"uniqueIndex;not null" json:"digest"` // "sha256:..."
	MediaType string `gorm:"not null" json:"media_type"`
	Content   string `gorm:"type:text;not null" json:"-"`
	Size      int64  `gorm:"not null" json:"size"`
}

// ContainerTag maps a mutable tag to a manifest digest.
type ContainerTag struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Namespace string `gorm:"uniqueIndex:idx_container_tag;not null" json:"namespace"`
	ImageName string `gorm:"uniqueIndex:idx_container_tag;not null" json:"image_name"`
	Tag       string `gorm:"uniqueIndex:idx_container_tag;not null" json:"tag"`
	Digest    string `gorm:"not null" json:"digest"`
	PullCount int64  `gorm:"not null;default:0" json:"pull_count"`
}
