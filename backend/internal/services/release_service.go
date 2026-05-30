package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitpier/internal/config"
	"gitpier/internal/models"

	"gorm.io/gorm"
)

var (
	ErrReleaseNotFound = errors.New("release not found")
	ErrAssetNotFound   = errors.New("release asset not found")
	ErrReleaseTagExists = errors.New("release tag already exists")
	ErrReleaseRepoEmpty = errors.New("repository has no commits")
	ErrReleaseBadTarget = errors.New("target commitish not found")
	ErrReleaseRepoGone  = errors.New("repository is unavailable on disk")
	ErrReleaseBadInput  = errors.New("invalid release input")
)

// ReleaseService manages releases and their binary assets.
type ReleaseService struct {
	db         *gorm.DB
	gitSvc     *GitService
	repoSvc    *RepoService
	assetsPath string // base dir for stored asset files
}

func NewReleaseService(db *gorm.DB, gitSvc *GitService, repoSvc *RepoService, cfg *config.Config) *ReleaseService {
	assetsPath := filepath.Join(cfg.WorkflowWorkspacePath, "release-assets")
	return &ReleaseService{
		db:         db,
		gitSvc:     gitSvc,
		repoSvc:    repoSvc,
		assetsPath: assetsPath,
	}
}

func (s *ReleaseService) baseQuery(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).
		Preload("CreatedBy").
		Preload("Assets")
}

// List returns all non-draft releases for a repository (drafts visible only to admins â€” caller filters).
func (s *ReleaseService) List(ctx context.Context, repoID string, includeDrafts bool) ([]models.Release, error) {
	var releases []models.Release
	q := s.baseQuery(ctx).Where("repo_id = ?", repoID)
	if !includeDrafts {
		q = q.Where("is_draft = false")
	}
	if err := q.Order("created_at DESC").Find(&releases).Error; err != nil {
		return nil, err
	}
	return releases, nil
}

// Get returns a single release by ID, checking it belongs to the given repo.
func (s *ReleaseService) Get(ctx context.Context, repoID, releaseID string) (*models.Release, error) {
	var r models.Release
	err := s.baseQuery(ctx).
		Where("id = ? AND repo_id = ?", releaseID, repoID).
		First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrReleaseNotFound
	}
	return &r, err
}

// GetByTag returns the release for a specific tag.
func (s *ReleaseService) GetByTag(ctx context.Context, repoID string, tagName string) (*models.Release, error) {
	var r models.Release
	err := s.baseQuery(ctx).
		Where("repo_id = ? AND tag_name = ?", repoID, tagName).
		First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrReleaseNotFound
	}
	return &r, err
}

// GetLatest returns the most recently published non-draft, non-prerelease release.
func (s *ReleaseService) GetLatest(ctx context.Context, repoID string) (*models.Release, error) {
	var r models.Release
	err := s.baseQuery(ctx).
		Where("repo_id = ? AND is_draft = false AND is_prerelease = false", repoID).
		Order("published_at DESC").
		First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrReleaseNotFound
	}
	return &r, err
}

type CreateReleaseInput struct {
	TagName      string
	TargetCommit string // branch or commit SHA; empty = default branch HEAD
	Name         string
	Body         string
	IsDraft      bool
	IsPrerelease bool
}

// Create creates a new release. If the tag doesn't exist in git yet, it is created.
func (s *ReleaseService) Create(ctx context.Context, repoID, userID string, repoPath string, input CreateReleaseInput) (*models.Release, error) {
	// Ensure tag is valid
	if strings.TrimSpace(input.TagName) == "" {
		return nil, fmt.Errorf("%w: tag_name is required", ErrReleaseBadInput)
	}

	// Create git tag if it doesn't already exist
	tagExists, err := s.gitSvc.TagExists(repoPath, input.TagName)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidGitParam):
			return nil, fmt.Errorf("%w: invalid tag name", ErrReleaseBadInput)
		case errors.Is(err, ErrGitRepositoryNotFound):
			return nil, ErrReleaseRepoGone
		default:
			return nil, fmt.Errorf("failed to inspect tag: %w", err)
		}
	}
	if !tagExists {
		msg := input.Name
		if msg == "" {
			msg = input.TagName
		}
		if err := s.gitSvc.CreateTag(repoPath, input.TagName, input.TargetCommit, msg); err != nil {
			switch {
			case errors.Is(err, ErrTagAlreadyExists):
				return nil, ErrReleaseTagExists
			case errors.Is(err, ErrEmptyRepository):
				return nil, ErrReleaseRepoEmpty
			case errors.Is(err, ErrGitReferenceNotFound):
				return nil, ErrReleaseBadTarget
			case errors.Is(err, ErrGitRepositoryNotFound):
				return nil, ErrReleaseRepoGone
			case errors.Is(err, ErrInvalidGitParam):
				return nil, fmt.Errorf("%w: invalid tag or target ref", ErrReleaseBadInput)
			default:
				return nil, fmt.Errorf("failed to create tag: %w", err)
			}
		}
	}

	now := time.Now()
	var publishedAt *time.Time
	if !input.IsDraft {
		publishedAt = &now
	}

	r := &models.Release{
		RepoID:       repoID,
		TagName:      input.TagName,
		TargetCommit: input.TargetCommit,
		Name:         input.Name,
		Body:         input.Body,
		IsDraft:      input.IsDraft,
		IsPrerelease: input.IsPrerelease,
		PublishedAt:  publishedAt,
		CreatedByID:  userID,
	}

	if err := s.db.WithContext(ctx).Create(r).Error; err != nil {
		return nil, err
	}

	return s.Get(ctx, repoID, r.ID)
}

type UpdateReleaseInput struct {
	Name         *string
	Body         *string
	IsDraft      *bool
	IsPrerelease *bool
}

