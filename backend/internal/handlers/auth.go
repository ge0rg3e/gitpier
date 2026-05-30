package handlers

import (
	"errors"
	"gitpier/internal/config"
	"gitpier/internal/middleware"
	"gitpier/internal/models"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,37}[a-zA-Z0-9])?$`)
var emailRe = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

type AuthHandler struct {
	authSvc      *services.AuthService
	antiSpamSvc  *services.AntiSpamService
	secureCookie bool // true when serving over HTTPS
	cfg          *config.Config
}

func NewAuthHandler(authSvc *services.AuthService, antiSpamSvc *services.AntiSpamService, appURL string, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		authSvc:      authSvc,
		antiSpamSvc:  antiSpamSvc,
		secureCookie: strings.HasPrefix(appURL, "https://"),
		cfg:          cfg,
	}
}

// setAuthCookie writes the JWT as an HttpOnly cookie so that browser clients
// are authenticated without ever exposing the token to JavaScript.
func (h *AuthHandler) setAuthCookie(c echo.Context, token string) {
	c.SetCookie(&http.Cookie{
		Name:     "gitpier_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   72 * 3600, // 3 days, matching JWT lifetime
	})
}

// clearAuthCookie removes the authentication cookie.
func (h *AuthHandler) clearAuthCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     "gitpier_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func (h *AuthHandler) withAbsoluteAvatarURL(c echo.Context, user *models.User) *models.User {
	if user == nil {
		return nil
	}
	resp := *user
	resp.AvatarURL = toAbsoluteURL(c, resp.AvatarURL)
	return &resp
}

type registerRequest struct {
	Username       string `json:"username"`
	Email          string `json:"email"`
	Password       string `json:"password"`
	GDPRConsent    bool   `json:"gdpr_consent"`
	TurnstileToken string `json:"turnstile_token"`
}

type loginRequest struct {
	Email                 string `json:"email"`
	Password              string `json:"password"`
	ChallengeToken        string `json:"challenge_token"`
	TwoFactorCode         string `json:"two_factor_code"`
	TwoFactorRecoveryCode string `json:"two_factor_recovery_code"`
	TurnstileToken        string `json:"turnstile_token"`
}

type authResponse struct {
	Token                   string       `json:"token,omitempty"`
	User                    *models.User `json:"user,omitempty"`
	RequiresTwoFactor       bool         `json:"requires_2fa,omitempty"`
	TwoFactorChallengeToken string       `json:"two_factor_challenge_token,omitempty"`
}

type registerOTPResponse struct {
	Message           string `json:"message"`
	RegistrationToken string `json:"registration_token"`
	ExpiresInSeconds  int64  `json:"expires_in_seconds"`
}

func (h *AuthHandler) Register(c echo.Context) error {
	var req registerRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if len(req.Username) < 1 || len(req.Username) > 39 {
		return echo.NewHTTPError(http.StatusBadRequest, "username must be 1-39 characters")
	}
	if !usernameRe.MatchString(req.Username) {
		return echo.NewHTTPError(http.StatusBadRequest, "username can only contain alphanumeric characters and hyphens")
	}
	if len(req.Password) < 8 {
		return echo.NewHTTPError(http.StatusBadRequest, "password must be at least 8 characters")
	}
	if req.Email == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "email is required")
	}
	if len(req.Email) > 254 || !emailRe.MatchString(req.Email) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid email address")
	}
	if !req.GDPRConsent {
		return echo.NewHTTPError(http.StatusBadRequest, "you must accept the privacy policy and terms of service to register")
	}

	// Use the direct TCP peer address for GDPR consent IP to prevent
	// spoofing via X-Forwarded-For when no trusted proxy is configured.
	ip, _, _ := net.SplitHostPort(c.Request().RemoteAddr)
	if ip == "" {
		ip = c.Request().RemoteAddr
	}

	// Verify Turnstile token
	if err := h.antiSpamSvc.VerifyTurnstileToken(c.Request().Context(), req.TurnstileToken, ip); err != nil {
		// Record failed attempt
		_ = h.antiSpamSvc.RecordAccountCreationAttempt(c.Request().Context(), ip, req.Email, c.Request().UserAgent(), false)
		return echo.NewHTTPError(http.StatusUnauthorized, "CAPTCHA verification failed. Please try again.")
	}

	// Check for disposable email
	if err := h.antiSpamSvc.CheckDisposableEmail(req.Email); err != nil {
		_ = h.antiSpamSvc.RecordAccountCreationAttempt(c.Request().Context(), ip, req.Email, c.Request().UserAgent(), false)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Check rate limiting
	if err := h.antiSpamSvc.CheckAccountCreationRate(c.Request().Context(), ip); err != nil {
		_ = h.antiSpamSvc.RecordAccountCreationAttempt(c.Request().Context(), ip, req.Email, c.Request().UserAgent(), false)
		return echo.NewHTTPError(http.StatusTooManyRequests, err.Error())
	}

	registrationToken, expiresAt, err := h.authSvc.RequestRegistrationOTP(c.Request().Context(), services.RegisterInput{
		Username:      req.Username,
		Email:         req.Email,
		Password:      req.Password,
		GDPRConsentIP: ip,
		IPAddress:     ip,
		UserAgent:     c.Request().UserAgent(),
	})
	if err != nil {
		// Record failed attempt
		_ = h.antiSpamSvc.RecordAccountCreationAttempt(c.Request().Context(), ip, req.Email, c.Request().UserAgent(), false)
		if errors.Is(err, services.ErrEmailTaken) || errors.Is(err, services.ErrUsernameTaken) {
			return echo.NewHTTPError(http.StatusConflict, "an account with those details already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to start registration")
	}

	return c.JSON(http.StatusAccepted, registerOTPResponse{
		Message:           "Verification code sent. Enter the OTP to complete account creation.",
		RegistrationToken: registrationToken,
		ExpiresInSeconds:  int64(time.Until(expiresAt).Seconds()),
	})
}

func (h *AuthHandler) VerifyRegistrationOTP(c echo.Context) error {
	var req struct {
		RegistrationToken string `json:"registration_token"`
		OTPCode           string `json:"otp_code"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.RegistrationToken) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "registration_token is required")
	}
	otp := strings.TrimSpace(req.OTPCode)
	if len(otp) != 6 {
		return echo.NewHTTPError(http.StatusBadRequest, "otp_code must be a 6-digit code")
	}
	if matched, _ := regexp.MatchString(`^[0-9]{6}$`, otp); !matched {
		return echo.NewHTTPError(http.StatusBadRequest, "otp_code must be a 6-digit code")
	}

	ip, _, _ := net.SplitHostPort(c.Request().RemoteAddr)
	if ip == "" {
		ip = c.Request().RemoteAddr
	}

	user, token, err := h.authSvc.VerifyRegistrationOTP(c.Request().Context(), req.RegistrationToken, otp, ip, c.Request().UserAgent())
	if err != nil {
		if errors.Is(err, services.ErrInvalidRegistrationOTP) {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired otp code")
		}
		if errors.Is(err, services.ErrEmailTaken) || errors.Is(err, services.ErrUsernameTaken) {
			return echo.NewHTTPError(http.StatusConflict, "an account with those details already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create account")
	}

	_ = h.antiSpamSvc.RecordAccountCreationAttempt(c.Request().Context(), ip, user.Email, c.Request().UserAgent(), true)

	h.setAuthCookie(c, token)
	return c.JSON(http.StatusCreated, authResponse{Token: token, User: h.withAbsoluteAvatarURL(c, user)})
}

