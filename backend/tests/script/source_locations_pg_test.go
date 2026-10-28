package script_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

func TestScriptLocatedPGAdmissionHistoryAndReplaySurviveDefaultSwitch(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_SCRIPT_LOCATION_DSN")
	if dsn == "" {
		t.Skip("set a task-owned glowloom_subject_storage_ loopback fixture")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "glowloom_subject_storage_") {
		t.Fatal("script location fixture requires an isolated task-owned loopback database")
	}
	owner, err := platformdb.Open(t.Context(), dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	var initialized bool
	if err := owner.DB.Raw(`SELECT to_regclass('script.script_source') IS NOT NULL`).Scan(&initialized).Error; err != nil {
		t.Fatal(err)
	}
	if !initialized {
		raw, err := os.ReadFile(filepath.Join("..", "..", "db", "schema.sql"))
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.DB.Exec(string(raw)).Error; err != nil {
			t.Fatal(err)
		}
		routes := pgmedia.NewStorageRoutes(owner.DB)
		sha := strings.Repeat("a", 64)
		if _, err := routes.ProveEmptyStorageDeployment(t.Context(), sha); err != nil {
			t.Fatal(err)
		}
		if err := routes.FinalizeStorageSchema(t.Context(), sha); err != nil {
			t.Fatal(err)
		}
	}
	routes := pgmedia.NewStorageRoutes(owner.DB)
	profiles := []media.StorageProfile{
		{ID: "script-fixture-a", Provider: "tos", Endpoint: "https://script-a.example.invalid", SigningEndpoint: "https://script-a.example.invalid", Region: "test-region", Bucket: "script-a"},
		{ID: "script-fixture-b", Provider: "tos", Endpoint: "https://script-b.example.invalid", SigningEndpoint: "https://script-b.example.invalid", Region: "test-region", Bucket: "script-b"},
	}
	if err := routes.RegisterProfiles(t.Context(), profiles); err != nil {
		t.Fatal(err)
	}
	prior, _ := routes.ReadActive(t.Context())
	firstRevision := "fixture-" + uuid.NewString()
	// Explicit low-level route fixture: no cloud/deployment acceptance is claimed.
	if _, err := routes.Activate(t.Context(), prior.Revision, firstRevision, profiles[0].ID); err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("options", "-c role=glowloom_app")
	u.RawQuery = q.Encode()
	runtime, err := platformdb.Open(t.Context(), u.String(), noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	actor, project := scriptActorProject(t, owner.DB)
	store := scriptStore(runtime.DB).WithObjectLocations(scriptwire.ProvideScriptObjectLocations)
	objects := &locatedSourceObjects{data: map[uuid.UUID][]byte{}}
	service := app.NewSourceService(store, objects, time.Now)
	_, input := sourceCommand()
	input.ProjectID = project
	receipt, err := service.Write(t.Context(), actor, input)
	if err != nil {
		t.Fatal("located script write", err)
	}
	base, err := store.LoadBase(t.Context(), actor, project, nil)
	if err != nil || len(base.Sources) != 1 || base.Sources[0].Rich.LocationID == uuid.Nil {
		t.Fatal("history did not retain physical identity", err)
	}
	if _, err := routes.Activate(t.Context(), firstRevision, "fixture-"+uuid.NewString(), profiles[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Source(t.Context(), actor, project, base.Sources[0].LineageID, nil); err != nil {
		t.Fatal("historical read followed new default", err)
	}
	originalKey := input.Key
	replayed, err := service.Write(t.Context(), actor, input)
	if err != nil || !reflect.DeepEqual(replayed, receipt) || len(objects.data) != 4 || objects.bare != 0 {
		t.Fatal("old command repinned or rewrote its objects", err)
	}
	input.Key, input.RequestID = uuid.New(), uuid.New()
	input.ExpectedRevision = 1
	input.BaseVersionID = &receipt.VersionID
	input.Sources[0].Title = "新默认目标"
	if _, err := service.Write(t.Context(), actor, input); err != nil {
		t.Fatal("new admission did not use the new fixed target", err)
	}
	var newCount int
	if err := owner.DB.Raw(`SELECT count(*) FROM script.object_intent i JOIN media.object_location l ON l.id=i.object_location_id WHERE i.actor_id=? AND i.request_id=? AND l.profile_id=? AND l.state='accepted'`, actor.ID, input.Key, profiles[1].ID).Scan(&newCount).Error; err != nil || newCount != 4 {
		t.Fatal("new output set did not freeze changed route", newCount, err)
	}
	var accepted int
	if err := owner.DB.Raw(`SELECT count(*) FROM script.object_intent i JOIN media.object_location l ON l.id=i.object_location_id AND l.object_key=i.object_key JOIN media.storage_work_intent w ON w.id=l.owner_intent_id WHERE i.actor_id=? AND i.request_id=? AND l.profile_id=? AND l.state='accepted' AND w.state='accepted'`, actor.ID, originalKey, profiles[0].ID).Scan(&accepted).Error; err != nil || accepted != 4 {
		t.Fatal("publication did not atomically accept the fixed set", accepted, err)
	}
	// A cancellation waiting for SDK drain must survive the owner's SQL callback.
	port := scriptwire.ProvideScriptObjectLocations(runtime.DB)
	intent, facts, err := port.Admit(t.Context(), project, uuid.New(), []domain.ObjectFact{{Key: "projects/" + project.String() + "/script/cancel-clock", SHA256: strings.Repeat("b", 64), ByteSize: 1, MIME: "text/plain"}})
	if err != nil {
		t.Fatal(err)
	}
	bounded, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	if err := port.Begin(bounded, intent, facts[0]); err != nil {
		t.Fatal(err)
	}
	if err := runtime.DB.Transaction(func(tx *gorm.DB) error {
		return scriptwire.ProvideScriptObjectLocations(tx).BeginRemoval(t.Context(), intent, facts[0])
	}); err != nil {
		t.Fatal("cancellation debt callback rolled back its clock", err)
	}
	var cancelled bool
	if err := owner.DB.Raw(`SELECT state='cancelled' AND closed_at IS NOT NULL FROM media.storage_work_intent WHERE id=?`, intent).Scan(&cancelled).Error; err != nil || !cancelled {
		t.Fatal("cancellation clock not durable", err)
	}
	if _, err := pgmedia.NewStorageRoutes(runtime.DB).ReadRemovableWorkLocation(t.Context(), facts[0].LocationID); !errors.Is(err, media.ErrStorageRouteConflict) {
		t.Fatal("undrained write was granted SDK removal", err)
	}
}
