package subject_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	pgsubject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/adapter/postgres"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
)

type legacyVoiceAccess struct{ owner subject.VoiceOwner }

func (a legacyVoiceAccess) Request(context.Context, identity.Principal, bool) (subject.VoiceOwner, error) {
	return a.owner, nil
}
func (a legacyVoiceAccess) Worker(context.Context, subject.VoiceWork) (subject.VoiceOwner, error) {
	return a.owner, nil
}

type legacyVoiceStorage struct{ routes *pgmedia.StorageRoutes }

func (s legacyVoiceStorage) Ready(ctx context.Context) (bool, error) {
	return s.routes.ReadyForWork(ctx)
}
func (legacyVoiceStorage) Pin(context.Context, uuid.UUID, uuid.UUID) (subject.VoiceStorageIntent, error) {
	return subject.VoiceStorageIntent{}, errors.New("must not pin an unready route")
}
func (legacyVoiceStorage) Read(context.Context, uuid.UUID, uuid.UUID) (subject.VoiceStorageIntent, error) {
	return subject.VoiceStorageIntent{}, media.ErrUnavailable
}

// This test creates its own new UUID database. The selected shared subject DB
// is used only as the validated administration connection; no table is dropped.
func TestVoiceLegacyMigrationWithoutStorageAndExplicitOptionsReadiness(t *testing.T) {
	_, admin := subjectPostgres(t)
	dsn, err := url.Parse(os.Getenv("GLOWLOOM_TEST_SUBJECT_DB_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	name := "glowloom_subject_alignment_vl_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec(`CREATE DATABASE ` + name).Error; err != nil {
		t.Fatal("create own legacy fixture database", err)
	}
	t.Log("owned legacy fixture database", name)
	dsn.Path = "/" + name
	open := func(dsn string) *gorm.DB {
		c, err := platformdb.Open(t.Context(), dsn, noop.NewTracerProvider())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c.DB
	}
	owner := open(dsn.String())
	q := dsn.Query()
	q.Set("options", "-c role=glowloom_app")
	dsn.RawQuery = q.Encode()
	appDB := open(dsn.String())
	base := `CREATE SCHEMA identity;CREATE SCHEMA workspace;CREATE SCHEMA catalog;CREATE SCHEMA media;CREATE SCHEMA subject;
 CREATE TABLE identity."user"(id uuid PRIMARY KEY);
 CREATE TABLE workspace.creation_scope(id uuid NOT NULL,physical_scope_id uuid NOT NULL,UNIQUE(id,physical_scope_id));
 CREATE TABLE catalog.model_release(id uuid PRIMARY KEY);CREATE TABLE catalog.provider(id uuid PRIMARY KEY);CREATE TABLE catalog.provider_config_version(id uuid PRIMARY KEY);CREATE TABLE catalog.provider_credential(id uuid PRIMARY KEY);CREATE TABLE catalog.model_profile_version(id uuid PRIMARY KEY);CREATE TABLE catalog.price_rule_version(id uuid PRIMARY KEY);
 CREATE TABLE media.media_asset(id uuid PRIMARY KEY);GRANT USAGE ON SCHEMA identity,workspace,catalog,media,subject TO glowloom_app;`
	if err := owner.Exec(base).Error; err != nil {
		t.Fatal("minimal pre-storage schema", err)
	}
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(source), "..", "..")
	migration, err := os.ReadFile(filepath.Join(root, "db", "subject_voice_incremental.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := owner.Exec(string(migration)).Error; err != nil {
			t.Fatal("additive migration must tolerate absent work storage and replay", err)
		}
	}
	var absent bool
	if err := owner.Raw(`SELECT to_regclass('media.storage_work_intent') IS NULL AND to_regclass('media.object_location') IS NULL AND to_regclass('media.storage_profile') IS NULL AND to_regclass('subject.voice_job') IS NOT NULL`).Scan(&absent).Error; err != nil || !absent {
		t.Fatal("voice migration fabricated storage facts", err)
	}
	var storageFKs int64
	if err := owner.Raw(`SELECT count(*) FROM pg_constraint WHERE conrelid='subject.voice_job'::regclass AND contype='f' AND confrelid IN (SELECT oid FROM pg_class WHERE relnamespace='media'::regnamespace AND relname IN ('storage_work_intent','object_location'))`).Scan(&storageFKs).Error; err != nil || storageFKs != 0 {
		t.Fatal("independent job still requires missing storage relations", err)
	}
	actor := identity.Principal{ID: uuid.New(), ScopeID: uuid.New(), OrgID: uuid.New()}
	authority := legacyVoiceAccess{subject.VoiceOwner{ScopeID: actor.ScopeID, PhysicalScopeID: actor.OrgID, PersonalScopeID: uuid.New(), PersonalOrgID: uuid.New()}}
	release := subject.VoiceRelease{VoiceModelOption: subject.VoiceModelOption{ReleaseID: uuid.New(), Voices: []subject.VoiceOption{{Key: "fixture-speaker", Name: "fixture"}}}}
	store := pgsubject.NewVoiceStore(appDB, pgsubject.VoiceFactories{Access: func(*gorm.DB) subject.VoiceAccess { return authority }, Catalog: func(*gorm.DB) subject.VoiceCatalog { return voiceCatalogFixture{release} }, Storage: func(tx *gorm.DB) subject.VoiceStorage { return legacyVoiceStorage{pgmedia.NewStorageRoutes(tx)} }})
	assertUnavailable := func() {
		t.Helper()
		options, err := store.Options(t.Context(), actor)
		if err != nil || options.Available || options.Reason != "voice_storage_unavailable" || len(options.Models) != 0 {
			t.Fatal("missing storage looked executable despite configured catalog fixture", options, err)
		}
	}
	assertUnavailable()
	schema, err := os.ReadFile(filepath.Join(root, "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"storage_profile", "storage_route", "storage_work_intent", "object_location"} {
		definition := regexp.MustCompile(`(?s)CREATE TABLE media\.` + table + ` \(.*?\n\);`).Find(schema)
		if len(definition) == 0 {
			t.Fatal("canonical storage definition missing", table)
		}
		if err := owner.Exec(string(definition)).Error; err != nil {
			t.Fatal("own fixture canonical storage table", table, err)
		}
		if err := owner.Exec(`GRANT SELECT ON media.` + table + ` TO glowloom_app`).Error; err != nil {
			t.Fatal(err)
		}
	}
	assertUnavailable() // Having tables does not activate or invent an output route.
	if err := owner.Exec(`INSERT INTO media.storage_profile(id,provider,endpoint,signing_endpoint,region,bucket,addressing_style,resource_identity) VALUES('legacy-fixture','minio','http://127.0.0.1:1','http://127.0.0.1:1','fixture','fixture','path',?)`, strings.Repeat("a", 64)).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`INSERT INTO media.storage_route(singleton,revision,profile_id) VALUES(1,'fixture-revision','legacy-fixture')`).Error; err != nil {
		t.Fatal(err)
	}
	options, err := store.Options(t.Context(), actor)
	if err != nil || !options.Available || options.Reason != "" {
		t.Fatal("existing explicit output route not recognized", options, err)
	}
	if err := owner.Exec(`REVOKE SELECT ON media.storage_route FROM glowloom_app`).Error; err != nil {
		t.Fatal(err)
	}
	_, err = store.Options(t.Context(), actor)
	var denied *pgconn.PgError
	if !errors.As(err, &denied) || denied.Code != "42501" {
		t.Fatal("unexpected database error was disguised as missing storage", err)
	}
}