func (h *AuthHandler) Logout(c echo.Context) error {
	// Revoke the session row if one is attached to this token.
	if sessionID, ok := c.Get(middleware.SessionIDKey).(string); ok && sessionID != "" {
		if user, ok := c.Get(middleware.UserContextKey).(*models.User); ok && user != nil {
			_ = h.authSvc.RevokeSession(c.Request().Context(), user.ID, sessionID)
		}
	}
	h.clearAuthCookie(c)
	return c.NoContent(http.StatusNoContent)
}

func (h *AuthHandler) Login(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.ChallengeToken == "" {
		if req.Email == "" || req.Password == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "email and password are required")
		}

		// Verify Turnstile token on login attempt
		if err := h.antiSpamSvc.VerifyTurnstileToken(c.Request().Context(), req.TurnstileToken, c.RealIP()); err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "CAPTCHA verification failed. Please try again.")
		}
	} else if req.TwoFactorCode == "" && req.TwoFactorRecoveryCode == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "two-factor code or recovery code is required")
	}

	result, err := h.authSvc.Login(c.Request().Context(), services.LoginInput{
		EmailOrUsername:       req.Email,
		Password:              req.Password,
		ChallengeToken:        req.ChallengeToken,
		TwoFactorCode:         req.TwoFactorCode,
		TwoFactorRecoveryCode: req.TwoFactorRecoveryCode,
		IPAddress:             c.RealIP(),
		UserAgent:             c.Request().UserAgent(),
	})
	if err != nil {
		if errors.Is(err, services.ErrInvalidTwoFactor) {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid two-factor code")
		}
		if errors.Is(err, services.ErrAccountLocked) {
			return echo.NewHTTPError(http.StatusTooManyRequests, "account temporarily locked — please try again later")
		}
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
	}

	if result.RequiresTwoFactor {
		return c.JSON(http.StatusOK, authResponse{RequiresTwoFactor: true, TwoFactorChallengeToken: result.TwoFactorChallengeToken})
	}

	h.setAuthCookie(c, result.Token)
	return c.JSON(http.StatusOK, authResponse{Token: result.Token, User: h.withAbsoluteAvatarURL(c, result.User)})
}

