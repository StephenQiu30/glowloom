package canvas_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

func raw(v string) json.RawMessage { return json.RawMessage(v) }
func otDoc() domain.Document {
	return domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Revision: 1, Nodes: []domain.Node{resourceNode("text")}, Edges: []domain.Edge{}}
}
func TestOperationsPreserveIndependentFieldsAndTextIntent(t *testing.T) {
	doc := otDoc()
	id := doc.Nodes[0].ID
	doc.Nodes[0].Config.Text = ptr("AB")
	a := []domain.Operation{{Type: "set", NodeID: id, Path: []string{"position"}, Value: raw(`{"x":20,"y":30}`)}}
	b := []domain.Operation{{Type: "text", NodeID: id, Path: []string{"config", "text"}, Text: domain.SequenceEdit{{Retain: 1}, {Insert: []domain.Atom{{Type: "text", Text: "X"}}}}}}
	ap, bp, err := domain.TransformOperationPair(doc, a, b)
	if err != nil {
		t.Fatal(err)
	}
	da, err := domain.ApplyOperations(doc, a)
	if err != nil {
		t.Fatal(err)
	}
	ab, err := domain.ApplyOperations(da.Document, bp)
	if err != nil {
		t.Fatal(err)
	}
	db, err := domain.ApplyOperations(doc, b)
	if err != nil {
		t.Fatal(err)
	}
	ba, err := domain.ApplyOperations(db.Document, ap)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []domain.Document{ab.Document, ba.Document} {
		if d.Nodes[0].X != 20 || d.Nodes[0].Y != 30 || *d.Nodes[0].Config.Text != "AXB" {
			t.Fatalf("compatible intent lost: %+v", d)
		}
	}
}
func TestOperationsSameFieldAuthorityAndClosedFields(t *testing.T) {
	doc := otDoc()
	id := doc.Nodes[0].ID
	a := []domain.Operation{{Type: "set", NodeID: id, Path: []string{"title"}, Value: raw(`"A"`)}}
	b := []domain.Operation{{Type: "set", NodeID: id, Path: []string{"title"}, Value: raw(`"B"`)}}
	ap, bp, err := domain.TransformOperationPair(doc, a, b)
	if err != nil {
		t.Fatal(err)
	}
	da, _ := domain.ApplyOperations(doc, a)
	ab, err := domain.ApplyOperations(da.Document, bp)
	if err != nil || ab.Document.Nodes[0].Title != "B" {
		t.Fatal("later write lost", err)
	}
	db, _ := domain.ApplyOperations(doc, b)
	ba, err := domain.ApplyOperations(db.Document, ap)
	if err != nil || ba.Document.Nodes[0].Title != "B" {
		t.Fatal("earlier overwrote later", err)
	}
	for _, path := range [][]string{{"id"}, {"input_revision"}, {"last_operation_id"}, {"config"}, {"config", "text"}, {"config", "unknown"}} {
		if _, err := domain.ApplyOperations(doc, []domain.Operation{{Type: "set", NodeID: id, Path: path, Value: raw(`"unsafe"`)}}); err == nil {
			t.Fatalf("untyped/protected field accepted: %v", path)
		}
	}
}
func TestOperationsValidateOriginalBaseBeforeTransform(t *testing.T) {
	doc := otDoc()
	id := doc.Nodes[0].ID
	doc.Nodes[0].Config.Text = ptr("AB")
	a := []domain.Operation{{Type: "text", NodeID: id, Path: []string{"config", "text"}, Text: domain.SequenceEdit{{Insert: []domain.Atom{{Type: "text", Text: "longer"}}}}}}
	b := []domain.Operation{{Type: "text", NodeID: id, Path: []string{"config", "text"}, Text: domain.SequenceEdit{{Retain: 3}}}}
	if _, _, err := domain.TransformOperationPair(doc, a, b); err == nil {
		t.Fatal("invalid old operation became valid after remote insertion")
	}
}
func TestOperationsStableCollectionInsertMoveAndNarrowConflicts(t *testing.T) {
	doc := otDoc()
	table := batchNode(nil)
	aID, bID, cID := uuid.New(), uuid.New(), uuid.New()
	table.Config.BatchTable.ReferenceColumns = []domain.BatchReferenceColumn{{ID: aID, Label: "A"}, {ID: bID, Label: "B"}, {ID: cID, Label: "C"}}
	table.Config.BatchTable.Rows[0].InputNodeIDs = []*uuid.UUID{nil, nil, nil}
	doc.Nodes = []domain.Node{table}
	path := []string{"config", "batch_table", "reference_columns"}
	xID, yID := uuid.New(), uuid.New()
	a := []domain.Operation{{Type: "insert", NodeID: table.ID, Path: path, ItemID: xID.String(), BeforeID: bID.String(), Value: raw(`{"id":"` + xID.String() + `","label":"X"}`)}}
	b := []domain.Operation{{Type: "insert", NodeID: table.ID, Path: path, ItemID: yID.String(), BeforeID: bID.String(), Value: raw(`{"id":"` + yID.String() + `","label":"Y"}`)}}
	ap, bp, err := domain.TransformOperationPair(doc, a, b)
	if err != nil {
		t.Fatal(err)
	}
	da, err := domain.ApplyOperations(doc, a)
	if err != nil {
		t.Fatal(err)
	}
	ab, err := domain.ApplyOperations(da.Document, bp)
	if err != nil {
		t.Fatal(err)
	}
	db, err := domain.ApplyOperations(doc, b)
	if err != nil {
		t.Fatal(err)
	}
	ba, err := domain.ApplyOperations(db.Document, ap)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []domain.Document{ab.Document, ba.Document} {
		cols := d.Nodes[0].Config.BatchTable.ReferenceColumns
		if len(cols) != 5 || cols[1].ID != xID || cols[2].ID != yID || len(d.Nodes[0].Config.BatchTable.Rows[0].InputNodeIDs) != 5 {
			t.Fatalf("stable anchor/slots lost: %+v", d)
		}
	}
	m1 := []domain.Operation{{Type: "move", NodeID: table.ID, Path: path, ItemID: aID.String(), BeforeID: cID.String()}}
	m2 := []domain.Operation{{Type: "move", NodeID: table.ID, Path: path, ItemID: bID.String(), BeforeID: cID.String()}}
	if _, _, err := domain.TransformOperationPair(doc, m1, m2); err == nil {
		t.Fatal("concurrent Move accepted without structural CAS")
	}
	field := []domain.Operation{{Type: "text", NodeID: table.ID, Path: []string{"config", "batch_table", "rows", "@" + table.Config.BatchTable.Rows[0].ID.String(), "prompt"}, Text: domain.SequenceEdit{{Insert: []domain.Atom{{Type: "text", Text: "保留"}}}}}}
	if _, _, err := domain.TransformOperationPair(doc, m1, field); err != nil {
		t.Fatal("Move blocked unrelated row field", err)
	}
}
func TestOperationsNodeDeletionDoesNotResurrectEditsAndCreationIsAtomic(t *testing.T) {
	doc := otDoc()
	id := doc.Nodes[0].ID
	edit := []domain.Operation{{Type: "set", NodeID: id, Path: []string{"title"}, Value: raw(`"edited"`)}}
	remove := []domain.Operation{{Type: "delete_node", NodeID: id}}
	if _, _, err := domain.TransformOperationPair(doc, remove, edit); err == nil {
		t.Fatal("edit after delete accepted")
	}
	if _, _, err := domain.TransformOperationPair(doc, edit, remove); err != nil {
		t.Fatal("later legal deletion rejected", err)
	}
	node := resourceNode("text")
	create := []domain.Operation{{Type: "create_node", Node: &node}, {Type: "set", NodeID: node.ID, Path: []string{"title"}, Value: raw(`"created and edited"`)}}
	out, err := domain.ApplyOperations(doc, create)
	if err != nil || len(out.Document.Nodes) != 2 || out.Document.Nodes[1].Title != "created and edited" {
		t.Fatal("causal creation batch failed", err)
	}
}
func TestOperationsModelContextNarrowParametersAndABA(t *testing.T) {
	doc := otDoc()
	n := generationNode()
	doc.Nodes = []domain.Node{n}
	context := raw(`{"capability":"image.generate","model_profile_id":null,"mode":""}`)
	a := []domain.Operation{{Type: "set", NodeID: n.ID, Path: []string{"config", "generation", "params", "seed"}, Value: raw(`10`), Context: context}}
	b := []domain.Operation{{Type: "set", NodeID: n.ID, Path: []string{"config", "generation", "params", "width"}, Value: raw(`512`), Context: context}}
	ap, bp, err := domain.TransformOperationPair(doc, a, b)
	if err != nil {
		t.Fatal(err)
	}
	x, _ := domain.ApplyOperations(doc, a)
	y, err := domain.ApplyOperations(x.Document, bp)
	if err != nil {
		t.Fatal(err)
	}
	x, _ = domain.ApplyOperations(doc, b)
	z, err := domain.ApplyOperations(x.Document, ap)
	if err != nil {
		t.Fatal(err)
	}
	for _, out := range []domain.OperationOutcome{y, z} {
		if !json.Valid(out.Document.Nodes[0].Config.Generation.Params) || string(out.Document.Nodes[0].Config.Generation.Params) != `{"seed":10,"width":512}` {
			t.Fatal("different parameter keys lost", string(out.Document.Nodes[0].Config.Generation.Params))
		}
	}
	bad := a[0]
	bad.Context = nil
	if _, err := domain.ApplyOperations(doc, []domain.Operation{bad}); err == nil {
		t.Fatal("missing model precondition accepted")
	}
	old := raw(`{"capability":"image.generate","model_profile_id":null,"mode":"","params":{},"output_count":1,"inputs":[]}`)
	next := raw(`{"capability":"image.generate","model_profile_id":null,"mode":"next","params":{},"output_count":1,"inputs":[]}`)
	switches := []domain.Operation{{Type: "set", NodeID: n.ID, Path: []string{"config", "generation", "model"}, Value: next, Expected: old, ExpectedExists: ptr(true)}, {Type: "set", NodeID: n.ID, Path: []string{"config", "generation", "model"}, Value: old, Expected: next, ExpectedExists: ptr(true)}}
	if _, _, err := domain.TransformOperationPair(doc, switches, b); err == nil {
		t.Fatal("ABA model touch was ignored")
	}
}
func TestOperationsFinalInputAdvancesOnlyOnce(t *testing.T) {
	doc := otDoc()
	n := draftNode(t, "image_generation")
	n.InputRevision = 1
	doc.Nodes = []domain.Node{n}
	out, err := domain.ApplyOperations(doc, []domain.Operation{{Type: "text", NodeID: n.ID, Path: []string{"config", "draft", "prompt"}, Text: domain.SequenceEdit{{Insert: []domain.Atom{{Type: "text", Text: "A"}}}}}, {Type: "text", NodeID: n.ID, Path: []string{"config", "draft", "negative_prompt"}, Text: domain.SequenceEdit{{Insert: []domain.Atom{{Type: "text", Text: "B"}}}}}})
	if err != nil || out.Document.Nodes[0].InputRevision != 2 || len(out.Inputs) != 1 || out.Inputs[0].Draft.Prompt != "AFirst" || out.Inputs[0].Draft.NegativePrompt != "B" {
		t.Fatal("intermediate input snapshots persisted", out, err)
	}
}
func TestOperationsColumnSlotsStayBoundThroughReorder(t *testing.T) {
	doc := otDoc()
	image := resourceNode("image")
	table := batchNode(&image.ID)
	column := table.Config.BatchTable.ReferenceColumns[0].ID
	other := uuid.New()
	table.Config.BatchTable.ReferenceColumns = append(table.Config.BatchTable.ReferenceColumns, domain.BatchReferenceColumn{ID: other, Label: "other"})
	table.Config.BatchTable.Rows[0].InputNodeIDs = append(table.Config.BatchTable.Rows[0].InputNodeIDs, nil)
	doc.Nodes = []domain.Node{image, table}
	row := table.Config.BatchTable.Rows[0].ID
	a := []domain.Operation{{Type: "move", NodeID: table.ID, Path: []string{"config", "batch_table", "reference_columns"}, ItemID: other.String(), BeforeID: column.String()}}
	b := []domain.Operation{{Type: "set", NodeID: table.ID, Path: []string{"config", "batch_table", "rows", "@" + row.String(), "input_node_ids", "@" + column.String()}, Value: raw(`null`)}}
	ap, bp, err := domain.TransformOperationPair(doc, a, b)
	if err != nil {
		t.Fatal(err)
	}
	x, _ := domain.ApplyOperations(doc, a)
	y, err := domain.ApplyOperations(x.Document, bp)
	if err != nil {
		t.Fatal(err)
	}
	x, _ = domain.ApplyOperations(doc, b)
	z, err := domain.ApplyOperations(x.Document, ap)
	if err != nil {
		t.Fatal(err)
	}
	for _, out := range []domain.OperationOutcome{y, z} {
		if out.Document.Nodes[1].Config.BatchTable.Rows[0].InputNodeIDs[1] != nil {
			t.Fatal("slot cleared wrong column")
		}
	}
}
func TestOperationsRejectReadOnlyGraphAndPermitDraftDeletion(t *testing.T) {
	doc := otDoc()
	n := draftNode(t, "image_generation")
	n.InputRevision = 1
	doc.Nodes = []domain.Node{n}
	if out, err := domain.ApplyOperations(doc, []domain.Operation{{Type: "delete_node", NodeID: n.ID}}); err != nil || len(out.Document.Nodes) != 0 {
		t.Fatal("editable draft cannot be removed", err)
	}
	doc.Nodes[0].Config.Text = ptr("opaque mixed input")
	if _, err := domain.ApplyOperations(doc, []domain.Operation{{Type: "delete_node", NodeID: n.ID}}); err == nil {
		t.Fatal("read-only graph escaped by deleting incompatible node")
	}
}
func composerNode(t *testing.T) domain.Node {
	n := draftNode(t, "image_generation")
	n.InputRevision = 1
	var draft promptdomain.Draft
	if err := json.Unmarshal([]byte(`{"schemaVersion":1,"document":{"type":"doc","content":[{"type":"paragraph"}]},"references":[],"modelVersion":null,"mode":null,"params":{},"labelState":{"highWater":{"image":0,"video":0,"audio":0,"subject":0},"allocations":{}}}`), &draft); err != nil {
		t.Fatal(err)
	}
	n.Config.Draft = &domain.Draft{Composer: &draft}
	return n
}
func TestOperationsReferenceNumbersAndAtomicMentionCleanup(t *testing.T) {
	doc := otDoc()
	n := composerNode(t)
	doc.Nodes = []domain.Node{n}
	path := []string{"config", "draft", "composer", "references"}
	binding := func(id string) json.RawMessage {
		return raw(`{"id":"` + id + `","source":{"kind":"media","sourceId":"asset-` + id + `","sourceVersion":"v1"},"bindingGeneration":1,"mediaKind":"image","role":"reference_image","order":0}`)
	}
	a := []domain.Operation{{Type: "insert", NodeID: n.ID, Path: path, ItemID: "a", Value: binding("a")}}
	b := []domain.Operation{{Type: "insert", NodeID: n.ID, Path: path, ItemID: "b", Value: binding("b")}}
	ap, bp, err := domain.TransformOperationPair(doc, a, b)
	if err != nil {
		t.Fatal(err)
	}
	x, _ := domain.ApplyOperations(doc, a)
	y, err := domain.ApplyOperations(x.Document, bp)
	if err != nil {
		t.Fatal(err)
	}
	x, _ = domain.ApplyOperations(doc, b)
	z, err := domain.ApplyOperations(x.Document, ap)
	if err != nil {
		t.Fatal(err)
	}
	for _, out := range []domain.OperationOutcome{y, z} {
		d := out.Document.Nodes[0].Config.Draft.Composer
		if d.LabelState.Allocations["a"].Ordinal != 1 || d.LabelState.Allocations["b"].Ordinal != 2 || d.References[1].ID != "b" {
			t.Fatal("canonical label order inconsistent", d)
		}
	}
	mention := []domain.Operation{{Type: "text", NodeID: n.ID, Path: []string{"config", "draft", "composer", "document"}, Text: domain.SequenceEdit{{Insert: []domain.Atom{{Type: "reference", ReferenceID: "a"}, {Type: "text", Text: "中"}, {Type: "reference", ReferenceID: "a"}}}}}}
	x, err = domain.ApplyOperations(y.Document, mention)
	if err != nil {
		t.Fatal(err)
	}
	x, err = domain.ApplyOperations(x.Document, []domain.Operation{{Type: "remove", NodeID: n.ID, Path: path, ItemID: "a"}})
	if err != nil {
		t.Fatal(err)
	}
	d := x.Document.Nodes[0].Config.Draft.Composer
	if len(d.References) != 1 || d.Document.Content[0].Content[0].Text != "中" || len(d.Document.Content[0].Content) != 1 || d.LabelState.Allocations["a"].Ordinal != 1 {
		t.Fatal("mention cleanup or tombstone lost", d)
	}
}
func TestOperationsTimelineLocksAreSharedAndCanBeExplicitlyUnlocked(t *testing.T) {
	doc := otDoc()
	n := timelineNode()
	n.Config.Timeline.Tracks[0].Locked = true
	doc.Nodes = []domain.Node{n}
	clip := n.Config.Timeline.Clips[0].ID
	track := n.Config.Timeline.Tracks[0].ID
	edit := domain.Operation{Type: "text", NodeID: n.ID, Path: []string{"config", "timeline", "clips", "@" + clip.String(), "text"}, Text: domain.SequenceEdit{{Insert: []domain.Atom{{Type: "text", Text: "新"}}}}}
	if _, err := domain.ApplyOperations(doc, []domain.Operation{edit}); err == nil {
		t.Fatal("locked track edited")
	}
	unlock := domain.Operation{Type: "set", NodeID: n.ID, Path: []string{"config", "timeline", "tracks", "@" + track.String(), "locked"}, Value: raw(`false`)}
	if _, err := domain.ApplyOperations(doc, []domain.Operation{unlock, edit}); err != nil {
		t.Fatal("explicit same-batch unlock rejected", err)
	}
}

