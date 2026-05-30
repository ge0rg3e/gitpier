package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gitpier/internal/models"

	"gorm.io/gorm"
)

var (
	ErrBlobNotFound     = errors.New("blob not found")
	ErrManifestNotFound = errors.New("manifest not found")
	ErrUploadNotFound   = errors.New("upload session not found")
	ErrDigestMismatch   = errors.New("digest mismatch")
	ErrPackageNotFound  = errors.New("package not found")
)

// PackageService manages OCI container image storage.
type PackageService struct {
	db           *gorm.DB
	packagesPath string // e.g. "./data/packages"
}

func NewPackageService(db *gorm.DB, packagesPath string) *PackageService {
	return &PackageService{db: db, packagesPath: packagesPath}
}

// blobsDir returns the directory where blobs are stored.
func (s *PackageService) blobsDir() string {
	return filepath.Join(s.packagesPath, "blobs")
}

// uploadsDir returns the directory for in-progress uploads.
func (s *PackageService) uploadsDir() string {
	return filepath.Join(s.packagesPath, "uploads")
}

// EnsureDirs creates the required storage directories.
func (s *PackageService) EnsureDirs() error {
	for _, d := range []string{s.blobsDir(), s.uploadsDir()} {
		if err := os.MkdirAll(d, 0750); err != nil {
			return err
		}
	}
	return nil
}

// EnsureRepo fetches or creates a ContainerRepository for the given namespace/name.
func (s *PackageService) EnsureRepo(ctx context.Context, namespace, name string, ownerID string, ownerType string) (*models.ContainerRepository, error) {
	var repo models.ContainerRepository
	err := s.db.WithContext(ctx).
		Where("namespace = ? AND name = ?", namespace, name).
		First(&repo).Error
	if err == nil {
		return &repo, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	repo = models.ContainerRepository{
		Namespace: namespace,
		Name:      name,
		IsPublic:  true,
		OwnerID:   ownerID,
		OwnerType: ownerType,
	}
	if err := s.db.WithContext(ctx).Create(&repo).Error; err != nil {
		return nil, err
	}
	return &repo, nil
}

// GetRepo looks up a container repository.
func (s *PackageService) GetRepo(ctx context.Context, namespace, name string) (*models.ContainerRepository, error) {
	var repo models.ContainerRepository
	if err := s.db.WithContext(ctx).
		Where("namespace = ? AND name = ?", namespace, name).
		First(&repo).Error; err != nil {
		return nil, err
	}
	return &repo, nil
}

// ListRepos returns all container repositories for a namespace.
func (s *PackageService) ListRepos(ctx context.Context, namespace string) ([]models.ContainerRepository, error) {
	var repos []models.ContainerRepository
	if err := s.db.WithContext(ctx).
		Where("namespace = ?", namespace).
		Order("name asc").
		Find(&repos).Error; err != nil {
		return nil, err
	}
	return repos, nil
}

// UpdateRepoVisibility updates a package visibility flag.
func (s *PackageService) UpdateRepoVisibility(ctx context.Context, namespace, name string, isPublic bool) (*models.ContainerRepository, error) {
	var repo models.ContainerRepository
	if err := s.db.WithContext(ctx).
		Where("namespace = ? AND name = ?", namespace, name).
		First(&repo).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPackageNotFound
		}
		return nil, err
	}

	if err := s.db.WithContext(ctx).Model(&repo).Update("is_public", isPublic).Error; err != nil {
		return nil, err
	}

	repo.IsPublic = isPublic
	return &repo, nil
}

// DeleteRepo removes a package and all package-scoped DB records (tags/manifests/uploads).
// Blobs are content-addressed and shared, so they are left in place for GC.
func (s *PackageService) DeleteRepo(ctx context.Context, namespace, name string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var repo models.ContainerRepository
		if err := tx.Where("namespace = ? AND name = ?", namespace, name).First(&repo).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPackageNotFound
			}
			return err
		}

		if err := tx.Where("namespace = ? AND image_name = ?", namespace, name).Delete(&models.ContainerTag{}).Error; err != nil {
			return err
		}
		if err := tx.Where("namespace = ? AND image_name = ?", namespace, name).Delete(&models.ContainerManifest{}).Error; err != nil {
			return err
		}
		if err := tx.Where("namespace = ? AND image_name = ?", namespace, name).Delete(&models.ContainerUpload{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&repo).Error; err != nil {
			return err
		}

		return nil
	})
}

// BlobExists returns true if a blob with the given digest is already stored.
func (s *PackageService) BlobExists(ctx context.Context, digest string) (*models.ContainerBlob, bool) {
	var blob models.ContainerBlob
	if err := s.db.WithContext(ctx).Where("digest = ?", digest).First(&blob).Error; err != nil {
		return nil, false
	}
	return &blob, true
}

// GetBlob returns the blob metadata for a digest.
func (s *PackageService) GetBlob(ctx context.Context, digest string) (*models.ContainerBlob, error) {
	var blob models.ContainerBlob
	if err := s.db.WithContext(ctx).Where("digest = ?", digest).First(&blob).Error; err != nil {
		return nil, ErrBlobNotFound
	}
	return &blob, nil
}

