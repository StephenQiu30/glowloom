package storygraph_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestProductionShotRejectsPayloadAndRelationDrift(t *testing.T) {
	tests := map[string]func(*storygraph.ProductionOwnerSnapshot, []*storygraph.Node){
		"unknown payload field": func(_ *storygraph.ProductionOwnerSnapshot, shots []*storygraph.Node) {
			mutateProductionPayload(t, shots[0], func(payload map[string]any) { payload["camera"] = "latest" })
		},
		"source Beat omitted": func(value *storygraph.ProductionOwnerSnapshot, shots []*storygraph.Node) {
			mutateProductionPayload(t, shots[0], func(payload map[string]any) { payload["source_beat_refs"] = []any{} })
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeRealizes && edge.ToNodeKey == shots[0].StoryNodeKey
			})
		},
		"source Beat duplicated": func(_ *storygraph.ProductionOwnerSnapshot, shots []*storygraph.Node) {
			mutateProductionPayload(t, shots[0], func(payload map[string]any) {
				refs := payload["source_beat_refs"].([]any)
				payload["source_beat_refs"] = append(refs, refs[0])
			})
		},
		"Scene used as source Beat": func(value *storygraph.ProductionOwnerSnapshot, shots []*storygraph.Node) {
			scene := productionNodeByType(t, value, storygraph.NodeTypeScene)
			mutateProductionPayload(t, shots[0], func(payload map[string]any) { payload["source_beat_refs"] = []storygraph.OwnerRef{scene.OwnerRef} })
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeRealizes && edge.ToNodeKey == shots[0].StoryNodeKey
			})
		},
		"realizes edge omitted": func(value *storygraph.ProductionOwnerSnapshot, shots []*storygraph.Node) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeRealizes && edge.ToNodeKey == shots[0].StoryNodeKey
			})
		},
		"Shot order omitted": func(value *storygraph.ProductionOwnerSnapshot, shots []*storygraph.Node) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypePrecedes && edge.FromNodeKey == shots[0].StoryNodeKey && edge.ToNodeKey == shots[1].StoryNodeKey
			})
		},
		"Shot order cycle": func(value *storygraph.ProductionOwnerSnapshot, shots []*storygraph.Node) {
			value.Graph.Edges = append(value.Graph.Edges, newEdge(t, storygraph.EdgeTypePrecedes, shots[1].StoryNodeKey, shots[0].StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: "shot:0001"}))
		},
		"Shot order not increasing": func(value *storygraph.ProductionOwnerSnapshot, shots []*storygraph.Node) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypePrecedes && edge.FromNodeKey == shots[1].StoryNodeKey && edge.ToNodeKey == shots[2].StoryNodeKey
			})
			value.Graph.Edges = append(value.Graph.Edges, newEdge(t, storygraph.EdgeTypePrecedes, shots[1].StoryNodeKey, shots[2].StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: "shot:0001"}))
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value, shots := productionShotFixture(t)
			mutate(&value, shots)
			if _, err := storygraph.Canonicalize(value.Graph); err == nil {
				t.Fatal("invalid Production Shot was accepted")
			}
		})
	}
}

func TestProductionShotAcceptsExactBeatAndOrderRelations(t *testing.T) {
	value, _ := productionShotFixture(t)
	if _, err := storygraph.Canonicalize(value.Graph); err != nil {
		t.Fatalf("exact Production Shots were rejected: %v", err)
	}
}

func productionShotFixture(t *testing.T) (storygraph.ProductionOwnerSnapshot, []*storygraph.Node) {
	t.Helper()
	value, _ := productionInteractionReferenceBindingFixture(t)
	scene := *productionNodeByType(t, &value, storygraph.NodeTypeScene)
	evidence := *productionNodeByType(t, &value, storygraph.NodeTypeSourceEvidence)
	beatRef := scene.OwnerRef
	beatRef.FragmentKey = "beat:shot-source"
	beatRef.FragmentContentHash = productionHash(beatRef.FragmentKey)
	beatPayload, err := json.Marshal(map[string]any{
		"payload_contract_id": "storygraph-production/narrative_beat-ref-payload-contract",
		"projection_hash":     beatRef.FragmentContentHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	beat := storygraph.Node{
		StoryNodeKey: mustNodeKey(t, storygraph.NodeTypeNarrativeBeat, beatRef), NodeType: storygraph.NodeTypeNarrativeBeat,
		OwnerRef: beatRef, EvidenceRefs: append([]storygraph.EvidenceRef(nil), scene.EvidenceRefs...), Payload: beatPayload,
	}
	value.Graph.Nodes = append(value.Graph.Nodes, beat)
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, beat.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeContains, scene.StoryNodeKey, beat.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: "beat:0001"}),
	)
	shots := make([]storygraph.Node, 0, 3)
	formalRef := storygraph.OwnerRef{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, OwnerKind: "production/storyboard",
		VersionFamily: "storyboard_formal_set", OwnerLogicalID: "storyboard:opening", OwnerVersionID: uuid.NewString(),
		OwnerRevision: 1, OwnerContentHash: productionHash("storyboard:opening"),
	}
	for _, fragmentKey := range []string{"shot:opening:0001", "shot:opening:0002", "shot:opening:0003"} {
		ref := formalRef
		ref.FragmentKey = fragmentKey
		ref.FragmentContentHash = productionHash(fragmentKey)
		raw, err := json.Marshal(map[string]any{
			"payload_contract_id": "storygraph-production/shot-payload-contract",
			"projection_hash":     ref.FragmentContentHash,
			"source_beat_refs":    []storygraph.OwnerRef{beat.OwnerRef},
		})
		if err != nil {
			t.Fatal(err)
		}
		shot := storygraph.Node{StoryNodeKey: mustNodeKey(t, storygraph.NodeTypeShot, ref), NodeType: storygraph.NodeTypeShot, OwnerRef: ref, EvidenceRefs: []storygraph.EvidenceRef{}, Payload: raw}
		shots = append(shots, shot)
		value.Graph.Edges = append(value.Graph.Edges, newEdge(t, storygraph.EdgeTypeRealizes, beat.StoryNodeKey, shot.StoryNodeKey, storygraph.EdgeQualifier{}))
	}
	value.Graph.Nodes = append(value.Graph.Nodes, shots...)
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypePrecedes, shots[0].StoryNodeKey, shots[1].StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: "shot:0002"}),
		newEdge(t, storygraph.EdgeTypePrecedes, shots[1].StoryNodeKey, shots[2].StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: "shot:0003"}),
	)
	result := make([]*storygraph.Node, 0, len(shots))
	for index := len(value.Graph.Nodes) - len(shots); index < len(value.Graph.Nodes); index++ {
		result = append(result, &value.Graph.Nodes[index])
	}
	return value, result
}