func TestOperationsRejectInvalidGeometryAndAmbiguousParameters(t *testing.T) {
	doc := otDoc()
	id := doc.Nodes[0].ID
	for _, op := range []domain.Operation{
		{Type: "set", NodeID: id, Path: []string{"position"}, Value: raw(`{"x":1000001,"y":0}`)},
		{Type: "set", NodeID: id, Path: []string{"size"}, Value: raw(`{"width":-1,"height":20}`)},
		{Type: "set", NodeID: id, Path: []string{"title"}, Value: raw(`""`)},
		{Type: "set", NodeID: id, Path: []string{"title"}, Value: raw(`"valid"`), Node: &doc.Nodes[0]},
	} {
		if _, err := domain.ApplyOperations(doc, []domain.Operation{op}); err == nil {
			t.Fatalf("invalid operation accepted: %+v", op)
		}
	}
	node := composerNode(t)
	doc.Nodes = []domain.Node{node}
	// negative_prompt is text even though ordinary model parameter keys are scalar.
	op := domain.Operation{Type: "set", NodeID: node.ID, Path: []string{"config", "draft", "composer", "params", "negative_prompt"}, Value: raw(`"overwrite"`)}
	if _, err := domain.ApplyOperations(doc, []domain.Operation{op}); err == nil {
		t.Fatal("whole negative prompt replacement accepted")
	}
}

