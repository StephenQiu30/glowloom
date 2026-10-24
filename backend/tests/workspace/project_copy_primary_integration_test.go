package workspace_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/projectcopyoperator"
	canvaspg "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/yjsruntime"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	workspacepg "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func primaryRepairStore(db *gorm.DB) *workspacepg.CopyPrimaryRepairStore {
	return workspacepg.NewCopyPrimaryRepairStore(db, func(tx *gorm.DB) workspaceapp.CopyPrimaryInspector { return canvaspg.NewProjectCopyStore(tx, nil, nil) })
}

func historicalPrimaryCopy(ctx context.Context, t *testing.T, db *gorm.DB) (identityapp.Principal, domain.ProjectCopyJob, []uuid.UUID) {
	t.Helper()
	migration, err := os.ReadFile("../../db/project_copy_primary_repair_incremental.sql")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		lifecycleFixtureSQL(t, db, string(migration))
	}
	actor, source := lifecycleActorProject(ctx, t, db)
	canvas := canvasfixture.NewService(workspaceCanvasStore(db, nil))
	var documents []uuid.UUID
	for range 2 {
		doc, err := canvas.Create(ctx, actor, source, uuid.NewString(), canvasapp.CreateInput{Name: "历史副本画布"})
		if err != nil {
			t.Fatal(err)
		}
		documents = append(documents, doc.ID)
	}
	// The old frozen workspace omitted the primary ID. Keep real owner snapshots,
	// receipts and publication rather than manufacturing a succeeded job row.
	store := projectCopyStore(db)
	job, err := store.Create(ctx, actor, copyInput(source), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	worker := uuid.New()
	if _, err := store.Claim(ctx, actor, job.ID, worker, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteMedia(ctx, actor, job.ID, worker); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteCanvases(ctx, actor, job.ID, worker); err != nil {
		t.Fatal(err)
	}
	job, err = store.Publish(ctx, actor, job.ID, worker)
	if err != nil {
		t.Fatal(err)
	}
	lifecycleFixtureSQL(t, db, `UPDATE workspace.project SET initial_canvas_id=? WHERE id=?`, documents[0], source)
	return actor, job, documents
}

func copyPrimaryEvidence(t *testing.T, db *gorm.DB, job domain.ProjectCopyJob) string {
	t.Helper()
	var value string
	if err := db.Raw(`SELECT jsonb_build_object('job',to_jsonb(j),'snapshot',to_jsonb(s),'receipt',to_jsonb(r))::text FROM workspace.project_copy_job j JOIN canvas.project_copy_snapshot s ON s.job_id=j.id JOIN canvas.project_copy_receipt r ON r.snapshot_id=s.id WHERE j.id=?`, job.ID).Scan(&value).Error; err != nil {
		t.Fatal(err)
	}
	return value
}

func TestHistoricalCopyPrimaryRepairPreservesContentAndReceipts(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, job, docs := historicalPrimaryCopy(ctx, t, db)
	store := primaryRepairStore(db)
	review, err := store.Review(ctx, actor.OrgID, job.ID)
	if err != nil || len(review.Candidates) != 2 || review.FrozenPrimary != uuid.Nil {
		t.Fatal("read-only candidates", review, err)
	}
	var selected canvasapp.ProjectCopyPrimaryCandidate
	for _, candidate := range review.Candidates {
		if candidate.SourceDocumentID == docs[1] {
			selected = candidate
		}
	}
	canvas := canvasfixture.NewService(workspaceCanvasStore(db, nil))
	text := "副本完成后用户继续编辑"
	before, err := canvas.Edit(ctx, actor, selected.TargetDocumentID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: selected.Revision, Operations: canvasfixture.Nodes([]canvasdomain.Node{{ID: uuid.New(), Title: "继续创作", NodeType: "text", NodeAction: "resource", Config: canvasdomain.NodeConfig{Text: &text}}})})
	if err != nil {
		t.Fatal(err)
	}
	input := workspaceapp.CopyPrimaryRepair{OrgID: actor.OrgID, JobID: job.ID, SourceDocumentID: docs[1], ReviewHash: review.ReviewHash, Reason: "验收历史副本缺失主关联"}
	if _, err := store.Apply(ctx, input); err == nil {
		t.Fatal("stale canvas revision accepted")
	}
	review, err = store.Review(ctx, actor.OrgID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	input.ReviewHash = review.ReviewHash
	evidence := copyPrimaryEvidence(t, db, job)
	receipt, err := store.Apply(ctx, input)
	if err != nil || receipt.TargetDocumentID != selected.TargetDocumentID || receipt.AfterRevision != review.ProjectRevision+1 || receipt.OperatorName == "" {
		t.Fatal("explicit recovery", receipt, err)
	}
	again, err := store.Apply(ctx, input)
	if err != nil || !reflect.DeepEqual(receipt, again) {
		t.Fatal("idempotent receipt", again, err)
	}
	if copyPrimaryEvidence(t, db, job) != evidence {
		t.Fatal("immutable copy evidence rewritten")
	}
	project, err := workspaceStore(db).FindProject(ctx, actor, job.TargetProjectID)
	if err != nil || project.CanvasID != selected.TargetDocumentID {
		t.Fatal("normal project entry", project, err)
	}
	after, err := canvas.Get(ctx, actor, project.CanvasID)
	// Primary document display names intentionally follow the project name.
	before.Name = project.Name
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if err != nil || string(a) != string(b) {
		t.Fatalf("content overwritten: %s / %s: %v", a, b, err)
	}
	input.SourceDocumentID = docs[0]
	if _, err := store.Apply(ctx, input); err == nil {
		t.Fatal("conflicting replay accepted")
	}
}

func TestHistoricalCopyPrimaryRepairRejectsInvalidEvidence(t *testing.T) {
	for _, scenario := range []string{"project_revision", "deleted_canvas", "wrong_org", "wrong_source", "missing_receipt", "corrupt_snapshot", "already_bound", "audit_failure", "pending_job", "deleted_project", "owner_receipt", "job_receipt"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, db := workspaceMigrationDB(t)
			actor, job, docs := historicalPrimaryCopy(ctx, t, db)
			store := primaryRepairStore(db)
			review, err := store.Review(ctx, actor.OrgID, job.ID)
			if err != nil {
				t.Fatal(err)
			}
			input := workspaceapp.CopyPrimaryRepair{OrgID: actor.OrgID, JobID: job.ID, SourceDocumentID: docs[0], ReviewHash: review.ReviewHash, Reason: "拒绝不匹配证据"}
			switch scenario {
			case "pending_job":
				queued, err := projectCopyStore(db).Create(ctx, actor, copyInput(job.SourceProjectID), time.Now())
				if err != nil {
					t.Fatal(err)
				}
				input.JobID = queued.ID
			case "deleted_project":
				lifecycleFixtureSQL(t, db, `UPDATE workspace.project SET is_delete=true WHERE id=?`, job.TargetProjectID)
			case "owner_receipt":
				lifecycleFixtureSQL(t, db, `UPDATE canvas.project_copy_receipt SET content_sha256=repeat('a',64) WHERE snapshot_id=?`, job.Manifest.CanvasSnapshotID)
			case "job_receipt":
				lifecycleFixtureSQL(t, db, `UPDATE workspace.project_copy_job SET canvas_receipt=jsonb_set(canvas_receipt,'{content_sha256}',to_jsonb(repeat('a',64))) WHERE id=?`, job.ID)
			case "project_revision":
				lifecycleFixtureSQL(t, db, `UPDATE workspace.project SET revision=revision+1 WHERE id=?`, job.TargetProjectID)
			case "deleted_canvas":
				lifecycleFixtureSQL(t, db, `UPDATE canvas.document SET is_delete=true WHERE id=?`, review.Candidates[0].TargetDocumentID)
			case "wrong_org":
				input.OrgID = uuid.New()
			case "wrong_source":
				input.SourceDocumentID = uuid.New()
			case "missing_receipt":
				lifecycleFixtureSQL(t, db, `DELETE FROM canvas.project_copy_receipt WHERE snapshot_id=?`, job.Manifest.CanvasSnapshotID)
			case "corrupt_snapshot":
				lifecycleFixtureSQL(t, db, `UPDATE canvas.project_copy_snapshot SET documents='[]' WHERE id=?`, job.Manifest.CanvasSnapshotID)
			case "already_bound":
				lifecycleFixtureSQL(t, db, `UPDATE workspace.project SET initial_canvas_id=? WHERE id=?`, review.Candidates[0].TargetDocumentID, job.TargetProjectID)
			case "audit_failure":
				lifecycleFixtureSQL(t, db, `ALTER TABLE workspace.project_copy_primary_repair ADD CONSTRAINT injected_audit_failure CHECK (reason='cannot match') NOT VALID`)
				defer lifecycleFixtureSQL(t, db, `ALTER TABLE workspace.project_copy_primary_repair DROP CONSTRAINT injected_audit_failure`)
			}
			if _, err := store.Apply(ctx, input); err == nil {
				t.Fatal("unsafe repair accepted")
			}
			var count int
			if err := db.Raw(`SELECT count(*) FROM workspace.project_copy_primary_repair WHERE job_id=?`, job.ID).Scan(&count).Error; err != nil || count != 0 {
				t.Fatal("failed repair wrote receipt", count, err)
			}
			if scenario != "already_bound" {
				var missing bool
				if err := db.Raw(`SELECT initial_canvas_id IS NULL FROM workspace.project WHERE id=?`, job.TargetProjectID).Scan(&missing).Error; err != nil || !missing {
					t.Fatal("failed repair changed project", err)
				}
			}
		})
	}
}

