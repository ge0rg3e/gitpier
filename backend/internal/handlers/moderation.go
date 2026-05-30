package handlers

import (
	"errors"
	"net/http"

	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

type ModerationHandler struct {
	modSvc  *services.ModerationService
	repoSvc *services.RepoService
	orgSvc  *services.OrgService
	authSvc *services.AuthService
}

func NewModerationHandler(
	modSvc *services.ModerationService,
	repoSvc *services.RepoService,
	orgSvc *services.OrgService,
	authSvc *services.AuthService,
) *ModerationHandler {
	return &ModerationHandler{modSvc: modSvc, repoSvc: repoSvc, orgSvc: orgSvc, authSvc: authSvc}
}

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }

// GET /api/v1/users/me/moderation
func (h *ModerationHandler) GetUserPolicy(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	policy, err := h.modSvc.GetOrCreateUserPolicy(c.Request().Context(), currentUser.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load policy")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"policy": policy})
}

// PUT /api/v1/users/me/moderation
func (h *ModerationHandler) UpdateUserPolicy(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	policy, err := h.modSvc.GetOrCreateUserPolicy(c.Request().Context(), currentUser.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load policy")
	}
	return h.updatePolicy(c, policy.ID)
}

// POST /api/v1/users/me/moderation/blocked-users
func (h *ModerationHandler) UserBlockUser(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	policy, err := h.modSvc.GetOrCreateUserPolicy(c.Request().Context(), currentUser.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load policy")
	}
	return h.blockUser(c, policy.ID)
}

// DELETE /api/v1/users/me/moderation/blocked-users/:userID
func (h *ModerationHandler) UserUnblockUser(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	policy, err := h.modSvc.GetOrCreateUserPolicy(c.Request().Context(), currentUser.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load policy")
	}
	return h.unblockUser(c, policy.ID)
}

// POST /api/v1/users/me/moderation/keywords
func (h *ModerationHandler) UserAddKeyword(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	policy, err := h.modSvc.GetOrCreateUserPolicy(c.Request().Context(), currentUser.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load policy")
	}
	return h.addKeyword(c, policy.ID)
}

// DELETE /api/v1/users/me/moderation/keywords/:keywordID
func (h *ModerationHandler) UserRemoveKeyword(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	policy, err := h.modSvc.GetOrCreateUserPolicy(c.Request().Context(), currentUser.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load policy")
	}
	return h.removeKeyword(c, policy.ID)
}

// GET /api/v1/orgs/:orgname/moderation
func (h *ModerationHandler) GetOrgPolicy(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	if err := h.requireOrgOwner(c, org); err != nil {
		return err
	}
	policy, err := h.modSvc.GetOrCreateOrgPolicy(c.Request().Context(), org.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load policy")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"policy": policy})
}

// PUT /api/v1/orgs/:orgname/moderation
func (h *ModerationHandler) UpdateOrgPolicy(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	if err := h.requireOrgOwner(c, org); err != nil {
		return err
	}
	policy, err := h.modSvc.GetOrCreateOrgPolicy(c.Request().Context(), org.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load policy")
	}
	return h.updatePolicy(c, policy.ID)
}

// POST /api/v1/orgs/:orgname/moderation/blocked-users
func (h *ModerationHandler) OrgBlockUser(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	if err := h.requireOrgOwner(c, org); err != nil {
		return err
	}
	policy, err := h.modSvc.GetOrCreateOrgPolicy(c.Request().Context(), org.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load policy")
	}
	return h.blockUser(c, policy.ID)
}

// DELETE /api/v1/orgs/:orgname/moderation/blocked-users/:userID
func (h *ModerationHandler) OrgUnblockUser(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	if err := h.requireOrgOwner(c, org); err != nil {
		return err
	}
	policy, err := h.modSvc.GetOrCreateOrgPolicy(c.Request().Context(), org.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load policy")
	}
	return h.unblockUser(c, policy.ID)
}

// POST /api/v1/orgs/:orgname/moderation/keywords
func (h *ModerationHandler) OrgAddKeyword(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	if err := h.requireOrgOwner(c, org); err != nil {
		return err
	}
	policy, err := h.modSvc.GetOrCreateOrgPolicy(c.Request().Context(), org.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load policy")
	}
	return h.addKeyword(c, policy.ID)
}

// DELETE /api/v1/orgs/:orgname/moderation/keywords/:keywordID
func (h *ModerationHandler) OrgRemoveKeyword(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	if err := h.requireOrgOwner(c, org); err != nil {
		return err
	}
	policy, err := h.modSvc.GetOrCreateOrgPolicy(c.Request().Context(), org.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load policy")
	}
	return h.removeKeyword(c, policy.ID)
}

// GET /api/v1/repos/:username/:repo/moderation
func (h *ModerationHandler) GetRepoPolicy(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}
	if err := h.requireRepoOwner(c, repo); err != nil {
		return err
	}
	policy, err := h.modSvc.GetOrCreateRepoPolicy(c.Request().Context(), repo.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load policy")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"policy": policy})
}

// PUT /api/v1/repos/:username/:repo/moderation
func (h *ModerationHandler) UpdateRepoPolicy(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}
	if err := h.requireRepoOwner(c, repo); err != nil {
		return err
	}
	policy, err := h.modSvc.GetOrCreateRepoPolicy(c.Request().Context(), repo.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load policy")
	}
	return h.updatePolicy(c, policy.ID)
}

// POST /api/v1/repos/:username/:repo/moderation/blocked-users
func (h *ModerationHandler) RepoBlockUser(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}
	if err := h.requireRepoOwner(c, repo); err != nil {
		return err
	}
	policy, err := h.modSvc.GetOrCreateRepoPolicy(c.Request().Context(), repo.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load policy")
	}
	return h.blockUser(c, policy.ID)
}

