package middleware

import (
	"net/http"
	"strings"

	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

const UserContextKey = "user"

// SessionIDKey holds the session token ID (jti/sid) of the current JWT session.
const SessionIDKey = "session_id"

// OAuthScopesKey holds the space-delimited scopes granted to the current OAuth token.
const OAuthScopesKey = "oauth_scopes"

// resolveUser attempts to authenticate the request using either a JWT or an OAuth
// bearer token (prefixed "glo_"). Returns the authenticated user or nil.
func resolveUser(
	c echo.Context,
	authSvc *services.AuthService,
	oauthSvc *services.OAuthFlowService,
) *models.User {
	if c.Request().Method == http.MethodOptions {
		return nil
	}

	token := extractToken(c)
	if token == "" {
		return nil
	}

	// OAuth access token (glo_ prefix).
	if strings.HasPrefix(token, "glo_") && oauthSvc != nil {
		user, scopes, err := oauthSvc.LookupToken(c.Request().Context(), token)
		if err == nil {
			c.Set(OAuthScopesKey, scopes)
			return user
		}
		return nil
	}

	// JWT session token.
	claims, err := authSvc.ValidateToken(token)
	if err != nil {
		return nil
	}
	if claims.TwoFAPending {
		return nil
	}
	// Store session ID so handlers can reference it (e.g. for logout / session list).
	if claims.SessionID != "" {
		c.Set(SessionIDKey, claims.SessionID)
		// Update last_seen_at in the background — fire-and-forget.
		go authSvc.TouchSession(claims.SessionID)
	}
	user, err := authSvc.GetUserByID(c.Request().Context(), claims.UserID)
	if err != nil {
		return nil
	}
	if user.IsSuspended {
		return nil
	}
	return user
}

func Auth(authSvc *services.AuthService, oauthSvc ...*services.OAuthFlowService) echo.MiddlewareFunc {
	var oauth *services.OAuthFlowService
	if len(oauthSvc) > 0 {
		oauth = oauthSvc[0]
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if user := resolveUser(c, authSvc, oauth); user != nil {
				c.Set(UserContextKey, user)
			}
			return next(c)
		}
	}
}

// RequireAuth returns 401 if the user is not authenticated.
func RequireAuth(authSvc *services.AuthService, oauthSvc ...*services.OAuthFlowService) echo.MiddlewareFunc {
	var oauth *services.OAuthFlowService
	if len(oauthSvc) > 0 {
		oauth = oauthSvc[0]
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token := extractToken(c)
			if token == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
			}

			user := resolveUser(c, authSvc, oauth)
			if user == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
			}

			c.Set(UserContextKey, user)
			return next(c)
		}
	}
}

// RequireRepoWritable blocks mutating repo-scoped routes when the repository is archived.
func RequireRepoWritable(repoSvc *services.RepoService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			method := c.Request().Method
			if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
				return next(c)
			}

			username := c.Param("username")
			repoName := c.Param("repo")
			if username == "" || repoName == "" {
				return next(c)
			}

			path := c.Path()
			if strings.HasSuffix(path, "/star") || strings.HasSuffix(path, "/fork") || strings.HasSuffix(path, "/archive") || strings.HasSuffix(path, "/unarchive") {
				return next(c)
			}

			repo, err := repoSvc.GetByOwnerAndName(c.Request().Context(), username, repoName)
			if err != nil {
				return next(c)
			}
			if repo.IsArchived {
				return echo.NewHTTPError(http.StatusConflict, "repository is archived and read-only")
			}

			return next(c)
		}
	}
}

// RequirePasswordVerification checks the X-Confirm-Password header against the
// authenticated user's stored password hash. Apply this to destructive endpoints
// so they cannot be called without explicit password confirmation, even if the
// JWT token is stolen or the frontend dialog is bypassed.
func RequirePasswordVerification(authSvc *services.AuthService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			password := c.Request().Header.Get("X-Confirm-Password")
			if password == "" {
				return echo.NewHTTPError(http.StatusForbidden, "password confirmation required")
			}

			user, ok := c.Get(UserContextKey).(*models.User)
			if !ok || user == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
			}

			if err := authSvc.VerifyPassword(c.Request().Context(), user.ID, password); err != nil {
				return echo.NewHTTPError(http.StatusForbidden, "incorrect password")
			}

			return next(c)
		}
	}
}

func extractToken(c echo.Context) string {
	// 1. Authorization header (preferred for API clients).
	auth := c.Request().Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	// 2. HttpOnly cookie (preferred for browser sessions — inaccessible to JS).
	if cookie, err := c.Request().Cookie("gitpier_token"); err == nil && cookie.Value != "" {
		return cookie.Value
	}

	return ""
}
