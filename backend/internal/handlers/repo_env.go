package handlers

import (
	"net/http"
	"regexp"

	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

// validEnvName enforces GitHub Actions naming rules: uppercase letters, digits, underscores,
// must start with a letter or underscore.
var validEnvName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// RepoEnvHandler handles CRUD for repository variables and secrets.
type RepoEnvHandler struct {
	repoEnvSvc *services.RepoEnvService
	repoSvc    *services.RepoService
}

func NewRepoEnvHandler(repoEnvSvc *services.RepoEnvService, repoSvc *services.RepoService) *RepoEnvHandler {
	return &RepoEnvHandler{repoEnvSvc: repoEnvSvc, repoSvc: repoSvc}
}

// ListVariables returns all variables for a repository (names + values).
// GET /repos/:username/:repo/actions/variables
func (h *RepoEnvHandler) ListVariables(c echo.Context) error {
	repo, err := h.requireWriteAccess(c)
	if err != nil {
		return err
	}
	vars, err := h.repoEnvSvc.ListVariables(repo.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list variables")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"variables": vars})
}

// SetVariable creates or updates a named variable.
// PUT /repos/:username/:repo/actions/variables/:name
func (h *RepoEnvHandler) SetVariable(c echo.Context) error {
	repo, err := h.requireWriteAccess(c)
	if err != nil {
		return err
	}
	name := c.Param("name")
	if !validEnvName.MatchString(name) {
		return echo.NewHTTPError(http.StatusBadRequest, "variable name must match [A-Z_][A-Z0-9_]*")
	}
	var body struct {
		Value string `json:"value"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := h.repoEnvSvc.SetVariable(repo.ID, name, body.Value); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to set variable")
	}
	return c.NoContent(http.StatusNoContent)
}

// DeleteVariable removes a named variable.
// DELETE /repos/:username/:repo/actions/variables/:name
func (h *RepoEnvHandler) DeleteVariable(c echo.Context) error {
	repo, err := h.requireWriteAccess(c)
	if err != nil {
		return err
	}
	name := c.Param("name")
	if err := h.repoEnvSvc.DeleteVariable(repo.ID, name); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete variable")
	}
	return c.NoContent(http.StatusNoContent)
}

// ListSecrets returns secret metadata (names + timestamps) — never the values.
// GET /repos/:username/:repo/actions/secrets
func (h *RepoEnvHandler) ListSecrets(c echo.Context) error {
	repo, err := h.requireWriteAccess(c)
	if err != nil {
		return err
	}
	secrets, err := h.repoEnvSvc.ListSecrets(repo.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list secrets")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"secrets": secrets})
}

// SetSecret encrypts and stores (or updates) a named secret.
// PUT /repos/:username/:repo/actions/secrets/:name
func (h *RepoEnvHandler) SetSecret(c echo.Context) error {
	repo, err := h.requireWriteAccess(c)
	if err != nil {
		return err
	}
	name := c.Param("name")
	if !validEnvName.MatchString(name) {
		return echo.NewHTTPError(http.StatusBadRequest, "secret name must match [A-Z_][A-Z0-9_]*")
	}
	var body struct {
		Value string `json:"value"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if body.Value == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "secret value cannot be empty")
	}
	if err := h.repoEnvSvc.SetSecret(repo.ID, name, body.Value); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to set secret")
	}
	return c.NoContent(http.StatusNoContent)
}

// DeleteSecret removes a named secret.
// DELETE /repos/:username/:repo/actions/secrets/:name
func (h *RepoEnvHandler) DeleteSecret(c echo.Context) error {
	repo, err := h.requireWriteAccess(c)
	if err != nil {
		return err
	}
	name := c.Param("name")
	if err := h.repoEnvSvc.DeleteSecret(repo.ID, name); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete secret")
	}
	return c.NoContent(http.StatusNoContent)
}

// requireWriteAccess validates that the authenticated user has write/admin access to the repo.
func (h *RepoEnvHandler) requireWriteAccess(c echo.Context) (*models.Repository, error) {
	username := c.Param("username")
	repoName := c.Param("repo")
	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	currentUser, ok := c.Get("user").(*models.User)
	if !ok || currentUser == nil || !h.repoSvc.IsAdminAccess(repo, currentUser.ID) {
		return nil, echo.NewHTTPError(http.StatusForbidden, "access denied")
	}
	return repo, nil
}
