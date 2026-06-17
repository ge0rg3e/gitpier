package services

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"gitpier/internal/models"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidCredentials     = errors.New("invalid credentials")
	ErrUserNotFound           = errors.New("user not found")
	ErrEmailTaken             = errors.New("email already taken")
	ErrUsernameTaken          = errors.New("username already taken")
	ErrInvalidRegistrationOTP = errors.New("invalid or expired registration code")
	ErrInvalidTwoFactor       = errors.New("invalid two-factor code")
	ErrTwoFactorRequired      = errors.New("two-factor authentication is required")
	ErrAccountLocked          = errors.New("account temporarily locked due to too many failed login attempts")
)

const (
	maxFailedLoginAttempts     = 10
	lockoutDuration            = 15 * time.Minute
	registrationOTPExpiry      = 10 * time.Minute
	registrationOTPMaxAttempts = 5
	sessionTouchMinInterval    = 60 * time.Second
)

type AuthService struct {
	db             *gorm.DB
	jwtSecret      []byte
	encryptionKey  [32]byte
	sessionTouches sync.Map
	regMailer      RegistrationMailer
}

type Claims struct {
	UserID       string `json:"user_id"`
	Username     string `json:"username"`
	Role         string `json:"role,omitempty"`
	TwoFAPending bool   `json:"two_fa_pending,omitempty"`
	TokenVersion int    `json:"tv,omitempty"`
	// SessionID links this token to a Session row for per-session revocation.
	SessionID string `json:"sid,omitempty"`
	jwt.RegisteredClaims
}

type RegisterInput struct {
	Username      string
	Email         string
	Password      string
	GDPRConsentIP string
	IPAddress     string
	UserAgent     string
}

type LoginInput struct {
	EmailOrUsername       string
	Password              string
	ChallengeToken        string
	TwoFactorCode         string
	TwoFactorRecoveryCode string
	IPAddress             string
	UserAgent             string
}

type LoginResult struct {
	User                    *models.User
	Token                   string
	RequiresTwoFactor       bool
	TwoFactorChallengeToken string
}

type TwoFactorSetup struct {
	Secret     string `json:"secret"`
	OTPAuthURL string `json:"otpauth_url"`
}

func NewAuthService(db *gorm.DB, jwtSecret, encryptionKey string, regMailer RegistrationMailer) *AuthService {
	return &AuthService{
		db:            db,
		jwtSecret:     []byte(jwtSecret),
		encryptionKey: sha256.Sum256([]byte(encryptionKey)),
		regMailer:     regMailer,
	}
}

func (s *AuthService) Register(ctx context.Context, input RegisterInput) (*models.User, string, error) {
	// Check if email is taken
	var existing models.User
	if err := s.db.WithContext(ctx).Where("email = ?", input.Email).First(&existing).Error; err == nil {
		return nil, "", ErrEmailTaken
	}

	// Check if username is taken
	if err := s.db.WithContext(ctx).Where("username = ?", input.Username).First(&existing).Error; err == nil {
		return nil, "", ErrUsernameTaken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", fmt.Errorf("failed to hash password: %w", err)
	}

	now := time.Now().UTC()
	user := &models.User{
		Username:      input.Username,
		Email:         input.Email,
		Password:      string(hash),
		GDPRConsentAt: &now,
		GDPRConsentIP: input.GDPRConsentIP,
	}

	if err := s.db.WithContext(ctx).Create(user).Error; err != nil {
		return nil, "", fmt.Errorf("failed to create user: %w", err)
	}

	token, err := s.createSessionAndToken(ctx, user, input.IPAddress, input.UserAgent)
	if err != nil {
		return nil, "", err
	}

	return user, token, nil
}

