package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

type IssueHandler struct {
	issueSvc   *services.IssueService
	repoSvc    *services.RepoService
	modSvc     *services.ModerationService
	webhookSvc *services.WebhookService
}

func NewIssueHandler(issueSvc *services.IssueService, repoSvc *services.RepoService) *IssueHandler {
	return &IssueHandler{issueSvc: issueSvc, repoSvc: repoSvc}
}

func (h *IssueHandler) SetModerationService(modSvc *services.ModerationService) {
	h.modSvc = modSvc
}

func (h *IssueHandler) SetWebhookService(webhookSvc *services.WebhookService, repoSvc *services.RepoService) {
	h.webhookSvc = webhookSvc
	h.repoSvc = repoSvc
}

func (h *IssueHandler) resolveRepo(c echo.Context) (*models.Repository, error) {
	username := c.Param("username")
	repoName := c.Param("repo")
	return h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
}

func (h *IssueHandler) parseNumber(c echo.Context) (uint, error) {
	n, err := strconv.ParseUint(c.Param("number"), 10, 64)
	if err != nil {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "invalid issue number")
	}
	return uint(n), nil
}

func (h *IssueHandler) List(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}
	if repo.IsPrivate && (currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false)) {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	status := c.QueryParam("status")

	// Optional label filter
	var labelIDs []string
	for _, raw := range c.QueryParams()["label_id"] {
		if raw != "" {
			labelIDs = append(labelIDs, raw)
		}
	}

	issues, err := h.issueSvc.GetByRepo(c.Request().Context(), repo.ID, status, labelIDs)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list issues")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"issues": issues})
}

func (h *IssueHandler) Get(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}
	if repo.IsPrivate && (currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false)) {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	number, err := h.parseNumber(c)
	if err != nil {
		return err
	}

	issue, err := h.issueSvc.GetByRepoAndNumber(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "issue not found")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"issue": issue})
}

func (h *IssueHandler) Create(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.IsPrivate && !h.repoSvc.HasAccess(repo, currentUser.ID, false) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	var req struct {
		Title      string   `json:"title"`
		Body       string   `json:"body"`
		IssueType  string   `json:"issue_type"`
		AssigneeID *string  `json:"assignee_id"`
		LabelIDs   []string `json:"label_ids"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title is required")
	}

	// Moderation check
	if h.modSvc != nil {
		if err := h.modSvc.CheckAllowed(c.Request().Context(), services.CheckInput{
			RepoID:      repo.ID,
			ActorID:     currentUser.ID,
			ActorJoined: currentUser.CreatedAt,
			ContextType: "issues",
			Content:     []string{req.Title, req.Body},
		}); err != nil {
			return ModerationError(err)
		}
	}

	issue, err := h.issueSvc.Create(c.Request().Context(), services.CreateIssueInput{
		Title:      req.Title,
		Body:       req.Body,
		IssueType:  req.IssueType,
		RepoID:     repo.ID,
		AuthorID:   currentUser.ID,
		AssigneeID: req.AssigneeID,
		LabelIDs:   req.LabelIDs,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create issue")
	}

	if h.webhookSvc != nil {
		h.webhookSvc.Deliver(c.Request().Context(), repo.ID, "issues", map[string]interface{}{
			"action": "opened",
			"issue":  issue,
			"repository": map[string]interface{}{
				"id":        repo.ID,
				"name":      repo.Name,
				"full_name": repo.Owner.Username + "/" + repo.Name,
			},
			"sender": map[string]interface{}{"login": currentUser.Username},
		})
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{"issue": issue})
}

func (h *IssueHandler) Update(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	number, err := h.parseNumber(c)
	if err != nil {
		return err
	}

	issue, err := h.issueSvc.GetByRepoAndNumber(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "issue not found")
	}

	if issue.AuthorID != currentUser.ID && repo.OwnerID != currentUser.ID {
		return echo.NewHTTPError(http.StatusForbidden, "only the author or repository owner can edit this issue")
	}

	var req struct {
		Title          *string `json:"title"`
		Body           *string `json:"body"`
		IssueType      *string `json:"issue_type"`
		AssigneeID     *string `json:"assignee_id"`
		ClearAssignee  bool    `json:"clear_assignee"`
		MilestoneID    *string `json:"milestone_id"`
		ClearMilestone bool    `json:"clear_milestone"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	updates := map[string]interface{}{}
	if req.Title != nil {
		if *req.Title == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "title cannot be empty")
		}
		updates["title"] = *req.Title
	}
	if req.Body != nil {
		updates["body"] = *req.Body
	}
	if req.IssueType != nil {
		updates["issue_type"] = *req.IssueType
	}
	if req.ClearAssignee {
		updates["assignee_id"] = nil
	} else if req.AssigneeID != nil {
		updates["assignee_id"] = *req.AssigneeID
	}
	if req.ClearMilestone {
		updates["milestone_id"] = nil
	} else if req.MilestoneID != nil {
		updates["milestone_id"] = *req.MilestoneID
	}

	updated, err := h.issueSvc.Update(c.Request().Context(), repo.ID, number, updates)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update issue")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"issue": updated})
}

