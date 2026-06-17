package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gitpier/internal/config"
	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

const (
	avatarMaxBytes = 2 << 20 // 2 MiB
)

// allowedMIME maps detected content-type → file extension.
var allowedMIME = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

type AvatarHandler struct {
	cfg     *config.Config
	authSvc *services.AuthService
	orgSvc  *services.OrgService
}

func NewAvatarHandler(cfg *config.Config, authSvc *services.AuthService, orgSvc *services.OrgService) *AvatarHandler {
	return &AvatarHandler{cfg: cfg, authSvc: authSvc, orgSvc: orgSvc}
}

// UploadUserAvatar handles POST /api/v1/users/me/avatar
func (h *AvatarHandler) UploadUserAvatar(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	data, ext, err := h.readAndValidate(c)
	if err != nil {
		return err
	}

	// Store under avatars/users/<userID>/
	dir := filepath.Join(h.cfg.AvatarsPath, "users", currentUser.ID)
	url, err := h.saveFile(dir, ext, data)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save avatar")
	}

	if err := h.authSvc.UpdateUser(c.Request().Context(), currentUser.ID, map[string]interface{}{
		"avatar_url": url,
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update avatar")
	}

	user, _ := h.authSvc.GetUserByID(c.Request().Context(), currentUser.ID)
	return c.JSON(http.StatusOK, map[string]interface{}{"avatar_url": user.AvatarURL})
}

// UploadOrgAvatar handles POST /api/v1/orgs/:orgname/avatar
func (h *AvatarHandler) UploadOrgAvatar(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	orgname := c.Param("orgname")
	org, err := h.orgSvc.GetByLogin(c.Request().Context(), orgname)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "organization not found")
	}
	if !h.orgSvc.IsOwner(c.Request().Context(), org.ID, currentUser.ID) {
		return echo.NewHTTPError(http.StatusForbidden, "only org owners can change the avatar")
	}

	data, ext, err := h.readAndValidate(c)
	if err != nil {
		return err
	}

	// Store under avatars/orgs/<orgID>/
	dir := filepath.Join(h.cfg.AvatarsPath, "orgs", org.ID)
	url, err := h.saveFile(dir, ext, data)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save avatar")
	}

	if err := h.orgSvc.Update(c.Request().Context(), org, map[string]interface{}{
		"avatar_url": url,
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update avatar")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"avatar_url": url})
}

// readAndValidate reads the uploaded file, checks its size and MIME type via
// magic bytes (not trusting the client Content-Type or filename extension).
// Returns the file bytes and the canonical extension.
func (h *AvatarHandler) readAndValidate(c echo.Context) ([]byte, string, error) {
	file, _, err := c.Request().FormFile("avatar")
	if err != nil {
		return nil, "", echo.NewHTTPError(http.StatusBadRequest, "missing avatar file")
	}
	defer file.Close()

	// Read up to maxBytes+1 to detect oversized uploads without buffering everything.
	limited := io.LimitReader(file, avatarMaxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", echo.NewHTTPError(http.StatusInternalServerError, "failed to read file")
	}
	if int64(len(data)) > avatarMaxBytes {
		return nil, "", echo.NewHTTPError(http.StatusRequestEntityTooLarge, "avatar must be 2 MB or smaller")
	}

	// Detect MIME type from magic bytes (first 512 bytes).
	detected := http.DetectContentType(data)
	// DetectContentType may return "image/jpeg; charset=..." — strip parameters.
	mime := strings.SplitN(detected, ";", 2)[0]
	mime = strings.TrimSpace(mime)

	ext, ok := allowedMIME[mime]
	if !ok {
		return nil, "", echo.NewHTTPError(http.StatusUnsupportedMediaType,
			"only JPEG, PNG, GIF and WebP images are allowed")
	}

	return data, ext, nil
}

// saveFile writes data into dir/<uuid><ext>, creates the directory if needed,
// and returns the public URL path (/avatars/...).
func (h *AvatarHandler) saveFile(dir, ext string, data []byte) (string, error) {
	// Ensure the directory is inside cfg.AvatarsPath (path traversal guard).
	absDir, err := filepath.Abs(dir)
	if err != nil || !strings.HasPrefix(absDir, h.cfg.AvatarsPath) {
		return "", fmt.Errorf("invalid path")
	}

	if err := os.MkdirAll(absDir, 0755); err != nil {
		return "", err
	}

	// Generate a random filename so users can't guess other people's URLs.
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	name := hex.EncodeToString(b) + ext
	dest := filepath.Join(absDir, name)

	if err := os.WriteFile(dest, data, 0644); err != nil {
		return "", err
	}

	// Return a URL relative to /avatars/ base path.
	rel, err := filepath.Rel(h.cfg.AvatarsPath, dest)
	if err != nil {
		return "", err
	}
	return "/avatars/" + filepath.ToSlash(rel), nil
}
