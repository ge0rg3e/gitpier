package handlers

import (
	"errors"
	"net/http"
	"strings"

	"gitpier/internal/config"
	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type OrgHandler struct {
	orgSvc      *services.OrgService
	authSvc     *services.AuthService
	followSvc   *services.FollowService
	repoSvc     *services.RepoService
	gitSvc      *services.GitService
	workflowSvc *services.WorkflowService
	db          *gorm.DB
	cfg         *config.Config
}

func NewOrgHandler(orgSvc *services.OrgService, authSvc *services.AuthService, followSvc *services.FollowService, repoSvc *services.RepoService, gitSvc *services.GitService, workflowSvc *services.WorkflowService, db *gorm.DB, cfg *config.Config) *OrgHandler {
	return &OrgHandler{orgSvc: orgSvc, authSvc: authSvc, followSvc: followSvc, repoSvc: repoSvc, gitSvc: gitSvc, workflowSvc: workflowSvc, db: db, cfg: cfg}
}

func (h *OrgHandler) resolveOrg(c echo.Context) (*models.Organization, error) {
	orgname := c.Param("orgname")
	org, err := h.orgSvc.GetByLogin(c.Request().Context(), orgname)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "organization not found")
	}
	return org, nil
}

func (h *OrgHandler) requireOrgOwner(c echo.Context, org *models.Organization) error {
	currentUser := c.Get("user").(*models.User)
	if !h.orgSvc.IsOwner(c.Request().Context(), org.ID, currentUser.ID) {
		return echo.NewHTTPError(http.StatusForbidden, "only org owners can perform this action")
	}
	return nil
}

func (h *OrgHandler) requireOrgMember(c echo.Context, org *models.Organization) error {
	currentUser := c.Get("user").(*models.User)
	if !h.orgSvc.IsMember(c.Request().Context(), org.ID, currentUser.ID) {
		return echo.NewHTTPError(http.StatusForbidden, "must be an org member")
	}
	return nil
}

