package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gitpier/internal/models"

	"gorm.io/gorm"
)

var (
	ErrRepoNotFound = errors.New("repository not found")
	ErrRepoExists   = errors.New("repository already exists")
	ErrAccessDenied = errors.New("access denied")
)

type RepoService struct {
	db        *gorm.DB
	reposPath string
}

func NewRepoService(db *gorm.DB, reposPath string) *RepoService {
	return &RepoService{db: db, reposPath: reposPath}
}

type CreateRepoInput struct {
	Name             string
	Description      string
	IsPrivate        bool
	OwnerID          string
	OrgID            *string // nil for personal repos
	DefaultBranch    string
	ForkedFromRepoID *string
}

func (s *RepoService) Create(ctx context.Context, input CreateRepoInput) (*models.Repository, error) {
	// Check for a duplicate name within the same namespace (org or personal).
	var existing models.Repository
	q := s.db.WithContext(ctx).Where("name = ?", input.Name)
	if input.OrgID != nil {
		q = q.Where("org_id = ?", *input.OrgID)
	} else {
		q = q.Where("owner_id = ? AND org_id IS NULL", input.OwnerID)
	}
	if q.First(&existing).Error == nil {
		return nil, ErrRepoExists
	}

	defaultBranch := input.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	repo := &models.Repository{
		Name:             input.Name,
		Description:      input.Description,
		IsPrivate:        input.IsPrivate,
		OwnerID:          input.OwnerID,
		OrgID:            input.OrgID,
		ForkedFromRepoID: input.ForkedFromRepoID,
		DefaultBranch:    defaultBranch,
	}

	if err := s.db.WithContext(ctx).Create(repo).Error; err != nil {
		return nil, fmt.Errorf("failed to create repository: %w", err)
	}

	if err := s.db.WithContext(ctx).Preload("Owner").Preload("Org").Preload("ForkedFromRepo.Owner").Preload("ForkedFromRepo.Org").Where("id = ?", repo.ID).First(repo).Error; err != nil {
		return nil, err
	}

	return repo, nil
}

// GetByOwnerAndName resolves a repo by namespace (user username or org login) and repo name.
func (s *RepoService) GetByOwnerAndName(ctx context.Context, namespace, repoName string) (*models.Repository, error) {
	var repo models.Repository

	// Try user namespace first (org_id must be NULL for personal repos)
	err := s.db.WithContext(ctx).
		Joins("Owner").
		Where("\"Owner\".username = ? AND repositories.name = ? AND repositories.org_id IS NULL", namespace, repoName).
		Preload("Owner").
		Preload("Org").
		Preload("ForkedFromRepo.Owner").
		Preload("ForkedFromRepo.Org").
		First(&repo).Error
	if err == nil {
		return &repo, nil
	}

	// Try org namespace
	var org models.Organization
	if err2 := s.db.WithContext(ctx).Where("login = ?", namespace).First(&org).Error; err2 != nil {
		return nil, ErrRepoNotFound
	}

	err = s.db.WithContext(ctx).
		Preload("Owner").
		Preload("Org").
		Preload("ForkedFromRepo.Owner").
		Preload("ForkedFromRepo.Org").
		Where("org_id = ? AND name = ?", org.ID, repoName).
		First(&repo).Error
	if err != nil {
		return nil, ErrRepoNotFound
	}
	return &repo, nil
}

func (s *RepoService) GetByID(ctx context.Context, id string) (*models.Repository, error) {
	var repo models.Repository
	if err := s.db.WithContext(ctx).Preload("Owner").Preload("Org").Preload("ForkedFromRepo.Owner").Preload("ForkedFromRepo.Org").Where("id = ?", id).First(&repo).Error; err != nil {
		return nil, ErrRepoNotFound
	}
	return &repo, nil
}

func (s *RepoService) ListByOwner(ctx context.Context, ownerUsername string, includePrivate bool) ([]models.Repository, error) {
	return s.ListByOwnerPaged(ctx, ownerUsername, includePrivate, 0, 0)
}