// RequestRegistrationOTP stores a pending registration and emits a one-time code
// that must be confirmed before the account is created.
func (s *AuthService) RequestRegistrationOTP(ctx context.Context, input RegisterInput) (string, time.Time, error) {
	var existing models.User
	if err := s.db.WithContext(ctx).Where("LOWER(email) = LOWER(?)", input.Email).First(&existing).Error; err == nil {
		return "", time.Time{}, ErrEmailTaken
	}
	if err := s.db.WithContext(ctx).Where("username = ?", input.Username).First(&existing).Error; err == nil {
		return "", time.Time{}, ErrUsernameTaken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to hash password: %w", err)
	}

	otpCode, err := randomDigits(6)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to generate otp: %w", err)
	}
	otpHash, err := bcrypt.GenerateFromPassword([]byte(otpCode), bcrypt.DefaultCost)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to hash otp: %w", err)
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to generate registration token: %w", err)
	}
	registrationToken := base64.RawURLEncoding.EncodeToString(tokenBytes)
	expiresAt := time.Now().UTC().Add(registrationOTPExpiry)

	pending := &models.PendingRegistration{
		RegistrationToken:    registrationToken,
		Username:             input.Username,
		Email:                input.Email,
		PasswordHash:         string(hash),
		GDPRConsentIP:        input.GDPRConsentIP,
		OTPHash:              string(otpHash),
		OTPExpiresAt:         expiresAt,
		RequestIPAddress:     input.IPAddress,
		RequestUserAgent:     input.UserAgent,
		VerificationAttempts: 0,
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("LOWER(email) = LOWER(?) OR username = ?", input.Email, input.Username).Delete(&models.PendingRegistration{}).Error; err != nil {
			return err
		}
		if err := tx.Create(pending).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create pending registration: %w", err)
	}

	if s.regMailer != nil {
		if err := s.regMailer.SendRegistrationOTP(ctx, pending.Email, pending.Username, otpCode, expiresAt); err != nil {
			_ = s.db.WithContext(ctx).Where("registration_token = ?", registrationToken).Delete(&models.PendingRegistration{}).Error
			return "", time.Time{}, fmt.Errorf("failed to send registration otp: %w", err)
		}
	} else {
		log.Printf("[OTP] registration email=%s code=%s username=%s expires=%s", pending.Email, otpCode, pending.Username, expiresAt.Format(time.RFC3339))
	}

	return registrationToken, expiresAt, nil
}

// VerifyRegistrationOTP validates a pending registration OTP and creates the user account.
func (s *AuthService) VerifyRegistrationOTP(ctx context.Context, registrationToken, otpCode, ipAddress, userAgent string) (*models.User, string, error) {
	registrationToken = strings.TrimSpace(registrationToken)
	otpCode = strings.TrimSpace(otpCode)
	if registrationToken == "" || otpCode == "" {
		return nil, "", ErrInvalidRegistrationOTP
	}

	var createdUser models.User
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var pending models.PendingRegistration
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("registration_token = ?", registrationToken).First(&pending).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidRegistrationOTP
			}
			return err
		}

		now := time.Now().UTC()
		if pending.OTPExpiresAt.Before(now) {
			_ = tx.Where("registration_token = ?", pending.RegistrationToken).Delete(&models.PendingRegistration{}).Error
			return ErrInvalidRegistrationOTP
		}
		if pending.VerificationAttempts >= registrationOTPMaxAttempts {
			return ErrInvalidRegistrationOTP
		}

		if err := bcrypt.CompareHashAndPassword([]byte(pending.OTPHash), []byte(otpCode)); err != nil {
			_ = tx.Model(&models.PendingRegistration{}).
				Where("registration_token = ?", pending.RegistrationToken).
				Update("verification_attempts", gorm.Expr("verification_attempts + 1")).Error
			return ErrInvalidRegistrationOTP
		}

		var existing models.User
		if err := tx.Where("LOWER(email) = LOWER(?)", pending.Email).First(&existing).Error; err == nil {
			return ErrEmailTaken
		}
		if err := tx.Where("username = ?", pending.Username).First(&existing).Error; err == nil {
			return ErrUsernameTaken
		}

		consentAt := now
		createdUser = models.User{
			Username:      pending.Username,
			Email:         pending.Email,
			Password:      pending.PasswordHash,
			GDPRConsentAt: &consentAt,
			GDPRConsentIP: pending.GDPRConsentIP,
		}

		if err := tx.Create(&createdUser).Error; err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}

		if err := tx.Where("registration_token = ?", pending.RegistrationToken).Delete(&models.PendingRegistration{}).Error; err != nil {
			return err
		}

		return nil
	}); err != nil {
		return nil, "", err
	}

	token, err := s.createSessionAndToken(ctx, &createdUser, ipAddress, userAgent)
	if err != nil {
		return nil, "", err
	}

	return &createdUser, token, nil
}

