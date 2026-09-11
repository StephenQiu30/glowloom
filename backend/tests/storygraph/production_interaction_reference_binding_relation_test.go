package storygraph_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestProductionInteractionReferenceBindingRejectsPayloadAndRelationDrift(t *testing.T) {
	tests := map[string]func(*storygraph.ProductionOwnerSnapshot, *storygraph.Node){
		"unknown payload field": func(_ *storygraph.ProductionOwnerSnapshot, binding *storygraph.Node) {
			mutateProductionPayload(t, binding, func(payload map[string]any) { payload["holder"] = "latest" })
		},
		"character asset closure omitted": func(value *storygraph.ProductionOwnerSnapshot, binding *storygraph.Node) {
			mutateProductionPayload(t, binding, func(payload map[string]any) { payload["character_asset_version_refs"] = []any{} })
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeBindsReferenceInput && edge.ToNodeKey == binding.StoryNodeKey && edge.Qualifier.ReferenceRole == "character_asset"
			})
		},
		"character asset used as prop": func(value *storygraph.ProductionOwnerSnapshot, binding *storygraph.Node) {
			character := productionAssetVersionByPurpose(t, value, "character_identity_anchor")
			prop := productionAssetVersionByPurpose(t, value, "prop_sheet")
			mutateProductionPayload(t, binding, func(payload map[string]any) { payload["prop_asset_version_ref"] = character.OwnerRef })
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeBindsReferenceInput && edge.FromNodeKey == prop.StoryNodeKey && edge.ToNodeKey == binding.StoryNodeKey
			})
			value.Graph.Edges = append(value.Graph.Edges, newEdge(t, storygraph.EdgeTypeBindsReferenceInput, character.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "prop_asset"}))
		},
		"base artifact selected as composition": func(value *storygraph.ProductionOwnerSnapshot, binding *storygraph.Node) {
			baseArtifact := productionArtifactByFamily(t, value, "asset_base_reference_set")
			mutateProductionPayload(t, binding, func(payload map[string]any) { payload["selected_composition_artifact_ref"] = baseArtifact.OwnerRef })
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeBindsReferenceOutput && edge.ToNodeKey == binding.StoryNodeKey
			})
			value.Graph.Edges = append(value.Graph.Edges, newEdge(t, storygraph.EdgeTypeBindsReferenceOutput, baseArtifact.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{}))
		},
		"scene target used as interaction target": func(value *storygraph.ProductionOwnerSnapshot, binding *storygraph.Node) {
			sceneTarget := productionNodeByFragment(t, value, storygraph.NodeTypeReferencePlanTarget, "target:scene-composition")
			mutateProductionPayload(t, binding, func(payload map[string]any) { payload["fulfilled_reference_target_ref"] = sceneTarget.OwnerRef })
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeFulfillsReferenceTarget && edge.ToNodeKey == binding.StoryNodeKey
			})
			value.Graph.Edges = append(value.Graph.Edges, newEdge(t, storygraph.EdgeTypeFulfillsReferenceTarget, sceneTarget.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{}))
		},
		"interaction input edge omitted": func(value *storygraph.ProductionOwnerSnapshot, binding *storygraph.Node) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeBindsReferenceInput && edge.ToNodeKey == binding.StoryNodeKey && edge.Qualifier.ReferenceRole == "interaction"
			})
		},
		"composition output edge omitted": func(value *storygraph.ProductionOwnerSnapshot, binding *storygraph.Node) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeBindsReferenceOutput && edge.ToNodeKey == binding.StoryNodeKey
			})
		},
		"target fulfillment edge omitted": func(value *storygraph.ProductionOwnerSnapshot, binding *storygraph.Node) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeFulfillsReferenceTarget && edge.ToNodeKey == binding.StoryNodeKey
			})
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value, binding := productionInteractionReferenceBindingFixture(t)
			mutate(&value, binding)
			if _, err := storygraph.Canonicalize(value.Graph); err == nil {
				t.Fatal("invalid Production Interaction Reference Binding was accepted")
			}
		})
	}
}

func TestProductionInteractionReferenceBindingAcceptsExactTargetClosure(t *testing.T) {
	value, _ := productionInteractionReferenceBindingFixture(t)
	if _, err := storygraph.Canonicalize(value.Graph); err != nil {
		t.Fatalf("exact Production Interaction Reference Binding was rejected: %v", err)
	}
}