func (s *RepoService) ListByOwnerPaged(ctx context.Context, ownerUsername string, includePrivate bool, limit, offset int) ([]models.Repository, error) {
	var repos []models.Repository
	q := s.db.WithContext(ctx).Preload("Owner").
		Joins("Owner").
		Where("\"Owner\".username = ? AND repositories.org_id IS NULL", ownerUsername)

	if !includePrivate {
		q = q.Where("repositories.is_private = false")
	}

	q = q.Order("repositories.updated_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}

	if err := q.Find(&repos).Error; err != nil {
		return nil, err
	}
	return repos, nil
}

func (s *RepoService) ListByOrg(ctx context.Context, orgID string, includePrivate bool) ([]models.Repository, error) {
	var repos []models.Repository
	q := s.db.WithContext(ctx).Preload("Owner").Preload("Org").Where("org_id = ?", orgID)
	if !includePrivate {
		q = q.Where("is_private = false")
	}
	if err := q.Order("updated_at DESC").Find(&repos).Error; err != nil {
		return nil, err
	}
	return repos, nil
}

func (s *RepoService) ListPublic(ctx context.Context, limit, offset int) ([]models.Repository, int64, error) {
	var repos []models.Repository
	var count int64

	s.db.WithContext(ctx).Model(&models.Repository{}).Where("is_private = false").Count(&count)

	err := s.db.WithContext(ctx).Preload("Owner").
		Where("is_private = false").
		Order("created_at DESC").
		Limit(limit).Offset(offset).
		Find(&repos).Error

	return repos, count, err
}

func (s *RepoService) Update(ctx context.Context, repo *models.Repository, updates map[string]interface{}) error {
	return s.db.WithContext(ctx).Model(repo).Updates(updates).Error
}

func (s *RepoService) UpdateLanguage(ctx context.Context, repoID string, language string) error {
	return s.db.WithContext(ctx).Model(&models.Repository{}).Where("id = ?", repoID).Update("language", language).Error
}

func (s *RepoService) Delete(ctx context.Context, repo *models.Repository) error {
	// WorkflowStep and WorkflowJob don't have repo_id; delete via subquery
	if err := s.db.WithContext(ctx).
		Where("job_id IN (SELECT id FROM workflow_jobs WHERE run_id IN (SELECT id FROM workflow_runs WHERE repo_id = ?))", repo.ID).
		Delete(&models.WorkflowStep{}).Error; err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).
		Where("run_id IN (SELECT id FROM workflow_runs WHERE repo_id = ?)", repo.ID).
		Delete(&models.WorkflowJob{}).Error; err != nil {
		return err
	}

	// Delete issue_labels first to avoid FK violations with issues and labels
	if err := s.db.WithContext(ctx).Exec("DELETE FROM issue_labels WHERE issue_id IN (SELECT id FROM issues WHERE repo_id = ?) OR label_id IN (SELECT id FROM labels WHERE repo_id = ?)", repo.ID, repo.ID).Error; err != nil {
		return err
	}

	// IssueComment has no repo_id; delete via subquery before issues are deleted
	if err := s.db.WithContext(ctx).
		Where("issue_id IN (SELECT id FROM issues WHERE repo_id = ?)", repo.ID).
		Delete(&models.IssueComment{}).Error; err != nil {
		return err
	}

	tables := []interface{}{
		&models.Star{},
		&models.WorkflowRun{},
		&models.PullRequest{},
		&models.RepoVariable{},
		&models.RepoSecret{},
		&models.Collaborator{},
		&models.WorkflowUsage{},
		&models.Issue{},
		&models.Label{},
	}
	for _, t := range tables {
		if err := s.db.WithContext(ctx).Where("repo_id = ?", repo.ID).Delete(t).Error; err != nil {
			return err
		}
	}
	return s.db.WithContext(ctx).Delete(repo).Error
}

