package workspace_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	canvasfixture "github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediapg "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	toolpg "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	workspacepg "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

type privateCopyReference struct {
	owner   *mediapg.ProjectCopyStore
	binding mediaapp.ProjectCopyBinding
}

func (r privateCopyReference) Reference(ctx context.Context, actor identityapp.Principal, project, asset uuid.UUID) (mediaapp.AssetSummary, error) {
	if project != r.binding.TargetProjectID {
		return mediaapp.AssetSummary{}, mediaapp.ErrNotFound
	}
	return r.owner.ReferenceCopiedAsset(ctx, actor, r.binding, asset)
}

func projectCopyStore(db *gorm.DB) *workspacepg.ProjectCopyStore {
	return workspaceCopyStore(db, func(tx *gorm.DB) workspaceapp.ProjectWorkGuard {
		return owningProjectWork{workspaceOperationStore(tx), workspaceMediaStore(tx), toolpg.NewStore(tx, nil, nil), toolpg.NewTranscriptionStore(tx, nil, nil), toolpg.NewDepthStore(tx, nil, nil), workspaceCopyStore(tx, nil, nil)}
	}, func(tx *gorm.DB) workspaceapp.ProjectCopyOwners {
		return workspaceapp.ProjectCopyOwners{
			Media: workspaceMediaCopyStore(tx), Budget: workspaceCopyBudgetStore(tx), Cover: actualProjectCopyCoverOwner{tx: tx},
			Canvas: workspaceCanvasCopyStore(tx, func(tx *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(workspaceMediaStore(tx), nil) }, func(tx *gorm.DB, b canvasapp.ProjectCopyBinding) canvasapp.MediaReader {
				return privateCopyReference{owner: workspaceMediaCopyStore(tx), binding: mediaapp.ProjectCopyBinding{JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID}}
			}),
		}
	})
}

func copyInput(id uuid.UUID) workspaceapp.ProjectCopyInput {
	return workspaceapp.ProjectCopyInput{SourceProjectID: id, ExpectedRevision: 1, TargetName: "完整复制项目", IdempotencyKey: uuid.New(), RequestID: uuid.NewString()}
}

