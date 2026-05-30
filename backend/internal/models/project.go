package models

import "time"

const (
	ProjectOwnerTypeUser = "user"
	ProjectOwnerTypeOrg  = "org"
)

type Project struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Title       string `gorm:"size:255;not null" json:"title"`
	Description string `json:"description"`
	IsPublic    bool   `gorm:"not null;default:true" json:"is_public"`

	OwnerUserID *string       `gorm:"index" json:"owner_user_id,omitempty"`
	OwnerUser   *User         `gorm:"foreignKey:OwnerUserID" json:"owner_user,omitempty"`
	OwnerOrgID  *string       `gorm:"index" json:"owner_org_id,omitempty"`
	OwnerOrg    *Organization `gorm:"foreignKey:OwnerOrgID" json:"owner_org,omitempty"`

	CreatedByID string `gorm:"not null;index" json:"created_by_id"`
	CreatedBy   User   `gorm:"foreignKey:CreatedByID" json:"created_by"`

	Columns []ProjectColumn `gorm:"foreignKey:ProjectID" json:"columns,omitempty"`
}

type ProjectColumn struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	ProjectID string        `gorm:"not null;index" json:"project_id"`
	Name      string        `gorm:"size:80;not null" json:"name"`
	Description string      `json:"description"`
	Color     string        `gorm:"size:20;not null;default:'#0ea5e9'" json:"color"`
	Position  int           `gorm:"not null;default:0" json:"position"`
	Items     []ProjectItem `gorm:"foreignKey:ColumnID" json:"items,omitempty"`
}

type ProjectItem struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	ProjectID string `gorm:"not null;index" json:"project_id"`
	ColumnID  string `gorm:"not null;index" json:"column_id"`
	Title     string `gorm:"size:255;not null" json:"title"`
	Body      string `json:"body"`
	Position  int    `gorm:"not null;default:0" json:"position"`

	AssigneeUserID *string `gorm:"index" json:"assignee_user_id,omitempty"`
	AssigneeUser   *User   `gorm:"foreignKey:AssigneeUserID" json:"assignee_user,omitempty"`
}
