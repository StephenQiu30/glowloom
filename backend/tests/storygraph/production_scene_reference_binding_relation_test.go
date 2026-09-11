package storygraph_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestProductionSceneReferenceBindingRejectsPayloadAndRelationDrift(t *testing.T) {
	tests := map[string]func(*storygraph.ProductionOwnerSnapshot, *storygraph.Node){
		"unknown payload field": func(_ *storygraph.ProductionOwnerSnapshot, binding *storygraph.Node) {
			mutateProductionPayload(t, binding, func(payload map[string]any) { payload["prompt"] = "latest" })
		},
		"occurrence closure omitted": func(value *storygraph.ProductionOwnerSnapshot, binding *storygraph.Node) {
			mutateProductionPayload(t, binding, func(payload map[string]any) { payload["occurrence_refs"] = []any{} })
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeBindsReferenceInput && edge.ToNodeKey == binding.StoryNodeKey && edge.Qualifier.ReferenceRole == "occurrence"
			})
		},
		"character asset omitted": func(value *storygraph.ProductionOwnerSnapshot, binding *storygraph.Node) {
			mutateProductionPayload(t, binding, func(payload map[string]any) { payload["character_asset_version_refs"] = []any{} })
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeBindsReferenceInput && edge.ToNodeKey == binding.StoryNodeKey && edge.Qualifier.ReferenceRole == "character_asset"
			})
		},
		"base artifact selected as composition": func(value *storygraph.ProductionOwnerSnapshot, binding *storygraph.Node) {
			baseArtifact := productionArtifactByFamily(t, value, "asset_base_reference_set")
			mutateProductionPayload(t, binding, func(payload map[string]any) { payload["selected_composition_artifact_ref"] = baseArtifact.OwnerRef })
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeBindsReferenceOutput && edge.ToNodeKey == binding.StoryNodeKey
			})
			value.Graph.Edges = append(value.Graph.Edges, newEdge(t, storygraph.EdgeTypeBindsReferenceOutput, baseArtifact.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{}))
		},
		"interaction target used as scene target": func(value *storygraph.ProductionOwnerSnapshot, binding *storygraph.Node) {
			interactionTarget := productionNodeByFragment(t, value, storygraph.NodeTypeReferencePlanTarget, "target:interaction-composition")
			mutateProductionPayload(t, binding, func(payload map[string]any) { payload["fulfilled_reference_target_ref"] = interactionTarget.OwnerRef })
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeFulfillsReferenceTarget && edge.ToNodeKey == binding.StoryNodeKey
			})
			value.Graph.Edges = append(value.Graph.Edges, newEdge(t, storygraph.EdgeTypeFulfillsReferenceTarget, interactionTarget.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{}))
		},
		"scene input edge omitted": func(value *storygraph.ProductionOwnerSnapshot, binding *storygraph.Node) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeBindsReferenceInput && edge.ToNodeKey == binding.StoryNodeKey && edge.Qualifier.ReferenceRole == "scene"
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
			value, binding := productionSceneReferenceBindingFixture(t)
			mutate(&value, binding)
			if _, err := storygraph.Canonicalize(value.Graph); err == nil {
				t.Fatal("invalid Production Scene Reference Binding was accepted")
			}
		})
	}
}

func TestProductionSceneReferenceBindingAcceptsExactTargetClosure(t *testing.T) {
	value, _ := productionSceneReferenceBindingFixture(t)
	if _, err := storygraph.Canonicalize(value.Graph); err != nil {
		t.Fatalf("exact Production Scene Reference Binding was rejected: %v", err)
	}
}

