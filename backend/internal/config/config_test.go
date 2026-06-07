package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultDataPathUsesLocalDataRoot(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	dataRoot := filepath.Join(cwd, "..", ".data")
	if err := os.MkdirAll(dataRoot, 0755); err != nil {
		t.Fatalf("failed to create local data root: %v", err)
	}

	got := defaultDataPath("repos", "/data/repos")
	want := filepath.Join(dataRoot, "repos")
	if got != want {
		t.Fatalf("expected local data path %q, got %q", want, got)
	}

	if got := defaultDevHTTPPort(); got != "8080" {
		t.Fatalf("expected local dev HTTP port %q, got %q", "8080", got)
	}
	if got := defaultDevSSHPort(); got != "2222" {
		t.Fatalf("expected local dev SSH port %q, got %q", "2222", got)
	}
	if got := defaultDevAppURL(); got != "http://localhost:5173" {
		t.Fatalf("expected local dev app URL %q, got %q", "http://localhost:5173", got)
	}
	if got := defaultDevAPIURL(); got != "http://localhost:8080" {
		t.Fatalf("expected local dev API URL %q, got %q", "http://localhost:8080", got)
	}
}

func TestDefaultDataPathFallsBackWithoutLocalDataRoot(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	got := defaultDataPath("repos", "/data/repos")
	if got != "/data/repos" {
		t.Fatalf("expected fallback path %q, got %q", "/data/repos", got)
	}

	if got := defaultDevHTTPPort(); got != "8828" {
		t.Fatalf("expected fallback HTTP port %q, got %q", "8828", got)
	}
	if got := defaultDevSSHPort(); got != "2424" {
		t.Fatalf("expected fallback SSH port %q, got %q", "2424", got)
	}
	if got := defaultDevAppURL(); got != "http://localhost:8828" {
		t.Fatalf("expected fallback app URL %q, got %q", "http://localhost:8828", got)
	}
	if got := defaultDevAPIURL(); got != "" {
		t.Fatalf("expected fallback API URL %q, got %q", "", got)
	}
}
