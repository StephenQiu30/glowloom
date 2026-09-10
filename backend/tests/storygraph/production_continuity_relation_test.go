package storygraph_test

import (
	"encoding/json"
	"testing"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestProductionContinuityRelationsRejectPayloadAndEdgeDrift(t *testing.T) {
	tests := map[string]func(*storygraph.ProductionOwnerSnapshot){
		"claim omits subject": func(value *storygraph.ProductionOwnerSnapshot) {
			mutateProductionPayload(t, productionContinuityNode(t, value), func(payload map[string]any) {
				delete(payload, "subject")
			})
		},
		"claim adds unknown field": func(value *storygraph.ProductionOwnerSnapshot) {
			mutateProductionPayload(t, productionContinuityNode(t, value), func(payload map[string]any) {
				payload["summary"] = "角色状态不变"
			})
		},
		"state persists changes state": func(value *storygraph.ProductionOwnerSnapshot) {
			mutateProductionPayload(t, productionContinuityNode(t, value), func(payload map[string]any) {
				payload["predicate"] = "state_changes"
			})
		},
		"claim loses subject edge": func(value *storygraph.ProductionOwnerSnapshot) {
			claim := productionContinuityNode(t, value)
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeClaimParticipant && edge.ToNodeKey == claim.StoryNodeKey
			})
		},
		"claim swaps state role": func(value *storygraph.ProductionOwnerSnapshot) {
			claim := productionContinuityNode(t, value)
			for index := range value.Graph.Edges {
				edge := &value.Graph.Edges[index]
				if edge.EdgeType == storygraph.EdgeTypeClaimState && edge.ToNodeKey == claim.StoryNodeKey && edge.Qualifier.StateRole == "before" {
					edge.Qualifier.StateRole = "after"
					edge.EdgeKey, _ = storygraph.DeriveEdgeKey(edge.EdgeType, edge.FromNodeKey, edge.ToNodeKey, edge.Qualifier)
					return
				}
			}
		},
		"claim loses end anchor": func(value *storygraph.ProductionOwnerSnapshot) {
			claim := productionContinuityNode(t, value)
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeClaimAnchor && edge.ToNodeKey == claim.StoryNodeKey && edge.Qualifier.AnchorRole == "scope_end"
			})
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := productionContinuityRelationFixture(t)
			mutate(&value)
			if _, err := storygraph.CompileProductionOwnerSnapshot(value); err == nil {
				t.Fatal("invalid production Continuity relation was accepted")
			}
		})
	}
}

func TestProductionContinuityRelationsAcceptExactStateTimeline(t *testing.T) {
	value := productionContinuityRelationFixture(t)
	if _, err := storygraph.CompileProductionOwnerSnapshot(value); err != nil {
		t.Fatalf("exact production Continuity relation was rejected: %v", err)
	}
}

func productionContinuityRelationFixture(t *testing.T) storygraph.ProductionOwnerSnapshot {
	t.Helper()
	value := productionStructureRelationFixture(t)
	asset := productionNodeByType(t, &value, storygraph.NodeTypeAssetIdentity)
	state := productionNodeByType(t, &value, storygraph.NodeTypeAssetState)
	firstScene := productionNodeByType(t, &value, storygraph.NodeTypeScene)
	secondScene := productionSceneByFragment(t, &value, "scene:second")
	episode := productionNodeByType(t, &value, storygraph.NodeTypeEpisode)
	evidence := productionNodeByType(t, &value, storygraph.NodeTypeSourceEvidence)

	claimRef := firstScene.OwnerRef
	claimRef.FragmentKey = "continuity:character-state"
	claimRef.FragmentContentHash = productionHash(claimRef.FragmentKey)
	payload, err := json.Marshal(map[string]any{
		"payload_contract_id":  "storygraph-production/continuity-state-payload-contract",
		"projection_hash":      claimRef.FragmentContentHash,
		"claim_type":           "continuity",
		"claim_series_key":     "continuity_series_character_state",
		"claim_revision":       1,
		"predicate":            "state_persists",
		"subject":              asset.OwnerRef,
		"state_before":         state.OwnerRef,
		"state_after":          state.OwnerRef,
		"anchor_start":         firstScene.OwnerRef,
		"anchor_end":           secondScene.OwnerRef,
		"valid_scope":          storygraph.ClaimScope{Kind: "episode", OwnerLogicalID: episode.OwnerRef.OwnerLogicalID},
		"story_time_start":     "storytime:0001",
		"story_time_end":       "storytime:0002",
		"status":               "asserted",
		"creator_decision_ref": nil,
		"supersedes_claim_ref": nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	claim := storygraph.Node{
		StoryNodeKey: mustNodeKey(t, storygraph.NodeTypeContinuityClaim, claimRef),
		NodeType:     storygraph.NodeTypeContinuityClaim, OwnerRef: claimRef,
		EvidenceRefs: append([]storygraph.EvidenceRef(nil), asset.EvidenceRefs...), Payload: payload,
	}
	value.Graph.Nodes = append(value.Graph.Nodes, claim)
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypeSupports, evidence.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeClaimParticipant, asset.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{ParticipantRole: "subject"}),
		newEdge(t, storygraph.EdgeTypeClaimState, state.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{StateRole: "before"}),
		newEdge(t, storygraph.EdgeTypeClaimState, state.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{StateRole: "after"}),
		newEdge(t, storygraph.EdgeTypeClaimAnchor, firstScene.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{AnchorRole: "scope_start"}),
		newEdge(t, storygraph.EdgeTypeClaimAnchor, secondScene.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{AnchorRole: "scope_end"}),
	)
	return value
}

func productionContinuityNode(t *testing.T, value *storygraph.ProductionOwnerSnapshot) *storygraph.Node {
	t.Helper()
	for index := range value.Graph.Nodes {
		if value.Graph.Nodes[index].NodeType == storygraph.NodeTypeContinuityClaim {
			return &value.Graph.Nodes[index]
		}
	}
	t.Fatal("missing production Continuity Claim")
	return nil
}
