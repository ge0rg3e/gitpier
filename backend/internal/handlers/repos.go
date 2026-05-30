package handlers

import (
	"errors"
	"fmt"
	"gitpier/internal/cache"
	"gitpier/internal/config"
	"gitpier/internal/models"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitpier/internal/services"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/labstack/echo/v4"
)

type RepoHandler struct {
	repoSvc *services.RepoService
	gitSvc  *services.GitService
	orgSvc  *services.OrgService
	authSvc *services.AuthService
	cache   cache.Store
	cfg     *config.Config
}

func NewRepoHandler(repoSvc *services.RepoService, gitSvc *services.GitService, orgSvc *services.OrgService, authSvc *services.AuthService, cacheStore cache.Store, cfg *config.Config) *RepoHandler {
	return &RepoHandler{repoSvc: repoSvc, gitSvc: gitSvc, orgSvc: orgSvc, authSvc: authSvc, cache: cacheStore, cfg: cfg}
}

func (h *RepoHandler) Create(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	if !canCreateRepositories(h.cfg, currentUser) {
		return echo.NewHTTPError(http.StatusForbidden, repoCreationRestrictedMessage)
	}

	var req struct {
		Name                 string `json:"name"`
		Description          string `json:"description"`
		IsPrivate            bool   `json:"is_private"`
		InitializeWithReadme bool   `json:"initialize_with_readme"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if !usernameRe.MatchString(req.Name) || len(req.Name) < 1 || len(req.Name) > 100 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid repository name")
	}

	repo, err := h.repoSvc.Create(c.Request().Context(), services.CreateRepoInput{
		Name:        req.Name,
		Description: req.Description,
		IsPrivate:   req.IsPrivate,
		OwnerID:     currentUser.ID,
	})
	if err != nil {
		if errors.Is(err, services.ErrRepoExists) {
			return echo.NewHTTPError(http.StatusConflict, "repository already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create repository")
	}

	// Initialize bare git repo on disk
	repoPath := h.repoSvc.RepoPath(currentUser.Username, repo.Name)
	if err := h.gitSvc.InitRepo(repoPath); err != nil {
		// Rollback DB entry
		h.repoSvc.Delete(c.Request().Context(), repo)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to initialize repository")
	}
	if req.InitializeWithReadme {
		if err := h.gitSvc.InitializeWithReadme(repoPath, repo.DefaultBranch, repo.Name, currentUser.Username, currentUser.Email); err != nil {
			h.repoSvc.Delete(c.Request().Context(), repo)
			_ = h.gitSvc.DeleteRepo(repoPath)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to initialize repository")
		}
	}
	_ = h.cache.DeleteByPrefix(c.Request().Context(), "cache:explore:")

	return c.JSON(http.StatusCreated, repo)
}

func (h *RepoHandler) Fork(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	sourceRepo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if sourceRepo.IsPrivate {
		return echo.NewHTTPError(http.StatusForbidden, "only public repositories can be forked")
	}

	var req struct {
		Owner              string  `json:"owner"`
		Name               string  `json:"name"`
		Description        *string `json:"description"`
		CopyMainBranchOnly bool    `json:"copy_main_branch_only"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	forkName := repoName
	if req.Name != "" {
		forkName = req.Name
	}

	if !usernameRe.MatchString(forkName) || len(forkName) < 1 || len(forkName) > 100 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid repository name")
	}

	targetNamespace := currentUser.Username
	targetOwnerID := currentUser.ID
	var targetOrgID *string
	if req.Owner != "" && req.Owner != currentUser.Username {
		org, orgErr := h.orgSvc.GetByLogin(c.Request().Context(), req.Owner)
		if orgErr != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid fork owner")
		}
		if !h.orgSvc.IsMember(c.Request().Context(), org.ID, currentUser.ID) {
			return echo.NewHTTPError(http.StatusForbidden, "must be an organization member to fork into this owner")
		}
		targetNamespace = org.Login
		targetOrgID = &org.ID
	}

	if existing, existingErr := h.repoSvc.GetByOwnerAndName(c.Request().Context(), targetNamespace, forkName); existingErr == nil && existing != nil {
		return echo.NewHTTPError(http.StatusConflict, "repository already exists")
	}

	forkDescription := sourceRepo.Description
	if req.Description != nil {
		forkDescription = *req.Description
	}

	forkRepo, err := h.repoSvc.Create(c.Request().Context(), services.CreateRepoInput{
		Name:             forkName,
		Description:      forkDescription,
		IsPrivate:        false,
		OwnerID:          targetOwnerID,
		OrgID:            targetOrgID,
		DefaultBranch:    sourceRepo.DefaultBranch,
		ForkedFromRepoID: &sourceRepo.ID,
	})
	if err != nil {
		if errors.Is(err, services.ErrRepoExists) {
			return echo.NewHTTPError(http.StatusConflict, "repository already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create fork")
	}

	sourceRepoPath := h.repoSvc.RepoPath(h.repoSvc.RepoNamespace(sourceRepo), sourceRepo.Name)
	forkRepoPath := h.repoSvc.RepoPath(targetNamespace, forkRepo.Name)
	if err := h.gitSvc.CloneForkRepo(sourceRepoPath, forkRepoPath, sourceRepo.DefaultBranch, req.CopyMainBranchOnly); err != nil {
		h.repoSvc.Delete(c.Request().Context(), forkRepo)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to clone repository")
	}
	_ = h.cache.DeleteByPrefix(c.Request().Context(), "cache:explore:")

	return c.JSON(http.StatusCreated, forkRepo)
}

