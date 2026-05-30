package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gitpier/internal/models"

	"gorm.io/gorm"
)

var (
	ErrProjectNotFound = errors.New("project not found")
	ErrColumnNotFound  = errors.New("project column not found")
	ErrItemNotFound    = errors.New("project item not found")
)

type ProjectService struct {
	db *gorm.DB
}

func NewProjectService(db *gorm.DB) *ProjectService {
	return &ProjectService{db: db}
}

func (s *ProjectService) CreateForUser(ctx context.Context, ownerUserID, creatorID, title, description string, isPublic bool) (*models.Project, error) {
	project := &models.Project{
		Title:       strings.TrimSpace(title),
		Description: strings.TrimSpace(description),
		OwnerUserID: &ownerUserID,
		CreatedByID: creatorID,
		IsPublic:    isPublic,
	}
	if project.Title == "" {
		return nil, errors.New("title is required")
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(project).Error; err != nil {
			return err
		}
		return s.createDefaultColumns(ctx, tx, project.ID)
	}); err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	return s.GetByID(ctx, project.ID, true)
}

func (s *ProjectService) CreateForOrg(ctx context.Context, ownerOrgID, creatorID, title, description string, isPublic bool) (*models.Project, error) {
	project := &models.Project{
		Title:       strings.TrimSpace(title),
		Description: strings.TrimSpace(description),
		OwnerOrgID:  &ownerOrgID,
		CreatedByID: creatorID,
		IsPublic:    isPublic,
	}
	if project.Title == "" {
		return nil, errors.New("title is required")
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(project).Error; err != nil {
			return err
		}
		return s.createDefaultColumns(ctx, tx, project.ID)
	}); err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	return s.GetByID(ctx, project.ID, true)
}

func (s *ProjectService) createDefaultColumns(ctx context.Context, tx *gorm.DB, projectID string) error {
	defaults := []models.ProjectColumn{
		{ProjectID: projectID, Name: "To Do", Color: "#22c55e", Position: 0},
		{ProjectID: projectID, Name: "In Progress", Color: "#f59e0b", Position: 1},
		{ProjectID: projectID, Name: "In Review", Color: "#8b5cf6", Position: 2},
		{ProjectID: projectID, Name: "Done", Color: "#06b6d4", Position: 3},
	}
	return tx.WithContext(ctx).Create(&defaults).Error
}

func (s *ProjectService) ListByOwnerUser(ctx context.Context, ownerUserID string, includePrivate bool) ([]models.Project, error) {
	q := s.db.WithContext(ctx).
		Preload("OwnerUser").
		Preload("OwnerOrg").
		Preload("CreatedBy").
		Preload("Columns", func(db *gorm.DB) *gorm.DB { return db.Order("position ASC") }).
		Where("owner_user_id = ?", ownerUserID)
	if !includePrivate {
		q = q.Where("is_public = true")
	}

	var projects []models.Project
	if err := q.Order("updated_at DESC").Find(&projects).Error; err != nil {
		return nil, err
	}
	return projects, nil
}

func (s *ProjectService) ListByOwnerOrg(ctx context.Context, ownerOrgID string, includePrivate bool) ([]models.Project, error) {
	q := s.db.WithContext(ctx).
		Preload("OwnerUser").
		Preload("OwnerOrg").
		Preload("CreatedBy").
		Preload("Columns", func(db *gorm.DB) *gorm.DB { return db.Order("position ASC") }).
		Where("owner_org_id = ?", ownerOrgID)
	if !includePrivate {
		q = q.Where("is_public = true")
	}

	var projects []models.Project
	if err := q.Order("updated_at DESC").Find(&projects).Error; err != nil {
		return nil, err
	}
	return projects, nil
}

func (s *ProjectService) GetByID(ctx context.Context, id string, includeItems bool) (*models.Project, error) {
	q := s.db.WithContext(ctx).
		Preload("OwnerUser").
		Preload("OwnerOrg").
		Preload("CreatedBy").
		Preload("Columns", func(db *gorm.DB) *gorm.DB { return db.Order("position ASC") })

	if includeItems {
		q = q.Preload("Columns.Items", func(db *gorm.DB) *gorm.DB { return db.Order("position ASC") }).
			Preload("Columns.Items.AssigneeUser")
	}

	var project models.Project
	if err := q.Where("id = ?", id).First(&project).Error; err != nil {
		return nil, ErrProjectNotFound
	}
	return &project, nil
}

func (s *ProjectService) UpdateProject(ctx context.Context, project *models.Project, updates map[string]interface{}) error {
	if titleRaw, ok := updates["title"]; ok {
		title, _ := titleRaw.(string)
		if strings.TrimSpace(title) == "" {
			return errors.New("title is required")
		}
		updates["title"] = strings.TrimSpace(title)
	}
	if descRaw, ok := updates["description"]; ok {
		desc, _ := descRaw.(string)
		updates["description"] = strings.TrimSpace(desc)
	}
	if err := s.db.WithContext(ctx).Model(project).Updates(updates).Error; err != nil {
		return err
	}
	return nil
}

