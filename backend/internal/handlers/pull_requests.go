package handlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

type PRHandler struct {
	prSvc      *services.PRService
	repoSvc    *services.RepoService
	gitSvc     *services.GitService
	authSvc    *services.AuthService
	modSvc     *services.ModerationService
	webhookSvc *services.WebhookService
}

func NewPRHandler(prSvc *services.PRService, repoSvc *services.RepoService, gitSvc *services.GitService, authSvc *services.AuthService) *PRHandler {
	return &PRHandler{prSvc: prSvc, repoSvc: repoSvc, gitSvc: gitSvc, authSvc: authSvc}
}

func (h *PRHandler) SetModerationService(modSvc *services.ModerationService) {
	h.modSvc = modSvc
}

func (h *PRHandler) SetWebhookService(webhookSvc *services.WebhookService, repoSvc *services.RepoService) {
	h.webhookSvc = webhookSvc
	h.repoSvc = repoSvc
}

// parsePRNumber parses the :number route param.
func parsePRNumber(c echo.Context) (uint, error) {
	n, err := strconv.ParseUint(c.Param("number"), 10, 64)
	if err != nil {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "invalid pull request number")
	}
	return uint(n), nil
}

func (h *PRHandler) Create(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	if repo.IsPrivate && !h.repoSvc.HasAccess(repo, currentUser.ID, false) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	var req struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		HeadRef     string  `json:"head_ref"`
		BaseRef     string  `json:"base_ref"`
		HeadSHA     string  `json:"head_sha"`
		IsDraft     bool    `json:"is_draft"`
		HeadRepoID  *string `json:"head_repo_id,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Title == "" || req.HeadRef == "" || req.BaseRef == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title, head_ref, and base_ref are required")
	}

	if req.HeadRepoID != nil && *req.HeadRepoID != repo.ID {
		headRepo, err := h.repoSvc.GetByID(c.Request().Context(), *req.HeadRepoID)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "head repository not found")
		}
		if headRepo.IsPrivate && !h.repoSvc.HasAccess(headRepo, currentUser.ID, false) {
			return echo.NewHTTPError(http.StatusForbidden, "access denied to head repository")
		}
	}

	if h.modSvc != nil {
		if err := h.modSvc.CheckAllowed(c.Request().Context(), services.CheckInput{
			RepoID:      repo.ID,
			ActorID:     currentUser.ID,
			ActorJoined: currentUser.CreatedAt,
			ContextType: "prs",
			Content:     []string{req.Title, req.Description},
		}); err != nil {
			return ModerationError(err)
		}
	}

	pr, err := h.prSvc.Create(c.Request().Context(), services.CreatePRInput{
		Title:       req.Title,
		Description: req.Description,
		HeadRef:     req.HeadRef,
		BaseRef:     req.BaseRef,
		HeadSHA:     req.HeadSHA,
		IsDraft:     req.IsDraft,
		RepoID:      repo.ID,
		HeadRepoID:  req.HeadRepoID,
		AuthorID:    currentUser.ID,
	})
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidBaseBranch):
			return echo.NewHTTPError(http.StatusBadRequest, "base branch does not exist")
		case errors.Is(err, services.ErrInvalidHeadBranch):
			return echo.NewHTTPError(http.StatusBadRequest, "head branch does not exist")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create pull request")
	}

	if h.webhookSvc != nil {
		h.webhookSvc.Deliver(c.Request().Context(), repo.ID, "pull_request", map[string]interface{}{
			"action":       "opened",
			"pull_request": pr,
			"repository": map[string]interface{}{
				"id":        repo.ID,
				"name":      repoName,
				"full_name": username + "/" + repoName,
			},
			"sender": map[string]interface{}{"login": currentUser.Username},
		})
	}

	return c.JSON(http.StatusCreated, pr)
}

func (h *PRHandler) List(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")
	status := c.QueryParam("status")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
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

	prs, err := h.prSvc.GetByRepo(c.Request().Context(), repo.ID, status)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list pull requests")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"pull_requests": prs})
}

func (h *PRHandler) Get(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	number, err := parsePRNumber(c)
	if err != nil {
		return err
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
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

	pr, err := h.prSvc.GetByNumber(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "pull request not found")
	}

	mergeable := false
	if pr.Status == models.PRStatusOpen && !pr.IsDraft {
		mergeable = h.prSvc.IsMergeable(c.Request().Context(), pr.ID)
	}

	repoPath := h.repoSvc.RepoPath(username, repoName)
	baseCommit, _ := h.gitSvc.GetHeadCommit(repoPath, pr.BaseRef)
	headRepoPath := repoPath
	if pr.HeadRepoID != nil && *pr.HeadRepoID != pr.RepoID {
		if headRepo, headErr := h.repoSvc.GetByID(c.Request().Context(), *pr.HeadRepoID); headErr == nil {
			headRepoPath = h.repoSvc.RepoPath(h.repoSvc.RepoNamespace(headRepo), headRepo.Name)
		}
	}
	headCommit, _ := h.gitSvc.GetHeadCommit(headRepoPath, pr.HeadRef)
	enrichCommitInfoAuthor(c.Request().Context(), h.authSvc, baseCommit)
	enrichCommitInfoAuthor(c.Request().Context(), h.authSvc, headCommit)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"pull_request": pr,
		"base_commit":  baseCommit,
		"head_commit":  headCommit,
		"mergeable":    mergeable,
	})
}

func (h *PRHandler) Update(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	number, err := parsePRNumber(c)
	if err != nil {
		return err
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	pr, err := h.prSvc.GetByNumber(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "pull request not found")
	}

	if pr.AuthorID != currentUser.ID && repo.OwnerID != currentUser.ID {
		return echo.NewHTTPError(http.StatusForbidden, "only the author or repository owner can update this pull request")
	}

	var body struct {
		AssigneeID    *string   `json:"assignee_id"`
		ClearAssignee bool      `json:"clear_assignee"`
		LabelIDs      *[]string `json:"label_ids"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	updates := map[string]interface{}{}
	if body.ClearAssignee {
		updates["assignee_id"] = nil
	} else if body.AssigneeID != nil {
		updates["assignee_id"] = body.AssigneeID
	}

	if len(updates) > 0 {
		if err := h.prSvc.Update(c.Request().Context(), pr, updates); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to update pull request")
		}
	}

	if body.LabelIDs != nil {
		if err := h.prSvc.SetLabels(c.Request().Context(), pr, *body.LabelIDs); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to update labels")
		}
	}

	updated, err := h.prSvc.GetByNumber(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to reload pull request")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"pull_request": updated})
}

