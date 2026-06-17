package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

// RegistryHandler implements the OCI Distribution Spec v2 for container images.
// Routes are registered under /v2/ (outside /api/v1).
type RegistryHandler struct {
	pkgSvc  *services.PackageService
	authSvc *services.AuthService
	orgSvc  *services.OrgService
	appURL  string // base URL for constructing Location headers
}

func NewRegistryHandler(pkgSvc *services.PackageService, authSvc *services.AuthService, orgSvc *services.OrgService, appURL string) *RegistryHandler {
	return &RegistryHandler{pkgSvc: pkgSvc, authSvc: authSvc, orgSvc: orgSvc, appURL: appURL}
}

// wwwAuthenticate sends the standard Docker 401 with a WWW-Authenticate challenge.
func (h *RegistryHandler) unauthorizedChallenge(c echo.Context) error {
	realm := h.appURL + "/v2/token"
	c.Response().Header().Set("WWW-Authenticate",
		`Bearer realm="`+realm+`",service="registry"`)
	return c.JSON(http.StatusUnauthorized, map[string]interface{}{
		"errors": []map[string]string{{"code": "UNAUTHORIZED", "message": "authentication required"}},
	})
}

// resolveRegistryUser extracts the authenticated user from the request.
// Accepts: Bearer registry-JWT, or HTTP Basic auth (username:password).
// Returns nil if the request carries no credentials (anonymous).
func (h *RegistryHandler) resolveRegistryUser(c echo.Context) (*models.User, error) {
	authHeader := c.Request().Header.Get("Authorization")
	if authHeader == "" {
		return nil, nil
	}

	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := h.authSvc.ValidateRegistryToken(token)
		if err != nil {
			return nil, err
		}
		return h.authSvc.GetUserByID(c.Request().Context(), claims.UserID)
	}

	if strings.HasPrefix(authHeader, "Basic ") {
		username, password, ok := c.Request().BasicAuth()
		if !ok {
			return nil, errors.New("invalid basic auth")
		}
		return h.authSvc.ValidateCredentials(c.Request().Context(), username, password)
	}

	return nil, nil
}

// requireRegistryAuth resolves the user and returns 401 with a challenge if anonymous.
func (h *RegistryHandler) requireRegistryAuth(c echo.Context) (*models.User, error) {
	user, err := h.resolveRegistryUser(c)
	if err != nil || user == nil {
		return nil, h.unauthorizedChallenge(c)
	}
	return user, nil
}

// canPush returns true if the user may push to the namespace.
func (h *RegistryHandler) canPush(c echo.Context, user *models.User, namespace string) bool {
	if user == nil {
		return false
	}
	if strings.EqualFold(user.Username, namespace) {
		return true
	}
	// Check if namespace is an org the user owns
	org, err := h.orgSvc.GetByLogin(c.Request().Context(), namespace)
	if err != nil {
		return false
	}
	return h.orgSvc.IsOwner(c.Request().Context(), org.ID, user.ID)
}

// canPull returns true if user may read from the namespace.
// For public images anyone can pull; for private images the user must be owner/member.
func (h *RegistryHandler) canPull(c echo.Context, user *models.User, namespace string, isPublic bool) bool {
	if isPublic {
		return true
	}
	if user == nil {
		return false
	}
	if strings.EqualFold(user.Username, namespace) {
		return true
	}
	org, err := h.orgSvc.GetByLogin(c.Request().Context(), namespace)
	if err != nil {
		return false
	}
	return h.orgSvc.IsMember(c.Request().Context(), org.ID, user.ID)
}

// Token issues a short-lived registry JWT.
// GET /v2/token?service=registry&scope=...
// Accepts Basic auth or an existing Bearer session token.
func (h *RegistryHandler) Token(c echo.Context) error {
	user, err := h.resolveRegistryUser(c)
	if err != nil || user == nil {
		return h.unauthorizedChallenge(c)
	}

	token, err := h.authSvc.IssueRegistryToken(user)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "could not issue token")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"token":      token,
		"expires_in": 900,
	})
}