func (h *IssueHandler) Close(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	number, err := h.parseNumber(c)
	if err != nil {
		return err
	}

	issue, err := h.issueSvc.GetByRepoAndNumber(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "issue not found")
	}

	if issue.AuthorID != currentUser.ID && repo.OwnerID != currentUser.ID {
		return echo.NewHTTPError(http.StatusForbidden, "only the author or repository owner can close this issue")
	}

	updated, err := h.issueSvc.Close(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to close issue")
	}

	if h.webhookSvc != nil {
		h.webhookSvc.Deliver(c.Request().Context(), repo.ID, "issues", map[string]interface{}{
			"action": "closed",
			"issue":  updated,
			"repository": map[string]interface{}{
				"id":        repo.ID,
				"name":      repo.Name,
				"full_name": repo.Owner.Username + "/" + repo.Name,
			},
			"sender": map[string]interface{}{"login": currentUser.Username},
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"issue": updated})
}

func (h *IssueHandler) Reopen(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	number, err := h.parseNumber(c)
	if err != nil {
		return err
	}

	issue, err := h.issueSvc.GetByRepoAndNumber(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "issue not found")
	}

	if issue.AuthorID != currentUser.ID && repo.OwnerID != currentUser.ID {
		return echo.NewHTTPError(http.StatusForbidden, "only the author or repository owner can reopen this issue")
	}

	updated, err := h.issueSvc.Reopen(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to reopen issue")
	}

	if h.webhookSvc != nil {
		h.webhookSvc.Deliver(c.Request().Context(), repo.ID, "issues", map[string]interface{}{
			"action": "reopened",
			"issue":  updated,
			"repository": map[string]interface{}{
				"id":        repo.ID,
				"name":      repo.Name,
				"full_name": repo.Owner.Username + "/" + repo.Name,
			},
			"sender": map[string]interface{}{"login": currentUser.Username},
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"issue": updated})
}

func (h *IssueHandler) Delete(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.OwnerID != currentUser.ID {
		return echo.NewHTTPError(http.StatusForbidden, "only the repository owner can delete issues")
	}

	number, err := h.parseNumber(c)
	if err != nil {
		return err
	}

	if err := h.issueSvc.Delete(c.Request().Context(), repo.ID, number); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete issue")
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *IssueHandler) ListLabels(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	labels, err := h.issueSvc.ListLabels(c.Request().Context(), repo.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list labels")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"labels": labels})
}

func (h *IssueHandler) CreateLabel(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.OwnerID != currentUser.ID && !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "only repository members with write access can create labels")
	}

	var req struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		Description string `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "label name is required")
	}
	if req.Color == "" {
		req.Color = "#0075ca"
	}

	label, err := h.issueSvc.CreateLabel(c.Request().Context(), repo.ID, req.Name, req.Color, req.Description)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create label")
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{"label": label})
}

func (h *IssueHandler) UpdateLabel(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.OwnerID != currentUser.ID && !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "only repository members with write access can update labels")
	}

	labelID := c.Param("labelID")
	if labelID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid label ID")
	}

	var req struct {
		Name        *string `json:"name"`
		Color       *string `json:"color"`
		Description *string `json:"description"`
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

	label, err := h.issueSvc.UpdateLabel(c.Request().Context(), labelID, repo.ID, updates)
	if err != nil {
		if errors.Is(err, services.ErrLabelNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "label not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update label")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"label": label})
}

func (h *IssueHandler) DeleteLabel(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.OwnerID != currentUser.ID {
		return echo.NewHTTPError(http.StatusForbidden, "only the repository owner can delete labels")
	}

	labelID := c.Param("labelID")
	if labelID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid label ID")
	}

	if err := h.issueSvc.DeleteLabel(c.Request().Context(), labelID, repo.ID); err != nil {
		if errors.Is(err, services.ErrLabelNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "label not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete label")
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *IssueHandler) SetLabels(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.OwnerID != currentUser.ID && !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	number, err := h.parseNumber(c)
	if err != nil {
		return err
	}

	var req struct {
		LabelIDs []string `json:"label_ids"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	issue, err := h.issueSvc.SetLabels(c.Request().Context(), repo.ID, number, req.LabelIDs)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update labels")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"issue": issue})
}