func (h *PRHandler) Close(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	number, err := parsePRNumber(c)
	if err != nil {
		return err
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	pr, err := h.prSvc.GetByNumber(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "pull request not found")
	}

	if pr.AuthorID != currentUser.ID && repo.OwnerID != currentUser.ID {
		return echo.NewHTTPError(http.StatusForbidden, "only the author or repository owner can close this pull request")
	}

	if err := h.prSvc.Close(c.Request().Context(), pr.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to close pull request")
	}

	if h.webhookSvc != nil {
		h.webhookSvc.Deliver(c.Request().Context(), repo.ID, "pull_request", map[string]interface{}{
			"action":       "closed",
			"pull_request": pr,
			"repository": map[string]interface{}{
				"id":        repo.ID,
				"name":      repoName,
				"full_name": username + "/" + repoName,
			},
			"sender": map[string]interface{}{"login": currentUser.Username},
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *PRHandler) Reopen(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	number, err := parsePRNumber(c)
	if err != nil {
		return err
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	pr, err := h.prSvc.GetByNumber(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "pull request not found")
	}

	if pr.AuthorID != currentUser.ID && repo.OwnerID != currentUser.ID {
		return echo.NewHTTPError(http.StatusForbidden, "only the author or repository owner can reopen this pull request")
	}

	if err := h.prSvc.Reopen(c.Request().Context(), pr.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to reopen pull request")
	}

	updated, _ := h.prSvc.GetByID(c.Request().Context(), pr.ID)
	return c.JSON(http.StatusOK, updated)
}

func (h *PRHandler) Merge(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	number, err := parsePRNumber(c)
	if err != nil {
		return err
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	isOwner := repo.OwnerID == currentUser.ID
	isCollaborator := h.repoSvc.HasAccess(repo, currentUser.ID, true)
	if !isOwner && !isCollaborator {
		return echo.NewHTTPError(http.StatusForbidden, "only maintainers can merge pull requests")
	}

	pr, err := h.prSvc.GetByNumber(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "pull request not found")
	}

	var req struct {
		MergeMethod string `json:"merge_method"` // "merge" | "squash" | "rebase"
		CommitTitle string `json:"commit_title"`
	}
	_ = c.Bind(&req)

	merged, err := h.prSvc.Merge(c.Request().Context(), services.MergePRInput{
		PRID:        pr.ID,
		Method:      req.MergeMethod,
		CommitTitle: req.CommitTitle,
		MergerID:    currentUser.ID,
		MergerName:  currentUser.Username,
		MergerEmail: currentUser.Email,
	})
	if err != nil {
		switch {
		case errors.Is(err, services.ErrPRNotOpen):
			return echo.NewHTTPError(http.StatusBadRequest, "pull request is not open")
		case errors.Is(err, services.ErrPRIsDraft):
			return echo.NewHTTPError(http.StatusBadRequest, "cannot merge a draft pull request")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if h.webhookSvc != nil {
		h.webhookSvc.Deliver(c.Request().Context(), repo.ID, "pull_request", map[string]interface{}{
			"action":       "closed",
			"merged":       true,
			"pull_request": merged,
			"repository": map[string]interface{}{
				"id":        repo.ID,
				"name":      repoName,
				"full_name": username + "/" + repoName,
			},
			"sender": map[string]interface{}{"login": currentUser.Username},
		})
	}

	return c.JSON(http.StatusOK, merged)
}

func (h *PRHandler) GetCommits(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	number, err := parsePRNumber(c)
	if err != nil {
		return err
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
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

	pr, err := h.prSvc.GetByNumber(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "pull request not found")
	}
	log.Printf("DEBUG: PR=%d BaseRef=%q HeadRef=%q", pr.ID, pr.BaseRef, pr.HeadRef)

	commits, err := h.prSvc.GetCommits(c.Request().Context(), pr.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get commits")
	}
	enrichCommitAuthors(c.Request().Context(), h.authSvc, commits)

	return c.JSON(http.StatusOK, map[string]interface{}{"commits": commits})
}

func (h *PRHandler) GetFiles(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	number, err := parsePRNumber(c)
	if err != nil {
		return err
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
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

	pr, err := h.prSvc.GetByNumber(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "pull request not found")
	}
	log.Printf("DEBUG GetFiles: PR=%d BaseRef=%q HeadRef=%q", pr.ID, pr.BaseRef, pr.HeadRef)

	diffs, err := h.prSvc.GetDiff(c.Request().Context(), pr.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get diff")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"files": diffs})
}

func (h *PRHandler) ListComments(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	number, err := parsePRNumber(c)
	if err != nil {
		return err
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
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

	pr, err := h.prSvc.GetByNumber(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "pull request not found")
	}

	comments, err := h.prSvc.ListComments(c.Request().Context(), pr.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list comments")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"comments": comments})
}

func (h *PRHandler) CreateComment(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	number, err := parsePRNumber(c)
	if err != nil {
		return err
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	if repo.IsPrivate && !h.repoSvc.HasAccess(repo, currentUser.ID, false) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	pr, err := h.prSvc.GetByNumber(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "pull request not found")
	}

	var req struct {
		Body string `json:"body"`
	}
	if err := c.Bind(&req); err != nil || req.Body == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "body is required")
	}

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

	comment, err := h.prSvc.AddComment(c.Request().Context(), pr.ID, currentUser.ID, req.Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create comment")
	}

	return c.JSON(http.StatusCreated, comment)
}

func (h *PRHandler) UpdateComment(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	commentIDStr := c.Param("commentID")
	if commentIDStr == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid comment ID")
	}

	var req struct {
		Body string `json:"body"`
	}
	if err := c.Bind(&req); err != nil || req.Body == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "body is required")
	}

	comment, err := h.prSvc.UpdateComment(c.Request().Context(), commentIDStr, currentUser.ID, req.Body)
	if err != nil {
		if errors.Is(err, services.ErrPRCommentNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "comment not found")
		}
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	}

	return c.JSON(http.StatusOK, comment)
}

func (h *PRHandler) DeleteComment(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	commentIDStr := c.Param("commentID")
	if commentIDStr == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid comment ID")
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	isOwner := repo.OwnerID == currentUser.ID
	if err := h.prSvc.DeleteComment(c.Request().Context(), commentIDStr, currentUser.ID, isOwner); err != nil {
		if errors.Is(err, services.ErrPRCommentNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "comment not found")
		}
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *PRHandler) ListReviews(c echo.Context) error {
	username := c.Param("username")
	repoName := c.Param("repo")

	number, err := parsePRNumber(c)
	if err != nil {
		return err
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
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

	pr, err := h.prSvc.GetByNumber(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "pull request not found")
	}

	reviews, err := h.prSvc.ListReviews(c.Request().Context(), pr.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list reviews")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"reviews": reviews})
}

func (h *PRHandler) CreateReview(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	username := c.Param("username")
	repoName := c.Param("repo")

	number, err := parsePRNumber(c)
	if err != nil {
		return err
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	if !h.repoSvc.HasAccess(repo, currentUser.ID, false) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	pr, err := h.prSvc.GetByNumber(c.Request().Context(), repo.ID, number)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "pull request not found")
	}

	var req struct {
		State     string `json:"state"` // APPROVED | CHANGES_REQUESTED | COMMENTED
		Body      string `json:"body"`
		CommitSHA string `json:"commit_sha"`
		Comments  []struct {
			Path      string `json:"path"`
			Line      int    `json:"line"`
			Side      string `json:"side"`
			Body      string `json:"body"`
			CommitSHA string `json:"commit_sha"`
		} `json:"comments"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.State == "" {
		req.State = models.PRReviewStateCommented
	}

	canSubmitDecisionReview := h.repoSvc.HasAccess(repo, currentUser.ID, false)
	if (req.State == models.PRReviewStateApproved || req.State == models.PRReviewStateChangesRequested) && !canSubmitDecisionReview {
		return echo.NewHTTPError(http.StatusForbidden, "only collaborators can approve or request changes")
	}

	var comments []services.CreateReviewCommentInput
	for _, rc := range req.Comments {
		comments = append(comments, services.CreateReviewCommentInput{
			Path:      rc.Path,
			Line:      rc.Line,
			Side:      rc.Side,
			Body:      rc.Body,
			CommitSHA: rc.CommitSHA,
		})
	}

	review, err := h.prSvc.CreateReview(c.Request().Context(), services.CreateReviewInput{
		PRID:      pr.ID,
		AuthorID:  currentUser.ID,
		CommitSHA: req.CommitSHA,
		State:     req.State,
		Body:      req.Body,
		Comments:  comments,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create review")
	}

	return c.JSON(http.StatusCreated, review)
}
