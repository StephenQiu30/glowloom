package workspace_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	canvasfixture "github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"

	"github.com/google/uuid"

	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func TestCreationRecoveryDoesNotWaitForCreationAdvisoryLock(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	key := uuid.New()
	locked := db.Begin()
	if locked.Error != nil {
		t.Fatal(locked.Error)
	}
	defer locked.Rollback()
	if err := locked.Exec(`SELECT pg_advisory_xact_lock(69360,hashtext(?))`, actor.ID.String()+":"+key.String()).Error; err != nil {
		t.Fatal(err)
	}
	queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	result, err := workspaceapp.NewProjectCreationRecoveryQuery(workspaceStore(db)).Resolve(queryCtx, actor, key)
	if err != nil || result.State != "unrecorded" {
		t.Fatalf("readonly query waited for creation writer: %+v %v", result, err)
	}
}

func TestCreationRecoveryRejectsBrokenMainAndCorruptReceipt(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	key := uuid.New()
	query := workspaceapp.NewProjectCreationRecoveryQuery(workspaceStore(db))
	created, err := workspaceapp.NewCreateProjectCommand(workspaceStore(db), time.Now).ExecuteIdempotent(ctx, actor, key, workspaceapp.CreateProjectInput{Name: "未命名画布", AspectRatio: "16:9", StyleType: "realistic", RequestID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE canvas.document SET is_delete=true WHERE id=?`, created.CanvasID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := query.Resolve(ctx, actor, key); !errors.Is(err, workspaceapp.ErrMainCanvasUnavailable) {
		t.Fatalf("broken main: %v", err)
	}
	if err := db.Exec(`UPDATE canvas.document SET is_delete=false WHERE id=?`, created.CanvasID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE workspace.project_creation_receipt SET response_body='{}'::jsonb WHERE actor_id=? AND idem_key=?`, actor.ID, key).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := query.Resolve(ctx, actor, key); !errors.Is(err, workspaceapp.ErrProjectDependencyUnavailable) {
		t.Fatalf("corrupt receipt: %v", err)
	}
}

func TestBlankCreationRollsBackWhenFinalReceiptCannotBeWritten(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	key := uuid.New()
	// A unique fixture-only trigger faults the last write, after project, graph, budget and activity insertion.
	suffix := uuid.New().String()
	function := "fault_" + strings.ReplaceAll(suffix, "-", "")
	trigger := function
	ddl := fmt.Sprintf(`CREATE FUNCTION workspace.%s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.actor_id='%s'::uuid THEN RAISE EXCEPTION 'fixture receipt write failure'; END IF; RETURN NEW; END $$`, function, actor.ID)
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON workspace.project_creation_receipt`, trigger))
		db.Exec(fmt.Sprintf(`DROP FUNCTION IF EXISTS workspace.%s()`, function))
	})
	if err := db.Exec(fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON workspace.project_creation_receipt FOR EACH ROW EXECUTE FUNCTION workspace.%s()`, trigger, function)).Error; err != nil {
		t.Fatal(err)
	}
	_, err := workspaceapp.NewCreateProjectCommand(workspaceStore(db), time.Now).ExecuteIdempotent(ctx, actor, key, workspaceapp.CreateProjectInput{Name: "未命名画布", AspectRatio: "16:9", StyleType: "realistic", RequestID: uuid.NewString()})
	if err == nil {
		t.Fatal("receipt failure accepted")
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM workspace.project WHERE owner_id=?`, actor.ID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("partial creation: %d %v", count, err)
	}
	result, err := workspaceapp.NewProjectCreationRecoveryQuery(workspaceStore(db)).Resolve(ctx, actor, key)
	if err != nil || result.State != "unrecorded" {
		t.Fatalf("rolled back receipt: %+v %v", result, err)
	}
	// Foreign keys ensure that no canvas, activity or budget can survive the rolled-back project.
}

func TestCreationRecoveryDoesNotExposeReceiptInAnotherOwnedScope(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	key := uuid.New()
	_, err := workspaceapp.NewCreateProjectCommand(workspaceStore(db), time.Now).ExecuteIdempotent(ctx, actor, key, workspaceapp.CreateProjectInput{Name: "未命名画布", AspectRatio: "16:9", StyleType: "realistic", RequestID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	teamActor := workspaceTeamActor(t, db, actor)
	if _, err := workspaceapp.NewProjectCreationRecoveryQuery(workspaceStore(db)).Resolve(ctx, teamActor, key); !errors.Is(err, workspaceapp.ErrProjectNotFound) {
		t.Fatalf("receipt exposed across owned scopes: %v", err)
	}
}

func TestProjectRenameAndCanvasSaveAvoidLockUpgrade(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	project, err := workspaceapp.NewCreateProjectCommand(workspaceStore(db), time.Now).ExecuteIdempotent(ctx, actor, uuid.New(), workspaceapp.CreateProjectInput{Name: "未命名画布", AspectRatio: "16:9", StyleType: "realistic", RequestID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	canvas := canvasfixture.NewService(workspaceCanvasStore(db, nil))
	name, text := "并发后的名称", "并发保存的文字"
	patch := projectChange(project.ID, 1, "patch")
	patch.Patch.Name = &name
	workCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := workspaceapp.NewProjectLifecycle(lifecycleStore(db, fixedProjectWork{}), time.Now).Change(workCtx, actor, patch)
		results <- err
	}()
	go func() {
		<-start
		_, err := canvas.Edit(workCtx, actor, project.CanvasID, uuid.NewString(), canvasapp.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: canvasfixture.Concat(canvasfixture.Nodes([]canvasdomain.Node{{ID: uuid.New(), Title: "并发文字", NodeType: "text", NodeAction: "resource", Config: canvasdomain.NodeConfig{Text: &text}}}))})
		results <- err
	}()
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	current, err := canvas.Get(ctx, actor, project.CanvasID)
	if err != nil || current.Name != name || len(current.Nodes) != 1 || current.Revision != 2 {
		t.Fatalf("concurrent canvas lost data: %+v %v", current, err)
	}
	var revision int64
	if err := db.Raw(`SELECT revision FROM workspace.project WHERE id=?`, project.ID).Scan(&revision).Error; err != nil || revision != 2 {
		t.Fatalf("content advanced project CAS: %d %v", revision, err)
	}
}
