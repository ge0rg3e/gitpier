package handlers

import (
	"errors"
	"net/http"
	"strings"

	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

// AppHandler handles all GitPier App endpoints.
type AppHandler struct {
	appSvc *services.AppService
	orgSvc *services.OrgService
}

func NewAppHandler(appSvc *services.AppService, orgSvc *services.OrgService) *AppHandler {
	return &AppHandler{appSvc: appSvc, orgSvc: orgSvc}
}

func parseIDParam(c echo.Context, name string) (string, error) {
	v := strings.TrimSpace(c.Param(name))
	if v == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid "+name)
	}
	return v, nil
}

func (h *AppHandler) resolveOrg(c echo.Context) (*models.Organization, error) {
	orgname := c.Param("orgname")
	if orgname == "" {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "missing orgname")
	}
	org, err := h.orgSvc.GetByLogin(c.Request().Context(), orgname)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "organization not found")
	}
	return org, nil
}

func (h *AppHandler) verifyOwnership(c echo.Context, app *models.App) error {
	currentUser := c.Get("user").(*models.User)
	switch app.OwnerType {
	case "user":
		if app.OwnerID != currentUser.ID {
			return echo.NewHTTPError(http.StatusForbidden, "not authorized")
		}
	case "org":
		if !h.orgSvc.IsOwner(c.Request().Context(), app.OwnerID, currentUser.ID) {
			return echo.NewHTTPError(http.StatusForbidden, "only org owners can manage org apps")
		}
	default:
		return echo.NewHTTPError(http.StatusForbidden, "not authorized")
	}
	return nil
}

// GET /api/v1/users/me/apps
func (h *AppHandler) ListUserApps(c echo.Context) error {
	user := c.Get("user").(*models.User)
	apps, err := h.appSvc.ListByOwner(c.Request().Context(), user.ID, "user")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list apps")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"apps": apps})
}

// POST /api/v1/users/me/apps
func (h *AppHandler) CreateUserApp(c echo.Context) error {
	user := c.Get("user").(*models.User)
	return h.createApp(c, user.ID, "user")
}

// GET /api/v1/orgs/:orgname/apps
func (h *AppHandler) ListOrgApps(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	apps, err := h.appSvc.ListByOwner(c.Request().Context(), org.ID, "org")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list org apps")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"apps": apps})
}

// POST /api/v1/orgs/:orgname/apps
func (h *AppHandler) CreateOrgApp(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	user := c.Get("user").(*models.User)
	if !h.orgSvc.IsOwner(c.Request().Context(), org.ID, user.ID) {
		return echo.NewHTTPError(http.StatusForbidden, "only org owners can create org apps")
	}
	return h.createApp(c, org.ID, "org")
}

type createAppRequest struct {
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	HomepageURL      string            `json:"homepage_url"`
	LogoURL          string            `json:"logo_url"`
	SetupURL         string            `json:"setup_url"`
	RedirectOnUpdate bool              `json:"redirect_on_update"`
	WebhookURL       string            `json:"webhook_url"`
	WebhookSecret    string            `json:"webhook_secret"`
	WebhookActive    *bool             `json:"webhook_active"`
	IsPublic         bool              `json:"is_public"`
	CallbackURLs     []string          `json:"callback_urls"`
	RequestUserAuth  bool              `json:"request_user_auth"`
	ExpireUserTokens *bool             `json:"expire_user_tokens"`
	EnableDeviceFlow bool              `json:"enable_device_flow"`
	RepoPermissions  map[string]string `json:"repo_permissions"`
	OrgPermissions   map[string]string `json:"org_permissions"`
	AcctPermissions  map[string]string `json:"account_permissions"`
	Events           []string          `json:"events"`
}

