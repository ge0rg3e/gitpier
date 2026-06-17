package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"gitpier/internal/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrOAuthCodeNotFound    = errors.New("authorization code not found or expired")
	ErrOAuthCodeUsed        = errors.New("authorization code already used")
	ErrOAuthCodeExpired     = errors.New("authorization code expired")
	ErrOAuthInvalidClient   = errors.New("invalid client credentials")
	ErrOAuthInvalidRedirect = errors.New("redirect_uri does not match registered callback")
	ErrOAuthPKCEFailed      = errors.New("PKCE verification failed")
	ErrOAuthTokenNotFound   = errors.New("token not found")
	ErrDeviceCodeNotFound   = errors.New("device code not found")
	ErrDeviceCodeExpired    = errors.New("device code expired")
	ErrDeviceFlowDisabled   = errors.New("device flow not enabled for this app")
	ErrAuthorizationPending = errors.New("authorization_pending")
	ErrSlowDown             = errors.New("slow_down")
	ErrOAuthAccessDenied    = errors.New("access_denied")
	ErrExpiredToken         = errors.New("expired_token")
)

// OAuthFlowService handles the OAuth 2.0 authorization flows.
type OAuthFlowService struct {
	db *gorm.DB
}

func NewOAuthFlowService(db *gorm.DB) *OAuthFlowService {
	return &OAuthFlowService{db: db}
}

// NormalizeScopes deduplicates and trims a space-delimited scope string.
func NormalizeScopes(scopes string) string {
	parts := strings.Fields(scopes)
	seen := make(map[string]bool, len(parts))
	result := make([]string, 0, len(parts))
	for _, s := range parts {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return strings.Join(result, " ")
}

// CreateAuthorizationCode issues a short-lived (10 min) single-use authorization code.
func (s *OAuthFlowService) CreateAuthorizationCode(
	ctx context.Context,
	appID, userID string,
	scopes, redirectURI, codeChallenge, challengeMethod string,
) (string, error) {
	code, err := generateHex(40)
	if err != nil {
		return "", fmt.Errorf("generate code: %w", err)
	}

	record := &models.OAuthCode{
		Code:            code,
		AppID:           appID,
		UserID:          userID,
		Scopes:          NormalizeScopes(scopes),
		RedirectURI:     redirectURI,
		CodeChallenge:   codeChallenge,
		ChallengeMethod: challengeMethod,
		ExpiresAt:       time.Now().Add(10 * time.Minute),
	}

	if err := s.db.WithContext(ctx).Create(record).Error; err != nil {
		return "", fmt.Errorf("store auth code: %w", err)
	}
	return code, nil
}

// ExchangeCode validates an authorization code and issues an access token.
// Returns (plaintext_token, scopes, error).
func (s *OAuthFlowService) ExchangeCode(
	ctx context.Context,
	clientID, clientSecret, code, redirectURI, codeVerifier string,
) (string, string, error) {
	var record models.OAuthCode
	if err := s.db.WithContext(ctx).Where("code = ?", code).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", ErrOAuthCodeNotFound
		}
		return "", "", err
	}

	if record.Used {
		return "", "", ErrOAuthCodeUsed
	}

	if time.Now().After(record.ExpiresAt) {
		return "", "", ErrOAuthCodeExpired
	}

	// Validate the app.
	var app models.OAuthApp
	if err := s.db.WithContext(ctx).Where("client_id = ?", clientID).First(&app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", ErrOAuthInvalidClient
		}
		return "", "", err
	}

	if app.ID != record.AppID {
		return "", "", ErrOAuthInvalidClient
	}

	// Verify client_secret.
	if err := bcrypt.CompareHashAndPassword([]byte(app.ClientSecretHash), []byte(clientSecret)); err != nil {
		return "", "", ErrOAuthInvalidClient
	}

	// Validate redirect_uri if it was included in the original request.
	if record.RedirectURI != "" && redirectURI != record.RedirectURI {
		return "", "", ErrOAuthInvalidRedirect
	}

	// PKCE verification (S256 only, matching GitHub's requirement).
	if record.CodeChallenge != "" {
		if codeVerifier == "" || record.ChallengeMethod != "S256" {
			return "", "", ErrOAuthPKCEFailed
		}
		h := sha256.Sum256([]byte(codeVerifier))
		computed := base64.RawURLEncoding.EncodeToString(h[:])
		if computed != record.CodeChallenge {
			return "", "", ErrOAuthPKCEFailed
		}
	}

	// Mark code as used (single-use guarantee).
	if err := s.db.WithContext(ctx).Model(&record).Update("used", true).Error; err != nil {
		return "", "", err
	}

	token, err := s.issueToken(ctx, app.ID, record.UserID, record.Scopes)
	if err != nil {
		return "", "", err
	}

	s.upsertAuthorization(ctx, app.ID, record.UserID, record.Scopes)

	return token, record.Scopes, nil
}