// CheckAPI implements GET /v2/
func (h *RegistryHandler) CheckAPI(c echo.Context) error {
	// Public registries should allow anonymous /v2/ checks.
	// If auth is provided but invalid, return a standard challenge.
	if _, err := h.resolveRegistryUser(c); err != nil {
		return h.unauthorizedChallenge(c)
	}
	c.Response().Header().Set("Docker-Distribution-API-Version", "registry/2.0")
	return c.JSON(http.StatusOK, map[string]string{})
}

// HeadBlob implements HEAD /v2/:namespace/:image/blobs/:digest
func (h *RegistryHandler) HeadBlob(c echo.Context) error {
	user, err := h.resolveRegistryUser(c)
	if err != nil {
		return h.unauthorizedChallenge(c)
	}
	namespace := c.Param("namespace")
	digest := c.Param("digest")

	blob, ok := h.pkgSvc.BlobExists(c.Request().Context(), digest)
	if !ok {
		return c.JSON(http.StatusNotFound, ociError("BLOB_UNKNOWN", "blob unknown to registry"))
	}

	// Check read access
	repo, _ := h.pkgSvc.GetRepo(c.Request().Context(), namespace, c.Param("image"))
	isPublic := repo == nil || repo.IsPublic
	if !h.canPull(c, user, namespace, isPublic) {
		return h.unauthorizedChallenge(c)
	}

	c.Response().Header().Set("Docker-Content-Digest", blob.Digest)
	c.Response().Header().Set("Content-Length", strconv.FormatInt(blob.Size, 10))
	c.Response().Header().Set("Content-Type", "application/octet-stream")
	return c.NoContent(http.StatusOK)
}

// GetBlob implements GET /v2/:namespace/:image/blobs/:digest
func (h *RegistryHandler) GetBlob(c echo.Context) error {
	user, err := h.resolveRegistryUser(c)
	if err != nil {
		return h.unauthorizedChallenge(c)
	}
	namespace := c.Param("namespace")
	digest := c.Param("digest")

	// Check read access
	repo, _ := h.pkgSvc.GetRepo(c.Request().Context(), namespace, c.Param("image"))
	isPublic := repo == nil || repo.IsPublic
	if !h.canPull(c, user, namespace, isPublic) {
		return h.unauthorizedChallenge(c)
	}

	f, blob, err := h.pkgSvc.OpenBlob(c.Request().Context(), digest)
	if err != nil {
		return c.JSON(http.StatusNotFound, ociError("BLOB_UNKNOWN", "blob unknown to registry"))
	}
	defer f.Close()

	c.Response().Header().Set("Docker-Content-Digest", blob.Digest)
	c.Response().Header().Set("Content-Length", strconv.FormatInt(blob.Size, 10))
	return c.Stream(http.StatusOK, "application/octet-stream", f)
}

// DeleteBlob implements DELETE /v2/:namespace/:image/blobs/:digest
func (h *RegistryHandler) DeleteBlob(c echo.Context) error {
	user, err := h.requireRegistryAuth(c)
	if err != nil {
		return err
	}
	namespace := c.Param("namespace")
	if !h.canPush(c, user, namespace) {
		return c.JSON(http.StatusForbidden, ociError("DENIED", "push access denied"))
	}
	// Blobs are shared; we don't delete them here â€” a garbage collector would do that.
	// Respond 202 Accepted as per the OCI spec.
	return c.NoContent(http.StatusAccepted)
}