func TestProductionInteractionReferenceBindingRejectsTargetOutsideClaimClosure(t *testing.T) {
	value, binding := productionInteractionReferenceBindingFixture(t)
	target := productionNodeByFragment(t, &value, storygraph.NodeTypeReferencePlanTarget, "target:interaction-composition")
	location := productionAssetNodeByKind(t, &value, storygraph.NodeTypeAssetIdentity, "location")
	locationSpecification := productionAssetNodeByKind(t, &value, storygraph.NodeTypeLocationSpecification, "location")
	locationState := productionAssetNodeByKind(t, &value, storygraph.NodeTypeAssetState, "location")
	locationOccurrence := productionOccurrenceByAssetKind(t, &value, "location")
	locationTarget := productionNodeByFragment(t, &value, storygraph.NodeTypeReferencePlanTarget, "target:location-board")
	mutateProductionPayload(t, target, func(payload map[string]any) {
		owners := payload["target_owner_refs"].(map[string]any)
		appendOwner := func(field string, ref storygraph.OwnerRef) {
			raw, err := json.Marshal(owners[field])
			if err != nil {
				t.Fatal(err)
			}
			var refs []storygraph.OwnerRef
			if err = json.Unmarshal(raw, &refs); err != nil {
				t.Fatal(err)
			}
			refs = append(refs, ref)
			sortProductionOwnerRefs(refs)
			owners[field] = refs
		}
		appendOwner("identity", location.OwnerRef)
		appendOwner("specification", locationSpecification.OwnerRef)
		appendOwner("state", locationState.OwnerRef)
		appendOwner("occurrence", locationOccurrence.OwnerRef)
		raw, err := json.Marshal(payload["depends_on_target_refs"])
		if err != nil {
			t.Fatal(err)
		}
		var dependencies []storygraph.OwnerRef
		if err = json.Unmarshal(raw, &dependencies); err != nil {
			t.Fatal(err)
		}
		dependencies = append(dependencies, locationTarget.OwnerRef)
		sortProductionOwnerRefs(dependencies)
		payload["depends_on_target_refs"] = dependencies
	})
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypePlansReference, location.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "identity"}),
		newEdge(t, storygraph.EdgeTypePlansReference, locationSpecification.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "specification"}),
		newEdge(t, storygraph.EdgeTypePlansReference, locationState.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "state"}),
		newEdge(t, storygraph.EdgeTypePlansReference, locationOccurrence.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "occurrence"}),
		newEdge(t, storygraph.EdgeTypeDependsOnReferenceTarget, locationTarget.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{}),
	)

	withoutBinding := value
	withoutBinding.Graph.Nodes = removeProductionNode(withoutBinding.Graph.Nodes, binding.StoryNodeKey)
	withoutBinding.Graph.Edges = removeProductionEdge(withoutBinding.Graph.Edges, func(edge storygraph.Edge) bool {
		return edge.FromNodeKey == binding.StoryNodeKey || edge.ToNodeKey == binding.StoryNodeKey
	})
	if _, err := storygraph.Canonicalize(withoutBinding.Graph); err != nil {
		t.Fatalf("expanded Target fixture is not independently valid: %v", err)
	}
	if _, err := storygraph.Canonicalize(value.Graph); err == nil {
		t.Fatal("Interaction Reference Binding accepted a Target outside its Claim participant closure")
	}
}

