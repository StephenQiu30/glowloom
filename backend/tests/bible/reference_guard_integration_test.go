package bible_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
	scriptextract "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/extract"
	scriptobjects "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/objects"
	scriptpg "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/postgres"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	scriptdomain "github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
)

func guardedBibleReference(ctx context.Context, db *gorm.DB, actor identityapp.Principal, project, asset uuid.UUID) (bool, error) {
	var found bool
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		found, err = pg.NewMediaReferenceGuard(tx, workspacepg.NewProjectContentAccessStore(tx)).HasMediaReferences(ctx, actor, project, asset)
		return err
	})
	return found, err
}
func guardedScriptReference(ctx context.Context, db *gorm.DB, actor identityapp.Principal, project, asset uuid.UUID) (bool, error) {
	var found bool
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		found, err = scriptpg.NewMediaReferenceGuard(tx, workspacepg.NewProjectContentAccessStore(tx)).HasMediaReferences(ctx, actor, project, asset)
		return err
	})
	return found, err
}

func TestBibleMediaReferencePGProtectsOldLooksAndRemovedSampleBinding(t *testing.T) {
	db, owner := bibleReferenceDB(t)
	objects := bibleObjects(t)
	c := coordinatedContentFixture(t, db, owner, objects)
	service := app.NewService(pg.NewStore(db, pg.Factories{Access: func(tx *gorm.DB) app.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) }, Media: func(tx *gorm.DB) app.MediaReferences {
		return actualBibleMedia{mediaapp.NewReferenceFactQuery(mediapg.NewLibraryStore(tx, bibleLibraryFactory, time.Now), objects)}
	}, Scopes: func(tx *gorm.DB) app.ScriptScopes {
		return currentBibleScopes{scriptpg.NewBibleScopes(tx, workspacepg.NewProjectContentAccessStore(tx))}
	}}), time.Now)
	detail, err := service.Find(t.Context(), c.actor, c.project, domain.KindCharacter, c.character)
	if err != nil {
		t.Fatal(err)
	}
	lookID := detail.Current.Character.Looks[0].ID
	removed, err := service.Change(t.Context(), c.actor, app.Command{ProjectID: c.project, Kind: domain.KindCharacter, Action: "references", EntryID: c.character, ExpectedRevision: detail.Head.Revision, LookID: &lookID, References: []app.ReferenceInput{}, Key: uuid.New(), RequestID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Change(t.Context(), c.actor, app.Command{ProjectID: c.project, Kind: domain.KindCharacter, Action: "voice_unbind", EntryID: c.character, ExpectedRevision: removed.Revision, Key: uuid.New(), RequestID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	for _, asset := range []uuid.UUID{c.image, c.audio} {
		if found, err := guardedBibleReference(t.Context(), db, c.actor, c.project, asset); err != nil || !found {
			t.Fatal("old immutable media reference lost", asset, found, err)
		}
	}
	if found, err := guardedBibleReference(t.Context(), db, c.actor, c.project, uuid.New()); err != nil || found {
		t.Fatal("unreferenced media fabricated", found, err)
	}
}

func guardedImportStore(db *gorm.DB, objects *objectstorage.Client) *scriptpg.ImportStore {
	return scriptpg.NewImportStore(db, func(tx *gorm.DB) scriptapp.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) }, func(tx *gorm.DB) scriptapp.SourceAssetReader {
		return mediaapp.NewDocumentSources(mediapg.NewDocumentSourceStore(tx), objects)
	})
}

func TestScriptMediaReferencePGProtectsQueuedDocumentBeforeSourcePublication(t *testing.T) {
	db, owner := bibleReferenceDB(t)
	objects := bibleObjects(t)
	actor, project := bibleActorProject(t, owner)
	document := bibleUploadedMedia(t, db, objects, actor, project, "待导入.txt", []byte("第一章\n王总：你好😀"))
	service := scriptapp.NewImportService(guardedImportStore(db, objects), time.Now)
	accepted, err := service.Create(t.Context(), actor, scriptapp.ImportCommand{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{document}, RightsConfirmed: true})
	if err != nil || accepted.Status != "queued" {
		t.Fatal("actual durable admission", accepted, err)
	}
	var count int64
	if err := owner.Table("script.script_source").Where("project_id=?", project).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("acceptance silently published", count, err)
	}
	if found, err := guardedScriptReference(t.Context(), db, actor, project, document); err != nil || !found {
		t.Fatal("recoverable source document can be purged", found, err)
	}
}

