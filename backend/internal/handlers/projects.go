package handlers

import (
	"errors"
	"net/http"
	"strings"

	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type ProjectHandler struct {
	projectSvc *services.ProjectService
	orgSvc     *services.OrgService
	db         *gorm.DB
}

func NewProjectHandler(projectSvc *services.ProjectService, orgSvc *services.OrgService, db *gorm.DB) *ProjectHandler {
	return &ProjectHandler{projectSvc: projectSvc, orgSvc: orgSvc, db: db}
}

func (h *ProjectHandler) resolveUserByUsername(c echo.Context, username string) (*models.User, error) {
	var user models.User
	if err := h.db.WithContext(c.Request().Context()).Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (h *ProjectHandler) getCurrentUser(c echo.Context) *models.User {
	if u, ok := c.Get("user").(*models.User); ok {
		return u
	}
	return nil
}

func (h *ProjectHandler) canViewProject(c echo.Context, project *models.Project) bool {
	viewer := h.getCurrentUser(c)

	if project.OwnerUserID != nil {
		if project.IsPublic {
			return true
		}
		if viewer == nil {
			return false
		}
		return viewer.ID == *project.OwnerUserID
	}

	if project.OwnerOrgID != nil {
		if viewer != nil && h.orgSvc.IsMember(c.Request().Context(), *project.OwnerOrgID, viewer.ID) {
			return true
		}
		return project.IsPublic
	}

	return false
}

func (h *ProjectHandler) canManageProject(c echo.Context, project *models.Project) bool {
	viewer := h.getCurrentUser(c)
	if viewer == nil {
		return false
	}

	if project.OwnerUserID != nil {
		return viewer.ID == *project.OwnerUserID
	}

	if project.OwnerOrgID != nil {
		return h.orgSvc.IsOwner(c.Request().Context(), *project.OwnerOrgID, viewer.ID)
	}

	return false
}

func (h *ProjectHandler) ListUserProjects(c echo.Context) error {
	owner, err := h.resolveUserByUsername(c, c.Param("username"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	viewer := h.getCurrentUser(c)
	includePrivate := viewer != nil && viewer.ID == owner.ID

	projects, err := h.projectSvc.ListByOwnerUser(c.Request().Context(), owner.ID, includePrivate)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list projects")
	}

	sanitizeProjectsForPublic(projects)
	return c.JSON(http.StatusOK, map[string]interface{}{"projects": projects})
}

func (h *ProjectHandler) CreateUserProject(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		IsPublic    *bool  `json:"is_public"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.Title) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title is required")
	}

	isPublic := true
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}

	project, err := h.projectSvc.CreateForUser(c.Request().Context(), currentUser.ID, currentUser.ID, req.Title, req.Description, isPublic)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create project")
	}

	sanitizeProjectForPublic(project)
	return c.JSON(http.StatusCreated, map[string]interface{}{"project": project})
}

func (h *ProjectHandler) ListOrgProjects(c echo.Context) error {
	org, err := h.orgSvc.GetByLogin(c.Request().Context(), c.Param("orgname"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "organization not found")
	}

	viewer := h.getCurrentUser(c)
	isMember := viewer != nil && h.orgSvc.IsMember(c.Request().Context(), org.ID, viewer.ID)
	if !org.IsPublic && !isMember {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	projects, err := h.projectSvc.ListByOwnerOrg(c.Request().Context(), org.ID, isMember)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list projects")
	}

	sanitizeProjectsForPublic(projects)
	return c.JSON(http.StatusOK, map[string]interface{}{"projects": projects})
}

func (h *ProjectHandler) CreateOrgProject(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	org, err := h.orgSvc.GetByLogin(c.Request().Context(), c.Param("orgname"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "organization not found")
	}
	if !h.orgSvc.IsOwner(c.Request().Context(), org.ID, currentUser.ID) {
		return echo.NewHTTPError(http.StatusForbidden, "only org owners can create projects")
	}

	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		IsPublic    *bool  `json:"is_public"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.Title) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title is required")
	}

	isPublic := true
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}

	project, err := h.projectSvc.CreateForOrg(c.Request().Context(), org.ID, currentUser.ID, req.Title, req.Description, isPublic)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create project")
	}

	sanitizeProjectForPublic(project)
	return c.JSON(http.StatusCreated, map[string]interface{}{"project": project})
}

func (h *ProjectHandler) resolveProject(c echo.Context, includeItems bool) (*models.Project, error) {
	project, err := h.projectSvc.GetByID(c.Request().Context(), c.Param("id"), includeItems)
	if err != nil {
		if errors.Is(err, services.ErrProjectNotFound) {
			return nil, echo.NewHTTPError(http.StatusNotFound, "project not found")
		}
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "failed to load project")
	}
	return project, nil
}

func (h *ProjectHandler) GetProject(c echo.Context) error {
	project, err := h.resolveProject(c, true)
	if err != nil {
		return err
	}
	if !h.canViewProject(c, project) {
		return echo.NewHTTPError(http.StatusNotFound, "project not found")
	}

	sanitizeProjectForPublic(project)
	return c.JSON(http.StatusOK, map[string]interface{}{"project": project})
}