// Update updates editable fields on a release.
func (s *ReleaseService) Update(ctx context.Context, repoID, releaseID string, input UpdateReleaseInput) (*models.Release, error) {
	r, err := s.Get(ctx, repoID, releaseID)
	if err != nil {
		return nil, err
	}

	wasDraft := r.IsDraft

	if input.Name != nil {
		r.Name = *input.Name
	}
	if input.Body != nil {
		r.Body = *input.Body
	}
	if input.IsDraft != nil {
		r.IsDraft = *input.IsDraft
	}
	if input.IsPrerelease != nil {
		r.IsPrerelease = *input.IsPrerelease
	}

	// Set PublishedAt when transitioning from draft to published
	if wasDraft && !r.IsDraft && r.PublishedAt == nil {
		now := time.Now()
		r.PublishedAt = &now
	}

	if err := s.db.WithContext(ctx).Save(r).Error; err != nil {
		return nil, err
	}

	return s.Get(ctx, repoID, releaseID)
}

// Delete deletes a release and all its stored asset files.
func (s *ReleaseService) Delete(ctx context.Context, repoID, releaseID string) error {
	r, err := s.Get(ctx, repoID, releaseID)
	if err != nil {
		return err
	}

	// Remove asset files from disk
	for _, asset := range r.Assets {
		_ = os.Remove(asset.StoragePath)
	}
	// Remove release asset directory
	_ = os.RemoveAll(filepath.Join(s.assetsPath, fmt.Sprintf("%d", releaseID)))

	// Delete DB records (assets cascade via FK)
	if err := s.db.WithContext(ctx).Where("release_id = ?", releaseID).Delete(&models.ReleaseAsset{}).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Delete(r).Error
}

// UploadAsset saves a binary asset to disk and records it in the database.
func (s *ReleaseService) UploadAsset(ctx context.Context, releaseID string, name, contentType string, reader io.Reader) (*models.ReleaseAsset, error) {
	// Sanitize filename
	name = filepath.Base(name)
	if name == "" || name == "." {
		return nil, fmt.Errorf("invalid asset name")
	}

	dir := filepath.Join(s.assetsPath, fmt.Sprintf("%d", releaseID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create asset dir: %w", err)
	}

	// Write to a temp file first, then rename for atomicity
	tmpFile, err := os.CreateTemp(dir, "upload-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	written, err := io.Copy(tmpFile, reader)
	tmpFile.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to write asset: %w", err)
	}

	finalPath := filepath.Join(dir, name)
	// If a file with this name already exists, remove it
	_ = os.Remove(finalPath)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to store asset: %w", err)
	}

	if contentType == "" {
		contentType = "application/octet-stream"
	}

	asset := &models.ReleaseAsset{
		ReleaseID:   releaseID,
		Name:        name,
		Size:        written,
		ContentType: contentType,
		StoragePath: finalPath,
	}
	if err := s.db.WithContext(ctx).Create(asset).Error; err != nil {
		_ = os.Remove(finalPath)
		return nil, err
	}
	return asset, nil
}

// DeleteAsset removes an asset file and its DB record.
func (s *ReleaseService) DeleteAsset(ctx context.Context, releaseID, assetID string) error {
	var asset models.ReleaseAsset
	err := s.db.WithContext(ctx).
		Where("id = ? AND release_id = ?", assetID, releaseID).
		First(&asset).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrAssetNotFound
	}
	if err != nil {
		return err
	}
	_ = os.Remove(asset.StoragePath)
	return s.db.WithContext(ctx).Delete(&asset).Error
}

// GetAsset returns a single asset record.
func (s *ReleaseService) GetAsset(ctx context.Context, assetID string) (*models.ReleaseAsset, error) {
	var asset models.ReleaseAsset
	if err := s.db.WithContext(ctx).First(&asset, assetID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAssetNotFound
		}
		return nil, err
	}
	return &asset, nil
}

// IncrementDownloadCount atomically increments the download counter for an asset.
func (s *ReleaseService) IncrementDownloadCount(ctx context.Context, assetID string) {
	s.db.WithContext(ctx).Model(&models.ReleaseAsset{}).
		Where("id = ?", assetID).
		UpdateColumn("download_count", gorm.Expr("download_count + 1"))
}

// UploadAssetFromWorkspace is called by the workflow runner to attach a file from
// the job workspace to the release identified by ownerUsername/repoName/tagName.
// globPattern is matched against files in workspaceDir; the first match is used.
func (s *ReleaseService) UploadAssetFromWorkspace(
	ctx context.Context,
	ownerUsername, repoName, tagName, globPattern, assetName, workspaceDir string,
) error {
	// Resolve glob relative to workspace
	pattern := globPattern
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(workspaceDir, pattern)
	}
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return fmt.Errorf("no files matched pattern %q", globPattern)
	}

	// Look up the release by owner+repo+tag
	repo, err := s.repoSvc.GetByOwnerAndName(ctx, ownerUsername, repoName)
	if err != nil {
		return fmt.Errorf("repository not found: %w", err)
	}
	release, err := s.GetByTag(ctx, repo.ID, tagName)
	if err != nil {
		return fmt.Errorf("release for tag %q not found: %w", tagName, err)
	}

	// Upload each matched file
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil || info.IsDir() {
			continue
		}

		name := assetName
		if name == "" || len(matches) > 1 {
			name = filepath.Base(match)
		}

		f, err := os.Open(match)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", match, err)
		}
		_, uploadErr := s.UploadAsset(ctx, release.ID, name, "application/octet-stream", f)
		f.Close()
		if uploadErr != nil {
			return fmt.Errorf("failed to upload %s: %w", name, uploadErr)
		}
	}

	return nil
}