// POST /api/v1/orgs
func (h *OrgHandler) Create(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	var req struct {
		Login       string `json:"login"`
		DisplayName string `json:"display_name"`
		Description string `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if !usernameRe.MatchString(req.Login) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid organization name: must be 1-39 alphanumeric characters or hyphens")
	}

	org, err := h.orgSvc.Create(c.Request().Context(), currentUser.ID, req.Login, req.DisplayName, req.Description)
	if err != nil {
		if errors.Is(err, services.ErrOrgExists) {
			return echo.NewHTTPError(http.StatusConflict, "organization name already taken")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create organization")
	}

	return c.JSON(http.StatusCreated, org)
}

// GET /api/v1/orgs/:orgname
func (h *OrgHandler) Get(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}

	isMember := currentUser != nil && h.orgSvc.IsMember(c.Request().Context(), org.ID, currentUser.ID)
	isOwner := currentUser != nil && h.orgSvc.IsOwner(c.Request().Context(), org.ID, currentUser.ID)
	isFollowing := currentUser != nil && h.followSvc.IsFollowingOrg(c.Request().Context(), currentUser.ID, org.ID)

	var memberCount, repoCount int64
	h.orgSvc.CountMembers(c.Request().Context(), org.ID, &memberCount)
	h.orgSvc.CountRepos(c.Request().Context(), org.ID, &repoCount)
	followerCount := h.followSvc.CountOrgFollowers(c.Request().Context(), org.ID)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"org":            org,
		"is_member":      isMember,
		"is_owner":       isOwner,
		"is_following":   isFollowing,
		"member_count":   memberCount,
		"repo_count":     repoCount,
		"follower_count": followerCount,
	})
}

// POST /api/v1/orgs/:orgname/follow
func (h *OrgHandler) Follow(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	currentUser := c.Get("user").(*models.User)

	if err := h.followSvc.FollowOrg(c.Request().Context(), currentUser.ID, org.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to follow organization")
	}

	return c.NoContent(http.StatusNoContent)
}

// DELETE /api/v1/orgs/:orgname/follow
func (h *OrgHandler) Unfollow(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	currentUser := c.Get("user").(*models.User)

	if err := h.followSvc.UnfollowOrg(c.Request().Context(), currentUser.ID, org.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to unfollow organization")
	}

	return c.NoContent(http.StatusNoContent)
}

// GET /api/v1/orgs/:orgname/followers
func (h *OrgHandler) ListFollowers(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}

	var viewer *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		viewer = u
	}
	if !org.IsPublic && (viewer == nil || !h.orgSvc.IsMember(c.Request().Context(), org.ID, viewer.ID)) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	follows, err := h.followSvc.ListOrgFollowers(c.Request().Context(), org.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list followers")
	}

	type followListItem struct {
		User        models.User `json:"user"`
		IsFollowing bool        `json:"is_following"`
		FollowsYou  bool        `json:"follows_you"`
	}

	items := make([]followListItem, 0, len(follows))
	for _, f := range follows {
		isFollowing := viewer != nil && viewer.ID != f.User.ID && h.followSvc.IsFollowingUser(c.Request().Context(), viewer.ID, f.User.ID)
		followsYou := viewer != nil && viewer.ID != f.User.ID && h.followSvc.IsFollowingUser(c.Request().Context(), f.User.ID, viewer.ID)
		user := f.User
		sanitizeUserForPublic(&user)
		items = append(items, followListItem{User: user, IsFollowing: isFollowing, FollowsYou: followsYou})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"users": items,
		"count": len(items),
	})
}

// PATCH /api/v1/orgs/:orgname
func (h *OrgHandler) Update(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	if err := h.requireOrgOwner(c, org); err != nil {
		return err
	}

	var req struct {
		DisplayName *string              `json:"display_name"`
		Description *string              `json:"description"`
		AvatarURL   *string              `json:"avatar_url"`
		Website     *string              `json:"website"`
		SocialLinks *[]models.SocialLink `json:"social_links"`
		Location    *string              `json:"location"`
		IsPublic    *bool                `json:"is_public"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	updates := make(map[string]interface{})
	if req.DisplayName != nil {
		updates["display_name"] = *req.DisplayName
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.AvatarURL != nil {
		updates["avatar_url"] = *req.AvatarURL
	}
	if req.Website != nil {
		updates["website"] = *req.Website
	}
	if req.SocialLinks != nil {
		cleaned := make([]models.SocialLink, 0, len(*req.SocialLinks))
		for _, link := range *req.SocialLinks {
			url := strings.TrimSpace(link.URL)
			if url == "" {
				continue
			}
			cleaned = append(cleaned, models.SocialLink{
				Label: strings.TrimSpace(link.Label),
				URL:   url,
			})
		}
		updates["social_links"] = models.SocialLinks(cleaned)
	}
	if req.Location != nil {
		updates["location"] = *req.Location
	}
	if req.IsPublic != nil {
		updates["is_public"] = *req.IsPublic
	}

	if err := h.orgSvc.Update(c.Request().Context(), org, updates); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update organization")
	}

	updated, _ := h.orgSvc.GetByID(c.Request().Context(), org.ID)
	return c.JSON(http.StatusOK, updated)
}

// DELETE /api/v1/orgs/:orgname
func (h *OrgHandler) Delete(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	if err := h.requireOrgOwner(c, org); err != nil {
		return err
	}

	// Delete org repositories first so org row deletion cannot be blocked by FK constraints.
	repos, err := h.orgSvc.ListOrgRepos(c.Request().Context(), org.ID, true)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load organization repositories")
	}
	for _, repo := range repos {
		repoPath := h.repoSvc.RepoPath(org.Login, repo.Name)
		if deleteErr := h.repoSvc.Delete(c.Request().Context(), &repo); deleteErr != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete organization repositories")
		}
		_ = h.gitSvc.DeleteRepo(repoPath)
	}

	if err := h.orgSvc.Delete(c.Request().Context(), org); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete organization")
	}

	return c.NoContent(http.StatusNoContent)
}

// GET /api/v1/orgs/:orgname/members
func (h *OrgHandler) ListMembers(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}
	if !org.IsPublic && (currentUser == nil || !h.orgSvc.IsMember(c.Request().Context(), org.ID, currentUser.ID)) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	members, err := h.orgSvc.ListMembers(c.Request().Context(), org.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list members")
	}
	sanitizeOrgMembersForPublic(members)

	return c.JSON(http.StatusOK, members)
}

