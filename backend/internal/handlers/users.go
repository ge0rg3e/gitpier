package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"gitpier/internal/cache"
	"gitpier/internal/models"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type UserHandler struct {
	authSvc     *services.AuthService
	followSvc   *services.FollowService
	repoSvc     *services.RepoService
	gitSvc      *services.GitService
	workflowSvc *services.WorkflowService
	db          *gorm.DB
	cache       cache.Store
}

const contributionAggregateTTL = 3 * time.Minute

type contributionAggregate struct {
	counts        map[string]int
	total         int
	currentStreak int
	longestStreak int
}

func NewUserHandler(authSvc *services.AuthService, followSvc *services.FollowService, repoSvc *services.RepoService, gitSvc *services.GitService, workflowSvc *services.WorkflowService, db *gorm.DB, cacheStore cache.Store) *UserHandler {
	return &UserHandler{authSvc: authSvc, followSvc: followSvc, repoSvc: repoSvc, gitSvc: gitSvc, workflowSvc: workflowSvc, db: db, cache: cacheStore}
}

type dashboardPullRequest struct {
	models.PullRequest
	RepoOwner string `json:"repo_owner"`
	RepoName  string `json:"repo_name"`
}

func (h *UserHandler) GetActionsUsage(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	used, limit, month, err := h.workflowSvc.GetActionsUsageForUser(currentUser.ID)
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

func (h *UserHandler) GetDashboard(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	ctx := c.Request().Context()

	recentLimit := 16
	if q := c.QueryParam("recent_limit"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 1 || n > 64 {
			return echo.NewHTTPError(http.StatusBadRequest, "recent_limit must be between 1 and 64")
		}
		recentLimit = n
	}

	var openPullRequests int64
	if err := h.db.WithContext(ctx).
		Model(&models.PullRequest{}).
		Where("author_id = ? AND status = ?", currentUser.ID, models.PRStatusOpen).
		Count(&openPullRequests).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load pull request count")
	}

	var openIssues int64
	if err := h.db.WithContext(ctx).
		Model(&models.Issue{}).
		Where("author_id = ? AND status = ?", currentUser.ID, models.IssueStatusOpen).
		Count(&openIssues).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load issue count")
	}

	var reviewRequests int64
	if err := h.db.WithContext(ctx).
		Model(&models.PullRequest{}).
		Where("assignee_id = ? AND status = ? AND author_id <> ?", currentUser.ID, models.PRStatusOpen, currentUser.ID).
		Count(&reviewRequests).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load review request count")
	}

	var recentPRs []models.PullRequest
	if err := h.db.WithContext(ctx).
		Preload("Author").
		Preload("Assignee").
		Preload("Labels").
		Preload("Repo.Owner").
		Preload("Repo.Org").
		Where("status = ? AND (author_id = ? OR (assignee_id = ? AND author_id <> ?))", models.PRStatusOpen, currentUser.ID, currentUser.ID, currentUser.ID).
		Order("updated_at DESC").
		Limit(recentLimit).
		Find(&recentPRs).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load recent pull requests")
	}

	recent := make([]dashboardPullRequest, 0, len(recentPRs))
	for _, pr := range recentPRs {
		owner := pr.Repo.Owner.Username
		if pr.Repo.Org != nil {
			owner = pr.Repo.Org.Login
		}
		recent = append(recent, dashboardPullRequest{
			PullRequest: pr,
			RepoOwner:   owner,
			RepoName:    pr.Repo.Name,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"open_pull_requests":   openPullRequests,
		"open_issues":          openIssues,
		"review_requests":      reviewRequests,
		"recent_pull_requests": recent,
	})
}