// issueToken generates and persists a new OAuth access token. Returns the plaintext "glo_â€¦" token.
func (s *OAuthFlowService) issueToken(ctx context.Context, appID, userID string, scopes string) (string, error) {
	raw, err := generateHex(40)
	if err != nil {
		return "", err
	}
	plaintext := "glo_" + raw

	h := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(h[:])

	if err := s.db.WithContext(ctx).Create(&models.OAuthToken{
		TokenHash: tokenHash,
		AppID:     appID,
		UserID:    userID,
		Scopes:    scopes,
	}).Error; err != nil {
		return "", fmt.Errorf("store token: %w", err)
	}
	return plaintext, nil
}

// upsertAuthorization creates or updates the OAuthAuthorization management record.
func (s *OAuthFlowService) upsertAuthorization(ctx context.Context, appID, userID string, scopes string) {
	var auth models.OAuthAuthorization
	result := s.db.WithContext(ctx).
		Where("app_id = ? AND user_id = ?", appID, userID).
		First(&auth)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		s.db.WithContext(ctx).Create(&models.OAuthAuthorization{
			AppID:  appID,
			UserID: userID,
			Scopes: scopes,
		})
	} else if result.Error == nil {
		s.db.WithContext(ctx).Model(&auth).Update("scopes", scopes)
	}
}

// LookupToken finds the user and scopes associated with a plaintext OAuth token.
// Used by the auth middleware.
func (s *OAuthFlowService) LookupToken(ctx context.Context, plaintext string) (*models.User, string, error) {
	h := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(h[:])

	var record models.OAuthToken
	if err := s.db.WithContext(ctx).
		Preload("User").
		Where("token_hash = ?", tokenHash).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrOAuthTokenNotFound
		}
		return nil, "", err
	}
	return &record.User, record.Scopes, nil
}

// RevokeToken deletes a specific access token by plaintext value.
func (s *OAuthFlowService) RevokeToken(ctx context.Context, plaintext string) error {
	h := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(h[:])
	return s.db.WithContext(ctx).Where("token_hash = ?", tokenHash).Delete(&models.OAuthToken{}).Error
}

// RevokeAllTokensForApp deletes all tokens a user has for a given app.
// Called when a user revokes an authorization.
func (s *OAuthFlowService) RevokeAllTokensForApp(ctx context.Context, appID, userID string) error {
	return s.db.WithContext(ctx).
		Where("app_id = ? AND user_id = ?", appID, userID).
		Delete(&models.OAuthToken{}).Error
}

// DeviceCodeResponse matches the GitHub device flow response format.
type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// CreateDeviceCode initiates the device authorization flow.
func (s *OAuthFlowService) CreateDeviceCode(ctx context.Context, clientID, scope, baseURL string) (*DeviceCodeResponse, error) {
	var app models.OAuthApp
	if err := s.db.WithContext(ctx).Where("client_id = ?", clientID).First(&app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOAuthInvalidClient
		}
		return nil, err
	}

	if !app.EnableDeviceFlow {
		return nil, ErrDeviceFlowDisabled
	}

	deviceCode, err := generateHex(40)
	if err != nil {
		return nil, err
	}

	userCode, err := generateUserCode()
	if err != nil {
		return nil, err
	}

	const expiresIn = 900 // 15 minutes, matching GitHub

	if err := s.db.WithContext(ctx).Create(&models.OAuthDeviceCode{
		DeviceCode: deviceCode,
		UserCode:   userCode,
		AppID:      app.ID,
		Scopes:     NormalizeScopes(scope),
		ExpiresAt:  time.Now().Add(time.Duration(expiresIn) * time.Second),
		Interval:   5,
	}).Error; err != nil {
		return nil, fmt.Errorf("store device code: %w", err)
	}

	verificationURI := baseURL + "/login/device"
	return &DeviceCodeResponse{
		DeviceCode:              deviceCode,
		UserCode:                userCode,
		VerificationURI:         verificationURI,
		VerificationURIComplete: verificationURI + "?user_code=" + userCode,
		ExpiresIn:               expiresIn,
		Interval:                5,
	}, nil
}

