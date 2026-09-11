package storygraph_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestBuildExpectedReferenceTargetSetDerivesAllTargetsFromP1Occurrences(t *testing.T) {
	value := productionInteractionRelationFixture(t)
	addProductionLocationRelation(t, &value)
	addProductionAllKindReferencePlan(t, &value)
	scene := productionNodeByType(t, &value, storygraph.NodeTypeScene)
	sceneScope := scene.OwnerRef.OwnerLogicalID

	result, err := storygraph.BuildExpectedReferenceTargetSet(storygraph.ExpectedReferenceTargetSetInput{
		OwnerSetHash: strings.Repeat("a", 64),
		P1ScopeKeys:  []string{sceneScope},
		Graph:        value.Graph,
	})
	if err != nil {
		t.Fatalf("build expected Reference Target set: %v", err)
	}
	if result.OwnerSetHash != strings.Repeat("a", 64) ||
		!slices.Equal(result.P1ScopeKeys, []string{sceneScope}) ||
		len(result.ExpectedTargetBusinessKeys) != 5 ||
		len(result.ExpectedTargetKeyRoot) != 64 ||
		!slices.IsSorted(result.ExpectedTargetBusinessKeys) {
		t.Fatalf("unexpected Reference Target set: %#v", result)
	}

	reordered := value.Graph
	slices.Reverse(reordered.Nodes)
	slices.Reverse(reordered.Edges)
	replayed, err := storygraph.BuildExpectedReferenceTargetSet(storygraph.ExpectedReferenceTargetSetInput{
		OwnerSetHash: strings.Repeat("a", 64),
		P1ScopeKeys:  []string{sceneScope},
		Graph:        reordered,
	})
	if err != nil || replayed.ExpectedTargetKeyRoot != result.ExpectedTargetKeyRoot ||
		!slices.Equal(replayed.ExpectedTargetBusinessKeys, result.ExpectedTargetBusinessKeys) {
		t.Fatalf("expected target proof is not deterministic: first=%#v replay=%#v err=%v", result, replayed, err)
	}
}

func TestBuildExpectedReferenceTargetSetUsesReviewedAnchorStateForSecondPass(t *testing.T) {
	value := productionInteractionRelationFixture(t)
	addProductionLocationRelation(t, &value)
	addProductionAllKindReferencePlan(t, &value)
	addCharacterAppearanceTarget(t, &value, true)
	sceneScope := productionNodeByType(t, &value, storygraph.NodeTypeScene).OwnerRef.OwnerLogicalID

	result, err := storygraph.BuildExpectedReferenceTargetSet(storygraph.ExpectedReferenceTargetSetInput{
		OwnerSetHash: strings.Repeat("b", 64), P1ScopeKeys: []string{sceneScope}, Graph: value.Graph,
	})
	if err != nil || len(result.ExpectedTargetBusinessKeys) != 6 {
		t.Fatalf("second-pass Character appearance target was not derived: result=%#v err=%v", result, err)
	}
	if !slices.ContainsFunc(result.ExpectedTargetBusinessKeys, func(key string) bool {
		return strings.Contains(key, `"character_appearance"`) && strings.Contains(key, "state:character-alternate-appearance")
	}) {
		t.Fatalf("appearance business key does not bind the non-anchor state: %v", result.ExpectedTargetBusinessKeys)
	}
}

