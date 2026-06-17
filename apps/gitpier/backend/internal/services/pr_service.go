package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"gitpier/internal/models"

	"gorm.io/gorm"
)

var (
	ErrPRNotFound        = errors.New("pull request not found")
	ErrPRExists          = errors.New("pull request already exists")
	ErrInvalidBaseBranch = errors.New("base branch does not exist")
	ErrInvalidHeadBranch = errors.New("head branch does not exist")
	ErrPRNotOpen         = errors.New("pull request is not open")
	ErrPRIsDraft         = errors.New("cannot merge a draft pull request")
	ErrPRCommentNotFound = errors.New("comment not found")
	ErrPRReviewNotFound  = errors.New("review not found")
)

type PRService struct {
	db      *gorm.DB
	gitSvc  *GitService
	repoSvc *RepoService
}

func NewPRService(db *gorm.DB, gitSvc *GitService, repoSvc *RepoService) *PRService {
	return &PRService{db: db, gitSvc: gitSvc, repoSvc: repoSvc}
}

type CreatePRInput struct {
	Title       string
	Description string
	HeadRef     string
	BaseRef     string
	HeadSHA     string
	IsDraft     bool
	RepoID      string
	HeadRepoID  *string
	AuthorID    string
}

func (s *PRService) Create(ctx context.Context, input CreatePRInput) (*models.PullRequest, error) {
	repo, err := s.repoSvc.GetByID(ctx, input.RepoID)
	if err != nil {
		return nil, ErrRepoNotFound
	}

	repoPath := s.repoSvc.RepoPath(repo.Owner.Username, repo.Name)
	headRepoPath := repoPath

	baseExists, _ := s.gitSvc.BranchExists(repoPath, input.BaseRef)
	if !baseExists {
		return nil, ErrInvalidBaseBranch
	}

	if input.HeadRepoID != nil {
		headRepo, err := s.repoSvc.GetByID(ctx, *input.HeadRepoID)
		if err != nil {
			return nil, ErrRepoNotFound
		}
		headRepoPath = s.repoSvc.RepoPath(headRepo.Owner.Username, headRepo.Name)

		if headRepo.ID != repo.ID {
			related := (headRepo.ForkedFromRepoID != nil && *headRepo.ForkedFromRepoID == repo.ID) ||
				(repo.ForkedFromRepoID != nil && *repo.ForkedFromRepoID == headRepo.ID)
			if !related {
				return nil, ErrInvalidHeadBranch
			}
		}
	}

	headExists, _ := s.gitSvc.BranchExists(headRepoPath, input.HeadRef)
	if !headExists {
		return nil, ErrInvalidHeadBranch
	}

	var maxNumber uint
	s.db.WithContext(ctx).Model(&models.PullRequest{}).
		Where("repo_id = ?", input.RepoID).
		Select("COALESCE(MAX(number), 0)").
		Scan(&maxNumber)

	pr := &models.PullRequest{
		Number:      maxNumber + 1,
		Title:       input.Title,
		Description: input.Description,
		Status:      models.PRStatusOpen,
		HeadRef:     input.HeadRef,
		BaseRef:     input.BaseRef,
		HeadSHA:     input.HeadSHA,
		IsDraft:     input.IsDraft,
		RepoID:      input.RepoID,
		HeadRepoID:  input.HeadRepoID,
		AuthorID:    input.AuthorID,
	}

	if err := s.db.WithContext(ctx).Create(pr).Error; err != nil {
		log.Printf("PR create error: %v", err)
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}

	s.db.WithContext(ctx).
		Preload("Author").
		Preload("Repo.Owner").
		Where("id = ?", pr.ID).
		First(pr)
	return pr, nil
}

func (s *PRService) GetByID(ctx context.Context, id string) (*models.PullRequest, error) {
	var pr models.PullRequest
	err := s.db.WithContext(ctx).
		Preload("Author").
		Preload("Repo.Owner").
		Preload("Repo.Org").
		Preload("HeadRepo.Owner").
		Preload("HeadRepo.Org").
		Preload("MergedBy").
		Preload("Assignee").
		Preload("Labels").
		Where("id = ?", id).
		First(&pr).Error
	if err != nil {
		return nil, ErrPRNotFound
	}
	return &pr, nil
}

