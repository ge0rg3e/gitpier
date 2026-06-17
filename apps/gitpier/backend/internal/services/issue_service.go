package services

import (
	"context"
	"errors"
	"fmt"

	"gitpier/internal/models"

	"gorm.io/gorm"
)

var (
	ErrIssueNotFound   = errors.New("issue not found")
	ErrLabelNotFound   = errors.New("label not found")
	ErrCommentNotFound = errors.New("comment not found")
)

type IssueService struct {
	db      *gorm.DB
	repoSvc *RepoService
}

func NewIssueService(db *gorm.DB, repoSvc *RepoService) *IssueService {
	return &IssueService{db: db, repoSvc: repoSvc}
}

type CreateIssueInput struct {
	Title      string
	Body       string
	IssueType  string
	RepoID     string
	AuthorID   string
	AssigneeID *string
	LabelIDs   []string
}

func (s *IssueService) Create(ctx context.Context, input CreateIssueInput) (*models.Issue, error) {
	// Determine the next issue number for this repo
	var maxNumber uint
	s.db.WithContext(ctx).Model(&models.Issue{}).
		Where("repo_id = ?", input.RepoID).
		Select("COALESCE(MAX(number), 0)").
		Scan(&maxNumber)

	issue := &models.Issue{
		Number:     maxNumber + 1,
		Title:      input.Title,
		Body:       input.Body,
		Status:     models.IssueStatusOpen,
		IssueType:  input.IssueType,
		RepoID:     input.RepoID,
		AuthorID:   input.AuthorID,
		AssigneeID: input.AssigneeID,
	}

	if err := s.db.WithContext(ctx).Create(issue).Error; err != nil {
		return nil, fmt.Errorf("failed to create issue: %w", err)
	}

	if len(input.LabelIDs) > 0 {
		var labels []models.Label
		s.db.WithContext(ctx).Where("id IN ? AND repo_id = ?", input.LabelIDs, input.RepoID).Find(&labels)
		s.db.WithContext(ctx).Model(issue).Association("Labels").Replace(labels)
	}

	return s.loadIssue(ctx, issue.ID)
}

func (s *IssueService) GetByRepoAndNumber(ctx context.Context, repoID string, number uint) (*models.Issue, error) {
	var issue models.Issue
	err := s.db.WithContext(ctx).
		Where("repo_id = ? AND number = ?", repoID, number).
		First(&issue).Error
	if err != nil {
		return nil, ErrIssueNotFound
	}
	return s.loadIssue(ctx, issue.ID)
}

func (s *IssueService) GetByRepo(ctx context.Context, repoID string, status string, labelIDs []string) ([]models.Issue, error) {
	q := s.db.WithContext(ctx).
		Preload("Author").
		Preload("Assignee").
		Preload("Labels").
		Preload("Milestone").
		Where("issues.repo_id = ?", repoID)

	if status != "" {
		q = q.Where("issues.status = ?", status)
	}

	if len(labelIDs) > 0 {
		q = q.Joins("JOIN issue_labels il ON il.issue_id = issues.id").
			Where("il.label_id IN ?", labelIDs)
	}

	var issues []models.Issue
	err := q.Order("issues.created_at DESC").Find(&issues).Error
	return issues, err
}

func (s *IssueService) Update(ctx context.Context, repoID string, number uint, updates map[string]interface{}) (*models.Issue, error) {
	issue, err := s.GetByRepoAndNumber(ctx, repoID, number)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(issue).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.loadIssue(ctx, issue.ID)
}

func (s *IssueService) Close(ctx context.Context, repoID string, number uint) (*models.Issue, error) {
	issue, err := s.GetByRepoAndNumber(ctx, repoID, number)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(issue).Update("status", models.IssueStatusClosed).Error; err != nil {
		return nil, err
	}
	return s.loadIssue(ctx, issue.ID)
}

func (s *IssueService) Reopen(ctx context.Context, repoID string, number uint) (*models.Issue, error) {
	issue, err := s.GetByRepoAndNumber(ctx, repoID, number)
	if err != nil {
		return nil, err
	}
	if issue.Status != models.IssueStatusClosed {
		return nil, errors.New("can only reopen closed issues")
	}
	if err := s.db.WithContext(ctx).Model(issue).Update("status", models.IssueStatusOpen).Error; err != nil {
		return nil, err
	}
	return s.loadIssue(ctx, issue.ID)
}

func (s *IssueService) Delete(ctx context.Context, repoID string, number uint) error {
	issue, err := s.GetByRepoAndNumber(ctx, repoID, number)
	if err != nil {
		return err
	}
	// Remove label associations first
	s.db.WithContext(ctx).Model(issue).Association("Labels").Clear()
	// Delete comments
	s.db.WithContext(ctx).Where("issue_id = ?", issue.ID).Delete(&models.IssueComment{})
	return s.db.WithContext(ctx).Delete(issue).Error
}

func (s *IssueService) ListLabels(ctx context.Context, repoID string) ([]models.Label, error) {
	var labels []models.Label
	err := s.db.WithContext(ctx).Where("repo_id = ?", repoID).Order("name").Find(&labels).Error
	return labels, err
}

func (s *IssueService) CreateLabel(ctx context.Context, repoID string, name, color, description string) (*models.Label, error) {
	label := &models.Label{
		Name:        name,
		Color:       color,
		Description: description,
		RepoID:      repoID,
	}
	if err := s.db.WithContext(ctx).Create(label).Error; err != nil {
		return nil, fmt.Errorf("failed to create label: %w", err)
	}
	return label, nil
}