func productionSceneReferenceBindingFixture(t *testing.T) (storygraph.ProductionOwnerSnapshot, *storygraph.Node) {
	t.Helper()
	value := productionInteractionRelationFixture(t)
	addProductionLocationRelation(t, &value)
	addProductionAllKindReferencePlan(t, &value)
	characterTarget := productionNodeByFragment(t, &value, storygraph.NodeTypeReferencePlanTarget, "target:character-anchor")
	locationTarget := productionNodeByFragment(t, &value, storygraph.NodeTypeReferencePlanTarget, "target:location-board")
	propTarget := productionNodeByFragment(t, &value, storygraph.NodeTypeReferencePlanTarget, "target:prop-sheet")
	characterVersion := addProductionBaseAssetVersion(t, &value, *characterTarget, "character_identity_anchor")
	locationVersion := addProductionBaseAssetVersion(t, &value, *locationTarget, "location_board")
	propVersion := addProductionBaseAssetVersion(t, &value, *propTarget, "prop_sheet")

	sceneTarget := *productionNodeByFragment(t, &value, storygraph.NodeTypeReferencePlanTarget, "target:scene-composition")
	policy := *productionNodeByType(t, &value, storygraph.NodeTypePolicySnapshot)
	style := *productionNodeByType(t, &value, storygraph.NodeTypeEffectiveStyleSnapshot)
	scene := *productionNodeByType(t, &value, storygraph.NodeTypeScene)
	interaction := *productionInteractionNode(t, &value)
	characterOccurrence := *productionOccurrenceByAssetKind(t, &value, "character")
	locationOccurrence := *productionOccurrenceByAssetKind(t, &value, "location")
	propOccurrence := *productionOccurrenceByAssetKind(t, &value, "prop")
	owner := func(ownerKind, family, logicalID string) storygraph.OwnerRef {
		return storygraph.OwnerRef{
			WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, OwnerKind: ownerKind,
			VersionFamily: family, OwnerLogicalID: logicalID, OwnerVersionID: uuid.NewString(),
			OwnerRevision: 1, OwnerContentHash: productionHash(logicalID),
		}
	}
	artifactRef := owner("asset", "asset_composition_artifact_set", "artifact:scene-composition")
	bindingRef := owner("production/reference", "reference_binding_set", "scene-binding:opening")
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
	binding := storygraph.Node{StoryNodeKey: mustNodeKey(t, storygraph.NodeTypeSceneReferenceBindingVersion, bindingRef), NodeType: storygraph.NodeTypeSceneReferenceBindingVersion, OwnerRef: bindingRef, EvidenceRefs: []storygraph.EvidenceRef{}, Payload: payload(
		"storygraph-production/scene-reference-binding-payload-contract", bindingRef.OwnerContentHash,
		map[string]any{
			"scene_ref":                         scene.OwnerRef,
			"occurrence_refs":                   sortedProductionOwnerRefs(characterOccurrence.OwnerRef, locationOccurrence.OwnerRef, propOccurrence.OwnerRef),
			"interaction_claim_refs":            []storygraph.OwnerRef{interaction.OwnerRef},
			"character_asset_version_refs":      []storygraph.OwnerRef{characterVersion.OwnerRef},
			"location_asset_version_ref":        locationVersion.OwnerRef,
			"prop_asset_version_refs":           []storygraph.OwnerRef{propVersion.OwnerRef},
			"selected_composition_artifact_ref": artifactRef, "fulfilled_reference_target_ref": sceneTarget.OwnerRef,
			"constraints": constraints,
		},
	)}
	value.Graph.Nodes = append(value.Graph.Nodes, artifact, binding)
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypeFulfillsReferenceTarget, sceneTarget.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeBindsReferenceInput, scene.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "scene"}),
		newEdge(t, storygraph.EdgeTypeBindsReferenceInput, characterOccurrence.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "occurrence"}),
		newEdge(t, storygraph.EdgeTypeBindsReferenceInput, locationOccurrence.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "occurrence"}),
		newEdge(t, storygraph.EdgeTypeBindsReferenceInput, propOccurrence.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "occurrence"}),
		newEdge(t, storygraph.EdgeTypeBindsReferenceInput, interaction.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "interaction"}),
		newEdge(t, storygraph.EdgeTypeBindsReferenceInput, characterVersion.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "character_asset"}),
		newEdge(t, storygraph.EdgeTypeBindsReferenceInput, locationVersion.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "location_asset"}),
		newEdge(t, storygraph.EdgeTypeBindsReferenceInput, propVersion.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "prop_asset"}),
		newEdge(t, storygraph.EdgeTypeBindsReferenceOutput, artifact.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeConstrains, policy.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{ConstraintRole: "policy"}),
		newEdge(t, storygraph.EdgeTypeConstrains, style.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{ConstraintRole: "style"}),
	)
	return value, &value.Graph.Nodes[len(value.Graph.Nodes)-1]
}

