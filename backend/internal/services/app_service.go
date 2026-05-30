package services

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gitpier/internal/models"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrAppNotFound          = errors.New("app not found")
	ErrAppForbidden         = errors.New("not authorized to manage this app")
	ErrAppSlugTaken         = errors.New("app slug already taken")
	ErrInstallationNotFound = errors.New("installation not found")
	ErrAppSuspended         = errors.New("app installation is suspended")
	ErrInvalidAppJWT        = errors.New("invalid app JWT")
	ErrTooManyKeys          = errors.New("app already has 10 private keys")
	ErrKeyNotFound          = errors.New("private key not found")
	ErrInvalidRepoSelection = errors.New("invalid repository selection")
	ErrRepoAccessDenied     = errors.New("selected repositories are outside installation account")
)

// slugRegexp validates URL-safe slugs: lowercase letters, digits, hyphens.
var slugRegexp = regexp.MustCompile(`^[a-z0-9][a-z0-9\-]{0,98}[a-z0-9]$|^[a-z0-9]$`)

// AppService manages gitpier App registrations, private keys,
// installations, and installation access tokens.
type AppService struct {
	db *gorm.DB
}

func NewAppService(db *gorm.DB) *AppService {
	return &AppService{db: db}
}

// generateToken creates a random hex string prefixed with the given prefix.
func generateToken(prefix string, length int) (string, error) {
	b := make([]byte, (length+1)/2)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b)[:length], nil
}

// sha256Hex returns the hex-encoded SHA-256 digest of s.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// buildSlug converts a name into a URL-safe slug.
func buildSlug(name string) string {
	slug := strings.ToLower(name)
	slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 100 {
		slug = slug[:100]
	}
	if slug == "" {
		slug = "app"
	}
	return slug
}

// uniqueSlug ensures the slug is unique, appending a numeric suffix if needed.
func (s *AppService) uniqueSlug(ctx context.Context, base string, excludeID string) string {
	slug := base
	for i := 1; ; i++ {
		var count int64
		q := s.db.WithContext(ctx).Model(&models.App{}).Where("slug = ?", slug)
		if excludeID != "" {
			q = q.Where("id != ?", excludeID)
		}
		q.Count(&count)
		if count == 0 {
			return slug
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}

type CreateAppInput struct {
	Name             string
	Description      string
	HomepageURL      string
	LogoURL          string
	SetupURL         string
	RedirectOnUpdate bool
	WebhookURL       string
	WebhookSecret    string
	WebhookActive    bool
	IsPublic         bool
	CallbackURLs     []string
	RequestUserAuth  bool
	ExpireUserTokens bool
	EnableDeviceFlow bool
	RepoPermissions  map[string]string
	OrgPermissions   map[string]string
	AcctPermissions  map[string]string
	Events           []string
	OwnerID          string
	OwnerType        string // "user" or "org"
}

// Create registers a new gitpier App. Returns the app and the plaintext client_secret.
func (s *AppService) Create(ctx context.Context, in CreateAppInput) (*models.App, string, error) {
	slug := buildSlug(in.Name)

	// App names (and their derived slugs) must be globally unique, matching GitHub's behaviour.
	var existing int64
	if err := s.db.WithContext(ctx).Model(&models.App{}).Where("slug = ?", slug).Count(&existing).Error; err != nil {
		return nil, "", fmt.Errorf("check slug uniqueness: %w", err)
	}
	if existing > 0 {
		return nil, "", ErrAppSlugTaken
	}

	clientID, err := generateHex(20)
	if err != nil {
		return nil, "", fmt.Errorf("generate client_id: %w", err)
	}
	secret, err := generateHex(40)
	if err != nil {
		return nil, "", fmt.Errorf("generate client_secret: %w", err)
	}
	secretHash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", fmt.Errorf("hash client_secret: %w", err)
	}

	app := &models.App{
		Name:             in.Name,
		Slug:             slug,
		Description:      in.Description,
		HomepageURL:      in.HomepageURL,
		LogoURL:          in.LogoURL,
		SetupURL:         in.SetupURL,
		RedirectOnUpdate: in.RedirectOnUpdate,
		WebhookURL:       in.WebhookURL,
		WebhookActive:    in.WebhookActive,
		IsPublic:         in.IsPublic,
		RequestUserAuth:  in.RequestUserAuth,
		ExpireUserTokens: in.ExpireUserTokens,
		EnableDeviceFlow: in.EnableDeviceFlow,
		ClientID:         clientID,
		ClientSecretHash: string(secretHash),
		OwnerID:          in.OwnerID,
		OwnerType:        in.OwnerType,
	}

	if in.WebhookSecret != "" {
		wsh, err := bcrypt.GenerateFromPassword([]byte(in.WebhookSecret), bcrypt.DefaultCost)
		if err != nil {
			return nil, "", fmt.Errorf("hash webhook_secret: %w", err)
		}
		app.WebhookSecretHash = string(wsh)
	}

	app.CallbackURLs = marshalJSON(in.CallbackURLs)
	app.RepoPermissions = marshalJSONMap(in.RepoPermissions)
	app.OrgPermissions = marshalJSONMap(in.OrgPermissions)
	app.AccountPermissions = marshalJSONMap(in.AcctPermissions)
	app.Events = marshalJSON(in.Events)

	if err := s.db.WithContext(ctx).Create(app).Error; err != nil {
		return nil, "", fmt.Errorf("create app: %w", err)
	}
	return app, secret, nil
}

// GetByID returns an app by primary key.
func (s *AppService) GetByID(ctx context.Context, id string) (*models.App, error) {
	var app models.App
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&app).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAppNotFound
	}
	return &app, err
}

