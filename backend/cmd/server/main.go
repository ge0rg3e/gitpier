package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gitpier/internal/cache"
	"gitpier/internal/config"
	"gitpier/internal/database"
	"gitpier/internal/handlers"
	apimiddleware "gitpier/internal/middleware"
	"gitpier/internal/models"
	"gitpier/internal/services"
	sshserver "gitpier/internal/ssh"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	db, err := database.Connect(cfg.DatabaseURL, cfg.DBMaxOpenConns, cfg.DBMaxIdleConns, cfg.DBConnMaxLifetimeMinutes)
	if err != nil {
		log.Fatalf("database connection error: %v", err)
	}

	if err := database.Migrate(db); err != nil {
		log.Fatalf("migration error: %v", err)
	}

	// Ensure repos directory exists
	if err := os.MkdirAll(cfg.ReposPath, 0755); err != nil {
		log.Fatalf("failed to create repos dir: %v", err)
	}

	// Ensure avatars directory exists
	if err := os.MkdirAll(cfg.AvatarsPath, 0755); err != nil {
		log.Fatalf("failed to create avatars dir: %v", err)
	}

	// Ensure packages directory exists
	if err := os.MkdirAll(cfg.PackagesPath, 0750); err != nil {
		log.Fatalf("failed to create packages dir: %v", err)
	}

	// Ensure markdown assets directory exists
	if err := os.MkdirAll(cfg.MarkdownAssetsPath, 0755); err != nil {
		log.Fatalf("failed to create markdown assets dir: %v", err)
	}

	// Services
	smtpEmailSvc := services.NewSMTPEmailService(services.SMTPEmailConfig{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		From:     cfg.SMTPFrom,
		FromName: cfg.SMTPFromName,
	})
	var regMailer services.RegistrationMailer
	if smtpEmailSvc.IsConfigured() {
		regMailer = smtpEmailSvc
		log.Printf("registration OTP emails enabled via SMTP")
	}

	authSvc := services.NewAuthService(db, cfg.JWTSecret, cfg.SecretEncryptionKey, regMailer)
	followSvc := services.NewFollowService(db)
	repoSvc := services.NewRepoService(db, cfg.ReposPath, cfg.RepoPublicSizeLimitBytes, cfg.RepoPrivateSizeLimitBytes)
	gitSvc := services.NewGitService(cfg.GitIdentityName, cfg.GitIdentityEmail)
	orgSvc := services.NewOrgService(db)
	cacheStore := cache.NewStore(cfg.RedisURL)

	// Fix any existing bare repos whose HEAD points to a non-existent ref (e.g. master vs main)
	gitSvc.MigrateHEADs(cfg.ReposPath)

	sshKeySvc := services.NewSSHKeyService(db)
	prSvc := services.NewPRService(db, gitSvc, repoSvc)
	repoEnvSvc := services.NewRepoEnvService(db, cfg.SecretEncryptionKey)
	issueSvc := services.NewIssueService(db, repoSvc)
	projectSvc := services.NewProjectService(db)

	// Workflow runner (connects to Docker-in-Docker; nil-safe if unavailable)
	workflowRunner, _ := services.NewWorkflowRunner(cfg)
	workflowSvc := services.NewWorkflowService(db, gitSvc, repoSvc, workflowRunner, cfg, repoEnvSvc)
	// Mark any runs stuck in-progress from a previous restart as cancelled
	workflowSvc.CancelStaleRuns()

	// Release service (depends on repoSvc + gitSvc)
	releaseSvc := services.NewReleaseService(db, gitSvc, repoSvc, cfg)
	// Wire releases into the workflow runner so upload-release-asset works
	workflowRunner.SetReleaseService(releaseSvc)

	searchSvc := services.NewSearchService(db, repoSvc)
	oauthAppSvc := services.NewOAuthAppService(db)
	oauthFlowSvc := services.NewOAuthFlowService(db)
	gitlodeAppSvc := services.NewAppService(db)

	// Anti-spam service
	antiSpamSvc, err := services.NewAntiSpamService(db, services.AntiSpamConfig{
		TurnstileSecretKey: cfg.TurnstileSecretKey,
		EnableTurnstile:    cfg.EnableTurnstile,
		EnableRateLimiting: cfg.EnableRateLimiting,
	})
	if err != nil {
		log.Fatalf("failed to initialize anti-spam service: %v", err)
	}
	defer antiSpamSvc.Close()

	// Handlers
	handlers.ConfigurePublicBaseURL(cfg.AppURL)
	authHandler := handlers.NewAuthHandler(authSvc, antiSpamSvc, cfg.AppURL, cfg)
	userHandler := handlers.NewUserHandler(authSvc, followSvc, repoSvc, gitSvc, workflowSvc, db, cacheStore)
	repoHandler := handlers.NewRepoHandler(repoSvc, gitSvc, orgSvc, authSvc, cacheStore, cfg)
	avatarHandler := handlers.NewAvatarHandler(cfg, authSvc, orgSvc)
	sshKeyHandler := handlers.NewSSHKeyHandler(sshKeySvc)
	prHandler := handlers.NewPRHandler(prSvc, repoSvc, gitSvc, authSvc)
	workflowHandler := handlers.NewWorkflowHandler(workflowSvc, repoSvc)
	repoEnvHandler := handlers.NewRepoEnvHandler(repoEnvSvc, repoSvc)
	releaseHandler := handlers.NewReleaseHandler(releaseSvc, repoSvc, gitSvc, workflowSvc)
	markdownAssetHandler := handlers.NewMarkdownAssetHandler(repoSvc, cfg.MarkdownAssetsPath)
	issueHandler := handlers.NewIssueHandler(issueSvc, repoSvc)
	projectHandler := handlers.NewProjectHandler(projectSvc, orgSvc, db)
	orgHandler := handlers.NewOrgHandler(orgSvc, authSvc, followSvc, repoSvc, gitSvc, workflowSvc, db, cfg)
	modSvc := services.NewModerationService(db)
	modHandler := handlers.NewModerationHandler(modSvc, repoSvc, orgSvc, authSvc)
	issueHandler.SetModerationService(modSvc)
	prHandler.SetModerationService(modSvc)
	searchHandler := handlers.NewSearchHandler(searchSvc, repoSvc, cacheStore)
	oauthAppHandler := handlers.NewOAuthAppHandler(oauthAppSvc, orgSvc)
	oauthFlowHandler := handlers.NewOAuthFlowHandler(oauthFlowSvc, oauthAppSvc)
	gitlodeAppHandler := handlers.NewAppHandler(gitlodeAppSvc, orgSvc)
	feedbackHandler := handlers.NewFeedbackHandler(db)
	adminSystemHandler := handlers.NewAdminSystemHandler(db, repoSvc, cfg.AdminSystemPassword)
	webhookSvc := services.NewWebhookService(db)
	webhookHandler := handlers.NewWebhookHandler(webhookSvc, repoSvc)
	// Package (Container Registry) service and handler
	pkgSvc := services.NewPackageService(db, cfg.PackagesPath)
	if err := pkgSvc.EnsureDirs(); err != nil {
		log.Fatalf("failed to create packages dirs: %v", err)
	}
	registryHandler := handlers.NewRegistryHandler(pkgSvc, authSvc, orgSvc, cfg.AppURL)

	// Wire webhook service into handlers that need to fire events
	issueHandler.SetWebhookService(webhookSvc, repoSvc)
	prHandler.SetWebhookService(webhookSvc, repoSvc)

	// Echo
	e := echo.New()
	e.HideBanner = true

	// Download workflow artifacts as a zip archive (authentication required).
	e.GET("/artifact-files/*", func(c echo.Context) error {
		rawPath := c.Param("*")
		absPath := filepath.Join(cfg.WorkflowWorkspacePath, rawPath)

		// Prevent path traversal: resolved path must remain inside the workspace root.
		cleanBase := filepath.Clean(cfg.WorkflowWorkspacePath)
		cleanPath := filepath.Clean(absPath)
		if cleanPath != cleanBase && !strings.HasPrefix(cleanPath, cleanBase+string(os.PathSeparator)) {
			return c.String(http.StatusForbidden, "access denied")
		}

		// IDOR check: parse the run ID from the path ("artifacts/run{id}/...")
		// and verify the requesting user has read access to the owning repo.
		currentUser := c.Get("user").(*models.User)
		parts := strings.SplitN(rawPath, string(os.PathSeparator), 3)
		if len(parts) >= 2 && strings.HasPrefix(parts[1], "run") {
			runIDStr := strings.TrimPrefix(parts[1], "run")
			if runID, err := strconv.ParseUint(runIDStr, 10, 64); err == nil {
				var run models.WorkflowRun
				if err := db.First(&run, uint(runID)).Error; err != nil {
					return c.String(http.StatusNotFound, "not found")
				}
				var repo models.Repository
				if err := db.First(&repo, run.RepoID).Error; err != nil {
					return c.String(http.StatusNotFound, "not found")
				}
				if repo.IsPrivate && !repoSvc.HasAccess(&repo, currentUser.ID, false) {
					return c.String(http.StatusForbidden, "access denied")
				}
			}
		}

		info, err := os.Stat(cleanPath)
		if err != nil {
			return c.String(http.StatusNotFound, "file not found")
		}
		if info.IsDir() {
			// Zip all files in the directory and send as a blob
			safeName := filepath.Base(cleanPath) + ".zip"
			buf := new(bytes.Buffer)
			zw := zip.NewWriter(buf)

			files, _ := os.ReadDir(cleanPath)
			for _, f := range files {
				if f.IsDir() || strings.HasPrefix(f.Name(), ".") {
					continue
				}
				filePath := filepath.Join(cleanPath, f.Name())
				file, err := os.Open(filePath)
				if err != nil {
					continue
				}
				w, err := zw.Create(f.Name())
				if err != nil {
					file.Close()
					continue
				}
				io.Copy(w, file)
				file.Close()
			}
			zw.Close()
			c.Response().Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(safeName, `"`, `_`)+`"`)
			c.Response().Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			return c.Blob(http.StatusOK, "application/zip", buf.Bytes())
		}
		// If not a directory, just serve the file as before
		safeName := filepath.Base(cleanPath)
		c.Response().Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(safeName, `"`, `_`)+`"`)
		c.Response().Header().Set("Content-Type", "application/octet-stream")
		c.Response().Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		return c.File(cleanPath)
	}, apimiddleware.RequireAuth(authSvc))

	// Global middleware
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())
	e.Use(middleware.BodyLimit("220MB"))
	e.Use(jsonPayloadGuard(2<<20, 120)) // 2 MiB JSON bodies, max nesting depth 120

	// Security headers
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Response().Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", "geolocation=(), microphone=()")
			if strings.HasPrefix(cfg.AppURL, "https://") {
				h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			}
			h.Set("Content-Security-Policy",
				"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'self'")
			return next(c)
		}
	})

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     cfg.CORSOrigins,
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAuthorization, "X-Confirm-Password", "X-System-Admin-Password"},
		AllowCredentials: true,
	}))
	// Global rate limiter for regular API/UI traffic.
	// Skip OCI registry routes because Docker push/pull performs many rapid
	// HEAD/GET/POST/PATCH/PUT/DELETE calls that should not be throttled here.
	e.Use(middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: middleware.NewRateLimiterMemoryStore(20),
		Skipper: func(c echo.Context) bool {
			path := c.Request().URL.Path
			return strings.HasPrefix(path, "/v2/")
		},
	}))

	// Per-IP rate limiter for authentication endpoints (brute-force protection).
	// Allows a burst of 10 and refills at 1 request per 6 seconds (~10/min).
	authRateLimiter := middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(
			middleware.RateLimiterMemoryStoreConfig{
				Rate:      rate.Limit(1.0 / 6.0),
				Burst:     10,
				ExpiresIn: 15 * time.Minute,
			}),
		IdentifierExtractor: func(c echo.Context) (string, error) {
			return c.RealIP(), nil
		},
		ErrorHandler: func(c echo.Context, err error) error {
			return echo.NewHTTPError(http.StatusTooManyRequests, "too many requests — please wait before trying again")
		},
		DenyHandler: func(c echo.Context, identifier string, err error) error {
			return echo.NewHTTPError(http.StatusTooManyRequests, "too many requests — please wait before trying again")
		},
	})
	// Dedicated per-IP limiter for registry token issuance.
	// Docker clients can retry token requests quickly, so this allows short bursts
	// but still throttles brute-force credential attempts against /v2/token.
	registryTokenRateLimiter := middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(
			middleware.RateLimiterMemoryStoreConfig{
				Rate:      rate.Limit(1.0 / 3.0),
				Burst:     10,
				ExpiresIn: 10 * time.Minute,
			}),
		IdentifierExtractor: func(c echo.Context) (string, error) {
			return c.RealIP(), nil
		},
		ErrorHandler: func(c echo.Context, err error) error {
			return echo.NewHTTPError(http.StatusTooManyRequests, "too many requests - please wait before trying again")
		},
		DenyHandler: func(c echo.Context, identifier string, err error) error {
			return echo.NewHTTPError(http.StatusTooManyRequests, "too many requests - please wait before trying again")
		},
	})

	// Soft auth: extract user from JWT or OAuth token if present (doesn't fail if missing)
	e.Use(apimiddleware.Auth(authSvc, oauthFlowSvc))

	// Serve uploaded avatars as static files
	e.Static("/avatars", cfg.AvatarsPath)

	// --- API routes ---
	api := e.Group("/api/v1")
	api.Use(apimiddleware.RequireRepoWritable(repoSvc))

	// Auth
	api.GET("/auth/username-availability", authHandler.CheckUsernameAvailability, authRateLimiter)
	api.POST("/auth/register", authHandler.Register, authRateLimiter)
	api.POST("/auth/register/verify", authHandler.VerifyRegistrationOTP, authRateLimiter)
	api.POST("/auth/login", authHandler.Login, authRateLimiter)
	api.POST("/auth/logout", authHandler.Logout)
	api.GET("/auth/me", authHandler.Me, apimiddleware.RequireAuth(authSvc, oauthFlowSvc))
	api.GET("/auth/repo-creation", authHandler.RepoCreationStatus, apimiddleware.RequireAuth(authSvc))
	api.POST("/auth/verify-password", authHandler.VerifyPassword, apimiddleware.RequireAuth(authSvc))
	api.POST("/auth/change-password", authHandler.ChangePassword, apimiddleware.RequireAuth(authSvc), authRateLimiter)
	api.POST("/auth/forgot-password", authHandler.ForgotPassword, authRateLimiter)
	api.POST("/auth/reset-password", authHandler.ResetPassword, authRateLimiter)
	api.GET("/auth/sessions", authHandler.ListSessions, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/auth/sessions/:token_id", authHandler.RevokeSession, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/auth/sessions", authHandler.RevokeOtherSessions, apimiddleware.RequireAuth(authSvc))
	api.GET("/auth/2fa/status", authHandler.TwoFactorStatus, apimiddleware.RequireAuth(authSvc))
	api.POST("/auth/2fa/setup", authHandler.TwoFactorSetup, apimiddleware.RequireAuth(authSvc))
	api.POST("/auth/2fa/enable", authHandler.TwoFactorEnable, apimiddleware.RequireAuth(authSvc), authRateLimiter)
	api.POST("/auth/2fa/disable", authHandler.TwoFactorDisable, apimiddleware.RequireAuth(authSvc), authRateLimiter)
	api.POST("/auth/2fa/recovery-codes/regenerate", authHandler.TwoFactorRegenerateRecoveryCodes, apimiddleware.RequireAuth(authSvc), authRateLimiter)

	// Public feedback (optional auth)
	api.POST("/feedback", feedbackHandler.Create)

	// Explore
	api.GET("/explore", repoHandler.Explore)

	// Search
	api.GET("/search", searchHandler.Search)

	// System admin dashboard (password-based, no user session required)
	api.GET("/admin/system", adminSystemHandler.GetSystemStats, authRateLimiter)
	api.GET("/admin/systems", adminSystemHandler.GetSystemStats, authRateLimiter)

	// Users
	api.GET("/users/:username", userHandler.GetProfile)
	api.GET("/users/:username/followers", userHandler.ListFollowers)
	api.GET("/users/:username/following", userHandler.ListFollowing)
	api.POST("/users/:username/follow", userHandler.FollowUser, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/users/:username/follow", userHandler.UnfollowUser, apimiddleware.RequireAuth(authSvc))
	api.GET("/users/:username/contributions", userHandler.GetContributions)
	api.GET("/users/:username/stats", userHandler.GetProfileStats)
	api.GET("/users/:username/projects", projectHandler.ListUserProjects)
	api.GET("/users/:username/widgets", userHandler.GetProfileWidgets)
	api.PATCH("/users/me", userHandler.UpdateProfile, apimiddleware.RequireAuth(authSvc))
	api.PATCH("/users/me/widgets", userHandler.UpdateProfileWidgets, apimiddleware.RequireAuth(authSvc))
	api.POST("/users/me/avatar", avatarHandler.UploadUserAvatar, apimiddleware.RequireAuth(authSvc))
	api.GET("/users/me/actions/usage", userHandler.GetActionsUsage, apimiddleware.RequireAuth(authSvc))
	api.GET("/users/me/dashboard", userHandler.GetDashboard, apimiddleware.RequireAuth(authSvc))
	api.GET("/users/me/export", userHandler.ExportData, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/users/me", userHandler.DeleteAccount, apimiddleware.RequireAuth(authSvc))
	api.POST("/users/me/projects", projectHandler.CreateUserProject, apimiddleware.RequireAuth(authSvc))

	// Projects
	api.GET("/projects/:id", projectHandler.GetProject)
	api.PATCH("/projects/:id", projectHandler.UpdateProject, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/projects/:id", projectHandler.DeleteProject, apimiddleware.RequireAuth(authSvc))
	api.POST("/projects/:id/columns", projectHandler.CreateColumn, apimiddleware.RequireAuth(authSvc))
	api.PATCH("/projects/:id/columns/:columnID", projectHandler.UpdateColumn, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/projects/:id/columns/:columnID", projectHandler.DeleteColumn, apimiddleware.RequireAuth(authSvc))
	api.POST("/projects/:id/items", projectHandler.CreateItem, apimiddleware.RequireAuth(authSvc))
	api.PATCH("/projects/:id/items/:itemID", projectHandler.UpdateItem, apimiddleware.RequireAuth(authSvc))
	api.POST("/projects/:id/items/:itemID/move", projectHandler.MoveItem, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/projects/:id/items/:itemID", projectHandler.DeleteItem, apimiddleware.RequireAuth(authSvc))

	// Repositories (CRUD)
	api.POST("/repos", repoHandler.Create, apimiddleware.RequireAuth(authSvc))
	api.GET("/repos/:username/:repo", repoHandler.Get)
	api.POST("/repos/:username/:repo/fork", repoHandler.Fork, apimiddleware.RequireAuth(authSvc))
	api.POST("/repos/:username/:repo/fork/sync", repoHandler.SyncFork, apimiddleware.RequireAuth(authSvc))
	api.GET("/repos/:username/:repo/forks", repoHandler.ListForks)
	api.PATCH("/repos/:username/:repo", repoHandler.Update, apimiddleware.RequireAuth(authSvc))
	api.POST("/repos/:username/:repo/archive", repoHandler.Archive, apimiddleware.RequireAuth(authSvc), apimiddleware.RequirePasswordVerification(authSvc))
	api.POST("/repos/:username/:repo/unarchive", repoHandler.Unarchive, apimiddleware.RequireAuth(authSvc), apimiddleware.RequirePasswordVerification(authSvc))
	api.POST("/repos/:username/:repo/visibility", repoHandler.SetVisibility, apimiddleware.RequireAuth(authSvc), apimiddleware.RequirePasswordVerification(authSvc))
	api.DELETE("/repos/:username/:repo", repoHandler.Delete, apimiddleware.RequireAuth(authSvc), apimiddleware.RequirePasswordVerification(authSvc))

	// Repository contents
	api.GET("/repos/:username/:repo/tree", repoHandler.ListTree)
	api.GET("/repos/:username/:repo/blob", repoHandler.GetBlob)
	api.PUT("/repos/:username/:repo/blob", repoHandler.UpdateBlob, apimiddleware.RequireAuth(authSvc))
	api.GET("/repos/:username/:repo/raw", repoHandler.GetRaw)
	api.GET("/repos/:username/:repo/zip", repoHandler.DownloadZip)
	api.GET("/repos/:username/:repo/commit/:sha", repoHandler.GetCommit)
	api.GET("/repos/:username/:repo/commit/:sha/diffs", repoHandler.GetCommitDiffs)
	api.GET("/repos/:username/:repo/commits", repoHandler.GetCommits)
	api.GET("/repos/:username/:repo/branches", repoHandler.GetBranches)
	api.POST("/repos/:username/:repo/branches", repoHandler.CreateBranch, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/repos/:username/:repo/branches", repoHandler.DeleteBranch, apimiddleware.RequireAuth(authSvc))
	api.GET("/repos/:username/:repo/tags", repoHandler.GetTags)
	api.POST("/repos/:username/:repo/tags", repoHandler.CreateTag, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/repos/:username/:repo/tags", repoHandler.DeleteTag, apimiddleware.RequireAuth(authSvc))
	api.GET("/repos/:username/:repo/compare", repoHandler.Compare)
	api.GET("/repos/:username/:repo/languages", repoHandler.GetLanguages)
	api.GET("/repos/:username/:repo/markdown-assets/:asset", markdownAssetHandler.Download)
	api.POST("/repos/:username/:repo/markdown-assets", markdownAssetHandler.Upload, apimiddleware.RequireAuth(authSvc))

	// Collaborators
	api.GET("/repos/:username/:repo/collaborators", repoHandler.ListCollaborators, apimiddleware.RequireAuth(authSvc))
	api.POST("/repos/:username/:repo/collaborators", repoHandler.AddCollaborator, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/repos/:username/:repo/collaborators/:userID", repoHandler.RemoveCollaborator, apimiddleware.RequireAuth(authSvc))

	// Stars
	api.GET("/repos/:username/:repo/star", repoHandler.GetStarringStatus)
	api.GET("/repos/:username/:repo/stars/history", repoHandler.GetStarHistory)
	api.POST("/repos/:username/:repo/star", repoHandler.Star, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/repos/:username/:repo/star", repoHandler.Unstar, apimiddleware.RequireAuth(authSvc))
	api.GET("/users/me/starred", repoHandler.GetStarredRepos, apimiddleware.RequireAuth(authSvc))
	api.GET("/users/:username/starred", repoHandler.GetUserStarredRepos)

	// Pull Requests
	api.GET("/repos/:username/:repo/pulls", prHandler.List)
	api.POST("/repos/:username/:repo/pulls", prHandler.Create, apimiddleware.RequireAuth(authSvc))
	api.GET("/repos/:username/:repo/pulls/:number", prHandler.Get)
	api.PATCH("/repos/:username/:repo/pulls/:number", prHandler.Update, apimiddleware.RequireAuth(authSvc))
	api.POST("/repos/:username/:repo/pulls/:number/close", prHandler.Close, apimiddleware.RequireAuth(authSvc))
	api.POST("/repos/:username/:repo/pulls/:number/reopen", prHandler.Reopen, apimiddleware.RequireAuth(authSvc))
	api.POST("/repos/:username/:repo/pulls/:number/merge", prHandler.Merge, apimiddleware.RequireAuth(authSvc))
	api.GET("/repos/:username/:repo/pulls/:number/commits", prHandler.GetCommits)
	api.GET("/repos/:username/:repo/pulls/:number/files", prHandler.GetFiles)
	api.GET("/repos/:username/:repo/pulls/:number/comments", prHandler.ListComments)
	api.POST("/repos/:username/:repo/pulls/:number/comments", prHandler.CreateComment, apimiddleware.RequireAuth(authSvc))
	api.PATCH("/repos/:username/:repo/pulls/:number/comments/:commentID", prHandler.UpdateComment, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/repos/:username/:repo/pulls/:number/comments/:commentID", prHandler.DeleteComment, apimiddleware.RequireAuth(authSvc))
	api.GET("/repos/:username/:repo/pulls/:number/reviews", prHandler.ListReviews)
	api.POST("/repos/:username/:repo/pulls/:number/reviews", prHandler.CreateReview, apimiddleware.RequireAuth(authSvc))

	// Workflow Actions
	api.GET("/repos/:username/:repo/actions", workflowHandler.ListRuns)
	api.GET("/repos/:username/:repo/actions/usage", workflowHandler.GetUsage, apimiddleware.RequireAuth(authSvc))
	api.GET("/repos/:username/:repo/actions/dispatchable", workflowHandler.ListDispatchable, apimiddleware.RequireAuth(authSvc))
	api.GET("/repos/:username/:repo/actions/:runID", workflowHandler.GetRun)
	api.DELETE("/repos/:username/:repo/actions/:runID", workflowHandler.DeleteRun, apimiddleware.RequireAuth(authSvc))
	api.POST("/repos/:username/:repo/actions/:runID/cancel", workflowHandler.CancelRun, apimiddleware.RequireAuth(authSvc))
	api.POST("/repos/:username/:repo/actions/:runID/rerun", workflowHandler.RerunWorkflow, apimiddleware.RequireAuth(authSvc))
	api.POST("/repos/:username/:repo/actions/dispatch", workflowHandler.DispatchRun, apimiddleware.RequireAuth(authSvc))

	// Repository Variables (Actions)
	api.GET("/repos/:username/:repo/actions/variables", repoEnvHandler.ListVariables, apimiddleware.RequireAuth(authSvc))
	api.PUT("/repos/:username/:repo/actions/variables/:name", repoEnvHandler.SetVariable, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/repos/:username/:repo/actions/variables/:name", repoEnvHandler.DeleteVariable, apimiddleware.RequireAuth(authSvc))

	// Repository Secrets (Actions) — values are write-only, never returned
	api.GET("/repos/:username/:repo/actions/secrets", repoEnvHandler.ListSecrets, apimiddleware.RequireAuth(authSvc))
	api.PUT("/repos/:username/:repo/actions/secrets/:name", repoEnvHandler.SetSecret, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/repos/:username/:repo/actions/secrets/:name", repoEnvHandler.DeleteSecret, apimiddleware.RequireAuth(authSvc))

	// Releases
	api.GET("/repos/:username/:repo/releases", releaseHandler.List)
	api.POST("/repos/:username/:repo/releases", releaseHandler.Create, apimiddleware.RequireAuth(authSvc))
	api.GET("/repos/:username/:repo/releases/latest", releaseHandler.GetLatest)
	api.GET("/repos/:username/:repo/releases/tags/:tag", releaseHandler.GetByTag)
	api.GET("/repos/:username/:repo/releases/tags", releaseHandler.GetTags)
	api.GET("/repos/:username/:repo/releases/assets/:assetId", releaseHandler.DownloadAsset)
	api.GET("/repos/:username/:repo/releases/:id", releaseHandler.GetByID)
	api.PATCH("/repos/:username/:repo/releases/:id", releaseHandler.Update, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/repos/:username/:repo/releases/:id", releaseHandler.Delete, apimiddleware.RequireAuth(authSvc))
	api.POST("/repos/:username/:repo/releases/:id/assets", releaseHandler.UploadAsset, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/repos/:username/:repo/releases/:id/assets/:assetId", releaseHandler.DeleteAsset, apimiddleware.RequireAuth(authSvc))
	api.GET("/repos/:username/:repo/releases/:id/source.:format", releaseHandler.DownloadSource)

	// Issues
	api.GET("/repos/:username/:repo/issues", issueHandler.List)
	api.POST("/repos/:username/:repo/issues", issueHandler.Create, apimiddleware.RequireAuth(authSvc))
	api.GET("/repos/:username/:repo/issues/:number", issueHandler.Get)
	api.PATCH("/repos/:username/:repo/issues/:number", issueHandler.Update, apimiddleware.RequireAuth(authSvc))
	api.POST("/repos/:username/:repo/issues/:number/close", issueHandler.Close, apimiddleware.RequireAuth(authSvc))
	api.POST("/repos/:username/:repo/issues/:number/reopen", issueHandler.Reopen, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/repos/:username/:repo/issues/:number", issueHandler.Delete, apimiddleware.RequireAuth(authSvc))
	api.GET("/repos/:username/:repo/issues/:number/comments", issueHandler.ListComments)
	api.POST("/repos/:username/:repo/issues/:number/comments", issueHandler.CreateComment, apimiddleware.RequireAuth(authSvc))
	api.PATCH("/repos/:username/:repo/issues/:number/comments/:commentID", issueHandler.UpdateComment, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/repos/:username/:repo/issues/:number/comments/:commentID", issueHandler.DeleteComment, apimiddleware.RequireAuth(authSvc))
	api.PUT("/repos/:username/:repo/issues/:number/labels", issueHandler.SetLabels, apimiddleware.RequireAuth(authSvc))

	// Labels
	api.GET("/repos/:username/:repo/labels", issueHandler.ListLabels)
	api.POST("/repos/:username/:repo/labels", issueHandler.CreateLabel, apimiddleware.RequireAuth(authSvc))
	api.PATCH("/repos/:username/:repo/labels/:labelID", issueHandler.UpdateLabel, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/repos/:username/:repo/labels/:labelID", issueHandler.DeleteLabel, apimiddleware.RequireAuth(authSvc))

	// Milestones
	api.GET("/repos/:username/:repo/milestones", issueHandler.ListMilestones)
	api.POST("/repos/:username/:repo/milestones", issueHandler.CreateMilestone, apimiddleware.RequireAuth(authSvc))
	api.PATCH("/repos/:username/:repo/milestones/:milestoneID", issueHandler.UpdateMilestone, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/repos/:username/:repo/milestones/:milestoneID", issueHandler.DeleteMilestone, apimiddleware.RequireAuth(authSvc))

	// Webhooks
	api.GET("/repos/:username/:repo/hooks", webhookHandler.List, apimiddleware.RequireAuth(authSvc))
	api.POST("/repos/:username/:repo/hooks", webhookHandler.Create, apimiddleware.RequireAuth(authSvc))
	api.GET("/repos/:username/:repo/hooks/:id", webhookHandler.Get, apimiddleware.RequireAuth(authSvc))
	api.PATCH("/repos/:username/:repo/hooks/:id", webhookHandler.Update, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/repos/:username/:repo/hooks/:id", webhookHandler.Delete, apimiddleware.RequireAuth(authSvc))
	api.GET("/repos/:username/:repo/hooks/:id/deliveries", webhookHandler.ListDeliveries, apimiddleware.RequireAuth(authSvc))
	api.GET("/repos/:username/:repo/hooks/:id/deliveries/:deliveryID", webhookHandler.GetDelivery, apimiddleware.RequireAuth(authSvc))
	api.POST("/repos/:username/:repo/hooks/:id/deliveries/:deliveryID/redeliver", webhookHandler.Redeliver, apimiddleware.RequireAuth(authSvc))

	// OAuth Apps (developer settings — user-owned)
	api.GET("/users/me/oauth-apps", oauthAppHandler.ListUserApps, apimiddleware.RequireAuth(authSvc))
	api.POST("/users/me/oauth-apps", oauthAppHandler.CreateUserApp, apimiddleware.RequireAuth(authSvc))

	// OAuth Apps (developer settings — org-owned)
	api.GET("/orgs/:orgname/oauth-apps", oauthAppHandler.ListOrgApps, apimiddleware.RequireAuth(authSvc))
	api.POST("/orgs/:orgname/oauth-apps", oauthAppHandler.CreateOrgApp, apimiddleware.RequireAuth(authSvc))

	// OAuth Apps (shared CRUD by app ID)
	api.GET("/oauth-apps/:id", oauthAppHandler.GetApp, apimiddleware.RequireAuth(authSvc))
	api.PATCH("/oauth-apps/:id", oauthAppHandler.UpdateApp, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/oauth-apps/:id", oauthAppHandler.DeleteApp, apimiddleware.RequireAuth(authSvc))
	api.POST("/oauth-apps/:id/regenerate-secret", oauthAppHandler.RegenerateSecret, apimiddleware.RequireAuth(authSvc))

	// Authorized OAuth Apps (apps the current user has authorized)
	api.GET("/users/me/authorized-apps", oauthAppHandler.ListAuthorizedApps, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/users/me/authorized-apps/:id", oauthAppHandler.RevokeAuthorization, apimiddleware.RequireAuth(authSvc))

	// OAuth 2.0 flow — consent page data + authorization submission (user must be logged in)
	api.GET("/oauth/app-info", oauthFlowHandler.GetAppInfo)
	api.POST("/oauth/authorize", oauthFlowHandler.Authorize, apimiddleware.RequireAuth(authSvc, oauthFlowSvc))
	api.GET("/oauth/device-info", oauthFlowHandler.GetDeviceInfo)
	api.POST("/oauth/device/approve", oauthFlowHandler.ApproveDevice, apimiddleware.RequireAuth(authSvc, oauthFlowSvc))
	api.POST("/oauth/device/deny", oauthFlowHandler.DenyDevice, apimiddleware.RequireAuth(authSvc, oauthFlowSvc))

	// OAuth 2.0 flow — token endpoints (called by third-party apps, outside /api/v1 to match GitHub)
	e.POST("/login/oauth/access_token", oauthFlowHandler.AccessToken)
	e.POST("/login/device/code", oauthFlowHandler.DeviceCode)

	// SSH Keys
	api.GET("/ssh-keys", sshKeyHandler.List, apimiddleware.RequireAuth(authSvc))
	api.POST("/ssh-keys", sshKeyHandler.Add, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/ssh-keys/:id", sshKeyHandler.Delete, apimiddleware.RequireAuth(authSvc), apimiddleware.RequirePasswordVerification(authSvc))

	// Organizations
	api.POST("/orgs", orgHandler.Create, apimiddleware.RequireAuth(authSvc))
	api.GET("/orgs/:orgname", orgHandler.Get)
	api.GET("/orgs/:orgname/followers", orgHandler.ListFollowers)
	api.POST("/orgs/:orgname/follow", orgHandler.Follow, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/orgs/:orgname/follow", orgHandler.Unfollow, apimiddleware.RequireAuth(authSvc))
	api.PATCH("/orgs/:orgname", orgHandler.Update, apimiddleware.RequireAuth(authSvc))
	api.GET("/orgs/:orgname/actions/usage", orgHandler.GetActionsUsage, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/orgs/:orgname", orgHandler.Delete, apimiddleware.RequireAuth(authSvc), apimiddleware.RequirePasswordVerification(authSvc))
	api.POST("/orgs/:orgname/avatar", avatarHandler.UploadOrgAvatar, apimiddleware.RequireAuth(authSvc))

	// Org members
	api.GET("/orgs/:orgname/members", orgHandler.ListMembers)
	api.POST("/orgs/:orgname/members", orgHandler.AddMember, apimiddleware.RequireAuth(authSvc))
	api.PATCH("/orgs/:orgname/members/:username", orgHandler.UpdateMemberRole, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/orgs/:orgname/members/:username", orgHandler.RemoveMember, apimiddleware.RequireAuth(authSvc))

	// Org teams
	api.GET("/orgs/:orgname/teams", orgHandler.ListTeams)
	api.POST("/orgs/:orgname/teams", orgHandler.CreateTeam, apimiddleware.RequireAuth(authSvc))
	api.GET("/orgs/:orgname/teams/:teamID", orgHandler.GetTeam)
	api.PATCH("/orgs/:orgname/teams/:teamID", orgHandler.UpdateTeam, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/orgs/:orgname/teams/:teamID", orgHandler.DeleteTeam, apimiddleware.RequireAuth(authSvc), apimiddleware.RequirePasswordVerification(authSvc))

	// Team members
	api.GET("/orgs/:orgname/teams/:teamID/members", orgHandler.ListTeamMembers)
	api.POST("/orgs/:orgname/teams/:teamID/members", orgHandler.AddTeamMember, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/orgs/:orgname/teams/:teamID/members/:username", orgHandler.RemoveTeamMember, apimiddleware.RequireAuth(authSvc))

	// Team repos
	api.GET("/orgs/:orgname/teams/:teamID/repos", orgHandler.ListTeamRepos)
	api.POST("/orgs/:orgname/teams/:teamID/repos", orgHandler.AddTeamRepo, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/orgs/:orgname/teams/:teamID/repos/:repoID", orgHandler.RemoveTeamRepo, apimiddleware.RequireAuth(authSvc))

	// Org repos
	api.GET("/orgs/:orgname/repos", orgHandler.ListRepos)
	api.POST("/orgs/:orgname/repos", orgHandler.CreateRepo, apimiddleware.RequireAuth(authSvc))
	api.GET("/orgs/:orgname/projects", projectHandler.ListOrgProjects)
	api.POST("/orgs/:orgname/projects", projectHandler.CreateOrgProject, apimiddleware.RequireAuth(authSvc))

	// User orgs (me must come before :username to avoid conflict)
	api.GET("/users/me/orgs", orgHandler.ListMyOrgs, apimiddleware.RequireAuth(authSvc))
	api.GET("/users/:username/orgs", orgHandler.ListUserOrgs)

	// ── Moderation ────────────────────────────────────────────────────────────

	// User-level moderation policy
	api.GET("/users/me/moderation", modHandler.GetUserPolicy, apimiddleware.RequireAuth(authSvc))
	api.PUT("/users/me/moderation", modHandler.UpdateUserPolicy, apimiddleware.RequireAuth(authSvc))
	api.POST("/users/me/moderation/blocked-users", modHandler.UserBlockUser, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/users/me/moderation/blocked-users/:userID", modHandler.UserUnblockUser, apimiddleware.RequireAuth(authSvc))
	api.POST("/users/me/moderation/keywords", modHandler.UserAddKeyword, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/users/me/moderation/keywords/:keywordID", modHandler.UserRemoveKeyword, apimiddleware.RequireAuth(authSvc))

	// Org-level moderation policy
	api.GET("/orgs/:orgname/moderation", modHandler.GetOrgPolicy, apimiddleware.RequireAuth(authSvc))
	api.PUT("/orgs/:orgname/moderation", modHandler.UpdateOrgPolicy, apimiddleware.RequireAuth(authSvc))
	api.POST("/orgs/:orgname/moderation/blocked-users", modHandler.OrgBlockUser, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/orgs/:orgname/moderation/blocked-users/:userID", modHandler.OrgUnblockUser, apimiddleware.RequireAuth(authSvc))
	api.POST("/orgs/:orgname/moderation/keywords", modHandler.OrgAddKeyword, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/orgs/:orgname/moderation/keywords/:keywordID", modHandler.OrgRemoveKeyword, apimiddleware.RequireAuth(authSvc))

	// Repo-level moderation policy
	api.GET("/repos/:username/:repo/moderation", modHandler.GetRepoPolicy, apimiddleware.RequireAuth(authSvc))
	api.PUT("/repos/:username/:repo/moderation", modHandler.UpdateRepoPolicy, apimiddleware.RequireAuth(authSvc))
	api.POST("/repos/:username/:repo/moderation/blocked-users", modHandler.RepoBlockUser, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/repos/:username/:repo/moderation/blocked-users/:userID", modHandler.RepoUnblockUser, apimiddleware.RequireAuth(authSvc))
	api.POST("/repos/:username/:repo/moderation/keywords", modHandler.RepoAddKeyword, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/repos/:username/:repo/moderation/keywords/:keywordID", modHandler.RepoRemoveKeyword, apimiddleware.RequireAuth(authSvc))

	// ── Container Packages (REST listing) ────────────────────────────────────
	api.GET("/packages/:namespace/:image", registryHandler.GetPackage)
	api.PATCH("/packages/:namespace/:image", registryHandler.UpdatePackage, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/packages/:namespace/:image", registryHandler.DeletePackage, apimiddleware.RequireAuth(authSvc))
	api.GET("/packages/:namespace", registryHandler.ListPackages)

	// ── OCI Distribution Spec v2 (Docker / container registry) ───────────────
	// Token endpoint: Docker clients exchange Basic creds for a short-lived JWT.
	e.GET("/v2/token", registryHandler.Token, registryTokenRateLimiter)
	// API version check
	e.GET("/v2/", registryHandler.CheckAPI)
	// Blobs
	e.HEAD("/v2/:namespace/:image/blobs/:digest", registryHandler.HeadBlob)
	e.GET("/v2/:namespace/:image/blobs/:digest", registryHandler.GetBlob)
	e.DELETE("/v2/:namespace/:image/blobs/:digest", registryHandler.DeleteBlob)
	// Blob uploads
	e.POST("/v2/:namespace/:image/blobs/uploads/", registryHandler.StartBlobUpload)
	e.PATCH("/v2/:namespace/:image/blobs/uploads/:uuid", registryHandler.PatchBlobUpload)
	e.PUT("/v2/:namespace/:image/blobs/uploads/:uuid", registryHandler.PutBlobUpload)
	// Manifests
	e.HEAD("/v2/:namespace/:image/manifests/:reference", registryHandler.HeadManifest)
	e.GET("/v2/:namespace/:image/manifests/:reference", registryHandler.GetManifest)
	e.PUT("/v2/:namespace/:image/manifests/:reference", registryHandler.PutManifest)
	e.DELETE("/v2/:namespace/:image/manifests/:reference", registryHandler.DeleteManifest)
	// Tags
	e.GET("/v2/:namespace/:image/tags/list", registryHandler.ListTags)

	// ── GitLode Apps ──────────────────────────────────────────────────────────

	// Developer: manage apps you own (user-owned)
	api.GET("/users/me/apps", gitlodeAppHandler.ListUserApps, apimiddleware.RequireAuth(authSvc))
	api.POST("/users/me/apps", gitlodeAppHandler.CreateUserApp, apimiddleware.RequireAuth(authSvc))

	// Developer: manage apps you own (org-owned)
	api.GET("/orgs/:orgname/apps", gitlodeAppHandler.ListOrgApps, apimiddleware.RequireAuth(authSvc))
	api.POST("/orgs/:orgname/apps", gitlodeAppHandler.CreateOrgApp, apimiddleware.RequireAuth(authSvc))

	// Developer: shared CRUD by app ID
	api.GET("/apps/:id", gitlodeAppHandler.GetApp, apimiddleware.RequireAuth(authSvc))
	api.PATCH("/apps/:id", gitlodeAppHandler.UpdateApp, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/apps/:id", gitlodeAppHandler.DeleteApp, apimiddleware.RequireAuth(authSvc))
	api.POST("/apps/:id/regenerate-secret", gitlodeAppHandler.RegenerateClientSecret, apimiddleware.RequireAuth(authSvc))

	// Developer: private key management
	api.GET("/apps/:id/keys", gitlodeAppHandler.ListKeys, apimiddleware.RequireAuth(authSvc))
	api.POST("/apps/:id/keys", gitlodeAppHandler.GenerateKey, apimiddleware.RequireAuth(authSvc))
	api.DELETE("/apps/:id/keys/:keyID", gitlodeAppHandler.DeleteKey, apimiddleware.RequireAuth(authSvc))

	// Developer: view installations of your app
	api.GET("/apps/:id/installations", gitlodeAppHandler.ListAppInstallations, apimiddleware.RequireAuth(authSvc))

	// Public: app info by slug (no auth required)
	api.GET("/apps/slug/:slug", gitlodeAppHandler.GetPublicApp)

	// User/org-facing: install / uninstall apps
	api.POST("/apps/slug/:slug/install", gitlodeAppHandler.InstallApp, apimiddleware.RequireAuth(authSvc, oauthFlowSvc))

	// Installations management
	api.GET("/installations/:installationID", gitlodeAppHandler.GetInstallation, apimiddleware.RequireAuth(authSvc, oauthFlowSvc))
	api.DELETE("/installations/:installationID", gitlodeAppHandler.UninstallApp, apimiddleware.RequireAuth(authSvc, oauthFlowSvc))
	api.PATCH("/installations/:installationID/repositories", gitlodeAppHandler.UpdateInstallationRepos, apimiddleware.RequireAuth(authSvc, oauthFlowSvc))
	api.PATCH("/installations/:installationID/permissions", gitlodeAppHandler.SyncInstallationPermissions, apimiddleware.RequireAuth(authSvc, oauthFlowSvc))
	api.PUT("/installations/:installationID/suspended", gitlodeAppHandler.SuspendInstallation, apimiddleware.RequireAuth(authSvc, oauthFlowSvc))
	api.DELETE("/installations/:installationID/suspended", gitlodeAppHandler.UnsuspendInstallation, apimiddleware.RequireAuth(authSvc, oauthFlowSvc))

	// User's installed apps
	api.GET("/users/me/installations", gitlodeAppHandler.ListUserInstallations, apimiddleware.RequireAuth(authSvc, oauthFlowSvc))
	api.GET("/orgs/:orgname/installations", gitlodeAppHandler.ListOrgInstallations, apimiddleware.RequireAuth(authSvc))

	// App-to-server: create installation access token (JWT auth, no user session required)
	// This endpoint is outside /api/v1 to match the GitHub convention.
	e.POST("/api/v1/app/installations/:installationID/access_tokens", gitlodeAppHandler.CreateInstallationToken)

	// Health check endpoint for Kubernetes liveness / readiness probes.
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// SSH server
	sshSrv := sshserver.NewServer(cfg, authSvc, repoSvc, gitSvc)
	sshSrv.SetWorkflowService(workflowSvc)
	sshSrv.SetWebhookService(webhookSvc)
	go func() {
		if err := sshSrv.ListenAndServe(fmt.Sprintf(":%s", cfg.SSHPort)); err != nil {
			log.Printf("SSH server error: %v", err)
		}
	}()

	// Graceful shutdown
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := e.Shutdown(ctx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	log.Printf("HTTP server starting on :%s", cfg.Port)
	if err := e.Start(fmt.Sprintf(":%s", cfg.Port)); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

// jsonPayloadGuard mitigates request-body DoS by bounding JSON size and nesting depth.
func jsonPayloadGuard(maxBytes int64, maxDepth int) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			if req == nil || req.Body == nil {
				return next(c)
			}

			contentType := strings.ToLower(req.Header.Get(echo.HeaderContentType))
			if !strings.Contains(contentType, "application/json") {
				return next(c)
			}

			body, err := io.ReadAll(io.LimitReader(req.Body, maxBytes+1))
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
			}
			_ = req.Body.Close()
			if int64(len(body)) > maxBytes {
				return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "JSON payload is too large")
			}

			trimmed := bytes.TrimSpace(body)
			if len(trimmed) > 0 {
				dec := json.NewDecoder(bytes.NewReader(trimmed))
				depth := 0
				for {
					tok, err := dec.Token()
					if err == io.EOF {
						break
					}
					if err != nil {
						return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON payload")
					}

					delim, ok := tok.(json.Delim)
					if !ok {
						continue
					}

					switch delim {
					case '{', '[':
						depth++
						if depth > maxDepth {
							return echo.NewHTTPError(http.StatusBadRequest, "JSON payload nesting is too deep")
						}
					case '}', ']':
						if depth > 0 {
							depth--
						}
					}
				}
			}

			req.Body = io.NopCloser(bytes.NewReader(body))
			return next(c)
		}
	}
}
