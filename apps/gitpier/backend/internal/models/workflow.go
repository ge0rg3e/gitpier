package models

import "time"

// WorkflowRun represents one execution of a workflow file triggered by a git event.
type WorkflowRun struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	RepoID       string     `gorm:"not null;index" json:"repo_id"`
	Repo         Repository `gorm:"foreignKey:RepoID" json:"repo,omitempty"`
	WorkflowName string     `gorm:"not null" json:"workflow_name"`
	WorkflowFile string     `gorm:"not null" json:"workflow_file"`
	Event        string     `gorm:"not null" json:"event"` // push, pull_request
	Branch       string     `gorm:"not null" json:"branch"`
	CommitSHA    string     `gorm:"not null" json:"commit_sha"`
	Status       string     `gorm:"not null;default:'pending'" json:"status"` // pending, running, success, failure, cancelled

	Jobs []WorkflowJob `gorm:"foreignKey:RunID" json:"jobs,omitempty"`
}

// WorkflowJob is one job within a workflow run.
type WorkflowJob struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	RunID      string     `gorm:"not null;index" json:"run_id"`
	Name       string     `gorm:"not null" json:"name"`
	Status     string     `gorm:"not null;default:'pending'" json:"status"` // pending, running, success, failure, skipped
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`

	Steps []WorkflowStep `gorm:"foreignKey:JobID" json:"steps,omitempty"`
}

// WorkflowStep is one step inside a job.
type WorkflowStep struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	JobID      string     `gorm:"not null;index" json:"job_id"`
	Name       string     `gorm:"not null" json:"name"`
	Status     string     `gorm:"not null;default:'pending'" json:"status"` // pending, running, success, failure, skipped
	ExitCode   *int       `json:"exit_code"`
	Log        string     `gorm:"type:text" json:"log"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

// WorkflowUsage tracks monthly run counts per repo for rate limiting.
type WorkflowUsage struct {
	ID       string `gorm:"primarykey" json:"id"`
	RepoID   string `gorm:"not null;uniqueIndex:idx_repo_month" json:"repo_id"`
	Month    string `gorm:"not null;uniqueIndex:idx_repo_month" json:"month"` // "2026-04"
	RunCount int    `gorm:"not null;default:0" json:"run_count"`
}

// WorkflowMinutesUsage tracks monthly Actions minutes usage per billing scope.
// Scope is either a user account or an organization account.
type WorkflowMinutesUsage struct {
	ID          string `gorm:"primarykey" json:"id"`
	ScopeType   string `gorm:"not null;uniqueIndex:idx_workflow_scope_month" json:"scope_type"` // "user" | "org"
	ScopeID     string `gorm:"not null;uniqueIndex:idx_workflow_scope_month" json:"scope_id"`
	Month       string `gorm:"not null;uniqueIndex:idx_workflow_scope_month" json:"month"` // "2026-05"
	MinutesUsed int    `gorm:"not null;default:0" json:"minutes_used"`
}