func (h *AppHandler) createApp(c echo.Context, ownerID string, ownerType string) error {
	var req createAppRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.Name) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if strings.TrimSpace(req.HomepageURL) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "homepage_url is required")
	}

	webhookActive := true
	if req.WebhookActive != nil {
		webhookActive = *req.WebhookActive
	}
	expireTokens := true
	if req.ExpireUserTokens != nil {
		expireTokens = *req.ExpireUserTokens
	}

	app, secret, err := h.appSvc.Create(c.Request().Context(), services.CreateAppInput{
		Name:             req.Name,
		Description:      req.Description,
		HomepageURL:      req.HomepageURL,
		LogoURL:          req.LogoURL,
		SetupURL:         req.SetupURL,
		RedirectOnUpdate: req.RedirectOnUpdate,
		WebhookURL:       req.WebhookURL,
		WebhookSecret:    req.WebhookSecret,
		WebhookActive:    webhookActive,
		IsPublic:         req.IsPublic,
		CallbackURLs:     req.CallbackURLs,
		RequestUserAuth:  req.RequestUserAuth,
		ExpireUserTokens: expireTokens,
		EnableDeviceFlow: req.EnableDeviceFlow,
		RepoPermissions:  req.RepoPermissions,
		OrgPermissions:   req.OrgPermissions,
		AcctPermissions:  req.AcctPermissions,
		Events:           req.Events,
		OwnerID:          ownerID,
		OwnerType:        ownerType,
	})
	if err != nil {
		if errors.Is(err, services.ErrAppSlugTaken) {
			return echo.NewHTTPError(http.StatusUnprocessableEntity, "an app with that name already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create app")
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"app":           app,
		"client_secret": secret,
	})
}

// GET /api/v1/apps/:id
func (h *AppHandler) GetApp(c echo.Context) error {
	id, err := parseIDParam(c, "id")
	if err != nil {
		return err
	}
	app, err := h.appSvc.GetByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, services.ErrAppNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "app not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get app")
	}
	if err := h.verifyOwnership(c, app); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"app": app})
}

