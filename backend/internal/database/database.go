package database

import (
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"reflect"
	"time"

	"gitpier/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(dsn string, maxOpenConns, maxIdleConns, connMaxLifetimeMins int) (*gorm.DB, error) {
	logLevel := logger.Warn
	if os.Getenv("DB_DEBUG") == "true" {
		logLevel = logger.Info
	}
	gormLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			LogLevel:                  logLevel,
			SlowThreshold:             time.Second,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return nil, err
	}
	if err := db.Callback().Create().Before("gorm:create").Register("gitpier:assign_uuid", assignUUIDBeforeCreate); err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(connMaxLifetimeMins) * time.Minute)

	return db, nil
}

func assignUUIDBeforeCreate(db *gorm.DB) {
	if db == nil || db.Statement == nil {
		return
	}
	setUUIDOnValue(db.Statement.ReflectValue)
}

func setUUIDOnValue(v reflect.Value) {
	if !v.IsValid() {
		return
	}
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		f := v.FieldByName("ID")
		if f.IsValid() && f.CanSet() && f.Kind() == reflect.String && f.String() == "" {
			if id, err := newUUIDv4(); err == nil {
				f.SetString(id)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			setUUIDOnValue(v.Index(i))
		}
	}
}

func newUUIDv4() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant RFC 4122
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func Migrate(db *gorm.DB) error {
	err := db.AutoMigrate(
		&models.User{},
		&models.UserFollow{},
		&models.Organization{},
		&models.OrgFollow{},
		&models.OrganizationMember{},
		&models.Team{},
		&models.TeamMember{},
		&models.TeamRepository{},
		&models.Repository{},
		&models.Collaborator{},
		&models.SSHKey{},
		&models.PersonalAccessToken{},
		&models.Star{},
		&models.PullRequest{},
		&models.PRComment{},
		&models.PRReview{},
		&models.PRReviewComment{},
		&models.WorkflowRun{},
		&models.WorkflowJob{},
		&models.WorkflowStep{},
		&models.WorkflowUsage{},
		&models.WorkflowMinutesUsage{},
		&models.RepoVariable{},
		&models.RepoSecret{},
		&models.Release{},
		&models.ReleaseAsset{},
		&models.Issue{},
		&models.Label{},
		&models.IssueComment{},
		&models.Milestone{},
		&models.Project{},
		&models.ProjectColumn{},
		&models.ProjectItem{},
		&models.ModerationPolicy{},
		&models.ModerationBlockedUser{},
		&models.ModerationBlockedKeyword{},
		&models.Webhook{},
		&models.WebhookDelivery{},
		&models.OAuthApp{},
		&models.OAuthAuthorization{},
		&models.OAuthCode{},
		&models.OAuthToken{},
		&models.OAuthDeviceCode{},
		// GitPier Apps
		&models.App{},
		&models.AppPrivateKey{},
		&models.AppInstallation{},
		&models.AppInstallationRepository{},
		&models.AppInstallationToken{},
		// Container Registry (OCI)
		&models.ContainerRepository{},
		&models.ContainerBlob{},
		&models.ContainerUpload{},
		&models.ContainerManifest{},
		&models.ContainerTag{},
		&models.Session{},
		// Anti-spam and security
		&models.AccountCreationAttempt{},
		&models.PendingRegistration{},
	)
	if err != nil {
		return err
	}

	// DB-level safety net: ensure any INSERT with missing/empty text ID gets a UUID.
	// This covers all tables, including writes that bypass GORM hooks.
	if err := db.Exec(`CREATE EXTENSION IF NOT EXISTS pgcrypto`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
CREATE OR REPLACE FUNCTION gitpier_assign_text_id()
RETURNS trigger AS $$
BEGIN
	IF NEW.id IS NULL OR NEW.id = '' THEN
		NEW.id := gen_random_uuid()::text;
	END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
DO $$
DECLARE
	r RECORD;
	trg_name text;
BEGIN
	FOR r IN
		SELECT table_schema, table_name
		FROM information_schema.columns
		WHERE column_name = 'id'
		  AND table_schema = 'public'
	LOOP
		trg_name := 'trg_assign_id_' || r.table_name;
		EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I.%I', trg_name, r.table_schema, r.table_name);
		EXECUTE format(
			'CREATE TRIGGER %I BEFORE INSERT ON %I.%I FOR EACH ROW EXECUTE FUNCTION gitpier_assign_text_id()',
			trg_name, r.table_schema, r.table_name
		);
	END LOOP;
END $$;
`).Error; err != nil {
		return err
	}

	return nil
}
