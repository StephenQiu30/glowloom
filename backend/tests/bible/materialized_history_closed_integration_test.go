package bible_test

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/lanverse/backend/internal/bible/adapter/postgres"
	app "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	platformdb "github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	scriptpg "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/postgres"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
)

func bibleClosedHistoryDB(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("LV_TEST_BIBLE_CLOSED_DB_DSN")
	if dsn == "" {
		t.Skip("set isolated LV_TEST_BIBLE_CLOSED_DB_DSN")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Path != "/lanverse_reference" {
		t.Fatal("closed history requires isolated lanverse_reference database", err)
	}
	query := parsed.Query()
	query.Set("options", "-c role=lanverse_app")
	parsed.RawQuery = query.Encode()
	open := func(dsn string) *gorm.DB {
		client, err := platformdb.Open(t.Context(), dsn, noop.NewTracerProvider())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := client.Close(); err != nil {
				t.Error(err)
			}
		})
		return client.DB
	}
	runtime, owner := open(parsed.String()), open(dsn)
	var role string
	if err := runtime.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "lanverse_app" {
		t.Fatal("closed history requires actual nonowner", role, err)
	}
	return runtime, owner
}

func bibleClosedService(db *gorm.DB, objects *objectstorage.Client) *app.Service {
	return app.NewService(pg.NewStore(db, pg.Factories{Access: func(tx *gorm.DB) app.ProjectAccess {
		return workspacepg.NewProjectContentAccessStore(tx)
	}, Media: func(tx *gorm.DB) app.MediaReferences {
		return actualBibleMedia{mediaapp.NewReferenceFactQuery(mediapg.NewLibraryStore(tx, bibleLibraryFactory, time.Now), objects)}
	}, Scopes: func(tx *gorm.DB) app.ScriptScopes {
		return currentBibleScopes{scriptpg.NewBibleScopes(tx, workspacepg.NewProjectContentAccessStore(tx))}
	}}), time.Now)
}

func bibleClosedCharacter(t *testing.T, db, owner *gorm.DB) (identityapp.Principal, domain.Version) {
	t.Helper()
	objects := bibleObjects(t)
	fixture := coordinatedContentFixture(t, db, owner, objects)
	service := bibleClosedService(db, objects)
	detail, err := service.Find(t.Context(), fixture.actor, fixture.project, domain.KindCharacter, fixture.character)
	if err != nil || detail.Current.Character.Voice == nil || len(detail.Current.Character.Looks[0].AppliesTo) != 1 {
		t.Fatal("actual materialized voice and formal scene fixture", err)
	}
	if _, err := service.Change(t.Context(), fixture.actor, app.Command{ProjectID: fixture.project, Kind: domain.KindCharacter, Action: "confirm", EntryID: fixture.character, ExpectedRevision: detail.Head.Revision, Key: uuid.New(), RequestID: uuid.New()}); err != nil {
		t.Fatal("actual latest version confirmation", err)
	}
	return fixture.actor, detail.Current
}

func bibleClosedMutation(t *testing.T, owner *gorm.DB, seed func(*gorm.DB), read func(*gorm.DB)) {
	t.Helper()
	tx := owner.WithContext(t.Context()).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Error(err)
		}
	}()
	seed(tx)
	if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
		t.Fatal(err)
	}
	var role string
	if err := tx.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "lanverse_app" {
		t.Fatal("historical reader lacks actual nonowner", role, err)
	}
	read(tx)
}

func bibleClosedReaders(t *testing.T, tx *gorm.DB, actor identityapp.Principal, version domain.Version, wantCorrupt bool) {
	t.Helper()
	store := bibleStore(tx)
	checks := []struct {
		name string
		read func() error
	}{
		{"version", func() error {
			_, err := store.Version(t.Context(), actor, version.ProjectID, version.Kind, version.EntryID, version.ID)
			return err
		}},
		{"detail", func() error {
			_, err := store.Find(t.Context(), actor, version.ProjectID, version.Kind, version.EntryID)
			return err
		}},
		{"list", func() error {
			_, err := store.List(t.Context(), actor, app.ListInput{ProjectID: version.ProjectID, Kind: version.Kind, Limit: 100})
			return err
		}},
		{"history", func() error {
			_, err := store.History(t.Context(), actor, version.ProjectID, version.Kind, version.EntryID, 100, nil)
			return err
		}},
		{"confirmed_reference", func() error {
			_, err := pg.NewReferences(tx, workspacepg.NewProjectContentAccessStore(tx)).Reference(t.Context(), actor, version.ProjectID, version.EntryID, &version.ID)
			return err
		}},
	}
	for _, check := range checks {
		err := check.read()
		if wantCorrupt && !errors.Is(err, domain.ErrCorruptHistory) || !wantCorrupt && err != nil {
			t.Errorf("%s materialized historical contract: corrupt=%t, err=%v", check.name, wantCorrupt, err)
		}
	}
}

