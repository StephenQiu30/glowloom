package storygraph_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestProductionInteractionRelationsRejectPayloadAndEdgeDrift(t *testing.T) {
	tests := map[string]func(*storygraph.ProductionOwnerSnapshot){
		"claim omits actors": func(value *storygraph.ProductionOwnerSnapshot) {
			mutateProductionPayload(t, productionInteractionNode(t, value), func(payload map[string]any) {
				delete(payload, "actor_occurrence_refs")
			})
		},
		"claim adds unknown field": func(value *storygraph.ProductionOwnerSnapshot) {
			mutateProductionPayload(t, productionInteractionNode(t, value), func(payload map[string]any) {
				payload["prompt"] = "人物拿起道具"
			})
		},
		"hold loses after holder": func(value *storygraph.ProductionOwnerSnapshot) {
			mutateProductionPayload(t, productionInteractionNode(t, value), func(payload map[string]any) {
				payload["holder_after"] = nil
			})
		},
		"prop points at character occurrence": func(value *storygraph.ProductionOwnerSnapshot) {
			character := productionOccurrenceByAssetKind(t, value, "character")
			mutateProductionPayload(t, productionInteractionNode(t, value), func(payload map[string]any) {
				payload["prop_occurrence_ref"] = character.OwnerRef
			})
		},
		"claim loses prop participant": func(value *storygraph.ProductionOwnerSnapshot) {
			claim := productionInteractionNode(t, value)
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeClaimParticipant && edge.ToNodeKey == claim.StoryNodeKey && edge.Qualifier.ParticipantRole == "prop"
			})
		},
		"claim loses character occurrence anchor": func(value *storygraph.ProductionOwnerSnapshot) {
			claim := productionInteractionNode(t, value)
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeClaimAnchor && edge.ToNodeKey == claim.StoryNodeKey && edge.Qualifier.AnchorRole == "character_occurrence"
			})
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := productionInteractionRelationFixture(t)
			mutate(&value)
			if _, err := storygraph.CompileProductionOwnerSnapshot(value); err == nil {
				t.Fatal("invalid production Interaction relation was accepted")
			}
		})
	}
}

func TestProductionInteractionRelationsAcceptExactHoldTransition(t *testing.T) {
	value := productionInteractionRelationFixture(t)
	if _, err := storygraph.CompileProductionOwnerSnapshot(value); err != nil {
		t.Fatalf("exact production Interaction relation was rejected: %v", err)
	}
}

