package handlers

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"regexp"
	"strings"

	"gitpier/internal/config"
	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

var gitHTTPNameRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,98}[A-Za-z0-9])?$|^[A-Za-z0-9]$`)

type GitHTTPHandler struct {
	cfg      *config.Config
	repoSvc  *services.RepoService
	gitSvc   *services.GitService
	tokenSvc *services.PersonalAccessTokenService

	workflowSvc *services.WorkflowService
	webhookSvc  *services.WebhookService
}

type gitHTTPAuthResult struct {
	user   *models.User
	scopes []string
}

func NewGitHTTPHandler(cfg *config.Config, repoSvc *services.RepoService, gitSvc *services.GitService, tokenSvc *services.PersonalAccessTokenService) *GitHTTPHandler {
	return &GitHTTPHandler{
		cfg:      cfg,
		repoSvc:  repoSvc,
		gitSvc:   gitSvc,
		tokenSvc: tokenSvc,
	}
}

func (h *GitHTTPHandler) SetWorkflowService(svc *services.WorkflowService) {
	h.workflowSvc = svc
}

func (h *GitHTTPHandler) SetWebhookService(svc *services.WebhookService) {
	h.webhookSvc = svc
}

func (h *GitHTTPHandler) InfoRefs(c echo.Context) error {
	service := c.QueryParam("service")
	if service != "git-upload-pack" && service != "git-receive-pack" {
		return echo.NewHTTPError(http.StatusBadRequest, "unsupported git service")
	}

	_, owner, repoName, auth, err := h.authorize(c, service == "git-receive-pack")
	if err != nil {
		return err
	}

	repoPath := h.repoSvc.RepoPath(owner, repoName)
	cmd := exec.Command(service, "--stateless-rpc", "--advertise-refs", repoPath)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, runErr := cmd.Output()
	if runErr != nil {
		log.Printf("git http advertise failed repo=%s/%s service=%s user=%s: %v %s", owner, repoName, service, authUsername(auth), runErr, strings.TrimSpace(stderr.String()))
		return echo.NewHTTPError(http.StatusInternalServerError, "git service failed")
	}

	c.Response().Header().Set(echo.HeaderContentType, fmt.Sprintf("application/x-%s-advertisement", service))
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().WriteHeader(http.StatusOK)
	if _, err := c.Response().Write(packetLine("# service=" + service + "\n")); err != nil {
		return err
	}
	if _, err := c.Response().Write([]byte("0000")); err != nil {
		return err
	}
	_, err = c.Response().Write(out)
	return err
}

func (h *GitHTTPHandler) UploadPack(c echo.Context) error {
	return h.rpc(c, "git-upload-pack", false)
}

func (h *GitHTTPHandler) ReceivePack(c echo.Context) error {
	return h.rpc(c, "git-receive-pack", true)
}

func (h *GitHTTPHandler) rpc(c echo.Context, service string, write bool) error {
	repo, owner, repoName, auth, err := h.authorize(c, write)
	if err != nil {
		return err
	}

	repoPath := h.repoSvc.RepoPath(owner, repoName)
	if write {
		if sizeStatus, err := h.repoSvc.CheckSizeLimit(repo, repoPath); err != nil {
			return echo.NewHTTPError(http.StatusForbidden, fmt.Sprintf("repository size check failed: %v", err))
		} else if sizeStatus != "" {
			c.Response().Header().Set("X-GitPier-Repo-Size", sizeStatus)
		}
	}

	var oldRefs map[string]string
	if write && (h.workflowSvc != nil || h.webhookSvc != nil) {
		oldRefs, _ = h.gitSvc.GetAllRefs(repoPath)
	}

	cmd := exec.Command(service, "--stateless-rpc", repoPath)
	cmd.Stdin = c.Request().Body
	cmd.Stdout = c.Response().Writer

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	c.Response().Header().Set(echo.HeaderContentType, fmt.Sprintf("application/x-%s-result", service))
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().WriteHeader(http.StatusOK)

	exitCode := 0
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
		log.Printf("git http rpc failed repo=%s/%s service=%s user=%s exit=%d: %s", owner, repoName, service, authUsername(auth), exitCode, strings.TrimSpace(stderr.String()))
	}

	if write && exitCode == 0 {
		capturedRepo := repo
		capturedOwner := owner
		capturedRepoName := repoName
		capturedRepoPath := repoPath
		capturedOldRefs := oldRefs
		capturedUser := auth.user
		go services.HandleSuccessfulPush(
			context.Background(),
			h.gitSvc,
			h.repoSvc,
			h.workflowSvc,
			h.webhookSvc,
			capturedRepo,
			capturedOwner,
			capturedRepoName,
			capturedRepoPath,
			h.cfg.AppURL,
			capturedUser.Username,
			capturedUser.Email,
			capturedUser.ID,
			capturedOldRefs,
		)
	}

	return nil
}

func (h *GitHTTPHandler) authorize(c echo.Context, write bool) (*models.Repository, string, string, *gitHTTPAuthResult, error) {
	owner := strings.TrimSpace(c.Param("owner"))
	repoPathSegment := strings.TrimSpace(c.Param("repoWithGit"))
	if !strings.HasSuffix(repoPathSegment, ".git") {
		return nil, "", "", nil, echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	repoName := strings.TrimSuffix(repoPathSegment, ".git")
	if !isValidGitHTTPComponent(owner) || !isValidGitHTTPComponent(repoName) {
		return nil, "", "", nil, echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), owner, repoName)
	if err != nil {
		return nil, "", "", nil, echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	auth, authErr := h.authenticate(c)
	if write || repo.IsPrivate {
		if authErr != nil {
			return nil, "", "", nil, h.requireGitAuth(c)
		}
	}

	if write {
		if !services.HasPATScope(auth.scopes, services.PATScopeRepoWrite) {
			return nil, "", "", nil, echo.NewHTTPError(http.StatusForbidden, "token does not allow repository writes")
		}
		if repo.IsArchived {
			return nil, "", "", nil, echo.NewHTTPError(http.StatusForbidden, "repository is archived and read-only")
		}
		if !h.repoSvc.HasAccess(repo, auth.user.ID, true) {
			return nil, "", "", nil, echo.NewHTTPError(http.StatusForbidden, "access denied")
		}
		return repo, owner, repoName, auth, nil
	}

	if auth != nil && !services.HasPATScope(auth.scopes, services.PATScopeRepoRead) {
		return nil, "", "", nil, echo.NewHTTPError(http.StatusForbidden, "token does not allow repository reads")
	}
	if repo.IsPrivate && !h.repoSvc.HasAccess(repo, auth.user.ID, false) {
		return nil, "", "", nil, echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	return repo, owner, repoName, auth, nil
}

func (h *GitHTTPHandler) authenticate(c echo.Context) (*gitHTTPAuthResult, error) {
	token := ""
	authHeader := c.Request().Header.Get(echo.HeaderAuthorization)
	if strings.HasPrefix(authHeader, "Bearer ") {
		token = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	} else if username, password, ok := c.Request().BasicAuth(); ok {
		token = strings.TrimSpace(password)
		if token == "" {
			token = strings.TrimSpace(username)
		}
	}
	if token == "" {
		return nil, services.ErrPersonalAccessTokenNotFound
	}

	user, scopes, err := h.tokenSvc.Lookup(c.Request().Context(), token)
	if err != nil {
		return nil, err
	}
	return &gitHTTPAuthResult{user: user, scopes: scopes}, nil
}

func (h *GitHTTPHandler) requireGitAuth(c echo.Context) error {
	c.Response().Header().Set(echo.HeaderWWWAuthenticate, `Basic realm="GitPier Git", charset="UTF-8"`)
	return echo.NewHTTPError(http.StatusUnauthorized, "personal access token required")
}

func packetLine(payload string) []byte {
	return []byte(fmt.Sprintf("%04x%s", len(payload)+4, payload))
}

func isValidGitHTTPComponent(value string) bool {
	return gitHTTPNameRe.MatchString(value)
}

func authUsername(auth *gitHTTPAuthResult) string {
	if auth == nil || auth.user == nil {
		return ""
	}
	return auth.user.Username
}