func (h *AuthHandler) TwoFactorStatus(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	enabled, hasPending, err := h.authSvc.GetTwoFactorStatus(c.Request().Context(), currentUser.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to fetch two-factor status")
	}

	return c.JSON(http.StatusOK, map[string]bool{
		"enabled":           enabled,
		"has_pending_setup": hasPending,
	})
}

func (h *AuthHandler) TwoFactorSetup(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	var req struct {
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "password is required")
	}
	if err := h.authSvc.VerifyPassword(c.Request().Context(), currentUser.ID, req.Password); err != nil {
		return echo.NewHTTPError(http.StatusForbidden, "incorrect password")
	}

	setup, err := h.authSvc.StartTwoFactorSetup(c.Request().Context(), currentUser.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to start two-factor setup")
	}

	return c.JSON(http.StatusOK, setup)
}

func (h *AuthHandler) TwoFactorEnable(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	var req struct {
		Code string `json:"code"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "authenticator code is required")
	}

	recoveryCodes, err := h.authSvc.EnableTwoFactor(c.Request().Context(), currentUser.ID, req.Code)
	if err != nil {
		if errors.Is(err, services.ErrInvalidTwoFactor) {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid two-factor code")
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"enabled":        true,
		"recovery_codes": recoveryCodes,
	})
}

func (h *AuthHandler) TwoFactorDisable(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	var req struct {
		Password     string `json:"password"`
		Code         string `json:"code"`
		RecoveryCode string `json:"recovery_code"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "password is required")
	}
	if req.Code == "" && req.RecoveryCode == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "authenticator code or recovery code is required")
	}
	if err := h.authSvc.VerifyPassword(c.Request().Context(), currentUser.ID, req.Password); err != nil {
		return echo.NewHTTPError(http.StatusForbidden, "incorrect password")
	}

	if err := h.authSvc.DisableTwoFactor(c.Request().Context(), currentUser.ID, req.Code, req.RecoveryCode); err != nil {
		if errors.Is(err, services.ErrInvalidTwoFactor) {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid two-factor code")
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]bool{"enabled": false})
}