func (s *PRService) GetByNumber(ctx context.Context, repoID string, number uint) (*models.PullRequest, error) {
	var pr models.PullRequest
	err := s.db.WithContext(ctx).
		Preload("Author").
		Preload("Repo.Owner").
		Preload("Repo.Org").
		Preload("HeadRepo.Owner").
		Preload("HeadRepo.Org").
		Preload("MergedBy").
		Preload("Assignee").
		Preload("Labels").
		Where("repo_id = ? AND number = ?", repoID, number).
		First(&pr).Error
	if err != nil {
		return nil, ErrPRNotFound
	}
	return &pr, nil
}

func (s *PRService) GetByRepo(ctx context.Context, repoID string, status string) ([]models.PullRequest, error) {
	var prs []models.PullRequest
	q := s.db.WithContext(ctx).
		Preload("Author").
		Preload("Repo.Owner").
		Preload("Repo.Org").
		Preload("HeadRepo.Owner").
		Preload("HeadRepo.Org").
		Where("repo_id = ?", repoID)

	if status != "" {
		q = q.Where("status = ?", status)
	}

	q = q.Order("created_at DESC")
	err := q.Find(&prs).Error
	return prs, err
}

func (s *PRService) ListByAuthor(ctx context.Context, authorID string) ([]models.PullRequest, error) {
	var prs []models.PullRequest
	err := s.db.WithContext(ctx).
		Preload("Author").
		Preload("Repo.Owner").
		Preload("Repo.Org").
		Preload("HeadRepo.Owner").
		Preload("HeadRepo.Org").
		Where("author_id = ?", authorID).
		Order("created_at DESC").
		Find(&prs).Error
	return prs, err
}

func (s *PRService) Update(ctx context.Context, pr *models.PullRequest, updates map[string]interface{}) error {
	return s.db.WithContext(ctx).Model(pr).Updates(updates).Error
}

func (s *PRService) SetLabels(ctx context.Context, pr *models.PullRequest, labelIDs []string) error {
	var lbls []models.Label
	if len(labelIDs) > 0 {
		if err := s.db.WithContext(ctx).Where("id IN ? AND repo_id = ?", labelIDs, pr.RepoID).Find(&lbls).Error; err != nil {
			return err
		}
	}
	return s.db.WithContext(ctx).Model(pr).Association("Labels").Replace(lbls)
}

func (s *PRService) Close(ctx context.Context, prID string) error {
	pr, err := s.GetByID(ctx, prID)
	if err != nil {
		return err
	}
	now := time.Now()
	return s.db.WithContext(ctx).Model(pr).Updates(map[string]interface{}{
		"status":    models.PRStatusClosed,
		"closed_at": now,
	}).Error
}

func (s *PRService) Reopen(ctx context.Context, prID string) error {
	pr, err := s.GetByID(ctx, prID)
	if err != nil {
		return err
	}
	if pr.Status != models.PRStatusClosed {
		return errors.New("can only reopen closed pull requests")
	}
	return s.db.WithContext(ctx).Model(pr).Updates(map[string]interface{}{
		"status":    models.PRStatusOpen,
		"closed_at": nil,
	}).Error
}

// MergePRInput contains everything needed to perform a PR merge.
type MergePRInput struct {
	PRID        string
	Method      string // "merge" | "squash" | "rebase"
	CommitTitle string
	MergerID    string
	MergerName  string
	MergerEmail string
}