func (s *IssueService) UpdateLabel(ctx context.Context, labelID, repoID string, updates map[string]interface{}) (*models.Label, error) {
	var label models.Label
	if err := s.db.WithContext(ctx).Where("id = ? AND repo_id = ?", labelID, repoID).First(&label).Error; err != nil {
		return nil, ErrLabelNotFound
	}
	if err := s.db.WithContext(ctx).Model(&label).Updates(updates).Error; err != nil {
		return nil, err
	}
	return &label, nil
}

func (s *IssueService) DeleteLabel(ctx context.Context, labelID, repoID string) error {
	res := s.db.WithContext(ctx).Where("id = ? AND repo_id = ?", labelID, repoID).Delete(&models.Label{})
	if res.RowsAffected == 0 {
		return ErrLabelNotFound
	}
	return res.Error
}

func (s *IssueService) SetLabels(ctx context.Context, repoID string, number uint, labelIDs []string) (*models.Issue, error) {
	issue, err := s.GetByRepoAndNumber(ctx, repoID, number)
	if err != nil {
		return nil, err
	}
	var labels []models.Label
	if len(labelIDs) > 0 {
		s.db.WithContext(ctx).Where("id IN ? AND repo_id = ?", labelIDs, repoID).Find(&labels)
	}
	if err := s.db.WithContext(ctx).Model(issue).Association("Labels").Replace(labels); err != nil {
		return nil, err
	}
	return s.loadIssue(ctx, issue.ID)
}

func (s *IssueService) ListComments(ctx context.Context, issueID string) ([]models.IssueComment, error) {
	var comments []models.IssueComment
	err := s.db.WithContext(ctx).
		Preload("Author").
		Where("issue_id = ?", issueID).
		Order("created_at ASC").
		Find(&comments).Error
	return comments, err
}

func (s *IssueService) CreateComment(ctx context.Context, issueID, authorID string, body string) (*models.IssueComment, error) {
	comment := &models.IssueComment{
		IssueID:  issueID,
		AuthorID: authorID,
		Body:     body,
	}
	if err := s.db.WithContext(ctx).Create(comment).Error; err != nil {
		return nil, fmt.Errorf("failed to create comment: %w", err)
	}
	s.db.WithContext(ctx).Preload("Author").Where("id = ?", comment.ID).First(comment)
	return comment, nil
}

func (s *IssueService) UpdateComment(ctx context.Context, commentID, authorID string, body string) (*models.IssueComment, error) {
	var comment models.IssueComment
	if err := s.db.WithContext(ctx).Where("id = ? AND author_id = ?", commentID, authorID).First(&comment).Error; err != nil {
		return nil, ErrCommentNotFound
	}
	if err := s.db.WithContext(ctx).Model(&comment).Update("body", body).Error; err != nil {
		return nil, err
	}
	s.db.WithContext(ctx).Preload("Author").Where("id = ?", comment.ID).First(&comment)
	return &comment, nil
}

func (s *IssueService) DeleteComment(ctx context.Context, commentID, requesterID string, isRepoOwner bool) error {
	var comment models.IssueComment
	if err := s.db.WithContext(ctx).Where("id = ?", commentID).First(&comment).Error; err != nil {
		return ErrCommentNotFound
	}
	if comment.AuthorID != requesterID && !isRepoOwner {
		return errors.New("forbidden")
	}
	return s.db.WithContext(ctx).Delete(&comment).Error
}

func (s *IssueService) loadIssue(ctx context.Context, id string) (*models.Issue, error) {
	var issue models.Issue
	err := s.db.WithContext(ctx).
		Preload("Author").
		Preload("Assignee").
		Preload("Labels").
		Preload("Comments.Author").
		Preload("Milestone").
		Where("id = ?", id).
		First(&issue).Error
	if err != nil {
		return nil, ErrIssueNotFound
	}
	return &issue, nil
}

var ErrMilestoneNotFound = errors.New("milestone not found")

func (s *IssueService) ListMilestones(ctx context.Context, repoID string, status string) ([]models.Milestone, error) {
	q := s.db.WithContext(ctx).Where("repo_id = ?", repoID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var milestones []models.Milestone
	err := q.Order("created_at DESC").Find(&milestones).Error
	return milestones, err
}

func (s *IssueService) CreateMilestone(ctx context.Context, repoID string, title, description string) (*models.Milestone, error) {
	m := &models.Milestone{
		Title:       title,
		Description: description,
		Status:      "open",
		RepoID:      repoID,
	}
	if err := s.db.WithContext(ctx).Create(m).Error; err != nil {
		return nil, fmt.Errorf("failed to create milestone: %w", err)
	}
	return m, nil
}

func (s *IssueService) GetMilestone(ctx context.Context, repoID, milestoneID string) (*models.Milestone, error) {
	var m models.Milestone
	if err := s.db.WithContext(ctx).Where("id = ? AND repo_id = ?", milestoneID, repoID).First(&m).Error; err != nil {
		return nil, ErrMilestoneNotFound
	}
	return &m, nil
}

func (s *IssueService) UpdateMilestone(ctx context.Context, repoID, milestoneID string, updates map[string]interface{}) (*models.Milestone, error) {
	m, err := s.GetMilestone(ctx, repoID, milestoneID)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(m).Updates(updates).Error; err != nil {
		return nil, err
	}
	return m, nil
}

func (s *IssueService) DeleteMilestone(ctx context.Context, repoID, milestoneID string) error {
	res := s.db.WithContext(ctx).Where("id = ? AND repo_id = ?", milestoneID, repoID).Delete(&models.Milestone{})
	if res.RowsAffected == 0 {
		return ErrMilestoneNotFound
	}
	return res.Error
}