func productionInteractionReferenceBindingFixture(t *testing.T) (storygraph.ProductionOwnerSnapshot, *storygraph.Node) {
	t.Helper()
	value, _ := productionSceneReferenceBindingFixture(t)
	interactionTarget := *productionNodeByFragment(t, &value, storygraph.NodeTypeReferencePlanTarget, "target:interaction-composition")
	interaction := *productionInteractionNode(t, &value)
	characterVersion := *productionAssetVersionByPurpose(t, &value, "character_identity_anchor")
	propVersion := *productionAssetVersionByPurpose(t, &value, "prop_sheet")
	policy := *productionNodeByType(t, &value, storygraph.NodeTypePolicySnapshot)
	style := *productionNodeByType(t, &value, storygraph.NodeTypeEffectiveStyleSnapshot)
	owner := func(logicalID string) storygraph.OwnerRef {
		return storygraph.OwnerRef{
			WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, OwnerKind: "production/reference",
			VersionFamily: "reference_binding_set", OwnerLogicalID: logicalID, OwnerVersionID: uuid.NewString(),
			OwnerRevision: 1, OwnerContentHash: productionHash(logicalID),
		}
	}
	artifactRef := characterVersion.OwnerRef
	artifactRef.OwnerKind = "asset"
	artifactRef.VersionFamily = "asset_composition_artifact_set"
	artifactRef.OwnerLogicalID = "artifact:interaction-composition"
	artifactRef.OwnerVersionID = uuid.NewString()
	artifactRef.OwnerContentHash = productionHash(artifactRef.OwnerLogicalID)
	bindingRef := owner("interaction-binding:opening")
	constraints := map[string]any{"world_rule_refs": []storygraph.OwnerRef{}, "policy_snapshot_ref": policy.OwnerRef, "effective_style_snapshot_ref": style.OwnerRef}
	payload := func(contractID, projectionHash string, fields map[string]any) json.RawMessage {
		fields["payload_contract_id"] = contractID
		fields["projection_hash"] = projectionHash
		raw, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	artifact := storygraph.Node{StoryNodeKey: mustNodeKey(t, storygraph.NodeTypeArtifact, artifactRef), NodeType: storygraph.NodeTypeArtifact, OwnerRef: artifactRef, EvidenceRefs: []storygraph.EvidenceRef{}, Payload: payload("storygraph-production/artifact-ref-payload-contract", artifactRef.OwnerContentHash, map[string]any{})}
	binding := storygraph.Node{StoryNodeKey: mustNodeKey(t, storygraph.NodeTypeInteractionReferenceBindingVersion, bindingRef), NodeType: storygraph.NodeTypeInteractionReferenceBindingVersion, OwnerRef: bindingRef, EvidenceRefs: []storygraph.EvidenceRef{}, Payload: payload(
		"storygraph-production/interaction-reference-binding-payload-contract", bindingRef.OwnerContentHash,
		map[string]any{
			"interaction_claim_ref": interaction.OwnerRef, "character_asset_version_refs": []storygraph.OwnerRef{characterVersion.OwnerRef},
			"prop_asset_version_ref": propVersion.OwnerRef, "selected_composition_artifact_ref": artifactRef,
			"fulfilled_reference_target_ref": interactionTarget.OwnerRef, "constraints": constraints,
		},
	)}
	value.Graph.Nodes = append(value.Graph.Nodes, artifact, binding)
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypeFulfillsReferenceTarget, interactionTarget.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeBindsReferenceInput, interaction.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "interaction"}),
		newEdge(t, storygraph.EdgeTypeBindsReferenceInput, characterVersion.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "character_asset"}),
		newEdge(t, storygraph.EdgeTypeBindsReferenceInput, propVersion.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "prop_asset"}),
		newEdge(t, storygraph.EdgeTypeBindsReferenceOutput, artifact.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeConstrains, policy.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{ConstraintRole: "policy"}),
		newEdge(t, storygraph.EdgeTypeConstrains, style.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{ConstraintRole: "style"}),
	)
	return value, &value.Graph.Nodes[len(value.Graph.Nodes)-1]
}

func productionAssetVersionByPurpose(t *testing.T, value *storygraph.ProductionOwnerSnapshot, purpose string) *storygraph.Node {
	t.Helper()
	for index := range value.Graph.Nodes {
		node := &value.Graph.Nodes[index]
		if node.NodeType != storygraph.NodeTypeAssetVersion {
			continue
		}
		var payload struct {
			Purpose string `json:"purpose"`
		}
		if json.Unmarshal(node.Payload, &payload) == nil && payload.Purpose == purpose {
			return node
		}
	}
	t.Fatalf("missing Production AssetVersion purpose %s", purpose)
	return nil
}

func removeProductionNode(nodes []storygraph.Node, key string) []storygraph.Node {
	result := make([]storygraph.Node, 0, len(nodes))
	for _, node := range nodes {
		if node.StoryNodeKey != key {
			result = append(result, node)
		}
	}
	return result
}
