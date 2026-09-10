package storygraph_test

import (
	"encoding/json"
	"testing"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestProductionStructureRelationsRejectParentAndSequenceDrift(t *testing.T) {
	tests := map[string]func(*storygraph.ProductionOwnerSnapshot){
		"scene loses episode parent": func(value *storygraph.ProductionOwnerSnapshot) {
			second := productionSceneByFragment(t, value, "scene:second")
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeContains && edge.ToNodeKey == second.StoryNodeKey
			})
		},
		"scene has two structural positions": func(value *storygraph.ProductionOwnerSnapshot) {
			episode := productionNodeByType(t, value, storygraph.NodeTypeEpisode)
			second := productionSceneByFragment(t, value, "scene:second")
			value.Graph.Edges = append(value.Graph.Edges, newEdge(t, storygraph.EdgeTypeContains, episode.StoryNodeKey, second.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: "scene:0003"}))
		},
		"scene loses adjacent ordering": func(value *storygraph.ProductionOwnerSnapshot) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypePrecedes && edge.FromNodeKey != edge.ToNodeKey
			})
		},
		"precedes qualifier differs from target position": func(value *storygraph.ProductionOwnerSnapshot) {
			for index := range value.Graph.Edges {
				edge := &value.Graph.Edges[index]
				if edge.EdgeType == storygraph.EdgeTypePrecedes {
					edge.Qualifier.SequenceKey = "scene:9999"
					edge.EdgeKey, _ = storygraph.DeriveEdgeKey(edge.EdgeType, edge.FromNodeKey, edge.ToNodeKey, edge.Qualifier)
					return
				}
			}
		},
		"beat loses adjacent ordering": func(value *storygraph.ProductionOwnerSnapshot) {
			firstBeat := productionNodeByFragment(t, value, storygraph.NodeTypeNarrativeBeat, "beat:first")
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypePrecedes && edge.FromNodeKey == firstBeat.StoryNodeKey
			})
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := productionStructureRelationFixture(t)
			mutate(&value)
			if _, err := storygraph.CompileProductionOwnerSnapshot(value); err == nil {
				t.Fatal("invalid production structure relation was accepted")
			}
		})
	}
}

func TestProductionPrecedesAllowsEpisodeAndNarrativeBeat(t *testing.T) {
	for _, nodeType := range []storygraph.NodeType{storygraph.NodeTypeEpisode, storygraph.NodeTypeNarrativeBeat} {
		if err := storygraph.ValidateEdgeEndpoint(
			storygraph.EdgeTypePrecedes,
			nodeType,
			nodeType,
			storygraph.EdgeQualifier{SequenceKey: "000000000002"},
		); err != nil {
			t.Fatalf("production sequence type %s was rejected: %v", nodeType, err)
		}
	}
}

func productionStructureRelationFixture(t *testing.T) storygraph.ProductionOwnerSnapshot {
	t.Helper()
	value := productionOwnerSnapshotFixture(t)
	episode := productionNodeByType(t, &value, storygraph.NodeTypeEpisode)
	first := productionNodeByType(t, &value, storygraph.NodeTypeScene)
	evidence := productionNodeByType(t, &value, storygraph.NodeTypeSourceEvidence)

	secondRef := first.OwnerRef
	secondRef.FragmentKey = "scene:second"
	secondRef.FragmentContentHash = productionHash(secondRef.FragmentKey)
	secondPayload, err := json.Marshal(map[string]any{
		"payload_contract_id": "storygraph-production/scene-ref-payload-contract",
		"projection_hash":     secondRef.FragmentContentHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	second := storygraph.Node{
		StoryNodeKey: mustNodeKey(t, storygraph.NodeTypeScene, secondRef), NodeType: storygraph.NodeTypeScene,
		OwnerRef: secondRef, EvidenceRefs: append([]storygraph.EvidenceRef(nil), first.EvidenceRefs...), Payload: secondPayload,
	}
	value.Graph.Nodes = append(value.Graph.Nodes, second)
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, second.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeContains, episode.StoryNodeKey, second.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: "scene:0002"}),
		newEdge(t, storygraph.EdgeTypePrecedes, first.StoryNodeKey, second.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: "scene:0002"}),
	)
	beat := func(fragment string) storygraph.Node {
		ref := first.OwnerRef
		ref.FragmentKey = fragment
		ref.FragmentContentHash = productionHash(fragment)
		payload, payloadErr := json.Marshal(map[string]any{
			"payload_contract_id": "storygraph-production/narrative_beat-ref-payload-contract",
			"projection_hash":     ref.FragmentContentHash,
		})
		if payloadErr != nil {
			t.Fatal(payloadErr)
		}
		return storygraph.Node{
			StoryNodeKey: mustNodeKey(t, storygraph.NodeTypeNarrativeBeat, ref), NodeType: storygraph.NodeTypeNarrativeBeat,
			OwnerRef: ref, EvidenceRefs: append([]storygraph.EvidenceRef(nil), first.EvidenceRefs...), Payload: payload,
		}
	}
	firstBeat, secondBeat := beat("beat:first"), beat("beat:second")
	value.Graph.Nodes = append(value.Graph.Nodes, firstBeat, secondBeat)
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, firstBeat.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, secondBeat.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeContains, first.StoryNodeKey, firstBeat.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: "000000000001"}),
		newEdge(t, storygraph.EdgeTypeContains, first.StoryNodeKey, secondBeat.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: "000000000002"}),
		newEdge(t, storygraph.EdgeTypePrecedes, firstBeat.StoryNodeKey, secondBeat.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: "000000000002"}),
	)
	return value
}

func productionSceneByFragment(t *testing.T, value *storygraph.ProductionOwnerSnapshot, fragment string) *storygraph.Node {
	return productionNodeByFragment(t, value, storygraph.NodeTypeScene, fragment)
}

func productionNodeByFragment(t *testing.T, value *storygraph.ProductionOwnerSnapshot, nodeType storygraph.NodeType, fragment string) *storygraph.Node {
	t.Helper()
	for index := range value.Graph.Nodes {
		node := &value.Graph.Nodes[index]
		if node.NodeType == nodeType && node.OwnerRef.FragmentKey == fragment {
			return node
		}
	}
	t.Fatalf("missing production %s fragment %s", nodeType, fragment)
	return nil
}