func addProductionBaseAssetVersion(t *testing.T, value *storygraph.ProductionOwnerSnapshot, target storygraph.Node, purpose string) storygraph.Node {
	t.Helper()
	var targetPayload struct {
		TargetOwnerRefs struct {
			Identity      []storygraph.OwnerRef `json:"identity"`
			Specification []storygraph.OwnerRef `json:"specification"`
			State         []storygraph.OwnerRef `json:"state"`
		} `json:"target_owner_refs"`
		Constraints map[string]any `json:"constraints"`
	}
	if err := json.Unmarshal(target.Payload, &targetPayload); err != nil {
		t.Fatal(err)
	}
	identity := productionNodeByOwnerRef(t, value, targetPayload.TargetOwnerRefs.Identity[0])
	specification := productionNodeByOwnerRef(t, value, targetPayload.TargetOwnerRefs.Specification[0])
	state := productionNodeByOwnerRef(t, value, targetPayload.TargetOwnerRefs.State[0])
	policy := productionNodeByOwnerRef(t, value, targetPayload.Constraints["policy_snapshot_ref"])
	style := productionNodeByOwnerRef(t, value, targetPayload.Constraints["effective_style_snapshot_ref"])
	logicalSuffix := purpose + ":" + uuid.NewString()
	owner := func(logicalID string) storygraph.OwnerRef {
		return storygraph.OwnerRef{
			WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, OwnerKind: "asset", VersionFamily: "asset_base_reference_set",
			OwnerLogicalID: logicalID, OwnerVersionID: uuid.NewString(), OwnerRevision: 1, OwnerContentHash: productionHash(logicalID),
		}
	}
	artifactRef, versionRef := owner("artifact:"+logicalSuffix), owner("asset-version:"+logicalSuffix)
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
	version := storygraph.Node{StoryNodeKey: mustNodeKey(t, storygraph.NodeTypeAssetVersion, versionRef), NodeType: storygraph.NodeTypeAssetVersion, OwnerRef: versionRef, EvidenceRefs: []storygraph.EvidenceRef{}, Payload: payload(
		"storygraph-production/asset-version-payload-contract", versionRef.OwnerContentHash,
		map[string]any{
			"purpose": purpose, "asset_identity_ref": identity.OwnerRef, "specification_ref": specification.OwnerRef,
			"asset_state_ref": state.OwnerRef, "identity_anchor_asset_version_ref": nil, "selected_artifact_ref": artifactRef,
			"fulfilled_reference_target_ref": target.OwnerRef, "constraints": targetPayload.Constraints,
		},
	)}
	value.Graph.Nodes = append(value.Graph.Nodes, artifact, version)
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypeMaterializes, identity.StoryNodeKey, version.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "asset"}),
		newEdge(t, storygraph.EdgeTypeMaterializes, specification.StoryNodeKey, version.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "specification"}),
		newEdge(t, storygraph.EdgeTypeMaterializes, state.StoryNodeKey, version.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "state"}),
		newEdge(t, storygraph.EdgeTypeMaterializes, artifact.StoryNodeKey, version.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "artifact"}),
		newEdge(t, storygraph.EdgeTypeFulfillsReferenceTarget, target.StoryNodeKey, version.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeConstrains, policy.StoryNodeKey, version.StoryNodeKey, storygraph.EdgeQualifier{ConstraintRole: "policy"}),
		newEdge(t, storygraph.EdgeTypeConstrains, style.StoryNodeKey, version.StoryNodeKey, storygraph.EdgeQualifier{ConstraintRole: "style"}),
	)
	return version
}

func productionNodeByOwnerRef(t *testing.T, value *storygraph.ProductionOwnerSnapshot, refValue any) *storygraph.Node {
	t.Helper()
	raw, err := json.Marshal(refValue)
	if err != nil {
		t.Fatal(err)
	}
	var ref storygraph.OwnerRef
	if err = json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	for index := range value.Graph.Nodes {
		if productionOwnerRefsEqual(value.Graph.Nodes[index].OwnerRef, ref) {
			return &value.Graph.Nodes[index]
		}
	}
	t.Fatal("missing production node by Owner ref")
	return nil
}

func productionOwnerRefsEqual(left, right storygraph.OwnerRef) bool {
	leftRaw, _ := json.Marshal(left)
	rightRaw, _ := json.Marshal(right)
	return string(leftRaw) == string(rightRaw)
}

func productionArtifactByFamily(t *testing.T, value *storygraph.ProductionOwnerSnapshot, family string) *storygraph.Node {
	t.Helper()
	for index := range value.Graph.Nodes {
		if value.Graph.Nodes[index].NodeType == storygraph.NodeTypeArtifact && value.Graph.Nodes[index].OwnerRef.VersionFamily == family {
			return &value.Graph.Nodes[index]
		}
	}
	t.Fatalf("missing Artifact family %s", family)
	return nil
}

func sortedProductionOwnerRefs(values ...storygraph.OwnerRef) []storygraph.OwnerRef {
	sortProductionOwnerRefs(values)
	return values
}