// HasAccess checks if a user can access a repo (write=true for push permission)
// IsAdminAccess returns true when the user has owner or admin-level access to the
// repository regardless of its archive state. Use this for settings and
// management endpoints that should remain accessible on archived repos.
func (s *RepoService) IsAdminAccess(repo *models.Repository, userID string) bool {
	// Personal repo owner.
	if repo.OrgID == nil && repo.OwnerID == userID {
		return true
	}
	// Org owner.
	if repo.OrgID != nil {
		var count int64
		s.db.Model(&models.OrganizationMember{}).
			Where("org_id = ? AND user_id = ? AND role = ?", *repo.OrgID, userID, models.OrgRoleOwner).
			Count(&count)
		if count > 0 {
			return true
		}
	}
	// Admin collaborator.
	var collab models.Collaborator
	err := s.db.Where("repo_id = ? AND user_id = ? AND permission = ?", repo.ID, userID, models.PermissionAdmin).First(&collab).Error
	return err == nil
}

func (s *RepoService) HasAccess(repo *models.Repository, userID string, write bool) bool {
	if write && repo.IsArchived {
		return false
	}

	// For personal repos, owner has full access
	if repo.OrgID == nil && repo.OwnerID == userID {
		return true
	}

	// For org repos, org owners have full access
	if repo.OrgID != nil {
		var count int64
		s.db.Model(&models.OrganizationMember{}).
			Where("org_id = ? AND user_id = ? AND role = ?", *repo.OrgID, userID, models.OrgRoleOwner).
			Count(&count)
		if count > 0 {
			return true
		}
	}

	// Check direct collaborator permissions
	var collab models.Collaborator
	err := s.db.Where("repo_id = ? AND user_id = ?", repo.ID, userID).First(&collab).Error
	if err == nil {
		if !write {
			return collab.Permission == models.PermissionRead ||
				collab.Permission == models.PermissionWrite ||
				collab.Permission == models.PermissionAdmin
		}
		return collab.Permission == models.PermissionWrite ||
			collab.Permission == models.PermissionAdmin
	}

	// For org repos, check team access
	if repo.OrgID != nil {
		var teamRepos []models.TeamRepository
		s.db.Where("repo_id = ?", repo.ID).Preload("Team").Find(&teamRepos)
		for _, tr := range teamRepos {
			var mc int64
			s.db.Model(&models.TeamMember{}).Where("team_id = ? AND user_id = ?", tr.TeamID, userID).Count(&mc)
			if mc > 0 {
				if !write {
					return true
				}
				return tr.Team.Permission == models.PermissionWrite ||
					tr.Team.Permission == models.PermissionAdmin
			}
		}
	}

	return false
}

func (s *RepoService) RepoPath(ownerUsername, repoName string) string {
	return filepath.Join(s.reposPath, ownerUsername, repoName+".git")
}

func (s *RepoService) RepoParentPath(ownerUsername string) string {
	return filepath.Join(s.reposPath, ownerUsername)
}

func (s *RepoService) RepoNamespace(repo *models.Repository) string {
	if repo.Org != nil {
		return repo.Org.Login
	}
	return repo.Owner.Username
}

// Collaborator management

func (s *RepoService) AddCollaborator(ctx context.Context, repoID, userID string, permission string) (*models.Collaborator, error) {
	collab := &models.Collaborator{
		RepoID:     repoID,
		UserID:     userID,
		Permission: permission,
	}

	if err := s.db.WithContext(ctx).Create(collab).Error; err != nil {
		return nil, fmt.Errorf("failed to add collaborator: %w", err)
	}

	s.db.WithContext(ctx).Preload("User").First(collab, collab.ID)
	return collab, nil
}

func (s *RepoService) RemoveCollaborator(ctx context.Context, repoID, userID string) error {
	return s.db.WithContext(ctx).
		Where("repo_id = ? AND user_id = ?", repoID, userID).
		Delete(&models.Collaborator{}).Error
}

