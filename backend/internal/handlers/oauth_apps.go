package handlers

import (
	"errors"
	"net/http"

	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

// OAuthAppHandler handles routes for creating and managing OAuth applications.
type OAuthAppHandler struct {
	oauthSvc *services.OAuthAppService
	orgSvc   *services.OrgService
}

func NewOAuthAppHandler(oauthSvc *services.OAuthAppService, orgSvc *services.OrgService) *OAuthAppHandler {
	return &OAuthAppHandler{oauthSvc: oauthSvc, orgSvc: orgSvc}
}

// parseAppID extracts and validates the :id path parameter.
func parseAppID(c echo.Context) (string, error) {
	id := c.Param("id")
	if id == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid app id")
	}
	return id, nil
}

// verifyAppOwnership checks that the caller owns the given OAuth app.
func (h *OAuthAppHandler) verifyAppOwnership(c echo.Context, app *models.OAuthApp) error {
	currentUser := c.Get("user").(*models.User)
	switch app.OwnerType {
	case "user":
		if app.OwnerID != currentUser.ID {
			return echo.NewHTTPError(http.StatusForbidden, "not authorized")
		}
	case "org":
		if !h.orgSvc.IsOwner(c.Request().Context(), app.OwnerID, currentUser.ID) {
			return echo.NewHTTPError(http.StatusForbidden, "only org owners can manage org OAuth apps")
		}
	default:
		return echo.NewHTTPError(http.StatusForbidden, "not authorized")
	}
	return nil
}

// GET /api/v1/users/me/oauth-apps
func (h *OAuthAppHandler) ListUserApps(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	apps, err := h.oauthSvc.ListByOwner(c.Request().Context(), currentUser.ID, "user")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list oauth apps")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"apps": apps})
}

// POST /api/v1/users/me/oauth-apps
func (h *OAuthAppHandler) CreateUserApp(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	return h.createApp(c, currentUser.ID, "user")
}

// GET /api/v1/orgs/:orgname/oauth-apps
func (h *OAuthAppHandler) ListOrgApps(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	apps, err := h.oauthSvc.ListByOwner(c.Request().Context(), org.ID, "org")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list oauth apps")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"apps": apps})
}

// POST /api/v1/orgs/:orgname/oauth-apps
func (h *OAuthAppHandler) CreateOrgApp(c echo.Context) error {
	org, err := h.resolveOrg(c)
	if err != nil {
		return err
	}
	currentUser := c.Get("user").(*models.User)
	if !h.orgSvc.IsOwner(c.Request().Context(), org.ID, currentUser.ID) {
		return echo.NewHTTPError(http.StatusForbidden, "only org owners can create org OAuth apps")
	}
	return h.createApp(c, org.ID, "org")
}

// GET /api/v1/oauth-apps/:id
func (h *OAuthAppHandler) GetApp(c echo.Context) error {
	id, err := parseAppID(c)
	if err != nil {
		return err
	}
	app, err := h.oauthSvc.GetByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, services.ErrOAuthAppNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "oauth app not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get oauth app")
	}
	if err := h.verifyAppOwnership(c, app); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"app": app})
}

// PATCH /api/v1/oauth-apps/:id
func (h *OAuthAppHandler) UpdateApp(c echo.Context) error {
	id, err := parseAppID(c)
	if err != nil {
		return err
	}
	app, err := h.oauthSvc.GetByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, services.ErrOAuthAppNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "oauth app not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get oauth app")
	}
	if err := h.verifyAppOwnership(c, app); err != nil {
		return err
	}

	var req struct {
		Name             *string `json:"name"`
		Description      *string `json:"description"`
		HomepageURL      *string `json:"homepage_url"`
		CallbackURL      *string `json:"callback_url"`
		LogoURL          *string `json:"logo_url"`
		EnableDeviceFlow *bool   `json:"enable_device_flow"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	updated, err := h.oauthSvc.Update(c.Request().Context(), id, services.UpdateOAuthAppInput{
		Name:             req.Name,
		Description:      req.Description,
		HomepageURL:      req.HomepageURL,
		CallbackURL:      req.CallbackURL,
		LogoURL:          req.LogoURL,
		EnableDeviceFlow: req.EnableDeviceFlow,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update oauth app")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"app": updated})
}

// DELETE /api/v1/oauth-apps/:id
func (h *OAuthAppHandler) DeleteApp(c echo.Context) error {
	id, err := parseAppID(c)
	if err != nil {
		return err
	}
	app, err := h.oauthSvc.GetByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, services.ErrOAuthAppNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "oauth app not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get oauth app")
	}
	if err := h.verifyAppOwnership(c, app); err != nil {
		return err
	}
	if err := h.oauthSvc.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete oauth app")
	}
	return c.NoContent(http.StatusNoContent)
}

// POST /api/v1/oauth-apps/:id/regenerate-secret
func (h *OAuthAppHandler) RegenerateSecret(c echo.Context) error {
	id, err := parseAppID(c)
	if err != nil {
		return err
	}
	app, err := h.oauthSvc.GetByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, services.ErrOAuthAppNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "oauth app not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get oauth app")
	}
	if err := h.verifyAppOwnership(c, app); err != nil {
		return err
	}
	secret, err := h.oauthSvc.RegenerateSecret(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to regenerate secret")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"client_secret": secret})
}

// GET /api/v1/users/me/authorized-apps
func (h *OAuthAppHandler) ListAuthorizedApps(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	auths, err := h.oauthSvc.ListAuthorizedApps(c.Request().Context(), currentUser.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list authorized apps")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"authorizations": auths})
}

// DELETE /api/v1/users/me/authorized-apps/:id
func (h *OAuthAppHandler) RevokeAuthorization(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	if err := h.oauthSvc.RevokeAuthorization(c.Request().Context(), id, currentUser.ID); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "authorization not found")
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *OAuthAppHandler) resolveOrg(c echo.Context) (*models.Organization, error) {
	orgname := c.Param("orgname")
	org, err := h.orgSvc.GetByLogin(c.Request().Context(), orgname)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "organization not found")
	}
	return org, nil
}

// createApp is the shared implementation for both user and org app creation.
func (h *OAuthAppHandler) createApp(c echo.Context, ownerID string, ownerType string) error {
	var req struct {
		Name             string `json:"name"`
		Description      string `json:"description"`
		HomepageURL      string `json:"homepage_url"`
		CallbackURL      string `json:"callback_url"`
		LogoURL          string `json:"logo_url"`
		EnableDeviceFlow bool   `json:"enable_device_flow"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if req.HomepageURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "homepage_url is required")
	}
	if req.CallbackURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "callback_url is required")
	}

	app, secret, err := h.oauthSvc.Create(c.Request().Context(), services.CreateOAuthAppInput{
		Name:             req.Name,
		Description:      req.Description,
		HomepageURL:      req.HomepageURL,
		CallbackURL:      req.CallbackURL,
		LogoURL:          req.LogoURL,
		EnableDeviceFlow: req.EnableDeviceFlow,
		OwnerID:          ownerID,
		OwnerType:        ownerType,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create oauth app")
	}

	// Return the secret in the creation response only Ã¢â‚¬â€ it will never be returned again.
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"app":           app,
		"client_secret": secret,
	})
}