// SessionResponse is the public view of a session used in the sessions list API.
type SessionResponse struct {
	ID         string    `json:"id"`
	TokenID    string    `json:"token_id"`
	IPAddress  string    `json:"ip_address"`
	UserAgent  string    `json:"user_agent"`
	Browser    string    `json:"browser"`
	OS         string    `json:"os"`
	IsMobile   bool      `json:"is_mobile"`
	LastSeenAt time.Time `json:"last_seen_at"`
	CreatedAt  time.Time `json:"created_at"`
	IsCurrent  bool      `json:"is_current"`
}

func (s *AuthService) Login(ctx context.Context, input LoginInput) (*LoginResult, error) {
	if input.ChallengeToken != "" {
		return s.completeTwoFactorLogin(ctx, input)
	}

	var user models.User
	if err := s.db.WithContext(ctx).
		Where("email = ? OR username = ?", input.EmailOrUsername, input.EmailOrUsername).
		First(&user).Error; err != nil {
		return nil, ErrInvalidCredentials
	}

	// Check account lockout before verifying the password.
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now().UTC()) {
		return nil, ErrAccountLocked
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)); err != nil {
		// Increment failed-attempt counter; lock after maxFailedLoginAttempts failures.
		updates := map[string]interface{}{
			"failed_login_attempts": gorm.Expr("failed_login_attempts + 1"),
		}
		if user.FailedLoginAttempts+1 >= maxFailedLoginAttempts {
			lockUntil := time.Now().UTC().Add(lockoutDuration)
			updates["locked_until"] = lockUntil
			updates["failed_login_attempts"] = 0
		}
		s.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", user.ID).Updates(updates)
		return nil, ErrInvalidCredentials
	}

	if user.IsSuspended {
		return nil, ErrInvalidCredentials
	}

	// Reset counter on successful password verification.
	if user.FailedLoginAttempts > 0 || user.LockedUntil != nil {
		s.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
			"failed_login_attempts": 0,
			"locked_until":          nil,
		})
	}

	if user.TwoFAEnabled {
		challengeToken, err := s.generateTwoFactorChallengeToken(&user)
		if err != nil {
			return nil, err
		}
		return &LoginResult{RequiresTwoFactor: true, TwoFactorChallengeToken: challengeToken}, nil
	}

	token, err := s.createSessionAndToken(ctx, &user, input.IPAddress, input.UserAgent)
	if err != nil {
		return nil, err
	}

	return &LoginResult{User: &user, Token: token}, nil
}

func (s *AuthService) completeTwoFactorLogin(ctx context.Context, input LoginInput) (*LoginResult, error) {
	claims, err := s.ValidateToken(input.ChallengeToken)
	if err != nil || !claims.TwoFAPending {
		return nil, ErrInvalidCredentials
	}

	var user models.User
	if err := s.db.WithContext(ctx).Where("id = ?", claims.UserID).First(&user).Error; err != nil {
		return nil, ErrInvalidCredentials
	}
	if !user.TwoFAEnabled {
		return nil, ErrInvalidCredentials
	}
	if user.IsSuspended {
		return nil, ErrInvalidCredentials
	}

	if err := s.verifyTwoFactorCode(ctx, &user, input.TwoFactorCode, input.TwoFactorRecoveryCode, true); err != nil {
		return nil, err
	}

	token, err := s.createSessionAndToken(ctx, &user, input.IPAddress, input.UserAgent)
	if err != nil {
		return nil, err
	}

	return &LoginResult{User: &user, Token: token}, nil
}