func (s *ProjectService) DeleteProject(ctx context.Context, project *models.Project) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ?", project.ID).Delete(&models.ProjectItem{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", project.ID).Delete(&models.ProjectColumn{}).Error; err != nil {
			return err
		}
		return tx.Delete(project).Error
	})
}

func (s *ProjectService) CreateColumn(ctx context.Context, project *models.Project, name, description, color string, position *int) (*models.ProjectColumn, error) {
	column := &models.ProjectColumn{
		ProjectID:   project.ID,
		Name:        strings.TrimSpace(name),
		Description: strings.TrimSpace(description),
		Color:       strings.TrimSpace(color),
	}
	if column.Name == "" {
		return nil, errors.New("column name is required")
	}
	if column.Color == "" {
		column.Color = "#0ea5e9"
	}

	if position != nil {
		column.Position = *position
	} else {
		maxPos, err := s.maxColumnPosition(ctx, project.ID)
		if err != nil {
			return nil, err
		}
		column.Position = maxPos + 1
	}

	if err := s.db.WithContext(ctx).Create(column).Error; err != nil {
		return nil, err
	}

	_ = s.normalizeColumnPositions(ctx, project.ID)
	if err := s.db.WithContext(ctx).Where("id = ?", column.ID).First(column).Error; err != nil {
		return nil, err
	}
	return column, nil
}

func (s *ProjectService) UpdateColumn(ctx context.Context, project *models.Project, columnID string, updates map[string]interface{}) (*models.ProjectColumn, error) {
	column, err := s.GetColumn(ctx, project.ID, columnID)
	if err != nil {
		return nil, err
	}
	if nameRaw, ok := updates["name"]; ok {
		name, _ := nameRaw.(string)
		if strings.TrimSpace(name) == "" {
			return nil, errors.New("column name is required")
		}
		updates["name"] = strings.TrimSpace(name)
	}
	if colorRaw, ok := updates["color"]; ok {
		color, _ := colorRaw.(string)
		if strings.TrimSpace(color) == "" {
			updates["color"] = "#0ea5e9"
		} else {
			updates["color"] = strings.TrimSpace(color)
		}
	}
	if descRaw, ok := updates["description"]; ok {
		desc, _ := descRaw.(string)
		updates["description"] = strings.TrimSpace(desc)
	}
	if err := s.db.WithContext(ctx).Model(column).Updates(updates).Error; err != nil {
		return nil, err
	}
	_ = s.normalizeColumnPositions(ctx, project.ID)
	if err := s.db.WithContext(ctx).Where("id = ?", column.ID).First(column).Error; err != nil {
		return nil, err
	}
	return column, nil
}

func (s *ProjectService) DeleteColumn(ctx context.Context, project *models.Project, columnID string) error {
	column, err := s.GetColumn(ctx, project.ID, columnID)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ? AND column_id = ?", project.ID, column.ID).Delete(&models.ProjectItem{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(column).Error; err != nil {
			return err
		}
		return s.normalizeColumnPositionsTx(ctx, tx, project.ID)
	})
}

func (s *ProjectService) GetColumn(ctx context.Context, projectID, columnID string) (*models.ProjectColumn, error) {
	var column models.ProjectColumn
	if err := s.db.WithContext(ctx).Where("id = ? AND project_id = ?", columnID, projectID).First(&column).Error; err != nil {
		return nil, ErrColumnNotFound
	}
	return &column, nil
}