// GetBySlug returns a publicly-visible app by its slug.
func (s *AppService) GetBySlug(ctx context.Context, slug string) (*models.App, error) {
	var app models.App
	err := s.db.WithContext(ctx).Where("slug = ?", slug).First(&app).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAppNotFound
	}
	return &app, err
}

// GetByClientID returns an app by its OAuth client ID.
func (s *AppService) GetByClientID(ctx context.Context, clientID string) (*models.App, error) {
	var app models.App
	err := s.db.WithContext(ctx).Where("client_id = ?", clientID).First(&app).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAppNotFound
	}
	return &app, err
}

// ListByOwner returns all apps owned by the given user or org.
func (s *AppService) ListByOwner(ctx context.Context, ownerID string, ownerType string) ([]models.App, error) {
	var apps []models.App
	err := s.db.WithContext(ctx).
		Where("owner_id = ? AND owner_type = ?", ownerID, ownerType).
		Order("created_at asc").
		Find(&apps).Error
	if err != nil {
		return nil, err
	}
	// Populate computed counts.
	for i := range apps {
		var instCount int64
		s.db.WithContext(ctx).Model(&models.AppInstallation{}).Where("app_id = ?", apps[i].ID).Count(&instCount)
		apps[i].InstallationCount = int(instCount)
		var keyCount int64
		s.db.WithContext(ctx).Model(&models.AppPrivateKey{}).Where("app_id = ?", apps[i].ID).Count(&keyCount)
		apps[i].KeyCount = int(keyCount)
	}
	return apps, nil
}

type UpdateAppInput struct {
	Name             *string
	Description      *string
	HomepageURL      *string
	LogoURL          *string
	SetupURL         *string
	RedirectOnUpdate *bool
	WebhookURL       *string
	WebhookSecret    *string // empty = don't change
	WebhookActive    *bool
	IsPublic         *bool
	CallbackURLs     *[]string
	RequestUserAuth  *bool
	ExpireUserTokens *bool
	EnableDeviceFlow *bool
	RepoPermissions  *map[string]string
	OrgPermissions   *map[string]string
	AcctPermissions  *map[string]string
	Events           *[]string
}