// StartBlobUpload implements POST /v2/:namespace/:image/blobs/uploads/
func (h *RegistryHandler) StartBlobUpload(c echo.Context) error {
	user, err := h.requireRegistryAuth(c)
	if err != nil {
		return err
	}
	namespace := c.Param("namespace")
	imageName := c.Param("image")

	if !h.canPush(c, user, namespace) {
		return c.JSON(http.StatusForbidden, ociError("DENIED", "push access denied"))
	}

	// Ensure namespace is owner; create repo if needed.
	ownerType, ownerID := h.resolveOwner(c, user, namespace)
	if ownerID == "" {
		return c.JSON(http.StatusForbidden, ociError("DENIED", "namespace not found"))
	}
	h.pkgSvc.EnsureRepo(c.Request().Context(), namespace, imageName, ownerID, ownerType)

	// Check if a complete monolithic upload (content in body with digest param)
	if digest := c.QueryParam("digest"); digest != "" {
		uploadUUID, err := h.pkgSvc.StartUpload(c.Request().Context(), namespace, imageName)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "start upload failed")
		}
		if _, err := h.pkgSvc.AppendUpload(c.Request().Context(), uploadUUID, c.Request().Body, 0); err != nil {
			h.pkgSvc.CancelUpload(c.Request().Context(), uploadUUID)
			return echo.NewHTTPError(http.StatusInternalServerError, "upload write failed")
		}
		blob, err := h.pkgSvc.FinalizeUpload(c.Request().Context(), uploadUUID, digest)
		if err != nil {
			if errors.Is(err, services.ErrDigestMismatch) {
				return c.JSON(http.StatusBadRequest, ociError("DIGEST_INVALID", "digest mismatch"))
			}
			return echo.NewHTTPError(http.StatusInternalServerError, "finalize failed")
		}
		c.Response().Header().Set("Docker-Content-Digest", blob.Digest)
		c.Response().Header().Set("Location", h.blobURL(namespace, imageName, blob.Digest))
		return c.NoContent(http.StatusCreated)
	}

	uploadUUID, err := h.pkgSvc.StartUpload(c.Request().Context(), namespace, imageName)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "start upload failed")
	}
	c.Response().Header().Set("Location", h.uploadURL(namespace, imageName, uploadUUID))
	c.Response().Header().Set("Docker-Upload-UUID", uploadUUID)
	c.Response().Header().Set("Range", "0-0")
	return c.NoContent(http.StatusAccepted)
}

// PatchBlobUpload implements PATCH /v2/:namespace/:image/blobs/uploads/:uuid
func (h *RegistryHandler) PatchBlobUpload(c echo.Context) error {
	user, err := h.requireRegistryAuth(c)
	if err != nil {
		return err
	}
	namespace := c.Param("namespace")
	uploadUUID := c.Param("uuid")

	if !h.canPush(c, user, namespace) {
		return c.JSON(http.StatusForbidden, ociError("DENIED", "push access denied"))
	}

	// Parse Content-Range header to get start offset
	var rangeStart int64
	if cr := c.Request().Header.Get("Content-Range"); cr != "" {
		fmt.Sscanf(cr, "%d-", &rangeStart)
	}

	newOffset, err := h.pkgSvc.AppendUpload(c.Request().Context(), uploadUUID, c.Request().Body, rangeStart)
	if err != nil {
		if errors.Is(err, services.ErrUploadNotFound) {
			return c.JSON(http.StatusNotFound, ociError("BLOB_UPLOAD_UNKNOWN", "upload not found"))
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "write failed")
	}

	imageName := c.Param("image")
	c.Response().Header().Set("Location", h.uploadURL(namespace, imageName, uploadUUID))
	c.Response().Header().Set("Docker-Upload-UUID", uploadUUID)
	c.Response().Header().Set("Range", "0-"+strconv.FormatInt(newOffset-1, 10))
	return c.NoContent(http.StatusAccepted)
}

