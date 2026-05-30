package models

import "time"

// RepoVariable stores a plain-text repository variable available in Actions workflows.
// Accessed in workflow YAML as ${{ vars.NAME }}.
type RepoVariable struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	RepoID string `gorm:"not null;uniqueIndex:idx_repo_var_name" json:"repo_id"`
	Name   string `gorm:"not null;uniqueIndex:idx_repo_var_name" json:"name"`
	Value  string `gorm:"not null" json:"value"`
}

// RepoSecret stores an AES-256-GCM encrypted repository secret available in Actions workflows.
// Accessed in workflow YAML as ${{ secrets.NAME }}.
// The encrypted value is never returned in API responses.
type RepoSecret struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	RepoID         string `gorm:"not null;uniqueIndex:idx_repo_secret_name" json:"repo_id"`
	Name           string `gorm:"not null;uniqueIndex:idx_repo_secret_name" json:"name"`
	EncryptedValue string `gorm:"not null" json:"-"` // never serialised
}
