package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gitpier/internal/models"

	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"
)

var ErrSSHKeyExists = errors.New("SSH key already added")

type SSHKeyService struct {
	db *gorm.DB
}

func NewSSHKeyService(db *gorm.DB) *SSHKeyService {
	return &SSHKeyService{db: db}
}

type AddSSHKeyInput struct {
	UserID string
	Title  string
	Key    string
}

func (s *SSHKeyService) Add(ctx context.Context, input AddSSHKeyInput) (*models.SSHKey, error) {
	// Parse the public key to get fingerprint
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(input.Key)))
	if err != nil {
		return nil, fmt.Errorf("invalid SSH public key: %w", err)
	}

	fingerprint := ssh.FingerprintSHA256(pubKey)

	// Check for duplicate fingerprint
	var existing models.SSHKey
	if err := s.db.WithContext(ctx).Where("fingerprint = ?", fingerprint).First(&existing).Error; err == nil {
		return nil, ErrSSHKeyExists
	}

	key := &models.SSHKey{
		UserID:      input.UserID,
		Title:       input.Title,
		Key:         strings.TrimSpace(input.Key),
		Fingerprint: fingerprint,
	}

	if err := s.db.WithContext(ctx).Create(key).Error; err != nil {
		return nil, fmt.Errorf("failed to add SSH key: %w", err)
	}

	return key, nil
}

func (s *SSHKeyService) List(ctx context.Context, userID string) ([]models.SSHKey, error) {
	var keys []models.SSHKey
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&keys).Error
	return keys, err
}

func (s *SSHKeyService) Delete(ctx context.Context, keyID, userID string) error {
	result := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", keyID, userID).
		Delete(&models.SSHKey{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("key not found")
	}
	return nil
}
