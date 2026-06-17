package models

import "time"

const (
	PRStatusOpen   = "open"
	PRStatusClosed = "closed"
	PRStatusMerged = "merged"

	PRMergeMethodMerge  = "merge"
	PRMergeMethodSquash = "squash"
	PRMergeMethodRebase = "rebase"

	PRReviewStateApproved         = "APPROVED"
	PRReviewStateChangesRequested = "CHANGES_REQUESTED"
	PRReviewStateCommented        = "COMMENTED"
	PRReviewStateDismissed        = "DISMISSED"
)

type PullRequest struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Number      uint   `gorm:"not null;index" json:"number"`
	Title       string `gorm:"size:255;not null" json:"title"`
	Description string `json:"description"`
	Status      string `gorm:"size:20;not null;default:open" json:"status"`

	HeadRef string `gorm:"size:255;not null" json:"head_ref"`
	BaseRef string `gorm:"size:255;not null" json:"base_ref"`
	HeadSHA string `gorm:"size:40" json:"head_sha"`

	IsDraft bool `gorm:"default:false" json:"is_draft"`

	MergedAt    *time.Time `json:"merged_at,omitempty"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	MergeSHA    string     `gorm:"size:40" json:"merge_sha,omitempty"`
	MergeMethod string     `gorm:"size:20" json:"merge_method,omitempty"`

	RepoID string     `gorm:"not null;index" json:"repo_id"`
	Repo   Repository `gorm:"foreignKey:RepoID" json:"repo,omitempty"`

	HeadRepoID *string     `gorm:"index" json:"head_repo_id,omitempty"`
	HeadRepo   *Repository `gorm:"foreignKey:HeadRepoID" json:"head_repo,omitempty"`

	AuthorID string `gorm:"not null;index" json:"author_id"`
	Author   User   `json:"author"`

	MergedByID *string `gorm:"index" json:"merged_by_id,omitempty"`
	MergedBy   *User   `gorm:"foreignKey:MergedByID" json:"merged_by,omitempty"`

	AssigneeID *string `gorm:"index" json:"assignee_id,omitempty"`
	Assignee   *User   `gorm:"foreignKey:AssigneeID" json:"assignee,omitempty"`
	Labels     []Label `gorm:"many2many:pr_labels;" json:"labels"`
}

type PRComment struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Body string `gorm:"not null" json:"body"`

	PRID     string `gorm:"not null;index" json:"pr_id"`
	AuthorID string `gorm:"not null;index" json:"author_id"`
	Author   User   `json:"author"`
}

// PRReview represents a submitted review (approve / request changes / comment).
type PRReview struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Body      string `json:"body"`
	State     string `gorm:"size:30;not null" json:"state"` // APPROVED | CHANGES_REQUESTED | COMMENTED
	CommitSHA string `gorm:"size:40" json:"commit_sha"`

	PRID     string `gorm:"not null;index" json:"pr_id"`
	AuthorID string `gorm:"not null;index" json:"author_id"`
	Author   User   `json:"author"`

	Comments []PRReviewComment `gorm:"foreignKey:ReviewID" json:"comments,omitempty"`
}

// PRReviewComment is an inline file comment attached to a review.
type PRReviewComment struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Body      string `gorm:"not null" json:"body"`
	Path      string `gorm:"size:500" json:"path"`
	Line      int    `json:"line"`
	Side      string `gorm:"size:5;default:'RIGHT'" json:"side"` // LEFT | RIGHT
	CommitSHA string `gorm:"size:40" json:"commit_sha"`

	ReviewID string `gorm:"not null;index" json:"review_id"`
	PRID     string `gorm:"not null;index" json:"pr_id"`
	AuthorID string `gorm:"not null;index" json:"author_id"`
	Author   User   `json:"author"`
}
