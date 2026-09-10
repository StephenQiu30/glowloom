package storygraph_test

import (
	"encoding/json"
	"testing"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestProductionNarrativeClaimRelationsRejectPayloadAndEdgeDrift(t *testing.T) {
	tests := map[string]func(*storygraph.ProductionOwnerSnapshot){
		"claim omits subject": func(value *storygraph.ProductionOwnerSnapshot) {
			mutateProductionPayload(t, productionNarrativeClaimNode(t, value), func(payload map[string]any) {
				delete(payload, "subject_ref")
			})
		},
		"claim copies statement": func(value *storygraph.ProductionOwnerSnapshot) {
			mutateProductionPayload(t, productionNarrativeClaimNode(t, value), func(payload map[string]any) {
				payload["statement"] = "林舟信任同伴"
			})
		},
		"claim publishes uncertain status": func(value *storygraph.ProductionOwnerSnapshot) {
			mutateProductionPayload(t, productionNarrativeClaimNode(t, value), func(payload map[string]any) {
				payload["status"] = "uncertain"
			})
		},
		"claim loses subject edge": func(value *storygraph.ProductionOwnerSnapshot) {
			claim := productionNarrativeClaimNode(t, value)
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeClaimParticipant && edge.ToNodeKey == claim.StoryNodeKey
			})
		},
		"claim loses episode anchor": func(value *storygraph.ProductionOwnerSnapshot) {
			claim := productionNarrativeClaimNode(t, value)
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeClaimAnchor && edge.ToNodeKey == claim.StoryNodeKey
			})
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := productionNarrativeClaimRelationFixture(t)
			mutate(&value)
			if _, err := storygraph.CompileProductionOwnerSnapshot(value); err == nil {
				t.Fatal("invalid production Narrative Claim relation was accepted")
			}
		})
	}
}

func TestProductionNarrativeClaimRelationsAcceptExactRelationship(t *testing.T) {
	value := productionNarrativeClaimRelationFixture(t)
	if _, err := storygraph.CompileProductionOwnerSnapshot(value); err != nil {
		t.Fatalf("exact production Narrative Claim relation was rejected: %v", err)
	}
}

func productionNarrativeClaimRelationFixture(t *testing.T) storygraph.ProductionOwnerSnapshot {
	t.Helper()
	value := productionOwnerSnapshotFixture(t)
	asset := productionNodeByType(t, &value, storygraph.NodeTypeAssetIdentity)
	episode := productionNodeByType(t, &value, storygraph.NodeTypeEpisode)
	evidence := productionNodeByType(t, &value, storygraph.NodeTypeSourceEvidence)
	claimRef := evidence.OwnerRef
	claimRef.FragmentKey = "claim:relationship-linzhou"
	claimRef.FragmentContentHash = productionHash(claimRef.FragmentKey)
	payload, err := json.Marshal(map[string]any{
		"payload_contract_id":  "storygraph-production/narrative-claim-payload-contract",
		"projection_hash":      claimRef.FragmentContentHash,
		"claim_series_key":     "claim_relationship_linzhou",
		"claim_revision":       1,
		"predicate":            "relationship",
		"subject_ref":          asset.OwnerRef,
		"object_ref":           nil,
		"participant_refs":     []storygraph.OwnerRef{},
		"anchor_refs":          []storygraph.OwnerRef{episode.OwnerRef},
		"valid_scope":          storygraph.ClaimScope{Kind: "project", OwnerLogicalID: value.ProjectID},
		"story_time_range":     nil,
		"polarity":             "neutral",
		"status":               "asserted",
		"creator_decision_ref": nil,
		"supersedes_claim_ref": nil,
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
		newEdge(t, storygraph.EdgeTypeClaimAnchor, episode.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{AnchorRole: "episode"}),
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
	t.Fatal("missing production Narrative Claim")
	return nil
}