func (s *PRService) Merge(ctx context.Context, input MergePRInput) (*models.PullRequest, error) {
	pr, err := s.GetByID(ctx, input.PRID)
	if err != nil {
		return nil, err
	}
	if pr.Status != models.PRStatusOpen {
		return nil, ErrPRNotOpen
	}
	if pr.IsDraft {
		return nil, ErrPRIsDraft
	}

	repo := pr.Repo
	repoPath := s.repoSvc.RepoPath(repo.Owner.Username, repo.Name)
	headRepoPath := repoPath

	if pr.HeadRepoID != nil && *pr.HeadRepoID != pr.RepoID {
		headRepo, err := s.repoSvc.GetByID(ctx, *pr.HeadRepoID)
		if err != nil {
			return nil, ErrRepoNotFound
		}
		headRepoPath = s.repoSvc.RepoPath(headRepo.Owner.Username, headRepo.Name)
	}

	method := input.Method
	if method == "" {
		method = models.PRMergeMethodMerge
	}

	commitTitle := input.CommitTitle
	if commitTitle == "" {
		commitTitle = pr.Title
	}

	mergerName := input.MergerName
	if mergerName == "" {
		mergerName = "GitPier"
	}
	mergerEmail := input.MergerEmail
	if mergerEmail == "" {
		mergerEmail = "noreply@gitpier.com"
	}

	mergeSHA, err := s.gitSvc.MergePR(repoPath, headRepoPath, pr.BaseRef, pr.HeadRef, method, commitTitle, mergerName, mergerEmail)
	if err != nil {
		return nil, fmt.Errorf("merge failed: %w", err)
	}

	now := time.Now()
	if err := s.db.WithContext(ctx).Model(pr).Updates(map[string]interface{}{
		"status":       models.PRStatusMerged,
		"merged_at":    now,
		"merge_sha":    mergeSHA,
		"merge_method": method,
		"merged_by_id": input.MergerID,
	}).Error; err != nil {
		return nil, err
	}

	return s.GetByID(ctx, pr.ID)
}

func (s *PRService) IsMergeable(ctx context.Context, prID string) bool {
	pr, err := s.GetByID(ctx, prID)
	if err != nil || pr.Status != models.PRStatusOpen {
		return false
	}

	repo := pr.Repo
	repoPath := s.repoSvc.RepoPath(repo.Owner.Username, repo.Name)
	headRepoPath := repoPath

	if pr.HeadRepoID != nil && *pr.HeadRepoID != pr.RepoID {
		headRepo, err := s.repoSvc.GetByID(ctx, *pr.HeadRepoID)
		if err != nil {
			return false
		}
		headRepoPath = s.repoSvc.RepoPath(headRepo.Owner.Username, headRepo.Name)
	}

	return s.gitSvc.IsMergeable(repoPath, headRepoPath, pr.BaseRef, pr.HeadRef)
}

func (s *PRService) GetCommits(ctx context.Context, prID string) ([]*CommitInfo, error) {
	pr, err := s.GetByID(ctx, prID)
	if err != nil {
		return nil, err
	}
	repoPath, headRepoPath, err := s.prRepoPaths(ctx, pr)
	if err != nil {
		return nil, err
	}
	return s.gitSvc.GetPRCommitsBetweenRepos(repoPath, headRepoPath, pr.BaseRef, pr.HeadRef, pr.HeadSHA)
}

func (s *PRService) GetDiff(ctx context.Context, prID string) ([]FileDiff, error) {
	pr, err := s.GetByID(ctx, prID)
	if err != nil {
		return nil, err
	}
	repoPath, headRepoPath, err := s.prRepoPaths(ctx, pr)
	if err != nil {
		return nil, err
	}
	return s.gitSvc.GetPRDiffBetweenRepos(repoPath, headRepoPath, pr.BaseRef, pr.HeadRef, pr.HeadSHA)
}

func (s *PRService) prRepoPaths(ctx context.Context, pr *models.PullRequest) (string, string, error) {
	repo := pr.Repo
	repoPath := s.repoSvc.RepoPath(repo.Owner.Username, repo.Name)
	headRepoPath := repoPath

	if pr.HeadRepoID != nil && *pr.HeadRepoID != pr.RepoID {
		headRepo, err := s.repoSvc.GetByID(ctx, *pr.HeadRepoID)
		if err != nil {
			return "", "", ErrRepoNotFound
		}
		headRepoPath = s.repoSvc.RepoPath(headRepo.Owner.Username, headRepo.Name)
	}

	return repoPath, headRepoPath, nil
}

