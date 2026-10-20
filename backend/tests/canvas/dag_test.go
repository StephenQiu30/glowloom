package canvas_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestDAGAnnotationsRejectDuplicateRelationsButAllowVisualCycles(t *testing.T) {
	a, b, c := resourceNode("text"), resourceNode("text"), resourceNode("text")
	edge := domain.Edge{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: a.ID, TargetNodeID: b.ID}
	doc := domain.Document{Nodes: []domain.Node{a, b, c}, Edges: []domain.Edge{edge}}
	duplicate := edge
	duplicate.ID = uuid.New()
	if _, err := domain.Apply(doc, []domain.Command{{Type: "Connect", Edges: []domain.Edge{duplicate}}}); !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatalf("duplicate annotation accepted: %v", err)
	}
	_, err := domain.Apply(doc, []domain.Command{{Type: "Connect", Edges: []domain.Edge{
		{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: b.ID, TargetNodeID: a.ID},
		{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: b.ID, TargetNodeID: c.ID},
		{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: c.ID, TargetNodeID: a.ID},
	}}})
	if err != nil {
		t.Fatalf("visual annotation cycle rejected: %v", err)
	}
}

func TestDAGUnclassifiedHistoryPreventsUnrelatedWrites(t *testing.T) {
	for _, kind := range []string{"input", "reference", "promote", "annotation"} {
		t.Run(kind, func(t *testing.T) {
			a, b := resourceNode("text"), resourceNode("text")
			edge := domain.Edge{ID: uuid.New(), EdgeType: kind, SourceNodeID: a.ID, TargetNodeID: b.ID}
			if kind == "annotation" {
				edge.Binding = json.RawMessage(`{"unknown":"relation"}`)
			}
			doc := domain.Document{Nodes: []domain.Node{a, b}, Edges: []domain.Edge{edge}}
			_, err := domain.Apply(doc, []domain.Command{{Type: "MoveNodes", Moves: []domain.Move{{ID: a.ID, X: 25}}}})
			if !errors.Is(err, domain.ErrUnsupportedCommand) || doc.Nodes[0].X != 0 {
				t.Fatalf("unclassified history was writable: %v", err)
			}
		})
	}
}

func TestDAGUnknownConfigIsPreservedAndCannotBeOverwritten(t *testing.T) {
	for _, raw := range []string{
		`{"text":"keep","future":{"input_node_id":"unknown"}}`,
		`{"director":{"shots":[{"screenshots":[{"future":"keep"}]}]}}`,
		`{"batch_table":{"version":1,"operation":"creative","concurrency":1,"output_count":1,"params":{},"reference_columns":[],"rows":[{"id":"00000000-0000-0000-0000-000000000001","future_binding":"keep"}]}}`,
	} {
		var config domain.NodeConfig
		if err := json.Unmarshal([]byte(raw), &config); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		var want, got any
		if err := json.Unmarshal([]byte(raw), &want); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("unknown persisted config was lost: %s", encoded)
		}
		n := resourceNode("text")
		n.Config = config
		_, err = domain.Apply(domain.Document{Nodes: []domain.Node{n}}, []domain.Command{{Type: "UpdateNodeConfig", ID: n.ID, Config: &domain.NodeConfig{Text: ptr("overwrite")}}})
		if !errors.Is(err, domain.ErrUnsupportedCommand) {
			t.Fatalf("unknown config overwritten: %v", err)
		}
	}
}

func TestDAGValidatesCompleteBatchRatherThanIntermediateReferences(t *testing.T) {
	image := resourceNode("image")
	batch := batchNode(&image.ID)
	doc, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{batch}}, {Type: "AddNodes", Nodes: []domain.Node{image}}})
	if err != nil || len(doc.Nodes) != 2 {
		t.Fatalf("valid final dependency graph rejected: %v", err)
	}
}

func TestDAGDeletingSourceCannotRewriteProtectedConsumers(t *testing.T) {
	for _, kind := range []string{"batch", "director", "timeline"} {
		t.Run(kind, func(t *testing.T) {
			image := resourceNode("image")
			var consumer domain.Node
			switch kind {
			case "batch":
				consumer = batchNode(&image.ID)
			case "director":
				consumer = directorNode()
				o := &consumer.Config.Director.Objects[0]
				o.Kind, o.BuiltinActor, o.SourceNodeID = "billboard", "", &image.ID
			case "timeline":
				consumer = timelineNode()
				consumer.Config.Timeline.Tracks[0].Kind = "image"
				c := &consumer.Config.Timeline.Clips[0]
				c.Kind, c.Text, c.NodeID = "image", "", &image.ID
			}
			consumer.LastOperationID = ptr(uuid.New())
			doc := domain.Document{Nodes: []domain.Node{image, consumer}}
			before, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			_, err = domain.Apply(doc, []domain.Command{{Type: "DeleteNodes", IDs: []uuid.UUID{image.ID}}})
			if !errors.Is(err, domain.ErrUnsupportedCommand) {
				t.Fatalf("protected consumer rewritten: %v", err)
			}
			after, err := json.Marshal(doc)
			if err != nil || string(before) != string(after) {
				t.Fatal("rejected deletion changed the input")
			}
		})
	}
}
