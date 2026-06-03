package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrInitManagedSecretsCreatesFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "data", "secrets.json")

	secrets, err := loadOrInitManagedSecrets(path, managedSecretsOverrides{})
	if err != nil {
		t.Fatalf("loadOrInitManagedSecrets returned error: %v", err)
	}

	if secrets.JWTSecret == "" || secrets.SecretEncryptionKey == "" || secrets.AdminSystemPassword == "" {
		t.Fatalf("expected all managed secrets to be generated, got %#v", secrets)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected secrets file to exist: %v", err)
	}

	var persisted managedSecrets
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("failed to decode persisted secrets: %v", err)
	}

	if persisted != *secrets {
		t.Fatalf("persisted secrets do not match returned secrets: %#v vs %#v", persisted, *secrets)
	}
}

func TestLoadOrInitManagedSecretsReusesExistingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "secrets.json")
	expected := managedSecrets{
		JWTSecret:           "jwt",
		SecretEncryptionKey: "enc",
		AdminSystemPassword: "admin",
	}

	data, err := json.Marshal(expected)
	if err != nil {
		t.Fatalf("failed to marshal test fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("failed to write test fixture: %v", err)
	}

	secrets, err := loadOrInitManagedSecrets(path, managedSecretsOverrides{})
	if err != nil {
		t.Fatalf("loadOrInitManagedSecrets returned error: %v", err)
	}

	if *secrets != expected {
		t.Fatalf("expected existing secrets to be reused, got %#v", *secrets)
	}
}

func TestLoadOrInitManagedSecretsPersistsOverrides(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "secrets.json")

	secrets, err := loadOrInitManagedSecrets(path, managedSecretsOverrides{
		JWTSecret:           "jwt-override",
		SecretEncryptionKey: "enc-override",
		AdminSystemPassword: "admin-override",
	})
	if err != nil {
		t.Fatalf("loadOrInitManagedSecrets returned error: %v", err)
	}

	if secrets.JWTSecret != "jwt-override" || secrets.SecretEncryptionKey != "enc-override" || secrets.AdminSystemPassword != "admin-override" {
		t.Fatalf("expected overrides to be applied, got %#v", *secrets)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected secrets file to exist: %v", err)
	}

	var persisted managedSecrets
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("failed to decode persisted secrets: %v", err)
	}

	if persisted != *secrets {
		t.Fatalf("persisted secrets do not match returned secrets: %#v vs %#v", persisted, *secrets)
	}
}
