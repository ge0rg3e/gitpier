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

	"gorm.io/gorm"
)

const (
	PATScopeRepoRead  = "repo:read"
	PATScopeRepoWrite = "repo:write"
)

var (
	ErrPersonalAccessTokenNotFound = errors.New("personal access token not found")
	ErrPersonalAccessTokenExpired  = errors.New("personal access token expired")
	ErrInvalidTokenScope           = errors.New("invalid token scope")
)

type PersonalAccessTokenService struct {
	db *gorm.DB
}

type CreatePersonalAccessTokenInput struct {
	UserID    string
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
}

type CreatedPersonalAccessToken struct {
	TokenRecord *models.PersonalAccessToken
	Plaintext   string
}

func NewPersonalAccessTokenService(db *gorm.DB) *PersonalAccessTokenService {
	return &PersonalAccessTokenService{db: db}
}

func (s *PersonalAccessTokenService) Create(ctx context.Context, input CreatePersonalAccessTokenInput) (*CreatedPersonalAccessToken, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("token name is required")
	}

	scopes, err := normalizePATScopes(input.Scopes)
	if err != nil {
		return nil, err
	}

	plaintext, err := generatePATPlaintext()
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	token := &models.PersonalAccessToken{
		UserID:     input.UserID,
		Name:       name,
		TokenHash:  patSHA256Hex(plaintext),
		TokenLast:  plaintext[len(plaintext)-8:],
		Scopes:     strings.Join(scopes, " "),
		ExpiresAt:  input.ExpiresAt,
		LastUsedAt: nil,
	}

	if err := s.db.WithContext(ctx).Create(token).Error; err != nil {
		return nil, fmt.Errorf("store token: %w", err)
	}

	return &CreatedPersonalAccessToken{TokenRecord: token, Plaintext: plaintext}, nil
}

func (s *PersonalAccessTokenService) List(ctx context.Context, userID string) ([]models.PersonalAccessToken, error) {
	var tokens []models.PersonalAccessToken
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&tokens).Error; err != nil {
		return nil, err
	}
	return tokens, nil
}

func (s *PersonalAccessTokenService) Delete(ctx context.Context, tokenID, userID string) error {
	result := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", tokenID, userID).
		Delete(&models.PersonalAccessToken{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrPersonalAccessTokenNotFound
	}
	return nil
}

func (s *PersonalAccessTokenService) Lookup(ctx context.Context, plaintext string) (*models.User, []string, error) {
	plaintext = strings.TrimSpace(plaintext)
	if plaintext == "" {
		return nil, nil, ErrPersonalAccessTokenNotFound
	}

	var token models.PersonalAccessToken
	if err := s.db.WithContext(ctx).
		Preload("User").
		Where("token_hash = ?", patSHA256Hex(plaintext)).
		First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrPersonalAccessTokenNotFound
		}
		return nil, nil, err
	}

	if token.ExpiresAt != nil && time.Now().UTC().After(*token.ExpiresAt) {
		return nil, nil, ErrPersonalAccessTokenExpired
	}
	if token.User.IsSuspended {
		return nil, nil, ErrPersonalAccessTokenNotFound
	}

	now := time.Now().UTC()
	_ = s.db.WithContext(ctx).
		Model(&models.PersonalAccessToken{}).
		Where("id = ?", token.ID).
		Update("last_used_at", now).Error

	return &token.User, strings.Fields(token.Scopes), nil
}

func HasPATScope(scopes []string, required string) bool {
	for _, scope := range scopes {
		if scope == required {
			return true
		}
		if required == PATScopeRepoRead && scope == PATScopeRepoWrite {
			return true
		}
	}
	return false
}

func normalizePATScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 {
		return []string{PATScopeRepoRead}, nil
	}

	seen := make(map[string]bool, len(scopes))
	normalized := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		switch scope {
		case PATScopeRepoRead, PATScopeRepoWrite:
		default:
			return nil, ErrInvalidTokenScope
		}
		if !seen[scope] {
			seen[scope] = true
			normalized = append(normalized, scope)
		}
	}
	return normalized, nil
}

func generatePATPlaintext() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "gp_pat_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func patSHA256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