func guardedImportWorker(db *gorm.DB, objects *objectstorage.Client) *scriptapp.ImportWorker {
	private := scriptobjects.NewStorage(objects)
	access := func(tx *gorm.DB) scriptapp.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) }
	sources := scriptapp.NewSourceService(scriptpg.NewSourceStore(db, access), private, time.Now)
	return scriptapp.NewImportWorker(guardedImportStore(db, objects), mediaapp.NewDocumentSources(mediapg.NewDocumentSourceStore(db), objects), scriptextract.NewExtractor(), func(a scriptapp.ImportAuthority) *scriptapp.SourceService {
		return scriptapp.NewSourceServiceSharingIO(scriptpg.NewImportSourceStore(db, access, a), private, time.Now, sources)
	}, scriptapp.NewSourceRecovery(scriptpg.NewImportRecoveryStore(db, access), sources, time.Now), time.Now)
}

func guardedImportDelivery(t *testing.T, owner *gorm.DB, actor identityapp.Principal, key uuid.UUID) scriptapp.ImportDelivery {
	t.Helper()
	var raw string
	read := owner.Raw(`SELECT o.payload::text FROM infra.outbox o JOIN script.import_command c ON c.event_id=o.id WHERE c.actor_id=? AND c.request_id=?`, actor.ID, key).Scan(&raw)
	if read.Error != nil || read.RowsAffected != 1 {
		t.Fatal("permanent import event", read.Error)
	}
	var envelope struct {
		Data scriptapp.ImportDelivery `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Data
}

func TestScriptMediaReferencePGCancelledUnpublishedAndDeletedPublishedSources(t *testing.T) {
	db, owner := bibleReferenceDB(t)
	objects := bibleObjects(t)
	actor, project := bibleActorProject(t, owner)
	document := bibleUploadedMedia(t, db, objects, actor, project, "原件历史.txt", []byte("第一章\n甲😀乙"))
	service := scriptapp.NewImportService(guardedImportStore(db, objects), time.Now)
	create := scriptapp.ImportCommand{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{document}, RightsConfirmed: true}
	queued, err := service.Create(t.Context(), actor, create)
	if err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	if _, err := service.Control(t.Context(), actor, scriptapp.ImportControl{ProjectID: project, JobID: queued.ID, Action: "cancel", ExpectedRevision: queued.Revision, Key: key, RequestID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	worker := guardedImportWorker(db, objects)
	if err := worker.Control(t.Context(), guardedImportDelivery(t, owner, actor, key)); err != nil {
		t.Fatal(err)
	}
	if found, err := guardedScriptReference(t.Context(), db, actor, project, document); err != nil || found {
		t.Fatal("stopped cancelled import invented a permanent source reference", found, err)
	}
	create.Key, create.RequestID = uuid.New(), uuid.New()
	published, err := service.Create(t.Context(), actor, create)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Execute(t.Context(), guardedImportDelivery(t, owner, actor, create.Key)); err != nil {
		t.Fatal(err)
	}
	published, err = service.Get(t.Context(), actor, project, published.ID)
	if err != nil || published.Status != "succeeded" || published.LatestVersionID == nil || published.Files[0].LineageID == nil {
		t.Fatal("formal document publication", published, err)
	}
	sources := scriptapp.NewSourceService(scriptpg.NewSourceStore(db, func(tx *gorm.DB) scriptapp.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) }), scriptobjects.NewStorage(objects), time.Now)
	if _, err := sources.Write(t.Context(), actor, scriptapp.SourceCommand{ProjectID: project, ExpectedRevision: published.LatestScriptRevision, BaseVersionID: published.LatestVersionID, LineageID: published.Files[0].LineageID, Action: "delete", RightsConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	if found, err := guardedScriptReference(t.Context(), db, actor, project, document); err != nil || !found {
		t.Fatal("deleted current chapter lost retained original-document history", found, err)
	}
}

func TestBibleScriptMediaReferencePGCurrentScopeAndCallerTransaction(t *testing.T) {
	db, owner := bibleReferenceDB(t)
	actor, project := bibleActorProject(t, owner)
	_, foreignProject := bibleActorProject(t, owner)
	queries := []struct {
		name  string
		query func(context.Context, *gorm.DB, identityapp.Principal, uuid.UUID, uuid.UUID) (bool, error)
		pool  func(context.Context) (bool, error)
	}{{"bible", guardedBibleReference, func(ctx context.Context) (bool, error) {
		return pg.NewMediaReferenceGuard(db, workspacepg.NewProjectContentAccessStore(db)).HasMediaReferences(ctx, actor, project, uuid.New())
	}}, {"script", guardedScriptReference, func(ctx context.Context) (bool, error) {
		return scriptpg.NewMediaReferenceGuard(db, workspacepg.NewProjectContentAccessStore(db)).HasMediaReferences(ctx, actor, project, uuid.New())
	}}}
	for _, query := range queries {
		t.Run(query.name, func(t *testing.T) {
			if found, err := query.query(t.Context(), db, actor, project, uuid.New()); err != nil || found {
				t.Fatal("empty authorized history", found, err)
			}
			if _, err := query.pool(t.Context()); err == nil {
				t.Fatal("guard released owning caller transaction")
			}
			if _, err := query.query(t.Context(), db, actor, foreignProject, uuid.New()); err == nil {
				t.Fatal("foreign history reported unreferenced")
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if _, err := query.query(ctx, db, actor, project, uuid.New()); err == nil {
				t.Fatal("cancelled authority read reported zero references")
			}
		})
	}
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	for _, query := range queries {
		if _, err := query.query(t.Context(), db, actor, project, uuid.New()); err == nil {
			t.Fatal("disabled actor read historical media", query.name)
		}
	}
}

func TestBibleMediaReferencePGCorruptSampleCannotBecomeNoReference(t *testing.T) {
	db, owner := bibleReferenceDB(t)
	objects := bibleObjects(t)
	c := coordinatedContentFixture(t, db, owner, objects)
	var original string
	if err := owner.Raw(`SELECT content::text FROM bible.voice_version WHERE project_id=? AND character_version_id=?`, c.project, c.pinnedVersion).Scan(&original).Error; err != nil || original == "" {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE bible.voice_version SET content='{}'::jsonb WHERE project_id=? AND character_version_id=?`, c.project, c.pinnedVersion).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Exec(`UPDATE bible.voice_version SET content=?::jsonb WHERE project_id=? AND character_version_id=?`, original, c.project, c.pinnedVersion).Error; err != nil {
			t.Error(err)
		}
	})
	if found, err := guardedBibleReference(t.Context(), db, c.actor, c.project, uuid.New()); err == nil || found {
		t.Fatal("corrupt sample was accepted as no downstream reference", found, err)
	}
}