// Update applies partial changes to an app.
func (s *AppService) Update(ctx context.Context, id string, in UpdateAppInput) (*models.App, error) {
	app, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if in.Name != nil {
		app.Name = *in.Name
		updates["name"] = *in.Name
		// Update slug too
		newSlug := s.uniqueSlug(ctx, buildSlug(*in.Name), id)
		app.Slug = newSlug
		updates["slug"] = newSlug
	}
	if in.Description != nil {
		app.Description = *in.Description
		updates["description"] = *in.Description
	}
	if in.HomepageURL != nil {
		app.HomepageURL = *in.HomepageURL
		updates["homepage_url"] = *in.HomepageURL
	}
	if in.LogoURL != nil {
		app.LogoURL = *in.LogoURL
		updates["logo_url"] = *in.LogoURL
	}
	if in.SetupURL != nil {
		app.SetupURL = *in.SetupURL
		updates["setup_url"] = *in.SetupURL
	}
	if in.RedirectOnUpdate != nil {
		app.RedirectOnUpdate = *in.RedirectOnUpdate
		updates["redirect_on_update"] = *in.RedirectOnUpdate
	}
	if in.WebhookURL != nil {
		app.WebhookURL = *in.WebhookURL
		updates["webhook_url"] = *in.WebhookURL
	}
	if in.WebhookSecret != nil && *in.WebhookSecret != "" {
		wsh, err := bcrypt.GenerateFromPassword([]byte(*in.WebhookSecret), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		app.WebhookSecretHash = string(wsh)
		updates["webhook_secret_hash"] = string(wsh)
	}
	if in.WebhookActive != nil {
		app.WebhookActive = *in.WebhookActive
		updates["webhook_active"] = *in.WebhookActive
	}
	if in.IsPublic != nil {
		app.IsPublic = *in.IsPublic
		updates["is_public"] = *in.IsPublic
	}
	if in.RequestUserAuth != nil {
		app.RequestUserAuth = *in.RequestUserAuth
		updates["request_user_auth"] = *in.RequestUserAuth
	}
	if in.ExpireUserTokens != nil {
		app.ExpireUserTokens = *in.ExpireUserTokens
		updates["expire_user_tokens"] = *in.ExpireUserTokens
	}
	if in.EnableDeviceFlow != nil {
		app.EnableDeviceFlow = *in.EnableDeviceFlow
		updates["enable_device_flow"] = *in.EnableDeviceFlow
	}
	if in.CallbackURLs != nil {
		j := marshalJSON(*in.CallbackURLs)
		app.CallbackURLs = j
		updates["callback_urls"] = j
	}
	if in.RepoPermissions != nil {
		j := marshalJSONMap(*in.RepoPermissions)
		app.RepoPermissions = j
		updates["repo_permissions"] = j
	}
	if in.OrgPermissions != nil {
		j := marshalJSONMap(*in.OrgPermissions)
		app.OrgPermissions = j
		updates["org_permissions"] = j
	}
	if in.AcctPermissions != nil {
		j := marshalJSONMap(*in.AcctPermissions)
		app.AccountPermissions = j
		updates["account_permissions"] = j
	}
	if in.Events != nil {
		j := marshalJSON(*in.Events)
		app.Events = j
		updates["events"] = j
	}

	if len(updates) == 0 {
		return app, nil
	}

	if err := s.db.WithContext(ctx).Model(&models.App{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update app: %w", err)
	}
	return app, nil
}

// Delete removes an app and all its installations, keys, and tokens.
func (s *AppService) Delete(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete installation tokens via installations
		var installIDs []string
		tx.Model(&models.AppInstallation{}).Where("app_id = ?", id).Pluck("id", &installIDs)
		if len(installIDs) > 0 {
			tx.Where("installation_id IN ?", installIDs).Delete(&models.AppInstallationToken{})
			tx.Where("installation_id IN ?", installIDs).Delete(&models.AppInstallationRepository{})
		}
		tx.Where("app_id = ?", id).Delete(&models.AppInstallation{})
		tx.Where("app_id = ?", id).Delete(&models.AppPrivateKey{})
		return tx.Delete(&models.App{}, id).Error
	})
}

