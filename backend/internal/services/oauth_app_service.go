package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"gitpier/internal/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrOAuthAppNotFound  = errors.New("oauth app not found")
	ErrOAuthAppForbidden = errors.New("not authorized to manage this oauth app")
)

// OAuthAppService manages OAuth application registrations and authorizations.
type OAuthAppService struct {
	db *gorm.DB
}

func NewOAuthAppService(db *gorm.DB) *OAuthAppService {
	return &OAuthAppService{db: db}
}

// generateHex returns a random lowercase hex string of exactly n characters.
func generateHex(n int) (string, error) {
	b := make([]byte, (n+1)/2)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b)[:n], nil
}

type CreateOAuthAppInput struct {
	Name             string
	Description      string
	HomepageURL      string
	CallbackURL      string
	LogoURL          string
	EnableDeviceFlow bool
	OwnerID          string
	OwnerType        string // "user" or "org"
}

// Create registers a new OAuth app. Returns the app and the plaintext client_secret
// (the only time it is ever returned in plaintext).
func (s *OAuthAppService) Create(ctx context.Context, in CreateOAuthAppInput) (*models.OAuthApp, string, error) {
	clientID, err := generateHex(20)
	if err != nil {
		return nil, "", fmt.Errorf("generate client_id: %w", err)
	}

	secret, err := generateHex(40)
	if err != nil {
		return nil, "", fmt.Errorf("generate client_secret: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", fmt.Errorf("hash client_secret: %w", err)
	}

	app := &models.OAuthApp{
		ClientID:         clientID,
		ClientSecretHash: string(hash),
		Name:             in.Name,
		Description:      in.Description,
		HomepageURL:      in.HomepageURL,
		CallbackURL:      in.CallbackURL,
		LogoURL:          in.LogoURL,
		EnableDeviceFlow: in.EnableDeviceFlow,
		OwnerID:          in.OwnerID,
		OwnerType:        in.OwnerType,
	}

	if err := s.db.WithContext(ctx).Create(app).Error; err != nil {
		return nil, "", fmt.Errorf("create oauth app: %w", err)
	}

	return app, secret, nil
}

// ListByOwner returns all OAuth apps owned by the given user or org.
func (s *OAuthAppService) ListByOwner(ctx context.Context, ownerID string, ownerType string) ([]models.OAuthApp, error) {
	var apps []models.OAuthApp
	if err := s.db.WithContext(ctx).
		Where("owner_id = ? AND owner_type = ?", ownerID, ownerType).
		Order("created_at asc").
		Find(&apps).Error; err != nil {
		return nil, err
	}
	// Populate authorization count for each app.
	for i := range apps {
		var count int64
		s.db.Model(&models.OAuthAuthorization{}).Where("app_id = ?", apps[i].ID).Count(&count)
		apps[i].AuthorizationCount = int(count)
	}
	return apps, nil
}

// GetByID returns an OAuth app by primary key.
func (s *OAuthAppService) GetByID(ctx context.Context, id string) (*models.OAuthApp, error) {
	var app models.OAuthApp
	if err := s.db.WithContext(ctx).First(&app, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOAuthAppNotFound
		}
		return nil, err
	}
	var count int64
	s.db.Model(&models.OAuthAuthorization{}).Where("app_id = ?", app.ID).Count(&count)
	app.AuthorizationCount = int(count)
	return &app, nil
}

// GetByClientID returns an OAuth app by its client_id (used during OAuth flows).
func (s *OAuthAppService) GetByClientID(ctx context.Context, clientID string) (*models.OAuthApp, error) {
	var app models.OAuthApp
	if err := s.db.WithContext(ctx).Where("client_id = ?", clientID).First(&app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOAuthAppNotFound
		}
		return nil, err
	}
	return &app, nil
}

type UpdateOAuthAppInput struct {
	Name             *string
	Description      *string
	HomepageURL      *string
	CallbackURL      *string
	LogoURL          *string
	EnableDeviceFlow *bool
}

// Update applies the non-nil fields of in to the app with the given id.
func (s *OAuthAppService) Update(ctx context.Context, id string, in UpdateOAuthAppInput) (*models.OAuthApp, error) {
	app, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if in.Name != nil {
		updates["name"] = *in.Name
	}
	if in.Description != nil {
		updates["description"] = *in.Description
	}
	if in.HomepageURL != nil {
		updates["homepage_url"] = *in.HomepageURL
	}
	if in.CallbackURL != nil {
		updates["callback_url"] = *in.CallbackURL
	}
	if in.LogoURL != nil {
		updates["logo_url"] = *in.LogoURL
	}
	if in.EnableDeviceFlow != nil {
		updates["enable_device_flow"] = *in.EnableDeviceFlow
	}

	if len(updates) > 0 {
		if err := s.db.WithContext(ctx).Model(app).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("update oauth app: %w", err)
		}
	}
	return s.GetByID(ctx, id)
}

// Delete removes an OAuth app and all associated authorizations.
func (s *OAuthAppService) Delete(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("app_id = ?", id).Delete(&models.OAuthAuthorization{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.OAuthApp{}, id).Error
	})
}

// RegenerateSecret generates a new client_secret for an app. Returns the new plaintext secret.
func (s *OAuthAppService) RegenerateSecret(ctx context.Context, id string) (string, error) {
	secret, err := generateHex(40)
	if err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash secret: %w", err)
	}

	if err := s.db.WithContext(ctx).Model(&models.OAuthApp{}).Where("id = ?", id).
		Update("client_secret_hash", string(hash)).Error; err != nil {
		return "", fmt.Errorf("update secret: %w", err)
	}

	return secret, nil
}

// ListAuthorizedApps returns all apps a given user has authorized.
func (s *OAuthAppService) ListAuthorizedApps(ctx context.Context, userID string) ([]models.OAuthAuthorization, error) {
	var auths []models.OAuthAuthorization
	if err := s.db.WithContext(ctx).
		Preload("App").
		Where("user_id = ?", userID).
		Order("created_at desc").
		Find(&auths).Error; err != nil {
		return nil, err
	}
	return auths, nil
}

// RevokeAuthorization removes a user's authorization for an app.
func (s *OAuthAppService) RevokeAuthorization(ctx context.Context, authID string, userID string) error {
	result := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", authID, userID).
		Delete(&models.OAuthAuthorization{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("authorization not found")
	}
	return nil
}

// VerifySecret checks if the provided plaintext secret matches the stored hash for an app.
func (s *OAuthAppService) VerifySecret(app *models.OAuthApp, secret string) bool {
	return bcrypt.CompareHashAndPassword([]byte(app.ClientSecretHash), []byte(secret)) == nil
}
