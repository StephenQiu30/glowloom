package storygraph_test

import (
	"encoding/json"
	"testing"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestProductionNarrativeClaimRejectsPayloadAndEdgeDrift(t *testing.T) {
	tests := map[string]func(*storygraph.ProductionOwnerSnapshot){
		"missing subject edge": func(value *storygraph.ProductionOwnerSnapshot) {
			claim := productionNarrativeClaimNode(t, value)
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeClaimParticipant && edge.ToNodeKey == claim.StoryNodeKey
			})
		},
		"missing anchor edge": func(value *storygraph.ProductionOwnerSnapshot) {
			claim := productionNarrativeClaimNode(t, value)
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeClaimAnchor && edge.ToNodeKey == claim.StoryNodeKey
			})
		},
		"invalid predicate": func(value *storygraph.ProductionOwnerSnapshot) {
			mutateProductionPayload(t, productionNarrativeClaimNode(t, value), func(payload map[string]any) {
				payload["predicate"] = "Protects"
			})
		},
		"unknown payload field": func(value *storygraph.ProductionOwnerSnapshot) {
			mutateProductionPayload(t, productionNarrativeClaimNode(t, value), func(payload map[string]any) {
				payload["statement"] = "不得复制 Owner 正文"
			})
		},
		"later revision without predecessor": func(value *storygraph.ProductionOwnerSnapshot) {
			mutateProductionPayload(t, productionNarrativeClaimNode(t, value), func(payload map[string]any) {
				payload["claim_revision"] = 2
			})
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := productionNarrativeClaimRelationFixture(t)
			mutate(&value)
			if _, err := storygraph.CompileProductionOwnerSnapshot(value); err == nil {
				t.Fatal("invalid Production Narrative Claim was accepted")
			}
		})
	}
}

func TestProductionNarrativeClaimAcceptsExactOwnerFacts(t *testing.T) {
	for name, nodeType := range map[string]storygraph.NodeType{
		"relationship":  storygraph.NodeTypeRelationshipClaim,
		"foreshadowing": storygraph.NodeTypeForeshadowingClaim,
		"payoff":        storygraph.NodeTypePayoffClaim,
	} {
		t.Run(name, func(t *testing.T) {
			value := productionNarrativeClaimRelationFixture(t)
			claim := productionNarrativeClaimNode(t, &value)
			if nodeType != claim.NodeType {
				oldKey := claim.StoryNodeKey
				claim.NodeType = nodeType
				claim.StoryNodeKey = mustNodeKey(t, nodeType, claim.OwnerRef)
				for index := range value.Graph.Edges {
					edge := &value.Graph.Edges[index]
					if edge.FromNodeKey == oldKey {
						edge.FromNodeKey = claim.StoryNodeKey
					}
					if edge.ToNodeKey == oldKey {
						edge.ToNodeKey = claim.StoryNodeKey
					}
					var err error
					edge.EdgeKey, err = storygraph.DeriveEdgeKey(edge.EdgeType, edge.FromNodeKey, edge.ToNodeKey, edge.Qualifier)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := storygraph.CompileProductionOwnerSnapshot(value); err != nil {
				t.Fatalf("exact Production Narrative Claim was rejected: %v", err)
			}
		})
	}
}

func productionNarrativeClaimRelationFixture(t *testing.T) storygraph.ProductionOwnerSnapshot {
	t.Helper()
	value := productionOwnerSnapshotFixture(t)
	asset := productionNodeByType(t, &value, storygraph.NodeTypeAssetIdentity)
	scene := productionNodeByType(t, &value, storygraph.NodeTypeScene)
	evidence := productionNodeByType(t, &value, storygraph.NodeTypeSourceEvidence)
	claimRef := evidence.OwnerRef
	claimRef.FragmentKey = "claim:relationship"
	claimRef.FragmentContentHash = productionHash(claimRef.FragmentKey)
	payload, err := json.Marshal(map[string]any{
		"payload_contract_id": "storygraph-production/narrative-claim-payload-contract",
		"projection_hash":     claimRef.FragmentContentHash,
		"claim_series_key":    "claim_linzhou_protects_home", "claim_revision": 1,
		"predicate": "protects", "subject_ref": asset.OwnerRef, "object_ref": nil,
		"participant_refs": []storygraph.OwnerRef{}, "anchor_refs": []storygraph.OwnerRef{scene.OwnerRef},
		"valid_scope":      storygraph.ClaimScope{Kind: "scene", OwnerLogicalID: scene.OwnerRef.OwnerLogicalID},
		"story_time_range": nil, "polarity": "positive", "status": "asserted",
		"creator_decision_ref": nil, "supersedes_claim_ref": nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	claim := storygraph.Node{
		StoryNodeKey: mustNodeKey(t, storygraph.NodeTypeRelationshipClaim, claimRef),
		NodeType:     storygraph.NodeTypeRelationshipClaim, OwnerRef: claimRef,
		EvidenceRefs: append([]storygraph.EvidenceRef(nil), asset.EvidenceRefs...), Payload: payload,
	}
	value.Graph.Nodes = append(value.Graph.Nodes, claim)
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypeSupports, evidence.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeClaimParticipant, asset.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{ParticipantRole: "subject"}),
		newEdge(t, storygraph.EdgeTypeClaimAnchor, scene.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{AnchorRole: "scene"}),
	)
	return value
}

func productionNarrativeClaimNode(t *testing.T, value *storygraph.ProductionOwnerSnapshot) *storygraph.Node {
	t.Helper()
	for index := range value.Graph.Nodes {
		if value.Graph.Nodes[index].NodeType == storygraph.NodeTypeRelationshipClaim {
			return &value.Graph.Nodes[index]
		}
	}
	t.Fatal("missing Production Narrative Claim")
	return nil
}