// PutBlobUpload implements PUT /v2/:namespace/:image/blobs/uploads/:uuid
func (h *RegistryHandler) PutBlobUpload(c echo.Context) error {
	user, err := h.requireRegistryAuth(c)
	if err != nil {
		return err
	}
	namespace := c.Param("namespace")
	imageName := c.Param("image")
	uploadUUID := c.Param("uuid")
	digest := c.QueryParam("digest")

	if !h.canPush(c, user, namespace) {
		return c.JSON(http.StatusForbidden, ociError("DENIED", "push access denied"))
	}
	if digest == "" {
		return c.JSON(http.StatusBadRequest, ociError("DIGEST_INVALID", "digest parameter required"))
	}

	// Append any remaining body data
	if c.Request().ContentLength != 0 {
		offset, _ := h.pkgSvc.GetUploadOffset(c.Request().Context(), uploadUUID)
		h.pkgSvc.AppendUpload(c.Request().Context(), uploadUUID, c.Request().Body, offset)
	}

	blob, err := h.pkgSvc.FinalizeUpload(c.Request().Context(), uploadUUID, digest)
	if err != nil {
		if errors.Is(err, services.ErrDigestMismatch) {
			return c.JSON(http.StatusBadRequest, ociError("DIGEST_INVALID", "digest mismatch"))
		}
		if errors.Is(err, services.ErrUploadNotFound) {
			return c.JSON(http.StatusNotFound, ociError("BLOB_UPLOAD_UNKNOWN", "upload not found"))
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "finalize failed")
	}

	c.Response().Header().Set("Docker-Content-Digest", blob.Digest)
	c.Response().Header().Set("Location", h.blobURL(namespace, imageName, blob.Digest))
	return c.NoContent(http.StatusCreated)
}

// HeadManifest implements HEAD /v2/:namespace/:image/manifests/:reference
func (h *RegistryHandler) HeadManifest(c echo.Context) error {
	user, err := h.resolveRegistryUser(c)
	if err != nil {
		return h.unauthorizedChallenge(c)
	}
	namespace, imageName, reference := c.Param("namespace"), c.Param("image"), c.Param("reference")

	manifest, err := h.pkgSvc.GetManifest(c.Request().Context(), namespace, imageName, reference)
	if err != nil {
		return c.JSON(http.StatusNotFound, ociError("MANIFEST_UNKNOWN", "manifest unknown"))
	}

	repo, _ := h.pkgSvc.GetRepo(c.Request().Context(), namespace, imageName)
	isPublic := repo == nil || repo.IsPublic
	if !h.canPull(c, user, namespace, isPublic) {
		return h.unauthorizedChallenge(c)
	}

	c.Response().Header().Set("Docker-Content-Digest", manifest.Digest)
	c.Response().Header().Set("Content-Type", manifest.MediaType)
	c.Response().Header().Set("Content-Length", strconv.FormatInt(manifest.Size, 10))
	return c.NoContent(http.StatusOK)
}

// GetManifest implements GET /v2/:namespace/:image/manifests/:reference
func (h *RegistryHandler) GetManifest(c echo.Context) error {
	user, err := h.resolveRegistryUser(c)
	if err != nil {
		return h.unauthorizedChallenge(c)
	}
	namespace, imageName, reference := c.Param("namespace"), c.Param("image"), c.Param("reference")

	manifest, err := h.pkgSvc.GetManifest(c.Request().Context(), namespace, imageName, reference)
	if err != nil {
		return c.JSON(http.StatusNotFound, ociError("MANIFEST_UNKNOWN", "manifest unknown"))
	}

	repo, _ := h.pkgSvc.GetRepo(c.Request().Context(), namespace, imageName)
	isPublic := repo == nil || repo.IsPublic
	if !h.canPull(c, user, namespace, isPublic) {
		return h.unauthorizedChallenge(c)
	}

	c.Response().Header().Set("Docker-Content-Digest", manifest.Digest)

	// Increment pull count when a tag reference (not a digest) is resolved.
	if !strings.HasPrefix(reference, "sha256:") {
		go h.pkgSvc.IncrementTagPullCount(c.Request().Context(), namespace, imageName, reference)
	}

	return c.Blob(http.StatusOK, manifest.MediaType, []byte(manifest.Content))
}

