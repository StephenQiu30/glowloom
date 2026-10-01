package workspace_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvaspg "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	toolpg "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/postgres"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	operationpg "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

type owningProjectWork []workspaceapp.ProjectWorkGuard

func (g owningProjectWork) HasInflightWork(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (bool, error) {
	blocked := false
	for _, owner := range g {
		active, err := owner.HasInflightWork(ctx, actor, project)
		if err != nil {
			return false, err
		}
		blocked = blocked || active
	}
	return blocked, nil
}
func owningLifecycleStore(db *gorm.DB) *workspacepg.Store {
	return workspacepg.NewStoreWithProjectWorkGuard(db, func(tx *gorm.DB) workspaceapp.ProjectWorkGuard {
		return owningProjectWork{operationpg.NewStore(tx), mediapg.NewStore(tx), toolpg.NewStore(tx, nil, nil)}
	})
}
func lifecycleActorProject(ctx context.Context, t *testing.T, db *gorm.DB) (identityapp.Principal, uuid.UUID) {
	t.Helper()
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	id := uuid.MustParse(insertWorkspaceProject(ctx, t, db, actor.OrgID.String(), "16:9", "realistic", nil))
	if err := db.Exec(`INSERT INTO billing.budget(id,project_id,limit_micros) VALUES(?,?,1000)`, uuid.New(), id).Error; err != nil {
		t.Fatal(err)
	}
	return actor, id
}
func lifecycleFixtureSQL(t *testing.T, db *gorm.DB, query string, args ...any) {
	t.Helper()
	if err := db.Exec(query, args...).Error; err != nil {
		t.Fatal(err)
	}
}

func TestProjectLifecycleOwningGuardsCoverDurableWorkAndKeepContent(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, project := lifecycleActorProject(ctx, t, db)
	operation, asset, job, canvas, node := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	lifecycleFixtureSQL(t, db, `INSERT INTO operation.operation(id,project_id,target_type,capability,mode,input_hash,origin,status,quote_micros,quote_expires_at) VALUES(?,?,'free','image.generate','text_to_image','synthetic-guard','upload','draft',0,clock_timestamp()+interval '1 hour')`, operation, project)
	lifecycleFixtureSQL(t, db, `INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,mime_type,byte_size) VALUES(?,?,'image','upload','ready',?,'image/png',12)`, asset, project, "lifecycle-test/"+asset.String())
	lifecycleFixtureSQL(t, db, `INSERT INTO canvas.document(id,project_id,name) VALUES(?,?,'preserved canvas')`, canvas, project)
	lifecycleFixtureSQL(t, db, `INSERT INTO canvas.node(id,document_id,title,node_type,node_action,config,x,y) VALUES(?,?,'preserved node','text','resource','{"text":"完整保留"}',12,34)`, node, canvas)
	lifecycleFixtureSQL(t, db, `INSERT INTO mediatool.export_job(id,project_id,org_id,actor_id,actor_role,canvas_id,node_id,source_revision,frozen,status,stage,progress,attempt,revision,created_at,updated_at) VALUES(?,?,?,?,'producer',?,?,1,'{}','failed','failed',0,1,1,clock_timestamp(),clock_timestamp())`, job, project, actor.OrgID, actor.ID, canvas, node)
	service := workspaceapp.NewProjectLifecycle(owningLifecycleStore(db), time.Now)
	for _, group := range []struct {
		table    string
		id       uuid.UUID
		statuses []string
		idle     string
	}{
		{"operation.operation", operation, []string{"confirmed", "submitting", "submitted", "succeeded", "ingesting", "unknown", "reconciling", "manual", "cancelling"}, "completed"},
		{"media.media_asset", asset, []string{"uploading", "processing"}, "ready"},
		{"mediatool.export_job", job, []string{"queued", "running", "cancel_requested"}, "failed"},
	} {
		for _, status := range group.statuses {
			lifecycleFixtureSQL(t, db, "UPDATE "+group.table+" SET status=? WHERE id=?", status, group.id)
			for _, action := range []string{"archive", "delete"} {
				if _, err := service.Change(ctx, actor, projectChange(project, 1, action)); !errors.Is(err, domain.ErrProjectHasInflightOperations) {
					t.Fatalf("%s/%s accepted %s: %v", group.table, status, action, err)
				}
			}
		}
		lifecycleFixtureSQL(t, db, "UPDATE "+group.table+" SET status=? WHERE id=?", group.idle, group.id)
	}
	snapshot := func() []string {
		t.Helper()
		var rows []string
		query := `SELECT row_to_json(d)::text FROM canvas.document d WHERE d.id=? UNION ALL SELECT row_to_json(n)::text FROM canvas.node n WHERE n.id=? UNION ALL SELECT row_to_json(a)::text FROM media.media_asset a WHERE a.id=? UNION ALL SELECT row_to_json(o)::text FROM operation.operation o WHERE o.id=? UNION ALL SELECT row_to_json(b)::text FROM billing.budget b WHERE b.project_id=? UNION ALL SELECT row_to_json(j)::text FROM mediatool.export_job j WHERE j.id=?`
		if err := db.Raw(query, canvas, node, asset, operation, project, job).Scan(&rows).Error; err != nil {
			t.Fatal(err)
		}
		return rows
	}
	before := snapshot()
	if _, err := service.Change(ctx, actor, projectChange(project, 1, "delete")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Change(ctx, actor, projectChange(project, 2, "restore")); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(); len(after) != 6 || !reflect.DeepEqual(before, after) {
		t.Fatal("soft deletion/restoration mutated retained business facts")
	}
}

func TestProjectLifecycleConcurrentCASAndDurableNoop(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, project := lifecycleActorProject(ctx, t, db)
	service := workspaceapp.NewProjectLifecycle(owningLifecycleStore(db), time.Now)
	input := projectChange(project, 1, "patch")
	name := "同键一次提交"
	input.Patch.Name = &name
	const count = 8
	results := make([]workspaceapp.ProjectSnapshot, count)
	errs := make([]error, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Go(func() { results[i], errs[i] = service.Change(ctx, actor, input) })
	}
	wg.Wait()
	for i := range count {
		if errs[i] != nil || results[i].Project.Revision != 2 || !results[i].Project.UpdateTime.Equal(results[0].Project.UpdateTime) {
			t.Fatalf("concurrent replay %d: %v", i, errs[i])
		}
	}
	var counts struct{ Events, Receipts int64 }
	if err := db.Raw(`SELECT (SELECT count(*) FROM infra.outbox WHERE partition_key=?) AS events,(SELECT count(*) FROM infra.idempotency_record WHERE actor_id=?) AS receipts`, project.String(), actor.ID).Scan(&counts).Error; err != nil {
		t.Fatal(err)
	}
	if counts.Events != 2 || counts.Receipts != 1 {
		t.Fatalf("duplicate commit %+v", counts)
	}
	nop := projectChange(project, 2, "patch")
	nop.Patch.Name = &name
	if _, err := service.Change(ctx, actor, nop); err != nil {
		t.Fatal(err)
	}
	a, b := projectChange(project, 2, "patch"), projectChange(project, 2, "patch")
	an, bn := "CAS A", "CAS B"
	a.Patch.Name = &an
	b.Patch.Name = &bn
	wg.Go(func() { _, errs[0] = service.Change(ctx, actor, a) })
	wg.Go(func() { _, errs[1] = service.Change(ctx, actor, b) })
	wg.Wait()
	firstWon := errs[0] == nil && errors.Is(errs[1], domain.ErrProjectRevisionConflict)
	secondWon := errs[1] == nil && errors.Is(errs[0], domain.ErrProjectRevisionConflict)
	if !firstWon && !secondWon {
		t.Fatalf("CAS results %v/%v", errs[0], errs[1])
	}
	// Replaying the original no-op after a later edit must return its first snapshot.
	old, err := service.Change(ctx, actor, nop)
	if err != nil || old.Project.Revision != 2 || old.Project.Name != name {
		t.Fatalf("no-op replay %+v %v", old, err)
	}
}

func TestProjectLifecycleRuntimeRoleAndAtomicFaults(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, project := lifecycleActorProject(ctx, t, db)
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		service := workspaceapp.NewProjectLifecycle(owningLifecycleStore(tx), time.Now)
		_, err := service.Change(ctx, actor, projectChange(project, 1, "archive"))
		return err
	}); err != nil {
		t.Fatal("runtime-role lifecycle", err)
	}
	service := workspaceapp.NewProjectLifecycle(owningLifecycleStore(db), time.Now)
	if _, err := service.Change(ctx, actor, projectChange(project, 2, "unarchive")); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"infra.outbox", "infra.idempotency_record"} {
		input := projectChange(project, 3, "delete")
		rollback := errors.New("test rollback")
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(`CREATE FUNCTION pg_temp.reject_lifecycle_write() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'synthetic lifecycle write fault'; END$$`).Error; err != nil {
				return err
			}
			if err := tx.Exec("CREATE TRIGGER lifecycle_test_fault BEFORE INSERT ON " + table + " FOR EACH ROW EXECUTE FUNCTION pg_temp.reject_lifecycle_write()").Error; err != nil {
				return err
			}
			if _, err := workspaceapp.NewProjectLifecycle(owningLifecycleStore(tx), time.Now).Change(ctx, actor, input); err == nil {
				t.Error("write fault reported success", table)
			}
			return rollback
		})
		if !errors.Is(err, rollback) {
			t.Fatal(err)
		}
		saved, err := service.Get(ctx, actor, project)
		if err != nil || saved.Project.Revision != 3 || saved.Project.IsDelete {
			t.Fatalf("fault leaked state %+v %v", saved, err)
		}
		var count int64
		if err := db.Raw(`SELECT count(*) FROM infra.idempotency_record WHERE actor_id=? AND idem_key=?`, actor.ID, input.IdempotencyKey.String()).Scan(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatal("fault recorded a success receipt")
		}
	}
	// Server transaction time prevents an expired recovery from using an earlier app clock.
	lifecycleFixtureSQL(t, db, `UPDATE workspace.project SET is_delete=true,delete_time=clock_timestamp()-interval '31 days',purge_after=clock_timestamp()-interval '1 second',revision=4 WHERE id=?`, project)
	oldClock := func() time.Time { return time.Now().Add(-time.Hour) }
	if _, err := workspaceapp.NewProjectLifecycle(owningLifecycleStore(db), oldClock).Change(ctx, actor, projectChange(project, 4, "restore")); !errors.Is(err, domain.ErrProjectRestoreExpired) {
		t.Fatalf("expired restore %v", err)
	}
}