func productionInteractionRelationFixture(t *testing.T) storygraph.ProductionOwnerSnapshot {
	t.Helper()
	value := productionOwnerSnapshotFixture(t)
	character := productionNodeByType(t, &value, storygraph.NodeTypeAssetIdentity)
	characterOccurrence := productionNodeByType(t, &value, storygraph.NodeTypeOccurrence)
	scene := productionNodeByType(t, &value, storygraph.NodeTypeScene)
	evidence := productionNodeByType(t, &value, storygraph.NodeTypeSourceEvidence)
	evidenceRef := character.EvidenceRefs[0]

	assetCollectionIndex := -1
	for index := range value.OwnerCollections {
		if value.OwnerCollections[index].VersionFamily == "asset_identity_state_set" {
			assetCollectionIndex = index
			break
		}
	}
	if assetCollectionIndex < 0 {
		t.Fatal("missing asset collection")
	}
	collection := value.OwnerCollections[assetCollectionIndex]
	propOwner := collection.Members[0]
	propOwner.LogicalID = "prop:letter"
	propOwner.VersionID = uuid.NewString()
	propOwner.ContentHash = productionHash(propOwner.LogicalID)
	collection.Members = append(collection.Members, propOwner)
	value.OwnerCollections[assetCollectionIndex] = productionCollection(
		t, value.WorkspaceID, value.ProjectID, collection.OwnerKind, collection.VersionFamily,
		collection.ScopeKind, collection.ScopeKey, collection.Members,
	)

	ownerRef := func(owner storygraph.OwnerVersionIdentity, fragment string) storygraph.OwnerRef {
		fragmentHash := ""
		if fragment != "" {
			fragmentHash = productionHash(fragment)
		}
		return storygraph.OwnerRef{
			WorkspaceID: owner.WorkspaceID, ProjectID: owner.ProjectID, OwnerKind: owner.OwnerKind,
			VersionFamily: owner.VersionFamily, OwnerLogicalID: owner.LogicalID,
			FragmentKey: fragment, FragmentContentHash: fragmentHash,
			OwnerVersionID: owner.VersionID, OwnerRevision: owner.Revision, OwnerContentHash: owner.ContentHash,
		}
	}
	bibleRef := evidence.OwnerRef
	fragmentRef := func(fragment string) storygraph.OwnerRef {
		result := bibleRef
		result.FragmentKey = fragment
		result.FragmentContentHash = productionHash(fragment)
		return result
	}
	payload := func(contractID, projectionHash string, fields map[string]any) json.RawMessage {
		fields["payload_contract_id"] = contractID
		fields["projection_hash"] = projectionHash
		encoded, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	node := func(nodeType storygraph.NodeType, owner storygraph.OwnerRef, evidenceRefs []storygraph.EvidenceRef, body json.RawMessage) storygraph.Node {
		return storygraph.Node{StoryNodeKey: mustNodeKey(t, nodeType, owner), NodeType: nodeType, OwnerRef: owner, EvidenceRefs: evidenceRefs, Payload: body}
	}

	propRef := ownerRef(propOwner, "")
	propSpecificationRef := fragmentRef("specification:prop-letter")
	propStateRef := ownerRef(propOwner, "state:sealed")
	propBindingRef := fragmentRef("binding:prop-letter")
	propOccurrenceRef := scene.OwnerRef
	propOccurrenceRef.FragmentKey = "occurrence:prop-letter"
	propOccurrenceRef.FragmentContentHash = productionHash(propOccurrenceRef.FragmentKey)
	prop := node(storygraph.NodeTypeAssetIdentity, propRef, []storygraph.EvidenceRef{evidenceRef}, payload(
		"storygraph-production/asset-identity-payload-contract", propRef.OwnerContentHash,
		map[string]any{"asset_kind": "prop", "creator_decision_ref": nil},
	))
	propSpecification := node(storygraph.NodeTypePropSpecification, propSpecificationRef, []storygraph.EvidenceRef{evidenceRef}, payload(
		"storygraph-production/specification-payload-contract", propSpecificationRef.FragmentContentHash,
		map[string]any{"asset_kind": "prop", "creator_decision_ref": nil},
	))
	propState := node(storygraph.NodeTypeAssetState, propStateRef, []storygraph.EvidenceRef{evidenceRef}, payload(
		"storygraph-production/asset-state-payload-contract", propStateRef.FragmentContentHash,
		map[string]any{"asset_kind": "prop", "state_key": "state:sealed", "story_time_range": nil, "creator_decision_ref": nil},
	))
	propBinding := node(storygraph.NodeTypeProductionBinding, propBindingRef, nil, payload(
		"storygraph-production/production-binding-payload-contract", propBindingRef.FragmentContentHash,
		map[string]any{"asset_identity_ref": prop.OwnerRef, "specification_ref": propSpecification.OwnerRef, "state_refs": []storygraph.OwnerRef{propState.OwnerRef}},
	))
	propOccurrence := node(storygraph.NodeTypeOccurrence, propOccurrenceRef, []storygraph.EvidenceRef{evidenceRef}, payload(
		"storygraph-production/occurrence-payload-contract", propOccurrenceRef.FragmentContentHash,
		map[string]any{"asset_identity_ref": prop.OwnerRef, "asset_state_ref": propState.OwnerRef, "scene_ref": scene.OwnerRef, "beat_ref": nil, "creator_decision_ref": nil},
	))
	value.Graph.Nodes = append(value.Graph.Nodes, prop, propSpecification, propState, propBinding, propOccurrence)
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, prop.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, propSpecification.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, propState.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, propOccurrence.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeDescribesIdentity, prop.StoryNodeKey, propSpecification.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeHasState, prop.StoryNodeKey, propState.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeMaterializes, prop.StoryNodeKey, propBinding.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "asset"}),
		newEdge(t, storygraph.EdgeTypeMaterializes, propSpecification.StoryNodeKey, propBinding.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "specification"}),
		newEdge(t, storygraph.EdgeTypeMaterializes, propState.StoryNodeKey, propBinding.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "state"}),
		newEdge(t, storygraph.EdgeTypeAnchorsOccurrence, scene.StoryNodeKey, propOccurrence.StoryNodeKey, storygraph.EdgeQualifier{AnchorRole: "scene"}),
		newEdge(t, storygraph.EdgeTypeInstantiatesOccurrence, propState.StoryNodeKey, propOccurrence.StoryNodeKey, storygraph.EdgeQualifier{}),
	)

	claimRef := scene.OwnerRef
	claimRef.FragmentKey = "interaction:hold-letter"
	claimRef.FragmentContentHash = productionHash(claimRef.FragmentKey)
	claimPayload := payload(
		"storygraph-production/continuity-interaction-payload-contract", claimRef.FragmentContentHash,
		map[string]any{
			"claim_type": "interaction", "claim_series_key": "interaction_series_hold_letter", "claim_revision": 1,
			"predicate": "hold", "actor_occurrence_refs": []storygraph.OwnerRef{characterOccurrence.OwnerRef},
			"prop_occurrence_ref": propOccurrence.OwnerRef, "scene_ref": scene.OwnerRef,
			"valid_scope": storygraph.ClaimScope{Kind: "scene", OwnerLogicalID: scene.OwnerRef.OwnerLogicalID},
			"story_time":  "storytime:0001", "status": "asserted", "hand": "right",
			"holder_before": nil, "holder_after": character.OwnerRef,
			"prop_state_before": propState.OwnerRef, "prop_state_after": propState.OwnerRef,
			"creator_decision_ref": nil, "supersedes_claim_ref": nil,
		},
	)
	claim := node(storygraph.NodeTypeContinuityClaim, claimRef, []storygraph.EvidenceRef{evidenceRef}, claimPayload)
	value.Graph.Nodes = append(value.Graph.Nodes, claim)
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypeSupports, evidence.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeClaimParticipant, characterOccurrence.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{ParticipantRole: "actor"}),
		newEdge(t, storygraph.EdgeTypeClaimParticipant, propOccurrence.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{ParticipantRole: "prop"}),
		newEdge(t, storygraph.EdgeTypeClaimParticipant, character.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{ParticipantRole: "holder_after"}),
		newEdge(t, storygraph.EdgeTypeClaimAnchor, scene.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{AnchorRole: "scene"}),
		newEdge(t, storygraph.EdgeTypeClaimAnchor, characterOccurrence.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{AnchorRole: "character_occurrence"}),
		newEdge(t, storygraph.EdgeTypeClaimAnchor, propOccurrence.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{AnchorRole: "prop_occurrence"}),
		newEdge(t, storygraph.EdgeTypeClaimState, propState.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{StateRole: "prop_before"}),
		newEdge(t, storygraph.EdgeTypeClaimState, propState.StoryNodeKey, claim.StoryNodeKey, storygraph.EdgeQualifier{StateRole: "prop_after"}),
	)
	return value
}