// PATCH /api/v1/apps/:id
func (h *AppHandler) UpdateApp(c echo.Context) error {
	id, err := parseIDParam(c, "id")
	if err != nil {
		return err
	}
	app, err := h.appSvc.GetByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, services.ErrAppNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "app not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get app")
	}
	if err := h.verifyOwnership(c, app); err != nil {
		return err
	}

	var req struct {
		Name             *string            `json:"name"`
		Description      *string            `json:"description"`
		HomepageURL      *string            `json:"homepage_url"`
		LogoURL          *string            `json:"logo_url"`
		SetupURL         *string            `json:"setup_url"`
		RedirectOnUpdate *bool              `json:"redirect_on_update"`
		WebhookURL       *string            `json:"webhook_url"`
		WebhookSecret    *string            `json:"webhook_secret"`
		WebhookActive    *bool              `json:"webhook_active"`
		IsPublic         *bool              `json:"is_public"`
		CallbackURLs     *[]string          `json:"callback_urls"`
		RequestUserAuth  *bool              `json:"request_user_auth"`
		ExpireUserTokens *bool              `json:"expire_user_tokens"`
		EnableDeviceFlow *bool              `json:"enable_device_flow"`
		RepoPermissions  *map[string]string `json:"repo_permissions"`
		OrgPermissions   *map[string]string `json:"org_permissions"`
		AcctPermissions  *map[string]string `json:"account_permissions"`
		Events           *[]string          `json:"events"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	updated, err := h.appSvc.Update(c.Request().Context(), id, services.UpdateAppInput{
		Name:             req.Name,
		Description:      req.Description,
		HomepageURL:      req.HomepageURL,
		LogoURL:          req.LogoURL,
		SetupURL:         req.SetupURL,
		RedirectOnUpdate: req.RedirectOnUpdate,
		WebhookURL:       req.WebhookURL,
		WebhookSecret:    req.WebhookSecret,
		WebhookActive:    req.WebhookActive,
		IsPublic:         req.IsPublic,
		CallbackURLs:     req.CallbackURLs,
		RequestUserAuth:  req.RequestUserAuth,
		ExpireUserTokens: req.ExpireUserTokens,
		EnableDeviceFlow: req.EnableDeviceFlow,
		RepoPermissions:  req.RepoPermissions,
		OrgPermissions:   req.OrgPermissions,
		AcctPermissions:  req.AcctPermissions,
		Events:           req.Events,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update app")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"app": updated})
}

// DELETE /api/v1/apps/:id
func (h *AppHandler) DeleteApp(c echo.Context) error {
	id, err := parseIDParam(c, "id")
	if err != nil {
		return err
	}
	app, err := h.appSvc.GetByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, services.ErrAppNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "app not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get app")
	}
	if err := h.verifyOwnership(c, app); err != nil {
		return err
	}
	if err := h.appSvc.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete app")
	}
	return c.NoContent(http.StatusNoContent)
}

// POST /api/v1/apps/:id/regenerate-secret
func (h *AppHandler) RegenerateClientSecret(c echo.Context) error {
	id, err := parseIDParam(c, "id")
	if err != nil {
		return err
	}
	app, err := h.appSvc.GetByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, services.ErrAppNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "app not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get app")
	}
	if err := h.verifyOwnership(c, app); err != nil {
		return err
	}
	_, secret, err := h.appSvc.RegenerateClientSecret(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to regenerate secret")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"client_secret": secret})
}

// GET /api/v1/apps/:id/keys
func (h *AppHandler) ListKeys(c echo.Context) error {
	id, err := parseIDParam(c, "id")
	if err != nil {
		return err
	}
	app, err := h.appSvc.GetByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, services.ErrAppNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "app not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get app")
	}
	if err := h.verifyOwnership(c, app); err != nil {
		return err
	}
	keys, err := h.appSvc.ListPrivateKeys(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list keys")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"keys": keys})
}

// POST /api/v1/apps/:id/keys
func (h *AppHandler) GenerateKey(c echo.Context) error {
	id, err := parseIDParam(c, "id")
	if err != nil {
		return err
	}
	app, err := h.appSvc.GetByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, services.ErrAppNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "app not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get app")
	}
	if err := h.verifyOwnership(c, app); err != nil {
		return err
	}
	key, privPEM, err := h.appSvc.GeneratePrivateKey(c.Request().Context(), id)
	if errors.Is(err, services.ErrTooManyKeys) {
		return echo.NewHTTPError(http.StatusBadRequest, "app already has 10 private keys")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate key")
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"key":         key,
		"private_key": privPEM, // returned once only
	})
}

// DELETE /api/v1/apps/:id/keys/:keyID
func (h *AppHandler) DeleteKey(c echo.Context) error {
	id, err := parseIDParam(c, "id")
	if err != nil {
		return err
	}
	keyID, err := parseIDParam(c, "keyID")
	if err != nil {
		return err
	}
	app, err := h.appSvc.GetByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, services.ErrAppNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "app not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get app")
	}
	if err := h.verifyOwnership(c, app); err != nil {
		return err
	}
	if err := h.appSvc.DeletePrivateKey(c.Request().Context(), id, keyID); err != nil {
		if errors.Is(err, services.ErrKeyNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "key not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete key")
	}
	return c.NoContent(http.StatusNoContent)
}

// GET /api/v1/apps/slug/:slug  Ã¢â‚¬â€ public, no auth required
func (h *AppHandler) GetPublicApp(c echo.Context) error {
	slug := c.Param("slug")
	app, err := h.appSvc.GetBySlug(c.Request().Context(), slug)
	if err != nil {
		if errors.Is(err, services.ErrAppNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "app not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get app")
	}
	// Return only public-safe fields.
	return c.JSON(http.StatusOK, map[string]interface{}{
		"id":                  app.ID,
		"slug":                app.Slug,
		"name":                app.Name,
		"description":         app.Description,
		"homepage_url":        app.HomepageURL,
		"logo_url":            app.LogoURL,
		"is_public":           app.IsPublic,
		"repo_permissions":    services.ParsePermissions(app.RepoPermissions),
		"org_permissions":     services.ParsePermissions(app.OrgPermissions),
		"account_permissions": services.ParsePermissions(app.AccountPermissions),
		"events":              services.ParseStringSlice(app.Events),
		"created_at":          app.CreatedAt,
	})
}

// GET /api/v1/users/me/installations
func (h *AppHandler) ListUserInstallations(c echo.Context) error {
	user := c.Get("user").(*models.User)
	installs, err := h.appSvc.ListInstallationsByAccount(c.Request().Context(), user.ID, "user")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list installations")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"installations": installs})
}

// GET /api/v1/orgs/:orgname/installations
func (h *AppHandler) ListOrgInstallations(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	user := c.Get("user").(*models.User)
	if !h.orgSvc.IsOwner(c.Request().Context(), org.ID, user.ID) {
		return echo.NewHTTPError(http.StatusForbidden, "only org owners can view org installations")
	}
	installs, err := h.appSvc.ListInstallationsByAccount(c.Request().Context(), org.ID, "org")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list org installations")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"installations": installs})
}

// POST /api/v1/apps/slug/:slug/install
// Installs the app on the caller's user account or an org they own.
func (h *AppHandler) InstallApp(c echo.Context) error {
	slug := c.Param("slug")
	app, err := h.appSvc.GetBySlug(c.Request().Context(), slug)
	if err != nil {
		if errors.Is(err, services.ErrAppNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "app not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get app")
	}

	user := c.Get("user").(*models.User)

	var req struct {
		// "user" to install on the caller's account, or an org login to install on an org.
		Target              string   `json:"target"`
		RepositorySelection string   `json:"repository_selection"` // "all" or "selected"
		RepoIDs             []string `json:"repo_ids"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.RepositorySelection == "" {
		req.RepositorySelection = "all"
	}

	accountID := user.ID
	accountType := "user"

	if req.Target != "" && req.Target != "user" {
		// Installing on an org.
		org, err := h.orgSvc.GetByLogin(c.Request().Context(), req.Target)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "organization not found")
		}
		if !h.orgSvc.IsOwner(c.Request().Context(), org.ID, user.ID) {
			return echo.NewHTTPError(http.StatusForbidden, "only org owners can install apps on an org")
		}
		accountID = org.ID
		accountType = "org"
	}

	// Private apps may only be installed by the owner.
	if !app.IsPublic {
		ownerCheck := false
		if app.OwnerType == "user" && app.OwnerID == user.ID {
			ownerCheck = true
		}
		if app.OwnerType == "org" && accountType == "org" {
			ownerCheck = h.orgSvc.IsOwner(c.Request().Context(), app.OwnerID, user.ID)
		}
		if !ownerCheck {
			return echo.NewHTTPError(http.StatusForbidden, "this app is private and can only be installed by its owner")
		}
	}

	installation, err := h.appSvc.Install(c.Request().Context(), services.CreateInstallationInput{
		AppID:               app.ID,
		AccountID:           accountID,
		AccountType:         accountType,
		RepositorySelection: req.RepositorySelection,
		RepoIDs:             req.RepoIDs,
	})
	if err != nil {
		if errors.Is(err, services.ErrInvalidRepoSelection) {
			return echo.NewHTTPError(http.StatusBadRequest, "repository_selection must be 'all' or 'selected'")
		}
		if errors.Is(err, services.ErrRepoAccessDenied) {
			return echo.NewHTTPError(http.StatusForbidden, "one or more selected repositories are outside the installation account")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to install app")
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"installation": installation})
}