func projectTransactionPID(t *testing.T, tx *gorm.DB) int {
	t.Helper()
	var pid int
	if err := tx.Raw(`SELECT pg_backend_pid()`).Scan(&pid).Error; err != nil {
		t.Fatal(err)
	}
	return pid
}
func waitProjectLock(ctx context.Context, t *testing.T, db *gorm.DB, blocker, blocked int) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		var found bool
		if err := db.Raw(`SELECT ?::integer = ANY(pg_blocking_pids(?))`, blocker, blocked).Scan(&found).Error; err != nil {
			t.Fatal(err)
		}
		if found {
			return
		}
		select {
		case <-timer.C:
			t.Fatal("did not observe the actual PostgreSQL project lock")
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-tick.C:
		}
	}
}

func TestProjectLifecycleProjectLockSerializesRealMediaAdmission(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, project := lifecycleActorProject(ctx, t, db)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	admission := db.WithContext(ctx).Begin()
	if admission.Error != nil {
		t.Fatal(admission.Error)
	}
	t.Cleanup(func() {
		if err := admission.Rollback().Error; err != nil && !errors.Is(err, gorm.ErrInvalidTransaction) && !errors.Is(err, sql.ErrTxDone) {
			t.Error(err)
		}
	})
	if err := mediapg.NewStore(admission).AuthorizeDerivedProject(ctx, actor, project, true); err != nil {
		t.Fatal(err)
	}
	archiveTx := db.WithContext(ctx).Begin()
	if archiveTx.Error != nil {
		t.Fatal(archiveTx.Error)
	}
	t.Cleanup(func() {
		if err := archiveTx.Rollback().Error; err != nil && !errors.Is(err, gorm.ErrInvalidTransaction) && !errors.Is(err, sql.ErrTxDone) {
			t.Error(err)
		}
	})
	blocker, blocked := projectTransactionPID(t, admission), projectTransactionPID(t, archiveTx)
	done := make(chan error, 1)
	finished := false
	defer func() {
		cancel()
		if !finished {
			<-done
		}
	}()
	go func() {
		_, err := workspaceapp.NewProjectLifecycle(owningLifecycleStore(archiveTx), time.Now).Change(ctx, actor, projectChange(project, 1, "archive"))
		done <- err
	}()
	waitProjectLock(ctx, t, db, blocker, blocked)
	now := time.Now().UTC()
	id := uuid.New()
	sha := strings.Repeat("a", 64)
	asset := mediadomain.MediaAsset{ID: id, ProjectID: project, Kind: mediadomain.KindImage, Origin: mediadomain.OriginSystem, Status: mediadomain.StatusProcessing, ObjectKey: fmt.Sprintf("projects/%s/image/2026/10/%s.png", project, id), MimeType: "image/png", ByteSize: 12, SHA256: &sha, ModerationStatus: mediadomain.ModerationPending, Revision: 1, CreateTime: now, UpdateTime: now}
	if err := mediapg.NewStore(admission).StoreDerived(ctx, actor, asset, nil); err != nil {
		t.Fatal(err)
	}
	if err := admission.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		finished = true
		if !errors.Is(err, domain.ErrProjectHasInflightOperations) {
			t.Fatalf("archive bypassed newly admitted processing media: %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Roll back the failed archive transaction before the next command.
	if err := archiveTx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	lifecycleFixtureSQL(t, db, `UPDATE media.media_asset SET status='failed' WHERE id=?`, id)
	if _, err := workspaceapp.NewProjectLifecycle(owningLifecycleStore(db), time.Now).Change(ctx, actor, projectChange(project, 1, "delete")); err != nil {
		t.Fatal(err)
	}
	if err := mediapg.NewStore(db).AuthorizeDerivedProject(ctx, actor, project, true); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatalf("new media admission ignored deletion: %v", err)
	}
}

func lifecycleQuote(t *testing.T, db *gorm.DB, actor identityapp.Principal, project uuid.UUID) operationapp.CreateFreeQuoteResult {
	t.Helper()
	provider, model, version, price := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	capability := "image.generate." + uuid.NewString()
	modelKey := "lifecycle-model-" + model.String()
	lifecycleFixtureSQL(t, db, `INSERT INTO catalog.provider(id,key,name,adapter_key,region) VALUES(?,?,'Lifecycle Test','test','domestic')`, provider, "lifecycle-"+provider.String())
	lifecycleFixtureSQL(t, db, `INSERT INTO catalog.provider_credential(id,provider_id,label,ciphertext,key_id,last4) VALUES(?,?,'synthetic',?,'test','1234')`, uuid.New(), provider, []byte("synthetic-no-provider-use"))
	lifecycleFixtureSQL(t, db, `INSERT INTO catalog.capability(id,key,output_type,modes,input_roles) VALUES(?,?,'image',ARRAY['text_to_image'],ARRAY['prompt'])`, uuid.New(), capability)
	lifecycleFixtureSQL(t, db, `INSERT INTO catalog.model_profile(id,model_key,provider_id,capability,display_name,status) VALUES(?,?,?,?,'Lifecycle Test','active')`, model, modelKey, provider, capability)
	lifecycleFixtureSQL(t, db, `INSERT INTO catalog.model_profile_version(id,model_profile_id,version_no,provider_model_id,modes,limits,param_schema,supports_query,supports_cancel,supports_callback,expected_max_ms) VALUES(?,?,1,'test',ARRAY['text_to_image'],'{"max_outputs":1}','[]',true,false,false,30000)`, version, model)
	lifecycleFixtureSQL(t, db, `UPDATE catalog.model_profile SET current_version_id=? WHERE id=?`, version, model)
	lifecycleFixtureSQL(t, db, `INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,effective_from) VALUES(?,?,1,'per_image','{"base_micros":1}',clock_timestamp()-interval '1 minute')`, price, model)
	quote, err := operationpg.NewStore(db).CreateFreeQuote(t.Context(), actor, operationapp.CreateFreeQuoteInput{ProjectID: project, RequestID: uuid.NewString(), FreeQuoteItemInput: operationapp.FreeQuoteItemInput{ModelKey: modelKey, Capability: capability, Mode: "text_to_image", Prompt: "合成行锁报价", Params: json.RawMessage(`{}`), OutputCount: 1}})
	if err != nil {
		t.Fatal(err)
	}
	return quote
}

func TestProjectLifecycleBlocksConfirmedQuoteAndRejectsNewAdmission(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, project := lifecycleActorProject(ctx, t, db)
	quote := lifecycleQuote(t, db, actor, project)
	operations := operationpg.NewStore(db)
	if _, err := operations.ConfirmSingleQuote(ctx, actor, operationapp.ConfirmSingleQuoteInput{ProjectID: project, OperationID: quote.OperationID, RequestID: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	lifecycle := workspaceapp.NewProjectLifecycle(owningLifecycleStore(db), time.Now)
	if _, err := lifecycle.Change(ctx, actor, projectChange(project, 1, "delete")); !errors.Is(err, domain.ErrProjectHasInflightOperations) {
		t.Fatalf("confirmed operation ignored: %v", err)
	}
	actor2, project2 := lifecycleActorProject(ctx, t, db)
	quote2 := lifecycleQuote(t, db, actor2, project2)
	if _, err := lifecycle.Change(ctx, actor2, projectChange(project2, 1, "archive")); err != nil {
		t.Fatal(err)
	}
	if _, err := operations.ConfirmSingleQuote(ctx, actor2, operationapp.ConfirmSingleQuoteInput{ProjectID: project2, OperationID: quote2.OperationID, RequestID: uuid.NewString()}); !errors.Is(err, operationpg.ErrNotFound) {
		t.Fatalf("archived quote newly confirmed: %v", err)
	}
	if _, err := canvasapp.NewService(canvaspg.NewStore(db, nil)).Create(ctx, actor2, project2, uuid.NewString(), canvasapp.CreateInput{Name: "should reject"}); !errors.Is(err, canvasapp.ErrNotFound) {
		t.Fatalf("archived canvas admission: %v", err)
	}
	// Formal tool admission reaches its real project authorizer before source processing.
	exports := toolpg.NewStore(db, func(*gorm.DB) toolapp.TimelineReader { return canvasapp.NewTimelineReader(canvaspg.NewStore(db, nil)) }, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(mediapg.NewStore(tx)) })
	if _, err := exports.Create(ctx, actor2, project2, uuid.New(), toolapp.CreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1}); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatalf("archived export admission: %v", err)
	}
}