func (s *AuthService) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	// Skip version check for short-lived two-factor challenge tokens.
	if !claims.TwoFAPending {
		var user models.User
		if err := s.db.Select("token_version").Where("id = ?", claims.UserID).First(&user).Error; err != nil {
			return nil, errors.New("invalid token")
		}
		if user.TokenVersion != claims.TokenVersion {
			return nil, errors.New("token has been revoked")
		}
		// Per-session revocation check.
		if claims.SessionID != "" {
			var session models.Session
			err := s.db.Select("revoked_at").Where("token_id = ?", claims.SessionID).First(&session).Error
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					// Session not found - treat as revoked
					return nil, errors.New("session has been revoked")
				}
				// Database error - don't invalidate the token, return the error
				log.Printf("[WARN] session lookup failed: %v", err)
				return nil, fmt.Errorf("session lookup failed: %w", err)
			}
			if session.RevokedAt != nil {
				return nil, errors.New("session has been revoked")
			}
		}
	}

	return claims, nil
}

func (s *AuthService) GetUserByID(ctx context.Context, id string) (*models.User, error) {
	var user models.User
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&user).Error; err != nil {
		return nil, ErrUserNotFound
	}
	return &user, nil
}

func (s *AuthService) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	var user models.User
	if err := s.db.WithContext(ctx).Where("username = ?", username).First(&user).Error; err != nil {
		return nil, ErrUserNotFound
	}
	return &user, nil
}

func (s *AuthService) GetUsersByEmails(ctx context.Context, emails []string) (map[string]*models.User, error) {
	unique := make([]string, 0, len(emails))
	seen := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		normalized := strings.ToLower(strings.TrimSpace(email))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		unique = append(unique, normalized)
	}
	if len(unique) == 0 {
		return map[string]*models.User{}, nil
	}

	var users []models.User
	if err := s.db.WithContext(ctx).Where("LOWER(email) IN ?", unique).Find(&users).Error; err != nil {
		return nil, err
	}

	byEmail := make(map[string]*models.User, len(users))
	for i := range users {
		user := users[i]
		byEmail[strings.ToLower(strings.TrimSpace(user.Email))] = &user
	}

	return byEmail, nil
}

func (s *AuthService) UpdateUser(ctx context.Context, id string, updates map[string]interface{}) error {
	return s.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", id).Updates(updates).Error
}

func (s *AuthService) VerifyPassword(ctx context.Context, userID string, password string) error {
	var user models.User
	if err := s.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
		return ErrUserNotFound
	}
	return bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
}

// ChangePassword verifies the current password, sets the new hash, and
// increments token_version to invalidate all existing sessions.
func (s *AuthService) ChangePassword(ctx context.Context, userID string, currentPassword, newPassword string) error {
	var user models.User
	if err := s.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
		return ErrUserNotFound
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	return s.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"password":      string(hash),
		"token_version": gorm.Expr("token_version + 1"),
	}).Error
}

func (s *AuthService) GetUserBySSHFingerprint(ctx context.Context, fingerprint string) (*models.User, error) {
	var key models.SSHKey
	if err := s.db.WithContext(ctx).Where("fingerprint = ?", fingerprint).First(&key).Error; err != nil {
		return nil, ErrUserNotFound
	}

	var user models.User
	if err := s.db.WithContext(ctx).Where("id = ?", key.UserID).First(&user).Error; err != nil {
		return nil, ErrUserNotFound
	}

	return &user, nil
}