func (h *RepoHandler) SyncFork(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	forkRepo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if forkRepo.OwnerID != currentUser.ID {
		return echo.NewHTTPError(http.StatusForbidden, "only the fork owner can sync this repository")
	}

	if forkRepo.ForkedFromRepoID == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "repository is not a fork")
	}

	upstreamRepo, err := h.repoSvc.GetByID(c.Request().Context(), *forkRepo.ForkedFromRepoID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "upstream repository not found")
	}

	if upstreamRepo.IsPrivate {
		return echo.NewHTTPError(http.StatusForbidden, "cannot sync from a private upstream repository")
	}

	forkRepoPath := h.repoSvc.RepoPath(h.repoSvc.RepoNamespace(forkRepo), forkRepo.Name)
	upstreamRepoPath := h.repoSvc.RepoPath(h.repoSvc.RepoNamespace(upstreamRepo), upstreamRepo.Name)

	result, err := h.gitSvc.SyncForkDefaultBranch(forkRepoPath, upstreamRepoPath, upstreamRepo.DefaultBranch)
	if err != nil {
		if errors.Is(err, services.ErrForkHasLocalChanges) {
			return echo.NewHTTPError(http.StatusConflict, "fork has local changes and cannot be synced automatically")
		}
		if errors.Is(err, services.ErrInvalidGitParam) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid branch")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to sync fork")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":     result.Status,
		"before_sha": result.BeforeSHA,
		"after_sha":  result.AfterSHA,
		"message":    result.Message,
	})
}

func (h *RepoHandler) ListForks(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	if repo.IsPrivate && (currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false)) {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	limit := 100
	if l := strings.TrimSpace(c.QueryParam("limit")); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			if n > 500 {
				n = 500
			}
			limit = n
		}
	}

	forks, err := h.repoSvc.ListForksForRepo(c.Request().Context(), repo.ID, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list forks")
	}

	visible := make([]models.Repository, 0, len(forks))
	for _, f := range forks {
		if f.IsPrivate {
			if currentUser == nil || !h.repoSvc.HasAccess(&f, currentUser.ID, false) {
				continue
			}
		}
		visible = append(visible, f)
	}
	sanitizeReposForPublic(visible)

	return c.JSON(http.StatusOK, map[string]interface{}{"forks": visible})
}

func (h *RepoHandler) Get(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.IsPrivate {
		if currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false) {
			return echo.NewHTTPError(http.StatusNotFound, "repository not found")
		}
	}

	repoPath := h.repoSvc.RepoPath(username, repoName)
	ref := c.QueryParam("ref")
	if ref == "" {
		ref = repo.DefaultBranch
	}

	includeBranches := c.QueryParam("include_branches") != "false"
	includeHead := c.QueryParam("include_head") != "false"
	includeStats := c.QueryParam("include_stats") != "false"
	includeSize := c.QueryParam("include_size") != "false"

	// Run requested fields concurrently so lightweight callers can skip expensive git work.
	var (
		branches    []string
		headCommit  *services.CommitInfo
		stats       *services.RepoStats
		branchCount int
		wg          sync.WaitGroup
	)
	if includeBranches {
		wg.Add(1)
		go func() { defer wg.Done(); branches, _ = h.gitSvc.GetBranches(repoPath) }()
	}
	if includeHead {
		wg.Add(1)
		go func() { defer wg.Done(); headCommit, _ = h.gitSvc.GetHeadCommit(repoPath, ref) }()
	}
	if includeStats {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stats, _ = h.gitSvc.GetStats(repoPath, ref)
			if !includeBranches {
				if b, err := h.gitSvc.GetBranches(repoPath); err == nil {
					branchCount = len(b)
				}
			}
			if t, err := h.gitSvc.GetTags(repoPath); err == nil {
				stats.Tags = len(t)
			}
		}()
	}
	if includeSize {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s, err := h.repoSvc.CalculateRepoSize(repoPath); err == nil {
				repo.Size = s
			}
		}()
	}
	wg.Wait()
	enrichCommitInfoAuthor(c.Request().Context(), h.authSvc, headCommit)
	if stats != nil {
		if includeBranches {
			stats.Branches = len(branches)
		} else {
			stats.Branches = branchCount
		}
	}
	forkCount, _ := h.repoSvc.GetForkCount(c.Request().Context(), repo.ID)
	repo.ForkCount = forkCount
	sanitizeRepoForPublic(repo)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"repo":        repo,
		"branches":    branches,
		"head_commit": headCommit,
		"stats":       stats,
	})
}