// RegenerateClientSecret creates a new client_secret for the app.
func (s *AppService) RegenerateClientSecret(ctx context.Context, id string) (*models.App, string, error) {
	app, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, "", err
	}
	secret, err := generateHex(40)
	if err != nil {
		return nil, "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", err
	}
	if err := s.db.WithContext(ctx).Model(app).Update("client_secret_hash", string(hash)).Error; err != nil {
		return nil, "", err
	}
	return app, secret, nil
}

// GeneratePrivateKey creates a 2048-bit RSA key pair.
// The public key is stored in the database. The private key PEM is returned
// exactly once Ã¢â‚¬â€ gitpier never stores it.
func (s *AppService) GeneratePrivateKey(ctx context.Context, appID string) (*models.AppPrivateKey, string, error) {
	// Enforce a limit of 10 keys per app (same as GitHub).
	var count int64
	s.db.WithContext(ctx).Model(&models.AppPrivateKey{}).Where("app_id = ?", appID).Count(&count)
	if count >= 10 {
		return nil, "", ErrTooManyKeys
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, "", fmt.Errorf("generate RSA key: %w", err)
	}

	// Encode private key to PEM (returned to caller).
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Encode public key to PEM (stored in DB).
	pubDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, "", fmt.Errorf("marshal public key: %w", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDER,
	})

	// Fingerprint = SHA-256 of DER-encoded public key.
	sum := sha256.Sum256(pubDER)
	fingerprint := hex.EncodeToString(sum[:])

	key := &models.AppPrivateKey{
		AppID:        appID,
		Fingerprint:  fingerprint,
		PublicKeyPEM: string(pubPEM),
	}
	if err := s.db.WithContext(ctx).Create(key).Error; err != nil {
		return nil, "", fmt.Errorf("store public key: %w", err)
	}

	return key, string(privPEM), nil
}

// ListPrivateKeys returns all key fingerprints for an app (no PEM data).
func (s *AppService) ListPrivateKeys(ctx context.Context, appID string) ([]models.AppPrivateKey, error) {
	var keys []models.AppPrivateKey
	err := s.db.WithContext(ctx).
		Select("id, created_at, app_id, fingerprint").
		Where("app_id = ?", appID).
		Order("created_at asc").
		Find(&keys).Error
	return keys, err
}

// DeletePrivateKey removes a key by ID (validates it belongs to the app).
func (s *AppService) DeletePrivateKey(ctx context.Context, appID, keyID string) error {
	result := s.db.WithContext(ctx).
		Where("id = ? AND app_id = ?", keyID, appID).
		Delete(&models.AppPrivateKey{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrKeyNotFound
	}
	return nil
}

// VerifyAppJWT validates a JWT signed by an app's private key.
// It returns the gitpier app identified by the "iss" claim.
func (s *AppService) VerifyAppJWT(ctx context.Context, tokenString string) (*models.App, error) {
	// Parse without verification first to extract the issuer.
	unverified, _, err := jwt.NewParser().ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return nil, ErrInvalidAppJWT
	}
	claims, ok := unverified.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidAppJWT
	}

	var appID string
	switch v := claims["iss"].(type) {
	case float64:
		appID = fmt.Sprintf("%.0f", v)
	case string:
		appID = v
	default:
		return nil, ErrInvalidAppJWT
	}

	// Load the app and its public keys.
	app, err := s.GetByID(ctx, appID)
	if err != nil {
		return nil, ErrInvalidAppJWT
	}
	var keys []models.AppPrivateKey
	if err := s.db.WithContext(ctx).Where("app_id = ?", appID).Find(&keys).Error; err != nil || len(keys) == 0 {
		return nil, ErrInvalidAppJWT
	}

	// Try each public key until one validates the token.
	for _, k := range keys {
		block, _ := pem.Decode([]byte(k.PublicKeyPEM))
		if block == nil {
			continue
		}
		pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			continue
		}
		rsaKey, ok := pubKey.(*rsa.PublicKey)
		if !ok {
			continue
		}
		_, err = jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return rsaKey, nil
		}, jwt.WithExpirationRequired())
		if err == nil {
			return app, nil // valid
		}
	}

	return nil, ErrInvalidAppJWT
}