func assertLifecycleFence(ctx context.Context, t *testing.T, db *gorm.DB, actor identityapp.Principal, project uuid.UUID, admit func(*gorm.DB) error) {
	t.Helper()
	ownedCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	admission := db.WithContext(ownedCtx).Begin()
	if admission.Error != nil {
		t.Fatal(admission.Error)
	}
	defer rollbackLifecycleTest(t, admission)
	if err := admit(admission); err != nil {
		t.Fatal(err)
	}
	archive := db.WithContext(ownedCtx).Begin()
	if archive.Error != nil {
		t.Fatal(archive.Error)
	}
	blocker, blocked := projectTransactionPID(t, admission), projectTransactionPID(t, archive)
	done := make(chan error, 1)
	var finished bool
	go func() {
		_, err := workspaceapp.NewProjectLifecycle(owningLifecycleStore(archive), time.Now).Change(ownedCtx, actor, projectChange(project, 1, "archive"))
		done <- err
	}()
	defer func() {
		if !finished {
			cancel()
			<-done
		}
		rollbackLifecycleTest(t, archive)
	}()
	waitProjectLock(ownedCtx, t, db, blocker, blocked)
	if err := admission.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		finished = true
		if !errors.Is(err, domain.ErrProjectHasInflightOperations) {
			t.Fatalf("lifecycle ignored committed admission: %v", err)
		}
	case <-ownedCtx.Done():
		t.Fatal(ownedCtx.Err())
	}
}