// DELETE /api/v1/repos/:username/:repo/moderation/blocked-users/:userID
func (h *ModerationHandler) RepoUnblockUser(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}
	if err := h.requireRepoOwner(c, repo); err != nil {
		return err
	}
	policy, err := h.modSvc.GetOrCreateRepoPolicy(c.Request().Context(), repo.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load policy")
	}
	return h.unblockUser(c, policy.ID)
}

// POST /api/v1/repos/:username/:repo/moderation/keywords
func (h *ModerationHandler) RepoAddKeyword(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}
	if err := h.requireRepoOwner(c, repo); err != nil {
		return err
	}
	policy, err := h.modSvc.GetOrCreateRepoPolicy(c.Request().Context(), repo.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load policy")
	}
	return h.addKeyword(c, policy.ID)
}

// DELETE /api/v1/repos/:username/:repo/moderation/keywords/:keywordID
func (h *ModerationHandler) RepoRemoveKeyword(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}
	if err := h.requireRepoOwner(c, repo); err != nil {
		return err
	}
	policy, err := h.modSvc.GetOrCreateRepoPolicy(c.Request().Context(), repo.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load policy")
	}
	return h.removeKeyword(c, policy.ID)
}

func (h *ModerationHandler) updatePolicy(c echo.Context, policyID string) error {
	var req struct {
		InheritFromOwner   *bool `json:"inherit_from_owner"`
		BlockIssues        *bool `json:"block_issues"`
		BlockPRs           *bool `json:"block_prs"`
		BlockPushes        *bool `json:"block_pushes"`
		BlockComments      *bool `json:"block_comments"`
		MaxIssuesPerDay    *int  `json:"max_issues_per_day"`
		MaxPRsPerDay       *int  `json:"max_prs_per_day"`
		MaxCommentsPerDay  *int  `json:"max_comments_per_day"`
		MinAccountAgeDays  *int  `json:"min_account_age_days"`
		RequireMinActivity *bool `json:"require_min_activity"`
		MinCommits         *int  `json:"min_commits"`
		MinContributions   *int  `json:"min_contributions"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	updated, err := h.modSvc.UpdatePolicy(c.Request().Context(), policyID, services.UpdatePolicyInput{
		InheritFromOwner:   req.InheritFromOwner,
		BlockIssues:        req.BlockIssues,
		BlockPRs:           req.BlockPRs,
		BlockPushes:        req.BlockPushes,
		BlockComments:      req.BlockComments,
		MaxIssuesPerDay:    req.MaxIssuesPerDay,
		MaxPRsPerDay:       req.MaxPRsPerDay,
		MaxCommentsPerDay:  req.MaxCommentsPerDay,
		MinAccountAgeDays:  req.MinAccountAgeDays,
		RequireMinActivity: req.RequireMinActivity,
		MinCommits:         req.MinCommits,
		MinContributions:   req.MinContributions,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update policy")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"policy": updated})
}

func (h *ModerationHandler) blockUser(c echo.Context, policyID string) error {
	var req struct {
		Username string `json:"username"`
		Reason   string `json:"reason"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Username == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "username is required")
	}

	target, err := h.authSvc.GetUserByUsername(c.Request().Context(), req.Username)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	bu, err := h.modSvc.BlockUser(c.Request().Context(), policyID, target.ID, req.Reason)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to block user")
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"blocked_user": bu})
}

