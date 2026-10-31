package subject_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
	canvaspg "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestCanvasSubjectProjectCopyPostgresWorkerProofAndReplay(t *testing.T) {
	runtime, owner := subjectPostgres(t)
	actor := subjectSession(t, owner)
	source, target := subjectProject(t, owner, actor), subjectProject(t, owner, actor)
	if err := owner.Exec(`UPDATE workspace.project SET status='copying' WHERE id=?`, target).Error; err != nil {
		t.Fatal(err)
	}
	primary := subjectFixtureMedia(t, owner, actor, "image")
	if err := owner.Exec(`UPDATE media.media_asset SET object_key=? WHERE id=?`, "personal/"+actor.OrgID.String()+"/"+actor.ID.String()+"/image/2026/10/"+primary.AssetID.String()+".png", primary.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	saved, err := subject.NewService(subjectFixtureStore(runtime, &memoryMedia{facts: map[uuid.UUID]domain.MediaFact{primary.AssetID: primary}}), time.Now).Save(t.Context(), actor, uuid.Nil, uuid.New(), subject.SaveInput{Name: "复制固定主体", Primary: subject.MediaSelection{AssetID: primary.AssetID}})
	if err != nil {
		t.Fatal(err)
	}
	store := canvaspg.NewScopedStore(runtime, nil, mediawire.NewCanvasProjectAccess).WithSubjectSources(subjectwire.NewPromptSources)
	writer := canvasfixture.NewEditService(store)
	doc, err := writer.Create(t.Context(), actor, source, uuid.NewString(), canvas.CreateInput{Name: "主体来源"})
	if err != nil {
		t.Fatal(err)
	}
	node := canvasdomain.Node{ID: uuid.New(), Title: "固定主体", NodeType: "subject", NodeAction: "resource", Config: canvasdomain.NodeConfig{Subject: &canvasdomain.SubjectConfig{Version: 1, SubjectID: &saved.ID, VersionID: &saved.VersionID}}}
	if _, err := writer.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: canvasfixture.Nodes([]canvasdomain.Node{node})}); err != nil {
		t.Fatal(err)
	}
	var composer promptdomain.Draft
	if err := json.Unmarshal([]byte(`{"schemaVersion":1,"document":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"生成固定角色"}]}]},"references":[],"modelVersion":null,"mode":null,"params":{},"labelState":{"highWater":{"image":0,"video":0,"audio":0,"subject":0},"allocations":{}}}`), &composer); err != nil {
		t.Fatal(err)
	}
	generation := canvasdomain.Node{ID: uuid.New(), Title: "待生成图片", NodeType: "image", NodeAction: "resource", Config: canvasdomain.NodeConfig{Editor: &canvasdomain.EditorConfig{SchemaVersion: 1}, Draft: &canvasdomain.Draft{Composer: &composer}}}
	edge := canvasdomain.Edge{ID: uuid.New(), SourceNodeID: node.ID, TargetNodeID: generation.ID, EdgeType: "annotation", Binding: json.RawMessage(`{"schemaVersion":1,"fromPortId":"subject","toPortId":"reference_subjects","purpose":"REFERENCE_SUBJECT","order":0,"binding":{"kind":"NODE_VALUE"}}`)}
	if _, err := writer.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: []canvasdomain.Operation{{Type: "create_node", Node: &generation}, {Type: "connect", Edge: &edge}}}); err != nil {
		t.Fatal(err)
	}
	binding := canvas.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: source, TargetProjectID: target}
	var snapshot canvas.ProjectCopySnapshot
	if err := runtime.Transaction(func(tx *gorm.DB) error {
		_, err := canvaspg.NewScopedProjectCopyStore(tx, nil, nil, mediawire.NewCanvasProjectAccess).Freeze(t.Context(), actor, binding, map[uuid.UUID]uuid.UUID{})
		if err == nil {
			t.Fatal("missing subject copy owner accepted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Transaction(func(tx *gorm.DB) error {
		var err error
		snapshot, err = canvaspg.NewScopedProjectCopyStore(tx, nil, nil, mediawire.NewCanvasProjectAccess).WithSubjectSources(subjectwire.NewCanvasCopySources).Freeze(t.Context(), actor, binding, map[uuid.UUID]uuid.UUID{})
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,owner_scope_id,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot,idem_key,request_sha256,admission_response) VALUES(?,?,?,?,?,1,?,'主体复制目标','queued','media','{}','{}',gen_random_uuid(),repeat('a',64),'{}')`, binding.JobID, actor.OrgID, actor.ID, actor.ScopeID, source, target).Error
	}); err != nil {
		t.Fatal("freeze", err)
	}
	worker := actor
	worker.SessionID = uuid.Nil
	worker.ScopeID = uuid.Nil
	factory := func(*gorm.DB, canvas.ProjectCopyBinding) canvas.MediaReader { return nil }
	runCopy := func() error {
		return runtime.Transaction(func(tx *gorm.DB) error {
			_, err := canvaspg.NewScopedProjectCopyStore(tx, nil, factory, mediawire.NewWorkerCanvasProjectAccess).WithSubjectSources(subjectwire.NewCanvasCopySources).Copy(t.Context(), worker, binding, snapshot)
			return err
		})
	}
	// Recheck before the private target has any canvas effect.
	if err := owner.Exec(`UPDATE subject.subject_head SET deleted_at=clock_timestamp() WHERE id=?`, saved.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := runCopy(); err == nil {
		t.Fatal("deleted fixed version copied")
	}
	var count int
	if err := owner.Raw(`SELECT count(*) FROM canvas.document WHERE project_id=?`, target).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("failed copy leaked target", count, err)
	}
	if err := owner.Exec(`UPDATE subject.subject_head SET deleted_at=NULL WHERE id=?`, saved.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := runCopy(); err != nil {
		t.Fatal("worker copy", err)
	}
	if err := runCopy(); err != nil {
		t.Fatal("idempotent copy replay", err)
	}
	var copied struct {
		ID     uuid.UUID
		Config []byte
	}
	if err := owner.Raw(`SELECT n.id,n.config FROM canvas.node n JOIN canvas.document d ON d.id=n.document_id WHERE d.project_id=? AND n.node_type='subject' AND NOT n.is_delete`, target).Scan(&copied).Error; err != nil {
		t.Fatal(err)
	}
	if copied.ID == node.ID {
		t.Fatal("node identity not remapped")
	}
	var config canvasdomain.NodeConfig
	if err := json.Unmarshal(copied.Config, &config); err != nil || config.Subject == nil || *config.Subject.SubjectID != saved.ID || *config.Subject.VersionID != saved.VersionID {
		t.Fatal("copied pin changed", string(copied.Config), err)
	}
	var frozen struct {
		NodeID        uuid.UUID
		InputRevision int64
		Draft         []byte
		SourceNodeID  uuid.UUID
		TargetNodeID  uuid.UUID
	}
	if err := owner.Raw(`SELECT n.id AS node_id,n.input_revision,i.draft,e.source_node_id,e.target_node_id FROM canvas.node n JOIN canvas.document d ON d.id=n.document_id JOIN canvas.node_input i ON i.node_id=n.id AND i.input_revision=n.input_revision JOIN canvas.edge e ON e.target_node_id=n.id WHERE d.project_id=? AND n.node_type='image'`, target).Scan(&frozen).Error; err != nil {
		t.Fatal(err)
	}
	var input canvasdomain.Draft
	if json.Unmarshal(frozen.Draft, &input) != nil || input.Composer == nil || len(input.Composer.References) != 1 || frozen.InputRevision != 1 || frozen.SourceNodeID != copied.ID || frozen.NodeID == generation.ID || frozen.TargetNodeID != frozen.NodeID || input.Composer.References[0].Source.SourceVersion != saved.VersionID.String() {
		t.Fatal("copied generation input/edge lost", frozen, input)
	}
	if err := owner.Exec(`UPDATE subject.subject_head SET deleted_at=clock_timestamp() WHERE id=?`, saved.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := runCopy(); err == nil {
		t.Fatal("replayed copy skipped current subject proof")
	}
	if err := owner.Exec(`UPDATE subject.subject_head SET deleted_at=NULL WHERE id=?`, saved.ID).Error; err != nil {
		t.Fatal(err)
	}
	// A target in another scope is not authorized merely because its physical org matches.
	other := subjectSession(t, owner)
	teamActor, _ := subjectTeam(t, owner, actor, other)
	if err := owner.Exec(`UPDATE workspace.project SET owner_scope_id=? WHERE id=?`, teamActor.ScopeID, target).Error; err != nil {
		t.Fatal(err)
	}
	if err := runCopy(); err == nil {
		t.Fatal("cross-scope copy accepted")
	}
}