func TestOperationEffectsRewindOnlyTouchedData(t *testing.T) {
	doc := otDoc()
	doc.Nodes[0].Config.Text = ptr("AB")
	op := []domain.Operation{{Type: "text", NodeID: doc.Nodes[0].ID, Path: []string{"config", "text"}, Text: domain.SequenceEdit{{Retain: 1}, {Delete: 1}}}}
	out, err := domain.ApplyOperations(doc, op)
	if err != nil {
		t.Fatal(err)
	}
	rewound, err := domain.RewindEffects(out.Document, out.Effects)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(doc)
	after, _ := json.Marshal(rewound)
	if string(before) != string(after) {
		t.Fatalf("cannot reconstruct original: %s", after)
	}
	if len(out.Effects) != 1 || len(out.Effects[0].Path) != 2 {
		t.Fatalf("unexpected full graph image: %+v", out.Effects)
	}
	created := resourceNode("text")
	out, err = domain.ApplyOperations(doc, []domain.Operation{{Type: "create_node", Node: &created}})
	if err != nil {
		t.Fatal(err)
	}
	rewound, err = domain.RewindEffects(out.Document, out.Effects)
	if err != nil || len(rewound.Nodes) != 1 {
		t.Fatal("create rewind failed", err)
	}
}

func TestReferenceRemovalTransformsConcurrentMentionPositions(t *testing.T) {
	n := composerNode(t)
	doc := otDoc()
	doc.Nodes = []domain.Node{n}
	ref := domain.Operation{Type: "insert", NodeID: n.ID, Path: []string{"config", "draft", "composer", "references"}, ItemID: "r", Value: raw(`{"id":"r","source":{"kind":"media","sourceId":"asset-r","sourceVersion":"v1"},"bindingGeneration":1,"mediaKind":"image","role":"reference_image","order":0}`)}
	out, err := domain.ApplyOperations(doc, []domain.Operation{ref})
	if err != nil {
		t.Fatal(err)
	}
	textPath := []string{"config", "draft", "composer", "document"}
	out, err = domain.ApplyOperations(out.Document, []domain.Operation{{Type: "text", NodeID: n.ID, Path: textPath, Text: domain.SequenceEdit{{Insert: []domain.Atom{{Type: "reference", ReferenceID: "r"}, {Type: "text", Text: "AB"}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	doc = out.Document
	a := []domain.Operation{{Type: "remove", NodeID: n.ID, Path: ref.Path, ItemID: "r"}}
	b := []domain.Operation{{Type: "text", NodeID: n.ID, Path: textPath, Text: domain.SequenceEdit{{Retain: 2}, {Insert: []domain.Atom{{Type: "text", Text: "X"}}}}}}
	ap, bp, err := domain.TransformOperationPair(doc, a, b)
	if err != nil {
		t.Fatal(err)
	}
	da, _ := domain.ApplyOperations(doc, a)
	ab, err := domain.ApplyOperations(da.Document, bp)
	if err != nil {
		t.Fatal(err)
	}
	db, _ := domain.ApplyOperations(doc, b)
	ba, err := domain.ApplyOperations(db.Document, ap)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []domain.Document{ab.Document, ba.Document} {
		atoms, err := domain.PromptSequence(d.Nodes[0].Config.Draft.Composer.Document)
		if err != nil || len(atoms) != 1 || atoms[0].Text != "AXB" {
			t.Fatal("mention cleanup lost text intent", atoms, err)
		}
	}
}

func TestOptionalContainerPresenceUsesCASAndCannotRevive(t *testing.T) {
	doc := otDoc()
	n := directorNode()
	n.Config.Director.Ground = nil
	doc.Nodes = []domain.Node{n}
	absent := false
	op := domain.Operation{Type: "set", NodeID: n.ID, Path: []string{"config", "director", "ground"}, ExpectedExists: &absent, Value: raw(`{"ground":{"visible":true,"opacity":1,"height":0}}`)}
	out, err := domain.ApplyOperations(doc, []domain.Operation{op})
	if err != nil {
		t.Fatal("optional creation", err)
	}
	present := true
	cleared := domain.Operation{Type: "set", NodeID: n.ID, Path: op.Path, ExpectedExists: &present, Expected: op.Value, Unset: true}
	out, err = domain.ApplyOperations(out.Document, []domain.Operation{cleared})
	if err != nil || out.Document.Nodes[0].Config.Director.Ground != nil {
		t.Fatal("optional cleared", err)
	}
}

func TestOperationsIndependentCreatesAndDisconnectsCommute(t *testing.T) {
	doc := otDoc()
	a, b := resourceNode("text"), resourceNode("text")
	if _, _, err := domain.TransformOperationPair(doc, []domain.Operation{{Type: "create_node", Node: &a}}, []domain.Operation{{Type: "create_node", Node: &b}}); err != nil {
		t.Fatal("independent creates conflict", err)
	}
	other := resourceNode("text")
	doc.Nodes = append(doc.Nodes, other)
	edge := domain.Edge{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: doc.Nodes[0].ID, TargetNodeID: other.ID}
	doc.Edges = []domain.Edge{edge}
	_, late, err := domain.TransformOperationPair(doc, []domain.Operation{{Type: "disconnect", NodeID: edge.ID}}, []domain.Operation{{Type: "disconnect", NodeID: edge.ID}})
	if err != nil || late[0].Type != "noop" {
		t.Fatal("known same-edge removal did not commute", err)
	}
}

func TestLegacyComposerMigrationPreservesInputAndChecksPremise(t *testing.T) {
	doc := otDoc()
	n := draftNode(t, "image_generation")
	n.InputRevision = 1
	n.Config.Draft.Prompt = "中\n😀"
	n.Config.Draft.NegativePrompt = "不要"
	doc.Nodes = []domain.Node{n}
	expected, _ := json.Marshal(n.Config.Draft)
	migration := domain.Operation{Type: "migrate_composer", NodeID: n.ID, Expected: expected}
	out, err := domain.ApplyOperations(doc, []domain.Operation{migration})
	if err != nil {
		t.Fatal(err)
	}
	d := out.Document.Nodes[0].Config.Draft.Composer
	if d == nil || d.Document.Content[1].Content[0].Text != "😀" || d.Params["negative_prompt"] != "不要" {
		t.Fatal("legacy intent lost", d)
	}
	if _, err := domain.ApplyOperations(out.Document, []domain.Operation{migration}); err == nil {
		t.Fatal("migration overwrote existing rich editor")
	}
	other := []domain.Operation{{Type: "text", NodeID: n.ID, Path: []string{"config", "draft", "prompt"}, Text: domain.SequenceEdit{{Insert: []domain.Atom{{Type: "text", Text: "X"}}}}}}
	if _, _, err := domain.TransformOperationPair(doc, other, []domain.Operation{migration}); err == nil {
		t.Fatal("migration discarded concurrent legacy prompt")
	}
}

func TestUndoDeletedGroupPreservesChildTitle(t *testing.T) {
	group, child := resourceNode("group"), resourceNode("text")
	child.ParentID = &group.ID
	floor := otDoc()
	floor.Nodes = []domain.Node{group, child}
	floor.Revision = 2
	actor := uuid.New()
	ops := [][]domain.Operation{{{Type: "delete_node", NodeID: group.ID}}, {{Type: "set", NodeID: child.ID, Path: []string{"title"}, Value: raw(`"changed"`)}}}
	history := []domain.ContributionBatch{}
	state := floor
	for i, batch := range ops {
		out, err := domain.ApplyOperations(state, batch)
		if err != nil {
			t.Fatal(err)
		}
		history = append(history, domain.ContributionBatch{Revision: int64(3 + i), ActorID: actor, OpID: []string{"delete", "edit"}[i], OriginalBase: state.Revision, Original: batch, Operations: batch, Effects: out.Effects, Touches: out.Changes})
		state = out.Document
		state.Revision = int64(3 + i)
	}
	inverse, err := domain.UndoContribution(floor, history, "delete", actor)
	if err != nil {
		t.Fatal(err)
	}
	out, err := domain.ApplyOperations(state, inverse)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, node := range out.Document.Nodes {
		if node.ID == child.ID {
			found = true
			if node.Title != "changed" || node.ParentID == nil || *node.ParentID != group.ID {
				t.Fatal("title or membership overwritten", node)
			}
		}
	}
	if !found {
		t.Fatal("child lost")
	}
}

func TestReferenceUndoRestoresMentionsWithNewBindingGeneration(t *testing.T) {
	doc := otDoc()
	n := composerNode(t)
	doc.Nodes = []domain.Node{n}
	actor := uuid.New()
	insert := domain.Operation{Type: "insert", NodeID: n.ID, Path: []string{"config", "draft", "composer", "references"}, ItemID: "r", Value: raw(`{"id":"r","source":{"kind":"media","sourceId":"asset-r","sourceVersion":"v1"},"bindingGeneration":1,"mediaKind":"image","role":"reference_image","order":0}`)}
	out, err := domain.ApplyOperations(doc, []domain.Operation{insert, {Type: "text", NodeID: n.ID, Path: []string{"config", "draft", "composer", "document"}, Text: domain.SequenceEdit{{Insert: []domain.Atom{{Type: "reference", ReferenceID: "r"}, {Type: "text", Text: "中"}, {Type: "reference", ReferenceID: "r"}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	doc = out.Document
	remove := domain.Operation{Type: "remove", NodeID: n.ID, Path: insert.Path, ItemID: "r"}
	out, err = domain.ApplyOperations(doc, []domain.Operation{remove})
	if err != nil {
		t.Fatal(err)
	}
	history := []domain.ContributionBatch{{Revision: 2, ActorID: actor, OpID: "remove", OriginalBase: 1, Original: []domain.Operation{remove}, Operations: []domain.Operation{remove}, Effects: out.Effects, Touches: out.Changes}}
	inverse, err := domain.UndoContribution(doc, history, "remove", actor)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := domain.ApplyOperations(out.Document, inverse)
	if err != nil {
		t.Fatal(err)
	}
	rich := restored.Document.Nodes[0].Config.Draft.Composer
	if rich.References[0].BindingGeneration != 2 || len(rich.Document.Content[0].Content) != 3 || rich.LabelState.Allocations["r"].Ordinal != 1 {
		t.Fatal("restoration reused stale binding or lost mentions", rich)
	}
}

func TestSlotUndoFollowsColumnIdentityAfterReordering(t *testing.T) {
	image := resourceNode("image")
	table := batchNode(&image.ID)
	second := uuid.New()
	table.Config.BatchTable.ReferenceColumns = append(table.Config.BatchTable.ReferenceColumns, domain.BatchReferenceColumn{ID: second, Label: "second"})
	table.Config.BatchTable.Rows[0].InputNodeIDs = append(table.Config.BatchTable.Rows[0].InputNodeIDs, nil)
	doc := otDoc()
	doc.Nodes = []domain.Node{image, table}
	first := table.Config.BatchTable.ReferenceColumns[0].ID
	actor := uuid.New()
	ops := [][]domain.Operation{{{Type: "set", NodeID: table.ID, Path: []string{"config", "batch_table", "rows", "@" + table.Config.BatchTable.Rows[0].ID.String(), "input_node_ids", "@" + first.String()}, Value: raw(`null`)}}, {{Type: "move", NodeID: table.ID, Path: []string{"config", "batch_table", "reference_columns"}, ItemID: first.String()}}}
	history := []domain.ContributionBatch{}
	state := doc
	for i, batch := range ops {
		out, err := domain.ApplyOperations(state, batch)
		if err != nil {
			t.Fatal(err)
		}
		history = append(history, domain.ContributionBatch{Revision: int64(2 + i), ActorID: actor, OpID: []string{"slot", "reorder"}[i], OriginalBase: state.Revision, Original: batch, Operations: batch, Effects: out.Effects, Touches: out.Changes})
		state = out.Document
		state.Revision = int64(2 + i)
	}
	inverse, err := domain.UndoContribution(doc, history, "slot", actor)
	if err != nil {
		t.Fatal(err)
	}
	out, err := domain.ApplyOperations(state, inverse)
	if err != nil {
		t.Fatal(err)
	}
	var restored *domain.BatchTableConfig
	for _, n := range out.Document.Nodes {
		if n.ID == table.ID {
			restored = n.Config.BatchTable
		}
	}
	if restored.ReferenceColumns[1].ID != first || restored.Rows[0].InputNodeIDs[0] != nil || restored.Rows[0].InputNodeIDs[1] == nil || *restored.Rows[0].InputNodeIDs[1] != image.ID {
		t.Fatal("undo restored to index instead of column ID", restored)
	}
}

func TestMarkdownSourceFormattingUsesNarrowCAS(t *testing.T) {
	doc := otDoc()
	doc.Nodes[0].Config.Text = ptr("AB")
	exists := true
	format := domain.Operation{Type: "set", NodeID: doc.Nodes[0].ID, Path: []string{"config", "text_source"}, ExpectedExists: &exists, Expected: raw(`{"text":"AB"}`), Value: raw(`{"text":"**AB**"}`)}
	out, err := domain.ApplyOperations(doc, []domain.Operation{format})
	if err != nil || *out.Document.Nodes[0].Config.Text != "**AB**" {
		t.Fatal("format not preserved", err)
	}
	typing := domain.Operation{Type: "text", NodeID: doc.Nodes[0].ID, Path: []string{"config", "text"}, Text: domain.SequenceEdit{{Retain: 1}, {Insert: []domain.Atom{{Type: "text", Text: "X"}}}}}
	if _, _, err := domain.TransformOperationPair(doc, []domain.Operation{typing}, []domain.Operation{format}); err == nil {
		t.Fatal("formatting overwrote concurrent typing")
	}
}

func TestOptionalNegativePromptUsesTextOTFromEmpty(t *testing.T) {
	doc := otDoc()
	n := composerNode(t)
	doc.Nodes = []domain.Node{n}
	path := []string{"config", "draft", "composer", "params", "negative_prompt"}
	a := domain.Operation{Type: "text", NodeID: n.ID, Path: path, Context: raw(`{"modelVersion":null,"mode":null}`), Text: domain.SequenceEdit{{Insert: []domain.Atom{{Type: "text", Text: "不要"}}}}}
	b := a
	b.Text = domain.SequenceEdit{{Insert: []domain.Atom{{Type: "text", Text: "模糊"}}}}
	_, bp, err := domain.TransformOperationPair(doc, []domain.Operation{a}, []domain.Operation{b})
	if err != nil {
		t.Fatal(err)
	}
	out, err := domain.ApplyOperations(doc, []domain.Operation{a})
	if err != nil {
		t.Fatal(err)
	}
	out, err = domain.ApplyOperations(out.Document, bp)
	if err != nil {
		t.Fatal(err)
	}
	if out.Document.Nodes[0].Config.Draft.Composer.Params["negative_prompt"] != "不要模糊" {
		t.Fatal(out.Document)
	}
	cleared := a
	cleared.Text = domain.SequenceEdit{{Delete: 4}}
	out, err = domain.ApplyOperations(out.Document, []domain.Operation{cleared})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := out.Document.Nodes[0].Config.Draft.Composer.Params["negative_prompt"]; exists {
		t.Fatal("empty text did not restore canonical absence")
	}
}

func TestFirstComposerEditUndoKeepsMigrationAndRestoresExactText(t *testing.T) {
	doc := otDoc()
	n := draftNode(t, "image_generation")
	n.InputRevision = 1
	n.Config.Draft.Prompt = "aaa"
	doc.Nodes = []domain.Node{n}
	actor := uuid.New()
	expected, _ := json.Marshal(n.Config.Draft)
	operations := []domain.Operation{{Type: "migrate_composer", NodeID: n.ID, Expected: expected}, {Type: "text", NodeID: n.ID, Path: []string{"config", "draft", "composer", "document"}, Text: domain.SequenceEdit{{Retain: 1}, {Delete: 1}}}}
	out, err := domain.ApplyOperations(doc, operations)
	if err != nil {
		t.Fatal(err)
	}
	history := []domain.ContributionBatch{{Revision: 2, ActorID: actor, OpID: "first", OriginalBase: 1, Original: operations, Operations: operations, Effects: out.Effects, Touches: out.Changes}}
	inverse, err := domain.UndoContribution(doc, history, "first", actor)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := domain.ApplyOperations(out.Document, inverse)
	if err != nil {
		t.Fatal(err)
	}
	d := restored.Document.Nodes[0].Config.Draft
	if d.Composer == nil || d.Composer.Document.Content[0].Content[0].Text != "aaa" {
		t.Fatal("migration or input lost", d)
	}
}

func TestOperationWireRejectsUnknownFieldsWithoutLosingRequestIdentity(t *testing.T) {
	var op domain.Operation
	payload := `{"type":"set","node_id":"` + uuid.NewString() + `","path":["title"],"value":"hello","unrecognized":{"v":1}}`
	if err := json.Unmarshal([]byte(payload), &op); err != nil {
		t.Fatal(err)
	}
	if err := domain.ValidateWireOperations([]domain.Operation{op}); err == nil {
		t.Fatal("unknown field accepted")
	}
	encoded, err := json.Marshal(op)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &fields)
	if fields["unrecognized"] == nil {
		t.Fatal("request hash lost unknown payload")
	}
	op = domain.Operation{Type: "set", NodeID: uuid.New(), Path: []string{"title"}, Value: raw(`"x"`), Unset: true}
	if err := domain.ValidateWireOperations([]domain.Operation{op}); err == nil {
		t.Fatal("ambiguous set accepted")
	}
}
func TestEffectsRejectCorruptEdgePreimage(t *testing.T) {
	doc := otDoc()
	doc.Edges = []domain.Edge{{ID: uuid.New(), SourceNodeID: doc.Nodes[0].ID, TargetNodeID: uuid.New()}}
	bad := doc.Edges[0]
	bad.TargetNodeID = uuid.New()
	encoded, _ := json.Marshal(bad)
	_, err := domain.RewindEffects(doc, []domain.Effect{{Entity: "edge", ID: bad.ID, AfterExists: true, After: encoded, BeforeExists: false}})
	if err == nil {
		t.Fatal("corrupt edge accepted")
	}
}

func TestUndoCreatedNodeRejectsNewTypedConsumer(t *testing.T) {
	doc := otDoc()
	doc.Nodes = nil
	actor := uuid.New()
	media := resourceNode("image")
	table := batchNode(nil)
	table.Config.BatchTable.ReferenceColumns = []domain.BatchReferenceColumn{{ID: uuid.New(), Label: "A"}}
	table.Config.BatchTable.Rows[0].InputNodeIDs = []*uuid.UUID{nil}
	doc.Nodes = []domain.Node{table}
	state := doc
	history := []domain.ContributionBatch{}
	batches := [][]domain.Operation{{{Type: "create_node", Node: &media}}, {{Type: "set", NodeID: table.ID, Path: []string{"config", "batch_table", "rows", "@" + table.Config.BatchTable.Rows[0].ID.String(), "input_node_ids", "@" + table.Config.BatchTable.ReferenceColumns[0].ID.String()}, Value: effectUUID(media.ID)}}}
	for i, ops := range batches {
		out, err := domain.ApplyOperations(state, ops)
		if err != nil {
			t.Fatal(err)
		}
		history = append(history, domain.ContributionBatch{Revision: int64(i + 2), ActorID: actor, OpID: []string{"created", "consumer"}[i], OriginalBase: int64(i + 1), Original: ops, Operations: ops, Effects: out.Effects, Touches: out.Changes})
		state = out.Document
		state.Revision = int64(i + 2)
	}
	if _, err := domain.UndoContribution(doc, history, "created", actor); err == nil {
		t.Fatal("undo erased another contribution's source slot")
	}
}
func effectUUID(id uuid.UUID) json.RawMessage { v, _ := json.Marshal(id); return v }
func TestTransformRejectsExcessivePairWork(t *testing.T) {
	doc := otDoc()
	ops := make([]domain.Operation, 65)
	for i := range ops {
		ops[i] = domain.Operation{Type: "set", NodeID: doc.Nodes[0].ID, Path: []string{"title"}, Value: raw(`"Name"`)}
	}
	if _, _, err := domain.TransformOperationPair(doc, ops, ops); err == nil {
		t.Fatal("unbounded transform work")
	}
}

func TestUndoTypingBatchDoesNotSpendQuadraticBudgetWithoutLaterWork(t *testing.T) {
	doc := otDoc()
	doc.Nodes[0].Config.Text = ptr("")
	ops := make([]domain.Operation, 50)
	for i := range ops {
		edit := domain.SequenceEdit{}
		if i > 0 {
			edit = append(edit, domain.SequenceComponent{Retain: i})
		}
		edit = append(edit, domain.SequenceComponent{Insert: []domain.Atom{{Type: "text", Text: "x"}}})
		ops[i] = domain.Operation{Type: "text", NodeID: doc.Nodes[0].ID, Path: []string{"config", "text"}, Text: edit}
	}

	out, err := domain.ApplyOperations(doc, ops)
	if err != nil {
		t.Fatal(err)
	}
	actor := uuid.New()
	inverse, err := domain.UndoContribution(doc, []domain.ContributionBatch{{Revision: 2, ActorID: actor, OpID: "typed", OriginalBase: 1, Original: ops, Operations: ops, Effects: out.Effects, Touches: out.Changes}}, "typed", actor)
	if err != nil {
		t.Fatal(err)
	}
	back, err := domain.ApplyOperations(out.Document, inverse)
	if err != nil || *back.Document.Nodes[0].Config.Text != "" {
		t.Fatal("typing undo failed", err)
	}
}
func TestOperationsRejectSameBatchIdentityReuse(t *testing.T) {
	doc := otDoc()
	n := resourceNode("text")
	if _, err := domain.ApplyOperations(doc, []domain.Operation{{Type: "create_node", Node: &n}, {Type: "delete_node", NodeID: n.ID}, {Type: "create_node", Node: &n}}); err == nil {
		t.Fatal("node identity reused")
	}
	table := batchNode(nil)
	doc.Nodes = []domain.Node{table}
	id := uuid.NewString()
	path := []string{"config", "batch_table", "rows"}
	v := raw(`{"id":"` + id + `","prompt":"","candidate_count":1,"input_node_ids":[]}`)
	if _, err := domain.ApplyOperations(doc, []domain.Operation{{Type: "insert", NodeID: table.ID, Path: path, ItemID: id, Value: v}, {Type: "remove", NodeID: table.ID, Path: path, ItemID: id}, {Type: "insert", NodeID: table.ID, Path: path, ItemID: id, Value: v}}); err == nil {
		t.Fatal("collection identity reused")
	}
}

func TestTransformBoundedMaximumCanvas(t *testing.T) {
	doc := otDoc()
	doc.Nodes = make([]domain.Node, 2000)
	for i := range doc.Nodes {
		doc.Nodes[i] = resourceNode("text")
		doc.Nodes[i].Config.Text = ptr("AB中文😀")
	}
	a := []domain.Operation{{Type: "text", NodeID: doc.Nodes[0].ID, Path: []string{"config", "text"}, Text: domain.SequenceEdit{{Insert: []domain.Atom{{Type: "text", Text: "X"}}}}}}
	b := []domain.Operation{{Type: "text", NodeID: doc.Nodes[0].ID, Path: []string{"config", "text"}, Text: domain.SequenceEdit{{Retain: 1}, {Delete: 1}}}}
	start := time.Now()
	_, bp, err := domain.TransformOperationPair(doc, a, b)
	if err != nil {
		t.Fatal(err)
	}
	first, err := domain.ApplyOperations(doc, a)
	if err != nil {
		t.Fatal(err)
	}
	last, err := domain.ApplyOperations(first.Document, bp)
	if err != nil || *last.Document.Nodes[0].Config.Text != "XA中文😀" {
		t.Fatal(err)
	}
	body, _ := json.Marshal(doc)
	t.Logf("nodes=2000 edges=0 bytes=%d history=1 pair=1 elapsed=%s", len(body), time.Since(start))
	if err := domain.ValidateTransformWork(doc, 4096); err == nil {
		t.Fatal("oversized estimated copy work accepted")
	}
}