type failedGuardWriteObjects struct{ *scriptobjects.Storage }

func (o failedGuardWriteObjects) PutIfAbsent(context.Context, string, io.Reader, int64, string, string) error {
	return errors.New("synthetic original object request failed before publication")
}

func TestScriptMediaReferencePGCorruptRecoverableWriteCannotBecomeNoReference(t *testing.T) {
	db, owner := bibleReferenceDB(t)
	objects := bibleObjects(t)
	actor, project := bibleActorProject(t, owner)
	key := uuid.New()
	source := scriptapp.NewSourceService(scriptpg.NewSourceStore(db, func(tx *gorm.DB) scriptapp.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) }), failedGuardWriteObjects{scriptobjects.NewStorage(objects)}, time.Now)
	_, err := source.Write(t.Context(), actor, scriptapp.SourceCommand{ProjectID: project, Action: "create", RightsConfirmed: true, Key: key, RequestID: uuid.New(), Sources: []scriptapp.SourceInput{{Kind: "chapter", Title: "仍待核验的原稿", Status: "draft", Document: scriptdomain.RichDocument{Type: "doc", Content: []scriptdomain.RichDocument{{Type: "paragraph", Content: []scriptdomain.RichDocument{{Type: "text", Text: "保留正文"}}}}}}}})
	if err == nil {
		t.Fatal("synthetic object failure was swallowed")
	}
	if found, err := guardedScriptReference(t.Context(), db, actor, project, uuid.New()); err != nil || found {
		t.Fatal("valid pending manual body invented media pin", found, err)
	}
	var original string
	if err := owner.Raw(`SELECT plan::text FROM script.command WHERE actor_id=? AND request_id=?`, actor.ID, key).Scan(&original).Error; err != nil || original == "" {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE script.command SET plan='{}'::jsonb WHERE actor_id=? AND request_id=?`, actor.ID, key).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Exec(`UPDATE script.command SET plan=?::jsonb WHERE actor_id=? AND request_id=?`, original, actor.ID, key).Error; err != nil {
			t.Error(err)
		}
	})
	if found, err := guardedScriptReference(t.Context(), db, actor, project, uuid.New()); err == nil || found {
		t.Fatal("unreadable pending source plan reported zero", found, err)
	}
}

// Reference admission checks may share the migrated reference test database,
// using independent UUID fixtures and an actual nonowner connection.
func bibleReferenceDB(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("LV_TEST_REFERENCE_DB_DSN")
	if dsn == "" {
		return bibleTestDB(t)
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Path != "/lanverse_reference" {
		t.Fatal("reference guard requires isolated lanverse_reference database", err)
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
		t.Fatal("reference guard requires actual nonowner", role, err)
	}
	return runtime, owner
}

func bibleReferenceHistory(t *testing.T, tx *gorm.DB, base domain.Version, count, looks, padding int) {
	t.Helper()
	content := *base.Character
	content.Looks = make([]domain.LookContent, looks)
	for i := range content.Looks {
		content.Looks[i] = domain.LookContent{ID: uuid.New(), Name: "历史造型", Description: strings.Repeat("界", padding), Default: i == 0}
		if err := tx.Exec(`INSERT INTO bible.look(id,org_id,project_id,character_id,created_at) VALUES(?,?,?,?,?)`, content.Looks[i].ID, base.OrgID, base.ProjectID, base.EntryID, base.CreatedAt).Error; err != nil {
			t.Fatal(err)
		}
	}
	encoded, digest, err := domain.EncodeCharacter(content)
	if err != nil {
		t.Fatal("synthetic typed immutable content", err)
	}
	type versionFact struct {
		ID     uuid.UUID `json:"id"`
		Number int       `json:"number"`
	}
	type lookFact struct {
		ID        uuid.UUID `json:"id"`
		LookID    uuid.UUID `json:"look_id"`
		VersionID uuid.UUID `json:"version_id"`
		Position  int       `json:"position"`
		Default   bool      `json:"is_default"`
	}
	versions := make([]versionFact, 0, count)
	rows := make([]lookFact, 0, count*looks)
	for i := range count {
		version := uuid.New()
		versions = append(versions, versionFact{version, int(base.Number) + i + 1})
		for position, look := range content.Looks {
			rows = append(rows, lookFact{uuid.NewSHA1(version, []byte("look/"+look.ID.String())), look.ID, version, position, look.Default})
		}
	}
	versionJSON, err := json.Marshal(versions)
	if err != nil {
		t.Fatal(err)
	}
	lookJSON, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	read := tx.Exec(`INSERT INTO bible.character_version(id,org_id,project_id,entry_id,version_no,previous_id,actor_id,created_at,origin,content,content_sha256)
 SELECT id,?,?,?,number,?,?,?,'manual',?::jsonb,? FROM jsonb_to_recordset(?::jsonb) AS r(id uuid,number bigint)`, base.OrgID, base.ProjectID, base.EntryID, base.ID, base.ActorID, base.CreatedAt, string(encoded), digest, string(versionJSON))
	if read.Error != nil || read.RowsAffected != int64(count) {
		t.Fatal("owned immutable version history", read.RowsAffected, read.Error)
	}
	read = tx.Exec(`INSERT INTO bible.look_version(id,org_id,project_id,look_id,character_version_id,position,name,description,is_default,applies_to)
 SELECT id,?,?,look_id,version_id,position,?,?,is_default,'null'::jsonb FROM jsonb_to_recordset(?::jsonb) AS r(id uuid,look_id uuid,version_id uuid,position integer,is_default boolean)`, base.OrgID, base.ProjectID, "历史造型", strings.Repeat("界", padding), string(lookJSON))
	if read.Error != nil || read.RowsAffected != int64(count*looks) {
		t.Fatal("owned immutable appearance history", read.RowsAffected, read.Error)
	}
}

func TestBibleMediaReferencePGBoundedCompleteHistory(t *testing.T) {
	db, owner := bibleReferenceDB(t)
	for _, test := range []struct {
		name                  string
		count, looks, padding int
	}{
		{"single_body", 1, 60, 8192},
		{"aggregate_bytes", 70, 20, 8192},
		{"fact_count", 25000, 1, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			actor, project := bibleActorProject(t, owner)
			service := app.NewService(bibleStore(db), time.Now)
			created, err := service.Change(t.Context(), actor, createCharacter(project))
			if err != nil {
				t.Fatal(err)
			}
			detail, err := service.Find(t.Context(), actor, project, domain.KindCharacter, created.EntryID)
			if err != nil {
				t.Fatal(err)
			}
			asset := uuid.New()
			if found, err := guardedBibleReference(t.Context(), db, actor, project, asset); err != nil || found {
				t.Fatal("valid small immutable history", found, err)
			}
			tx := owner.WithContext(t.Context()).Begin()
			if tx.Error != nil {
				t.Fatal(tx.Error)
			}
			defer func() {
				if err := tx.Rollback().Error; err != nil {
					t.Error(err)
				}
			}()
			bibleReferenceHistory(t, tx, detail.Current, test.count, test.looks, test.padding)
			if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
				t.Fatal(err)
			}
			var role string
			if err := tx.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "lanverse_app" {
				t.Fatal("history guard lacks actual nonowner", role, err)
			}
			found, err := pg.NewMediaReferenceGuard(tx, workspacepg.NewProjectContentAccessStore(tx)).HasMediaReferences(t.Context(), actor, project, asset)
			if !errors.Is(err, app.ErrUnavailable) || found {
				t.Fatal("oversized complete history was not rejected before materialization", found, err)
			}
		})
	}
}
