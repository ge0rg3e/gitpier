package handlers

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

const markdownAssetMaxBytes int64 = 200 << 20 // 200 MiB

type MarkdownAssetHandler struct {
	repoSvc    *services.RepoService
	assetsPath string
}

func NewMarkdownAssetHandler(repoSvc *services.RepoService, assetsPath string) *MarkdownAssetHandler {
	return &MarkdownAssetHandler{repoSvc: repoSvc, assetsPath: assetsPath}
}

func (h *MarkdownAssetHandler) resolveRepo(c echo.Context) (*models.Repository, error) {
	namespace := c.Param("username")
	repoName := c.Param("repo")

	repo, err := h.repoSvc.GetByOwnerAndName(c.Request().Context(), namespace, repoName)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}
	return repo, nil
}

func (h *MarkdownAssetHandler) requireRead(c echo.Context, repo *models.Repository) error {
	if !repo.IsPrivate {
		return nil
	}
	currentUser, _ := c.Get("user").(*models.User)
	if currentUser == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}
	if !h.repoSvc.HasAccess(repo, currentUser.ID, false) {
		return echo.NewHTTPError(http.StatusForbidden, "insufficient permissions")
	}
	return nil
}

func (h *MarkdownAssetHandler) requireWrite(c echo.Context, repo *models.Repository) error {
	currentUser, _ := c.Get("user").(*models.User)
	if currentUser == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}
	if !h.repoSvc.HasAccess(repo, currentUser.ID, true) {
		return echo.NewHTTPError(http.StatusForbidden, "insufficient permissions")
	}
	return nil
}

func (h *MarkdownAssetHandler) repoDir(repoID string) string {
	return filepath.Join(h.assetsPath, fmt.Sprintf("%d", repoID))
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func sanitizeFilename(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "asset"
	}
	base = strings.ReplaceAll(base, " ", "-")
	builder := strings.Builder{}
	for _, r := range base {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			builder.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	cleaned := strings.Trim(builder.String(), "-.")
	if cleaned == "" {
		return "asset"
	}
	if len(cleaned) > 120 {
		return cleaned[:120]
	}
	return cleaned
}

func markdownForAsset(fileName, assetURL, contentType string) string {
	if strings.HasPrefix(contentType, "image/") {
		return fmt.Sprintf("![%s](%s)", fileName, assetURL)
	}
	return fmt.Sprintf("[%s](%s)", fileName, assetURL)
}

func findExistingAssetByHash(dir, hash string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) > len(hash) && strings.HasPrefix(name, hash) && name[len(hash)] == '.' {
			return name, nil
		}
	}
	return "", nil
}

// Upload handles POST /repos/:username/:repo/markdown-assets
func (h *MarkdownAssetHandler) Upload(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}
	if err := h.requireWrite(c, repo); err != nil {
		return err
	}

	file, header, err := c.Request().FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file is required")
	}
	defer file.Close()

	if header.Size > 0 && header.Size > markdownAssetMaxBytes {
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "file exceeds 200MB limit")
	}

	buffered := bufio.NewReader(file)
	sniff, _ := buffered.Peek(512)
	if len(sniff) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "file is empty")
	}

	contentType := http.DetectContentType(sniff)
	if !strings.HasPrefix(contentType, "image/") && !strings.HasPrefix(contentType, "video/") {
		return echo.NewHTTPError(http.StatusBadRequest, "only image and video files are supported")
	}

	safeName := sanitizeFilename(header.Filename)
	ext := strings.ToLower(filepath.Ext(safeName))
	if ext == "" {
		if exts, _ := mime.ExtensionsByType(contentType); len(exts) > 0 {
			ext = exts[0]
		} else if strings.HasPrefix(contentType, "image/") {
			ext = ".png"
		} else {
			ext = ".mp4"
		}
	}

	dir := h.repoDir(repo.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create asset directory")
	}

	rnd, err := randomHex(8)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate asset name")
	}
	tmpName := fmt.Sprintf("upload-%d-%s.tmp", time.Now().UnixNano(), rnd)
	tmpPath := filepath.Join(dir, tmpName)
	out, err := os.Create(tmpPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save asset")
	}
	defer out.Close()
	defer os.Remove(tmpPath)

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(out, hasher), io.LimitReader(buffered, markdownAssetMaxBytes+1))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to read uploaded file")
	}
	if written == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "file is empty")
	}
	if written > markdownAssetMaxBytes {
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "file exceeds 200MB limit")
	}
	if err := out.Close(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to finalize uploaded file")
	}

	hashHex := hex.EncodeToString(hasher.Sum(nil))
	if existingName, err := findExistingAssetByHash(dir, hashHex); err == nil && existingName != "" {
		assetURL := fmt.Sprintf("/api/v1/repos/%s/%s/markdown-assets/%s", c.Param("username"), c.Param("repo"), existingName)
		return c.JSON(http.StatusCreated, map[string]interface{}{
			"asset_url":     assetURL,
			"content_type":  contentType,
			"original_name": safeName,
			"markdown":      markdownForAsset(safeName, assetURL, contentType),
		})
	} else if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to check existing assets")
	}

	storedName := fmt.Sprintf("%s%s", hashHex, ext)
	finalPath := filepath.Join(dir, storedName)
	if _, err := os.Stat(finalPath); err == nil {
		assetURL := fmt.Sprintf("/api/v1/repos/%s/%s/markdown-assets/%s", c.Param("username"), c.Param("repo"), storedName)
		return c.JSON(http.StatusCreated, map[string]interface{}{
			"asset_url":     assetURL,
			"content_type":  contentType,
			"original_name": safeName,
			"markdown":      markdownForAsset(safeName, assetURL, contentType),
		})
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to finalize uploaded asset")
	}

	assetURL := fmt.Sprintf("/api/v1/repos/%s/%s/markdown-assets/%s", c.Param("username"), c.Param("repo"), storedName)
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"asset_url":     assetURL,
		"content_type":  contentType,
		"original_name": safeName,
		"markdown":      markdownForAsset(safeName, assetURL, contentType),
	})
}

// Download handles GET /repos/:username/:repo/markdown-assets/:asset
func (h *MarkdownAssetHandler) Download(c echo.Context) error {
	repo, err := h.resolveRepo(c)
	if err != nil {
		return err
	}
	if err := h.requireRead(c, repo); err != nil {
		return err
	}

	asset := c.Param("asset")
	if asset == "" || strings.Contains(asset, "/") || strings.Contains(asset, "\\") || strings.Contains(asset, "..") {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid asset path")
	}

	path := filepath.Join(h.repoDir(repo.ID), asset)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return echo.NewHTTPError(http.StatusNotFound, "asset not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read asset")
	}

	c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	c.Response().Header().Set("Content-Disposition", "inline")
	return c.File(path)
}