func rollbackLifecycleTest(t *testing.T, tx *gorm.DB) {
	t.Helper()
	if err := tx.Rollback().Error; err != nil && !errors.Is(err, sql.ErrTxDone) && !errors.Is(err, gorm.ErrInvalidTransaction) && !errors.Is(err, context.Canceled) {
		t.Error(err)
	}
}

func TestProjectLifecycleOperationAndExportAdmissionLocks(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	t.Run("operation confirmation", func(t *testing.T) {
		actor, project := lifecycleActorProject(ctx, t, db)
		quote := lifecycleQuote(t, db, actor, project)
		assertLifecycleFence(ctx, t, db, actor, project, func(tx *gorm.DB) error {
			_, err := operationpg.NewStore(tx).ConfirmSingleQuote(ctx, actor, operationapp.ConfirmSingleQuoteInput{ProjectID: project, OperationID: quote.OperationID, RequestID: uuid.NewString()})
			return err
		})
	})
	t.Run("formal export start", func(t *testing.T) {
		actor, project := lifecycleActorProject(ctx, t, db)
		canvasStore := canvaspg.NewStore(db, nil)
		canvas := canvasapp.NewService(canvasStore)
		doc, err := canvas.Create(ctx, actor, project, uuid.NewString(), canvasapp.CreateInput{Name: "生命周期导出竞态"})
		if err != nil {
			t.Fatal(err)
		}
		track, node := uuid.New(), uuid.New()
		config := canvasdomain.TimelineConfig{Version: 1, AspectRatio: "16:9", FPS: 30, Tracks: []canvasdomain.TimelineTrack{{ID: track, Kind: "text", Label: "文本", Visible: true}}, Clips: []canvasdomain.TimelineClip{{ID: uuid.New(), TrackID: track, Kind: "text", Title: "文本", Text: "合成锁证据", DurationMS: 1000, Volume: 1}}, SubtitleStyle: canvasdomain.SubtitleStyle{FontSize: 32, Color: "#ffffff", Position: "bottom"}}
		saved, err := canvas.Execute(ctx, actor, doc.ID, uuid.NewString(), canvasapp.CommandsInput{ExpectedRevision: doc.Revision, Commands: []canvasdomain.Command{{Type: "AddNodes", Nodes: []canvasdomain.Node{{ID: node, Title: "时间线", NodeType: "timeline", NodeAction: "tool", Config: canvasdomain.NodeConfig{Timeline: &config}}}}}})
		if err != nil {
			t.Fatal(err)
		}
		assertLifecycleFence(ctx, t, db, actor, project, func(tx *gorm.DB) error {
			exports := toolpg.NewStore(tx, func(ownerTx *gorm.DB) toolapp.TimelineReader {
				return canvasapp.NewTimelineReader(canvaspg.NewStore(ownerTx, nil))
			}, func(ownerTx *gorm.DB) toolapp.DerivedMedia {
				return mediaapp.NewDerivedService(mediapg.NewStore(ownerTx))
			})
			_, err := exports.Create(ctx, actor, project, uuid.New(), toolapp.CreateInput{CanvasID: doc.ID, NodeID: node, Revision: saved.Revision})
			return err
		})
	})
}