func (s *AuthService) GetTwoFactorStatus(ctx context.Context, userID string) (bool, bool, error) {
	var user models.User
	if err := s.db.WithContext(ctx).Select("id", "two_fa_enabled", "two_fa_secret").Where("id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, false, ErrUserNotFound
		}
		return false, false, err
	}
	hasPendingSetup := !user.TwoFAEnabled && strings.TrimSpace(user.TwoFASecret) != ""
	return user.TwoFAEnabled, hasPendingSetup, nil
}

func (s *AuthService) StartTwoFactorSetup(ctx context.Context, userID string) (*TwoFactorSetup, error) {
	var user models.User
	if err := s.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, ErrUserNotFound
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "GitPier",
		AccountName: user.Email,
		Algorithm:   otp.AlgorithmSHA1,
		Digits:      otp.DigitsSix,
		Period:      30,
		SecretSize:  20,
	})
	if err != nil {
		return nil, fmt.Errorf("generate totp key: %w", err)
	}

	encryptedSecret, err := s.encrypt(key.Secret())
	if err != nil {
		return nil, fmt.Errorf("encrypt totp secret: %w", err)
	}

	if err := s.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"two_fa_enabled":        false,
		"two_fa_secret":         encryptedSecret,
		"two_fa_recovery_codes": "",
	}).Error; err != nil {
		return nil, err
	}

	return &TwoFactorSetup{Secret: key.Secret(), OTPAuthURL: key.URL()}, nil
}

func (s *AuthService) EnableTwoFactor(ctx context.Context, userID string, code string) ([]string, error) {
	var user models.User
	if err := s.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, ErrUserNotFound
	}

	if strings.TrimSpace(user.TwoFASecret) == "" {
		return nil, errors.New("two-factor setup has not been started")
	}

	if err := s.verifyTwoFactorCode(ctx, &user, code, "", false); err != nil {
		return nil, err
	}

	plainCodes, hashedCodes, err := s.generateRecoveryCodes(10)
	if err != nil {
		return nil, err
	}
	hashesJSON, err := json.Marshal(hashedCodes)
	if err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"two_fa_enabled":        true,
		"two_fa_recovery_codes": string(hashesJSON),
		"token_version":         gorm.Expr("token_version + 1"),
	}).Error; err != nil {
		return nil, err
	}

	return plainCodes, nil
}

func (s *AuthService) DisableTwoFactor(ctx context.Context, userID string, code, recoveryCode string) error {
	var user models.User
	if err := s.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
		return ErrUserNotFound
	}
	if !user.TwoFAEnabled {
		return nil
	}

	if err := s.verifyTwoFactorCode(ctx, &user, code, recoveryCode, true); err != nil {
		return err
	}

	return s.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"two_fa_enabled":        false,
		"two_fa_secret":         "",
		"two_fa_recovery_codes": "",
		"token_version":         gorm.Expr("token_version + 1"),
	}).Error
}

func (s *AuthService) RegenerateTwoFactorRecoveryCodes(ctx context.Context, userID string, code, recoveryCode string) ([]string, error) {
	var user models.User
	if err := s.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, ErrUserNotFound
	}
	if !user.TwoFAEnabled {
		return nil, errors.New("two-factor authentication is not enabled")
	}

	if err := s.verifyTwoFactorCode(ctx, &user, code, recoveryCode, true); err != nil {
		return nil, err
	}

	plainCodes, hashedCodes, err := s.generateRecoveryCodes(10)
	if err != nil {
		return nil, err
	}
	hashesJSON, err := json.Marshal(hashedCodes)
	if err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", userID).Update("two_fa_recovery_codes", string(hashesJSON)).Error; err != nil {
		return nil, err
	}

	return plainCodes, nil
}