func TestBuildExpectedReferenceTargetSetRejectsMissingTargetAndScopeDrift(t *testing.T) {
	for name, testCase := range map[string]struct {
		mutate    func(*storygraph.ProductionOwnerSnapshot)
		errorCode string
	}{
		"missing Scene target": {
			mutate: func(value *storygraph.ProductionOwnerSnapshot) {
				target := productionNodeByFragment(t, value, storygraph.NodeTypeReferencePlanTarget, "target:scene-composition")
				value.Graph.Nodes = removeProductionNode(value.Graph.Nodes, target.StoryNodeKey)
				value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
					return edge.FromNodeKey == target.StoryNodeKey || edge.ToNodeKey == target.StoryNodeKey
				})
			},
			errorCode: "missing_expected_reference_target",
		},
		"character Anchor scope drift": {
			mutate: func(value *storygraph.ProductionOwnerSnapshot) {
				target := productionNodeByFragment(t, value, storygraph.NodeTypeReferencePlanTarget, "target:character-anchor")
				mutateProductionPayload(t, target, func(payload map[string]any) {
					payload["coverage_scope_keys"] = []string{"scene:outside"}
				})
			},
			errorCode: "reference_target_input_mismatch",
		},
		"character Anchor occurrence omitted": {
			mutate: func(value *storygraph.ProductionOwnerSnapshot) {
				target := productionNodeByFragment(t, value, storygraph.NodeTypeReferencePlanTarget, "target:character-anchor")
				occurrence := productionOccurrenceByAssetKind(t, value, "character")
				removeProductionTargetOwnerRef(t, target, "occurrence", occurrence.OwnerRef)
				value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
					return edge.EdgeType == storygraph.EdgeTypePlansReference && edge.FromNodeKey == occurrence.StoryNodeKey && edge.ToNodeKey == target.StoryNodeKey
				})
			},
			errorCode: "reference_target_input_mismatch",
		},
		"unexpected target without occurrence": {
			mutate: func(value *storygraph.ProductionOwnerSnapshot) {
				addCharacterAppearanceTarget(t, value, false)
			},
			errorCode: "unexpected_reference_target",
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := productionInteractionRelationFixture(t)
			addProductionLocationRelation(t, &value)
			addProductionAllKindReferencePlan(t, &value)
			scene := productionNodeByType(t, &value, storygraph.NodeTypeScene)
			testCase.mutate(&value)

			_, err := storygraph.BuildExpectedReferenceTargetSet(storygraph.ExpectedReferenceTargetSetInput{
				OwnerSetHash: strings.Repeat("a", 64),
				P1ScopeKeys:  []string{scene.OwnerRef.OwnerLogicalID},
				Graph:        value.Graph,
			})
			if err == nil || !strings.Contains(err.Error(), testCase.errorCode) {
				t.Fatalf("expected %s, got %v", testCase.errorCode, err)
			}
		})
	}
}