func (h *UserHandler) GetProfile(c echo.Context) error {
	username := c.Param("username")

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}

	// Get user by username
	user, err := h.authSvc.GetUserByUsername(c.Request().Context(), username)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	// List repos (only public unless viewing own profile)
	includePrivate := currentUser != nil && currentUser.Username == username
	limit := 0
	offset := 0
	if q := c.QueryParam("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if q := c.QueryParam("offset"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n >= 0 {
			offset = n
		}
	}

	repos, err := h.repoSvc.ListByOwnerPaged(c.Request().Context(), username, includePrivate, limit, offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list repositories")
	}
	sanitizeUserForPublic(user)
	sanitizeReposForPublic(repos)

	followerCount := h.followSvc.CountUserFollowers(c.Request().Context(), user.ID)
	followingCount := h.followSvc.CountUserFollowing(c.Request().Context(), user.ID) + h.followSvc.CountOrgFollowing(c.Request().Context(), user.ID)
	isFollowing := currentUser != nil && currentUser.ID != user.ID && h.followSvc.IsFollowingUser(c.Request().Context(), currentUser.ID, user.ID)
	followsYou := currentUser != nil && currentUser.ID != user.ID && h.followSvc.IsFollowingUser(c.Request().Context(), user.ID, currentUser.ID)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"user":            user,
		"repos":           repos,
		"follower_count":  followerCount,
		"following_count": followingCount,
		"is_following":    isFollowing,
		"follows_you":     followsYou,
	})
}

func (h *UserHandler) FollowUser(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	targetUsername := c.Param("username")

	target, err := h.authSvc.GetUserByUsername(c.Request().Context(), targetUsername)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	if err := h.followSvc.FollowUser(c.Request().Context(), currentUser.ID, target.ID); err != nil {
		if err == services.ErrCannotFollowSelf {
			return echo.NewHTTPError(http.StatusBadRequest, "cannot follow yourself")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to follow user")
	}
	h.invalidateUserCaches(c.Request().Context(), currentUser.ID)
	h.invalidateUserCaches(c.Request().Context(), target.ID)

	return c.NoContent(http.StatusNoContent)
}

func (h *UserHandler) UnfollowUser(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	targetUsername := c.Param("username")

	target, err := h.authSvc.GetUserByUsername(c.Request().Context(), targetUsername)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	if err := h.followSvc.UnfollowUser(c.Request().Context(), currentUser.ID, target.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to unfollow user")
	}
	h.invalidateUserCaches(c.Request().Context(), currentUser.ID)
	h.invalidateUserCaches(c.Request().Context(), target.ID)

	return c.NoContent(http.StatusNoContent)
}

func (h *UserHandler) ListFollowers(c echo.Context) error {
	targetUsername := c.Param("username")
	target, err := h.authSvc.GetUserByUsername(c.Request().Context(), targetUsername)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	follows, err := h.followSvc.ListUserFollowers(c.Request().Context(), target.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list followers")
	}

	var viewer *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		viewer = u
	}

	type followListItem struct {
		User        models.User `json:"user"`
		IsFollowing bool        `json:"is_following"`
		FollowsYou  bool        `json:"follows_you"`
	}

	items := make([]followListItem, 0, len(follows))
	for _, f := range follows {
		isFollowing := viewer != nil && viewer.ID != f.Follower.ID && h.followSvc.IsFollowingUser(c.Request().Context(), viewer.ID, f.Follower.ID)
		followsYou := viewer != nil && viewer.ID != f.Follower.ID && h.followSvc.IsFollowingUser(c.Request().Context(), f.Follower.ID, viewer.ID)
		user := f.Follower
		sanitizeUserForPublic(&user)
		items = append(items, followListItem{User: user, IsFollowing: isFollowing, FollowsYou: followsYou})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"users": items,
		"count": len(items),
	})
}

func (h *UserHandler) ListFollowing(c echo.Context) error {
	targetUsername := c.Param("username")
	target, err := h.authSvc.GetUserByUsername(c.Request().Context(), targetUsername)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	userFollows, err := h.followSvc.ListUserFollowing(c.Request().Context(), target.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list following")
	}
	orgFollows, err := h.followSvc.ListOrgFollowing(c.Request().Context(), target.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list following")
	}

	var viewer *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		viewer = u
	}

	type followListItem struct {
		EntityType  string               `json:"entity_type"`
		User        *models.User         `json:"user,omitempty"`
		Org         *models.Organization `json:"org,omitempty"`
		IsFollowing bool                 `json:"is_following"`
		FollowsYou  bool                 `json:"follows_you"`
	}

	userItems := make([]followListItem, 0, len(userFollows))
	for _, f := range userFollows {
		isFollowing := viewer != nil && viewer.ID != f.Following.ID && h.followSvc.IsFollowingUser(c.Request().Context(), viewer.ID, f.Following.ID)
		followsYou := viewer != nil && viewer.ID != f.Following.ID && h.followSvc.IsFollowingUser(c.Request().Context(), f.Following.ID, viewer.ID)
		u := f.Following
		sanitizeUserForPublic(&u)
		userItems = append(userItems, followListItem{EntityType: "user", User: &u, IsFollowing: isFollowing, FollowsYou: followsYou})
	}

	items := make([]followListItem, 0, len(userFollows)+len(orgFollows))
	items = append(items, userItems...)
	for _, f := range orgFollows {
		isFollowing := viewer != nil && h.followSvc.IsFollowingOrg(c.Request().Context(), viewer.ID, f.OrgID)
		o := f.Org
		items = append(items, followListItem{EntityType: "org", Org: &o, IsFollowing: isFollowing, FollowsYou: false})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"users": userItems,
		"items": items,
		"count": len(items),
	})
}

