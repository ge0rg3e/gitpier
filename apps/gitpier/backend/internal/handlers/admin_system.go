package handlers

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type AdminSystemHandler struct {
	db            *gorm.DB
	repoSvc       *services.RepoService
	adminPassword string
}

func NewAdminSystemHandler(db *gorm.DB, repoSvc *services.RepoService, adminPassword string) *AdminSystemHandler {
	return &AdminSystemHandler{
		db:            db,
		repoSvc:       repoSvc,
		adminPassword: adminPassword,
	}
}

func (h *AdminSystemHandler) GetSystemStats(c echo.Context) error {
	if strings.TrimSpace(h.adminPassword) == "" {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "system admin dashboard is not configured")
	}

	providedPassword := c.Request().Header.Get("X-System-Admin-Password")
	if !constantTimeEquals(providedPassword, h.adminPassword) {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid system admin password")
	}

	ctx := c.Request().Context()

	var totalRepos int64
	var publicRepos int64
	var privateRepos int64
	var archivedRepos int64
	var suspendedRepos int64
	var totalUsers int64
	var suspendedUsers int64
	var totalOrgs int64
	var suspendedOrgs int64
	var totalIssues int64
	var openIssues int64
	var closedIssues int64
	var totalPRs int64
	var openPRs int64
	var closedPRs int64
	var mergedPRs int64
	var totalWorkflowRuns int64
	var runningWorkflowRuns int64
	var successfulWorkflowRuns int64
	var failedWorkflowRuns int64

	if err := h.db.WithContext(ctx).Model(&models.Repository{}).Count(&totalRepos).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to fetch repository totals")
	}
	_ = h.db.WithContext(ctx).Model(&models.Repository{}).Where("is_private = ?", false).Count(&publicRepos).Error
	_ = h.db.WithContext(ctx).Model(&models.Repository{}).Where("is_private = ?", true).Count(&privateRepos).Error
	_ = h.db.WithContext(ctx).Model(&models.Repository{}).Where("is_archived = ?", true).Count(&archivedRepos).Error
	_ = h.db.WithContext(ctx).Model(&models.Repository{}).Where("is_suspended = ?", true).Count(&suspendedRepos).Error

	_ = h.db.WithContext(ctx).Model(&models.User{}).Count(&totalUsers).Error
	_ = h.db.WithContext(ctx).Model(&models.User{}).Where("is_suspended = ?", true).Count(&suspendedUsers).Error
	_ = h.db.WithContext(ctx).Model(&models.Organization{}).Count(&totalOrgs).Error
	_ = h.db.WithContext(ctx).Model(&models.Organization{}).Where("is_suspended = ?", true).Count(&suspendedOrgs).Error

	_ = h.db.WithContext(ctx).Model(&models.Issue{}).Count(&totalIssues).Error
	_ = h.db.WithContext(ctx).Model(&models.Issue{}).Where("status = ?", models.IssueStatusOpen).Count(&openIssues).Error
	_ = h.db.WithContext(ctx).Model(&models.Issue{}).Where("status = ?", models.IssueStatusClosed).Count(&closedIssues).Error

	_ = h.db.WithContext(ctx).Model(&models.PullRequest{}).Count(&totalPRs).Error
	_ = h.db.WithContext(ctx).Model(&models.PullRequest{}).Where("status = ?", models.PRStatusOpen).Count(&openPRs).Error
	_ = h.db.WithContext(ctx).Model(&models.PullRequest{}).Where("status = ?", models.PRStatusClosed).Count(&closedPRs).Error
	_ = h.db.WithContext(ctx).Model(&models.PullRequest{}).Where("status = ?", models.PRStatusMerged).Count(&mergedPRs).Error

	_ = h.db.WithContext(ctx).Model(&models.WorkflowRun{}).Count(&totalWorkflowRuns).Error
	_ = h.db.WithContext(ctx).Model(&models.WorkflowRun{}).Where("status = ?", "running").Count(&runningWorkflowRuns).Error
	_ = h.db.WithContext(ctx).Model(&models.WorkflowRun{}).Where("status = ?", "success").Count(&successfulWorkflowRuns).Error
	_ = h.db.WithContext(ctx).Model(&models.WorkflowRun{}).Where("status = ?", "failure").Count(&failedWorkflowRuns).Error

	var sumRow struct {
		Total int64
	}
	if err := h.db.WithContext(ctx).Model(&models.Repository{}).Select("COALESCE(SUM(size), 0) AS total").Scan(&sumRow).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to calculate repository size totals")
	}

	var allRepos []models.Repository
	if err := h.db.WithContext(ctx).Preload("Owner").Preload("Org").Find(&allRepos).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to fetch repositories")
	}

	var fsTotalBytes int64
	scanErrors := 0
	for i := range allRepos {
		repo := &allRepos[i]
		namespace := h.repoSvc.RepoNamespace(repo)
		if namespace == "" {
			scanErrors++
			continue
		}

		repoPath := h.repoSvc.RepoPath(namespace, repo.Name)
		size, err := h.repoSvc.CalculateRepoSize(repoPath)
		if err != nil {
			scanErrors++
			continue
		}
		fsTotalBytes += size
	}

	var largestRepos []models.Repository
	if err := h.db.WithContext(ctx).Preload("Owner").Preload("Org").Order("size DESC").Limit(5).Find(&largestRepos).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to fetch largest repositories")
	}

	largest := make([]map[string]interface{}, 0, len(largestRepos))
	for i := range largestRepos {
		repo := &largestRepos[i]
		namespace := h.repoSvc.RepoNamespace(repo)
		largest = append(largest, map[string]interface{}{
			"id":         repo.ID,
			"namespace":  namespace,
			"name":       repo.Name,
			"full_name":  namespace + "/" + repo.Name,
			"size_bytes": repo.Size,
		})
	}

	avgSizeBytes := int64(0)
	if totalRepos > 0 {
		avgSizeBytes = sumRow.Total / totalRepos
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"generated_at": time.Now().UTC(),
		"repositories": map[string]interface{}{
			"total":                       totalRepos,
			"public":                      publicRepos,
			"private":                     privateRepos,
			"archived":                    archivedRepos,
			"suspended":                   suspendedRepos,
			"total_size_bytes":            sumRow.Total,
			"total_size_gb":               bytesToGB(sumRow.Total),
			"filesystem_total_size_bytes": fsTotalBytes,
			"filesystem_total_size_gb":    bytesToGB(fsTotalBytes),
			"average_size_bytes":          avgSizeBytes,
			"average_size_mb":             bytesToMB(avgSizeBytes),
			"filesystem_scan_errors":      scanErrors,
		},
		"users": map[string]interface{}{
			"total":     totalUsers,
			"suspended": suspendedUsers,
		},
		"organizations": map[string]interface{}{
			"total":     totalOrgs,
			"suspended": suspendedOrgs,
		},
		"issues": map[string]interface{}{
			"total":  totalIssues,
			"open":   openIssues,
			"closed": closedIssues,
		},
		"pull_requests": map[string]interface{}{
			"total":  totalPRs,
			"open":   openPRs,
			"closed": closedPRs,
			"merged": mergedPRs,
		},
		"workflow_runs": map[string]interface{}{
			"total":   totalWorkflowRuns,
			"running": runningWorkflowRuns,
			"success": successfulWorkflowRuns,
			"failure": failedWorkflowRuns,
		},
		"largest_repositories": largest,
	})
}

func constantTimeEquals(a, b string) bool {
	aBytes := []byte(a)
	bBytes := []byte(b)
	if len(aBytes) != len(bBytes) {
		return false
	}
	return subtle.ConstantTimeCompare(aBytes, bBytes) == 1
}

func bytesToGB(v int64) float64 {
	return float64(v) / 1024 / 1024 / 1024
}

func bytesToMB(v int64) float64 {
	return float64(v) / 1024 / 1024
}