func (s *PRService) ListComments(ctx context.Context, prID string) ([]models.PRComment, error) {
	var comments []models.PRComment
	err := s.db.WithContext(ctx).
		Preload("Author").
		Where("pr_id = ?", prID).
		Order("created_at ASC").
		Find(&comments).Error
	return comments, err
}

func (s *PRService) AddComment(ctx context.Context, prID, authorID string, body string) (*models.PRComment, error) {
	c := &models.PRComment{
		PRID:     prID,
		AuthorID: authorID,
		Body:     body,
	}
	if err := s.db.WithContext(ctx).Create(c).Error; err != nil {
		return nil, fmt.Errorf("failed to create comment: %w", err)
	}
	s.db.WithContext(ctx).
		Preload("Author").
		Where("id = ?", c.ID).
		First(c)
	return c, nil
}

func (s *PRService) UpdateComment(ctx context.Context, commentID, authorID string, body string) (*models.PRComment, error) {
	var c models.PRComment
	if err := s.db.WithContext(ctx).Where("id = ?", commentID).First(&c).Error; err != nil {
		return nil, ErrPRCommentNotFound
	}
	if c.AuthorID != authorID {
		return nil, errors.New("not the comment author")
	}
	if err := s.db.WithContext(ctx).Model(&c).Update("body", body).Error; err != nil {
		return nil, err
	}
	s.db.WithContext(ctx).
		Preload("Author").
		Where("id = ?", c.ID).
		First(&c)
	return &c, nil
}

func (s *PRService) DeleteComment(ctx context.Context, commentID, authorID string, isRepoOwner bool) error {
	var c models.PRComment
	if err := s.db.WithContext(ctx).Where("id = ?", commentID).First(&c).Error; err != nil {
		return ErrPRCommentNotFound
	}
	if c.AuthorID != authorID && !isRepoOwner {
		return errors.New("not authorised to delete this comment")
	}
	return s.db.WithContext(ctx).Delete(&c).Error
}

type CreateReviewInput struct {
	PRID      string
	AuthorID  string
	CommitSHA string
	State     string
	Body      string
	Comments  []CreateReviewCommentInput
}

type CreateReviewCommentInput struct {
	Path      string
	Line      int
	Side      string
	Body      string
	CommitSHA string
}

func (s *PRService) CreateReview(ctx context.Context, input CreateReviewInput) (*models.PRReview, error) {
	review := &models.PRReview{
		PRID:      input.PRID,
		AuthorID:  input.AuthorID,
		CommitSHA: input.CommitSHA,
		State:     input.State,
		Body:      input.Body,
	}
	if err := s.db.WithContext(ctx).Create(review).Error; err != nil {
		return nil, fmt.Errorf("failed to create review: %w", err)
	}
	for _, rc := range input.Comments {
		side := rc.Side
		if side == "" {
			side = "RIGHT"
		}
		c := &models.PRReviewComment{
			ReviewID:  review.ID,
			PRID:      input.PRID,
			AuthorID:  input.AuthorID,
			Path:      rc.Path,
			Line:      rc.Line,
			Side:      side,
			Body:      rc.Body,
			CommitSHA: rc.CommitSHA,
		}
		s.db.WithContext(ctx).Create(c)
	}
	s.db.WithContext(ctx).
		Preload("Author").
		Preload("Comments.Author").
		Where("id = ?", review.ID).
		First(review)
	return review, nil
}

func (s *PRService) ListReviews(ctx context.Context, prID string) ([]models.PRReview, error) {
	var reviews []models.PRReview
	err := s.db.WithContext(ctx).
		Preload("Author").
		Preload("Comments.Author").
		Where("pr_id = ?", prID).
		Order("created_at ASC").
		Find(&reviews).Error
	return reviews, err
}