func (h *ModerationHandler) unblockUser(c echo.Context, policyID string) error {
	userID := c.Param("userID")
	if userID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user ID")
	}
	if err := h.modSvc.UnblockUser(c.Request().Context(), policyID, userID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to unblock user")
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *ModerationHandler) addKeyword(c echo.Context, policyID string) error {
	var req struct {
		Keyword string `json:"keyword"`
		ApplyTo string `json:"apply_to"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Keyword == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "keyword is required")
	}
	if req.ApplyTo == "" {
		req.ApplyTo = models.KeywordApplyAll
	}

	kw, err := h.modSvc.AddKeyword(c.Request().Context(), policyID, req.Keyword, req.ApplyTo)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to add keyword")
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"keyword": kw})
}

func (h *ModerationHandler) removeKeyword(c echo.Context, policyID string) error {
	kwID := c.Param("keywordID")
	if kwID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid keyword ID")
	}
	if err := h.modSvc.RemoveKeyword(c.Request().Context(), policyID, kwID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to remove keyword")
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *ModerationHandler) resolveRepo(c echo.Context) (*models.Repository, error) {
	username := c.Param("username")
	repoName := c.Param("repo")
	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	return repo, nil
}

func (h *ModerationHandler) requireRepoOwner(c echo.Context, repo *models.Repository) error {
	currentUser := c.Get("user").(*models.User)
	if !h.repoSvc.IsAdminAccess(repo, currentUser.ID) {
		return echo.NewHTTPError(http.StatusForbidden, "only the repository owner can manage moderation settings")
	}
	return nil
}

func (h *ModerationHandler) resolveOrg(c echo.Context) (*models.Organization, error) {
	orgname := c.Param("orgname")
	org, err := h.orgSvc.GetByLogin(c.Request().Context(), orgname)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "organization not found")
	}
	return org, nil
}

func (h *ModerationHandler) requireOrgOwner(c echo.Context, org *models.Organization) error {
	currentUser := c.Get("user").(*models.User)
	if !h.orgSvc.IsOwner(c.Request().Context(), org.ID, currentUser.ID) {
		return echo.NewHTTPError(http.StatusForbidden, "only org owners can manage moderation settings")
	}
	return nil
}

// moderationError converts service errors to HTTP errors with human-readable messages.
func ModerationError(err error) *echo.HTTPError {
	switch {
	case errors.Is(err, services.ErrModerationBlocked):
		return echo.NewHTTPError(http.StatusForbidden, "You are blocked from interacting with this repository.")
	case errors.Is(err, services.ErrModerationFeatureBlocked):
		return echo.NewHTTPError(http.StatusForbidden, "This type of interaction is currently disabled for this repository.")
	case errors.Is(err, services.ErrModerationRateLimit):
		return echo.NewHTTPError(http.StatusTooManyRequests, "You have reached the interaction limit. Please try again later.")
	case errors.Is(err, services.ErrModerationAccountAge):
		return echo.NewHTTPError(http.StatusForbidden, "Your account does not meet the minimum age requirement to interact with this repository.")
	case errors.Is(err, services.ErrModerationActivity):
		return echo.NewHTTPError(http.StatusForbidden, "Your account does not meet the minimum activity requirements for this repository.")
	case errors.Is(err, services.ErrModerationKeyword):
		return echo.NewHTTPError(http.StatusUnprocessableEntity, err.Error())
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, "moderation check failed")
	}
}