type CreateInstallationInput struct {
	AppID               string
	AccountID           string
	AccountType         string   // "user" or "org"
	RepositorySelection string   // "all" or "selected"
	RepoIDs             []string // only when RepositorySelection == "selected"
}

func normalizeRepoSelection(selection string) string {
	switch strings.ToLower(strings.TrimSpace(selection)) {
	case "", "all":
		return "all"
	case "selected":
		return "selected"
	default:
		return ""
	}
}

func (s *AppService) validateSelectedRepoIDs(ctx context.Context, accountID, accountType string, repoIDs []string) ([]string, error) {
	if len(repoIDs) == 0 {
		return []string{}, nil
	}

	seen := make(map[string]struct{}, len(repoIDs))
	normalized := make([]string, 0, len(repoIDs))
	for _, id := range repoIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	if len(normalized) == 0 {
		return []string{}, nil
	}

	var repos []models.Repository
	if err := s.db.WithContext(ctx).
		Select("id", "owner_id", "org_id").
		Where("id IN ?", normalized).
		Find(&repos).Error; err != nil {
		return nil, err
	}
	if len(repos) != len(normalized) {
		return nil, ErrRepoAccessDenied
	}

	repoByID := make(map[string]models.Repository, len(repos))
	for _, repo := range repos {
		repoByID[repo.ID] = repo
	}

	for _, repoID := range normalized {
		repo, ok := repoByID[repoID]
		if !ok {
			return nil, ErrRepoAccessDenied
		}
		switch accountType {
		case "user":
			if repo.OrgID != nil || repo.OwnerID != accountID {
				return nil, ErrRepoAccessDenied
			}
		case "org":
			if repo.OrgID == nil || *repo.OrgID != accountID {
				return nil, ErrRepoAccessDenied
			}
		default:
			return nil, ErrRepoAccessDenied
		}
	}

	return normalized, nil
}

// Install creates an installation of an app on an account.
// If the app is already installed on the same account, it updates the repo selection.
func (s *AppService) Install(ctx context.Context, in CreateInstallationInput) (*models.AppInstallation, error) {
	app, err := s.GetByID(ctx, in.AppID)
	if err != nil {
		return nil, err
	}

	selection := normalizeRepoSelection(in.RepositorySelection)
	if selection == "" {
		return nil, ErrInvalidRepoSelection
	}
	in.RepositorySelection = selection
	if in.RepositorySelection == "selected" {
		in.RepoIDs, err = s.validateSelectedRepoIDs(ctx, in.AccountID, in.AccountType, in.RepoIDs)
		if err != nil {
			return nil, err
		}
	}

	// Upsert: find existing installation or create a new one.
	var installation models.AppInstallation
	err = s.db.WithContext(ctx).
		Where("app_id = ? AND account_id = ? AND account_type = ?", in.AppID, in.AccountID, in.AccountType).
		First(&installation).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// New installation Ã¢â‚¬â€ copy permission snapshots from app.
		installation = models.AppInstallation{
			AppID:               in.AppID,
			AccountID:           in.AccountID,
			AccountType:         in.AccountType,
			RepositorySelection: in.RepositorySelection,
			RepoPermissions:     app.RepoPermissions,
			OrgPermissions:      app.OrgPermissions,
			AccountPermissions:  app.AccountPermissions,
		}
		if err := s.db.WithContext(ctx).Omit(clause.Associations).Create(&installation).Error; err != nil {
			return nil, fmt.Errorf("create installation: %w", err)
		}
	} else if err != nil {
		return nil, err
	} else {
		// Update existing installation.
		s.db.WithContext(ctx).Model(&installation).Updates(map[string]interface{}{
			"repository_selection": in.RepositorySelection,
			"suspended_at":         nil,
			"suspended_by":         nil,
		})
	}

	// Manage repository selection.
	s.db.WithContext(ctx).Where("installation_id = ?", installation.ID).Delete(&models.AppInstallationRepository{})
	if in.RepositorySelection == "selected" {
		for _, repoID := range in.RepoIDs {
			s.db.WithContext(ctx).Create(&models.AppInstallationRepository{
				InstallationID: installation.ID,
				RepoID:         repoID,
			})
		}
	}

	// Reload with associations.
	_ = s.db.WithContext(ctx).
		Preload("App").
		Preload("Repositories.Repo.Owner").
		Where("id = ?", installation.ID).
		First(&installation).Error

	return &installation, nil
}

