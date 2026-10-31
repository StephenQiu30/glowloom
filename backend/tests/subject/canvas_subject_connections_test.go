package subject_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	promptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/prompt"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
	canvaspg "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	mediapg "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	promptpg "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/postgres"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	subjectapp "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestCanvasSubjectConnectionPostgresFreezeReadbackAndUndo(t *testing.T) {
	runtime, owner := subjectPostgres(t)
	actor := subjectSession(t, owner)
	project := subjectProject(t, owner, actor)
	foreign := subjectProject(t, owner, actor)
	primary := subjectFixtureMedia(t, owner, actor, "image")
	if err := owner.Exec(`UPDATE media.media_asset SET object_key=? WHERE id=?`, "personal/"+actor.OrgID.String()+"/"+actor.ID.String()+"/image/2026/10/"+primary.AssetID.String()+".png", primary.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	subjects := subjectapp.NewService(subjectFixtureStore(runtime, &memoryMedia{facts: map[uuid.UUID]subject.MediaFact{primary.AssetID: primary}}), time.Now)
	pin, err := subjects.Save(t.Context(), actor, uuid.Nil, uuid.New(), subjectapp.SaveInput{Name: "连线固定旧主体", Primary: subjectapp.MediaSelection{AssetID: primary.AssetID}})
	if err != nil {
		t.Fatal(err)
	}
	localAsset := subjectFixtureMedia(t, owner, actor, "image")
	if err := owner.Exec(`UPDATE media.media_asset SET project_id=?,personal_org_id=NULL,personal_actor_id=NULL,object_key=? WHERE id=?`, project, "projects/"+project.String()+"/image/2026/10/"+localAsset.AssetID.String()+".png", localAsset.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	newStore := func() *canvaspg.Store {
		return canvaspg.NewScopedStore(runtime, func(tx *gorm.DB) canvasapp.MediaReader {
			return mediaapp.NewAssetQuery(mediapg.NewScopedStore(tx, mediawire.NewMediaScopeAccess), nil)
		}, mediawire.NewCanvasProjectAccess).WithSubjectSources(subjectwire.NewPromptSources)
	}
	writer := canvasfixture.NewEditService(newStore())
	doc, err := writer.Create(t.Context(), actor, project, uuid.NewString(), canvasapp.CreateInput{Name: "主体连线接受"})
	if err != nil {
		t.Fatal(err)
	}
	source := canvas.Node{ID: uuid.New(), Title: "主体1", NodeType: "subject", NodeAction: "resource", Config: canvas.NodeConfig{Subject: &canvas.SubjectConfig{Version: 1}}}
	var composer prompt.Draft
	if err := json.Unmarshal([]byte(`{"schemaVersion":1,"document":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"使用主体创作"}]}]},"references":[],"modelVersion":null,"mode":null,"params":{},"labelState":{"highWater":{"image":0,"video":0,"audio":0,"subject":0},"allocations":{}}}`), &composer); err != nil {
		t.Fatal(err)
	}
	target := canvas.Node{ID: uuid.New(), Title: "图片1", NodeType: "image", NodeAction: "resource", Config: canvas.NodeConfig{Editor: &canvas.EditorConfig{SchemaVersion: 1}, Draft: &canvas.Draft{Composer: &composer}}}
	edge := canvas.Edge{ID: uuid.New(), SourceNodeID: source.ID, TargetNodeID: target.ID, EdgeType: "annotation", Binding: json.RawMessage(`{"schemaVersion":1,"fromPortId":"subject","toPortId":"reference_subjects","purpose":"REFERENCE_SUBJECT","order":0,"binding":{"kind":"NODE_VALUE"}}`)}
	submit := func(key string, ops ...canvas.Operation) {
		t.Helper()
		r, err := writer.Submit(t.Context(), actor, doc.ID, key, canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, Operations: ops})
		if err != nil || r.Status != "accepted" {
			t.Fatal("submit", r, err)
		}
	}
	read := func() canvas.Document {
		t.Helper()
		d, err := canvasapp.NewService(newStore()).Get(t.Context(), actor, doc.ID)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	pointer := func(d canvas.Document) prompt.InputSource {
		t.Helper()
		for _, n := range d.Nodes {
			if n.ID == target.ID {
				return prompt.InputSource{Kind: "canvas_input", CanvasID: doc.ID, CanvasRevision: d.Revision, NodeID: target.ID, InputRevision: n.InputRevision}
			}
		}
		t.Fatal("target gone")
		return prompt.InputSource{}
	}
	inputReader := promptpg.NewDraftStore(runtime, promptwire.DraftAccess).WithSubjectSources(subjectwire.NewPromptSources)
	input := func(d canvas.Document) prompt.Draft {
		t.Helper()
		v, err := inputReader.ReadInput(t.Context(), actor, project, pointer(d), false)
		if err != nil {
			t.Fatal("read formal input", err)
		}
		return v
	}
	// A compound creation has exactly one edge but no usable reference.
	submit(uuid.NewString(), canvas.Operation{Type: "create_node", Node: &target}, canvas.Operation{Type: "create_node", Node: &source}, canvas.Operation{Type: "connect", Edge: &edge})
	empty := read()
	if len(empty.Nodes) != 2 || len(empty.Edges) != 1 || len(input(empty).References) != 0 {
		t.Fatal("empty topology lost or executed")
	}
	setSubject := func(before, after *canvas.SubjectConfig) {
		t.Helper()
		expected, _ := json.Marshal(map[string]any{"subject": before})
		value, _ := json.Marshal(map[string]any{"subject": after})
		yes := true
		submit(uuid.NewString(), canvas.Operation{Type: "set", NodeID: source.ID, Path: []string{"config", "subject"}, ExpectedExists: &yes, Expected: expected, Value: value})
	}
	pinned := &canvas.SubjectConfig{Version: 1, SubjectID: &pin.ID, VersionID: &pin.VersionID}
	setSubject(source.Config.Subject, pinned)
	saved := read()
	frozen := input(saved)
	if pointer(saved).InputRevision != 2 || len(frozen.References) != 1 || frozen.References[0].Source.SourceVersion != pin.VersionID.String() || frozen.References[0].Source.Description != "连线固定旧主体" || len(frozen.References[0].Source.Media) != 1 {
		t.Fatal("pin did not compile", frozen)
	}
	if _, err := inputReader.ReadInput(t.Context(), actor, foreign, pointer(saved), false); err == nil {
		t.Fatal("foreign project accepted")
	}
	if _, err := inputReader.ReadInput(t.Context(), actor, project, pointer(empty), true); err == nil {
		t.Fatal("old target input accepted")
	}
	// Library head changes do not silently replace the frozen library version.
	_, err = subjects.Save(t.Context(), actor, pin.ID, uuid.New(), subjectapp.SaveInput{ExpectedRevision: pin.Revision, Name: "库中新头", Primary: subjectapp.MediaSelection{AssetID: primary.AssetID}})
	if err != nil {
		t.Fatal(err)
	}
	submit(uuid.NewString(), canvas.Operation{Type: "set", NodeID: source.ID, Path: []string{"position"}, Value: json.RawMessage(`{"x":9,"y":0}`)})
	moved := read()
	if pointer(moved).InputRevision != 2 || input(moved).References[0].Source.Description != "连线固定旧主体" {
		t.Fatal("head or layout changed frozen pin")
	}
	local := &canvas.SubjectConfig{Version: 1, Local: &canvas.SubjectLocal{Description: "局部连线说明", Primary: &canvas.SubjectMedia{AssetID: localAsset.AssetID, AssetRevision: 1, Kind: "image"}, References: []canvas.SubjectMedia{}}}
	setSubject(pinned, local)
	changed := read()
	ref := input(changed).References[0]
	if pointer(changed).InputRevision != 3 || ref.Source.Kind != "canvas_subject" || ref.Source.SourceVersion != "3" || ref.Source.Media[0].SourceID != localAsset.AssetID.String() {
		t.Fatal("local version not frozen", ref)
	}
	// Copy the accepted selection to another real canvas in the same project.
	destination, err := writer.Create(t.Context(), actor, project, uuid.NewString(), canvasapp.CreateInput{Name: "同项目目标"})
	if err != nil {
		t.Fatal(err)
	}
	var copies []canvas.Node
	body, _ := json.Marshal(changed.Nodes)
	if err := json.Unmarshal(body, &copies); err != nil {
		t.Fatal(err)
	}
	ids := map[uuid.UUID]uuid.UUID{}
	operations := []canvas.Operation{}
	for i := range copies {
		old := copies[i].ID
		copies[i].ID = uuid.New()
		ids[old] = copies[i].ID
		copies[i].InputRevision = 0
		copies[i].ContentVersion = 0
		copies[i].LastOperationID = nil
		copies[i].Title += " 副本"
		operations = append(operations, canvas.Operation{Type: "create_node", Node: &copies[i]})
	}
	copyEdge := changed.Edges[0]
	copyEdge.ID = uuid.New()
	copyEdge.SourceNodeID = ids[source.ID]
	copyEdge.TargetNodeID = ids[target.ID]
	operations = append(operations, canvas.Operation{Type: "connect", Edge: &copyEdge})
	if receipt, err := writer.Submit(t.Context(), actor, destination.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, Operations: operations}); err != nil || receipt.Status != "accepted" {
		t.Fatal("same project copy", receipt, err)
	}
	copiedDoc, err := canvasapp.NewService(newStore()).Get(t.Context(), actor, destination.ID)
	if err != nil || len(copiedDoc.Nodes) != 2 || len(copiedDoc.Edges) != 1 {
		t.Fatal("target readback", copiedDoc, err)
	}
	copyPointer := prompt.InputSource{Kind: "canvas_input", CanvasID: destination.ID, CanvasRevision: copiedDoc.Revision, NodeID: ids[target.ID], InputRevision: 1}
	copyInput, err := inputReader.ReadInput(t.Context(), actor, project, copyPointer, false)
	if err != nil || len(copyInput.References) != 1 || copyInput.References[0].Source.SourceID != ids[source.ID].String() || copyInput.References[0].Source.SourceVersion != "1" || copyInput.References[0].Source.Media[0].SourceID != localAsset.AssetID.String() {
		t.Fatal("copied subject input", copyInput, err)
	}
	if original := read(); original.Revision != changed.Revision || len(original.Nodes) != 2 || original.Nodes[0].ID == ids[target.ID] {
		t.Fatal("copy mutated source", original)
	}
	deleteCopy := uuid.NewString()
	if r, err := writer.Submit(t.Context(), actor, destination.ID, deleteCopy, canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, Operations: []canvas.Operation{{Type: "delete_node", NodeID: ids[source.ID]}, {Type: "delete_node", NodeID: ids[target.ID]}}}); err != nil || r.Status != "accepted" {
		t.Fatal("copy delete", r, err)
	}
	if r, err := writer.Submit(t.Context(), actor, destination.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, UndoOf: deleteCopy}); err != nil || r.Status != "accepted" {
		t.Fatal("copy undo", r, err)
	}
	copiedDoc, err = canvasapp.NewService(newStore()).Get(t.Context(), actor, destination.ID)
	if err != nil || len(copiedDoc.Nodes) != 2 || len(copiedDoc.Edges) != 1 {
		t.Fatal("copy undo readback", copiedDoc, err)
	}
	copyPointer.CanvasRevision = copiedDoc.Revision
	copyPointer.InputRevision = 2
	copyInput, err = inputReader.ReadInput(t.Context(), actor, project, copyPointer, false)
	if err != nil || copyInput.References[0].Source.SourceVersion != "2" {
		t.Fatal("copy undo stale local reference", copyInput, err)
	}
	var snapshot struct{ Draft []byte }
	if err := owner.Raw(`SELECT draft FROM canvas.node_input WHERE node_id=? AND input_revision=3`, target.ID).Scan(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	var archived canvas.Draft
	if json.Unmarshal(snapshot.Draft, &archived) != nil || archived.Composer.References[0].Source.SourceVersion != "3" {
		t.Fatal("immutable input omitted graph source")
	}
	versions, err := newStore().YjsVersions(t.Context(), actor, doc.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var localVersion uuid.UUID
	for _, v := range versions {
		if int64(v.Revision) == changed.Revision {
			localVersion = v.ID
		}
	}
	if localVersion == uuid.Nil {
		t.Fatal("accepted local graph has no history checkpoint")
	}
	key := uuid.NewString()
	submit(key, canvas.Operation{Type: "delete_node", NodeID: source.ID})
	removed := read()
	if len(removed.Edges) != 0 || len(input(removed).References) != 0 || pointer(removed).InputRevision != 4 {
		t.Fatal("deleted source remains executable")
	}
	restored, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, UndoOf: key})
	if err != nil || restored.Status != "accepted" {
		t.Fatal("undo", restored, err)
	}
	again := read()
	if len(again.Edges) != 1 || pointer(again).InputRevision != 5 || input(again).References[0].Source.SourceVersion != "4" {
		t.Fatal("undo failed fresh version freeze", again)
	}
	if _, err := writer.RestoreVersion(t.Context(), actor, doc.ID, localVersion); err != nil {
		t.Fatal("restore accepted graph", err)
	}
	historical := read()
	if len(historical.Edges) != 1 || pointer(historical).InputRevision != 6 || input(historical).References[0].Source.SourceVersion != "5" {
		t.Fatal("history reused target or source revision", historical)
	}
	if _, err := inputReader.ReadInput(t.Context(), actor, project, pointer(again), true); err == nil {
		t.Fatal("pre-restore quote remained executable")
	}
	if err := owner.Raw(`SELECT draft FROM canvas.node_input WHERE node_id=? AND input_revision=6`, target.ID).Scan(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(snapshot.Draft, &archived) != nil || archived.Composer.References[0].Source.SourceVersion != "5" {
		t.Fatal("history failed to freeze effective input")
	}
	if err := owner.Exec(`UPDATE media.media_asset SET status='rejected',moderation_status='rejected' WHERE id=?`, localAsset.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := inputReader.ReadInput(t.Context(), actor, project, pointer(historical), false); err == nil {
		t.Fatal("revoked source quoted")
	}
}
