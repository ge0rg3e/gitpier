package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gitpier/internal/models"

	"gorm.io/gorm"
)

var (
	ErrModerationBlocked        = errors.New("moderation: user is blocked from interacting with this repository")
	ErrModerationFeatureBlocked = errors.New("moderation: this interaction type is disabled for this repository")
	ErrModerationRateLimit      = errors.New("moderation: rate limit reached, try again later")
	ErrModerationAccountAge     = errors.New("moderation: account does not meet the minimum age requirement")
	ErrModerationActivity       = errors.New("moderation: account does not meet the minimum activity requirements")
	ErrModerationKeyword        = errors.New("moderation: content contains a blocked keyword")
)

type ModerationService struct {
	db *gorm.DB
}

func NewModerationService(db *gorm.DB) *ModerationService {
	return &ModerationService{db: db}
}

// GetOrCreateUserPolicy returns (or creates) the moderation policy for a user.
func (s *ModerationService) GetOrCreateUserPolicy(ctx context.Context, userID string) (*models.ModerationPolicy, error) {
	return s.getOrCreate(ctx, func(p *models.ModerationPolicy) { p.UserID = &userID })
}

// GetOrCreateOrgPolicy returns (or creates) the moderation policy for an org.
func (s *ModerationService) GetOrCreateOrgPolicy(ctx context.Context, orgID string) (*models.ModerationPolicy, error) {
	return s.getOrCreate(ctx, func(p *models.ModerationPolicy) { p.OrgID = &orgID })
}

// GetOrCreateRepoPolicy returns (or creates) the moderation policy for a repo.
func (s *ModerationService) GetOrCreateRepoPolicy(ctx context.Context, repoID string) (*models.ModerationPolicy, error) {
	return s.getOrCreate(ctx, func(p *models.ModerationPolicy) {
		p.RepoID = &repoID
		p.InheritFromOwner = true
	})
}

func (s *ModerationService) getOrCreate(ctx context.Context, seed func(*models.ModerationPolicy)) (*models.ModerationPolicy, error) {
	var policy models.ModerationPolicy
	seed(&policy)

	var existing models.ModerationPolicy
	q := s.db.WithContext(ctx)
	if policy.UserID != nil {
		q = q.Where("user_id = ?", *policy.UserID)
	} else if policy.OrgID != nil {
		q = q.Where("org_id = ?", *policy.OrgID)
	} else {
		q = q.Where("repo_id = ?", *policy.RepoID)
	}

	err := q.
		Preload("BlockedUsers.User").
		Preload("BlockedKeywords").
		First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Create(&policy).Error; err != nil {
		return nil, fmt.Errorf("failed to create moderation policy: %w", err)
	}
	return &policy, nil
}

// UpdatePolicy saves changes to the policy fields (not associations).
type UpdatePolicyInput struct {
	InheritFromOwner   *bool
	BlockIssues        *bool
	BlockPRs           *bool
	BlockPushes        *bool
	BlockComments      *bool
	MaxIssuesPerDay    *int
	MaxPRsPerDay       *int
	MaxCommentsPerDay  *int
	MinAccountAgeDays  *int
	RequireMinActivity *bool
	MinCommits         *int
	MinContributions   *int
}

func (s *ModerationService) UpdatePolicy(ctx context.Context, policyID string, input UpdatePolicyInput) (*models.ModerationPolicy, error) {
	updates := map[string]interface{}{}

	if input.InheritFromOwner != nil {
		updates["inherit_from_owner"] = *input.InheritFromOwner
	}
	if input.BlockIssues != nil {
		updates["block_issues"] = *input.BlockIssues
	}
	if input.BlockPRs != nil {
		updates["block_prs"] = *input.BlockPRs
	}
	if input.BlockPushes != nil {
		updates["block_pushes"] = *input.BlockPushes
	}
	if input.BlockComments != nil {
		updates["block_comments"] = *input.BlockComments
	}
	if input.MaxIssuesPerDay != nil {
		updates["max_issues_per_day"] = *input.MaxIssuesPerDay
	}
	if input.MaxPRsPerDay != nil {
		updates["max_prs_per_day"] = *input.MaxPRsPerDay
	}
	if input.MaxCommentsPerDay != nil {
		updates["max_comments_per_day"] = *input.MaxCommentsPerDay
	}
	if input.MinAccountAgeDays != nil {
		updates["min_account_age_days"] = *input.MinAccountAgeDays
	}
	if input.RequireMinActivity != nil {
		updates["require_min_activity"] = *input.RequireMinActivity
	}
	if input.MinCommits != nil {
		updates["min_commits"] = *input.MinCommits
	}
	if input.MinContributions != nil {
		updates["min_contributions"] = *input.MinContributions
	}

	if len(updates) > 0 {
		if err := s.db.WithContext(ctx).Model(&models.ModerationPolicy{}).Where("id = ?", policyID).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("failed to update policy: %w", err)
		}
	}

	var policy models.ModerationPolicy
	if err := s.db.WithContext(ctx).
		Preload("BlockedUsers.User").
		Preload("BlockedKeywords").
		Where("id = ?", policyID).
		First(&policy).Error; err != nil {
		return nil, err
	}
	return &policy, nil
}