func (h *UserHandler) UpdateProfile(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	var req struct {
		Bio         *string `json:"bio"`
		AvatarURL   *string `json:"avatar_url"`
		DisplayName *string `json:"display_name"`
		Location    *string `json:"location"`
		Website     *string `json:"website"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	updates := map[string]interface{}{}
	if req.Bio != nil {
		updates["bio"] = *req.Bio
	}
	if req.AvatarURL != nil {
		updates["avatar_url"] = *req.AvatarURL
	}
	if req.DisplayName != nil {
		updates["display_name"] = *req.DisplayName
	}
	if req.Location != nil {
		updates["location"] = *req.Location
	}
	if req.Website != nil {
		updates["website"] = *req.Website
	}

	if len(updates) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no profile fields provided")
	}

	if err := h.authSvc.UpdateUser(c.Request().Context(), currentUser.ID, updates); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update profile")
	}
	h.invalidateUserCaches(c.Request().Context(), currentUser.ID)

	// Reload
	user, _ := h.authSvc.GetUserByID(c.Request().Context(), currentUser.ID)
	return c.JSON(http.StatusOK, user)
}

func (h *UserHandler) GetContributions(c echo.Context) error {
	username := c.Param("username")
	ctx := c.Request().Context()

	user, err := h.authSvc.GetUserByUsername(ctx, username)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}
	includePrivate := currentUser != nil && currentUser.Username == username

	agg, err := h.getContributionAggregate(ctx, user, includePrivate)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load contributions")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"contributions": cloneContributionCounts(agg.counts),
		"total":         agg.total,
	})
}

func contributionCacheKey(userID string, includePrivate bool) string {
	return fmt.Sprintf("cache:user:%d:contrib:%t", userID, includePrivate)
}

func (h *UserHandler) invalidateUserCaches(ctx context.Context, userID string) {
	_ = h.cache.DeleteByPrefix(ctx, fmt.Sprintf("cache:user:%d:", userID))
}

func cloneContributionCounts(src map[string]int) map[string]int {
	if len(src) == 0 {
		return map[string]int{}
	}
	dst := make(map[string]int, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func (h *UserHandler) getContributionAggregate(ctx context.Context, user *models.User, includePrivate bool) (contributionAggregate, error) {
	key := contributionCacheKey(user.ID, includePrivate)
	now := time.Now().UTC()
	cachedBytes, err := h.cache.RememberJSON(ctx, key, contributionAggregateTTL, func() (interface{}, error) {
		repos, err := h.listContributionRepos(ctx, user, includePrivate)
		if err != nil {
			return nil, err
		}

		since := now.AddDate(-1, 0, 0)
		allCounts := make(map[string]int)
		var allCountsMu sync.Mutex
		var wg sync.WaitGroup
		workers := 4
		sem := make(chan struct{}, workers)

		for _, repo := range repos {
			// Repo activity updates timestamps on push; skip stale repos outside window.
			if repo.UpdatedAt.Before(since) {
				continue
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(repo models.Repository) {
				defer wg.Done()
				defer func() { <-sem }()

				namespace := repo.Owner.Username
				if repo.Org != nil {
					namespace = repo.Org.Login
				}
				repoPath := h.repoSvc.RepoPath(namespace, repo.Name)
				counts, err := h.gitSvc.GetContributions(repoPath, user.Email, since)
				if err != nil {
					return
				}

				allCountsMu.Lock()
				for day, count := range counts {
					allCounts[day] += count
				}
				allCountsMu.Unlock()
			}(repo)
		}

		wg.Wait()

		total := 0
		for _, v := range allCounts {
			total += v
		}
		currentStreak, longestStreak := computeStreaks(allCounts)

		agg := contributionAggregate{
			counts:        cloneContributionCounts(allCounts),
			total:         total,
			currentStreak: currentStreak,
			longestStreak: longestStreak,
		}
		return agg, nil
	})
	if err != nil {
		return contributionAggregate{}, err
	}

	var agg contributionAggregate
	if err := json.Unmarshal(cachedBytes, &agg); err != nil {
		return contributionAggregate{}, err
	}
	return agg, nil
}

// listContributionRepos returns repositories that should count toward a user's
// profile contribution activity.
//
// Visibility rules:
// - Public repos are always included.
// - Private repos are included only when the viewer is the profile owner.
// - Organization repos are included for orgs where the target user is a member.
func (h *UserHandler) listContributionRepos(ctx context.Context, user *models.User, includePrivate bool) ([]models.Repository, error) {
	repos, err := h.repoSvc.ListByOwner(ctx, user.Username, includePrivate)
	if err != nil {
		return nil, err
	}

	var memberships []models.OrganizationMember
	if err := h.db.WithContext(ctx).
		Where("user_id = ?", user.ID).
		Find(&memberships).Error; err != nil {
		return nil, err
	}

	for _, membership := range memberships {
		orgRepos, err := h.repoSvc.ListByOrg(ctx, membership.OrgID, includePrivate)
		if err != nil {
			return nil, err
		}
		repos = append(repos, orgRepos...)
	}

	return repos, nil
}

// ExportData returns all personal data stored for the authenticated user (GDPR Art. 20).
func (h *UserHandler) ExportData(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	ctx := c.Request().Context()

	user, err := h.authSvc.GetUserByID(ctx, currentUser.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load user data")
	}

	repos, _ := h.repoSvc.ListByOwner(ctx, user.Username, true)

	var sshKeys []models.SSHKey
	h.db.WithContext(ctx).Where("user_id = ?", user.ID).Find(&sshKeys)

	export := map[string]interface{}{
		"account": map[string]interface{}{
			"username":        user.Username,
			"display_name":    user.DisplayName,
			"email":           user.Email,
			"bio":             user.Bio,
			"location":        user.Location,
			"website":         user.Website,
			"created_at":      user.CreatedAt,
			"gdpr_consent_at": user.GDPRConsentAt,
		},
		"repositories": repos,
		"ssh_keys":     sshKeys,
		"exported_at":  time.Now().UTC(),
	}

	c.Response().Header().Set("Content-Disposition", `attachment; filename="gitpier-data-export.json"`)
	return c.JSON(http.StatusOK, export)
}

// DeleteAccount permanently deletes the authenticated user's account (GDPR Art. 17).
func (h *UserHandler) DeleteAccount(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	ctx := c.Request().Context()

	var req struct {
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "password is required to confirm account deletion")
	}

	// Re-fetch full user to get hashed password
	var user models.User
	if err := h.db.WithContext(ctx).First(&user, currentUser.ID).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load user")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return echo.NewHTTPError(http.StatusForbidden, "incorrect password")
	}

	// Delete in a transaction: ssh keys, stars, then user
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tx.Where("user_id = ?", user.ID).Delete(&models.SSHKey{})
		tx.Where("user_id = ?", user.ID).Delete(&models.Star{})
		tx.Where("follower_id = ? OR following_id = ?", user.ID, user.ID).Delete(&models.UserFollow{})
		tx.Where("user_id = ?", user.ID).Delete(&models.OrgFollow{})
		if err := tx.Delete(&user).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete account")
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "account deleted"})
}

// defaultWidgets returns the default widget configuration for a profile.
func defaultWidgets() map[string]bool {
	return map[string]bool{
		"stats":              true,
		"top_languages":      true,
		"contribution_graph": true,
		"streak":             true,
		"activity":           true,
	}
}

// GetProfileWidgets returns the widget visibility settings for a user's profile.
func (h *UserHandler) GetProfileWidgets(c echo.Context) error {
	username := c.Param("username")
	user, err := h.authSvc.GetUserByUsername(c.Request().Context(), username)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	widgets := defaultWidgets()
	if user.ProfileWidgets != "" && user.ProfileWidgets != "{}" {
		if err := json.Unmarshal([]byte(user.ProfileWidgets), &widgets); err == nil {
			// Ensure all default keys exist
			for k, v := range defaultWidgets() {
				if _, ok := widgets[k]; !ok {
					widgets[k] = v
				}
			}
		}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"widgets": widgets})
}

// UpdateProfileWidgets updates the widget visibility settings for the authenticated user.
func (h *UserHandler) UpdateProfileWidgets(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	var req map[string]bool
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Only allow known widget keys
	allowed := defaultWidgets()
	sanitized := make(map[string]bool)
	for k := range allowed {
		if v, ok := req[k]; ok {
			sanitized[k] = v
		} else {
			sanitized[k] = allowed[k]
		}
	}

	b, err := json.Marshal(sanitized)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to encode widgets")
	}

	if err := h.authSvc.UpdateUser(c.Request().Context(), currentUser.ID, map[string]interface{}{
		"profile_widgets": string(b),
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update widgets")
	}
	h.invalidateUserCaches(c.Request().Context(), currentUser.ID)

	return c.JSON(http.StatusOK, map[string]interface{}{"widgets": sanitized})
}

type langStat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// GetProfileStats returns aggregated statistics for a user's profile.
func (h *UserHandler) GetProfileStats(c echo.Context) error {
	username := c.Param("username")
	ctx := c.Request().Context()

	user, err := h.authSvc.GetUserByUsername(ctx, username)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	var currentUser *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		currentUser = u
	}
	includePrivate := currentUser != nil && currentUser.Username == username

	// Total public repos
	var totalRepos int64
	repoQuery := h.db.WithContext(ctx).Model(&models.Repository{}).Where("owner_id = ?", user.ID)
	if !includePrivate {
		repoQuery = repoQuery.Where("is_private = false")
	}
	repoQuery.Count(&totalRepos)

	// Total stars received
	var totalStars int64
	h.db.WithContext(ctx).Model(&models.Star{}).
		Joins("JOIN repositories ON repositories.id = stars.repo_id").
		Where("repositories.owner_id = ?", user.ID).
		Count(&totalStars)

	// Total PRs authored
	var totalPRs int64
	h.db.WithContext(ctx).Model(&models.PullRequest{}).
		Where("author_id = ?", user.ID).
		Count(&totalPRs)

	// Total issues authored
	var totalIssues int64
	h.db.WithContext(ctx).Model(&models.Issue{}).
		Where("author_id = ?", user.ID).
		Count(&totalIssues)

	// Top languages from repos
	type langCount struct {
		Language string
		Cnt      int64
	}
	var langRows []langCount
	langQuery := h.db.WithContext(ctx).
		Model(&models.Repository{}).
		Select("language, COUNT(*) as cnt").
		Where("owner_id = ? AND language IS NOT NULL AND language != ''", user.ID)
	if !includePrivate {
		langQuery = langQuery.Where("is_private = false")
	}
	langQuery.Group("language").Order("cnt DESC").Limit(6).Scan(&langRows)

	topLangs := make([]langStat, 0, len(langRows))
	for _, r := range langRows {
		topLangs = append(topLangs, langStat{Name: r.Language, Count: int(r.Cnt)})
	}
	sort.Slice(topLangs, func(i, j int) bool { return topLangs[i].Count > topLangs[j].Count })

	// Commits in the last year + streak (shared cached aggregation)
	agg, err := h.getContributionAggregate(ctx, user, includePrivate)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load contribution stats")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"total_stars":    totalStars,
		"total_repos":    totalRepos,
		"total_prs":      totalPRs,
		"total_issues":   totalIssues,
		"total_commits":  agg.total,
		"top_languages":  topLangs,
		"current_streak": agg.currentStreak,
		"longest_streak": agg.longestStreak,
	})
}

// computeStreaks returns (currentStreak, longestStreak) from a dayâ†’count map.
func computeStreaks(contribs map[string]int) (int, int) {
	if len(contribs) == 0 {
		return 0, 0
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)

	current := 0
	longest := 0
	streak := 0

	for i := 0; i < 365; i++ {
		d := today.AddDate(0, 0, -i)
		key := d.Format("2006-01-02")
		if contribs[key] > 0 {
			streak++
			if i == 0 || current > 0 {
				current = streak
			}
			if streak > longest {
				longest = streak
			}
		} else {
			if i == 0 {
				// no commit today â€” current streak is 0
				current = 0
			} else if current == streak-1 {
				// streak just broke
				current = 0
			}
			streak = 0
		}
	}
	return current, longest
}
