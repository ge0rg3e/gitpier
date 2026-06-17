package handlers

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

// ReleaseHandler serves the Releases API.
type ReleaseHandler struct {
	releaseSvc  *services.ReleaseService
	repoSvc     *services.RepoService
	gitSvc      *services.GitService
	workflowSvc *services.WorkflowService
}

func NewReleaseHandler(releaseSvc *services.ReleaseService, repoSvc *services.RepoService, gitSvc *services.GitService, workflowSvc *services.WorkflowService) *ReleaseHandler {
	return &ReleaseHandler{releaseSvc: releaseSvc, repoSvc: repoSvc, gitSvc: gitSvc, workflowSvc: workflowSvc}
}

func (h *ReleaseHandler) triggerPublishedReleaseWorkflow(ctx context.Context, repo *models.Repository, tagName string) {
	if h.workflowSvc == nil || strings.TrimSpace(tagName) == "" {
		return
	}

	namespace := repo.Owner.Username
	if repo.OrgID != nil && repo.Org != nil {
		namespace = repo.Org.Login
	}
	repoPath := h.repoSvc.RepoPath(namespace, repo.Name)

	commit, commitErr := h.gitSvc.GetHeadCommit(repoPath, tagName)
	if commitErr != nil {
		log.Printf("release workflow trigger skipped (tag resolve failed): %v", commitErr)
		return
	}

	if err := h.workflowSvc.TriggerWorkflows(
		ctx,
		repo.ID,
		namespace,
		repo.Name,
		"release",
		tagName,
		commit.SHA,
		"published",
	); err != nil {
		log.Printf("release workflow trigger error: %v", err)
	}
}

// resolveRepo is a small helper that fetches the repo and checks read access.
func (h *ReleaseHandler) resolveRepo(c echo.Context) (*models.Repository, error) {
	username := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.IsPrivate {
		currentUser, ok := c.Get("user").(*models.User)
		if !ok || currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false) {
			return nil, echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
		}
	}
	return repo, nil
}

// requireAdmin checks that the current user can administer the repo (owner or collaborator).
func (h *ReleaseHandler) requireAdmin(c echo.Context, repo *models.Repository) error {
	currentUser, ok := c.Get("user").(*models.User)
	if !ok || currentUser == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}
	if !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "insufficient permissions")
	}
	return nil
}

// List returns all releases for a repository.
// GET /repos/:username/:repo/releases
func (h *ReleaseHandler) List(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}

	// Drafts visible only to admins
	includeDrafts := false
	currentUser, ok := c.Get("user").(*models.User)
	if ok && currentUser != nil && h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		includeDrafts = true
	}

	releases, err := h.releaseSvc.List(c.Request().Context(), repo.ID, includeDrafts)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to fetch releases")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"releases": releases})
}

// GetLatest returns the most recent stable (non-draft, non-prerelease) release.
// GET /repos/:username/:repo/releases/latest
func (h *ReleaseHandler) GetLatest(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}
	r, err := h.releaseSvc.GetLatest(c.Request().Context(), repo.ID)
	if errors.Is(err, services.ErrReleaseNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "no releases found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to fetch release")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"release": r})
}

// GetByTag returns the release attached to a specific tag.
// GET /repos/:username/:repo/releases/tags/:tag
func (h *ReleaseHandler) GetByTag(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}
	tagName := c.Param("tag")
	r, err := h.releaseSvc.GetByTag(c.Request().Context(), repo.ID, tagName)
	if errors.Is(err, services.ErrReleaseNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "release not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to fetch release")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"release": r})
}

// GetByID returns a single release by its numeric ID.
// GET /repos/:username/:repo/releases/:id
func (h *ReleaseHandler) GetByID(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid release ID")
	}
	r, err := h.releaseSvc.Get(c.Request().Context(), repo.ID, id)
	if errors.Is(err, services.ErrReleaseNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "release not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to fetch release")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"release": r})
}

