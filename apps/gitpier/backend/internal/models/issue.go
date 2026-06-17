package models

import "time"

const (
	IssueStatusOpen   = "open"
	IssueStatusClosed = "closed"
)

type Issue struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Number    uint   `gorm:"not null;index" json:"number"`
	Title     string `gorm:"size:255;not null" json:"title"`
	Body      string `json:"body"`
	Status    string `gorm:"size:20;not null;default:open" json:"status"`
	IssueType string `gorm:"size:50" json:"issue_type"`

	RepoID      string     `gorm:"not null;index" json:"repo_id"`
	Repo        Repository `gorm:"foreignKey:RepoID" json:"repo,omitempty"`
	AuthorID    string     `gorm:"not null;index" json:"author_id"`
	Author      User       `json:"author"`
	AssigneeID  *string    `gorm:"index" json:"assignee_id,omitempty"`
	Assignee    *User      `gorm:"foreignKey:AssigneeID" json:"assignee,omitempty"`
	MilestoneID *string    `gorm:"index" json:"milestone_id,omitempty"`
	Milestone   *Milestone `gorm:"foreignKey:MilestoneID" json:"milestone,omitempty"`

	Labels   []Label        `gorm:"many2many:issue_labels;" json:"labels"`
	Comments []IssueComment `gorm:"foreignKey:IssueID" json:"comments,omitempty"`
}

type Label struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Name        string `gorm:"size:50;not null" json:"name"`
	Color       string `gorm:"size:7;not null;default:'#0075ca'" json:"color"`
	Description string `gorm:"size:255" json:"description"`

	RepoID string `gorm:"not null;index" json:"repo_id"`
}

type IssueComment struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Body string `gorm:"not null" json:"body"`

	IssueID  string `gorm:"not null;index" json:"issue_id"`
	AuthorID string `gorm:"not null;index" json:"author_id"`
	Author   User   `json:"author"`
}