// PutManifest implements PUT /v2/:namespace/:image/manifests/:reference
func (h *RegistryHandler) PutManifest(c echo.Context) error {
	user, err := h.requireRegistryAuth(c)
	if err != nil {
		return err
	}
	namespace, imageName, reference := c.Param("namespace"), c.Param("image"), c.Param("reference")

	if !h.canPush(c, user, namespace) {
		return c.JSON(http.StatusForbidden, ociError("DENIED", "push access denied"))
	}

	body, err := io.ReadAll(io.LimitReader(c.Request().Body, 4*1024*1024)) // 4 MB max for a manifest
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "could not read body")
	}

	mediaType := c.Request().Header.Get("Content-Type")
	if mediaType == "" {
		mediaType = "application/vnd.docker.distribution.manifest.v2+json"
	}

	// Ensure repo exists
	ownerType, ownerID := h.resolveOwner(c, user, namespace)
	if ownerID != "" {
		h.pkgSvc.EnsureRepo(c.Request().Context(), namespace, imageName, ownerID, ownerType)
	}

	manifest, err := h.pkgSvc.PutManifest(c.Request().Context(), namespace, imageName, reference, mediaType, string(body))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "store manifest failed")
	}

	c.Response().Header().Set("Docker-Content-Digest", manifest.Digest)
	c.Response().Header().Set("Location", h.manifestURL(namespace, imageName, manifest.Digest))
	return c.NoContent(http.StatusCreated)
}

// DeleteManifest implements DELETE /v2/:namespace/:image/manifests/:reference
func (h *RegistryHandler) DeleteManifest(c echo.Context) error {
	user, err := h.requireRegistryAuth(c)
	if err != nil {
		return err
	}
	namespace, imageName, reference := c.Param("namespace"), c.Param("image"), c.Param("reference")

	if !h.canPush(c, user, namespace) {
		return c.JSON(http.StatusForbidden, ociError("DENIED", "push access denied"))
	}

	if err := h.pkgSvc.DeleteManifest(c.Request().Context(), namespace, imageName, reference); err != nil {
		return c.JSON(http.StatusNotFound, ociError("MANIFEST_UNKNOWN", "manifest unknown"))
	}
	return c.NoContent(http.StatusAccepted)
}

// ListTags implements GET /v2/:namespace/:image/tags/list
func (h *RegistryHandler) ListTags(c echo.Context) error {
	user, err := h.resolveRegistryUser(c)
	if err != nil {
		return h.unauthorizedChallenge(c)
	}
	namespace, imageName := c.Param("namespace"), c.Param("image")

	repo, _ := h.pkgSvc.GetRepo(c.Request().Context(), namespace, imageName)
	isPublic := repo == nil || repo.IsPublic
	if !h.canPull(c, user, namespace, isPublic) {
		return h.unauthorizedChallenge(c)
	}

	n := 0
	if nStr := c.QueryParam("n"); nStr != "" {
		n, _ = strconv.Atoi(nStr)
	}
	last := c.QueryParam("last")

	tags, err := h.pkgSvc.ListTags(c.Request().Context(), namespace, imageName, last, n)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "list tags failed")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"name": namespace + "/" + imageName,
		"tags": tags,
	})
}

// ListPackages implements GET /api/v1/packages/:namespace
func (h *RegistryHandler) ListPackages(c echo.Context) error {
	namespace := c.Param("namespace")
	repos, err := h.pkgSvc.ListRepos(c.Request().Context(), namespace)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "list packages failed")
	}

	currentUser, _ := c.Get("user").(*models.User)
	visible := make([]models.ContainerRepository, 0, len(repos))
	for _, repo := range repos {
		if repo.IsPublic || h.canPull(c, currentUser, namespace, false) {
			visible = append(visible, repo)
		}
	}

	return c.JSON(http.StatusOK, visible)
}