func addCharacterAppearanceTarget(t *testing.T, value *storygraph.ProductionOwnerSnapshot, withOccurrence bool) {
	t.Helper()
	character := productionAssetNodeByKind(t, value, storygraph.NodeTypeAssetIdentity, "character")
	specification := productionAssetNodeByKind(t, value, storygraph.NodeTypeCharacterSpecification, "character")
	state := *productionAssetNodeByKind(t, value, storygraph.NodeTypeAssetState, "character")
	occurrence := *productionOccurrenceByAssetKind(t, value, "character")
	scene := productionNodeByType(t, value, storygraph.NodeTypeScene)
	evidence := productionNodeByType(t, value, storygraph.NodeTypeSourceEvidence)
	anchor := productionNodeByFragment(t, value, storygraph.NodeTypeReferencePlanTarget, "target:character-anchor")
	plan := productionNodeByType(t, value, storygraph.NodeTypeApprovedReferencePlanVersion)
	policy := productionNodeByType(t, value, storygraph.NodeTypePolicySnapshot)
	style := productionNodeByType(t, value, storygraph.NodeTypeEffectiveStyleSnapshot)

	state.OwnerRef.FragmentKey = "state:character-alternate-appearance"
	state.OwnerRef.FragmentContentHash = productionHash(state.OwnerRef.FragmentKey)
	state.StoryNodeKey = mustNodeKey(t, state.NodeType, state.OwnerRef)
	mutateProductionPayload(t, &state, func(payload map[string]any) {
		payload["projection_hash"] = state.OwnerRef.FragmentContentHash
		payload["state_key"] = "state:character-alternate-appearance"
	})
	binding := productionBindingByAsset(t, value, character.OwnerRef)
	mutateProductionPayload(t, binding, func(payload map[string]any) {
		payload["state_refs"] = appendSortedProductionOwnerRef(t, payload["state_refs"], state.OwnerRef)
	})
	value.Graph.Nodes = append(value.Graph.Nodes, state)
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, state.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeHasState, character.StoryNodeKey, state.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeMaterializes, state.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "state"}),
	)
	occurrences := []*storygraph.Node{}
	if withOccurrence {
		occurrence.OwnerRef.FragmentKey = "occurrence:character-alternate-appearance"
		occurrence.OwnerRef.FragmentContentHash = productionHash(occurrence.OwnerRef.FragmentKey)
		occurrence.StoryNodeKey = mustNodeKey(t, occurrence.NodeType, occurrence.OwnerRef)
		mutateProductionPayload(t, &occurrence, func(payload map[string]any) {
			payload["projection_hash"] = occurrence.OwnerRef.FragmentContentHash
			payload["asset_state_ref"] = state.OwnerRef
		})
		value.Graph.Nodes = append(value.Graph.Nodes, occurrence)
		value.Graph.Edges = append(value.Graph.Edges,
			newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, occurrence.StoryNodeKey, storygraph.EdgeQualifier{}),
			newEdge(t, storygraph.EdgeTypeAnchorsOccurrence, scene.StoryNodeKey, occurrence.StoryNodeKey, storygraph.EdgeQualifier{AnchorRole: "scene"}),
			newEdge(t, storygraph.EdgeTypeInstantiatesOccurrence, state.StoryNodeKey, occurrence.StoryNodeKey, storygraph.EdgeQualifier{}),
		)
		occurrences = []*storygraph.Node{&occurrence}
	}

	constraints := map[string]any{
		"world_rule_refs": []storygraph.OwnerRef{}, "policy_snapshot_ref": policy.OwnerRef,
		"effective_style_snapshot_ref": style.OwnerRef,
	}
	fulfillment := "optional"
	if withOccurrence {
		fulfillment = "required"
	}
	input := productionReferenceTargetTestInput{
		kind: "character_appearance", fragment: "target:character-alternate-appearance", fulfillment: fulfillment,
		identities: []*storygraph.Node{character}, specifications: []*storygraph.Node{specification},
		states: []*storygraph.Node{&state}, scenes: []*storygraph.Node{scene}, occurrences: occurrences,
		interactions: []*storygraph.Node{}, dependencies: []*storygraph.Node{anchor},
	}
	target := addProductionReferenceTargetNode(t, value, plan.OwnerRef, style.OwnerRef, constraints, input)
	value.Graph.Nodes = append(value.Graph.Nodes, target)
	addProductionReferenceTargetEdges(t, value, *plan, *policy, *style, &target, input, 6)
	if withOccurrence {
		sceneTarget := productionNodeByFragment(t, value, storygraph.NodeTypeReferencePlanTarget, "target:scene-composition")
		addProductionTargetOwnerRef(t, sceneTarget, "state", state.OwnerRef)
		addProductionTargetOwnerRef(t, sceneTarget, "occurrence", occurrence.OwnerRef)
		addProductionOwnerRefToPayload(t, sceneTarget, "depends_on_target_refs", target.OwnerRef)
		value.Graph.Edges = append(value.Graph.Edges,
			newEdge(t, storygraph.EdgeTypePlansReference, state.StoryNodeKey, sceneTarget.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "state"}),
			newEdge(t, storygraph.EdgeTypePlansReference, occurrence.StoryNodeKey, sceneTarget.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: "occurrence"}),
			newEdge(t, storygraph.EdgeTypeDependsOnReferenceTarget, target.StoryNodeKey, sceneTarget.StoryNodeKey, storygraph.EdgeQualifier{}),
		)
	}
}

func productionBindingByAsset(t *testing.T, value *storygraph.ProductionOwnerSnapshot, asset storygraph.OwnerRef) *storygraph.Node {
	t.Helper()
	for index := range value.Graph.Nodes {
		node := &value.Graph.Nodes[index]
		if node.NodeType != storygraph.NodeTypeProductionBinding {
			continue
		}
		var payload struct {
			AssetIdentityRef storygraph.OwnerRef `json:"asset_identity_ref"`
		}
		if json.Unmarshal(node.Payload, &payload) == nil && productionOwnerRefsEqual(payload.AssetIdentityRef, asset) {
			return node
		}
	}
	t.Fatal("missing Production Binding for Asset")
	return nil
}