func TestBibleMaterializedClosedPGVoiceUnknownFields(t *testing.T) {
	db, owner := bibleClosedHistoryDB(t)
	actor, version := bibleClosedCharacter(t, db, owner)
	var original string
	if err := owner.Raw(`SELECT content::text FROM bible.voice_version WHERE org_id=? AND project_id=? AND character_version_id=?`, actor.OrgID, version.ProjectID, version.ID).Scan(&original).Error; err != nil || original == "" {
		t.Fatal(err)
	}
	for _, path := range [][]string{nil, {"sample"}, {"sample", "media"}} {
		name := strings.Join(path, "/")
		if name == "" {
			name = "top_level"
		}
		t.Run(name, func(t *testing.T) {
			var raw map[string]any
			if err := json.Unmarshal([]byte(original), &raw); err != nil {
				t.Fatal(err)
			}
			target := raw
			for _, key := range path {
				target = target[key].(map[string]any)
			}
			target["unknown_media"] = map[string]any{"asset_id": uuid.NewString()}
			encoded, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			bibleClosedMutation(t, owner, func(tx *gorm.DB) {
				if err := tx.Exec(`UPDATE bible.voice_version SET content=?::jsonb WHERE org_id=? AND project_id=? AND character_version_id=?`, string(encoded), actor.OrgID, version.ProjectID, version.ID).Error; err != nil {
					t.Fatal(err)
				}
			}, func(tx *gorm.DB) { bibleClosedReaders(t, tx, actor, version, true) })
		})
	}
	var after string
	if err := owner.Raw(`SELECT content::text FROM bible.voice_version WHERE org_id=? AND project_id=? AND character_version_id=?`, actor.OrgID, version.ProjectID, version.ID).Scan(&after).Error; err != nil || after != original {
		t.Fatal("voice reader rewrote durable history", err)
	}
}

func TestBibleMaterializedClosedPGAppliesToUnknownScopeFields(t *testing.T) {
	db, owner := bibleClosedHistoryDB(t)
	actor, version := bibleClosedCharacter(t, db, owner)
	look := uuid.NewSHA1(version.ID, []byte("look/"+version.Character.Looks[0].ID.String()))
	var original string
	if err := owner.Raw(`SELECT applies_to::text FROM bible.look_version WHERE org_id=? AND project_id=? AND id=?`, actor.OrgID, version.ProjectID, look).Scan(&original).Error; err != nil {
		t.Fatal(err)
	}
	var raw []map[string]any
	if err := json.Unmarshal([]byte(original), &raw); err != nil || len(raw) != 1 {
		t.Fatal("actual formal typed scope", err)
	}
	raw[0]["unknown_media"] = map[string]any{"asset_id": uuid.NewString()}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	bibleClosedMutation(t, owner, func(tx *gorm.DB) {
		if err := tx.Exec(`UPDATE bible.look_version SET applies_to=?::jsonb WHERE org_id=? AND project_id=? AND id=?`, string(encoded), actor.OrgID, version.ProjectID, look).Error; err != nil {
			t.Fatal(err)
		}
	}, func(tx *gorm.DB) { bibleClosedReaders(t, tx, actor, version, true) })
	var after string
	if err := owner.Raw(`SELECT applies_to::text FROM bible.look_version WHERE org_id=? AND project_id=? AND id=?`, actor.OrgID, version.ProjectID, look).Scan(&after).Error; err != nil || after != original {
		t.Fatal("scope reader rewrote durable history", err)
	}
}