func TestProjectCopyAtomicAdmissionCompleteGraphAndPublication(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, project := lifecycleActorProject(ctx, t, db)
	orgPreset, projectPreset := uuid.New(), uuid.New()
	lifecycleFixtureSQL(t, db, `INSERT INTO workspace.style_preset(id,org_id,owner_scope_id,name,style_type,prompt_fragment) VALUES(?,?,?,'组织风格','realistic','组织风格不引用财务')`, orgPreset, actor.OrgID, actor.ScopeID)
	lifecycleFixtureSQL(t, db, `INSERT INTO workspace.style_preset(id,org_id,owner_scope_id,project_id,name,style_type,prompt_fragment,negative_prompt) VALUES(?,?,?,?,'项目风格','realistic','保持角色','不模糊')`, projectPreset, actor.OrgID, actor.ScopeID, project)
	lifecycleFixtureSQL(t, db, `UPDATE workspace.project SET style_preset_id=?,description='完整描述',allow_overseas_models=true,default_models='{"image":"configured-image"}',aigc_mark_style='{"preset":"bottom_right","opacity":0.5}' WHERE id=?`, orgPreset, project)
	canvas := canvasfixture.NewService(workspaceCanvasStore(db, nil))
	document, err := canvas.Create(ctx, actor, project, uuid.NewString(), canvasapp.CreateInput{Name: "独立画布"})
	if err != nil {
		t.Fatal(err)
	}
	lifecycleFixtureSQL(t, db, `UPDATE workspace.project SET initial_canvas_id=? WHERE id=?`, document.ID, project)
	nodeID := uuid.New()
	text := "冻结内容"
	original, err := canvas.Edit(ctx, actor, document.ID, uuid.NewString(), canvasapp.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: canvasfixture.Concat(canvasfixture.Nodes([]canvasdomain.Node{{ID: nodeID, Title: "剧情", NodeType: "text", NodeAction: "resource", Config: canvasdomain.NodeConfig{Text: &text}}}))})
	if err != nil {
		t.Fatal(err)
	}
	input := copyInput(project)
	service := workspaceapp.NewProjectCopyService(projectCopyStore(db), time.Now)
	job, err := service.Create(ctx, actor, input)
	if err != nil || job.Manifest.Documents != 1 || job.Status != "queued" {
		t.Fatal("atomic admission", job, err)
	}
	if _, err := workspaceStore(db).FindProject(ctx, actor, job.TargetProjectID); !errors.Is(err, workspaceapp.ErrProjectNotFound) {
		t.Fatal("copying project visible", err)
	}
	var budget struct{ LimitMicros, ReservedMicros, SettledMicros int64 }
	if err := db.Raw(`SELECT limit_micros,reserved_micros,settled_micros FROM billing.budget WHERE project_id=?`, job.TargetProjectID).Scan(&budget).Error; err != nil || budget.LimitMicros != 0 || budget.ReservedMicros != 0 || budget.SettledMicros != 0 {
		t.Fatal("source budget copied", budget, err)
	}
	fresh := workspaceapp.NewProjectCopyService(projectCopyStore(db), time.Now)
	again, err := fresh.Create(ctx, actor, input)
	if err != nil || !reflect.DeepEqual(job, again) {
		t.Fatal("durable admission replay", err)
	}
	changed := input
	changed.TargetName = "不同副本"
	if _, err := fresh.Create(ctx, actor, changed); !errors.Is(err, workspaceapp.ErrIdempotencyConflict) {
		t.Fatal("key reused with different copy", err)
	}
	worker := uuid.New()
	claimed, err := projectCopyStore(db).Claim(ctx, actor, job.ID, worker, false, time.Now())
	if err != nil || claimed.WorkerID != worker {
		t.Fatal("claim", claimed, err)
	}
	if _, err := projectCopyStore(db).Publish(ctx, actor, job.ID, worker); !errors.Is(err, domain.ErrProjectCopyStateConflict) {
		t.Fatal("partial target published", err)
	}
	if _, err := projectCopyStore(db).CompleteMedia(ctx, actor, job.ID, uuid.New()); !errors.Is(err, domain.ErrProjectCopyWorkerConflict) {
		t.Fatal("stale worker changed stage", err)
	}
	if _, err := projectCopyStore(db).CompleteMedia(ctx, actor, job.ID, worker); err != nil {
		t.Fatal("empty genuine media set", err)
	}
	newText := "源随后编辑"
	if _, err := canvas.Edit(ctx, actor, document.ID, uuid.NewString(), canvasapp.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: canvasfixture.Concat(canvasfixture.TextSource(nodeID, text, newText))}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectCopyStore(db).CompleteCanvases(ctx, actor, job.ID, worker); err != nil {
		t.Fatal("complete frozen graph", err)
	}
	for _, corrupt := range []struct{ name, mutate, restore string }{
		{"primary_canvas", `UPDATE workspace.project SET initial_canvas_id=NULL WHERE id=?`, `UPDATE workspace.project SET initial_canvas_id=(SELECT id FROM canvas.document WHERE project_id=workspace.project.id) WHERE id=?`},
		{"workspace", `UPDATE workspace.project SET description='越过冻结规格' WHERE id=?`, `UPDATE workspace.project SET description='完整描述' WHERE id=?`},
		{"canvas", `UPDATE canvas.node SET config='{"text":"被破坏的目标内容"}'::jsonb WHERE document_id IN(SELECT id FROM canvas.document WHERE project_id=?)`, `UPDATE canvas.node SET config='{"text":"冻结内容"}'::jsonb WHERE document_id IN(SELECT id FROM canvas.document WHERE project_id=?)`},
		{"budget", `UPDATE billing.budget SET limit_micros=1 WHERE project_id=?`, `UPDATE billing.budget SET limit_micros=0 WHERE project_id=?`},
	} {
		t.Run("reject_changed_"+corrupt.name, func(t *testing.T) {
			lifecycleFixtureSQL(t, db, corrupt.mutate, job.TargetProjectID)
			if _, err := projectCopyStore(db).Publish(ctx, actor, job.ID, worker); err == nil {
				t.Fatal("mutated target published from receipt only")
			}
			var status string
			if err := db.Raw(`SELECT status FROM workspace.project WHERE id=?`, job.TargetProjectID).Scan(&status).Error; err != nil || status != "copying" {
				t.Fatal("rejected publication exposed incomplete project", status, err)
			}
			if current, err := projectCopyStore(db).Find(ctx, actor, job.ID); err != nil || current.Status != "running" || current.Stage != "finalizing" || current.WorkerID != worker {
				t.Fatal("rejected publication changed job or fence", current, err)
			}
			lifecycleFixtureSQL(t, db, corrupt.restore, job.TargetProjectID)
		})
	}
	published, err := projectCopyStore(db).Publish(ctx, actor, job.ID, worker)
	if err != nil || published.Status != "succeeded" {
		t.Fatal("atomic publication", published, err)
	}
	settings, err := workspaceapp.NewProjectLifecycle(workspaceStore(db), time.Now).Get(ctx, actor, job.TargetProjectID)
	if err != nil || settings.Project.Name != input.TargetName || settings.Project.Description != "完整描述" || !settings.Project.AllowOverseasModels || settings.DefaultModels["image"] != "configured-image" || settings.Project.StylePresetID == orgPreset {
		t.Fatal("own settings incomplete", settings, err)
	}
	presets, err := workspaceStore(db).ListStylePresets(ctx, actor, &job.TargetProjectID)
	if err != nil || len(presets) != 3 {
		t.Fatal("global context plus copied private styles missing", presets, err)
	}
	docs, err := canvas.List(ctx, actor, job.TargetProjectID)
	if err != nil || len(docs) != 1 {
		t.Fatal("published graph list", docs, err)
	}
	if settings.Project.CanvasID != docs[0].ID {
		t.Fatalf("published copy missing its mapped primary canvas: got %s, want %s", settings.Project.CanvasID, docs[0].ID)
	}
	copied, err := canvas.Get(ctx, actor, docs[0].ID)
	if err != nil || copied.ID == document.ID || copied.Revision != 1 || len(copied.Nodes) != 1 || copied.Nodes[0].ID == nodeID || *copied.Nodes[0].Config.Text != *original.Nodes[0].Config.Text {
		t.Fatal("whole frozen graph not independent", copied, err)
	}
	// Original admission remains stable even after the completed job moved on.
	replayed, err := fresh.Create(ctx, actor, input)
	if err != nil || replayed.Status != "queued" || replayed.ID != job.ID {
		t.Fatal("admission receipt replaced by current state", replayed, err)
	}
}