// OpenBlob returns a ReadSeekCloser for the blob's content.
func (s *PackageService) OpenBlob(ctx context.Context, digest string) (*os.File, *models.ContainerBlob, error) {
	blob, err := s.GetBlob(ctx, digest)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(blob.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("blob file missing: %w", err)
	}
	return f, blob, nil
}

// StartUpload creates a new in-progress upload session and returns the UUID.
func (s *PackageService) StartUpload(ctx context.Context, namespace, imageName string) (string, error) {
	id, err := randomHex(16)
	if err != nil {
		return "", fmt.Errorf("generate upload id: %w", err)
	}
	tmpPath := filepath.Join(s.uploadsDir(), id)
	// Create empty file to reserve the slot
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("create upload file: %w", err)
	}
	f.Close()

	upload := models.ContainerUpload{
		UUID:      id,
		Namespace: namespace,
		ImageName: imageName,
		Offset:    0,
		Path:      tmpPath,
	}
	if err := s.db.WithContext(ctx).Create(&upload).Error; err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	return id, nil
}

// AppendUpload appends data to an in-progress upload. Returns the new offset.
func (s *PackageService) AppendUpload(ctx context.Context, uploadUUID string, r io.Reader, rangeStart int64) (int64, error) {
	var upload models.ContainerUpload
	if err := s.db.WithContext(ctx).Where("uuid = ?", uploadUUID).First(&upload).Error; err != nil {
		return 0, ErrUploadNotFound
	}
	if rangeStart != upload.Offset {
		return upload.Offset, fmt.Errorf("invalid range start: expected %d, got %d", upload.Offset, rangeStart)
	}

	f, err := os.OpenFile(upload.Path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return 0, fmt.Errorf("open upload file: %w", err)
	}
	defer f.Close()

	n, err := io.Copy(f, r)
	if err != nil {
		return upload.Offset, fmt.Errorf("write upload data: %w", err)
	}

	newOffset := upload.Offset + n
	if err := s.db.WithContext(ctx).Model(&upload).Update("offset", newOffset).Error; err != nil {
		return newOffset, err
	}
	return newOffset, nil
}

// FinalizeUpload verifies the digest, moves the blob into permanent storage, and
// records it in the database. Returns the stored blob.
func (s *PackageService) FinalizeUpload(ctx context.Context, uploadUUID, expectedDigest string) (*models.ContainerBlob, error) {
	var upload models.ContainerUpload
	if err := s.db.WithContext(ctx).Where("uuid = ?", uploadUUID).First(&upload).Error; err != nil {
		return nil, ErrUploadNotFound
	}

	// Verify digest
	actualDigest, size, err := digestFile(upload.Path)
	if err != nil {
		return nil, fmt.Errorf("digest upload: %w", err)
	}
	if actualDigest != expectedDigest {
		return nil, ErrDigestMismatch
	}

	// If a blob with this digest already exists, reuse it (de-duplication)
	if existing, ok := s.BlobExists(ctx, actualDigest); ok {
		// Clean up temp file and upload record
		os.Remove(upload.Path)
		s.db.WithContext(ctx).Delete(&upload)
		return existing, nil
	}

	// Move temp file to permanent location
	blobPath := filepath.Join(s.blobsDir(), blobFilename(actualDigest))
	if err := os.Rename(upload.Path, blobPath); err != nil {
		// Rename may fail across devices; fall back to copy
		if err2 := copyBlob(upload.Path, blobPath); err2 != nil {
			return nil, fmt.Errorf("store blob: %w", err2)
		}
		os.Remove(upload.Path)
	}

	blob := models.ContainerBlob{
		Digest: actualDigest,
		Size:   size,
		Path:   blobPath,
	}
	if err := s.db.WithContext(ctx).Create(&blob).Error; err != nil {
		// Another goroutine may have raced us; return existing if so
		if existing, ok := s.BlobExists(ctx, actualDigest); ok {
			return existing, nil
		}
		return nil, err
	}

	s.db.WithContext(ctx).Delete(&upload)
	return &blob, nil
}

// CancelUpload removes the temp file and upload record.
func (s *PackageService) CancelUpload(ctx context.Context, uploadUUID string) {
	var upload models.ContainerUpload
	if err := s.db.WithContext(ctx).Where("uuid = ?", uploadUUID).First(&upload).Error; err != nil {
		return
	}
	os.Remove(upload.Path)
	s.db.WithContext(ctx).Delete(&upload)
}

// GetUploadOffset returns the current byte offset for an upload session.
func (s *PackageService) GetUploadOffset(ctx context.Context, uploadUUID string) (int64, error) {
	var upload models.ContainerUpload
	if err := s.db.WithContext(ctx).Where("uuid = ?", uploadUUID).First(&upload).Error; err != nil {
		return 0, ErrUploadNotFound
	}
	return upload.Offset, nil
}