func (s *RepoService) ListCollaborators(ctx context.Context, repoID string) ([]models.Collaborator, error) {
	var collabs []models.Collaborator
	err := s.db.WithContext(ctx).Preload("User").
		Where("repo_id = ?", repoID).
		Find(&collabs).Error
	return collabs, err
}

var (
	ErrAlreadyStarred = errors.New("already starred")
	ErrNotStarred     = errors.New("not starred")
)

func (s *RepoService) Star(ctx context.Context, userID, repoID string) (*models.Star, error) {
	var existing models.Star
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND repo_id = ?", userID, repoID).
		First(&existing).Error
	if err == nil {
		return nil, ErrAlreadyStarred
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	star := &models.Star{
		UserID: userID,
		RepoID: repoID,
	}
	if err := s.db.WithContext(ctx).Create(star).Error; err != nil {
		return nil, fmt.Errorf("failed to star repository: %w", err)
	}

	s.db.WithContext(ctx).Preload("Repo.Owner").First(star, star.ID)
	return star, nil
}

func (s *RepoService) Unstar(ctx context.Context, userID, repoID string) error {
	result := s.db.WithContext(ctx).
		Where("user_id = ? AND repo_id = ?", userID, repoID).
		Delete(&models.Star{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotStarred
	}
	return nil
}

func (s *RepoService) IsStarred(ctx context.Context, userID, repoID string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).
		Model(&models.Star{}).
		Where("user_id = ? AND repo_id = ?", userID, repoID).
		Count(&count).Error
	return count > 0, err
}

func (s *RepoService) ListStarred(ctx context.Context, userID string) ([]models.Star, error) {
	var stars []models.Star
	err := s.db.WithContext(ctx).
		Preload("Repo.Owner").
		Preload("Repo.Org").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&stars).Error
	return stars, err
}

func (s *RepoService) GetStarCount(ctx context.Context, repoID string) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).
		Model(&models.Star{}).
		Where("repo_id = ?", repoID).
		Count(&count).Error
	return count, err
}

func (s *RepoService) ListStarsForRepo(ctx context.Context, repoID string) ([]models.Star, error) {
	var stars []models.Star
	err := s.db.WithContext(ctx).
		Select("id", "user_id", "repo_id", "created_at").
		Where("repo_id = ?", repoID).
		Order("created_at ASC").
		Find(&stars).Error
	return stars, err
}

func (s *RepoService) GetForkCount(ctx context.Context, repoID string) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).
		Model(&models.Repository{}).
		Where("forked_from_repo_id = ?", repoID).
		Count(&count).Error
	return count, err
}

func (s *RepoService) ListForksForRepo(ctx context.Context, repoID string, limit int) ([]models.Repository, error) {
	var forks []models.Repository
	q := s.db.WithContext(ctx).
		Preload("Owner").
		Preload("Org").
		Preload("ForkedFromRepo.Owner").
		Preload("ForkedFromRepo.Org").
		Where("forked_from_repo_id = ?", repoID).
		Order("updated_at DESC")

	if limit > 0 {
		q = q.Limit(limit)
	}

	if err := q.Find(&forks).Error; err != nil {
		return nil, err
	}
	return forks, nil
}

// GetSizeLimit returns the size limit in bytes for a repository
// Public repos: 1GB, Private repos: 750MB
func (s *RepoService) GetSizeLimit(repo *models.Repository) int64 {
	if repo.SizeLimitBytes > 0 {
		return repo.SizeLimitBytes
	}
	if repo.IsPrivate {
		return 750 * 1024 * 1024 // 750MB for private
	}
	return 1024 * 1024 * 1024 // 1GB for public
}