// GetPackage implements GET /api/v1/packages/:namespace/:image.
// It returns package metadata and recent tags for the details page.
func (h *RegistryHandler) GetPackage(c echo.Context) error {
	namespace := c.Param("namespace")
	imageName := c.Param("image")

	repo, err := h.pkgSvc.GetRepo(c.Request().Context(), namespace, imageName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "package not found")
	}

	currentUser, _ := c.Get("user").(*models.User)
	if !repo.IsPublic && !h.canPull(c, currentUser, namespace, false) {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	tags, err := h.pkgSvc.ListTagEntries(c.Request().Context(), namespace, imageName, 100)
	if err != nil {
		// Non-fatal: return empty tags so the UI still loads
		tags = nil
	}

	pullTag := "latest"
	if len(tags) > 0 {
		pullTag = tags[0].Tag
	}
	pullCommand := "docker pull " + publicHost(c) + "/" + namespace + "/" + imageName + ":" + pullTag

	return c.JSON(http.StatusOK, map[string]interface{}{
		"package":      repo,
		"tags":         tags,
		"tags_count":   len(tags),
		"pull_command": pullCommand,
	})
}

// UpdatePackage implements PATCH /api/v1/packages/:namespace/:image
// Supports updating visibility for package owners (user namespace or org owners).
func (h *RegistryHandler) UpdatePackage(c echo.Context) error {
	namespace := c.Param("namespace")
	imageName := c.Param("image")

	currentUser, _ := c.Get("user").(*models.User)
	if currentUser == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}
	if !h.canPush(c, currentUser, namespace) {
		return echo.NewHTTPError(http.StatusForbidden, "only the package owner can update settings")
	}

	var req struct {
		IsPublic *bool `json:"is_public"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.IsPublic == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "is_public is required")
	}

	repo, err := h.pkgSvc.UpdateRepoVisibility(c.Request().Context(), namespace, imageName, *req.IsPublic)
	if err != nil {
		if errors.Is(err, services.ErrPackageNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "package not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update package visibility")
	}

	return c.JSON(http.StatusOK, repo)
}

// DeletePackage implements DELETE /api/v1/packages/:namespace/:image
func (h *RegistryHandler) DeletePackage(c echo.Context) error {
	namespace := c.Param("namespace")
	imageName := c.Param("image")

	currentUser, _ := c.Get("user").(*models.User)
	if currentUser == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}
	if !h.canPush(c, currentUser, namespace) {
		return echo.NewHTTPError(http.StatusForbidden, "only the package owner can delete this package")
	}

	if err := h.pkgSvc.DeleteRepo(c.Request().Context(), namespace, imageName); err != nil {
		if errors.Is(err, services.ErrPackageNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "package not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete package")
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *RegistryHandler) blobURL(namespace, image, digest string) string {
	return "/v2/" + namespace + "/" + image + "/blobs/" + digest
}

func (h *RegistryHandler) uploadURL(namespace, image, uuid string) string {
	return "/v2/" + namespace + "/" + image + "/blobs/uploads/" + uuid
}

func (h *RegistryHandler) manifestURL(namespace, image, digest string) string {
	return "/v2/" + namespace + "/" + image + "/manifests/" + digest
}

// resolveOwner resolves the ownerID and type for a namespace (user or org).
func (h *RegistryHandler) resolveOwner(c echo.Context, user *models.User, namespace string) (string, string) {
	if user != nil && strings.EqualFold(user.Username, namespace) {
		return "user", user.ID
	}
	org, err := h.orgSvc.GetByLogin(c.Request().Context(), namespace)
	if err == nil {
		return "org", org.ID
	}
	return "", ""
}

func ociError(code, message string) map[string]interface{} {
	return map[string]interface{}{
		"errors": []map[string]string{
			{"code": code, "message": message},
		},
	}
}