// GetInstallation returns an installation by ID.
func (s *AppService) GetInstallation(ctx context.Context, id string) (*models.AppInstallation, error) {
	var inst models.AppInstallation
	err := s.db.WithContext(ctx).
		Preload("App").
		Preload("Repositories.Repo.Owner").
		Where("id = ?", id).
		First(&inst).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInstallationNotFound
	}
	return &inst, err
}

// ListInstallationsByApp returns all installations for a given app.
func (s *AppService) ListInstallationsByApp(ctx context.Context, appID string) ([]models.AppInstallation, error) {
	var installations []models.AppInstallation
	err := s.db.WithContext(ctx).
		Preload("Repositories.Repo.Owner").
		Where("app_id = ?", appID).
		Order("created_at asc").
		Find(&installations).Error
	return installations, err
}

// ListInstallationsByAccount returns all app installations for a user or org account.
func (s *AppService) ListInstallationsByAccount(ctx context.Context, accountID string, accountType string) ([]models.AppInstallation, error) {
	var installations []models.AppInstallation
	err := s.db.WithContext(ctx).
		Preload("App").
		Preload("Repositories.Repo.Owner").
		Where("account_id = ? AND account_type = ?", accountID, accountType).
		Order("created_at asc").
		Find(&installations).Error
	return installations, err
}

// Uninstall removes an app installation.
func (s *AppService) Uninstall(ctx context.Context, installationID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tx.Where("installation_id = ?", installationID).Delete(&models.AppInstallationToken{})
		tx.Where("installation_id = ?", installationID).Delete(&models.AppInstallationRepository{})
		return tx.Delete(&models.AppInstallation{}, installationID).Error
	})
}

// SuspendInstallation suspends an installation (blocks API access).
func (s *AppService) SuspendInstallation(ctx context.Context, installationID string, byUserID string) error {
	now := time.Now()
	return s.db.WithContext(ctx).Model(&models.AppInstallation{}).
		Where("id = ?", installationID).
		Updates(map[string]interface{}{
			"suspended_at": now,
			"suspended_by": byUserID,
		}).Error
}

// UnsuspendInstallation lifts a suspension.
func (s *AppService) UnsuspendInstallation(ctx context.Context, installationID string) error {
	return s.db.WithContext(ctx).Model(&models.AppInstallation{}).
		Where("id = ?", installationID).
		Updates(map[string]interface{}{
			"suspended_at": nil,
			"suspended_by": nil,
		}).Error
}