// DownloadZip returns a source zip archive for a repository ref.
// GET /repos/:username/:repo/zip?ref=main
func (h *RepoHandler) DownloadZip(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	if repo.IsPrivate && (currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false)) {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	ref := c.QueryParam("ref")
	if ref == "" {
		ref = repo.DefaultBranch
	}

	repoPath := h.repoSvc.RepoPath(h.repoSvc.RepoNamespace(repo), repo.Name)
	archive, err := h.gitSvc.GetArchiveWithPrefix(repoPath, ref, "zip", "")
	if err != nil {
		msg := strings.ToLower(err.Error())
		switch {
		case errors.Is(err, services.ErrEmptyRepository),
			strings.Contains(msg, "not a valid object name"),
			strings.Contains(msg, "unknown revision"),
			strings.Contains(msg, "needed a single revision"),
			strings.Contains(msg, "does not have any commits yet"):
			return echo.NewHTTPError(http.StatusNotFound, "repository has no commits")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate archive")
	}

	filename := fmt.Sprintf("%s-%s.zip", repo.Name, strings.ReplaceAll(ref, "/", "-"))
	c.Response().Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(filename, `"`, `_`)+`"`)
	c.Response().Header().Set("Cache-Control", "no-cache")
	return c.Blob(http.StatusOK, "application/zip", archive)
}

func (h *RepoHandler) Delete(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	// Only owner or admin collaborator can delete
	if repo.OwnerID != currentUser.ID {
		return echo.NewHTTPError(http.StatusForbidden, "only the owner can delete this repository")
	}

	repoPath := h.repoSvc.RepoPath(username, repoName)
	if err := h.repoSvc.Delete(c.Request().Context(), repo); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete repository")
	}
	_ = h.cache.DeleteByPrefix(c.Request().Context(), "cache:explore:")

	_ = h.gitSvc.DeleteRepo(repoPath)
	return c.NoContent(http.StatusNoContent)
}

func (h *RepoHandler) Update(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	var req struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		IsPrivate     bool   `json:"is_private"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Handle rename
	if req.Name != "" && req.Name != repo.Name {
		if !usernameRe.MatchString(req.Name) || len(req.Name) < 1 || len(req.Name) > 100 {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid repository name")
		}
		// Check name is available
		existing, _ := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, req.Name)
		if existing != nil && existing.ID != repo.ID {
			return echo.NewHTTPError(http.StatusConflict, "repository name already exists")
		}
		// Rename git directory
		oldPath := h.repoSvc.RepoPath(username, repo.Name)
		newPath := h.repoSvc.RepoPath(username, req.Name)
		if err := h.gitSvc.RenameRepo(oldPath, newPath); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to rename repository")
		}
	}

	updates := map[string]interface{}{}
	if req.Name != "" && req.Name != repo.Name {
		updates["name"] = req.Name
	}
	if req.Description != "" || repo.Description != "" {
		updates["description"] = req.Description
	}
	updates["is_private"] = req.IsPrivate
	if req.DefaultBranch != "" {
		updates["default_branch"] = req.DefaultBranch
	}

	if err := h.repoSvc.Update(c.Request().Context(), repo, updates); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update repository")
	}
	_ = h.cache.DeleteByPrefix(c.Request().Context(), "cache:explore:")

	updated, _ := h.repoSvc.GetByID(c.Request().Context(), repo.ID)
	return c.JSON(http.StatusOK, updated)
}

func (h *RepoHandler) SetVisibility(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	if repo.OwnerID != currentUser.ID {
		return echo.NewHTTPError(http.StatusForbidden, "only the owner can change repository visibility")
	}

	var req struct {
		Private bool `json:"private"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if err := h.repoSvc.Update(c.Request().Context(), repo, map[string]interface{}{"is_private": req.Private}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update visibility")
	}
	_ = h.cache.DeleteByPrefix(c.Request().Context(), "cache:explore:")

	updated, _ := h.repoSvc.GetByID(c.Request().Context(), repo.ID)
	return c.JSON(http.StatusOK, updated)
}

