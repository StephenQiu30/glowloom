package storygraph_test

import (
	"encoding/json"
	"testing"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestProductionShotBindingRejectsPayloadAndRelationDrift(t *testing.T) {
	tests := map[string]func(*storygraph.ProductionOwnerSnapshot, []*storygraph.Node){
		"unknown payload field": func(_ *storygraph.ProductionOwnerSnapshot, bindings []*storygraph.Node) {
			mutateProductionPayload(t, bindings[0], func(payload map[string]any) { payload["current_assets"] = true })
		},
		"occurrence closure omitted": func(value *storygraph.ProductionOwnerSnapshot, bindings []*storygraph.Node) {
			occurrence := productionOccurrenceByAssetKind(t, value, "character")
			removeProductionOwnerRefFromPayload(t, bindings[0], "occurrence_refs", occurrence.OwnerRef)
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.FromNodeKey == occurrence.StoryNodeKey && edge.ToNodeKey == bindings[0].StoryNodeKey &&
					(edge.EdgeType == storygraph.EdgeTypeInforms || edge.EdgeType == storygraph.EdgeTypeBindsInput)
			})
		},
		"asset version omitted": func(value *storygraph.ProductionOwnerSnapshot, bindings []*storygraph.Node) {
			assetVersion := productionAssetVersionByPurpose(t, value, "character_identity_anchor")
			removeProductionOwnerRefFromPayload(t, bindings[0], "asset_version_refs", assetVersion.OwnerRef)
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeBindsInput && edge.FromNodeKey == assetVersion.StoryNodeKey && edge.ToNodeKey == bindings[0].StoryNodeKey
			})
		},
		"Shot input edge omitted": func(value *storygraph.ProductionOwnerSnapshot, bindings []*storygraph.Node) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeBindsInput && edge.ToNodeKey == bindings[0].StoryNodeKey && edge.Qualifier.BindingRole == "shot"
			})
		},
		"occurrence informs edge omitted": func(value *storygraph.ProductionOwnerSnapshot, bindings []*storygraph.Node) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeInforms && edge.ToNodeKey == bindings[0].StoryNodeKey && edge.Qualifier.InformsRole == "occurrence"
			})
		},
		"style input edge omitted": func(value *storygraph.ProductionOwnerSnapshot, bindings []*storygraph.Node) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeBindsInput && edge.ToNodeKey == bindings[0].StoryNodeKey && edge.Qualifier.BindingRole == "style"
			})
		},
		"policy constraint edge omitted": func(value *storygraph.ProductionOwnerSnapshot, bindings []*storygraph.Node) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeConstrains && edge.ToNodeKey == bindings[0].StoryNodeKey && edge.Qualifier.ConstraintRole == "policy"
			})
		},
		"one Shot binding omitted": func(value *storygraph.ProductionOwnerSnapshot, bindings []*storygraph.Node) {
			value.Graph.Nodes = removeProductionNode(value.Graph.Nodes, bindings[0].StoryNodeKey)
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.FromNodeKey == bindings[0].StoryNodeKey || edge.ToNodeKey == bindings[0].StoryNodeKey
			})
		},
		"one Shot has duplicate bindings": func(value *storygraph.ProductionOwnerSnapshot, bindings []*storygraph.Node) {
			originalKey := bindings[0].StoryNodeKey
			duplicate := *bindings[0]
			duplicate.OwnerRef.FragmentKey += ":duplicate"
			duplicate.OwnerRef.FragmentContentHash = productionHash(duplicate.OwnerRef.FragmentKey)
			duplicate.StoryNodeKey = mustNodeKey(t, storygraph.NodeTypeShotProductionBindingVersion, duplicate.OwnerRef)
			mutateProductionPayload(t, &duplicate, func(payload map[string]any) {
				payload["projection_hash"] = duplicate.OwnerRef.FragmentContentHash
			})
			value.Graph.Nodes = append(value.Graph.Nodes, duplicate)
			edges := append([]storygraph.Edge(nil), value.Graph.Edges...)
			for _, edge := range edges {
				if edge.ToNodeKey == originalKey {
					value.Graph.Edges = append(value.Graph.Edges, newEdge(t, edge.EdgeType, edge.FromNodeKey, duplicate.StoryNodeKey, edge.Qualifier))
				}
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value, bindings := productionShotBindingFixture(t)
			mutate(&value, bindings)
			if _, err := storygraph.Canonicalize(value.Graph); err == nil {
				t.Fatal("invalid Production Shot Binding was accepted")
			}
		})
	}
}

func TestProductionShotBindingAcceptsExactInputClosure(t *testing.T) {
	value, _ := productionShotBindingFixture(t)
	if _, err := storygraph.Canonicalize(value.Graph); err != nil {
		t.Fatalf("exact Production Shot Bindings were rejected: %v", err)
	}
}

