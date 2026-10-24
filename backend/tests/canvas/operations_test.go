package canvas_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

func raw(v string) json.RawMessage { return json.RawMessage(v) }
func otDoc() domain.Document {
	return domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Revision: 1, Nodes: []domain.Node{resourceNode("text")}, Edges: []domain.Edge{}}
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