func TestCopyPrimaryOperatorRequiresExplicitTargetAndReview(t *testing.T) {
	org, job, source := uuid.NewString(), uuid.NewString(), uuid.NewString()
	valid := []string{"apply", "--org", org, "--job", job, "--source-canvas", source, "--review-hash", strings.Repeat("a", 64), "--reason", "历史修复"}
	if _, err := projectcopyoperator.ParseCommand(valid); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{nil, {"repair"}, {"review", "--org", org}, {"review", "--org", org, "--job", job, "--source-canvas", source}, valid[:7], append(append([]string{}, valid...), "extra")} {
		if _, err := projectcopyoperator.ParseCommand(args); err == nil {
			t.Fatal("ambiguous command accepted", args)
		}
	}
	if _, err := projectcopyoperator.ParseCommand([]string{"review", "--org", org, "--job", job}); err != nil {
		t.Fatal(err)
	}
}

func TestHistoricalCopyPrimaryConcurrentRepairAndRuntimeDenial(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, job, docs := historicalPrimaryCopy(ctx, t, db)
	store := primaryRepairStore(db)
	review, err := store.Review(ctx, actor.OrgID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	input := workspaceapp.CopyPrimaryRepair{OrgID: actor.OrgID, JobID: job.ID, SourceDocumentID: docs[0], ReviewHash: review.ReviewHash, Reason: "并发操作员重试"}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
			return err
		}
		_, err := primaryRepairStore(tx).Apply(ctx, input)
		return err
	}); err == nil {
		t.Fatal("runtime role repaired historical project")
	}
	var wg sync.WaitGroup
	results := make([]workspaceapp.CopyPrimaryRepairReceipt, 2)
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); results[i], errs[i] = store.Apply(ctx, input) }()
	}
	wg.Wait()
	if errs[0] != nil || errs[1] != nil || !reflect.DeepEqual(results[0], results[1]) {
		t.Fatal("concurrent retry", errs, results)
	}
	var count int
	if err := db.Raw(`SELECT count(*) FROM workspace.project_copy_primary_repair WHERE job_id=?`, job.ID).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("duplicate audit", count, err)
	}
}