func productionInteractionNode(t *testing.T, value *storygraph.ProductionOwnerSnapshot) *storygraph.Node {
	t.Helper()
	for index := range value.Graph.Nodes {
		if value.Graph.Nodes[index].NodeType != storygraph.NodeTypeContinuityClaim {
			continue
		}
		var payload struct {
			ClaimType string `json:"claim_type"`
		}
		if json.Unmarshal(value.Graph.Nodes[index].Payload, &payload) == nil && payload.ClaimType == "interaction" {
			return &value.Graph.Nodes[index]
		}
	}
	t.Fatal("missing production Interaction Claim")
	return nil
}

func productionOccurrenceByAssetKind(t *testing.T, value *storygraph.ProductionOwnerSnapshot, kind string) *storygraph.Node {
	t.Helper()
	assets := make(map[string]string)
	for _, node := range value.Graph.Nodes {
		if node.NodeType != storygraph.NodeTypeAssetIdentity {
			continue
		}
		var payload struct {
			AssetKind string `json:"asset_kind"`
		}
		if json.Unmarshal(node.Payload, &payload) == nil {
			encoded, _ := json.Marshal(node.OwnerRef)
			assets[string(encoded)] = payload.AssetKind
		}
	}
	for index := range value.Graph.Nodes {
		node := &value.Graph.Nodes[index]
		if node.NodeType != storygraph.NodeTypeOccurrence {
			continue
		}
		var payload struct {
			AssetIdentityRef storygraph.OwnerRef `json:"asset_identity_ref"`
		}
		if json.Unmarshal(node.Payload, &payload) == nil {
			encoded, _ := json.Marshal(payload.AssetIdentityRef)
			if assets[string(encoded)] == kind {
				return node
			}
		}
	}
	t.Fatalf("missing production %s occurrence", kind)
	return nil
}
