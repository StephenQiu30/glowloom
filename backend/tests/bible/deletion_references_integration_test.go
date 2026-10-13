package bible_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"reflect"
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
	platformdb "github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
)

func deletionDatabase(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("LV_TEST_SUBJECT_RECOVERY_DB_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/lanverse_subject_validation_") {
		t.Fatal("requires task-owned subject validation database")
	}
	open := func(d string) *gorm.DB {
		c, e := platformdb.Open(t.Context(), d, noop.NewTracerProvider())
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c.DB
	}
	owner := open(dsn)
	q := parsed.Query()
	q.Set("options", "-c role=lanverse_app")
	parsed.RawQuery = q.Encode()
	return open(parsed.String()), owner
}

func seedDialogue(t *testing.T, db *gorm.DB, actor identityapp.Principal, project, character, version uuid.UUID) uuid.UUID {
	t.Helper()
	script, split, episode, structure, scene, line := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	hash := strings.Repeat("a", 64)
	err := db.Transaction(func(tx *gorm.DB) error {
		queries := []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO script.script_version(id,org_id,project_id,version_no,source_ids,source_spans,text_key,text_sha256,text_bytes,rich_key,rich_sha256,rich_bytes,content_hash,document_sha256,source_manifest_sha256,char_count,created_at) VALUES(?,?,?,1,'{}','[]',?, ?,1,?, ?,1,?,?,?,100,now())`, []any{script, actor.OrgID, project, script.String() + "/text", hash, script.String() + "/rich", hash, hash, hash, hash}},
			{`INSERT INTO script.split_set(id,org_id,project_id,version_id,kind,origin,boundaries,warnings,created_at) VALUES(?,?,?,?,'formal','manual','[]','[]',now())`, []any{split, actor.OrgID, project, script}},
			{`INSERT INTO script.version_head(org_id,project_id,version_id,candidate_split_set_id,confirmed_split_set_id) VALUES(?,?,?,?,?)`, []any{actor.OrgID, project, script, split, split}},
			{`INSERT INTO script.project_state(project_id,org_id,revision,draft_version_id,updated_at) VALUES(?,?,1,?,now())`, []any{project, actor.OrgID, script}},
			{`INSERT INTO script.episode(id,org_id,project_id,script_version_id,split_set_id,seq_no,title,span_start,span_end,revision) VALUES(?,?,?,?,?,1,'test',0,100,1)`, []any{episode, actor.OrgID, project, script, split}},
			{`INSERT INTO script.episode_structure(id,org_id,project_id,episode_id,version_no,source_hash,document,actor_id,created_at) VALUES(?,?,?,?,1,?,'{}',?,now())`, []any{structure, actor.OrgID, project, episode, hash, actor.ID}},
			{`UPDATE script.episode SET current_structure_id=?,confirmed_structure_id=? WHERE id=?`, []any{structure, structure, episode}},
			{`INSERT INTO script.scene(id,org_id,project_id,episode_structure_id,scene_key,seq_no,heading,location_text,time_of_day,span_start,span_end) VALUES(?,?,?,?,?,1,'test','','',0,100)`, []any{scene, actor.OrgID, project, structure, uuid.New()}},
			{`INSERT INTO script.dialogue_line(id,org_id,project_id,scene_id,line_key,seq_no,kind,content,speaker_text,character_id,character_version_id,emotion,content_hash,span_start,span_end) VALUES(?,?,?,?,?,1,'dialogue','test','speaker',?,?,'',?,0,4)`, []any{line, actor.OrgID, project, scene, uuid.New(), character, version, hash}},
		}
		for _, q := range queries {
			if err := tx.Exec(q.sql, q.args...).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return episode
}

func TestSubjectDeletionRechecksRealReferencesAndRestoresIdentity(t *testing.T) {
	db, owner := deletionDatabase(t)
	actor, project := bibleActorProject(t, owner)
	service := app.NewService(deletionStore(db), time.Now)
	created, err := service.Change(t.Context(), actor, createCharacter(project))
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := service.Change(t.Context(), actor, app.Command{ProjectID: project, Kind: domain.KindCharacter, EntryID: created.EntryID, Action: "confirm", ExpectedRevision: created.Revision, Key: uuid.New(), RequestID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	command := app.Command{ProjectID: project, Kind: domain.KindCharacter, EntryID: created.EntryID, Action: "delete", ExpectedRevision: confirmed.Revision, Key: uuid.New(), RequestID: uuid.New()}
	proof := func() app.ImpactProof {
		t.Helper()
		_, err := service.Change(t.Context(), actor, command)
		var impact *app.ImpactConflict
		if !errors.As(err, &impact) {
			t.Fatalf("expected impact: %v", err)
		}
		return impact.Proof
	}
	empty := proof()
	if empty.CurrentLines != 0 || empty.HistoricalLines != 0 {
		t.Fatal(empty)
	}
	episode := seedDialogue(t, owner, actor, project, created.EntryID, created.VersionID)
	command.AcknowledgedImpact = &empty
	current := proof()
	if current.SHA256 == empty.SHA256 || current.CurrentLines != 1 || current.HistoricalLines != 0 {
		t.Fatal(current)
	}
	// Even a changed owning head with identical immutable text invalidates review.
	if err := owner.Exec(`UPDATE script.episode SET is_delete=true,revision=revision+1 WHERE id=?`, episode).Error; err != nil {
		t.Fatal(err)
	}
	command.AcknowledgedImpact = &current
	historical := proof()
	if historical.SHA256 == current.SHA256 || historical.CurrentLines != 0 || historical.HistoricalLines != 1 {
		t.Fatal(historical)
	}
	command.AcknowledgedImpact = &historical
	deleted, err := service.Change(t.Context(), actor, command)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.Change(t.Context(), actor, command)
	if err != nil || !reflect.DeepEqual(replay, deleted) {
		t.Fatalf("permanent delete replay: %v", err)
	}
	checkReference := func(wantDeleted bool) {
		t.Helper()
		err := db.Transaction(func(tx *gorm.DB) error {
			_, err := pg.NewReferences(tx, workspacepg.NewProjectContentAccessStore(tx)).Reference(t.Context(), actor, project, created.EntryID, &created.VersionID)
			return err
		})
		if wantDeleted && !errors.Is(err, app.ErrNotFound) || !wantDeleted && err != nil {
			t.Fatalf("reference admission: %v", err)
		}
	}
	checkReference(true)
	restored, err := app.NewService(deletionStore(db), time.Now).Change(t.Context(), actor, app.Command{ProjectID: project, Kind: domain.KindCharacter, EntryID: created.EntryID, Action: "restore", ExpectedRevision: deleted.Revision, Key: uuid.New(), RequestID: uuid.New()})
	if err != nil || restored.VersionID != created.VersionID {
		t.Fatalf("restore identity/version: %v", err)
	}
	checkReference(false)
	var lines int64
	if err := db.Raw(`SELECT count(*) FROM script.dialogue_line WHERE project_id=? AND character_id=? AND character_version_id=?`, project, created.EntryID, created.VersionID).Scan(&lines).Error; err != nil || lines != 1 {
		t.Fatalf("history changed: %d %v", lines, err)
	}
	foreign, _ := bibleActorProject(t, owner)
	if _, err := service.Change(t.Context(), foreign, command); err == nil {
		t.Fatal("cross-org deletion allowed")
	}
}

func TestSubjectDeletionAliasImpactAndAuditRollback(t *testing.T) {
	db, owner := deletionDatabase(t)
	actor, project := bibleActorProject(t, owner)
	service := app.NewService(deletionStore(db), time.Now)
	target, err := service.Change(t.Context(), actor, createCharacter(project))
	if err != nil {
		t.Fatal(err)
	}
	alias, err := service.Change(t.Context(), actor, createCharacter(project))
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE bible.character SET redirect_id=?,revision=revision+1 WHERE id=?`, target.EntryID, alias.EntryID).Error; err != nil {
		t.Fatal(err)
	}
	seedDialogue(t, owner, actor, project, alias.EntryID, alias.VersionID)
	command := app.Command{ProjectID: project, Kind: domain.KindCharacter, EntryID: target.EntryID, Action: "delete", ExpectedRevision: target.Revision, Key: uuid.New(), RequestID: uuid.New()}
	_, err = service.Change(t.Context(), actor, command)
	var impact *app.ImpactConflict
	if !errors.As(err, &impact) || impact.Proof.Aliases != 1 || impact.Proof.CurrentLines != 1 {
		t.Fatalf("alias reference omitted: %+v %v", impact, err)
	}
	command.AcknowledgedImpact = &impact.Proof
	failed := app.NewService(failingAuditStore{deletionStore(db)}, time.Now)
	if _, err := failed.Change(t.Context(), actor, command); err == nil {
		t.Fatal("audit failure accepted deletion")
	}
	detail, err := service.Find(t.Context(), actor, project, domain.KindCharacter, target.EntryID)
	if err != nil || detail.Head.Deleted || detail.Head.Revision != target.Revision {
		t.Fatalf("partial deletion persisted: %v", err)
	}
	var commands int64
	if err := db.Raw(`SELECT count(*) FROM bible.command WHERE actor_id=? AND request_id=?`, actor.ID, command.Key).Scan(&commands).Error; err != nil || commands != 0 {
		t.Fatal("failed deletion left a success receipt", err)
	}
}

// Fault injection returns an audit failure after real writes, inside the real transaction.
type failingAuditStore struct{ app.Store }

func (s failingAuditStore) Write(ctx context.Context, actor identityapp.Principal, project uuid.UUID, f func(app.Transaction) (app.Receipt, error)) (app.Receipt, error) {
	return s.Store.Write(ctx, actor, project, func(tx app.Transaction) (app.Receipt, error) { return f(failingAuditTransaction{tx}) })
}

type failingAuditTransaction struct{ app.Transaction }

func (t failingAuditTransaction) Record(ctx context.Context, actor identityapp.Principal, c app.Command, hash string, r app.Receipt, now time.Time) error {
	if err := t.Transaction.Record(ctx, actor, c, hash, r, now); err != nil {
		return err
	}
	return errors.New("injected audit failure")
}
