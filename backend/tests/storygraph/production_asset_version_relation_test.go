package storygraph_test

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/google/uuid"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestProductionAssetVersionRejectsPayloadAndRelationDrift(t *testing.T) {
	tests := map[string]func(*storygraph.ProductionOwnerSnapshot, *storygraph.Node){
		"unknown payload field": func(_ *storygraph.ProductionOwnerSnapshot, assetVersion *storygraph.Node) {
			mutateProductionPayload(t, assetVersion, func(payload map[string]any) {
				payload["latest_artifact"] = true
			})
		},
		"purpose and identity kind mismatch": func(_ *storygraph.ProductionOwnerSnapshot, assetVersion *storygraph.Node) {
			mutateProductionPayload(t, assetVersion, func(payload map[string]any) {
				payload["purpose"] = "location_board"
			})
		},
		"appearance omits identity anchor": func(_ *storygraph.ProductionOwnerSnapshot, assetVersion *storygraph.Node) {
			mutateProductionPayload(t, assetVersion, func(payload map[string]any) {
				payload["purpose"] = "character_appearance"
			})
		},
		"identity anchor purpose carries anchor": func(_ *storygraph.ProductionOwnerSnapshot, assetVersion *storygraph.Node) {
			mutateProductionPayload(t, assetVersion, func(payload map[string]any) {
				payload["identity_anchor_asset_version_ref"] = assetVersion.OwnerRef
			})
		},
		"selected artifact ref drifts": func(value *storygraph.ProductionOwnerSnapshot, assetVersion *storygraph.Node) {
			style := productionNodeByType(t, value, storygraph.NodeTypeEffectiveStyleSnapshot)
			mutateProductionPayload(t, assetVersion, func(payload map[string]any) {
				payload["selected_artifact_ref"] = style.OwnerRef
			})
		},
		"composition artifact cannot back a base asset version": func(value *storygraph.ProductionOwnerSnapshot, assetVersion *storygraph.Node) {
			artifact := productionNodeByType(t, value, storygraph.NodeTypeArtifact)
			compositionArtifact := *artifact
			compositionArtifact.OwnerRef.VersionFamily = "asset_composition_artifact_set"
			compositionArtifact.OwnerRef.OwnerLogicalID = "artifact:scene:opening:composition"
			compositionArtifact.OwnerRef.OwnerVersionID = uuid.NewString()
			compositionArtifact.OwnerRef.OwnerContentHash = productionHash("artifact:scene:opening:composition")
			compositionArtifact.StoryNodeKey = mustNodeKey(t, compositionArtifact.NodeType, compositionArtifact.OwnerRef)
			payload, err := json.Marshal(map[string]any{
				"payload_contract_id": "storygraph-production/artifact-ref-payload-contract",
				"projection_hash":     compositionArtifact.OwnerRef.OwnerContentHash,
			})
			if err != nil {
				t.Fatal(err)
			}
			compositionArtifact.Payload = payload
			value.Graph.Nodes = append(value.Graph.Nodes, compositionArtifact)
			mutateProductionPayload(t, assetVersion, func(payload map[string]any) {
				payload["selected_artifact_ref"] = compositionArtifact.OwnerRef
			})
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeMaterializes && edge.ToNodeKey == assetVersion.StoryNodeKey && edge.Qualifier.BindingRole == "artifact"
			})
			value.Graph.Edges = append(value.Graph.Edges, newEdge(t, storygraph.EdgeTypeMaterializes, compositionArtifact.StoryNodeKey, assetVersion.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "artifact"}))
		},
		"reference target identity input drifts": func(value *storygraph.ProductionOwnerSnapshot, _ *storygraph.Node) {
			target := productionNodeByType(t, value, storygraph.NodeTypeReferencePlanTarget)
			mutateProductionPayload(t, target, func(payload map[string]any) {
				ownerRefs := payload["target_owner_refs"].(map[string]any)
				ownerRefs["identity"] = []any{}
			})
		},
		"not generated target has a result": func(value *storygraph.ProductionOwnerSnapshot, _ *storygraph.Node) {
			target := productionNodeByType(t, value, storygraph.NodeTypeReferencePlanTarget)
			mutateProductionPayload(t, target, func(payload map[string]any) {
				payload["fulfillment"] = "not_generated"
			})
		},
		"missing artifact materialization": func(value *storygraph.ProductionOwnerSnapshot, assetVersion *storygraph.Node) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeMaterializes && edge.ToNodeKey == assetVersion.StoryNodeKey && edge.Qualifier.BindingRole == "artifact"
			})
		},
		"missing target fulfillment": func(value *storygraph.ProductionOwnerSnapshot, assetVersion *storygraph.Node) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeFulfillsReferenceTarget && edge.ToNodeKey == assetVersion.StoryNodeKey
			})
		},
		"missing style constraint": func(value *storygraph.ProductionOwnerSnapshot, assetVersion *storygraph.Node) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeConstrains && edge.ToNodeKey == assetVersion.StoryNodeKey && edge.Qualifier.ConstraintRole == "style"
			})
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value, assetVersion := productionAssetVersionFixture(t)
			mutate(&value, assetVersion)
			if _, err := storygraph.Canonicalize(value.Graph); err == nil {
				t.Fatal("invalid Production AssetVersion was accepted")
			}
		})
	}
}