// Create creates a new release.
// POST /repos/:username/:repo/releases
func (h *ReleaseHandler) Create(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}
	if err := h.requireAdmin(c, repo); err != nil {
		return err
	}

	var req struct {
		TagName      string `json:"tag_name"`
		TargetCommit string `json:"target_commitish"`
		Name         string `json:"name"`
		Body         string `json:"body"`
		IsDraft      bool   `json:"is_draft"`
		IsPrerelease bool   `json:"is_prerelease"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.TagName == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "tag_name is required")
	}

	currentUser := c.Get("user").(*models.User)
	namespace := repo.Owner.Username
	if repo.OrgID != nil && repo.Org != nil {
		namespace = repo.Org.Login
	}
	repoPath := h.repoSvc.RepoPath(namespace, repo.Name)

	r, err := h.releaseSvc.Create(c.Request().Context(), repo.ID, currentUser.ID, repoPath, services.CreateReleaseInput{
		TagName:      req.TagName,
		TargetCommit: req.TargetCommit,
		Name:         req.Name,
		Body:         req.Body,
		IsDraft:      req.IsDraft,
		IsPrerelease: req.IsPrerelease,
	})
	if err != nil {
		log.Printf("release create error: %v", err)
		switch {
		case errors.Is(err, services.ErrReleaseBadInput):
			return echo.NewHTTPError(http.StatusBadRequest, "invalid tag name or target branch/commit")
		case errors.Is(err, services.ErrReleaseBadTarget):
			return echo.NewHTTPError(http.StatusBadRequest, "target branch/commit was not found")
		case errors.Is(err, services.ErrReleaseRepoEmpty):
			return echo.NewHTTPError(http.StatusBadRequest, "cannot create a release on an empty repository")
		case errors.Is(err, services.ErrReleaseTagExists):
			return echo.NewHTTPError(http.StatusConflict, "tag already exists")
		case errors.Is(err, services.ErrReleaseRepoGone):
			return echo.NewHTTPError(http.StatusConflict, "repository git data is missing on disk")
		default:
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to create release")
		}
	}

	if !r.IsDraft {
		h.triggerPublishedReleaseWorkflow(c.Request().Context(), repo, r.TagName)
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"release": r})
}

// Update edits a release.
// PATCH /repos/:username/:repo/releases/:id
func (h *ReleaseHandler) Update(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}
	if err := h.requireAdmin(c, repo); err != nil {
		return err
	}
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid release ID")
	}

	var req struct {
		Name         *string `json:"name"`
		Body         *string `json:"body"`
		IsDraft      *bool   `json:"is_draft"`
		IsPrerelease *bool   `json:"is_prerelease"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	existingRelease, err := h.releaseSvc.Get(c.Request().Context(), repo.ID, id)
	if errors.Is(err, services.ErrReleaseNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "release not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to fetch release")
	}

	r, err := h.releaseSvc.Update(c.Request().Context(), repo.ID, id, services.UpdateReleaseInput{
		Name:         req.Name,
		Body:         req.Body,
		IsDraft:      req.IsDraft,
		IsPrerelease: req.IsPrerelease,
	})
	if errors.Is(err, services.ErrReleaseNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "release not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if existingRelease.IsDraft && !r.IsDraft {
		h.triggerPublishedReleaseWorkflow(c.Request().Context(), repo, r.TagName)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"release": r})
}

// Delete deletes a release and all its assets.
// DELETE /repos/:username/:repo/releases/:id
func (h *ReleaseHandler) Delete(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}
	if err := h.requireAdmin(c, repo); err != nil {
		return err
	}
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid release ID")
	}

	if err := h.releaseSvc.Delete(c.Request().Context(), repo.ID, id); err != nil {
		if errors.Is(err, services.ErrReleaseNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "release not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// UploadAsset attaches a binary asset to a release.
// POST /repos/:username/:repo/releases/:id/assets
func (h *ReleaseHandler) UploadAsset(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}
	if err := h.requireAdmin(c, repo); err != nil {
		return err
	}
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid release ID")
	}

	// Verify the release exists and belongs to this repo
	if _, err := h.releaseSvc.Get(c.Request().Context(), repo.ID, id); err != nil {
		if errors.Is(err, services.ErrReleaseNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "release not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	file, header, err := c.Request().FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file is required")
	}
	defer file.Close()

	// Allow caller to override the filename via a "name" form field
	name := c.FormValue("name")
	if name == "" {
		name = header.Filename
	}

	contentType := header.Header.Get("Content-Type")

	asset, err := h.releaseSvc.UploadAsset(c.Request().Context(), id, name, contentType, file)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"asset": asset})
}

