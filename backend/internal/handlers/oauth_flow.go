package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"gitpier/internal/middleware"
	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

// OAuthFlowHandler implements the GitHub-compatible OAuth 2.0 authorization endpoints.
// These live outside the /api/v1 prefix to match GitHub's URL structure.
type OAuthFlowHandler struct {
	svc    *services.OAuthFlowService
	appSvc *services.OAuthAppService
}

func NewOAuthFlowHandler(svc *services.OAuthFlowService, appSvc *services.OAuthAppService) *OAuthFlowHandler {
	return &OAuthFlowHandler{svc: svc, appSvc: appSvc}
}

// Returns app metadata for the frontend consent page.

func (h *OAuthFlowHandler) GetAppInfo(c echo.Context) error {
	clientID := c.QueryParam("client_id")
	if clientID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "client_id is required")
	}

	app, err := h.svc.GetAppInfoForConsent(c.Request().Context(), clientID)
	if err != nil {
		if errors.Is(err, services.ErrOAuthAppNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "application not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.JSON(http.StatusOK, app)
}

// Called by the frontend after the user clicks "Authorize". Creates an auth code
// and returns the redirect URI with the code and state appended.
//
// Body (JSON):
//
//	{ "client_id", "redirect_uri", "scope", "state",
//	  "code_challenge", "code_challenge_method" }

type authorizeRequest struct {
	ClientID            string `json:"client_id"`
	RedirectURI         string `json:"redirect_uri"`
	Scope               string `json:"scope"`
	State               string `json:"state"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
}

func (h *OAuthFlowHandler) Authorize(c echo.Context) error {
	user, ok := c.Get(middleware.UserContextKey).(*models.User)
	if !ok || user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	var req authorizeRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.ClientID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "client_id is required")
	}

	// Validate PKCE method (only S256 supported, matching GitHub).
	if req.CodeChallenge != "" && req.CodeChallengeMethod != "S256" {
		return echo.NewHTTPError(http.StatusBadRequest, "unsupported code_challenge_method; use S256")
	}

	app, err := h.svc.GetAppInfoForConsent(c.Request().Context(), req.ClientID)
	if err != nil {
		if errors.Is(err, services.ErrOAuthAppNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "application not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	// Validate redirect_uri: must match or be a prefix of the registered callback.
	if req.RedirectURI != "" && !isValidRedirectURI(req.RedirectURI, app.CallbackURL) {
		return echo.NewHTTPError(http.StatusBadRequest, "redirect_uri does not match the registered callback URL")
	}

	redirectURI := req.RedirectURI
	if redirectURI == "" {
		redirectURI = app.CallbackURL
	}

	code, err := h.svc.CreateAuthorizationCode(
		c.Request().Context(),
		app.ID, user.ID,
		req.Scope, redirectURI,
		req.CodeChallenge, req.CodeChallengeMethod,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create authorization code")
	}

	callbackURL := fmt.Sprintf("%s?code=%s", redirectURI, code)
	if req.State != "" {
		callbackURL += "&state=" + req.State
	}

	return c.JSON(http.StatusOK, map[string]string{
		"redirect_uri": callbackURL,
	})
}

// isValidRedirectURI checks that the provided redirect_uri matches the app's registered
// callback_url. We require an exact match to avoid open-redirect attacks.
func isValidRedirectURI(provided, registered string) bool {
	return provided == registered
}

// accessTokenRequest supports both application/x-www-form-urlencoded and application/json.
type accessTokenRequest struct {
	GrantType    string `json:"grant_type"    form:"grant_type"`
	ClientID     string `json:"client_id"     form:"client_id"`
	ClientSecret string `json:"client_secret" form:"client_secret"`
	Code         string `json:"code"          form:"code"`
	RedirectURI  string `json:"redirect_uri"  form:"redirect_uri"`
	CodeVerifier string `json:"code_verifier" form:"code_verifier"`
	DeviceCode   string `json:"device_code"   form:"device_code"`
}

// Exchanges an authorization code for an access token (authorization_code grant)
// or polls for a device flow token (device_code grant).
//
// Request: application/x-www-form-urlencoded or application/json.
// Response: form-encoded by default; JSON if Accept: application/json.

func (h *OAuthFlowHandler) AccessToken(c echo.Context) error {
	var req accessTokenRequest
	if err := c.Bind(&req); err != nil {
		return h.oauthError(c, "invalid_request", "could not parse request body")
	}

	if req.GrantType == "" {
		req.GrantType = "authorization_code"
	}

	switch req.GrantType {
	case "authorization_code":
		return h.handleAuthorizationCodeGrant(c, req)
	case "urn:ietf:params:oauth:grant-type:device_code":
		return h.handleDeviceCodeGrant(c, req)
	default:
		return h.oauthError(c, "unsupported_grant_type", "grant type not supported")
	}
}

func (h *OAuthFlowHandler) handleAuthorizationCodeGrant(c echo.Context, req accessTokenRequest) error {
	if req.ClientID == "" || req.ClientSecret == "" || req.Code == "" {
		return h.oauthError(c, "invalid_request", "client_id, client_secret, and code are required")
	}

	token, scopes, err := h.svc.ExchangeCode(
		c.Request().Context(),
		req.ClientID, req.ClientSecret, req.Code, req.RedirectURI, req.CodeVerifier,
	)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrOAuthCodeNotFound),
			errors.Is(err, services.ErrOAuthCodeUsed),
			errors.Is(err, services.ErrOAuthCodeExpired):
			return h.oauthError(c, "bad_verification_code", err.Error())
		case errors.Is(err, services.ErrOAuthInvalidClient):
			return h.oauthError(c, "incorrect_client_credentials", "invalid client_id or client_secret")
		case errors.Is(err, services.ErrOAuthInvalidRedirect):
			return h.oauthError(c, "redirect_uri_mismatch", "redirect_uri does not match")
		case errors.Is(err, services.ErrOAuthPKCEFailed):
			return h.oauthError(c, "incorrect_client_credentials", "PKCE verification failed")
		default:
			return h.oauthError(c, "server_error", "internal server error")
		}
	}

	return h.tokenResponse(c, token, scopes)
}

func (h *OAuthFlowHandler) handleDeviceCodeGrant(c echo.Context, req accessTokenRequest) error {
	if req.ClientID == "" || req.DeviceCode == "" {
		return h.oauthError(c, "invalid_request", "client_id and device_code are required")
	}

	token, scopes, err := h.svc.PollDeviceFlow(c.Request().Context(), req.ClientID, req.DeviceCode)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrAuthorizationPending):
			return h.oauthError(c, "authorization_pending", "the user has not yet authorized the device")
		case errors.Is(err, services.ErrSlowDown):
			return h.oauthError(c, "slow_down", "polling too frequently; increase interval")
		case errors.Is(err, services.ErrExpiredToken):
			return h.oauthError(c, "expired_token", "the device code has expired")
		case errors.Is(err, services.ErrOAuthAccessDenied):
			return h.oauthError(c, "access_denied", "the user denied the request")
		case errors.Is(err, services.ErrDeviceCodeNotFound):
			return h.oauthError(c, "bad_device_code", "device code not found")
		case errors.Is(err, services.ErrOAuthInvalidClient):
			return h.oauthError(c, "incorrect_client_credentials", "invalid client_id")
		default:
			return h.oauthError(c, "server_error", "internal server error")
		}
	}

	return h.tokenResponse(c, token, scopes)
}

// deviceCodeRequest supports both application/x-www-form-urlencoded and application/json.
type deviceCodeRequest struct {
	ClientID string `json:"client_id" form:"client_id"`
	Scope    string `json:"scope"     form:"scope"`
}

// Initiates the device flow. Called by headless apps (CLI tools, etc.).
//
// Body: client_id, scope.
// Response: JSON with device_code, user_code, verification_uri, expires_in, interval.

func (h *OAuthFlowHandler) DeviceCode(c echo.Context) error {
	var req deviceCodeRequest
	if err := c.Bind(&req); err != nil || req.ClientID == "" {
		return h.oauthError(c, "invalid_request", "client_id is required")
	}

	baseURL := publicBaseURL(c)
	if baseURL == "" {
		return h.oauthError(c, "server_error", "oauth device flow requires APP_URL to be configured")
	}

	resp, err := h.svc.CreateDeviceCode(c.Request().Context(), req.ClientID, req.Scope, baseURL)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrOAuthInvalidClient):
			return h.oauthError(c, "incorrect_client_credentials", "invalid client_id")
		case errors.Is(err, services.ErrDeviceFlowDisabled):
			return h.oauthError(c, "device_flow_disabled", "device flow is not enabled for this application")
		default:
			return h.oauthError(c, "server_error", "internal server error")
		}
	}

	return c.JSON(http.StatusOK, resp)
}

// Returns the app info for a user_code so the frontend can render the device activation page.

func (h *OAuthFlowHandler) GetDeviceInfo(c echo.Context) error {
	userCode := c.QueryParam("user_code")
	if userCode == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "user_code is required")
	}

	record, app, err := h.svc.GetDeviceCodeForActivation(c.Request().Context(), userCode)
	if err != nil {
		if errors.Is(err, services.ErrDeviceCodeNotFound) || errors.Is(err, services.ErrDeviceCodeExpired) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"user_code":  record.UserCode,
		"scopes":     record.Scopes,
		"expires_at": record.ExpiresAt,
		"app": map[string]interface{}{
			"name":         app.Name,
			"description":  app.Description,
			"homepage_url": app.HomepageURL,
			"logo_url":     app.LogoURL,
		},
	})
}

// Authenticat user approves a device code.

func (h *OAuthFlowHandler) ApproveDevice(c echo.Context) error {
	user, ok := c.Get(middleware.UserContextKey).(*models.User)
	if !ok || user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	var body struct {
		UserCode string `json:"user_code"`
	}
	if err := c.Bind(&body); err != nil || body.UserCode == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "user_code is required")
	}

	app, err := h.svc.ApproveDeviceCode(c.Request().Context(), body.UserCode, user.ID)
	if err != nil {
		if errors.Is(err, services.ErrDeviceCodeNotFound) || errors.Is(err, services.ErrDeviceCodeExpired) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":  "device authorized successfully",
		"app_name": app.Name,
	})
}

// Authenticated user denies a device code.

func (h *OAuthFlowHandler) DenyDevice(c echo.Context) error {
	user, ok := c.Get(middleware.UserContextKey).(*models.User)
	if !ok || user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	var body struct {
		UserCode string `json:"user_code"`
	}
	if err := c.Bind(&body); err != nil || body.UserCode == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "user_code is required")
	}

	if err := h.svc.DenyDeviceCode(c.Request().Context(), body.UserCode); err != nil {
		if errors.Is(err, services.ErrDeviceCodeNotFound) || errors.Is(err, services.ErrDeviceCodeExpired) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "device authorization denied"})
}

// tokenResponse writes an access token in the format requested by the client.
// Default is application/x-www-form-urlencoded (matching GitHub). If the client
// sends Accept: application/json, a JSON object is returned instead.
func (h *OAuthFlowHandler) tokenResponse(c echo.Context, token, scopes string) error {
	accept := c.Request().Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		return c.JSON(http.StatusOK, map[string]string{
			"access_token": token,
			"token_type":   "bearer",
			"scope":        scopes,
		})
	}

	// Default: form-encoded, matching GitHub's behavior.
	body := fmt.Sprintf("access_token=%s&scope=%s&token_type=bearer",
		token,
		strings.ReplaceAll(scopes, " ", "%20"),
	)
	return c.String(http.StatusOK, body)
}

// oauthError writes an OAuth error response (form-encoded or JSON).
func (h *OAuthFlowHandler) oauthError(c echo.Context, errCode, desc string) error {
	accept := c.Request().Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		return c.JSON(http.StatusOK, map[string]string{
			"error":             errCode,
			"error_description": desc,
		})
	}

	body := fmt.Sprintf("error=%s&error_description=%s",
		errCode,
		strings.ReplaceAll(desc, " ", "+"),
	)
	return c.String(http.StatusOK, body)
}