func (h *RepoHandler) Archive(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	if repo.OwnerID != currentUser.ID {
		return echo.NewHTTPError(http.StatusForbidden, "only the owner can archive this repository")
	}
	if repo.IsArchived {
		return c.JSON(http.StatusOK, repo)
	}

	now := time.Now().UTC()
	updates := map[string]interface{}{
		"is_archived": true,
		"archived_at": now,
	}
	if err := h.repoSvc.Update(c.Request().Context(), repo, updates); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to archive repository")
	}
	_ = h.cache.DeleteByPrefix(c.Request().Context(), "cache:explore:")

	updated, _ := h.repoSvc.GetByID(c.Request().Context(), repo.ID)
	return c.JSON(http.StatusOK, updated)
}

func (h *RepoHandler) Unarchive(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	if repo.OwnerID != currentUser.ID {
		return echo.NewHTTPError(http.StatusForbidden, "only the owner can unarchive this repository")
	}
	if !repo.IsArchived {
		return c.JSON(http.StatusOK, repo)
	}

	updates := map[string]interface{}{
		"is_archived": false,
		"archived_at": nil,
	}
	if err := h.repoSvc.Update(c.Request().Context(), repo, updates); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to unarchive repository")
	}
	_ = h.cache.DeleteByPrefix(c.Request().Context(), "cache:explore:")

	updated, _ := h.repoSvc.GetByID(c.Request().Context(), repo.ID)
	return c.JSON(http.StatusOK, updated)
}

func (h *RepoHandler) ListTree(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.IsPrivate && (currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false)) {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	ref := c.QueryParam("ref")
	if ref == "" {
		ref = repo.DefaultBranch
	}
	path := c.QueryParam("path")
	includeMeta := c.QueryParam("include_meta") != "false"
	includeHead := c.QueryParam("include_head") != "false"

	repoPath := h.repoSvc.RepoPath(username, repoName)
	entries, err := h.gitSvc.ListTree(repoPath, ref, path, includeMeta)
	if err != nil {
		if errors.Is(err, services.ErrEmptyRepository) {
			return c.JSON(http.StatusOK, map[string]interface{}{"files": []interface{}{}, "empty": true})
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list tree")
	}

	var headCommit *services.CommitInfo
	if includeHead {
		headCommit, _ = h.gitSvc.GetHeadCommit(repoPath, ref)
		enrichCommitInfoAuthor(c.Request().Context(), h.authSvc, headCommit)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"files":       entries,
		"head_commit": headCommit,
	})
}

func (h *RepoHandler) GetBlob(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.IsPrivate && (currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false)) {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	ref := c.QueryParam("ref")
	if ref == "" {
		ref = repo.DefaultBranch
	}
	filePath := c.QueryParam("path")
	if filePath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path is required")
	}

	repoPath := h.repoSvc.RepoPath(username, repoName)
	content, err := h.gitSvc.GetBlob(repoPath, ref, filePath)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "file not found")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"content": string(content),
		"path":    filePath,
		"ref":     ref,
		"size":    len(content),
	})
}

func (h *RepoHandler) GetRaw(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.IsPrivate && (currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false)) {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	ref := c.QueryParam("ref")
	if ref == "" {
		ref = repo.DefaultBranch
	}
	filePath := c.QueryParam("path")
	if filePath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path is required")
	}

	repoPath := h.repoSvc.RepoPath(username, repoName)
	content, err := h.gitSvc.GetBlob(repoPath, ref, filePath)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "file not found")
	}

	contentType := mime.TypeByExtension(filepath.Ext(filePath))
	if contentType == "" {
		contentType = http.DetectContentType(content)
	}

	// Prevent stored XSS: never serve user-controlled content as a renderable
	// document type. HTML, SVG and XML must be sent as plain text.
	baseMIME := strings.SplitN(contentType, ";", 2)[0]
	unsafeMIME := map[string]bool{
		"text/html":             true,
		"image/svg+xml":         true,
		"application/xhtml+xml": true,
		"text/xml":              true,
		"application/xml":       true,
	}
	if unsafeMIME[baseMIME] {
		contentType = "text/plain; charset=utf-8"
	}

	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	c.Response().Header().Set("Content-Security-Policy", "sandbox")
	return c.Blob(http.StatusOK, contentType, content)
}

