package canvas_test

import (
	"encoding/json"
	"errors"
	"testing"

	canvasfixture "github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func TestProjectCopyCanvasPersistsFrozenGraphsAndHidesTarget(t *testing.T) {
	database := canvasDB(t, "GLOWLOOM_TEST_PROJECT_COPY_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	service := canvasfixture.NewService(canvasStore(database))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "复制源画布"})
	if err != nil {
		t.Fatal(err)
	}
	node := resourceNode("text")
	node.Config.Text = ptr("冻结的内容")
	if _, err := service.Edit(t.Context(), actor, doc.ID, uuid.NewString(), application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: canvasfixture.Concat(canvasfixture.Nodes([]domain.Node{node}))}); err != nil {
		t.Fatal(err)
	}
	binding := application.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: project, TargetProjectID: uuid.New()}
	if err := database.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type,status) VALUES(?,?,?,?,'未发布完整复制','16:9','realistic','copying')`, binding.TargetProjectID, actor.OrgID, actor.ID, actor.ScopeID).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot application.ProjectCopySnapshot
	if err := database.Transaction(func(tx *gorm.DB) error {
		var err error
		snapshot, err = pgcanvas.NewScopedProjectCopyStore(tx, nil, nil, mediawire.NewCanvasProjectAccess).Freeze(t.Context(), actor, binding, map[uuid.UUID]uuid.UUID{})
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,owner_scope_id,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot,idem_key,request_sha256,admission_response) VALUES(?,?,?,?,?,1,?,'未发布完整复制','queued','media','{}','{}',gen_random_uuid(),repeat('a',64),'{}')`, binding.JobID, actor.OrgID, actor.ID, actor.ScopeID, project, binding.TargetProjectID).Error
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Edit(t.Context(), actor, doc.ID, uuid.NewString(), application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: canvasfixture.Concat(canvasfixture.TextSource(node.ID, *node.Config.Text, "源随后被编辑"))}); err != nil {
		t.Fatal(err)
	}
	// No referenced media exists in this text-only graph; the reader remains an
	// actual owner query so an unexpected reference would fail closed.
	factory := func(tx *gorm.DB, _ application.ProjectCopyBinding) application.MediaReader {
		return mediaapp.NewAssetQuery(pgmedia.NewScopedStore(tx, mediawire.NewMediaScopeAccess), nil)
	}
	var receipt application.ProjectCopyReceipt
	if err := database.Transaction(func(tx *gorm.DB) error {
		var err error
		receipt, err = pgcanvas.NewScopedProjectCopyStore(tx, nil, factory, mediawire.NewCanvasProjectAccess).Copy(t.Context(), actor, binding, snapshot)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if receipt.Documents != 1 || receipt.ContentSHA256 == "" {
		t.Fatal("missing content receipt", receipt)
	}
	if _, err := service.List(t.Context(), actor, binding.TargetProjectID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("unpublished canvas visible through normal list", err)
	}
	var target struct{ ID uuid.UUID }
	if err := database.Raw(`SELECT id FROM canvas.document WHERE project_id=?`, binding.TargetProjectID).Scan(&target).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(t.Context(), actor, target.ID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("unpublished canvas visible through normal get", err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		replayed, err := pgcanvas.NewScopedProjectCopyStore(tx, nil, factory, mediawire.NewCanvasProjectAccess).Copy(t.Context(), actor, binding, snapshot)
		if err != nil || replayed != receipt {
			return errors.New("persistent receipt mismatch")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Only the maintenance owner can corrupt a snapshot. Even then cleanup must
	// verify its immutable digest before deriving any target identities.
	var frozen struct{ Documents []byte }
	if err := database.Raw(`SELECT documents FROM canvas.project_copy_snapshot WHERE id=?`, snapshot.ID).Scan(&frozen).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE canvas.project_copy_snapshot SET documents=jsonb_set(documents,'{0,name}','"损坏冻结内容"'::jsonb) WHERE id=?`, snapshot.ID).Error; err != nil {
		t.Fatal(err)
	}
	err = database.Transaction(func(tx *gorm.DB) error {
		return pgcanvas.NewScopedProjectCopyStore(tx, nil, factory, mediawire.NewCanvasProjectAccess).Cleanup(t.Context(), actor, binding, snapshot)
	})
	if !errors.Is(err, domain.ErrInvalidProjectCopy) {
		t.Fatal("corrupt snapshot accepted for destructive cleanup", err)
	}
	var liveTargets int
	if err := database.Raw(`SELECT count(*) FROM canvas.document WHERE project_id=? AND NOT is_delete`, binding.TargetProjectID).Scan(&liveTargets).Error; err != nil || liveTargets != 1 {
		t.Fatal("invalid cleanup modified the private target", liveTargets, err)
	}
	if err := database.Exec(`UPDATE canvas.project_copy_snapshot SET documents=?::jsonb WHERE id=?`, string(frozen.Documents), snapshot.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE workspace.project SET status='active' WHERE id=?`, binding.TargetProjectID).Error; err != nil {
		t.Fatal(err)
	}
	fresh, err := canvasfixture.NewService(canvasStore(database)).Get(t.Context(), actor, target.ID)
	if err != nil || fresh.Revision != 1 || len(fresh.Nodes) != 1 || *fresh.Nodes[0].Config.Text != "冻结的内容" || fresh.Nodes[0].ID == node.ID {
		t.Fatal("copied frozen graph not independently refreshable", fresh, err)
	}
}

func TestProjectCopyCanvasRejectsUnknownConfigWithoutPartialSnapshot(t *testing.T) {
	database := canvasDB(t, "GLOWLOOM_TEST_PROJECT_COPY_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	doc, err := canvasfixture.NewService(canvasStore(database)).Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "未知来源配置"})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO canvas.node(id,document_id,title,node_type,node_action,config,x,y) VALUES(?,?,'未知字段','text','resource',?::jsonb,0,0)`, uuid.New(), doc.ID, json.RawMessage(`{"text":"内容","unknown_source_capability":true}`)).Error; err != nil {
		t.Fatal(err)
	}
	binding := application.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: project, TargetProjectID: uuid.New()}
	if err := database.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type,status) VALUES(?,?,?,?,'未发布','16:9','realistic','copying')`, binding.TargetProjectID, actor.OrgID, actor.ID, actor.ScopeID).Error; err != nil {
		t.Fatal(err)
	}
	err = database.Transaction(func(tx *gorm.DB) error {
		_, err := pgcanvas.NewScopedProjectCopyStore(tx, nil, nil, mediawire.NewCanvasProjectAccess).Freeze(t.Context(), actor, binding, map[uuid.UUID]uuid.UUID{})
		return err
	})
	if !errors.Is(err, domain.ErrUnsupportedProjectCopy) {
		t.Fatal("unknown persisted content dropped", err)
	}
	var count int
	if err := database.Raw(`SELECT count(*) FROM canvas.project_copy_snapshot WHERE job_id=?`, binding.JobID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("partial unknown snapshot persisted", count, err)
	}
}