func (s *ProjectService) CreateItem(ctx context.Context, project *models.Project, columnID, title, body string, position *int, assigneeUserID *string) (*models.ProjectItem, error) {
	if _, err := s.GetColumn(ctx, project.ID, columnID); err != nil {
		return nil, err
	}

	item := &models.ProjectItem{
		ProjectID:      project.ID,
		ColumnID:       columnID,
		Title:          strings.TrimSpace(title),
		Body:           strings.TrimSpace(body),
		AssigneeUserID: assigneeUserID,
	}
	if item.Title == "" {
		return nil, errors.New("item title is required")
	}

	if position != nil {
		item.Position = *position
	} else {
		maxPos, err := s.maxItemPosition(ctx, project.ID, columnID)
		if err != nil {
			return nil, err
		}
		item.Position = maxPos + 1
	}

	if err := s.db.WithContext(ctx).Create(item).Error; err != nil {
		return nil, err
	}

	_ = s.normalizeItemPositions(ctx, project.ID, columnID)
	if err := s.db.WithContext(ctx).Preload("AssigneeUser").Where("id = ?", item.ID).First(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (s *ProjectService) UpdateItem(ctx context.Context, project *models.Project, itemID string, updates map[string]interface{}) (*models.ProjectItem, error) {
	item, err := s.GetItem(ctx, project.ID, itemID)
	if err != nil {
		return nil, err
	}
	if titleRaw, ok := updates["title"]; ok {
		title, _ := titleRaw.(string)
		if strings.TrimSpace(title) == "" {
			return nil, errors.New("item title is required")
		}
		updates["title"] = strings.TrimSpace(title)
	}
	if bodyRaw, ok := updates["body"]; ok {
		body, _ := bodyRaw.(string)
		updates["body"] = strings.TrimSpace(body)
	}
	if assignee, hasAssignee := updates["assignee_user_id"]; hasAssignee {
		if assignee == "" {
			updates["assignee_user_id"] = nil
		}
	}
	if err := s.db.WithContext(ctx).Model(item).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Preload("AssigneeUser").Where("id = ?", item.ID).First(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (s *ProjectService) MoveItem(ctx context.Context, project *models.Project, itemID string, toColumnID *string, toPosition *int) (*models.ProjectItem, error) {
	item, err := s.GetItem(ctx, project.ID, itemID)
	if err != nil {
		return nil, err
	}

	targetColumnID := item.ColumnID
	if toColumnID != nil && *toColumnID != "" {
		targetColumnID = *toColumnID
		if _, err := s.GetColumn(ctx, project.ID, targetColumnID); err != nil {
			return nil, err
		}
	}

	targetPos := item.Position
	if toPosition != nil {
		targetPos = *toPosition
	} else if targetColumnID != item.ColumnID {
		maxPos, err := s.maxItemPosition(ctx, project.ID, targetColumnID)
		if err != nil {
			return nil, err
		}
		targetPos = maxPos + 1
	}

	sourceColumnID := item.ColumnID
	if err := s.db.WithContext(ctx).Model(item).Updates(map[string]interface{}{
		"column_id": targetColumnID,
		"position":  targetPos,
	}).Error; err != nil {
		return nil, err
	}

	_ = s.normalizeItemPositions(ctx, project.ID, sourceColumnID)
	_ = s.normalizeItemPositions(ctx, project.ID, targetColumnID)

	if err := s.db.WithContext(ctx).Preload("AssigneeUser").Where("id = ?", item.ID).First(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (s *ProjectService) DeleteItem(ctx context.Context, project *models.Project, itemID string) error {
	item, err := s.GetItem(ctx, project.ID, itemID)
	if err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Delete(item).Error; err != nil {
		return err
	}
	return s.normalizeItemPositions(ctx, project.ID, item.ColumnID)
}

func (s *ProjectService) GetItem(ctx context.Context, projectID, itemID string) (*models.ProjectItem, error) {
	var item models.ProjectItem
	if err := s.db.WithContext(ctx).Where("id = ? AND project_id = ?", itemID, projectID).First(&item).Error; err != nil {
		return nil, ErrItemNotFound
	}
	return &item, nil
}

func (s *ProjectService) maxColumnPosition(ctx context.Context, projectID string) (int, error) {
	var result struct{ Max int }
	if err := s.db.WithContext(ctx).Model(&models.ProjectColumn{}).Where("project_id = ?", projectID).Select("COALESCE(MAX(position), -1) AS max").Scan(&result).Error; err != nil {
		return 0, err
	}
	return result.Max, nil
}

func (s *ProjectService) maxItemPosition(ctx context.Context, projectID, columnID string) (int, error) {
	var result struct{ Max int }
	if err := s.db.WithContext(ctx).Model(&models.ProjectItem{}).Where("project_id = ? AND column_id = ?", projectID, columnID).Select("COALESCE(MAX(position), -1) AS max").Scan(&result).Error; err != nil {
		return 0, err
	}
	return result.Max, nil
}

func (s *ProjectService) normalizeColumnPositions(ctx context.Context, projectID string) error {
	return s.normalizeColumnPositionsTx(ctx, s.db, projectID)
}

func (s *ProjectService) normalizeColumnPositionsTx(ctx context.Context, tx *gorm.DB, projectID string) error {
	var columns []models.ProjectColumn
	if err := tx.WithContext(ctx).Where("project_id = ?", projectID).Order("position ASC, created_at ASC").Find(&columns).Error; err != nil {
		return err
	}
	for i := range columns {
		if columns[i].Position == i {
			continue
		}
		if err := tx.WithContext(ctx).Model(&models.ProjectColumn{}).Where("id = ?", columns[i].ID).Update("position", i).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *ProjectService) normalizeItemPositions(ctx context.Context, projectID, columnID string) error {
	var items []models.ProjectItem
	if err := s.db.WithContext(ctx).Where("project_id = ? AND column_id = ?", projectID, columnID).Order("position ASC, created_at ASC").Find(&items).Error; err != nil {
		return err
	}
	for i := range items {
		if items[i].Position == i {
			continue
		}
		if err := s.db.WithContext(ctx).Model(&models.ProjectItem{}).Where("id = ?", items[i].ID).Update("position", i).Error; err != nil {
			return err
		}
	}
	return nil
}
