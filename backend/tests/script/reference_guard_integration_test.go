package script_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/adapter/extract"
	scriptobjects "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/objects"
	pg "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/postgres"
	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
)

func scriptReferenceDB(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	db, owner := scriptTestDB(t)
	var name string
	if err := owner.Raw(`SELECT current_database()`).Scan(&name).Error; err != nil || name != "lanverse_reference" {
		t.Fatal("reference guard requires isolated lanverse_reference database", name, err)
	}
	return db, owner
}

func scriptMediaReference(ctx context.Context, db *gorm.DB, actor identityapp.Principal, project, asset uuid.UUID) (bool, error) {
	var found bool
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		found, err = pg.NewMediaReferenceGuard(tx, workspacepg.NewProjectContentAccessStore(tx)).HasMediaReferences(ctx, actor, project, asset)
		return err
	})
	return found, err
}

// Synthetic historical mutations remain in an owned transaction and are rolled
// back. The actual guard runs under the nonowner role, never the schema owner.
func scriptReferenceMutation(t *testing.T, owner *gorm.DB, actor identityapp.Principal, project, asset uuid.UUID, seed func(*gorm.DB)) (bool, error) {
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
		t.Fatal("guard mutation fixture lacks actual nonowner", role, err)
	}
	return pg.NewMediaReferenceGuard(tx, workspacepg.NewProjectContentAccessStore(tx)).HasMediaReferences(t.Context(), actor, project, asset)
}

type failedReferenceObjects struct{ *scriptobjects.Storage }

func (o failedReferenceObjects) PutIfAbsent(context.Context, string, io.Reader, int64, string, string) error {
	return errors.New("synthetic original request failed before publication")
}

func scriptReferencePendingPlan(t *testing.T, db, owner *gorm.DB, actor identityapp.Principal, project uuid.UUID) (uuid.UUID, string) {
	t.Helper()
	objects := failedReferenceObjects{scriptobjects.NewStorage(scriptStorage(t))}
	_, command := sourceCommand()
	command.ProjectID = project
	if _, err := app.NewSourceService(scriptStore(db), objects, time.Now).Write(t.Context(), actor, command); err == nil {
		t.Fatal("synthetic object failure was swallowed")
	}
	var plan string
	read := owner.Raw(`SELECT c.plan::text FROM script.command c JOIN script.command_state s USING(actor_id,request_id) WHERE c.actor_id=? AND c.request_id=? AND s.status='pending'`, actor.ID, command.Key).Scan(&plan)
	if read.Error != nil || read.RowsAffected != 1 || plan == "" {
		t.Fatal("actual durable pending plan missing", read.Error)
	}
	if found, err := scriptMediaReference(t.Context(), db, actor, project, uuid.New()); err != nil || found {
		t.Fatal("valid manual plan invented media reference", found, err)
	}
	return command.Key, plan
}

func TestScriptMediaReferenceGuardPGUnknownPendingFieldsFailClosed(t *testing.T) {
	db, owner := scriptReferenceDB(t)
	actor, project := scriptActorProject(t, owner)
	key, original := scriptReferencePendingPlan(t, db, owner, actor, project)
	asset := uuid.New()
	for _, nested := range []bool{false, true} {
		name := "top_level"
		if nested {
			name = "nested_provenance"
		}
		t.Run(name, func(t *testing.T) {
			var plan map[string]any
			if err := json.Unmarshal([]byte(original), &plan); err != nil {
				t.Fatal(err)
			}
			target := plan
			if nested {
				target = plan["new_sources"].([]any)[0].(map[string]any)["provenance"].(map[string]any)
			}
			target["unknown_media"] = map[string]any{"asset_id": asset.String()}
			encoded, err := json.Marshal(plan)
			if err != nil {
				t.Fatal(err)
			}
			found, err := scriptReferenceMutation(t, owner, actor, project, asset, func(tx *gorm.DB) {
				if err := tx.Exec(`UPDATE script.command SET plan=?::jsonb WHERE actor_id=? AND request_id=?`, string(encoded), actor.ID, key).Error; err != nil {
					t.Fatal(err)
				}
			})
			if !errors.Is(err, app.ErrUnavailable) || found {
				t.Fatal("unknown pending historical field was treated as no reference", found, err)
			}
		})
	}
	var after string
	if err := owner.Raw(`SELECT plan::text FROM script.command WHERE actor_id=? AND request_id=?`, actor.ID, key).Scan(&after).Error; err != nil || after != original {
		t.Fatal("guard mutation changed durable plan", err)
	}
}