func (s *AuthService) verifyTwoFactorCode(ctx context.Context, user *models.User, code, recoveryCode string, consumeRecovery bool) error {
	secret, err := s.decrypt(user.TwoFASecret)
	if err != nil || strings.TrimSpace(secret) == "" {
		return ErrInvalidTwoFactor
	}

	trimmedCode := strings.TrimSpace(code)
	if trimmedCode != "" {
		if !totp.Validate(trimmedCode, secret) {
			return ErrInvalidTwoFactor
		}
		return nil
	}

	trimmedRecovery := normalizeRecoveryCode(recoveryCode)
	if trimmedRecovery == "" {
		return ErrInvalidTwoFactor
	}

	hashes, err := parseRecoveryHashes(user.TwoFARecoveryCodes)
	if err != nil {
		return ErrInvalidTwoFactor
	}

	for i, hash := range hashes {
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(trimmedRecovery)) == nil {
			if consumeRecovery {
				hashes = append(hashes[:i], hashes[i+1:]...)
				next, _ := json.Marshal(hashes)
				if err := s.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", user.ID).Update("two_fa_recovery_codes", string(next)).Error; err != nil {
					return err
				}
				user.TwoFARecoveryCodes = string(next)
			}
			return nil
		}
	}

	return ErrInvalidTwoFactor
}

func (s *AuthService) generateRecoveryCodes(count int) ([]string, []string, error) {
	plain := make([]string, 0, count)
	hashes := make([]string, 0, count)
	for i := 0; i < count; i++ {
		raw, err := randomString("ABCDEFGHJKLMNPQRSTUVWXYZ23456789", 8)
		if err != nil {
			return nil, nil, err
		}
		code := raw[:4] + "-" + raw[4:]
		normalized := normalizeRecoveryCode(code)
		hash, err := bcrypt.GenerateFromPassword([]byte(normalized), bcrypt.DefaultCost)
		if err != nil {
			return nil, nil, err
		}
		plain = append(plain, code)
		hashes = append(hashes, string(hash))
	}
	return plain, hashes, nil
}

func parseRecoveryHashes(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	var hashes []string
	if err := json.Unmarshal([]byte(raw), &hashes); err != nil {
		return nil, err
	}
	return hashes, nil
}

func normalizeRecoveryCode(code string) string {
	cleaned := strings.ToUpper(strings.TrimSpace(code))
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	return cleaned
}

func randomString(alphabet string, length int) (string, error) {
	b := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b), nil
}

func randomDigits(length int) (string, error) {
	return randomString("0123456789", length)
}

func (s *AuthService) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(s.encryptionKey[:])
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

func (s *AuthService) decrypt(encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.encryptionKey[:])
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