// UpdateInstallationRepos replaces the selected-repository list for an installation.
func (s *AppService) UpdateInstallationRepos(ctx context.Context, installationID string, selection string, repoIDs []string) error {
	normalizedSelection := normalizeRepoSelection(selection)
	if normalizedSelection == "" {
		return ErrInvalidRepoSelection
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var inst models.AppInstallation
		if err := tx.Where("id = ?", installationID).First(&inst).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInstallationNotFound
			}
			return err
		}

		validatedRepoIDs := []string{}
		var err error
		if normalizedSelection == "selected" {
			validatedRepoIDs, err = s.validateSelectedRepoIDs(ctx, inst.AccountID, inst.AccountType, repoIDs)
			if err != nil {
				return err
			}
		}

		tx.Model(&models.AppInstallation{}).Where("id = ?", installationID).
			Update("repository_selection", normalizedSelection)
		tx.Where("installation_id = ?", installationID).Delete(&models.AppInstallationRepository{})
		if normalizedSelection == "selected" {
			for _, repoID := range validatedRepoIDs {
				tx.Create(&models.AppInstallationRepository{
					InstallationID: installationID,
					RepoID:         repoID,
				})
			}
		}
		return nil
	})
}

// SyncInstallationPermissions refreshes installation permission snapshots from
// the app's current permission configuration.
func (s *AppService) SyncInstallationPermissions(ctx context.Context, installationID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var inst models.AppInstallation
		if err := tx.Where("id = ?", installationID).First(&inst).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInstallationNotFound
			}
			return err
		}

		var app models.App
		if err := tx.Where("id = ?", inst.AppID).First(&app).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAppNotFound
			}
			return err
		}

		return tx.Model(&models.AppInstallation{}).Where("id = ?", installationID).Updates(map[string]interface{}{
			"repo_permissions":    app.RepoPermissions,
			"org_permissions":     app.OrgPermissions,
			"account_permissions": app.AccountPermissions,
		}).Error
	})
}

// CreateInstallationToken issues a short-lived server-to-server token.
// The caller must have already verified the app's JWT.
func (s *AppService) CreateInstallationToken(ctx context.Context, installationID string) (string, time.Time, error) {
	inst, err := s.GetInstallation(ctx, installationID)
	if err != nil {
		return "", time.Time{}, err
	}
	if inst.SuspendedAt != nil {
		return "", time.Time{}, ErrAppSuspended
	}

	tok, err := generateToken("gla_", 40)
	if err != nil {
		return "", time.Time{}, err
	}
	hash := sha256Hex(tok)
	expiresAt := time.Now().Add(time.Hour)

	record := &models.AppInstallationToken{
		InstallationID: installationID,
		TokenHash:      hash,
		ExpiresAt:      expiresAt,
	}
	if err := s.db.WithContext(ctx).Create(record).Error; err != nil {
		return "", time.Time{}, fmt.Errorf("store installation token: %w", err)
	}

	// Clean up expired tokens for this installation.
	s.db.WithContext(ctx).
		Where("installation_id = ? AND expires_at < ?", installationID, time.Now()).
		Delete(&models.AppInstallationToken{})

	return tok, expiresAt, nil
}

// ValidateInstallationToken looks up an installation by a plaintext token.
// Returns the installation on success, or an error if invalid/expired.
func (s *AppService) ValidateInstallationToken(ctx context.Context, token string) (*models.AppInstallation, error) {
	hash := sha256Hex(token)
	var record models.AppInstallationToken
	err := s.db.WithContext(ctx).
		Where("token_hash = ? AND expires_at > ?", hash, time.Now()).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.New("invalid or expired installation token")
	}
	if err != nil {
		return nil, err
	}
	return s.GetInstallation(ctx, record.InstallationID)
}

func marshalJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func marshalJSONMap(m map[string]string) string {
	if m == nil {
		return "{}"
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// ParsePermissions parses a JSON permission map stored in the database.
func ParsePermissions(raw string) map[string]string {
	var m map[string]string
	json.Unmarshal([]byte(raw), &m)
	if m == nil {
		return map[string]string{}
	}
	return m
}

// ParseStringSlice parses a JSON string slice stored in the database.
func ParseStringSlice(raw string) []string {
	var s []string
	json.Unmarshal([]byte(raw), &s)
	if s == nil {
		return []string{}
	}
	return s
}