// generateUserCode creates an 8-char code in XXXX-XXXX format (no I/O to avoid confusion).
func generateUserCode() (string, error) {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	result := make([]byte, 9)
	for i := 0; i < 4; i++ {
		result[i] = chars[int(b[i])%len(chars)]
	}
	result[4] = '-'
	for i := 0; i < 4; i++ {
		result[5+i] = chars[int(b[4+i])%len(chars)]
	}
	return string(result), nil
}

// GetDeviceCodeForActivation looks up a device_code record by user_code for the browser activation page.
func (s *OAuthFlowService) GetDeviceCodeForActivation(ctx context.Context, userCode string) (*models.OAuthDeviceCode, *models.OAuthApp, error) {
	var record models.OAuthDeviceCode
	if err := s.db.WithContext(ctx).
		Where("user_code = ?", strings.ToUpper(userCode)).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrDeviceCodeNotFound
		}
		return nil, nil, err
	}

	if time.Now().After(record.ExpiresAt) {
		return nil, nil, ErrDeviceCodeExpired
	}

	if record.ApprovedByUserID != nil || record.Denied {
		return nil, nil, errors.New("device code already used")
	}

	var app models.OAuthApp
	s.db.WithContext(ctx).First(&app, record.AppID)
	return &record, &app, nil
}

// ApproveDeviceCode marks a device_code as approved by the authenticated user.
func (s *OAuthFlowService) ApproveDeviceCode(ctx context.Context, userCode string, userID string) (*models.OAuthApp, error) {
	record, app, err := s.GetDeviceCodeForActivation(ctx, userCode)
	if err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Model(record).
		Update("approved_by_user_id", userID).Error; err != nil {
		return nil, err
	}
	return app, nil
}

// DenyDeviceCode marks a device_code as denied by the authenticated user.
func (s *OAuthFlowService) DenyDeviceCode(ctx context.Context, userCode string) error {
	var record models.OAuthDeviceCode
	if err := s.db.WithContext(ctx).
		Where("user_code = ?", strings.ToUpper(userCode)).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrDeviceCodeNotFound
		}
		return err
	}

	if time.Now().After(record.ExpiresAt) {
		return ErrDeviceCodeExpired
	}

	return s.db.WithContext(ctx).Model(&record).Update("denied", true).Error
}

// PollDeviceFlow is called by the third-party app to check if the user has approved.
// Returns (plaintext_token, scopes, error).
// Error values: ErrAuthorizationPending, ErrSlowDown, ErrExpiredToken, ErrOAuthAccessDenied.
func (s *OAuthFlowService) PollDeviceFlow(ctx context.Context, clientID, deviceCode string) (string, string, error) {
	var app models.OAuthApp
	if err := s.db.WithContext(ctx).Where("client_id = ?", clientID).First(&app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", ErrOAuthInvalidClient
		}
		return "", "", err
	}

	var record models.OAuthDeviceCode
	if err := s.db.WithContext(ctx).
		Where("device_code = ? AND app_id = ?", deviceCode, app.ID).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", ErrDeviceCodeNotFound
		}
		return "", "", err
	}

	if time.Now().After(record.ExpiresAt) {
		return "", "", ErrExpiredToken
	}

	if record.Denied {
		return "", "", ErrOAuthAccessDenied
	}

	// Enforce minimum polling interval.
	now := time.Now()
	if record.LastPolledAt != nil {
		minInterval := time.Duration(record.Interval) * time.Second
		if now.Sub(*record.LastPolledAt) < minInterval {
			// Increase interval on every slow_down response.
			s.db.WithContext(ctx).Model(&record).Updates(map[string]interface{}{
				"last_polled_at": now,
				"interval":       record.Interval + 5,
			})
			return "", "", ErrSlowDown
		}
	}

	// Update last_polled_at.
	s.db.WithContext(ctx).Model(&record).Update("last_polled_at", now)

	if record.ApprovedByUserID == nil {
		return "", "", ErrAuthorizationPending
	}

	token, err := s.issueToken(ctx, app.ID, *record.ApprovedByUserID, record.Scopes)
	if err != nil {
		return "", "", err
	}

	s.upsertAuthorization(ctx, app.ID, *record.ApprovedByUserID, record.Scopes)

	// Remove device code so it cannot be polled again.
	s.db.WithContext(ctx).Delete(&record)

	return token, record.Scopes, nil
}

// GetAppInfoForConsent returns the app information needed to render the consent page.
func (s *OAuthFlowService) GetAppInfoForConsent(ctx context.Context, clientID string) (*models.OAuthApp, error) {
	var app models.OAuthApp
	if err := s.db.WithContext(ctx).Where("client_id = ?", clientID).First(&app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOAuthAppNotFound
		}
		return nil, err
	}
	return &app, nil
}
