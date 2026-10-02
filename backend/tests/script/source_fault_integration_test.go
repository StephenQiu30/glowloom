package script_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

func scriptNullInsert(t *testing.T, owner *gorm.DB, table, column string, id uuid.UUID) func() {
	t.Helper()
	name := "script_null_" + fmt.Sprint(uuid.New().ID())
	if err := owner.Exec(`CREATE FUNCTION script.` + name + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$`).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`CREATE TRIGGER ` + name + ` BEFORE INSERT ON ` + table + ` FOR EACH ROW WHEN (NEW.` + column + `='` + id.String() + `'::uuid) EXECUTE FUNCTION script.` + name + `()`).Error; err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			if err := owner.Exec(`DROP TRIGGER ` + name + ` ON ` + table).Error; err != nil {
				t.Error(err)
			}
			if err := owner.Exec(`DROP FUNCTION script.` + name + `()`).Error; err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(release)
	return release
}

func TestScriptSourcePGMissingFormalInsertRollsBackWholePublication(t *testing.T) {
	for _, table := range []string{"script.version_source", "script.split_set", "script.version_head"} {
		t.Run(table, func(t *testing.T) {
			db, owner := scriptTestDB(t)
			actor, pid := scriptActorProject(t, owner)
			objects := &sourceObjects{data: make(map[string][]byte)}
			_, in := sourceCommand()
			in.ProjectID = pid
			release := scriptNullInsert(t, owner, table, "project_id", pid)
			service := app.NewSourceService(scriptStore(db), objects, time.Now)
			if _, err := service.Write(t.Context(), actor, in); err == nil {
				t.Fatal("missing immutable row published a successful receipt")
			}
			s, v, c := scriptCounts(t, owner, pid)
			var revision, events, receipts int64
			if err := owner.Raw(`SELECT revision FROM workspace.project WHERE id=?`, pid).Scan(&revision).Error; err != nil {
				t.Fatal(err)
			}
			if err := owner.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key=?`, pid.String()).Scan(&events).Error; err != nil {
				t.Fatal(err)
			}
			if err := owner.Raw(`SELECT count(*) FROM script.command_result WHERE actor_id=?`, actor.ID).Scan(&receipts).Error; err != nil {
				t.Fatal(err)
			}
			if s != 0 || v != 0 || c != 1 || revision != 1 || events != 0 || receipts != 0 {
				t.Fatalf("incomplete publication escaped rollback: sources=%d versions=%d command=%d revision=%d events=%d receipts=%d", s, v, c, revision, events, receipts)
			}
			release()
			if _, err := service.Write(t.Context(), actor, in); err != nil {
				t.Fatal("original key recovery", err)
			}
		})
	}
}

func TestScriptReviewPGMissingEpisodeOrLineRollsBackWholeConfirmation(t *testing.T) {
	for _, table := range []string{"script.episode", "script.action_line"} {
		t.Run(table, func(t *testing.T) {
			db, owner := scriptTestDB(t)
			actor, pid := scriptActorProject(t, owner)
			objects := &sourceObjects{data: make(map[string][]byte)}
			store := scriptStore(db)
			sources := app.NewSourceService(store, objects, time.Now)
			_, input := sourceCommand()
			input.ProjectID = pid
			input.Sources[0].Document = domain.RichDocument{Type: "doc", Content: []domain.RichDocument{{Type: "paragraph", Content: []domain.RichDocument{{Type: "text", Text: "走进客厅\n你好😀 "}}}}}
			saved, err := sources.Write(t.Context(), actor, input)
			if err != nil {
				t.Fatal(err)
			}
			view, err := store.Episodes(t.Context(), actor, pid, saved.VersionID)
			if err != nil {
				t.Fatal(err)
			}
			command := app.SplitCommand{ProjectID: pid, VersionID: saved.VersionID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 1, CandidateSetID: saved.SplitSetID, Boundaries: view.Candidate.Boundaries}
			if table == "script.episode" {
				scriptNullInsert(t, owner, table, "project_id", pid)
			}
			formal, err := store.ConfirmSplit(t.Context(), actor, command, time.Now().UTC())
			expectedRevision := int64(1)
			if table == "script.episode" {
				if err == nil {
					t.Fatal("formal confirmation omitted actual episode")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				expectedRevision = 2
				scriptNullInsert(t, owner, table, "project_id", pid)
				structure := app.StructureCommand{ProjectID: pid, EpisodeID: formal.Episodes[0].ID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 2, ExpectedEpisodeRevision: 1, Document: manualStructure()}
				if _, err := app.NewEpisodeService(store, sources, time.Now).SaveStructure(t.Context(), actor, structure); err == nil {
					t.Fatal("structured source omitted action line")
				}
			}
			var revision, structures int64
			if err := owner.Raw(`SELECT revision FROM script.project_state WHERE project_id=?`, pid).Scan(&revision).Error; err != nil {
				t.Fatal(err)
			}
			if err := owner.Table("script.episode_structure").Where("project_id=?", pid).Count(&structures).Error; err != nil || structures != 0 || revision != expectedRevision {
				t.Fatal("formal failure escaped rollback", revision, structures, err)
			}
		})
	}
}

func TestScriptSourcePGMissingRequestNamespaceNeverCreatesIntentOrObjects(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	objects := &sourceObjects{data: make(map[string][]byte)}
	_, input := sourceCommand()
	input.ProjectID = pid
	release := scriptNullInsert(t, owner, "script.request", "project_id", pid)
	if _, err := app.NewSourceService(scriptStore(db), objects, time.Now).Write(t.Context(), actor, input); err == nil {
		t.Fatal("source accepted without permanent cross-command scope")
	}
	if s, v, c := scriptCounts(t, owner, pid); s != 0 || v != 0 || c != 0 || objects.puts != 0 {
		t.Fatal("namespace failure wrote intent or private objects", s, v, c, objects.puts)
	}
	release()
	if _, err := app.NewSourceService(scriptStore(db), objects, time.Now).Write(t.Context(), actor, input); err != nil {
		t.Fatal("original key after proven rollback", err)
	}
}