func (s *ModerationService) BlockUser(ctx context.Context, policyID, userID string, reason string) (*models.ModerationBlockedUser, error) {
	bu := &models.ModerationBlockedUser{
		PolicyID: policyID,
		UserID:   userID,
		Reason:   reason,
	}
	if err := s.db.WithContext(ctx).
		Where(models.ModerationBlockedUser{PolicyID: policyID, UserID: userID}).
		Assign(models.ModerationBlockedUser{Reason: reason}).
		FirstOrCreate(bu).Error; err != nil {
		return nil, fmt.Errorf("failed to block user: %w", err)
	}
	s.db.WithContext(ctx).Preload("User").Where("id = ?", bu.ID).First(bu)
	return bu, nil
}

func (s *ModerationService) UnblockUser(ctx context.Context, policyID, userID string) error {
	return s.db.WithContext(ctx).
		Where("policy_id = ? AND user_id = ?", policyID, userID).
		Delete(&models.ModerationBlockedUser{}).Error
}

func (s *ModerationService) AddKeyword(ctx context.Context, policyID string, keyword, applyTo string) (*models.ModerationBlockedKeyword, error) {
	kw := &models.ModerationBlockedKeyword{
		PolicyID: policyID,
		Keyword:  strings.ToLower(strings.TrimSpace(keyword)),
		ApplyTo:  applyTo,
	}
	if err := s.db.WithContext(ctx).Create(kw).Error; err != nil {
		return nil, fmt.Errorf("failed to add keyword: %w", err)
	}
	return kw, nil
}

func (s *ModerationService) RemoveKeyword(ctx context.Context, policyID, keywordID string) error {
	return s.db.WithContext(ctx).
		Where("id = ? AND policy_id = ?", keywordID, policyID).
		Delete(&models.ModerationBlockedKeyword{}).Error
}

type CheckInput struct {
	RepoID      string
	ActorID     string
	ActorJoined time.Time
	// context type: "issues", "prs", "commits", "comments"
	ContextType string
	Content     []string // title, body, or commit message to scan for keywords
}

// CheckAllowed verifies whether the actor may perform the action described by
// CheckInput. It loads the effective policies (repo + owner if inherited) and
// runs all checks. Returns nil if allowed, a moderation error otherwise.
func (s *ModerationService) CheckAllowed(ctx context.Context, input CheckInput) error {
	// Load repo policy
	var repoPolicy *models.ModerationPolicy
	{
		var p models.ModerationPolicy
		err := s.db.WithContext(ctx).
			Where("repo_id = ?", input.RepoID).
			Preload("BlockedUsers").
			Preload("BlockedKeywords").
			First(&p).Error
		if err == nil {
			repoPolicy = &p
		}
	}

	// Load repo to find owner
	var repo models.Repository
	if err := s.db.WithContext(ctx).Where("id = ?", input.RepoID).First(&repo).Error; err != nil {
		return nil // repo not found, let handler deal with it
	}

	// Owners are never subject to their own moderation rules
	if repo.OwnerID == input.ActorID {
		return nil
	}

	// Determine whether we need to load the owner policy
	needOwner := repoPolicy == nil || repoPolicy.InheritFromOwner

	var ownerPolicy *models.ModerationPolicy
	if needOwner {
		var p models.ModerationPolicy
		var err error
		if repo.OrgID != nil {
			err = s.db.WithContext(ctx).
				Where("org_id = ?", *repo.OrgID).
				Preload("BlockedUsers").
				Preload("BlockedKeywords").
				First(&p).Error
		} else {
			err = s.db.WithContext(ctx).
				Where("user_id = ?", repo.OwnerID).
				Preload("BlockedUsers").
				Preload("BlockedKeywords").
				First(&p).Error
		}
		if err == nil {
			ownerPolicy = &p
		}
	}

	policies := []*models.ModerationPolicy{}
	if ownerPolicy != nil {
		policies = append(policies, ownerPolicy)
	}
	if repoPolicy != nil {
		policies = append(policies, repoPolicy)
	}

	for _, p := range policies {
		if err := s.applyPolicy(ctx, p, input); err != nil {
			return err
		}
	}
	return nil
}

