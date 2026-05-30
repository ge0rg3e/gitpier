package models

import "time"

// App is a registered gitpier App (similar to GitHub Apps).
// It can act on its own behalf (server-to-server via installation access tokens)
// or on behalf of a user (user access tokens via OAuth 2.0 flow).
type App struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Basic info
	Name        string `gorm:"size:100;not null" json:"name"`
	Slug        string `gorm:"uniqueIndex;size:100;not null" json:"slug"` // URL-safe, unique globally
	Description string `gorm:"size:1000" json:"description"`
	HomepageURL string `gorm:"size:500;not null" json:"homepage_url"`
	LogoURL     string `gorm:"size:500" json:"logo_url"`

	// Post-installation redirect
	SetupURL         string `gorm:"size:500" json:"setup_url"`
	RedirectOnUpdate bool   `gorm:"default:false" json:"redirect_on_update"`

	// Webhook configuration
	WebhookURL        string `gorm:"size:500" json:"webhook_url"`
	WebhookSecretHash string `gorm:"size:100" json:"-"` // bcrypt hash, never returned
	WebhookActive     bool   `gorm:"default:true" json:"webhook_active"`

	// Visibility
	// IsPublic=false: only the owner account can install the app.
	// IsPublic=true:  any gitpier user/org can install it.
	IsPublic bool `gorm:"default:false" json:"is_public"`

	// User authorization (OAuth-like flow, user-to-server tokens)
	// CallbackURLs is a JSON array of allowed redirect URIs.
	CallbackURLs     string `gorm:"type:text;not null;default:'[]'" json:"callback_urls"`
	RequestUserAuth  bool   `gorm:"default:false" json:"request_user_auth"`
	ExpireUserTokens bool   `gorm:"default:true" json:"expire_user_tokens"`
	EnableDeviceFlow bool   `gorm:"default:false" json:"enable_device_flow"`

	// OAuth credentials (for user-to-server auth, same pattern as OAuthApp)
	ClientID         string `gorm:"uniqueIndex;size:40" json:"client_id"`
	ClientSecretHash string `gorm:"size:100" json:"-"` // bcrypt hash

	// Fine-grained permissions (JSON objects: { "contents": "read", "issues": "write" })
	// Each permission value is "none", "read", or "write".
	RepoPermissions    string `gorm:"type:text;not null;default:'{}'" json:"repo_permissions"`
	OrgPermissions     string `gorm:"type:text;not null;default:'{}'" json:"org_permissions"`
	AccountPermissions string `gorm:"type:text;not null;default:'{}'" json:"account_permissions"`

	// Webhook events the app subscribes to (JSON array of event names)
	Events string `gorm:"type:text;not null;default:'[]'" json:"events"`

	// Owner (user or organization)
	OwnerID   string `gorm:"not null;index" json:"owner_id"`
	OwnerType string `gorm:"size:10;not null;default:'user'" json:"owner_type"` // "user" or "org"

	// Computed fields (not stored)
	InstallationCount int `gorm:"-" json:"installation_count,omitempty"`
	KeyCount          int `gorm:"-" json:"key_count,omitempty"`
}

// AppPrivateKey stores the RSA public key for a gitpier App.
// Each app can have up to 10 active key pairs. The developer holds
// the corresponding private key PEM (returned only at generation time).
type AppPrivateKey struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	AppID string `gorm:"not null;index" json:"app_id"`

	// SHA256 fingerprint of the DER-encoded public key, displayed as hex.
	Fingerprint string `gorm:"size:80;not null" json:"fingerprint"`

	// PEM-encoded RSA public key. Used to verify JWTs the app signs.
	PublicKeyPEM string `gorm:"type:text;not null" json:"-"`
}

// AppInstallation records that a gitpier App has been installed on a
// user account or organization (with optional specific-repo access).
type AppInstallation struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	AppID string `gorm:"not null;uniqueIndex:idx_app_account" json:"app_id"`
	App   App    `gorm:"foreignKey:AppID" json:"app"`

	// The account that installed the app.
	AccountID   string `gorm:"not null;uniqueIndex:idx_app_account" json:"account_id"`
	AccountType string `gorm:"size:10;not null" json:"account_type"` // "user" or "org"

	// "all" = every repo the account owns; "selected" = only repos in AppInstallationRepository.
	RepositorySelection string `gorm:"size:20;not null;default:'all'" json:"repository_selection"`

	// Snapshot of the permissions granted at install time.
	// Copied from the app's permissions at the moment of installation.
	RepoPermissions    string `gorm:"type:text;not null;default:'{}'" json:"repo_permissions"`
	OrgPermissions     string `gorm:"type:text;not null;default:'{}'" json:"org_permissions"`
	AccountPermissions string `gorm:"type:text;not null;default:'{}'" json:"account_permissions"`

	// Suspension
	SuspendedAt *time.Time `json:"suspended_at"`
	SuspendedBy *string    `json:"suspended_by,omitempty"`

	Repositories []AppInstallationRepository `gorm:"foreignKey:InstallationID" json:"repositories,omitempty"`
}

// AppInstallationRepository lists the specific repos accessible when
// RepositorySelection == "selected".
type AppInstallationRepository struct {
	ID             string `gorm:"primarykey" json:"id"`
	InstallationID string `gorm:"not null;uniqueIndex:idx_install_repo" json:"installation_id"`
	RepoID         string `gorm:"not null;uniqueIndex:idx_install_repo" json:"repo_id"`

	Repo Repository `gorm:"foreignKey:RepoID" json:"repo"`
}

// AppInstallationToken is a short-lived server-to-server access token.
// The app authenticates with a JWT (signed with its private key) to receive one.
// Tokens are prefixed "gla_" and expire after 1 hour.
type AppInstallationToken struct {
	ID             string    `gorm:"primarykey" json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	InstallationID string    `gorm:"not null;index" json:"installation_id"`
	// SHA-256 hex digest of the plaintext token. Never stored in plaintext.
	TokenHash string    `gorm:"size:64;not null;uniqueIndex" json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
}