func (h *RepoHandler) GetCommits(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.IsPrivate && (currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false)) {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	ref := c.QueryParam("ref")
	if ref == "" {
		ref = repo.DefaultBranch
	}

	limit := 30
	if l := c.QueryParam("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	offset := 0
	if o := c.QueryParam("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	filters := services.CommitFilters{
		Author: strings.TrimSpace(c.QueryParam("author")),
		Query:  strings.TrimSpace(c.QueryParam("q")),
	}

	var parseErr error
	filters.Since, parseErr = parseCommitFilterDate(c.QueryParam("since"), false)
	if parseErr != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid since date")
	}
	filters.Until, parseErr = parseCommitFilterDate(c.QueryParam("until"), true)
	if parseErr != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid until date")
	}
	if filters.Since != "" && filters.Until != "" && filters.Since > filters.Until {
		return echo.NewHTTPError(http.StatusBadRequest, "since date must be before until date")
	}

	repoPath := h.repoSvc.RepoPath(username, repoName)

	// Run page fetch and total count concurrently ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â count can be slow on large repos.
	var (
		commits   []*services.CommitInfo
		hasMore   bool
		commitErr error
		total     int
		countErr  error
		wg        sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		commits, hasMore, commitErr = h.gitSvc.GetCommitsFiltered(repoPath, ref, limit, offset, filters)
	}()
	go func() {
		defer wg.Done()
		total, countErr = h.gitSvc.CountCommitsFiltered(repoPath, ref, filters)
	}()
	wg.Wait()

	if commitErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get commits")
	}
	if countErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get commits")
	}
	enrichCommitAuthors(c.Request().Context(), h.authSvc, commits)
	totalPages := 0
	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"commits":     commits,
		"ref":         ref,
		"author":      filters.Author,
		"q":           filters.Query,
		"since":       c.QueryParam("since"),
		"until":       c.QueryParam("until"),
		"limit":       limit,
		"offset":      offset,
		"has_more":    hasMore,
		"total":       total,
		"total_pages": totalPages,
	})
}

func parseCommitFilterDate(raw string, endOfDay bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}

	if t, err := time.Parse("2006-01-02", raw); err == nil {
		if endOfDay {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		return t.UTC().Format(time.RFC3339), nil
	}

	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return "", err
	}
	if endOfDay {
		return parsed.UTC().Format(time.RFC3339), nil
	}
	return parsed.UTC().Format(time.RFC3339), nil
}

func (h *RepoHandler) GetCommit(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")
	sha := c.Param("sha")

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.IsPrivate && (currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false)) {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	repoPath := h.repoSvc.RepoPath(username, repoName)
	if v := strings.ToLower(c.QueryParam("meta")); v == "1" || v == "true" {
		info, err := h.gitSvc.GetCommitInfo(repoPath, sha)
		if err != nil {
			log.Printf("GetCommitInfo error: %v", err)
			return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("commit not found: %v", err))
		}
		enrichCommitInfoAuthor(c.Request().Context(), h.authSvc, info)
		return c.JSON(http.StatusOK, info)
	}

	log.Printf("GetCommitDetail: repoPath=%s, sha=%s", repoPath, sha)
	detail, err := h.gitSvc.GetCommitDetail(repoPath, sha)
	if err != nil {
		log.Printf("GetCommitDetail error: %v", err)
		return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("commit not found: %v", err))
	}
	enrichCommitDetailAuthor(c.Request().Context(), h.authSvc, detail)

	return c.JSON(http.StatusOK, detail)
}

func (h *RepoHandler) GetCommitDiffs(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")
	sha := c.Param("sha")

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.IsPrivate && (currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false)) {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	limit := 25
	if l := c.QueryParam("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			if n > 100 {
				n = 100
			}
			limit = n
		}
	}

	offset := 0
	if o := c.QueryParam("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	repoPath := h.repoSvc.RepoPath(username, repoName)
	diffs, total, hasMore, err := h.gitSvc.GetCommitDiffs(repoPath, sha, limit, offset)
	if err != nil {
		log.Printf("GetCommitDiffs error: %v", err)
		return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("commit not found: %v", err))
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"sha":      sha,
		"diffs":    diffs,
		"limit":    limit,
		"offset":   offset,
		"has_more": hasMore,
		"total":    total,
	})
}

