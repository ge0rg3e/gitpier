package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	// Server
	Port    string
	SSHPort string
	AppURL  string

	// Database
	DatabaseURL string
	RedisURL    string

	// JWT
	JWTSecret string

	// Git identity defaults for tag/commit operations when runtime git config is missing.
	GitIdentityName  string
	GitIdentityEmail string

	// Admin dashboard password for /admin/system endpoint.
	AdminSystemPassword string

	// Storage
	ReposPath                 string
	SSHHostKeyPath            string
	AvatarsPath               string
	PackagesPath              string
	MarkdownAssetsPath        string
	RepoPublicSizeLimitBytes  int64
	RepoPrivateSizeLimitBytes int64

	// Secrets encryption
	SecretEncryptionKey string

	// Database connection pool
	DBMaxOpenConns           int
	DBMaxIdleConns           int
	DBConnMaxLifetimeMinutes int

	// CORS — allowed origins (derived from APP_URL)
	CORSOrigins []string

	// Workflow runner
	DockerHost                       string
	WorkflowRunnerImage              string
	WorkflowMinutesLimitPerMonth     int
	WorkflowMaxConcurrentRuns        int
	WorkflowContainerMemory          string
	WorkflowContainerCPUs            string
	WorkflowContainerNetworkMode     string
	WorkflowWorkspacePath            string
	WorkflowAllowDockerSocket        bool
	WorkflowContainerPidsLimit       int
	WorkflowContainerReadOnlyRootfs  bool
	WorkflowContainerNoNewPrivileges bool
	WorkflowContainerDropAllCaps     bool

	// Security: Cloudflare Turnstile
	TurnstileSecretKey string
	TurnstileSiteKey   string
	EnableTurnstile    bool

	// Security: Anti-spam
	EnableRateLimiting bool

	// Repository creation policy
	RestrictRepoCreation   bool
	RepoCreationAllowUsers []string
	SelfHostURL            string

	// Email: SMTP delivery
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SMTPFromName string
}