func TestProductionAssetVersionAcceptsExactOwnerRefsAndEdges(t *testing.T) {
	value, _ := productionAssetVersionFixture(t)
	if _, err := storygraph.Canonicalize(value.Graph); err != nil {
		t.Fatalf("exact Production AssetVersion was rejected: %v", err)
	}
}

func TestProductionAssetVersionAcceptsAppearanceBoundToExactIdentityAnchor(t *testing.T) {
	value, _ := productionAssetVersionFixture(t)
	appearance := addProductionCharacterAppearance(t, &value)
	if _, err := storygraph.Canonicalize(value.Graph); err != nil {
		t.Fatalf("identity-bound character appearance was rejected: %v", err)
	}

	target := productionNodeByFragment(t, &value, storygraph.NodeTypeReferencePlanTarget, "target:character:linzhou:appearance-wounded")
	mutateProductionPayload(t, target, func(payload map[string]any) {
		payload["depends_on_target_refs"] = []any{}
	})
	if _, err := storygraph.Canonicalize(value.Graph); err == nil {
		t.Fatalf("appearance %s without its exact anchor target dependency was accepted", appearance.StoryNodeKey)
	}
}

func productionAssetVersionFixture(t *testing.T) (storygraph.ProductionOwnerSnapshot, *storygraph.Node) {
	t.Helper()
	value := productionOwnerSnapshotFixture(t)
	asset := productionNodeByType(t, &value, storygraph.NodeTypeAssetIdentity)
	specification := productionNodeByType(t, &value, storygraph.NodeTypeCharacterSpecification)
	state := productionNodeByType(t, &value, storygraph.NodeTypeAssetState)
	scene := productionNodeByType(t, &value, storygraph.NodeTypeScene)
	occurrence := productionNodeByType(t, &value, storygraph.NodeTypeOccurrence)

	owner := func(ownerKind, family, logicalID string) storygraph.OwnerRef {
		hash := productionHash(logicalID)
		return storygraph.OwnerRef{
			WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, OwnerKind: ownerKind,
			VersionFamily: family, OwnerLogicalID: logicalID, OwnerVersionID: uuid.NewString(),
			OwnerRevision: 1, OwnerContentHash: hash,
		}
	}
	fragment := func(base storygraph.OwnerRef, key string) storygraph.OwnerRef {
		base.FragmentKey = key
		base.FragmentContentHash = productionHash(key)
		return base
	}
	payload := func(contractID, projectionHash string, fields map[string]any) json.RawMessage {
		fields["payload_contract_id"] = contractID
		fields["projection_hash"] = projectionHash
		raw, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	node := func(nodeType storygraph.NodeType, ref storygraph.OwnerRef, body json.RawMessage) storygraph.Node {
		return storygraph.Node{StoryNodeKey: mustNodeKey(t, nodeType, ref), NodeType: nodeType, OwnerRef: ref, EvidenceRefs: []storygraph.EvidenceRef{}, Payload: body}
	}

	policyRef := owner("preset", "preset_effective_set", "policy:faithful")
	styleRef := owner("preset", "preset_effective_set", "style:faithful")
	planRef := owner("production/reference", "reference_plan_set", "reference-plan:main")
	targetRef := fragment(planRef, "target:character:linzhou:identity-anchor")
	artifactRef := owner("asset", "asset_base_reference_set", "artifact:character:linzhou:identity-anchor")
	assetVersionRef := owner("asset", "asset_base_reference_set", "asset-version:character:linzhou:identity-anchor")
	constraints := map[string]any{
		"world_rule_refs": []storygraph.OwnerRef{}, "policy_snapshot_ref": policyRef,
		"effective_style_snapshot_ref": styleRef,
	}
	policy := node(storygraph.NodeTypePolicySnapshot, policyRef, payload("storygraph-production/policy_snapshot-ref-payload-contract", policyRef.OwnerContentHash, map[string]any{}))
	style := node(storygraph.NodeTypeEffectiveStyleSnapshot, styleRef, payload("storygraph-production/effective_style_snapshot-ref-payload-contract", styleRef.OwnerContentHash, map[string]any{}))
	plan := node(storygraph.NodeTypeApprovedReferencePlanVersion, planRef, payload("storygraph-production/approved_reference_plan_version-ref-payload-contract", planRef.OwnerContentHash, map[string]any{}))
	target := node(storygraph.NodeTypeReferencePlanTarget, targetRef, payload("storygraph-production/reference-plan-target-payload-contract", targetRef.FragmentContentHash, map[string]any{
		"target_kind": "character_identity_anchor", "fulfillment": "required",
		"target_owner_refs": map[string]any{
			"identity": []storygraph.OwnerRef{asset.OwnerRef}, "specification": []storygraph.OwnerRef{specification.OwnerRef},
			"state": []storygraph.OwnerRef{state.OwnerRef}, "style": []storygraph.OwnerRef{styleRef},
			"scene": []storygraph.OwnerRef{scene.OwnerRef}, "occurrence": []storygraph.OwnerRef{occurrence.OwnerRef},
			"interaction": []storygraph.OwnerRef{},
		},
		"coverage_scope_keys": []string{"scene:opening"}, "depends_on_target_refs": []storygraph.OwnerRef{}, "constraints": constraints,
	}))
	artifact := node(storygraph.NodeTypeArtifact, artifactRef, payload("storygraph-production/artifact-ref-payload-contract", artifactRef.OwnerContentHash, map[string]any{}))
	assetVersion := node(storygraph.NodeTypeAssetVersion, assetVersionRef, payload("storygraph-production/asset-version-payload-contract", assetVersionRef.OwnerContentHash, map[string]any{
		"purpose": "character_identity_anchor", "asset_identity_ref": asset.OwnerRef,
		"specification_ref": specification.OwnerRef, "asset_state_ref": state.OwnerRef,
		"identity_anchor_asset_version_ref": nil, "selected_artifact_ref": artifactRef,
		"fulfilled_reference_target_ref": targetRef, "constraints": constraints,
	}))
	value.Graph.Nodes = append(value.Graph.Nodes, policy, style, plan, target, artifact, assetVersion)
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypeContainsReferenceTarget, plan.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: "target:0001"}),
		newEdge(t, storygraph.EdgeTypePlansReference, asset.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "identity"}),
		newEdge(t, storygraph.EdgeTypePlansReference, specification.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "specification"}),
		newEdge(t, storygraph.EdgeTypePlansReference, state.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "state"}),
		newEdge(t, storygraph.EdgeTypePlansReference, style.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "style"}),
		newEdge(t, storygraph.EdgeTypePlansReference, scene.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "scene"}),
		newEdge(t, storygraph.EdgeTypePlansReference, occurrence.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "occurrence"}),
		newEdge(t, storygraph.EdgeTypeConstrains, policy.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{ConstraintRole: "policy"}),
		newEdge(t, storygraph.EdgeTypeConstrains, style.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{ConstraintRole: "style"}),
		newEdge(t, storygraph.EdgeTypeMaterializes, asset.StoryNodeKey, assetVersion.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "asset"}),
		newEdge(t, storygraph.EdgeTypeMaterializes, specification.StoryNodeKey, assetVersion.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "specification"}),
		newEdge(t, storygraph.EdgeTypeMaterializes, state.StoryNodeKey, assetVersion.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "state"}),
		newEdge(t, storygraph.EdgeTypeMaterializes, artifact.StoryNodeKey, assetVersion.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "artifact"}),
		newEdge(t, storygraph.EdgeTypeFulfillsReferenceTarget, target.StoryNodeKey, assetVersion.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeConstrains, policy.StoryNodeKey, assetVersion.StoryNodeKey, storygraph.EdgeQualifier{ConstraintRole: "policy"}),
		newEdge(t, storygraph.EdgeTypeConstrains, style.StoryNodeKey, assetVersion.StoryNodeKey, storygraph.EdgeQualifier{ConstraintRole: "style"}),
	)
	return value, &value.Graph.Nodes[len(value.Graph.Nodes)-1]
}

