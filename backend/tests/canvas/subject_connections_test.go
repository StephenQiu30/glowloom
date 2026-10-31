package canvas_test

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func subjectConnectionFixture(t *testing.T) (domain.Document, domain.Node, domain.Node) {
	t.Helper()
	source := subjectNode(t, `{"subject":{"version":1}}`)
	source.InputRevision = 1
	target := resourceInputNode(t, "image")
	target.InputRevision = 1
	edge := domain.Edge{ID: uuid.New(), SourceNodeID: source.ID, TargetNodeID: target.ID, EdgeType: "annotation", Binding: raw(`{"schemaVersion":1,"fromPortId":"subject","toPortId":"reference_subjects","purpose":"REFERENCE_SUBJECT","order":0,"binding":{"kind":"NODE_VALUE"}}`)}
	// Target first: upstream revisions must not depend on graph array order.
	return domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Nodes: []domain.Node{target, source}, Edges: []domain.Edge{edge}}, source, target
}
func TestSubjectConnectionEmptyAndClosedPorts(t *testing.T) {
	doc, _, target := subjectConnectionFixture(t)
	if err := domain.ValidateGraph(doc); err != nil {
		t.Fatal("empty edge is valid topology", err)
	}
	input, err := domain.GenerationInput(doc, target)
	if err != nil || len(input.Composer.References) != 0 {
		t.Fatal("empty source became executable", input, err)
	}
	for _, kind := range []string{"image", "video", "audio"} {
		doc.Nodes[0].NodeType = kind
		if err := domain.ValidateGraph(doc); err != nil {
			t.Fatal(kind, err)
		}
	}
	for _, kind := range []string{"text", "subject", "timeline", "director"} {
		doc.Nodes[0].NodeType = kind
		if err := domain.ValidateGraph(doc); err == nil {
			t.Fatal("unsupported target accepted", kind)
		}
	}
	doc, _, _ = subjectConnectionFixture(t)
	duplicate := doc.Edges[0]
	duplicate.ID = uuid.New()
	doc.Edges = append(doc.Edges, duplicate)
	if err := domain.ValidateGraph(doc); err == nil {
		t.Fatal("duplicate subject edge accepted")
	}
}
func TestSubjectConnectionFreezesLocalVersionAndPreservesComposer(t *testing.T) {
	doc, source, target := subjectConnectionFixture(t)
	local := &domain.SubjectLocal{Description: "真实局部说明", Primary: &domain.SubjectMedia{AssetID: uuid.New(), AssetRevision: 3, Kind: "video"}, References: []domain.SubjectMedia{}}
	before, _ := json.Marshal(map[string]any{"subject": source.Config.Subject})
	next := *source.Config.Subject
	next.Local = local
	value, _ := json.Marshal(map[string]any{"subject": next})
	outcome, err := domain.ApplyOperations(doc, []domain.Operation{{Type: "set", NodeID: source.ID, Path: []string{"config", "subject"}, ExpectedExists: ptr(true), Expected: before, Value: value}})
	if err != nil {
		t.Fatal(err)
	}
	got := outcome.Document.Nodes[0]
	if got.InputRevision != 2 || len(got.Config.Draft.Composer.References) != 0 {
		t.Fatal("derived input overwrote composer or failed to advance", got)
	}
	input, err := domain.GenerationInput(outcome.Document, got)
	if err != nil || len(input.Composer.References) != 1 {
		t.Fatal(input, err)
	}
	ref := input.Composer.References[0]
	if ref.Source.Kind != "canvas_subject" || ref.Source.SourceID != source.ID.String() || ref.Source.SourceVersion != strconv.FormatInt(outcome.Document.Nodes[1].InputRevision, 10) || ref.Source.Media[0].SourceVersion != "3" || ref.MediaKind != "video" {
		t.Fatal("wrong frozen source", ref)
	}
	if err := input.Composer.Validate(); err != nil {
		t.Fatal("invalid effective input", err)
	}
	if len(target.Config.Draft.Composer.References) != 0 {
		t.Fatal("mutated source draft")
	}
	moved, err := domain.ApplyOperations(outcome.Document, []domain.Operation{{Type: "set", NodeID: source.ID, Path: []string{"position"}, Value: raw(`{"x":12,"y":0}`)}})
	if err != nil || moved.Document.Nodes[0].InputRevision != 2 || len(moved.Inputs) != 0 {
		t.Fatal("layout advanced generation input", moved, err)
	}
	removed, err := domain.ApplyOperations(outcome.Document, []domain.Operation{{Type: "delete_node", NodeID: source.ID}})
	if err != nil || len(removed.Document.Edges) != 0 || removed.Document.Nodes[0].InputRevision != 3 {
		t.Fatal("removal did not invalidate target", removed, err)
	}
}

