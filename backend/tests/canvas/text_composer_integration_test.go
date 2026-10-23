package canvas_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestTextComposerPostgresReopenUndoRedoSourceGuard(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	store := canvasStore(runtime)
	read := application.NewService(store)
	write := application.NewCollaborationService(store)
	doc, err := read.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "Tiptap text references"})
	if err != nil {
		t.Fatal(err)
	}
	submit := func(base int64, key string, ops []domain.Operation, undo string) application.CollaborationReceipt {
		t.Helper()
		r, err := write.Submit(t.Context(), actor, doc.ID, key, application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: base, Operations: ops, UndoOf: undo})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	node := resourceNode("text")
	if r := submit(1, uuid.NewString(), []domain.Operation{{Type: "create_node", Node: &node}}, ""); r.Status != "accepted" {
		t.Fatal(r)
	}
	draft, err := prompt.DecodeDraft([]byte(textComposerJSON))
	if err != nil {
		t.Fatal(err)
	}
	draft.References = nil
	draft.Document.Content[0].Content = draft.Document.Content[0].Content[:1]
	draft.LabelState.Allocations = map[string]prompt.LabelAllocation{}
	assets := []uuid.UUID{}
	for i, kind := range []string{"image", "video", "audio"} {
		asset := seedCanvasMedia(t, owner, project, kind)
		assets = append(assets, asset)
		id := "reference-" + kind
		draft.References = append(draft.References, prompt.Reference{ID: id, Source: prompt.ReferenceSource{Kind: "media", SourceID: asset.String(), SourceVersion: "1"}, BindingGeneration: 1, MediaKind: kind, Role: "reference_" + kind, Order: i})
		draft.LabelState.HighWater[kind] = 3
		draft.LabelState.Allocations[id] = prompt.LabelAllocation{LabelKind: kind, Ordinal: 3}
		draft.Document.Content[0].Content = append(draft.Document.Content[0].Content, prompt.Inline{Type: "reference", Attrs: &prompt.ReferenceAttrs{ReferenceID: id}})
	}
	draft.Document.Content[0].Content = append(draft.Document.Content[0].Content, draft.Document.Content[0].Content[1])
	value, _ := json.Marshal(domain.NodeConfig{Format: "tiptap", FormatVersion: 1, Composer: &draft})
	op := domain.Operation{Type: "set", NodeID: node.ID, Path: []string{"config", "text_content"}, ExpectedExists: ptr(true), Expected: raw(`{"text":"文字","format":"plain","format_version":1}`), Value: value}
	key := uuid.NewString()
	if r := submit(2, key, []domain.Operation{op}, ""); r.Status != "accepted" {
		t.Fatalf("save: %+v", r)
	}
	reopened, err := application.NewService(canvasStore(runtime)).Get(t.Context(), actor, doc.ID)
	if err != nil || len(reopened.Nodes) != 1 || !reflect.DeepEqual(reopened.Nodes[0].Config.Composer, &draft) {
		t.Fatalf("reopen lost references: %+v %v", reopened, err)
	}
	// Replaying an identical request cannot duplicate bindings or labels.
	if r := submit(2, key, []domain.Operation{op}, ""); r.Status != "accepted" {
		t.Fatal(r)
	}
	if r := submit(2, uuid.NewString(), []domain.Operation{op}, ""); r.Status != "rejected" {
		t.Fatal("stale save accepted", r)
	}
	undo := uuid.NewString()
	if r := submit(3, undo, nil, key); r.Status != "accepted" {
		t.Fatal("undo", r)
	}
	if r := submit(4, uuid.NewString(), nil, undo); r.Status != "accepted" {
		t.Fatal("redo", r)
	}
	reopened, err = read.Get(t.Context(), actor, doc.ID)
	if err != nil || !reflect.DeepEqual(reopened.Nodes[0].Config.Composer, &draft) {
		t.Fatal("redo changed bindings or labels", err)
	}
	for _, asset := range assets {
		if found, err := canvasReferenceCheck(t, owner, actor, project, asset); err != nil || !found {
			t.Fatal("reference retention lost", found, err)
		}
	}
	// A foreign source must be rejected even if its shape and source version are valid.
	_, foreignProject := seedCanvasActor(t, owner)
	foreign := seedCanvasMedia(t, owner, foreignProject, "image")
	invalid := draft
	invalid.References = append([]prompt.Reference(nil), draft.References...)
	invalid.References[0].Source.SourceID = foreign.String()
	bad, _ := json.Marshal(domain.NodeConfig{Format: "tiptap", FormatVersion: 1, Composer: &invalid})
	op.Expected = value
	op.Value = bad
	if r := submit(5, uuid.NewString(), []domain.Operation{op}, ""); r.Status != "rejected" {
		t.Fatal("foreign reference accepted", r)
	}
	var inputs int
	if err := runtime.Raw(`SELECT count(*) FROM canvas.node_input WHERE node_id=?`, node.ID).Scan(&inputs).Error; err != nil || inputs != 0 {
		t.Fatal("text node created generation inputs", inputs, err)
	}
	// Removed nodes still retain references for undo and recovery.
	if r := submit(5, uuid.NewString(), []domain.Operation{{Type: "delete_node", NodeID: node.ID}}, ""); r.Status != "accepted" {
		t.Fatal(r)
	}
	if found, err := canvasReferenceCheck(t, owner, actor, project, assets[0]); err != nil || !found {
		t.Fatal("deleted text lost retained references", found, err)
	}
}