// PutManifest stores a manifest (by digest) and updates the tag if a tag reference is given.
func (s *PackageService) PutManifest(ctx context.Context, namespace, imageName, reference, mediaType, content string) (*models.ContainerManifest, error) {
	// Compute digest of the manifest content
	h := sha256.Sum256([]byte(content))
	digest := "sha256:" + hex.EncodeToString(h[:])

	manifest := models.ContainerManifest{
		Namespace: namespace,
		ImageName: imageName,
		Digest:    digest,
		MediaType: mediaType,
		Content:   content,
		Size:      int64(len(content)),
	}

	// Upsert by digest (same content â†’ same digest)
	if err := s.db.WithContext(ctx).
		Where("digest = ?", digest).
		Assign(manifest).
		FirstOrCreate(&manifest).Error; err != nil {
		return nil, err
	}

	// If reference looks like a tag (not a digest), update/create the tag
	if !isDigestRef(reference) {
		tag := models.ContainerTag{
			Namespace: namespace,
			ImageName: imageName,
			Tag:       reference,
			Digest:    digest,
		}
		if err := s.db.WithContext(ctx).
			Omit("PullCount"). // use DB default so INSERT works before migration adds the column
			Where("namespace = ? AND image_name = ? AND tag = ?", namespace, imageName, reference).
			Assign(models.ContainerTag{Digest: digest}).
			FirstOrCreate(&tag).Error; err != nil {
			return nil, err
		}
	}

	return &manifest, nil
}

// GetManifest looks up a manifest by tag or digest reference.
func (s *PackageService) GetManifest(ctx context.Context, namespace, imageName, reference string) (*models.ContainerManifest, error) {
	digest := reference
	if !isDigestRef(reference) {
		// Resolve tag to digest
		var tag models.ContainerTag
		if err := s.db.WithContext(ctx).
			Where("namespace = ? AND image_name = ? AND tag = ?", namespace, imageName, reference).
			First(&tag).Error; err != nil {
			return nil, ErrManifestNotFound
		}
		digest = tag.Digest
	}

	var manifest models.ContainerManifest
	if err := s.db.WithContext(ctx).
		Where("digest = ?", digest).
		First(&manifest).Error; err != nil {
		return nil, ErrManifestNotFound
	}
	return &manifest, nil
}

// DeleteManifest removes a manifest by digest or tag.
func (s *PackageService) DeleteManifest(ctx context.Context, namespace, imageName, reference string) error {
	digest := reference
	if !isDigestRef(reference) {
		var tag models.ContainerTag
		if err := s.db.WithContext(ctx).
			Where("namespace = ? AND image_name = ? AND tag = ?", namespace, imageName, reference).
			First(&tag).Error; err != nil {
			return ErrManifestNotFound
		}
		digest = tag.Digest
		s.db.WithContext(ctx).Delete(&tag)
	}
	return s.db.WithContext(ctx).
		Where("namespace = ? AND image_name = ? AND digest = ?", namespace, imageName, digest).
		Delete(&models.ContainerManifest{}).Error
}

// ListTags returns a page of tags for a repository.
func (s *PackageService) ListTags(ctx context.Context, namespace, imageName string, last string, n int) ([]string, error) {
	q := s.db.WithContext(ctx).
		Model(&models.ContainerTag{}).
		Where("namespace = ? AND image_name = ?", namespace, imageName).
		Order("tag asc")

	if last != "" {
		q = q.Where("tag > ?", last)
	}
	if n > 0 {
		q = q.Limit(n)
	}

	var tags []models.ContainerTag
	if err := q.Find(&tags).Error; err != nil {
		return nil, err
	}

	names := make([]string, len(tags))
	for i, t := range tags {
		names[i] = t.Tag
	}
	return names, nil
}

// IncrementTagPullCount atomically increments the pull_count for a tag.
func (s *PackageService) IncrementTagPullCount(ctx context.Context, namespace, imageName, tag string) {
	s.db.WithContext(ctx).Model(&models.ContainerTag{}).
		Where("namespace = ? AND image_name = ? AND tag = ?", namespace, imageName, tag).
		UpdateColumn("pull_count", gorm.Expr("pull_count + 1"))
}

// ListTagEntries returns tag rows ordered by most recently updated.
func (s *PackageService) ListTagEntries(ctx context.Context, namespace, imageName string, limit int) ([]models.ContainerTag, error) {
	q := s.db.WithContext(ctx).
		Model(&models.ContainerTag{}).
		Where("namespace = ? AND image_name = ?", namespace, imageName).
		Order("updated_at desc")

	if limit > 0 {
		q = q.Limit(limit)
	}

	var tags []models.ContainerTag
	if err := q.Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

func digestFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), n, nil
}

func blobFilename(digest string) string {
	// Replace ":" with "-" to make a safe filename
	for i, c := range digest {
		if c == ':' {
			return digest[:i] + "-" + digest[i+1:]
		}
	}
	return digest
}

func isDigestRef(ref string) bool {
	return len(ref) > 7 && ref[:7] == "sha256:"
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func copyBlob(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