func TestSubjectConnectionDeterministicLabelsAndCinemaComposition(t *testing.T) {
	doc, source, target := subjectConnectionFixture(t)
	id, version := uuid.New(), uuid.New()
	doc.Nodes[1].Config.Subject = &domain.SubjectConfig{Version: 1, SubjectID: &id, VersionID: &version}
	other := doc.Nodes[1]
	other.ID = uuid.New()
	other.Config.Subject = &domain.SubjectConfig{Version: 1, Local: &domain.SubjectLocal{Primary: &domain.SubjectMedia{AssetID: uuid.New(), AssetRevision: 1, Kind: "image"}, References: []domain.SubjectMedia{}}}
	second := doc.Edges[0]
	second.ID = uuid.New()
	second.SourceNodeID = other.ID
	second.Binding = raw(`{"schemaVersion":1,"fromPortId":"subject","toPortId":"reference_subjects","purpose":"REFERENCE_SUBJECT","order":1,"binding":{"kind":"NODE_VALUE"}}`)
	_, kit, _ := cinemaFixture(t)
	kitEdge := domain.Edge{ID: uuid.New(), SourceNodeID: kit.ID, TargetNodeID: target.ID, EdgeType: "annotation", Binding: raw(`{"schemaVersion":1,"fromPortId":"camera","toPortId":"camera","purpose":"CINEMA_CAMERA","order":0,"binding":{"kind":"NODE_VALUE"}}`)}
	doc.Nodes = append(doc.Nodes, other, kit)
	doc.Edges = append([]domain.Edge{second, kitEdge}, doc.Edges...)
	if err := domain.ValidateGraph(doc); err != nil {
		t.Fatal(err)
	}
	input, err := domain.GenerationInput(doc, target)
	if err != nil {
		t.Fatal(err)
	}
	refs := input.Composer.References
	if len(refs) != 2 || refs[0].Source.SourceID != id.String() || refs[0].Source.SourceVersion != version.String() || refs[1].Source.SourceID != other.ID.String() || input.Composer.CinemaKit.Camera.FocalLength != "35mm" {
		t.Fatal("source order or cinema lost", input)
	}
	if input.Composer.LabelState.Allocations[refs[0].ID].Ordinal != 1 || input.Composer.LabelState.Allocations[refs[1].ID].Ordinal != 2 {
		t.Fatal("unstable labels", input.Composer.LabelState)
	}
	if len(target.Config.Draft.Composer.References) != 0 || target.Config.Draft.Composer.CinemaKit != nil {
		t.Fatal("mutated editable composer")
	}
	target.Config.Draft.Composer.LabelState.Allocations[refs[0].ID] = input.Composer.LabelState.Allocations[refs[0].ID]
	if _, err := domain.GenerationInput(doc, target); err == nil {
		t.Fatal("editable composer forged graph reference", source)
	}
}

func TestSubjectConnectionReinitializedTargetPreservesInputHighWater(t *testing.T) {
	doc, _, target := subjectConnectionFixture(t)
	draft := target.Config.Draft
	doc.Nodes[0].Config.Draft = nil
	doc.Nodes[0].InputRevision = 7 // A history restore may remove the composer, not its lifetime.
	value, _ := json.Marshal(map[string]any{"draft": draft})
	out, err := domain.ApplyOperations(doc, []domain.Operation{{Type: "set", NodeID: target.ID, Path: []string{"config", "draft"}, ExpectedExists: ptr(true), Expected: raw(`{}`), Value: value}})
	if err != nil || out.Document.Nodes[0].InputRevision != 8 || len(out.Inputs) != 1 || out.Inputs[0].Revision != 8 {
		t.Fatal("reused historical input revision", out, err)
	}
}
