package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type managedSecrets struct {
	JWTSecret           string `json:"jwt_secret"`
	SecretEncryptionKey string `json:"secret_encryption_key"`
	AdminSystemPassword string `json:"system_admin_password"`
}

type managedSecretsOverrides struct {
	JWTSecret           string
	SecretEncryptionKey string
	AdminSystemPassword string
}

func loadOrInitManagedSecrets(path string, overrides managedSecretsOverrides) (*managedSecrets, error) {
	secrets := &managedSecrets{}
	existed := false

	if data, err := os.ReadFile(path); err == nil {
		existed = true
		if err := json.Unmarshal(data, secrets); err != nil {
			return nil, fmt.Errorf("decode managed secrets file: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read managed secrets file: %w", err)
	}

	changed := !existed

	if overrides.JWTSecret != "" && overrides.JWTSecret != secrets.JWTSecret {
		secrets.JWTSecret = overrides.JWTSecret
		changed = true
	}
	if overrides.SecretEncryptionKey != "" && overrides.SecretEncryptionKey != secrets.SecretEncryptionKey {
		secrets.SecretEncryptionKey = overrides.SecretEncryptionKey
		changed = true
	}
	if overrides.AdminSystemPassword != "" && overrides.AdminSystemPassword != secrets.AdminSystemPassword {
		secrets.AdminSystemPassword = overrides.AdminSystemPassword
		changed = true
	}

	if secrets.JWTSecret == "" {
		value, err := generateManagedSecret(32)
		if err != nil {
			return nil, err
		}
		secrets.JWTSecret = value
		changed = true
	}
	if secrets.SecretEncryptionKey == "" {
		value, err := generateManagedSecret(32)
		if err != nil {
			return nil, err
		}
		secrets.SecretEncryptionKey = value
		changed = true
	}
	if secrets.AdminSystemPassword == "" {
		value, err := generateManagedSecret(20)
		if err != nil {
			return nil, err
		}
		secrets.AdminSystemPassword = value
		changed = true
	}

	if changed {
		if err := writeManagedSecrets(path, secrets); err != nil {
			return nil, err
		}
	}

	return secrets, nil
}

func generateManagedSecret(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate managed secret: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func writeManagedSecrets(path string, secrets *managedSecrets) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create managed secrets dir: %w", err)
	}

	data, err := json.MarshalIndent(secrets, "", "  ")
	if err != nil {
		return fmt.Errorf("encode managed secrets file: %w", err)
	}
	data = append(data, '\n')

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("write managed secrets temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace managed secrets file: %w", err)
	}

	return nil
}