func (s *AuthService) generateToken(user *models.User) (string, error) {
	claims := &Claims{
		UserID:       user.ID,
		Username:     user.Username,
		Role:         user.Role,
		TokenVersion: user.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(72 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// newSessionID generates a random 16-byte hex session identifier.
func newSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// parseUserAgent does lightweight UA detection without external dependencies.
func parseUserAgent(ua string) (browser, os string, isMobile bool) {
	l := strings.ToLower(ua)
	isMobile = strings.Contains(l, "mobile") || strings.Contains(l, "iphone") || strings.Contains(l, "android")
	switch {
	case strings.Contains(l, "windows"):
		os = "Windows"
	case strings.Contains(l, "mac os x") || strings.Contains(l, "macos"):
		os = "macOS"
	case strings.Contains(l, "android"):
		os = "Android"
	case strings.Contains(l, "iphone") || strings.Contains(l, "ipad"):
		os = "iOS"
	case strings.Contains(l, "linux"):
		os = "Linux"
	default:
		os = "Unknown"
	}
	switch {
	case strings.Contains(l, "edg"):
		browser = "Edge"
	case strings.Contains(l, "firefox"):
		browser = "Firefox"
	case strings.Contains(l, "chrome"):
		browser = "Chrome"
	case strings.Contains(l, "safari"):
		browser = "Safari"
	case strings.Contains(l, "curl"):
		browser = "curl"
	case strings.Contains(l, "git/"):
		browser = "Git"
	default:
		browser = "Unknown"
	}
	return
}

// createSessionAndToken creates a Session row and generates a JWT embedding the session ID.
func (s *AuthService) createSessionAndToken(ctx context.Context, user *models.User, ipAddress, userAgent string) (string, error) {
	sessionID := newSessionID()
	browser, os, isMobile := parseUserAgent(userAgent)
	now := time.Now().UTC()
	session := &models.Session{
		UserID:     user.ID,
		TokenID:    sessionID,
		IPAddress:  ipAddress,
		UserAgent:  userAgent,
		Browser:    browser,
		OS:         os,
		IsMobile:   isMobile,
		LastSeenAt: now,
	}
	if err := s.db.WithContext(ctx).Create(session).Error; err != nil {
		// Non-fatal: fall back to token without session tracking.
		log.Printf("[WARN] failed to create session row: %v", err)
		return s.generateToken(user)
	}

	claims := &Claims{
		UserID:       user.ID,
		Username:     user.Username,
		Role:         user.Role,
		TokenVersion: user.TokenVersion,
		SessionID:    sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(72 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        sessionID,
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(s.jwtSecret)
}

// IssueSessionToken creates a new session and JWT for the given user.
// Used after password changes to keep the user logged in under a fresh session.
func (s *AuthService) IssueSessionToken(ctx context.Context, userID string, ipAddress, userAgent string) (string, error) {
	var user models.User
	if err := s.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
		return "", ErrUserNotFound
	}
	return s.createSessionAndToken(ctx, &user, ipAddress, userAgent)
}

// IssueTokenForUser loads the latest user state (including the updated
// token_version after a password change) and generates a fresh JWT.
func (s *AuthService) IssueTokenForUser(ctx context.Context, userID string) (string, error) {
	var user models.User
	if err := s.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
		return "", ErrUserNotFound
	}
	return s.generateToken(&user)
}

// TouchSession updates last_seen_at for the given session ID. Intended to be
// called from a goroutine so it does not block the request path.
func (s *AuthService) TouchSession(tokenID string) {
	if strings.TrimSpace(tokenID) == "" {
		return
	}

	now := time.Now().UTC()
	if lastRaw, ok := s.sessionTouches.Load(tokenID); ok {
		if last, ok := lastRaw.(time.Time); ok && now.Sub(last) < sessionTouchMinInterval {
			return
		}
	}

	cutoff := now.Add(-sessionTouchMinInterval)
	res := s.db.Model(&models.Session{}).
		Where("token_id = ? AND revoked_at IS NULL AND (last_seen_at IS NULL OR last_seen_at < ?)", tokenID, cutoff).
		Update("last_seen_at", now)
	if res.Error == nil {
		s.sessionTouches.Store(tokenID, now)
	}
}

// ListSessions returns all active (non-revoked) sessions for the user,
// marking which one is the current request's session.
func (s *AuthService) ListSessions(ctx context.Context, userID string, currentTokenID string) ([]SessionResponse, error) {
	var sessions []models.Session
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Order("last_seen_at DESC").
		Find(&sessions).Error; err != nil {
		return nil, err
	}
	resp := make([]SessionResponse, 0, len(sessions))
	for _, sess := range sessions {
		resp = append(resp, SessionResponse{
			ID:         sess.ID,
			TokenID:    sess.TokenID,
			IPAddress:  sess.IPAddress,
			UserAgent:  sess.UserAgent,
			Browser:    sess.Browser,
			OS:         sess.OS,
			IsMobile:   sess.IsMobile,
			LastSeenAt: sess.LastSeenAt,
			CreatedAt:  sess.CreatedAt,
			IsCurrent:  sess.TokenID == currentTokenID,
		})
	}
	return resp, nil
}

// RevokeSession marks a single session as revoked. Only the owning user may revoke it.
func (s *AuthService) RevokeSession(ctx context.Context, userID string, tokenID string) error {
	now := time.Now().UTC()
	result := s.db.WithContext(ctx).Model(&models.Session{}).
		Where("user_id = ? AND token_id = ? AND revoked_at IS NULL", userID, tokenID).
		Update("revoked_at", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrUserNotFound // session not found or already revoked
	}
	return nil
}

// RevokeOtherSessions revokes every session for the user except the current one.
func (s *AuthService) RevokeOtherSessions(ctx context.Context, userID string, currentTokenID string) error {
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&models.Session{}).
		Where("user_id = ? AND token_id != ? AND revoked_at IS NULL", userID, currentTokenID).
		Update("revoked_at", now).Error
}

func (s *AuthService) generateTwoFactorChallengeToken(user *models.User) (string, error) {
	claims := &Claims{
		UserID:       user.ID,
		Username:     user.Username,
		TwoFAPending: true,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// ValidateCredentials checks username/password and returns the user. Used by the
// container registry token endpoint.
func (s *AuthService) ValidateCredentials(ctx context.Context, username, password string) (*models.User, error) {
	var user models.User
	if err := s.db.WithContext(ctx).
		Where("username = ? OR email = ?", username, username).
		First(&user).Error; err != nil {
		return nil, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	if user.IsSuspended {
		return nil, ErrInvalidCredentials
	}
	return &user, nil
}

// RegistryTokenClaims is a short-lived JWT for the OCI container registry.
type RegistryTokenClaims struct {
	Username string `json:"username"`
	UserID   string `json:"user_id"`
	jwt.RegisteredClaims
}

// IssueRegistryToken issues a short-lived (15 min) JWT for registry auth.
func (s *AuthService) IssueRegistryToken(user *models.User) (string, error) {
	claims := &RegistryTokenClaims{
		Username: user.Username,
		UserID:   user.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Audience:  jwt.ClaimStrings{"registry"},
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// RequestPasswordReset generates a secure reset token for the given email address,
// stores it (hashed) in the database and logs it to the console for debug purposes.
// If the email is not found the function returns nil to avoid account enumeration.
func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) error {
	var user models.User
	if err := s.db.WithContext(ctx).Where("LOWER(email) = LOWER(?)", email).First(&user).Error; err != nil {
		// Do not reveal whether the email exists.
		return nil
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("failed to generate reset token: %w", err)
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)
	expires := time.Now().UTC().Add(time.Hour)

	if err := s.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
		"password_reset_token":      token,
		"password_reset_expires_at": expires,
	}).Error; err != nil {
		return err
	}

	log.Printf("[DEBUG] Password reset token for %s: %s (expires: %s)", user.Email, token, expires.Format(time.RFC3339))
	return nil
}

// ResetPassword validates the reset token and sets a new password.
// The token is consumed immediately (single-use) and all sessions are invalidated.
func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword string) error {
	if strings.TrimSpace(token) == "" {
		return ErrInvalidCredentials
	}

	var user models.User
	if err := s.db.WithContext(ctx).Where("password_reset_token = ?", token).First(&user).Error; err != nil {
		return ErrInvalidCredentials
	}
	if user.PasswordResetExpiresAt == nil || user.PasswordResetExpiresAt.Before(time.Now().UTC()) {
		return ErrInvalidCredentials
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	return s.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
		"password":                  string(hash),
		"password_reset_token":      "",
		"password_reset_expires_at": nil,
		"token_version":             gorm.Expr("token_version + 1"),
	}).Error
}

// ValidateRegistryToken validates a registry JWT and returns the claims.
func (s *AuthService) ValidateRegistryToken(tokenStr string) (*RegistryTokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &RegistryTokenClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidCredentials
	}
	claims, ok := token.Claims.(*RegistryTokenClaims)
	if !ok {
		return nil, ErrInvalidCredentials
	}
	// Verify audience
	for _, aud := range claims.Audience {
		if aud == "registry" {
			return claims, nil
		}
	}
	return nil, ErrInvalidCredentials
}
