package models

import "time"

const (
	ModerationScopeUser = "user"
	ModerationScopeOrg  = "org"
	ModerationScopeRepo = "repo"

	KeywordApplyAll     = "all"
	KeywordApplyIssues  = "issues"
	KeywordApplyPRs     = "prs"
	KeywordApplyCommits = "commits"
)

// ModerationPolicy holds all moderation settings for a user, org, or repo.
// Exactly one of UserID / OrgID / RepoID will be non-nil.
type ModerationPolicy struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Scope ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â exactly one is set
	UserID *string `gorm:"uniqueIndex;index" json:"user_id,omitempty"`
	OrgID  *string `gorm:"uniqueIndex;index" json:"org_id,omitempty"`
	RepoID *string `gorm:"uniqueIndex;index" json:"repo_id,omitempty"`

	// Repo-only: when true the owner (user or org) policy rules are also applied
	InheritFromOwner bool `gorm:"default:true" json:"inherit_from_owner"`

	BlockIssues   bool `gorm:"default:false" json:"block_issues"`
	BlockPRs      bool `gorm:"default:false" json:"block_prs"`
	BlockPushes   bool `gorm:"default:false" json:"block_pushes"`
	BlockComments bool `gorm:"default:false" json:"block_comments"`

	MaxIssuesPerDay   int `gorm:"default:0" json:"max_issues_per_day"`
	MaxPRsPerDay      int `gorm:"default:0" json:"max_prs_per_day"`
	MaxCommentsPerDay int `gorm:"default:0" json:"max_comments_per_day"`

	MinAccountAgeDays int `gorm:"default:0" json:"min_account_age_days"`

	RequireMinActivity bool `gorm:"default:false" json:"require_min_activity"`
	MinCommits         int  `gorm:"default:0" json:"min_commits"`
	MinContributions   int  `gorm:"default:0" json:"min_contributions"`

	BlockedUsers    []ModerationBlockedUser    `gorm:"foreignKey:PolicyID" json:"blocked_users,omitempty"`
	BlockedKeywords []ModerationBlockedKeyword `gorm:"foreignKey:PolicyID" json:"blocked_keywords,omitempty"`
}

// ModerationBlockedUser blocks a specific user from interacting with the scope.
type ModerationBlockedUser struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	PolicyID string `gorm:"not null;index;uniqueIndex:idx_mod_policy_user" json:"policy_id"`
	UserID   string `gorm:"not null;uniqueIndex:idx_mod_policy_user" json:"user_id"`
	Reason   string `json:"reason"`

	User User `json:"user"`
}

// ModerationBlockedKeyword blocks content containing a keyword.
type ModerationBlockedKeyword struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	PolicyID string `gorm:"not null;index" json:"policy_id"`
	Keyword  string `gorm:"not null" json:"keyword"`
	// all | issues | prs | commits
	ApplyTo string `gorm:"not null;default:all" json:"apply_to"`
}