func TestProjectCopyAdmissionRollbackScopeAndConcurrentIdempotency(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, project := lifecycleActorProject(ctx, t, db)
	other, _ := lifecycleActorProject(ctx, t, db)
	service := workspaceapp.NewProjectCopyService(projectCopyStore(db), time.Now)
	input := copyInput(project)
	if _, err := service.Create(ctx, other, input); !errors.Is(err, workspaceapp.ErrProjectNotFound) {
		t.Fatal("foreign source copied", err)
	}
	stale := input
	stale.ExpectedRevision = 2
	if _, err := service.Create(ctx, actor, stale); !errors.Is(err, domain.ErrProjectRevisionConflict) {
		t.Fatal("stale source copied", err)
	}
	var group sync.WaitGroup
	results := make(chan domain.ProjectCopyJob, 2)
	failures := make(chan error, 2)
	for range 2 {
		group.Go(func() {
			job, err := service.Create(ctx, actor, input)
			if err != nil {
				failures <- err
			} else {
				results <- job
			}
		})
	}
	group.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal("concurrent admission", err)
	}
	var id uuid.UUID
	for result := range results {
		if id != uuid.Nil && result.ID != id {
			t.Fatal("duplicate frozen job")
		}
		id = result.ID
	}
	var count int
	if err := db.Raw(`SELECT count(*) FROM workspace.project_copy_job WHERE source_project_id=?`, project).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("multiple durable jobs", count, err)
	}
}