func (h *IssueHandler) ListComments(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}
	if repo.IsPrivate && (currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false)) {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	number, err := h.parseNumber(c)
	if err != nil {
		return err
	}

	issue, err := h.issueSvc.GetByRepoAndNumber(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "issue not found")
	}

	comments, err := h.issueSvc.ListComments(c.Request().Context(), issue.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list comments")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"comments": comments})
}

func (h *IssueHandler) CreateComment(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.IsPrivate && !h.repoSvc.HasAccess(repo, currentUser.ID, false) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	number, err := h.parseNumber(c)
	if err != nil {
		return err
	}

	issue, err := h.issueSvc.GetByRepoAndNumber(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "issue not found")
	}

	var req struct {
		Body string `json:"body"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Body == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "comment body is required")
	}

	// Moderation check
	if h.modSvc != nil {
		if err := h.modSvc.CheckAllowed(c.Request().Context(), services.CheckInput{
			RepoID:      repo.ID,
			ActorID:     currentUser.ID,
			ActorJoined: currentUser.CreatedAt,
			ContextType: "comments",
			Content:     []string{req.Body},
		}); err != nil {
			return ModerationError(err)
		}
	}

	comment, err := h.issueSvc.CreateComment(c.Request().Context(), issue.ID, currentUser.ID, req.Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create comment")
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{"comment": comment})
}

func (h *IssueHandler) UpdateComment(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	commentID := c.Param("commentID")
	if commentID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid comment ID")
	}

	var req struct {
		Body string `json:"body"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Body == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "comment body is required")
	}

	comment, err := h.issueSvc.UpdateComment(c.Request().Context(), commentID, currentUser.ID, req.Body)
	if err != nil {
		if errors.Is(err, services.ErrCommentNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "comment not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update comment")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"comment": comment})
}

func (h *IssueHandler) DeleteComment(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	commentID := c.Param("commentID")
	if commentID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid comment ID")
	}

	isOwner := repo.OwnerID == currentUser.ID
	if err := h.issueSvc.DeleteComment(c.Request().Context(), commentID, currentUser.ID, isOwner); err != nil {
		if errors.Is(err, services.ErrCommentNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "comment not found")
		}
		if err.Error() == "forbidden" {
			return echo.NewHTTPError(http.StatusForbidden, "not allowed to delete this comment")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete comment")
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *IssueHandler) ListMilestones(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	status := c.QueryParam("status")
	milestones, err := h.issueSvc.ListMilestones(c.Request().Context(), repo.ID, status)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list milestones")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"milestones": milestones})
}

func (h *IssueHandler) CreateMilestone(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	if repo.OwnerID != currentUser.ID && !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := c.Bind(&req); err != nil || req.Title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title is required")
	}
	m, err := h.issueSvc.CreateMilestone(c.Request().Context(), repo.ID, req.Title, req.Description)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create milestone")
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"milestone": m})
}

func (h *IssueHandler) UpdateMilestone(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	if repo.OwnerID != currentUser.ID && !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}
	milestoneID := c.Param("milestoneID")
	if milestoneID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid milestone ID")
	}
	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
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
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	m, err := h.issueSvc.UpdateMilestone(c.Request().Context(), repo.ID, milestoneID, updates)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update milestone")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"milestone": m})
}

func (h *IssueHandler) DeleteMilestone(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	repo, err := h.resolveRepo(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	if repo.OwnerID != currentUser.ID && !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}
	milestoneID := c.Param("milestoneID")
	if milestoneID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid milestone ID")
	}
	if err := h.issueSvc.DeleteMilestone(c.Request().Context(), repo.ID, milestoneID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete milestone")
	}
	return c.NoContent(http.StatusNoContent)
}