func Load() (*Config, error) {
	// Load .env file if it exists
	_ = godotenv.Load()

	cfg := &Config{
		Port:                      getEnv("PORT", "8080"),
		SSHPort:                   getEnv("SSH_PORT", "2222"),
		AppURL:                    getEnv("APP_URL", "http://localhost:8080"),
		DatabaseURL:               getEnv("DATABASE_URL", ""),
		RedisURL:                  getEnvOrEmpty("REDIS_URL", "redis://redis:6379/0"),
		JWTSecret:                 getEnv("JWT_SECRET", ""),
		GitIdentityName:           getEnv("GIT_IDENTITY_NAME", "GitPier"),
		GitIdentityEmail:          getEnv("GIT_IDENTITY_EMAIL", "noreply@gitpier.local"),
		AdminSystemPassword:       getEnvOrEmpty("SYSTEM_ADMIN_PASSWORD", ""),
		ReposPath:                 getEnv("REPOS_PATH", "/data/repos"),
		SSHHostKeyPath:            getEnv("SSH_HOST_KEY_PATH", "/data/ssh/ssh_host_key"),
		AvatarsPath:               getEnv("AVATARS_PATH", "/data/avatars"),
		PackagesPath:              getEnv("PACKAGES_PATH", "/data/packages"),
		MarkdownAssetsPath:        getEnv("MARKDOWN_ASSETS_PATH", "/data/markdown-assets"),
		RepoPublicSizeLimitBytes:  getEnvInt64("REPO_STORAGE_LIMIT_PUBLIC_MB", 5120) * 1024 * 1024,
		RepoPrivateSizeLimitBytes: getEnvInt64("REPO_STORAGE_LIMIT_PRIVATE_MB", 5120) * 1024 * 1024,

		// Derive the encryption key from a dedicated env var; fall back to JWT_SECRET so
		// existing deployments keep working without extra configuration.
		SecretEncryptionKey: getEnv("SECRET_ENCRYPTION_KEY", getEnv("JWT_SECRET", "")),

		DBMaxOpenConns:           getEnvInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:           getEnvInt("DB_MAX_IDLE_CONNS", 10),
		DBConnMaxLifetimeMinutes: getEnvInt("DB_CONN_MAX_LIFETIME_MINUTES", 5),
		CORSOrigins:              []string{getEnv("APP_URL", "http://localhost:8080")},

		DockerHost:                       getEnvOrEmpty("DOCKER_HOST", "tcp://dind:2375"),
		WorkflowRunnerImage:              getEnv("WORKFLOW_RUNNER_IMAGE", "gitpier/action-runner:latest"),
		WorkflowMinutesLimitPerMonth:     getEnvInt("WORKFLOW_MINUTES_LIMIT_PER_MONTH", 5000),
		WorkflowMaxConcurrentRuns:        getEnvInt("WORKFLOW_MAX_CONCURRENT_RUNS", 3),
		WorkflowContainerMemory:          getEnv("WORKFLOW_CONTAINER_MEMORY", "500m"),
		WorkflowContainerCPUs:            getEnv("WORKFLOW_CONTAINER_CPUS", "0.5"),
		WorkflowContainerNetworkMode:     normalizeWorkflowContainerNetworkMode(getEnvOrEmpty("WORKFLOW_CONTAINER_NETWORK_MODE", "bridge")),
		WorkflowWorkspacePath:            getEnv("WORKFLOW_WORKSPACE_PATH", "/data/workflow-workspaces"),
		WorkflowAllowDockerSocket:        getEnvOrEmpty("WORKFLOW_ALLOW_DOCKER_SOCKET", "true") == "true",
		WorkflowContainerPidsLimit:       getEnvInt("WORKFLOW_CONTAINER_PIDS_LIMIT", 256),
		WorkflowContainerReadOnlyRootfs:  getEnvOrEmpty("WORKFLOW_CONTAINER_READONLY_ROOTFS", "true") == "true",
		WorkflowContainerNoNewPrivileges: getEnvOrEmpty("WORKFLOW_CONTAINER_NO_NEW_PRIVILEGES", "true") == "true",
		WorkflowContainerDropAllCaps:     getEnvOrEmpty("WORKFLOW_CONTAINER_DROP_ALL_CAPS", "true") == "true",

		TurnstileSecretKey: getEnvOrEmpty("TURNSTILE_SECRET_KEY", "1x0000000000000000000000000000000AA"), // Disabled by default
		TurnstileSiteKey:   getEnvOrEmpty("TURNSTILE_SITE_KEY", "1x00000000000000000000AA"),              // Disabled by default
		EnableTurnstile:    getEnvOrEmpty("ENABLE_TURNSTILE", "false") == "true",

		EnableRateLimiting: getEnvOrEmpty("ENABLE_RATE_LIMITING", "true") == "true",

		RestrictRepoCreation: getEnvOrEmpty("RESTRICT_REPO_CREATION", "false") == "true",
		RepoCreationAllowUsers: parseNormalizedUsernames(
			getEnvOrEmpty("REPO_CREATION_ALLOWED_USERS", ""),
		),
		SelfHostURL: getEnvOrEmpty("SELF_HOST_URL", "https://github.com/gitpier/gitpier"),

		SMTPHost:     getEnvOrEmpty("SMTP_HOST", ""),
		SMTPPort:     getEnvInt("SMTP_PORT", 587),
		SMTPUsername: getEnvOrEmpty("SMTP_USERNAME", ""),
		SMTPPassword: getEnvOrEmpty("SMTP_PASSWORD", ""),
		SMTPFrom:     getEnvOrEmpty("SMTP_FROM", "noreply@gitpier.com"),
		SMTPFromName: getEnvOrEmpty("SMTP_FROM_NAME", "GitPier"),
	}

	// Resolve relative paths to absolute so they work regardless of working directory.
	if p, err := filepath.Abs(cfg.ReposPath); err == nil {
		cfg.ReposPath = p
	}
	if p, err := filepath.Abs(cfg.AvatarsPath); err == nil {
		cfg.AvatarsPath = p
	}
	if p, err := filepath.Abs(cfg.SSHHostKeyPath); err == nil {
		cfg.SSHHostKeyPath = p
	}
	if p, err := filepath.Abs(cfg.WorkflowWorkspacePath); err == nil {
		cfg.WorkflowWorkspacePath = p
	}
	if p, err := filepath.Abs(cfg.MarkdownAssetsPath); err == nil {
		cfg.MarkdownAssetsPath = p
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		sanitized := sanitizeEnvValue(val)
		if sanitized != "" {
			return sanitized
		}
	}
	return defaultVal
}

// getEnvOrEmpty returns the env value if set (even if empty), otherwise defaultVal.
// Use this for keys where an explicit empty value means "disabled".
func getEnvOrEmpty(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return sanitizeEnvValue(val)
	}
	return defaultVal
}

// sanitizeEnvValue trims whitespace and treats comment-only values as empty.
// This supports patterns like `KEY= # disabled` in .env files.
func sanitizeEnvValue(val string) string {
	trimmed := strings.TrimSpace(val)
	if strings.HasPrefix(trimmed, "#") {
		return ""
	}
	return trimmed
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvInt64(key string, defaultVal int64) int64 {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			return i
		}
	}
	return defaultVal
}

func normalizeWorkflowContainerNetworkMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "none":
		return "none"
	case "bridge":
		return "bridge"
	default:
		return "bridge"
	}
}

func parseNormalizedUsernames(s string) []string {
	parts := strings.Split(s, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		username := strings.ToLower(strings.TrimSpace(part))
		if username == "" {
			continue
		}
		if _, exists := seen[username]; exists {
			continue
		}
		seen[username] = struct{}{}
		out = append(out, username)
	}
	return out
}