func (h *AuthHandler) TwoFactorRegenerateRecoveryCodes(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	var req struct {
		Password     string `json:"password"`
		Code         string `json:"code"`
		RecoveryCode string `json:"recovery_code"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "password is required")
	}
	if req.Code == "" && req.RecoveryCode == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "authenticator code or recovery code is required")
	}
	if err := h.authSvc.VerifyPassword(c.Request().Context(), currentUser.ID, req.Password); err != nil {
		return echo.NewHTTPError(http.StatusForbidden, "incorrect password")
	}

	recoveryCodes, err := h.authSvc.RegenerateTwoFactorRecoveryCodes(c.Request().Context(), currentUser.ID, req.Code, req.RecoveryCode)
	if err != nil {
		if errors.Is(err, services.ErrInvalidTwoFactor) {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid two-factor code")
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"recovery_codes": recoveryCodes,
	})
}

func (h *AuthHandler) Me(c echo.Context) error {
	user := c.Get("user").(*models.User)
	return c.JSON(http.StatusOK, h.withAbsoluteAvatarURL(c, user))
}

func (h *AuthHandler) RepoCreationStatus(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	canCreate := canCreateRepositories(h.cfg, currentUser)
	selfHostURL := "https://github.com/gitpier/gitpier"
	if h.cfg != nil && strings.TrimSpace(h.cfg.SelfHostURL) != "" {
		selfHostURL = strings.TrimSpace(h.cfg.SelfHostURL)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"can_create_repositories": canCreate,
		"restricted":              h.cfg != nil && h.cfg.RestrictRepoCreation,
		"self_host_url":           selfHostURL,
	})
}

func (h *AuthHandler) VerifyPassword(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	var req struct {
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "password is required")
	}

	if err := h.authSvc.VerifyPassword(c.Request().Context(), currentUser.ID, req.Password); err != nil {
		return echo.NewHTTPError(http.StatusForbidden, "incorrect password")
	}

	return c.NoContent(http.StatusNoContent)
}

// ChangePassword lets an authenticated user update their password.
// The current password must be supplied for verification. All existing
// sessions are invalidated by incrementing the token version.
func (h *AuthHandler) ChangePassword(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.CurrentPassword == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "current_password is required")
	}
	if len(req.NewPassword) < 8 {
		return echo.NewHTTPError(http.StatusBadRequest, "new password must be at least 8 characters")
	}
	if req.CurrentPassword == req.NewPassword {
		return echo.NewHTTPError(http.StatusBadRequest, "new password must differ from the current password")
	}

	if err := h.authSvc.ChangePassword(c.Request().Context(), currentUser.ID, req.CurrentPassword, req.NewPassword); err != nil {
		if errors.Is(err, services.ErrInvalidCredentials) {
			return echo.NewHTTPError(http.StatusForbidden, "incorrect current password")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to change password")
	}

	// Issue a fresh token for the new session so the caller stays logged in.
	newToken, err := h.authSvc.IssueSessionToken(c.Request().Context(), currentUser.ID, c.RealIP(), c.Request().UserAgent())
	if err != nil {
		// Non-fatal: the password was changed. The client will be asked to re-login.
		h.clearAuthCookie(c)
		return c.NoContent(http.StatusNoContent)
	}
	h.setAuthCookie(c, newToken)
	return c.JSON(http.StatusOK, map[string]string{"token": newToken})
}

// ForgotPassword accepts an email address and generates a single-use, time-limited
// reset token. The token is logged to the console ([DEBUG]) instead of being emailed.
// The response is always 200 to prevent account enumeration.
func (h *AuthHandler) ForgotPassword(c echo.Context) error {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Email == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "email is required")
	}

	// Errors are intentionally swallowed to avoid leaking whether the email exists.
	_ = h.authSvc.RequestPasswordReset(c.Request().Context(), req.Email)

	return c.JSON(http.StatusOK, map[string]string{
		"message": "If an account with that email exists, a password reset token has been generated. Check the server console.",
	})
}

// ResetPassword consumes a password-reset token and updates the user's password.
// All existing sessions are invalidated.
func (h *AuthHandler) ResetPassword(c echo.Context) error {
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Token == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "token is required")
	}
	if len(req.NewPassword) < 8 {
		return echo.NewHTTPError(http.StatusBadRequest, "new password must be at least 8 characters")
	}

	if err := h.authSvc.ResetPassword(c.Request().Context(), req.Token, req.NewPassword); err != nil {
		if errors.Is(err, services.ErrInvalidCredentials) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid or expired reset token")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to reset password")
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Password has been reset. You can now sign in with your new password.",
	})
}

// ListSessions returns all active sessions for the authenticated user.
func (h *AuthHandler) ListSessions(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	currentTokenID, _ := c.Get(middleware.SessionIDKey).(string)

	sessions, err := h.authSvc.ListSessions(c.Request().Context(), currentUser.ID, currentTokenID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list sessions")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"sessions": sessions})
}

// RevokeSession revokes a single session by its token ID.
func (h *AuthHandler) RevokeSession(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	tokenID := c.Param("token_id")
	if tokenID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "token_id is required")
	}
	// Prevent revoking the current session via this endpoint (use logout for that).
	currentTokenID, _ := c.Get(middleware.SessionIDKey).(string)
	if tokenID == currentTokenID {
		return echo.NewHTTPError(http.StatusBadRequest, "use the logout endpoint to end the current session")
	}
	if err := h.authSvc.RevokeSession(c.Request().Context(), currentUser.ID, tokenID); err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to revoke session")
	}
	return c.NoContent(http.StatusNoContent)
}

// RevokeOtherSessions revokes all sessions except the current one.
func (h *AuthHandler) RevokeOtherSessions(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	currentTokenID, _ := c.Get(middleware.SessionIDKey).(string)
	if err := h.authSvc.RevokeOtherSessions(c.Request().Context(), currentUser.ID, currentTokenID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to revoke sessions")
	}
	return c.NoContent(http.StatusNoContent)
}