// DELETE /api/v1/installations/:installationID
func (h *AppHandler) UninstallApp(c echo.Context) error {
	instID, err := parseIDParam(c, "installationID")
	if err != nil {
		return err
	}
	inst, err := h.appSvc.GetInstallation(c.Request().Context(), instID)
	if err != nil {
		if errors.Is(err, services.ErrInstallationNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "installation not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get installation")
	}
	user := c.Get("user").(*models.User)
	if err := h.verifyInstallationAccess(c, inst, user); err != nil {
		return err
	}
	if err := h.appSvc.Uninstall(c.Request().Context(), instID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to uninstall app")
	}
	return c.NoContent(http.StatusNoContent)
}

// PATCH /api/v1/installations/:installationID/repositories
func (h *AppHandler) UpdateInstallationRepos(c echo.Context) error {
	instID, err := parseIDParam(c, "installationID")
	if err != nil {
		return err
	}
	inst, err := h.appSvc.GetInstallation(c.Request().Context(), instID)
	if err != nil {
		if errors.Is(err, services.ErrInstallationNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "installation not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get installation")
	}
	user := c.Get("user").(*models.User)
	if err := h.verifyInstallationAccess(c, inst, user); err != nil {
		return err
	}

	var req struct {
		RepositorySelection string   `json:"repository_selection"`
		RepoIDs             []string `json:"repo_ids"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.RepositorySelection == "" {
		req.RepositorySelection = "all"
	}
	if err := h.appSvc.UpdateInstallationRepos(c.Request().Context(), instID, req.RepositorySelection, req.RepoIDs); err != nil {
		if errors.Is(err, services.ErrInstallationNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "installation not found")
		}
		if errors.Is(err, services.ErrInvalidRepoSelection) {
			return echo.NewHTTPError(http.StatusBadRequest, "repository_selection must be 'all' or 'selected'")
		}
		if errors.Is(err, services.ErrRepoAccessDenied) {
			return echo.NewHTTPError(http.StatusForbidden, "one or more selected repositories are outside the installation account")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update repositories")
	}
	updated, _ := h.appSvc.GetInstallation(c.Request().Context(), instID)
	return c.JSON(http.StatusOK, map[string]interface{}{"installation": updated})
}

// PATCH /api/v1/installations/:installationID/permissions
// Approves updated app permissions for an existing installation by syncing the
// installation permission snapshot from the app's current settings.
func (h *AppHandler) SyncInstallationPermissions(c echo.Context) error {
	instID, err := parseIDParam(c, "installationID")
	if err != nil {
		return err
	}
	inst, err := h.appSvc.GetInstallation(c.Request().Context(), instID)
	if err != nil {
		if errors.Is(err, services.ErrInstallationNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "installation not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get installation")
	}
	user := c.Get("user").(*models.User)
	if err := h.verifyInstallationAccess(c, inst, user); err != nil {
		return err
	}

	if err := h.appSvc.SyncInstallationPermissions(c.Request().Context(), instID); err != nil {
		if errors.Is(err, services.ErrInstallationNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "installation not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update installation permissions")
	}

	updated, _ := h.appSvc.GetInstallation(c.Request().Context(), instID)
	return c.JSON(http.StatusOK, map[string]interface{}{"installation": updated})
}

// GET /api/v1/installations/:installationID
func (h *AppHandler) GetInstallation(c echo.Context) error {
	instID, err := parseIDParam(c, "installationID")
	if err != nil {
		return err
	}
	inst, err := h.appSvc.GetInstallation(c.Request().Context(), instID)
	if err != nil {
		if errors.Is(err, services.ErrInstallationNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "installation not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get installation")
	}
	user := c.Get("user").(*models.User)
	if err := h.verifyInstallationAccess(c, inst, user); err != nil {
		// Also allow the app owner to view installations of their app.
		app := inst.App
		if !(app.OwnerType == "user" && app.OwnerID == user.ID) &&
			!(app.OwnerType == "org" && h.orgSvc.IsOwner(c.Request().Context(), app.OwnerID, user.ID)) {
			return echo.NewHTTPError(http.StatusForbidden, "not authorized")
		}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"installation": inst})
}

// GET /api/v1/apps/:id/installations
func (h *AppHandler) ListAppInstallations(c echo.Context) error {
	id, err := parseIDParam(c, "id")
	if err != nil {
		return err
	}
	app, err := h.appSvc.GetByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, services.ErrAppNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "app not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get app")
	}
	if err := h.verifyOwnership(c, app); err != nil {
		return err
	}
	installs, err := h.appSvc.ListInstallationsByApp(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list installations")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"installations": installs})
}

// PUT /api/v1/installations/:installationID/suspended
func (h *AppHandler) SuspendInstallation(c echo.Context) error {
	instID, err := parseIDParam(c, "installationID")
	if err != nil {
		return err
	}
	inst, err := h.appSvc.GetInstallation(c.Request().Context(), instID)
	if err != nil {
		if errors.Is(err, services.ErrInstallationNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "installation not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get installation")
	}
	user := c.Get("user").(*models.User)
	// Only the app owner can suspend.
	app := inst.App
	if !(app.OwnerType == "user" && app.OwnerID == user.ID) &&
		!(app.OwnerType == "org" && h.orgSvc.IsOwner(c.Request().Context(), app.OwnerID, user.ID)) {
		return echo.NewHTTPError(http.StatusForbidden, "only the app owner can suspend installations")
	}
	if err := h.appSvc.SuspendInstallation(c.Request().Context(), instID, user.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to suspend installation")
	}
	return c.NoContent(http.StatusNoContent)
}

// DELETE /api/v1/installations/:installationID/suspended
func (h *AppHandler) UnsuspendInstallation(c echo.Context) error {
	instID, err := parseIDParam(c, "installationID")
	if err != nil {
		return err
	}
	inst, err := h.appSvc.GetInstallation(c.Request().Context(), instID)
	if err != nil {
		if errors.Is(err, services.ErrInstallationNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "installation not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get installation")
	}
	user := c.Get("user").(*models.User)
	app := inst.App
	if !(app.OwnerType == "user" && app.OwnerID == user.ID) &&
		!(app.OwnerType == "org" && h.orgSvc.IsOwner(c.Request().Context(), app.OwnerID, user.ID)) {
		return echo.NewHTTPError(http.StatusForbidden, "only the app owner can unsuspend installations")
	}
	if err := h.appSvc.UnsuspendInstallation(c.Request().Context(), instID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to unsuspend installation")
	}
	return c.NoContent(http.StatusNoContent)
}

// POST /api/v1/app/installations/:installationID/access_tokens
// The caller must authenticate with a JWT signed by the app's private key.
// Authorization: Bearer <JWT>
func (h *AppHandler) CreateInstallationToken(c echo.Context) error {
	instID, err := parseIDParam(c, "installationID")
	if err != nil {
		return err
	}

	// Extract Bearer JWT from Authorization header.
	authHeader := c.Request().Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return echo.NewHTTPError(http.StatusUnauthorized, "bearer JWT required")
	}
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

	app, err := h.appSvc.VerifyAppJWT(c.Request().Context(), tokenStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid app JWT")
	}

	// Verify the installation belongs to this app.
	inst, err := h.appSvc.GetInstallation(c.Request().Context(), instID)
	if err != nil {
		if errors.Is(err, services.ErrInstallationNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "installation not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get installation")
	}
	if inst.AppID != app.ID {
		return echo.NewHTTPError(http.StatusForbidden, "installation does not belong to this app")
	}

	tok, expiresAt, err := h.appSvc.CreateInstallationToken(c.Request().Context(), instID)
	if errors.Is(err, services.ErrAppSuspended) {
		return echo.NewHTTPError(http.StatusForbidden, "app installation is suspended")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create installation token")
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"token":      tok,
		"expires_at": expiresAt,
		"permissions": map[string]interface{}{
			"repository":   services.ParsePermissions(inst.RepoPermissions),
			"organization": services.ParsePermissions(inst.OrgPermissions),
			"account":      services.ParsePermissions(inst.AccountPermissions),
		},
		"repository_selection": inst.RepositorySelection,
	})
}

// verifyInstallationAccess checks that the caller has authority over the account
// that has the installation (either it's their user account or an org they own).
func (h *AppHandler) verifyInstallationAccess(c echo.Context, inst *models.AppInstallation, user *models.User) error {
	if inst.AccountType == "user" && inst.AccountID == user.ID {
		return nil
	}
	if inst.AccountType == "org" && h.orgSvc.IsOwner(c.Request().Context(), inst.AccountID, user.ID) {
		return nil
	}
	return echo.NewHTTPError(http.StatusForbidden, "not authorized")
}