func TestHistoricalCopyPrimaryRepairPreservesYjsRecovery(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, job, _ := historicalPrimaryCopy(ctx, t, db)
	repair := primaryRepairStore(db)
	review, err := repair.Review(ctx, actor.OrgID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := review.Candidates[0]
	root, err := filepath.Abs("../../../canvas-collab")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, "node", "--input-type=module", "-e", `import {runtimeServer} from './src/server.mjs';const s=runtimeServer({token:'isolated-copy-runtime-token-32-bytes'});s.listen(0,'127.0.0.1',()=>console.log(s.address().port));`)
	command.Dir = root
	pipe, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
	scanner := bufio.NewScanner(pipe)
	if !scanner.Scan() {
		t.Fatal("runtime did not start", stderr.String())
	}
	runtime := yjsruntime.New("http://127.0.0.1:"+scanner.Text(), "isolated-copy-runtime-token-32-bytes")
	canvas := canvasfixture.NewService(workspaceCanvasStore(db, nil))
	node := uuid.New()
	text := "恢复前"
	edited, err := canvas.Edit(ctx, actor, candidate.TargetDocumentID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: candidate.Revision, Operations: canvasfixture.Nodes([]canvasdomain.Node{{ID: node, Title: "保留协作", NodeType: "text", NodeAction: "resource", Config: canvasdomain.NodeConfig{Text: &text}}})})
	if err != nil {
		t.Fatal(err)
	}
	yjs := canvasapp.NewYjsService(workspaceCanvasStore(db, nil), runtime)
	state, err := yjs.Initialize(ctx, actor, candidate.TargetDocumentID, canvasapp.Revision(edited.Revision))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"state": state.State, "nodeId": node, "text": "断线期间待发送"})
	edit := exec.CommandContext(ctx, "node", filepath.Join(root, "tests/service-fixture.mjs"))
	edit.Stdin = bytes.NewReader(body)
	output, err := edit.Output()
	if err != nil {
		t.Fatal(err)
	}
	update := canvasapp.YjsUpdate{Epoch: state.Epoch, SessionID: uuid.New(), UpdateID: uuid.New()}
	if err := json.Unmarshal(output, &update); err != nil {
		t.Fatal(err)
	}
	review, err = repair.Review(ctx, actor.OrgID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	input := workspaceapp.CopyPrimaryRepair{OrgID: actor.OrgID, JobID: job.ID, SourceDocumentID: candidate.SourceDocumentID, ReviewHash: review.ReviewHash, Reason: "协作副本历史入口恢复"}
	if _, err := repair.Apply(ctx, input); err != nil {
		t.Fatal(err)
	}
	loaded, err := yjs.Load(ctx, actor, candidate.TargetDocumentID)
	if err != nil || !bytes.Equal(state.State, loaded.State) || state.YjsBinding != loaded.YjsBinding || state.GraphHash != loaded.GraphHash {
		t.Fatal("repair changed durable Yjs state", err)
	}
	accepted, err := yjs.Submit(ctx, actor, candidate.TargetDocumentID, update)
	if err != nil || accepted.Status != "accepted" {
		t.Fatal("pending update after repaired entry", accepted, err)
	}
	fresh := canvasapp.NewYjsService(workspaceCanvasStore(db, nil), runtime)
	replay, err := fresh.Submit(ctx, actor, candidate.TargetDocumentID, update)
	if err != nil || !reflect.DeepEqual(accepted, replay) {
		t.Fatal("reconnect receipt", replay, err)
	}
	recovered, err := fresh.Load(ctx, actor, candidate.TargetDocumentID)
	projection, marshalErr := json.Marshal(recovered.Document.Nodes)
	if err != nil || marshalErr != nil || recovered.CommitSeq != state.CommitSeq+1 || !strings.Contains(string(projection), "断线期间待发送") {
		t.Fatal("pending content not durable", err)
	}
}