// DeleteAsset removes a binary asset from a release.
// DELETE /repos/:username/:repo/releases/:id/assets/:assetId
func (h *ReleaseHandler) DeleteAsset(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}
	if err := h.requireAdmin(c, repo); err != nil {
		return err
	}
	id := c.Param("id")
	assetID := c.Param("assetId")
	if id == "" || assetID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid ID")
	}

	if err := h.releaseSvc.DeleteAsset(c.Request().Context(), id, assetID); err != nil {
		if errors.Is(err, services.ErrAssetNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "asset not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// DownloadAsset streams a release asset to the client.
// GET /repos/:username/:repo/releases/assets/:assetId
func (h *ReleaseHandler) DownloadAsset(c echo.Context) error {
	// NOTE: No resolveRepo check here â€” asset ID alone is the address; we rely
	// on the asset record owning a valid release which owns a valid repo. The
	// caller still needs read access so we check via the URL params.
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}

	assetID := c.Param("assetId")
	if assetID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid asset ID")
	}

	asset, err := h.releaseSvc.GetAsset(c.Request().Context(), assetID)
	if errors.Is(err, services.ErrAssetNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "asset not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Verify asset belongs to a release in this repo (security check)
	release, err := h.releaseSvc.Get(c.Request().Context(), repo.ID, asset.ReleaseID)
	if err != nil || release.RepoID != repo.ID {
		return echo.NewHTTPError(http.StatusNotFound, "asset not found")
	}

	h.releaseSvc.IncrementDownloadCount(c.Request().Context(), asset.ID)

	f, err := os.Open(asset.StoragePath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "asset file not found on server")
	}
	defer f.Close()

	c.Response().Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(asset.Name, `"`, `_`)+`"`)
	c.Response().Header().Set("Cache-Control", "no-cache")
	_, err = io.Copy(c.Response().Writer, f)
	return err
}

// DownloadSource returns a source archive (zip or tar.gz) for a release's tag.
// GET /repos/:username/:repo/releases/:id/source.zip
// GET /repos/:username/:repo/releases/:id/source.tar.gz
func (h *ReleaseHandler) DownloadSource(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid release ID")
	}

	r, err := h.releaseSvc.Get(c.Request().Context(), repo.ID, id)
	if errors.Is(err, services.ErrReleaseNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "release not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	format := c.Param("format") // "zip" or "tar.gz"
	if format != "zip" && format != "tar.gz" {
		return echo.NewHTTPError(http.StatusBadRequest, "format must be zip or tar.gz")
	}

	repoNamespace := repo.Owner.Username
	if repo.OrgID != nil && repo.Org != nil {
		repoNamespace = repo.Org.Login
	}
	repoPath := h.repoSvc.RepoPath(repoNamespace, repo.Name)
	data, err := h.gitSvc.GetArchive(repoPath, r.TagName, format)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate archive")
	}

	mimeType := "application/zip"
	if format == "tar.gz" {
		mimeType = "application/gzip"
	}
	filename := repo.Name + "-" + r.TagName + "." + format

	c.Response().Header().Set("Content-Disposition", "attachment; filename="+filename)
	c.Response().Header().Set("Cache-Control", "no-cache")
	return c.Blob(http.StatusOK, mimeType, data)
}

// GetTags returns the list of git tags for a repository.
// GET /repos/:username/:repo/releases/tags
func (h *ReleaseHandler) GetTags(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}
	tagNamespace := repo.Owner.Username
	if repo.OrgID != nil && repo.Org != nil {
		tagNamespace = repo.Org.Login
	}
	repoPath := h.repoSvc.RepoPath(tagNamespace, repo.Name)
	tags, err := h.gitSvc.GetTags(repoPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to fetch tags")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"tags": tags})
}