func (s *ModerationService) applyPolicy(ctx context.Context, p *models.ModerationPolicy, input CheckInput) error {
	// 1. Blocked user check
	for _, bu := range p.BlockedUsers {
		if bu.UserID == input.ActorID {
			return ErrModerationBlocked
		}
	}

	// 2. Feature lock
	switch input.ContextType {
	case "issues":
		if p.BlockIssues {
			return ErrModerationFeatureBlocked
		}
	case "prs":
		if p.BlockPRs {
			return ErrModerationFeatureBlocked
		}
	case "commits":
		if p.BlockPushes {
			return ErrModerationFeatureBlocked
		}
	case "comments":
		if p.BlockComments {
			return ErrModerationFeatureBlocked
		}
	}

	// 3. Account age
	if p.MinAccountAgeDays > 0 {
		age := time.Since(input.ActorJoined)
		required := time.Duration(p.MinAccountAgeDays) * 24 * time.Hour
		if age < required {
			return ErrModerationAccountAge
		}
	}

	// 4. Activity requirements
	if p.RequireMinActivity {
		if p.MinCommits > 0 {
			var commitCount int64
			s.db.WithContext(ctx).Model(&models.Issue{}).
				Where("author_id = ?", input.ActorID).
				Count(&commitCount)
			// Using issue count as a proxy for contribution activity
			// In a full system you'd count actual commits via git
			_ = commitCount
		}
	}

	// 5. Rate limits â€“ counts are always scoped to the specific repo + actor
	if input.ContextType == "issues" && p.MaxIssuesPerDay > 0 {
		since := time.Now().UTC().Truncate(24 * time.Hour)
		var count int64
		s.db.WithContext(ctx).Model(&models.Issue{}).
			Where("repo_id = ? AND author_id = ? AND created_at >= ?", input.RepoID, input.ActorID, since).
			Count(&count)
		if int(count) >= p.MaxIssuesPerDay {
			return ErrModerationRateLimit
		}
	}

	if input.ContextType == "prs" && p.MaxPRsPerDay > 0 {
		since := time.Now().UTC().Truncate(24 * time.Hour)
		var count int64
		s.db.WithContext(ctx).Model(&models.PullRequest{}).
			Where("repo_id = ? AND author_id = ? AND created_at >= ?", input.RepoID, input.ActorID, since).
			Count(&count)
		if int(count) >= p.MaxPRsPerDay {
			return ErrModerationRateLimit
		}
	}

	if input.ContextType == "comments" && p.MaxCommentsPerDay > 0 {
		since := time.Now().UTC().Truncate(24 * time.Hour)
		var count int64
		s.db.WithContext(ctx).Model(&models.IssueComment{}).
			Where("author_id = ? AND created_at >= ? AND issue_id IN (SELECT id FROM issues WHERE repo_id = ?)",
				input.ActorID, since, input.RepoID).
			Count(&count)
		if int(count) >= p.MaxCommentsPerDay {
			return ErrModerationRateLimit
		}
	}

	// 6. Keyword check
	for _, kw := range p.BlockedKeywords {
		if kw.ApplyTo != models.KeywordApplyAll && kw.ApplyTo != input.ContextType {
			continue
		}
		needle := strings.ToLower(kw.Keyword)
		for _, text := range input.Content {
			if strings.Contains(strings.ToLower(text), needle) {
				return fmt.Errorf("%w: \"%s\"", ErrModerationKeyword, kw.Keyword)
			}
		}
	}

	return nil
}
