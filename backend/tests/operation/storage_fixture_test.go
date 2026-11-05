package operation_test

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm/logger"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/tests/storagefixture"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/storageoperator"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// businessStorageConfig uses only the explicitly selected isolated storage fixture.
func businessStorageConfig(t *testing.T) config.ObjectStorageConfig {
	t.Helper()
	return storagefixture.Configured(t)
}

func businessStorageRegistry(t *testing.T) *objectstorage.Registry {
	t.Helper()
	return storagefixture.OpenConfigured(t)
}

func businessStorageEnvironment(t *testing.T, database *gorm.DB) {
	t.Helper()
	environment, err := storageoperator.Configuration(businessStorageConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pgmedia.NewStorageRoutes(database).ApplyEnvironment(t.Context(), environment); err != nil {
		t.Fatal(err)
	}
}

func businessPinOperation(t *testing.T, database *gorm.DB, identity operationapp.ProviderDispatchIdentity) {
	t.Helper()
	businessStorageEnvironment(t, database)
	if _, err := pgmedia.NewStorageRoutes(database).PinStorageWork(t.Context(), mediaapp.StorageWorkOwner{Kind: mediaapp.StorageWorkKindOperation, OwnerID: identity.ProjectID, WorkID: identity.OperationID}); err != nil {
		t.Fatal(err)
	}
}

// businessFixtureLocation creates metadata-only historical storage evidence. It
// does not create bytes and must never be used as proof of real object I/O.
func businessFixtureLocation(t *testing.T, database *gorm.DB, key string) uuid.UUID {
	t.Helper()
	cfg := businessStorageConfig(t)
	p := cfg.Profiles[cfg.DefaultWriteProfile]
	profile := mediaapp.StorageProfile{ID: cfg.DefaultWriteProfile, Provider: string(p.Provider), Endpoint: p.Endpoint, SigningEndpoint: p.SigningEndpoint, Bucket: p.Bucket, Region: p.Region, AddressingStyle: string(p.AddressingStyle)}
	if err := pgmedia.NewStorageRoutes(database).RegisterProfiles(t.Context(), []mediaapp.StorageProfile{profile}); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if err := database.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state) VALUES(?,?,?,?,'accepted') ON CONFLICT(profile_id,bucket,object_key) DO NOTHING`, id, profile.ID, profile.Bucket, key).Error; err != nil {
		t.Fatal(err)
	}
	var saved struct{ ID uuid.UUID }
	if err := database.Raw(`SELECT id FROM media.object_location WHERE profile_id=? AND bucket=? AND object_key=?`, profile.ID, profile.Bucket, key).Scan(&saved).Error; err != nil {
		t.Fatal(err)
	}
	return saved.ID
}
func businessFixtureKey(id any) string { return "fixture/" + fmt.Sprint(id) }

// businessEmptyFixtureDB gives SQL-shape fixtures their own empty database so
// repeated and race runs never reuse a prior test's shortened schema.
func businessEmptyFixtureDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Hostname() != "127.0.0.1" || !strings.HasPrefix(strings.TrimPrefix(parsed.Path, "/"), "glowloom_") {
		t.Fatal("SQL fixture requires an isolated loopback database", err)
	}
	admin, err := gorm.Open(gormpostgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	name := "glowloom_business_sql_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec(`CREATE DATABASE ` + name + ` TEMPLATE template0`).Error; err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	database, err := gorm.Open(gormpostgres.Open(parsed.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := database.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if err := admin.Exec(`DROP DATABASE ` + name + ` WITH (FORCE)`).Error; err != nil {
			t.Error("remove exact SQL fixture database", err)
		}
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return database
}

// The SQL projection fixtures retain real physical identity columns without
// substituting their deliberately minimal schemas for production migration tests.
const businessSQLLocationTables = `
 CREATE TABLE media.storage_profile(id text PRIMARY KEY,provider text NOT NULL,endpoint text NOT NULL,signing_endpoint text NOT NULL,region text NOT NULL,bucket text NOT NULL,addressing_style text NOT NULL,resource_identity text NOT NULL,created_at timestamptz DEFAULT now());
 CREATE TABLE media.object_location(id uuid PRIMARY KEY,profile_id text NOT NULL REFERENCES media.storage_profile(id),bucket text NOT NULL,object_key text NOT NULL,state text NOT NULL,byte_size bigint,sha256 text,mime_type text,created_at timestamptz DEFAULT now(),updated_at timestamptz DEFAULT now(),owner_session_id uuid,owner_intent_id uuid,write_until timestamptz,UNIQUE(profile_id,bucket,object_key));`