func (s *RepoService) CreateStorageRequest(ctx context.Context, repoID, requestedByUserID string, requestedLimit int64, message string) (*models.StorageIncreaseRequest, error) {
	request := &models.StorageIncreaseRequest{
		RepoID:            repoID,
		RequestedByUserID: requestedByUserID,
		RequestedLimit:    requestedLimit,
		Message:           message,
		Status:            models.StorageRequestStatusPending,
	}
	if err := s.db.WithContext(ctx).Create(request).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Preload("Repo.Owner").
		Preload("Repo.Org").
		Preload("RequestedByUser").
		Where("id = ?", request.ID).First(request).Error; err != nil {
		return nil, err
	}
	return request, nil
}

func (s *RepoService) ListStorageRequests(ctx context.Context, status string, limit int) ([]models.StorageIncreaseRequest, error) {
	q := s.db.WithContext(ctx).
		Preload("Repo.Owner").
		Preload("Repo.Org").
		Preload("RequestedByUser").
		Preload("ReviewedByUser").
		Order("created_at DESC")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	var requests []models.StorageIncreaseRequest
	if err := q.Find(&requests).Error; err != nil {
		return nil, err
	}
	return requests, nil
}

func (s *RepoService) ReviewStorageRequest(ctx context.Context, requestID, reviewerID string, status, reviewNote string, approvedLimit int64) (*models.StorageIncreaseRequest, error) {
	var req models.StorageIncreaseRequest
	if err := s.db.WithContext(ctx).Where("id = ?", requestID).First(&req).Error; err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	updates := map[string]interface{}{
		"status":              status,
		"review_note":         reviewNote,
		"reviewed_by_user_id": reviewerID,
		"reviewed_at":         now,
	}
	if err := s.db.WithContext(ctx).Model(&req).Updates(updates).Error; err != nil {
		return nil, err
	}
	if status == models.StorageRequestStatusApproved && approvedLimit > 0 {
		if err := s.db.WithContext(ctx).
			Model(&models.Repository{}).
			Where("id = ?", req.RepoID).
			Update("size_limit_bytes", approvedLimit).Error; err != nil {
			return nil, err
		}
	}

	if err := s.db.WithContext(ctx).
		Preload("Repo.Owner").
		Preload("Repo.Org").
		Preload("RequestedByUser").
		Preload("ReviewedByUser").
		Where("id = ?", requestID).First(&req).Error; err != nil {
		return nil, err
	}
	return &req, nil
}

// CalculateRepoSize calculates the total size of a git repository in bytes
func (s *RepoService) CalculateRepoSize(repoPath string) (int64, error) {
	var totalSize int64
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})
	return totalSize, err
}

// UpdateRepoSize calculates and updates the repository size in the database
func (s *RepoService) UpdateRepoSize(ctx context.Context, repo *models.Repository, repoPath string) error {
	size, err := s.CalculateRepoSize(repoPath)
	if err != nil {
		return fmt.Errorf("failed to calculate repo size: %w", err)
	}
	return s.db.WithContext(ctx).Model(repo).Update("size", size).Error
}

// CheckSizeLimit checks if a repository has exceeded its size limit
// Returns the size status string for display and an error if limit exceeded
func (s *RepoService) CheckSizeLimit(repo *models.Repository, repoPath string) (string, error) {
	currentSize, err := s.CalculateRepoSize(repoPath)
	if err != nil {
		return "", err
	}

	limit := s.GetSizeLimit(repo)
	percent := (currentSize * 100) / limit

	// Format size for display
	var sizeStr, limitStr string
	sizeStr = formatBytes(currentSize)
	limitStr = formatBytes(limit)

	status := fmt.Sprintf("Repository size: %s / %s (%.1f%%)", sizeStr, limitStr, float64(percent))

	if currentSize > limit {
		return status, fmt.Errorf("repository size limit exceeded: %s > %s", sizeStr, limitStr)
	}

	// Show warning at 90% capacity
	if percent >= 90 {
		return status + " Ã¢Å¡Â Ã¯Â¸Â  WARNING: approaching size limit", nil
	}

	return status, nil
}

// formatBytes converts bytes to human-readable format
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