func TestHistoricalCopyPrimaryReviewDoesNotWriteAndArchivedStaysReadOnly(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, job, _ := historicalPrimaryCopy(ctx, t, db)
	lifecycleFixtureSQL(t, db, `UPDATE workspace.project SET status='archived',revision=revision+1 WHERE id=?`, job.TargetProjectID)
	var before, after string
	query := `SELECT to_jsonb(p)::text FROM workspace.project p WHERE id=?`
	if err := db.Raw(query, job.TargetProjectID).Scan(&before).Error; err != nil {
		t.Fatal(err)
	}
	evidence := copyPrimaryEvidence(t, db, job)
	store := primaryRepairStore(db)
	review, err := store.Review(ctx, actor.OrgID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(query, job.TargetProjectID).Scan(&after).Error; err != nil || before != after || evidence != copyPrimaryEvidence(t, db, job) {
		t.Fatal("review wrote data", err)
	}
	input := workspaceapp.CopyPrimaryRepair{OrgID: actor.OrgID, JobID: job.ID, SourceDocumentID: review.Candidates[0].SourceDocumentID, ReviewHash: review.ReviewHash, Reason: "归档历史副本入口恢复"}
	receipt, err := store.Apply(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := canvasfixture.NewService(workspaceCanvasStore(db, nil)).Get(ctx, actor, receipt.TargetDocumentID)
	if err != nil || doc.Writable {
		t.Fatal("repair made archived target writable", err)
	}
}