// GET /api/v1/orgs/:orgname/actions/usage
func (h *OrgHandler) GetActionsUsage(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	if err := h.requireOrgOwner(c, org); err != nil {
		return err
	}

	used, limit, month, err := h.workflowSvc.GetActionsUsageForOrg(org.ID)
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

// POST /api/v1/orgs/:orgname/members
func (h *OrgHandler) AddMember(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	if err := h.requireOrgOwner(c, org); err != nil {
		return err
	}

	var req struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	target, err := h.authSvc.GetUserByUsername(c.Request().Context(), req.Username)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	if addErr := h.orgSvc.AddMember(c.Request().Context(), org.ID, target.ID, req.Role); addErr != nil {
		if errors.Is(addErr, services.ErrAlreadyMember) {
			return echo.NewHTTPError(http.StatusConflict, "user is already a member")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to add member")
	}

	m, _ := h.orgSvc.GetMembership(c.Request().Context(), org.ID, target.ID)
	return c.JSON(http.StatusCreated, m)
}

// PATCH /api/v1/orgs/:orgname/members/:username
func (h *OrgHandler) UpdateMemberRole(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	if err := h.requireOrgOwner(c, org); err != nil {
		return err
	}

	target, err := h.authSvc.GetUserByUsername(c.Request().Context(), c.Param("username"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	var req struct {
		Role string `json:"role"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if updateErr := h.orgSvc.UpdateMemberRole(c.Request().Context(), org.ID, target.ID, req.Role); updateErr != nil {
		return echo.NewHTTPError(http.StatusBadRequest, updateErr.Error())
	}

	m, _ := h.orgSvc.GetMembership(c.Request().Context(), org.ID, target.ID)
	return c.JSON(http.StatusOK, m)
}

// DELETE /api/v1/orgs/:orgname/members/:username
func (h *OrgHandler) RemoveMember(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}

	currentUser := c.Get("user").(*models.User)
	targetUsername := c.Param("username")

	target, err := h.authSvc.GetUserByUsername(c.Request().Context(), targetUsername)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	// Only owners can remove others; members can remove themselves
	if currentUser.ID != target.ID {
		if !h.orgSvc.IsOwner(c.Request().Context(), org.ID, currentUser.ID) {
			return echo.NewHTTPError(http.StatusForbidden, "only org owners can remove members")
		}
	}

	if removeErr := h.orgSvc.RemoveMember(c.Request().Context(), org.ID, target.ID); removeErr != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user is not a member")
	}

	return c.NoContent(http.StatusNoContent)
}

// GET /api/v1/orgs/:orgname/teams
func (h *OrgHandler) ListTeams(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}

	// Only members can see teams
	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}
	if !org.IsPublic {
		if currentUser == nil || !h.orgSvc.IsMember(c.Request().Context(), org.ID, currentUser.ID) {
			return echo.NewHTTPError(http.StatusForbidden, "access denied")
		}
	}

	teams, err := h.orgSvc.ListTeams(c.Request().Context(), org.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list teams")
	}

	return c.JSON(http.StatusOK, teams)
}

// POST /api/v1/orgs/:orgname/teams
func (h *OrgHandler) CreateTeam(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	if err := h.requireOrgOwner(c, org); err != nil {
		return err
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Permission  string `json:"permission"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "team name is required")
	}

	team, createErr := h.orgSvc.CreateTeam(c.Request().Context(), org.ID, req.Name, req.Description, req.Permission)
	if createErr != nil {
		if errors.Is(createErr, services.ErrTeamExists) {
			return echo.NewHTTPError(http.StatusConflict, "team already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create team")
	}

	return c.JSON(http.StatusCreated, team)
}

// GET /api/v1/orgs/:orgname/teams/:teamID
func (h *OrgHandler) GetTeam(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}
	if !org.IsPublic && (currentUser == nil || !h.orgSvc.IsMember(c.Request().Context(), org.ID, currentUser.ID)) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	teamID := c.Param("teamID")
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid team ID")
	}

	team, err := h.orgSvc.GetTeam(c.Request().Context(), teamID)
	if err != nil || team.OrgID != org.ID {
		return echo.NewHTTPError(http.StatusNotFound, "team not found")
	}

	return c.JSON(http.StatusOK, team)
}

// PATCH /api/v1/orgs/:orgname/teams/:teamID
func (h *OrgHandler) UpdateTeam(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	if err := h.requireOrgOwner(c, org); err != nil {
		return err
	}

	teamID := c.Param("teamID")
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid team ID")
	}

	team, err := h.orgSvc.GetTeam(c.Request().Context(), teamID)
	if err != nil || team.OrgID != org.ID {
		return echo.NewHTTPError(http.StatusNotFound, "team not found")
	}

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Permission  *string `json:"permission"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Permission != nil {
		updates["permission"] = *req.Permission
	}

	if updateErr := h.orgSvc.UpdateTeam(c.Request().Context(), team, updates); updateErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update team")
	}

	updated, _ := h.orgSvc.GetTeam(c.Request().Context(), team.ID)
	return c.JSON(http.StatusOK, updated)
}

// DELETE /api/v1/orgs/:orgname/teams/:teamID
func (h *OrgHandler) DeleteTeam(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	if err := h.requireOrgOwner(c, org); err != nil {
		return err
	}

	teamID := c.Param("teamID")
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid team ID")
	}

	team, err := h.orgSvc.GetTeam(c.Request().Context(), teamID)
	if err != nil || team.OrgID != org.ID {
		return echo.NewHTTPError(http.StatusNotFound, "team not found")
	}

	if deleteErr := h.orgSvc.DeleteTeam(c.Request().Context(), team); deleteErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete team")
	}

	return c.NoContent(http.StatusNoContent)
}

// GET /api/v1/orgs/:orgname/teams/:teamID/members
func (h *OrgHandler) ListTeamMembers(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}
	if !org.IsPublic && (currentUser == nil || !h.orgSvc.IsMember(c.Request().Context(), org.ID, currentUser.ID)) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	teamID := c.Param("teamID")
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid team ID")
	}

	team, err := h.orgSvc.GetTeam(c.Request().Context(), teamID)
	if err != nil || team.OrgID != org.ID {
		return echo.NewHTTPError(http.StatusNotFound, "team not found")
	}

	members, err := h.orgSvc.ListTeamMembers(c.Request().Context(), team.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list team members")
	}
	sanitizeTeamMembersForPublic(members)

	return c.JSON(http.StatusOK, members)
}

// POST /api/v1/orgs/:orgname/teams/:teamID/members
func (h *OrgHandler) AddTeamMember(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	if err := h.requireOrgOwner(c, org); err != nil {
		return err
	}

	teamID := c.Param("teamID")
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid team ID")
	}

	team, err := h.orgSvc.GetTeam(c.Request().Context(), teamID)
	if err != nil || team.OrgID != org.ID {
		return echo.NewHTTPError(http.StatusNotFound, "team not found")
	}

	var req struct {
		Username string `json:"username"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	target, err := h.authSvc.GetUserByUsername(c.Request().Context(), req.Username)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	// User must be an org member first
	if !h.orgSvc.IsMember(c.Request().Context(), org.ID, target.ID) {
		return echo.NewHTTPError(http.StatusBadRequest, "user must be an org member before being added to a team")
	}

	if addErr := h.orgSvc.AddTeamMember(c.Request().Context(), team.ID, target.ID); addErr != nil {
		if errors.Is(addErr, services.ErrAlreadyMember) {
			return echo.NewHTTPError(http.StatusConflict, "user is already in this team")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to add team member")
	}

	members, _ := h.orgSvc.ListTeamMembers(c.Request().Context(), team.ID)
	return c.JSON(http.StatusCreated, members)
}

// DELETE /api/v1/orgs/:orgname/teams/:teamID/members/:username
func (h *OrgHandler) RemoveTeamMember(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	if err := h.requireOrgOwner(c, org); err != nil {
		return err
	}

	teamID := c.Param("teamID")
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid team ID")
	}

	team, err := h.orgSvc.GetTeam(c.Request().Context(), teamID)
	if err != nil || team.OrgID != org.ID {
		return echo.NewHTTPError(http.StatusNotFound, "team not found")
	}

	target, err := h.authSvc.GetUserByUsername(c.Request().Context(), c.Param("username"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	if removeErr := h.orgSvc.RemoveTeamMember(c.Request().Context(), team.ID, target.ID); removeErr != nil {
		return echo.NewHTTPError(http.StatusNotFound, removeErr.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// GET /api/v1/orgs/:orgname/teams/:teamID/repos
func (h *OrgHandler) ListTeamRepos(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}
	if !org.IsPublic && (currentUser == nil || !h.orgSvc.IsMember(c.Request().Context(), org.ID, currentUser.ID)) {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	teamID := c.Param("teamID")
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid team ID")
	}

	team, err := h.orgSvc.GetTeam(c.Request().Context(), teamID)
	if err != nil || team.OrgID != org.ID {
		return echo.NewHTTPError(http.StatusNotFound, "team not found")
	}

	repos, err := h.orgSvc.ListTeamRepos(c.Request().Context(), team.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list team repos")
	}
	sanitizeTeamReposForPublic(repos)

	return c.JSON(http.StatusOK, repos)
}

// POST /api/v1/orgs/:orgname/teams/:teamID/repos
func (h *OrgHandler) AddTeamRepo(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	if err := h.requireOrgOwner(c, org); err != nil {
		return err
	}

	teamID := c.Param("teamID")
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid team ID")
	}

	team, err := h.orgSvc.GetTeam(c.Request().Context(), teamID)
	if err != nil || team.OrgID != org.ID {
		return echo.NewHTTPError(http.StatusNotFound, "team not found")
	}

	var req struct {
		RepoName string `json:"repo_name"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), org.Login, req.RepoName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	// Ensure repo belongs to this org
	if repo.OrgID == nil || *repo.OrgID != org.ID {
		return echo.NewHTTPError(http.StatusBadRequest, "repository does not belong to this organization")
	}

	if addErr := h.orgSvc.AddTeamRepo(c.Request().Context(), team.ID, repo.ID); addErr != nil {
		return echo.NewHTTPError(http.StatusConflict, addErr.Error())
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{"message": "repository added to team"})
}

// DELETE /api/v1/orgs/:orgname/teams/:teamID/repos/:repoID
func (h *OrgHandler) RemoveTeamRepo(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	if err := h.requireOrgOwner(c, org); err != nil {
		return err
	}

	teamID := c.Param("teamID")
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid team ID")
	}

	team, err := h.orgSvc.GetTeam(c.Request().Context(), teamID)
	if err != nil || team.OrgID != org.ID {
		return echo.NewHTTPError(http.StatusNotFound, "team not found")
	}

	repoID := c.Param("repoID")
	if repoID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid repo ID")
	}

	if removeErr := h.orgSvc.RemoveTeamRepo(c.Request().Context(), team.ID, repoID); removeErr != nil {
		return echo.NewHTTPError(http.StatusNotFound, removeErr.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// GET /api/v1/orgs/:orgname/repos
func (h *OrgHandler) ListRepos(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}
	includePrivate := currentUser != nil && h.orgSvc.IsMember(c.Request().Context(), org.ID, currentUser.ID)

	repos, err := h.orgSvc.ListOrgRepos(c.Request().Context(), org.ID, includePrivate)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list repositories")
	}
	sanitizeReposForPublic(repos)

	return c.JSON(http.StatusOK, repos)
}

// POST /api/v1/orgs/:orgname/repos
func (h *OrgHandler) CreateRepo(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}

	currentUser := c.Get("user").(*models.User)
	if !h.orgSvc.IsMember(c.Request().Context(), org.ID, currentUser.ID) {
		return echo.NewHTTPError(http.StatusForbidden, "must be an org member to create repositories")
	}
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

	repo, createErr := h.repoSvc.Create(c.Request().Context(), services.CreateRepoInput{
		Name:        req.Name,
		Description: req.Description,
		IsPrivate:   req.IsPrivate,
		OwnerID:     currentUser.ID,
		OrgID:       &org.ID,
	})
	if createErr != nil {
		if errors.Is(createErr, services.ErrRepoExists) {
			return echo.NewHTTPError(http.StatusConflict, "repository already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create repository")
	}

	// Initialize bare git repo on disk under the org namespace
	repoPath := h.repoSvc.RepoPath(org.Login, repo.Name)
	if initErr := h.gitSvc.InitRepo(repoPath); initErr != nil {
		h.repoSvc.Delete(c.Request().Context(), repo)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to initialize repository")
	}
	if req.InitializeWithReadme {
		if initErr := h.gitSvc.InitializeWithReadme(repoPath, repo.DefaultBranch, repo.Name, currentUser.Username, currentUser.Email); initErr != nil {
			h.repoSvc.Delete(c.Request().Context(), repo)
			_ = h.gitSvc.DeleteRepo(repoPath)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to initialize repository")
		}
	}

	return c.JSON(http.StatusCreated, repo)
}

// GET /api/v1/users/:username/orgs
func (h *OrgHandler) ListUserOrgs(c echo.Context) error {
	target, err := h.authSvc.GetUserByUsername(c.Request().Context(), c.Param("username"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	orgs, err := h.orgSvc.ListByMember(c.Request().Context(), target.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list organizations")
	}
	for i := range orgs {
		orgs[i].AvatarURL = toAbsoluteURL(c, orgs[i].AvatarURL)
	}

	return c.JSON(http.StatusOK, orgs)
}

// GET /api/v1/users/me/orgs
func (h *OrgHandler) ListMyOrgs(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	orgs, err := h.orgSvc.ListByMember(c.Request().Context(), currentUser.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list organizations")
	}
	for i := range orgs {
		orgs[i].AvatarURL = toAbsoluteURL(c, orgs[i].AvatarURL)
	}
	return c.JSON(http.StatusOK, orgs)
}
