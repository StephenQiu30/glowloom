package subject_test

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
	canvaspg "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	mediapg "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	promptpg "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/postgres"
	promptapp "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestCanvasSubjectLocalPostgresRevisionAndSources(t *testing.T) {
	runtime, owner := subjectPostgres(t)
	actor := subjectSession(t, owner)
	project := subjectProject(t, owner, actor)
	foreign := subjectProject(t, owner, actor)
	primary := subjectFixtureMedia(t, owner, actor, "image")
	if err := owner.Exec(`UPDATE media.media_asset SET project_id=?,personal_org_id=NULL,personal_actor_id=NULL,object_key=? WHERE id=?`, project, "projects/"+project.String()+"/image/2026/10/"+primary.AssetID.String()+".png", primary.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	store := canvaspg.NewScopedStore(runtime, func(tx *gorm.DB) canvasapp.MediaReader {
		return mediaapp.NewAssetQuery(mediapg.NewScopedStore(tx, mediawire.NewMediaScopeAccess), nil)
	}, mediawire.NewCanvasProjectAccess).WithSubjectSources(subjectwire.NewPromptSources)
	writer := canvasfixture.NewEditService(store)
	doc, err := writer.Create(t.Context(), actor, project, uuid.NewString(), canvasapp.CreateInput{Name: "局部主体"})
	if err != nil {
		t.Fatal(err)
	}
	local := &canvasdomain.SubjectLocal{Description: "局部说明", Primary: &canvasdomain.SubjectMedia{AssetID: primary.AssetID, AssetRevision: 1, Kind: "image"}, References: []canvasdomain.SubjectMedia{}}
	node := canvasdomain.Node{ID: uuid.New(), Title: "主体初名", NodeType: "subject", NodeAction: "resource", Config: canvasdomain.NodeConfig{Subject: &canvasdomain.SubjectConfig{Version: 1, Local: local}}}
	submit := func(want string, operations ...canvasdomain.Operation) {
		t.Helper()
		result, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, Operations: operations})
		if err != nil || result.Status != want {
			t.Fatalf("submit want %s got %+v %v", want, result, err)
		}
	}
	submit("accepted", canvasdomain.Operation{Type: "create_node", Node: &node})
	resolve := func(p uuid.UUID, revision int64) (promptdomain.ReferenceSource, error) {
		var source promptdomain.ReferenceSource
		err := runtime.Transaction(func(tx *gorm.DB) error {
			var e error
			source, e = subjectwire.NewPromptSources(tx).(promptapp.CanvasSubjectSources).ResolveCanvasSubject(t.Context(), actor, p, node.ID.String(), strconv.FormatInt(revision, 10))
			return e
		})
		return source, err
	}
	first, err := resolve(project, 1)
	if err != nil || first.Kind != "canvas_subject" || first.Description != "主体初名：局部说明" || len(first.Media) != 1 || first.Media[0].SourceID != primary.AssetID.String() {
		t.Fatal("fixed local input", first, err)
	}
	validate := func(source promptdomain.ReferenceSource) error {
		return runtime.Transaction(func(tx *gorm.DB) error {
			return promptpg.ValidateDraftSources(tx, actor, project, promptdomain.Draft{References: []promptdomain.Reference{{Source: source}}}, true, subjectwire.NewPromptSources(tx))
		})
	}
	if err := validate(first); err != nil {
		t.Fatal("generation local source", err)
	}
	if _, err := resolve(foreign, 1); err == nil {
		t.Fatal("cross-project source accepted")
	}
	move := canvasdomain.Operation{Type: "set", NodeID: node.ID, Path: []string{"position"}, Value: json.RawMessage(`{"x":34,"y":9}`)}
	submit("accepted", move)
	if _, err := resolve(project, 1); err != nil {
		t.Fatal("layout advanced input", err)
	}
	submit("accepted", canvasdomain.Operation{Type: "set", NodeID: node.ID, Path: []string{"title"}, Value: json.RawMessage(`"主体新名"`)})
	if _, err := resolve(project, 1); err == nil {
		t.Fatal("renamed old input accepted")
	}
	renamed, err := resolve(project, 2)
	if err != nil || renamed.Description != "主体新名：局部说明" {
		t.Fatal("renamed input", renamed, err)
	}
	if err := validate(first); err == nil {
		t.Fatal("stale generation source accepted")
	}
	expected, _ := json.Marshal(map[string]any{"subject": node.Config.Subject})
	exists := true
	local.Description = "修改说明"
	value, _ := json.Marshal(map[string]any{"subject": node.Config.Subject})
	submit("accepted", canvasdomain.Operation{Type: "set", NodeID: node.ID, Path: []string{"config", "subject"}, ExpectedExists: &exists, Expected: expected, Value: value})
	latest, err := resolve(project, 3)
	if err != nil || latest.Description != "主体新名：修改说明" {
		t.Fatal("updated input", latest, err)
	}
	var snapshots int64
	if err := owner.Table("canvas.node_input").Where("node_id=?", node.ID).Count(&snapshots).Error; err != nil || snapshots != 3 {
		t.Fatal("immutable snapshot count", snapshots, err)
	}

	// Undo restores the identity but creates a fresh immutable local input.
	deleteKey := uuid.NewString()
	removed, err := writer.Submit(t.Context(), actor, doc.ID, deleteKey, canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, Operations: []canvasdomain.Operation{{Type: "delete_node", NodeID: node.ID}}})
	if err != nil || removed.Status != "accepted" {
		t.Fatal("remove", removed, err)
	}
	if _, err := resolve(project, 3); err == nil {
		t.Fatal("deleted local source accepted")
	}
	restored, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, UndoOf: deleteKey})
	if err != nil || restored.Status != "accepted" {
		t.Fatal("undo remove", restored, err)
	}
	latest, err = resolve(project, 4)
	if err != nil || latest.Description != "主体新名：修改说明" {
		t.Fatal("restored fixed input", latest, err)
	}
	if _, err := resolve(project, 3); err == nil {
		t.Fatal("undo reused stale local input")
	}
	tampered := latest
	tampered.Description = "客户端替换说明"
	if err := validate(tampered); err == nil {
		t.Fatal("forged source accepted")
	}
	if err := owner.Exec(`UPDATE media.media_asset SET status='rejected',moderation_status='rejected' WHERE id=?`, primary.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	if err := validate(latest); err == nil {
		t.Fatal("revoked local generation accepted")
	}
	move.Value = json.RawMessage(`{"x":35,"y":9}`)
	submit("rejected", move)
}
