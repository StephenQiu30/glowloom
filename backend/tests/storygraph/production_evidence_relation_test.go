package storygraph_test

import (
	"strings"
	"testing"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestProductionEvidenceRelationsRejectMissingAndWrongEdges(t *testing.T) {
	tests := map[string]func(*storygraph.ProductionOwnerSnapshot){
		"missing evidence edge": func(value *storygraph.ProductionOwnerSnapshot) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeDerivedFrom && edge.ToNodeKey == value.Graph.Nodes[4].StoryNodeKey
			})
		},
		"source bypasses evidence": func(value *storygraph.ProductionOwnerSnapshot) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeDerivedFrom && edge.ToNodeKey == value.Graph.Nodes[1].StoryNodeKey
			})
			value.Graph.Edges = append(value.Graph.Edges, newEdge(
				t, storygraph.EdgeTypeDerivedFrom, value.Graph.Nodes[0].StoryNodeKey,
				value.Graph.Nodes[1].StoryNodeKey, storygraph.EdgeQualifier{},
			))
		},
		"claim edge on fact": func(value *storygraph.ProductionOwnerSnapshot) {
			for index := range value.Graph.Edges {
				edge := &value.Graph.Edges[index]
				if edge.EdgeType == storygraph.EdgeTypeDerivedFrom && edge.ToNodeKey == value.Graph.Nodes[3].StoryNodeKey {
					edge.EdgeType = storygraph.EdgeTypeSupports
					edge.EdgeKey, _ = storygraph.DeriveEdgeKey(edge.EdgeType, edge.FromNodeKey, edge.ToNodeKey, edge.Qualifier)
					return
				}
			}
		},
		"orphan evidence root": func(value *storygraph.ProductionOwnerSnapshot) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeDerivedFrom && edge.ToNodeKey == value.Graph.Nodes[2].StoryNodeKey
			})
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := productionOwnerSnapshotFixture(t)
			mutate(&value)
			if _, err := storygraph.CompileProductionOwnerSnapshot(value); err == nil {
				t.Fatal("invalid production evidence relation was accepted")
			}
		})
	}
}

func TestProductionEvidenceRelationsRejectNonCanonicalEvidenceIdentity(t *testing.T) {
	value := productionOwnerSnapshotFixture(t)
	evidenceNode := &value.Graph.Nodes[2]
	oldKey := evidenceNode.StoryNodeKey
	evidenceNode.OwnerRef.FragmentKey = strings.Replace(evidenceNode.OwnerRef.FragmentKey, ":000000000000:", ":0:", 1)
	evidenceNode.StoryNodeKey = mustNodeKey(t, storygraph.NodeTypeSourceEvidence, evidenceNode.OwnerRef)
	for index := range value.Graph.Edges {
		edge := &value.Graph.Edges[index]
		if edge.FromNodeKey == oldKey {
			edge.FromNodeKey = evidenceNode.StoryNodeKey
		}
		if edge.ToNodeKey == oldKey {
			edge.ToNodeKey = evidenceNode.StoryNodeKey
		}
		edge.EdgeKey, _ = storygraph.DeriveEdgeKey(edge.EdgeType, edge.FromNodeKey, edge.ToNodeKey, edge.Qualifier)
	}
	if _, err := storygraph.CompileProductionOwnerSnapshot(value); err == nil {
		t.Fatal("non-canonical production evidence identity was accepted")
	}
}

func removeProductionEdge(edges []storygraph.Edge, remove func(storygraph.Edge) bool) []storygraph.Edge {
	result := make([]storygraph.Edge, 0, len(edges))
	for _, edge := range edges {
		if !remove(edge) {
			result = append(result, edge)
		}
	}
	return result
}
