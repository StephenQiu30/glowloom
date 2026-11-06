package subject_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
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

type voiceReadinessAccess struct{ owner subject.VoiceOwner }

func (a voiceReadinessAccess) Request(context.Context, identity.Principal, bool) (subject.VoiceOwner, error) {
	return a.owner, nil
}
func (a voiceReadinessAccess) Worker(context.Context, subject.VoiceWork) (subject.VoiceOwner, error) {
	return a.owner, nil
}

type voiceReadinessStorage struct{ routes *pgmedia.StorageRoutes }

func (s voiceReadinessStorage) Ready(ctx context.Context) (bool, error) {
	return s.routes.ReadyForWork(ctx)
}
func (voiceReadinessStorage) Pin(context.Context, uuid.UUID, uuid.UUID) (subject.VoiceStorageIntent, error) {
	return subject.VoiceStorageIntent{}, errors.New("must not pin an unready route")
}
func (voiceReadinessStorage) Read(context.Context, uuid.UUID, uuid.UUID) (subject.VoiceStorageIntent, error) {
	return subject.VoiceStorageIntent{}, media.ErrUnavailable
}

// Use a separate empty database so route and ACL changes stay local to this test.
func TestVoiceOptionsRequireExplicitStorageRoute(t *testing.T) {
	_, admin := subjectPostgres(t)
	dsn, err := url.Parse(os.Getenv("GLOWLOOM_TEST_SUBJECT_DB_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	name := "glowloom_subject_alignment_voice_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec(`CREATE DATABASE ` + name).Error; err != nil {
		t.Fatal("create own voice readiness fixture database", err)
	}
	t.Cleanup(func() {
		if err := admin.WithContext(context.Background()).Exec(`DROP DATABASE ` + name).Error; err != nil {
			t.Error("drop owned voice readiness fixture", err)
		}
	})
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
	schema, err := os.ReadFile(filepath.Join("..", "..", "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Transaction(func(tx *gorm.DB) error { return tx.Exec(string(schema)).Error }); err != nil {
		t.Fatal("initialize voice readiness fixture", err)
	}
	actor := identity.Principal{ID: uuid.New(), ScopeID: uuid.New(), OrgID: uuid.New()}
	authority := voiceReadinessAccess{subject.VoiceOwner{ScopeID: actor.ScopeID, PhysicalScopeID: actor.OrgID, PersonalScopeID: uuid.New(), PersonalOrgID: uuid.New()}}
	release := subject.VoiceRelease{VoiceModelOption: subject.VoiceModelOption{ReleaseID: uuid.New(), Voices: []subject.VoiceOption{{Key: "fixture-speaker", Name: "fixture"}}}}
	store := pgsubject.NewVoiceStore(appDB, pgsubject.VoiceFactories{Access: func(*gorm.DB) subject.VoiceAccess { return authority }, Catalog: func(*gorm.DB) subject.VoiceCatalog { return voiceCatalogFixture{release} }, Storage: func(tx *gorm.DB) subject.VoiceStorage { return voiceReadinessStorage{pgmedia.NewStorageRoutes(tx)} }})
	options, err := store.Options(t.Context(), actor)
	if err != nil || options.Available || options.Reason != "voice_storage_unavailable" || len(options.Models) != 0 {
		t.Fatal("empty schema looked executable without an explicit storage route", options, err)
	}
	if err := owner.Exec(`INSERT INTO media.storage_profile(id,provider,endpoint,signing_endpoint,region,bucket,addressing_style,resource_identity) VALUES('voice-route-fixture','cos','http://127.0.0.1:1','http://127.0.0.1:1','fixture','fixture','',?)`, strings.Repeat("a", 64)).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`INSERT INTO media.storage_route(singleton,revision,profile_id) VALUES(1,'1','voice-route-fixture')`).Error; err != nil {
		t.Fatal(err)
	}
	options, err = store.Options(t.Context(), actor)
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