func TestBibleMaterializedClosedPGResultSourceAcceptedFieldsAndUnknown(t *testing.T) {
	db, owner := bibleClosedHistoryDB(t)
	actor, version := bibleClosedCharacter(t, db, owner)
	// These are accepted historical provenance facts, not proof that an AI
	// provider ran or that synthetic operation/output identifiers were executed.
	source := domain.ResultSource{OperationID: uuid.New(), OutputID: uuid.New(), OutputSHA256: strings.Repeat("a", 64), InputSHA256: strings.Repeat("b", 64)}
	known, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, unknown := range []bool{false, true} {
		name := "accepted_fields"
		if unknown {
			name = "unknown_nested_value"
		}
		t.Run(name, func(t *testing.T) {
			var raw map[string]any
			if err := json.Unmarshal(known, &raw); err != nil {
				t.Fatal(err)
			}
			if unknown {
				raw["unknown_media"] = map[string]any{"asset_id": uuid.NewString()}
			}
			encoded, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			bibleClosedMutation(t, owner, func(tx *gorm.DB) {
				if err := tx.Exec(`UPDATE bible.character_version SET origin='ai',result_source=?::jsonb WHERE org_id=? AND project_id=? AND id=?`, string(encoded), actor.OrgID, version.ProjectID, version.ID).Error; err != nil {
					t.Fatal(err)
				}
			}, func(tx *gorm.DB) {
				bibleClosedReaders(t, tx, actor, version, unknown)
				if !unknown {
					loaded, err := bibleStore(tx).Version(t.Context(), actor, version.ProjectID, version.Kind, version.EntryID, version.ID)
					if err != nil || loaded.Result == nil || *loaded.Result != source || loaded.ContentSHA256 != version.ContentSHA256 {
						t.Fatal("accepted provenance fields or immutable body digest lost", err)
					}
				}
			})
		})
	}
}

func TestBibleMaterializedClosedPGVoicePointerNullBoundaries(t *testing.T) {
	db, owner := bibleClosedHistoryDB(t)
	actor, version := bibleClosedCharacter(t, db, owner)
	for _, active := range []bool{false, true} {
		name := "inactive_catalog_null"
		field := "catalog"
		if active {
			name, field = "active_sample_null", "sample"
		}
		t.Run(name, func(t *testing.T) {
			bibleClosedMutation(t, owner, func(tx *gorm.DB) {
				if err := tx.Exec(`UPDATE bible.voice_version SET content=jsonb_set(content,?, 'null'::jsonb) WHERE org_id=? AND project_id=? AND character_version_id=?`, "{"+field+"}", actor.OrgID, version.ProjectID, version.ID).Error; err != nil {
					t.Fatal(err)
				}
			}, func(tx *gorm.DB) { bibleClosedReaders(t, tx, actor, version, active) })
		})
	}
}

func TestBibleMaterializedClosedPGEmptyScopeRepresentationsPreserveHistory(t *testing.T) {
	db, owner := bibleClosedHistoryDB(t)
	actor, project := bibleActorProject(t, owner)
	service := app.NewService(bibleStore(db), time.Now)
	created, err := service.Change(t.Context(), actor, createCharacter(project))
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Change(t.Context(), actor, app.Command{ProjectID: project, Kind: domain.KindCharacter, Action: "look_create", EntryID: created.EntryID, ExpectedRevision: created.Revision, Look: &app.LookInput{Name: "空范围造型", AppliesTo: []domain.LookScope{}}, Key: uuid.New(), RequestID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := service.Find(t.Context(), actor, project, domain.KindCharacter, created.EntryID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Change(t.Context(), actor, app.Command{ProjectID: project, Kind: domain.KindCharacter, Action: "confirm", EntryID: created.EntryID, ExpectedRevision: detail.Head.Revision, Key: uuid.New(), RequestID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	var before struct{ Content, SHA, Scopes string }
	if err := owner.Raw(`SELECT c.content::text,c.content_sha256 AS sha,l.applies_to::text AS scopes FROM bible.character_version c JOIN bible.look_version l ON l.character_version_id=c.id WHERE c.id=? AND l.position=1`, detail.Current.ID).Scan(&before).Error; err != nil || before.Scopes != "[]" {
		t.Fatal("actual explicit empty scope history", before.Scopes, err)
	}
	for _, representation := range []string{"[]", "null", "sql_null"} {
		t.Run(representation, func(t *testing.T) {
			bibleClosedMutation(t, owner, func(tx *gorm.DB) {
				query := `UPDATE bible.look_version SET applies_to=?::jsonb WHERE org_id=? AND project_id=? AND character_version_id=? AND position=1`
				var value any = representation
				if representation == "sql_null" {
					value = nil
				}
				if err := tx.Exec(query, value, actor.OrgID, project, detail.Current.ID).Error; err != nil {
					t.Fatal(err)
				}
			}, func(tx *gorm.DB) { bibleClosedReaders(t, tx, actor, detail.Current, false) })
		})
	}
	var after struct{ Content, SHA, Scopes string }
	if err := owner.Raw(`SELECT c.content::text,c.content_sha256 AS sha,l.applies_to::text AS scopes FROM bible.character_version c JOIN bible.look_version l ON l.character_version_id=c.id WHERE c.id=? AND l.position=1`, detail.Current.ID).Scan(&after).Error; err != nil || after != before {
		t.Fatal("strict reader changed canonical body, SHA or explicit empty history", err)
	}
}
