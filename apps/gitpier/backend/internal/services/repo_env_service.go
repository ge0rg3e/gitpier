package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"time"

	"gitpier/internal/models"

	"gorm.io/gorm"
)

// RepoSecretInfo is the public representation of a secret â€” name and timestamps only,
// never the value.
type RepoSecretInfo struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RepoEnvService manages repository-scoped variables and secrets.
type RepoEnvService struct {
	db  *gorm.DB
	key [32]byte // AES-256 key derived from the encryption key material
}

// NewRepoEnvService creates a service that uses encryptionKey to protect secret values.
// The key is hashed with SHA-256 so any string length is accepted.
func NewRepoEnvService(db *gorm.DB, encryptionKey string) *RepoEnvService {
	key := sha256.Sum256([]byte(encryptionKey))
	return &RepoEnvService{db: db, key: key}
}

// ListVariables returns all variables for a repository (name + value).
func (s *RepoEnvService) ListVariables(repoID string) ([]models.RepoVariable, error) {
	var vars []models.RepoVariable
	return vars, s.db.Where("repo_id = ?", repoID).Order("name asc").Find(&vars).Error
}

// SetVariable creates or updates a named variable for a repository.
func (s *RepoEnvService) SetVariable(repoID string, name, value string) error {
	var v models.RepoVariable
	err := s.db.Where("repo_id = ? AND name = ?", repoID, name).First(&v).Error
	if err == gorm.ErrRecordNotFound {
		return s.db.Create(&models.RepoVariable{RepoID: repoID, Name: name, Value: value}).Error
	}
	if err != nil {
		return err
	}
	return s.db.Model(&v).Update("value", value).Error
}

// DeleteVariable removes a named variable for a repository.
func (s *RepoEnvService) DeleteVariable(repoID string, name string) error {
	return s.db.Where("repo_id = ? AND name = ?", repoID, name).Delete(&models.RepoVariable{}).Error
}

// GetVarsMap returns all variables for a repository as a map (for workflow injection).
func (s *RepoEnvService) GetVarsMap(repoID string) map[string]string {
	var vars []models.RepoVariable
	s.db.Where("repo_id = ?", repoID).Find(&vars)
	result := make(map[string]string, len(vars))
	for _, v := range vars {
		result[v.Name] = v.Value
	}
	return result
}

// ListSecrets returns secret metadata (name + timestamps) without values.
func (s *RepoEnvService) ListSecrets(repoID string) ([]RepoSecretInfo, error) {
	var secrets []models.RepoSecret
	if err := s.db.Where("repo_id = ?", repoID).Order("name asc").Find(&secrets).Error; err != nil {
		return nil, err
	}
	result := make([]RepoSecretInfo, len(secrets))
	for i, sec := range secrets {
		result[i] = RepoSecretInfo{Name: sec.Name, CreatedAt: sec.CreatedAt, UpdatedAt: sec.UpdatedAt}
	}
	return result, nil
}

// SetSecret encrypts and stores (or updates) a named secret for a repository.
func (s *RepoEnvService) SetSecret(repoID string, name, value string) error {
	encrypted, err := s.encrypt(value)
	if err != nil {
		return fmt.Errorf("encrypt secret: %w", err)
	}
	var sec models.RepoSecret
	err = s.db.Where("repo_id = ? AND name = ?", repoID, name).First(&sec).Error
	if err == gorm.ErrRecordNotFound {
		return s.db.Create(&models.RepoSecret{RepoID: repoID, Name: name, EncryptedValue: encrypted}).Error
	}
	if err != nil {
		return err
	}
	return s.db.Model(&sec).Update("encrypted_value", encrypted).Error
}

// DeleteSecret removes a named secret for a repository.
func (s *RepoEnvService) DeleteSecret(repoID string, name string) error {
	return s.db.Where("repo_id = ? AND name = ?", repoID, name).Delete(&models.RepoSecret{}).Error
}

// GetSecretsMap returns all decrypted secrets for a repository as a map.
// This is intended for internal use by the workflow runner only â€” values are never sent to clients.
func (s *RepoEnvService) GetSecretsMap(repoID string) map[string]string {
	var secrets []models.RepoSecret
	s.db.Where("repo_id = ?", repoID).Find(&secrets)
	result := make(map[string]string, len(secrets))
	for _, sec := range secrets {
		if val, err := s.decrypt(sec.EncryptedValue); err == nil {
			result[sec.Name] = val
		}
	}
	return result
}

func (s *RepoEnvService) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (s *RepoEnvService) decrypt(encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
