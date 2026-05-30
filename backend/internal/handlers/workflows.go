package handlers

import (
	"net/http"
	"strconv"

	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

// WorkflowHandler serves the Actions API.
type WorkflowHandler struct {
	workflowSvc *services.WorkflowService
	repoSvc     *services.RepoService
}

func NewWorkflowHandler(workflowSvc *services.WorkflowService, repoSvc *services.RepoService) *WorkflowHandler {
	return &WorkflowHandler{workflowSvc: workflowSvc, repoSvc: repoSvc}
}

// ListRuns returns a paginated list of workflow runs for a repository.
// GET /repos/:username/:repo/actions
func (h *WorkflowHandler) ListRuns(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	// Private repo: require auth
	if repo.IsPrivate {
		currentUser, ok := c.Get("user").(*models.User)
		if !ok || currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false) {
			return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
		}
	}

	limit := 20
	offset := 0
	if l := c.QueryParam("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	if o := c.QueryParam("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	runs, total, err := h.workflowSvc.GetRunsByRepo(repo.ID, limit, offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to fetch runs")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"runs":   runs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetRun returns a single workflow run with all jobs and step logs.
// GET /repos/:username/:repo/actions/:runID
func (h *WorkflowHandler) GetRun(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")
	runIDStr := c.Param("runID")
	if runIDStr == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid run ID")
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.IsPrivate {
		currentUser, ok := c.Get("user").(*models.User)
		if !ok || currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false) {
			return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
		}
	}

	run, err := h.workflowSvc.GetRun(runIDStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "run not found")
	}

	// Ensure the run belongs to this repo
	if run.RepoID != repo.ID {
		return echo.NewHTTPError(http.StatusNotFound, "run not found")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"run": run,
	})
}

// CancelRun cancels a pending/running workflow run.
// POST /repos/:username/:repo/actions/:runID/cancel
func (h *WorkflowHandler) CancelRun(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")
	runIDStr := c.Param("runID")
	if runIDStr == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid run ID")
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	currentUser, ok := c.Get("user").(*models.User)
	if !ok || currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	if err := h.workflowSvc.CancelRun(runIDStr); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to cancel run")
	}

	return c.NoContent(http.StatusNoContent)
}

// DispatchRun manually triggers workflow_dispatch workflows.
// POST /repos/:username/:repo/actions/dispatch
func (h *WorkflowHandler) DispatchRun(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	currentUser, ok := c.Get("user").(*models.User)
	if !ok || currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	var body struct {
		Ref          string `json:"ref"`
		WorkflowFile string `json:"workflow_file"`
	}
	if err := c.Bind(&body); err != nil || body.Ref == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "ref is required")
	}

	count, err := h.workflowSvc.DispatchWorkflow(c.Request().Context(), repo.ID, username, repoName, body.Ref, body.WorkflowFile)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, err.Error())
	}
	if count == 0 {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "no workflow files found on this ref")
	}

	return c.JSON(http.StatusAccepted, map[string]interface{}{"runs_created": count})
}

// ListDispatchable returns workflow files that have a workflow_dispatch trigger at the given ref.
// GET /repos/:username/:repo/actions/dispatchable?ref=<branch>
func (h *WorkflowHandler) ListDispatchable(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")
	ref := c.QueryParam("ref")
	if ref == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "ref is required")
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	currentUser, ok := c.Get("user").(*models.User)
	if !ok || currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	paths, err := h.workflowSvc.ListDispatchableWorkflows(username, repoName, ref)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list workflows")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"workflows": paths})
}

// RerunWorkflow re-runs an existing workflow run.
// POST /repos/:username/:repo/actions/:runID/rerun
func (h *WorkflowHandler) RerunWorkflow(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")
	runIDStr := c.Param("runID")
	if runIDStr == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid run ID")
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	currentUser, ok := c.Get("user").(*models.User)
	if !ok || currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	run, err := h.workflowSvc.GetRun(runIDStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "run not found")
	}

	if run.RepoID != repo.ID {
		return echo.NewHTTPError(http.StatusNotFound, "run not found")
	}

	newRunID, err := h.workflowSvc.RerunWorkflow(c.Request().Context(), runIDStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"run_id": newRunID})
}

// DeleteRun deletes a workflow run.
// DELETE /repos/:username/:repo/actions/:runID
func (h *WorkflowHandler) DeleteRun(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")
	runIDStr := c.Param("runID")
	if runIDStr == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid run ID")
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	currentUser, ok := c.Get("user").(*models.User)
	if !ok || currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	run, err := h.workflowSvc.GetRun(runIDStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "run not found")
	}

	if run.RepoID != repo.ID {
		return echo.NewHTTPError(http.StatusNotFound, "run not found")
	}

	if err := h.workflowSvc.DeleteRun(runIDStr); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete run")
	}

	return c.NoContent(http.StatusNoContent)
}

// GetUsage returns monthly Actions minutes usage for the account/org that owns the repo.
// GET /repos/:username/:repo/actions/usage
func (h *WorkflowHandler) GetUsage(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	currentUser, ok := c.Get("user").(*models.User)
	if !ok || currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	used, limit, month, err := h.workflowSvc.GetActionsUsageForRepo(repo.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load actions usage")
	}

	remaining := limit - used
	if remaining < 0 {
		remaining = 0
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"used_minutes":      used,
		"limit_minutes":     limit,
		"remaining_minutes": remaining,
		"month":             month,
	})
}