func (h *ProjectHandler) UpdateProject(c echo.Context) error {
	project, err := h.resolveProject(c, false)
	if err != nil {
		return err
	}
	if !h.canManageProject(c, project) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		IsPublic    *bool   `json:"is_public"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	updates := map[string]interface{}{}
	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.IsPublic != nil {
		updates["is_public"] = *req.IsPublic
	}
	if len(updates) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no updates provided")
	}

	if err := h.projectSvc.UpdateProject(c.Request().Context(), project, updates); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	updated, err := h.projectSvc.GetByID(c.Request().Context(), project.ID, true)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to reload project")
	}

	sanitizeProjectForPublic(updated)
	return c.JSON(http.StatusOK, map[string]interface{}{"project": updated})
}

func (h *ProjectHandler) DeleteProject(c echo.Context) error {
	project, err := h.resolveProject(c, false)
	if err != nil {
		return err
	}
	if !h.canManageProject(c, project) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	if err := h.projectSvc.DeleteProject(c.Request().Context(), project); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete project")
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *ProjectHandler) CreateColumn(c echo.Context) error {
	project, err := h.resolveProject(c, false)
	if err != nil {
		return err
	}
	if !h.canManageProject(c, project) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Color       string `json:"color"`
		Position    *int   `json:"position"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	column, err := h.projectSvc.CreateColumn(c.Request().Context(), project, req.Name, req.Description, req.Color, req.Position)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{"column": column})
}

func (h *ProjectHandler) UpdateColumn(c echo.Context) error {
	project, err := h.resolveProject(c, false)
	if err != nil {
		return err
	}
	if !h.canManageProject(c, project) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Color       *string `json:"color"`
		Position    *int    `json:"position"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Color != nil {
		updates["color"] = *req.Color
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Position != nil {
		updates["position"] = *req.Position
	}
	if len(updates) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no updates provided")
	}

	column, err := h.projectSvc.UpdateColumn(c.Request().Context(), project, c.Param("columnID"), updates)
	if err != nil {
		if errors.Is(err, services.ErrColumnNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "column not found")
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"column": column})
}

func (h *ProjectHandler) DeleteColumn(c echo.Context) error {
	project, err := h.resolveProject(c, false)
	if err != nil {
		return err
	}
	if !h.canManageProject(c, project) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	if err := h.projectSvc.DeleteColumn(c.Request().Context(), project, c.Param("columnID")); err != nil {
		if errors.Is(err, services.ErrColumnNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "column not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete column")
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *ProjectHandler) CreateItem(c echo.Context) error {
	project, err := h.resolveProject(c, false)
	if err != nil {
		return err
	}
	if !h.canManageProject(c, project) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	var req struct {
		ColumnID       string  `json:"column_id"`
		Title          string  `json:"title"`
		Body           string  `json:"body"`
		Position       *int    `json:"position"`
		AssigneeUserID *string `json:"assignee_user_id"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	item, err := h.projectSvc.CreateItem(c.Request().Context(), project, req.ColumnID, req.Title, req.Body, req.Position, req.AssigneeUserID)
	if err != nil {
		if errors.Is(err, services.ErrColumnNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "column not found")
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if item.AssigneeUser != nil {
		sanitizeUserForPublic(item.AssigneeUser)
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"item": item})
}

func (h *ProjectHandler) UpdateItem(c echo.Context) error {
	project, err := h.resolveProject(c, false)
	if err != nil {
		return err
	}
	if !h.canManageProject(c, project) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	var req struct {
		Title          *string `json:"title"`
		Body           *string `json:"body"`
		AssigneeUserID *string `json:"assignee_user_id"`
		ClearAssignee  bool    `json:"clear_assignee"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	updates := map[string]interface{}{}
	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Body != nil {
		updates["body"] = *req.Body
	}
	if req.ClearAssignee {
		updates["assignee_user_id"] = nil
	} else if req.AssigneeUserID != nil {
		updates["assignee_user_id"] = *req.AssigneeUserID
	}
	if len(updates) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no updates provided")
	}

	item, err := h.projectSvc.UpdateItem(c.Request().Context(), project, c.Param("itemID"), updates)
	if err != nil {
		if errors.Is(err, services.ErrItemNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "item not found")
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if item.AssigneeUser != nil {
		sanitizeUserForPublic(item.AssigneeUser)
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"item": item})
}

func (h *ProjectHandler) MoveItem(c echo.Context) error {
	project, err := h.resolveProject(c, false)
	if err != nil {
		return err
	}
	if !h.canManageProject(c, project) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	var req struct {
		ColumnID *string `json:"column_id"`
		Position *int    `json:"position"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	item, err := h.projectSvc.MoveItem(c.Request().Context(), project, c.Param("itemID"), req.ColumnID, req.Position)
	if err != nil {
		if errors.Is(err, services.ErrItemNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "item not found")
		}
		if errors.Is(err, services.ErrColumnNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "column not found")
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if item.AssigneeUser != nil {
		sanitizeUserForPublic(item.AssigneeUser)
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"item": item})
}

func (h *ProjectHandler) DeleteItem(c echo.Context) error {
	project, err := h.resolveProject(c, false)
	if err != nil {
		return err
	}
	if !h.canManageProject(c, project) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	if err := h.projectSvc.DeleteItem(c.Request().Context(), project, c.Param("itemID")); err != nil {
		if errors.Is(err, services.ErrItemNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "item not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete item")
	}

	return c.NoContent(http.StatusNoContent)
}

func sanitizeProjectForPublic(project *models.Project) {
	if project == nil {
		return
	}
	if project.OwnerUser != nil {
		sanitizeUserForPublic(project.OwnerUser)
	}
	if project.OwnerOrg != nil {
		// keep org as-is
	}
	sanitizeUserForPublic(&project.CreatedBy)
	for ci := range project.Columns {
		for ii := range project.Columns[ci].Items {
			if project.Columns[ci].Items[ii].AssigneeUser != nil {
				sanitizeUserForPublic(project.Columns[ci].Items[ii].AssigneeUser)
			}
		}
	}
}

func sanitizeProjectsForPublic(projects []models.Project) {
	for i := range projects {
		sanitizeProjectForPublic(&projects[i])
	}
}