func addProductionCharacterAppearance(t *testing.T, value *storygraph.ProductionOwnerSnapshot) *storygraph.Node {
	t.Helper()
	asset := *productionNodeByType(t, value, storygraph.NodeTypeAssetIdentity)
	specification := *productionNodeByType(t, value, storygraph.NodeTypeCharacterSpecification)
	anchorState := *productionNodeByType(t, value, storygraph.NodeTypeAssetState)
	anchorTarget := *productionNodeByType(t, value, storygraph.NodeTypeReferencePlanTarget)
	anchorAssetVersion := *productionNodeByType(t, value, storygraph.NodeTypeAssetVersion)
	plan := *productionNodeByType(t, value, storygraph.NodeTypeApprovedReferencePlanVersion)
	policy := *productionNodeByType(t, value, storygraph.NodeTypePolicySnapshot)
	style := *productionNodeByType(t, value, storygraph.NodeTypeEffectiveStyleSnapshot)
	scene := *productionNodeByType(t, value, storygraph.NodeTypeScene)
	evidence := *productionNodeByType(t, value, storygraph.NodeTypeSourceEvidence)
	anchorOccurrence := *productionNodeByType(t, value, storygraph.NodeTypeOccurrence)

	appearanceState := anchorState
	appearanceState.OwnerRef.FragmentKey = "state:zz-appearance-wounded"
	appearanceState.OwnerRef.FragmentContentHash = productionHash(appearanceState.OwnerRef.FragmentKey)
	appearanceState.StoryNodeKey = mustNodeKey(t, appearanceState.NodeType, appearanceState.OwnerRef)
	mutateProductionPayload(t, &appearanceState, func(payload map[string]any) {
		payload["projection_hash"] = appearanceState.OwnerRef.FragmentContentHash
		payload["state_key"] = "appearance:wounded"
	})

	appearanceOccurrence := anchorOccurrence
	appearanceOccurrence.OwnerRef.FragmentKey = "occurrence:zz-appearance-wounded"
	appearanceOccurrence.OwnerRef.FragmentContentHash = productionHash(appearanceOccurrence.OwnerRef.FragmentKey)
	appearanceOccurrence.StoryNodeKey = mustNodeKey(t, appearanceOccurrence.NodeType, appearanceOccurrence.OwnerRef)
	mutateProductionPayload(t, &appearanceOccurrence, func(payload map[string]any) {
		payload["projection_hash"] = appearanceOccurrence.OwnerRef.FragmentContentHash
		payload["asset_state_ref"] = appearanceState.OwnerRef
	})

	binding := productionNodeByType(t, value, storygraph.NodeTypeProductionBinding)
	mutateProductionPayload(t, binding, func(payload map[string]any) {
		encoded, err := json.Marshal(payload["state_refs"])
		if err != nil {
			t.Fatal(err)
		}
		var refs []storygraph.OwnerRef
		if err = json.Unmarshal(encoded, &refs); err != nil {
			t.Fatal(err)
		}
		refs = append(refs, appearanceState.OwnerRef)
		sort.Slice(refs, func(left, right int) bool { return refs[left].FragmentKey < refs[right].FragmentKey })
		payload["state_refs"] = refs
	})

	appearanceTarget := anchorTarget
	appearanceTarget.OwnerRef.FragmentKey = "target:character:linzhou:appearance-wounded"
	appearanceTarget.OwnerRef.FragmentContentHash = productionHash(appearanceTarget.OwnerRef.FragmentKey)
	appearanceTarget.StoryNodeKey = mustNodeKey(t, appearanceTarget.NodeType, appearanceTarget.OwnerRef)
	mutateProductionPayload(t, &appearanceTarget, func(payload map[string]any) {
		payload["projection_hash"] = appearanceTarget.OwnerRef.FragmentContentHash
		payload["target_kind"] = "character_appearance"
		ownerRefs := payload["target_owner_refs"].(map[string]any)
		ownerRefs["state"] = []storygraph.OwnerRef{appearanceState.OwnerRef}
		ownerRefs["occurrence"] = []storygraph.OwnerRef{appearanceOccurrence.OwnerRef}
		payload["depends_on_target_refs"] = []storygraph.OwnerRef{anchorTarget.OwnerRef}
	})

	appearanceArtifact := *productionNodeByType(t, value, storygraph.NodeTypeArtifact)
	appearanceArtifact.OwnerRef.OwnerLogicalID = "artifact:character:linzhou:appearance-wounded"
	appearanceArtifact.OwnerRef.OwnerVersionID = uuid.NewString()
	appearanceArtifact.OwnerRef.OwnerContentHash = productionHash(appearanceArtifact.OwnerRef.OwnerLogicalID)
	appearanceArtifact.StoryNodeKey = mustNodeKey(t, appearanceArtifact.NodeType, appearanceArtifact.OwnerRef)
	mutateProductionPayload(t, &appearanceArtifact, func(payload map[string]any) {
		payload["projection_hash"] = appearanceArtifact.OwnerRef.OwnerContentHash
	})

	appearanceAssetVersion := anchorAssetVersion
	appearanceAssetVersion.OwnerRef.OwnerLogicalID = "asset-version:character:linzhou:appearance-wounded"
	appearanceAssetVersion.OwnerRef.OwnerVersionID = uuid.NewString()
	appearanceAssetVersion.OwnerRef.OwnerContentHash = productionHash(appearanceAssetVersion.OwnerRef.OwnerLogicalID)
	appearanceAssetVersion.StoryNodeKey = mustNodeKey(t, appearanceAssetVersion.NodeType, appearanceAssetVersion.OwnerRef)
	mutateProductionPayload(t, &appearanceAssetVersion, func(payload map[string]any) {
		payload["projection_hash"] = appearanceAssetVersion.OwnerRef.OwnerContentHash
		payload["purpose"] = "character_appearance"
		payload["asset_state_ref"] = appearanceState.OwnerRef
		payload["identity_anchor_asset_version_ref"] = anchorAssetVersion.OwnerRef
		payload["selected_artifact_ref"] = appearanceArtifact.OwnerRef
		payload["fulfilled_reference_target_ref"] = appearanceTarget.OwnerRef
	})

	value.Graph.Nodes = append(value.Graph.Nodes, appearanceState, appearanceOccurrence, appearanceTarget, appearanceArtifact, appearanceAssetVersion)
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, appearanceState.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, appearanceOccurrence.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeHasState, asset.StoryNodeKey, appearanceState.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeMaterializes, appearanceState.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "state"}),
		newEdge(t, storygraph.EdgeTypeAnchorsOccurrence, scene.StoryNodeKey, appearanceOccurrence.StoryNodeKey, storygraph.EdgeQualifier{AnchorRole: "scene"}),
		newEdge(t, storygraph.EdgeTypeInstantiatesOccurrence, appearanceState.StoryNodeKey, appearanceOccurrence.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeContainsReferenceTarget, plan.StoryNodeKey, appearanceTarget.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: "target:0002"}),
		newEdge(t, storygraph.EdgeTypeDependsOnReferenceTarget, anchorTarget.StoryNodeKey, appearanceTarget.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypePlansReference, asset.StoryNodeKey, appearanceTarget.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "identity"}),
		newEdge(t, storygraph.EdgeTypePlansReference, specification.StoryNodeKey, appearanceTarget.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "specification"}),
		newEdge(t, storygraph.EdgeTypePlansReference, appearanceState.StoryNodeKey, appearanceTarget.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "state"}),
		newEdge(t, storygraph.EdgeTypePlansReference, style.StoryNodeKey, appearanceTarget.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "style"}),
		newEdge(t, storygraph.EdgeTypePlansReference, scene.StoryNodeKey, appearanceTarget.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "scene"}),
		newEdge(t, storygraph.EdgeTypePlansReference, appearanceOccurrence.StoryNodeKey, appearanceTarget.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "occurrence"}),
		newEdge(t, storygraph.EdgeTypeConstrains, policy.StoryNodeKey, appearanceTarget.StoryNodeKey, storygraph.EdgeQualifier{ConstraintRole: "policy"}),
		newEdge(t, storygraph.EdgeTypeConstrains, style.StoryNodeKey, appearanceTarget.StoryNodeKey, storygraph.EdgeQualifier{ConstraintRole: "style"}),
		newEdge(t, storygraph.EdgeTypeMaterializes, asset.StoryNodeKey, appearanceAssetVersion.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "asset"}),
		newEdge(t, storygraph.EdgeTypeMaterializes, specification.StoryNodeKey, appearanceAssetVersion.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "specification"}),
		newEdge(t, storygraph.EdgeTypeMaterializes, appearanceState.StoryNodeKey, appearanceAssetVersion.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "state"}),
		newEdge(t, storygraph.EdgeTypeMaterializes, anchorAssetVersion.StoryNodeKey, appearanceAssetVersion.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "identity_anchor"}),
		newEdge(t, storygraph.EdgeTypeMaterializes, appearanceArtifact.StoryNodeKey, appearanceAssetVersion.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "artifact"}),
		newEdge(t, storygraph.EdgeTypeFulfillsReferenceTarget, appearanceTarget.StoryNodeKey, appearanceAssetVersion.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeConstrains, policy.StoryNodeKey, appearanceAssetVersion.StoryNodeKey, storygraph.EdgeQualifier{ConstraintRole: "policy"}),
		newEdge(t, storygraph.EdgeTypeConstrains, style.StoryNodeKey, appearanceAssetVersion.StoryNodeKey, storygraph.EdgeQualifier{ConstraintRole: "style"}),
	)
	return &value.Graph.Nodes[len(value.Graph.Nodes)-1]
}