func productionShotBindingFixture(t *testing.T) (storygraph.ProductionOwnerSnapshot, []*storygraph.Node) {
	t.Helper()
	value, shots := productionShotBaseFixture(t)
	characterOccurrence := productionOccurrenceByAssetKind(t, &value, "character")
	locationOccurrence := productionOccurrenceByAssetKind(t, &value, "location")
	propOccurrence := productionOccurrenceByAssetKind(t, &value, "prop")
	characterVersion := productionAssetVersionByPurpose(t, &value, "character_identity_anchor")
	locationVersion := productionAssetVersionByPurpose(t, &value, "location_board")
	propVersion := productionAssetVersionByPurpose(t, &value, "prop_sheet")
	sceneBinding := productionNodeByType(t, &value, storygraph.NodeTypeSceneReferenceBindingVersion)
	interactionBinding := productionNodeByType(t, &value, storygraph.NodeTypeInteractionReferenceBindingVersion)
	policy := productionNodeByType(t, &value, storygraph.NodeTypePolicySnapshot)
	style := productionNodeByType(t, &value, storygraph.NodeTypeEffectiveStyleSnapshot)
	occurrences := []*storygraph.Node{characterOccurrence, locationOccurrence, propOccurrence}
	assetVersions := []*storygraph.Node{characterVersion, locationVersion, propVersion}
	ownerRefs := func(nodes []*storygraph.Node) []storygraph.OwnerRef {
		refs := make([]storygraph.OwnerRef, 0, len(nodes))
		for _, node := range nodes {
			refs = append(refs, node.OwnerRef)
		}
		sortProductionOwnerRefs(refs)
		return refs
	}
	constraints := map[string]any{"world_rule_refs": []storygraph.OwnerRef{}, "policy_snapshot_ref": policy.OwnerRef, "effective_style_snapshot_ref": style.OwnerRef}
	bindings := make([]storygraph.Node, 0, len(shots))
	for _, shot := range shots {
		ref := shot.OwnerRef
		ref.FragmentKey = "binding:" + shot.OwnerRef.FragmentKey
		ref.FragmentContentHash = productionHash(ref.FragmentKey)
		raw, err := json.Marshal(map[string]any{
			"payload_contract_id": "storygraph-production/shot-production-binding-payload-contract",
			"projection_hash":     ref.FragmentContentHash, "shot_ref": shot.OwnerRef,
			"occurrence_refs": ownerRefs(occurrences), "asset_version_refs": ownerRefs(assetVersions),
			"scene_reference_binding_refs":       []storygraph.OwnerRef{sceneBinding.OwnerRef},
			"interaction_reference_binding_refs": []storygraph.OwnerRef{interactionBinding.OwnerRef},
			"effective_style_snapshot_ref":       style.OwnerRef, "constraints": constraints,
		})
		if err != nil {
			t.Fatal(err)
		}
		binding := storygraph.Node{StoryNodeKey: mustNodeKey(t, storygraph.NodeTypeShotProductionBindingVersion, ref), NodeType: storygraph.NodeTypeShotProductionBindingVersion, OwnerRef: ref, EvidenceRefs: []storygraph.EvidenceRef{}, Payload: raw}
		bindings = append(bindings, binding)
		value.Graph.Edges = append(value.Graph.Edges,
			newEdge(t, storygraph.EdgeTypeBindsInput, shot.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "shot"}),
			newEdge(t, storygraph.EdgeTypeBindsInput, style.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "style"}),
			newEdge(t, storygraph.EdgeTypeBindsInput, sceneBinding.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "scene_reference"}),
			newEdge(t, storygraph.EdgeTypeBindsInput, interactionBinding.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "interaction_reference"}),
			newEdge(t, storygraph.EdgeTypeInforms, sceneBinding.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{InformsRole: "scene_reference"}),
			newEdge(t, storygraph.EdgeTypeInforms, interactionBinding.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{InformsRole: "interaction_reference"}),
			newEdge(t, storygraph.EdgeTypeConstrains, policy.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{ConstraintRole: "policy"}),
			newEdge(t, storygraph.EdgeTypeConstrains, style.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{ConstraintRole: "style"}),
		)
		for _, occurrence := range occurrences {
			value.Graph.Edges = append(value.Graph.Edges,
				newEdge(t, storygraph.EdgeTypeBindsInput, occurrence.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "occurrence"}),
				newEdge(t, storygraph.EdgeTypeInforms, occurrence.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{InformsRole: "occurrence"}),
			)
		}
		for _, assetVersion := range assetVersions {
			value.Graph.Edges = append(value.Graph.Edges, newEdge(t, storygraph.EdgeTypeBindsInput, assetVersion.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "asset_version"}))
		}
	}
	value.Graph.Nodes = append(value.Graph.Nodes, bindings...)
	result := make([]*storygraph.Node, 0, len(bindings))
	for index := len(value.Graph.Nodes) - len(bindings); index < len(value.Graph.Nodes); index++ {
		result = append(result, &value.Graph.Nodes[index])
	}
	return value, result
}

func removeProductionOwnerRefFromPayload(t *testing.T, node *storygraph.Node, field string, removed storygraph.OwnerRef) {
	t.Helper()
	mutateProductionPayload(t, node, func(payload map[string]any) {
		raw, err := json.Marshal(payload[field])
		if err != nil {
			t.Fatal(err)
		}
		var refs []storygraph.OwnerRef
		if err = json.Unmarshal(raw, &refs); err != nil {
			t.Fatal(err)
		}
		result := make([]storygraph.OwnerRef, 0, len(refs))
		for _, ref := range refs {
			if !productionOwnerRefsEqual(ref, removed) {
				result = append(result, ref)
			}
		}
		payload[field] = result
	})
}