func (h *RepoHandler) GetBranches(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.IsPrivate && (currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false)) {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	repoPath := h.repoSvc.RepoPath(h.repoSvc.RepoNamespace(repo), repo.Name)
	branches, err := h.gitSvc.GetBranches(repoPath)
	if err != nil {
		log.Printf("GetBranches error for %s/%s (%s): %v", username, repoName, repoPath, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get branches")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"branches": branches})
}

func (h *RepoHandler) CreateBranch(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	var req struct {
		Name    string `json:"name"`
		FromRef string `json:"from_ref"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	req.Name = strings.TrimSpace(req.Name)
	req.FromRef = strings.TrimSpace(req.FromRef)
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "branch name is required")
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	fromRef := req.FromRef
	if fromRef == "" {
		fromRef = repo.DefaultBranch
	}
	if fromRef == "" {
		fromRef = "main"
	}

	repoPath := h.repoSvc.RepoPath(username, repoName)
	// Some repos may exist with HEAD pointing at default branch but without any branch refs.
	// Bootstrap the source branch first so branch creation from "main" works reliably.
	if branches, listErr := h.gitSvc.GetBranches(repoPath); listErr == nil && len(branches) == 0 {
		if initErr := h.gitSvc.InitializeDefaultBranch(repoPath, fromRef, currentUser.Username, currentUser.Email, ""); initErr != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to bootstrap default branch: %v", initErr))
		}
		if req.Name == fromRef {
			return c.JSON(http.StatusCreated, map[string]interface{}{"name": req.Name, "from_ref": fromRef})
		}
	}
	if err := h.gitSvc.CreateBranch(repoPath, req.Name, fromRef); err != nil {
		// Self-heal repos that have no refs yet (e.g. created during an initialization regression):
		// bootstrap the source branch, then retry branch creation.
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			if branches, listErr := h.gitSvc.GetBranches(repoPath); listErr == nil && len(branches) == 0 {
				if initErr := h.gitSvc.InitializeDefaultBranch(repoPath, fromRef, currentUser.Username, currentUser.Email, ""); initErr == nil {
					if req.Name == fromRef {
						return c.JSON(http.StatusCreated, map[string]interface{}{"name": req.Name, "from_ref": fromRef})
					}
					if retryErr := h.gitSvc.CreateBranch(repoPath, req.Name, fromRef); retryErr == nil {
						return c.JSON(http.StatusCreated, map[string]interface{}{"name": req.Name, "from_ref": fromRef})
					}
				}
			}
		}
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to create branch: %v", err))
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{"name": req.Name, "from_ref": fromRef})
}

func (h *RepoHandler) DeleteBranch(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	var req struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "branch name is required")
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}
	if req.Name == repo.DefaultBranch {
		return echo.NewHTTPError(http.StatusBadRequest, "cannot delete the default branch")
	}

	repoPath := h.repoSvc.RepoPath(username, repoName)
	if err := h.gitSvc.DeleteBranch(repoPath, req.Name); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to delete branch: %v", err))
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *RepoHandler) GetTags(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.IsPrivate && (currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false)) {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	repoPath := h.repoSvc.RepoPath(username, repoName)
	tags, err := h.gitSvc.GetTags(repoPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get tags")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"tags": tags})
}

func (h *RepoHandler) CreateTag(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	var req struct {
		Name      string `json:"name"`
		TargetRef string `json:"target_ref"`
		Message   string `json:"message"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "tag name is required")
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	targetRef := req.TargetRef
	if targetRef == "" {
		targetRef = repo.DefaultBranch
	}

	repoPath := h.repoSvc.RepoPath(username, repoName)
	if err := h.gitSvc.CreateTag(repoPath, req.Name, targetRef, req.Message); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to create tag: %v", err))
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{"name": req.Name, "target_ref": targetRef})
}

func (h *RepoHandler) DeleteTag(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	var req struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "tag name is required")
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	repoPath := h.repoSvc.RepoPath(username, repoName)
	if err := h.gitSvc.DeleteTag(repoPath, req.Name); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to delete tag: %v", err))
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *RepoHandler) AddCollaborator(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.OwnerID != currentUser.ID {
		return echo.NewHTTPError(http.StatusForbidden, "only the owner can manage collaborators")
	}

	var req struct {
		UserID     string `json:"user_id"`
		Permission string `json:"permission"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Permission != "read" && req.Permission != "write" && req.Permission != "admin" {
		return echo.NewHTTPError(http.StatusBadRequest, "permission must be read, write, or admin")
	}

	collab, err := h.repoSvc.AddCollaborator(c.Request().Context(), repo.ID, req.UserID, req.Permission)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to add collaborator")
	}

	return c.JSON(http.StatusCreated, collab)
}

func (h *RepoHandler) RemoveCollaborator(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.OwnerID != currentUser.ID {
		return echo.NewHTTPError(http.StatusForbidden, "only the owner can manage collaborators")
	}

	userIDStr := c.Param("userID")
	if userIDStr == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user ID")
	}

	if err := h.repoSvc.RemoveCollaborator(c.Request().Context(), repo.ID, userIDStr); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to remove collaborator")
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *RepoHandler) ListCollaborators(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if !h.repoSvc.HasAccess(repo, currentUser.ID, false) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	collabs, err := h.repoSvc.ListCollaborators(c.Request().Context(), repo.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list collaborators")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"collaborators": collabs})
}

func (h *RepoHandler) Explore(c echo.Context) error {
	limit := 20
	offset := 0

	if l := c.QueryParam("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}
	if o := c.QueryParam("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	key := fmt.Sprintf("cache:explore:%d:%d", limit, offset)
	body, err := h.cache.RememberJSON(c.Request().Context(), key, 45*time.Second, func() (interface{}, error) {
		repos, count, err := h.repoSvc.ListPublic(c.Request().Context(), limit, offset)
		if err != nil {
			return nil, err
		}
		sanitizeReposForPublic(repos)
		return map[string]interface{}{
			"repos":  repos,
			"total":  count,
			"limit":  limit,
			"offset": offset,
		}, nil
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list repositories")
	}
	return c.Blob(http.StatusOK, echo.MIMEApplicationJSONCharsetUTF8, body)
}

func (h *RepoHandler) Star(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.IsPrivate {
		if currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false) {
			return echo.NewHTTPError(http.StatusNotFound, "repository not found")
		}
	}

	star, err := h.repoSvc.Star(c.Request().Context(), currentUser.ID, repo.ID)
	if err != nil {
		if errors.Is(err, services.ErrAlreadyStarred) {
			return echo.NewHTTPError(http.StatusConflict, "already starred")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to star repository")
	}
	_ = h.cache.DeleteByPrefix(c.Request().Context(), "cache:explore:")
	_ = h.cache.DeleteByPrefix(c.Request().Context(), fmt.Sprintf("cache:user:%d:", repo.OwnerID))

	return c.JSON(http.StatusCreated, star)
}

func (h *RepoHandler) Unstar(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if err := h.repoSvc.Unstar(c.Request().Context(), currentUser.ID, repo.ID); err != nil {
		if errors.Is(err, services.ErrNotStarred) {
			return echo.NewHTTPError(http.StatusNotFound, "not starred")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to unstar repository")
	}
	_ = h.cache.DeleteByPrefix(c.Request().Context(), "cache:explore:")
	_ = h.cache.DeleteByPrefix(c.Request().Context(), fmt.Sprintf("cache:user:%d:", repo.OwnerID))

	return c.NoContent(http.StatusNoContent)
}

func (h *RepoHandler) GetStarringStatus(c echo.Context) error {
	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}
	username := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	if repo.IsPrivate && (currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false)) {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	starred := false
	if currentUser != nil {
		starred, _ = h.repoSvc.IsStarred(c.Request().Context(), currentUser.ID, repo.ID)
	}

	starCount, _ := h.repoSvc.GetStarCount(c.Request().Context(), repo.ID)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"starred": starred,
		"count":   starCount,
	})
}

func (h *RepoHandler) GetStarHistory(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.IsPrivate && (currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false)) {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	stars, err := h.repoSvc.ListStarsForRepo(c.Request().Context(), repo.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list stars")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"stars": stars,
	})
}

func (h *RepoHandler) GetStarredRepos(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	stars, err := h.repoSvc.ListStarred(c.Request().Context(), currentUser.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list starred repositories")
	}
	sanitizeStarsForPublic(stars)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"stars": stars,
	})
}

func (h *RepoHandler) GetUserStarredRepos(c echo.Context) error {
	username := c.Param("username")
	user, err := h.authSvc.GetUserByUsername(c.Request().Context(), username)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	stars, err := h.repoSvc.ListStarred(c.Request().Context(), user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list starred repositories")
	}

	public := make([]models.Star, 0, len(stars))
	for _, s := range stars {
		if !s.Repo.IsPrivate {
			public = append(public, s)
		}
	}
	sanitizeStarsForPublic(public)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"stars": public,
	})
}

// Compare returns the commits, files and mergeability between two refs in a repo.
// GET /repos/:username/:repo/compare?base=main&head=feature
func (h *RepoHandler) Compare(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")
	base := c.QueryParam("base")
	head := c.QueryParam("head")
	headRepoID := strings.TrimSpace(c.QueryParam("head_repo_id"))

	if base == "" || head == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "base and head query params required")
	}

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.IsPrivate && (currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false)) {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	headRepo := repo
	if headRepoID != "" {
		headCandidate, err := h.repoSvc.GetByID(c.Request().Context(), headRepoID)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "head repository not found")
		}
		if headCandidate.IsPrivate && (currentUser == nil || !h.repoSvc.HasAccess(headCandidate, currentUser.ID, false)) {
			return echo.NewHTTPError(http.StatusNotFound, "head repository not found")
		}

		if headCandidate.ID != repo.ID {
			related := (headCandidate.ForkedFromRepoID != nil && *headCandidate.ForkedFromRepoID == repo.ID) ||
				(repo.ForkedFromRepoID != nil && *repo.ForkedFromRepoID == headCandidate.ID)
			if !related {
				return echo.NewHTTPError(http.StatusBadRequest, "head repository must be this repository or a directly related fork")
			}
		}
		headRepo = headCandidate
	}

	baseRepoPath := h.repoSvc.RepoPath(h.repoSvc.RepoNamespace(repo), repo.Name)
	headRepoPath := h.repoSvc.RepoPath(h.repoSvc.RepoNamespace(headRepo), headRepo.Name)

	commits, err := h.gitSvc.GetPRCommitsBetweenRepos(baseRepoPath, headRepoPath, base, head, "")
	if err != nil {
		commits = nil
	}
	enrichCommitAuthors(c.Request().Context(), h.authSvc, commits)

	files, err := h.gitSvc.GetPRDiffBetweenRepos(baseRepoPath, headRepoPath, base, head, "")
	if err != nil {
		files = nil
	}

	mergeable := h.gitSvc.IsMergeable(baseRepoPath, headRepoPath, base, head)

	// Count unique contributors
	contributors := map[string]struct{}{}
	for _, c := range commits {
		contributors[c.Author.Name] = struct{}{}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"commits":      commits,
		"files":        files,
		"mergeable":    mergeable,
		"contributors": len(contributors),
	})
}

// GetLanguages returns the language breakdown for a repository.
// GET /repos/:username/:repo/languages
func (h *RepoHandler) GetLanguages(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	if repo.IsPrivate && (currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false)) {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	repoPath := h.repoSvc.RepoPath(username, repoName)
	breakdown := h.gitSvc.GetLanguageBreakdown(repoPath, repo.DefaultBranch)
	if breakdown == nil {
		breakdown = []services.LanguageStat{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"languages": breakdown,
	})
}

// RequestStorage handles storage limit increase requests for public repositories
func (h *RepoHandler) RequestStorage(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	// Only repo owner can request storage
	if repo.OwnerID != currentUser.ID {
		return echo.NewHTTPError(http.StatusForbidden, "only repository owner can request storage")
	}

	// Only public repos can request storage
	if repo.IsPrivate {
		return echo.NewHTTPError(http.StatusBadRequest, "storage requests are only available for public repositories")
	}

	type storageRequest struct {
		Message             string `json:"message"`
		RequestedLimitBytes int64  `json:"requested_limit_bytes"`
	}
	var req storageRequest
	if err := c.Bind(&req); err != nil && !errors.Is(err, io.EOF) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.RequestedLimitBytes < 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "requested_limit_bytes must be non-negative")
	}

	_, err = h.repoSvc.CreateStorageRequest(c.Request().Context(), repo.ID, currentUser.ID, req.RequestedLimitBytes, req.Message)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to submit storage request")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":  "requested",
		"message": "Your storage request has been submitted. Our team will review it and contact you soon.",
	})
}

// UpdateBlob creates or updates a single file in a repository and commits the change.
// PUT /repos/:username/:repo/blob
func (h *RepoHandler) UpdateBlob(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	// Require write access
	if !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "you do not have write access to this repository")
	}

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Message string `json:"message"`
		Branch  string `json:"branch"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Path == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path is required")
	}
	// Enforce a reasonable file size cap (5 MB)
	const maxFileSize = 5 * 1024 * 1024
	if len(req.Content) > maxFileSize {
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "file content exceeds 5 MB limit")
	}

	branch := req.Branch
	if branch == "" {
		branch = repo.DefaultBranch
	}

	repoPath := h.repoSvc.RepoPath(h.repoSvc.RepoNamespace(repo), repoName)

	sha, err := h.gitSvc.CommitFile(repoPath, branch, req.Path, req.Content, req.Message, currentUser.Username, currentUser.Email)
	if err != nil {
		if err.Error() == "no changes to commit" {
			return echo.NewHTTPError(http.StatusUnprocessableEntity, "no changes to commit")
		}
		log.Printf("CommitFile error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to commit file")
	}
	_ = h.cache.DeleteByPrefix(c.Request().Context(), "cache:explore:")
	_ = h.cache.DeleteByPrefix(c.Request().Context(), fmt.Sprintf("cache:user:%d:", repo.OwnerID))

	return c.JSON(http.StatusOK, map[string]interface{}{
		"sha":     sha,
		"path":    req.Path,
		"branch":  branch,
		"message": "File committed successfully",
	})
}