// This compact, known-field replay metadata exercises history budgets without
// storing any private body or claiming that synthetic commands were published.
func scriptReferenceBudgetPlan(t *testing.T, actor identityapp.Principal, project uuid.UUID, padding int) string {
	t.Helper()
	version := uuid.New().String()
	plan := map[string]any{
		"command":  map[string]any{"project_id": project.String(), "key": uuid.New().String(), "action": strings.Repeat("a", padding)},
		"actor_id": actor.ID.String(), "org_id": actor.OrgID.String(), "request_hash": strings.Repeat("0", 64),
		"version":    map[string]any{"id": version, "org_id": actor.OrgID.String(), "project_id": project.String()},
		"candidate":  map[string]any{"version_id": version, "org_id": actor.OrgID.String(), "project_id": project.String()},
		"created_at": time.Now().UTC(), "reuse": true,
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func scriptReferenceSeedPlans(t *testing.T, tx *gorm.DB, actor identityapp.Principal, project uuid.UUID, plan string, count int) {
	t.Helper()
	read := tx.Exec(`WITH requests AS (
 INSERT INTO script.request(actor_id,request_id,org_id,project_id,action,request_hash,created_at)
 SELECT ?,gen_random_uuid(),?,?,'source_create',repeat('0',64),clock_timestamp() FROM generate_series(1,?)
 RETURNING actor_id,request_id,org_id,project_id,action,request_hash,created_at
), commands AS (
 INSERT INTO script.command(actor_id,request_id,org_id,project_id,action,request_hash,plan,created_at)
 SELECT actor_id,request_id,org_id,project_id,action,request_hash,jsonb_set(?::jsonb,'{command,key}',to_jsonb(request_id::text)),created_at FROM requests
 RETURNING actor_id,request_id
)
 INSERT INTO script.command_state(actor_id,request_id,id,status,updated_at)
 SELECT actor_id,request_id,gen_random_uuid(),'pending',clock_timestamp() FROM commands`, actor.ID, actor.OrgID, project, count, plan)
	if read.Error != nil || read.RowsAffected != int64(count) {
		t.Fatal("owned synthetic pending history", read.RowsAffected, read.Error)
	}
}

func TestScriptMediaReferenceGuardPGBoundedPendingHistory(t *testing.T) {
	_, owner := scriptReferenceDB(t)
	for _, test := range []struct {
		name    string
		padding int
		count   int
	}{
		{"single_body", 1 << 20, 1},
		{"aggregate_bytes", 512 << 10, 129},
		{"row_count", 0, 50001},
	} {
		t.Run(test.name, func(t *testing.T) {
			actor, project := scriptActorProject(t, owner)
			asset := uuid.New()
			// Prove the same closed metadata schema is readable below each budget.
			baseline := scriptReferenceBudgetPlan(t, actor, project, 0)
			found, err := scriptReferenceMutation(t, owner, actor, project, asset, func(tx *gorm.DB) {
				scriptReferenceSeedPlans(t, tx, actor, project, baseline, 1)
			})
			if err != nil || found {
				t.Fatal("valid bounded replay metadata rejected", found, err)
			}
			plan := scriptReferenceBudgetPlan(t, actor, project, test.padding)
			if test.name == "row_count" && len(plan)*test.count >= 64<<20 {
				t.Fatal("row budget fixture exceeds aggregate budget first")
			}
			found, err = scriptReferenceMutation(t, owner, actor, project, asset, func(tx *gorm.DB) {
				scriptReferenceSeedPlans(t, tx, actor, project, plan, test.count)
			})
			if !errors.Is(err, app.ErrUnavailable) || found {
				t.Fatal("oversized pending history was reported unreferenced", found, err)
			}
			var remaining int64
			if err := owner.Table("script.command").Where("project_id=?", project).Count(&remaining).Error; err != nil || remaining != 0 {
				t.Fatal("synthetic history transaction was not rolled back", remaining, err)
			}
		})
	}
}

func TestScriptMediaReferenceGuardPGActualOriginalAndCompleteImportHistory(t *testing.T) {
	db, owner := scriptReferenceDB(t)
	actor, project := scriptActorProject(t, owner)
	objects := scriptStorage(t)
	document := scriptDocument(t, db, objects, actor, project, "原件历史.txt", []byte("第一章\n甲😀乙"))
	imports := app.NewImportService(scriptImportStore(db, objects), time.Now)
	command := app.ImportCommand{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{document}, RightsConfirmed: true}
	queued, err := imports.Create(t.Context(), actor, command)
	if err != nil || queued.Status != "queued" {
		t.Fatal("actual original admission", queued, err)
	}
	if found, err := scriptMediaReference(t.Context(), db, actor, project, document); err != nil || !found {
		t.Fatal("queued original document lost pin", found, err)
	}
	cancelKey := uuid.New()
	if _, err := imports.Control(t.Context(), actor, app.ImportControl{ProjectID: project, JobID: queued.ID, Action: "cancel", ExpectedRevision: queued.Revision, Key: cancelKey, RequestID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	worker := scriptImportWorker(db, objects, extract.NewExtractor())
	if err := worker.Control(t.Context(), importDelivery(t, owner, actor, cancelKey)); err != nil {
		t.Fatal(err)
	}
	if found, err := scriptMediaReference(t.Context(), db, actor, project, document); err != nil || found {
		t.Fatal("stopped unpublished import fabricated permanent pin", found, err)
	}
	command.Key, command.RequestID = uuid.New(), uuid.New()
	published, err := imports.Create(t.Context(), actor, command)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Execute(t.Context(), importDelivery(t, owner, actor, command.Key)); err != nil {
		t.Fatal(err)
	}
	published, err = imports.Get(t.Context(), actor, project, published.ID)
	if err != nil || published.Status != "succeeded" || published.LatestVersionID == nil || len(published.Files) != 1 || published.Files[0].LineageID == nil {
		t.Fatal("actual immutable source publication", published, err)
	}
	sources := app.NewSourceService(scriptStore(db), scriptobjects.NewStorage(objects), time.Now)
	if _, err := sources.Write(t.Context(), actor, app.SourceCommand{ProjectID: project, ExpectedRevision: published.LatestScriptRevision, BaseVersionID: published.LatestVersionID, LineageID: published.Files[0].LineageID, Action: "delete", RightsConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	if found, err := scriptMediaReference(t.Context(), db, actor, project, document); err != nil || !found {
		t.Fatal("removed current chapter lost complete historical original pin", found, err)
	}
}

func TestScriptMediaReferenceGuardPGCallerTransactionAndCurrentScope(t *testing.T) {
	db, owner := scriptReferenceDB(t)
	actor, project := scriptActorProject(t, owner)
	_, foreign := scriptActorProject(t, owner)
	asset := uuid.New()
	if found, err := scriptMediaReference(t.Context(), db, actor, project, asset); err != nil || found {
		t.Fatal("authorized empty history", found, err)
	}
	if _, err := pg.NewMediaReferenceGuard(db, workspacepg.NewProjectContentAccessStore(db)).HasMediaReferences(t.Context(), actor, project, asset); err == nil {
		t.Fatal("guard accepted connection pool instead of owning transaction")
	}
	if _, err := scriptMediaReference(t.Context(), db, actor, foreign, asset); err == nil {
		t.Fatal("foreign scope was reported empty")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := scriptMediaReference(ctx, db, actor, project, asset); err == nil {
		t.Fatal("cancelled authorization was reported empty")
	}
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := scriptMediaReference(t.Context(), db, actor, project, asset); err == nil {
		t.Fatal("disabled current actor read history")
	}
}
