package storygraph_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestBuildReferencePlanSeedInventoryDerivesBackendOwnedPlanningFacts(t *testing.T) {
	value := productionInteractionRelationFixture(t)
	addProductionLocationRelation(t, &value)
	addProductionAllKindReferencePlan(t, &value)
	addCharacterAppearanceTarget(t, &value, true)
	removeReferencePlanProjection(&value)
	sceneScope := productionNodeByType(t, &value, storygraph.NodeTypeScene).OwnerRef.OwnerLogicalID

	result, err := storygraph.BuildReferencePlanSeedInventory(storygraph.ReferencePlanSeedInventoryInput{
		OwnerSetHash: strings.Repeat("a", 64), P1ScopeKeys: []string{sceneScope}, Graph: value.Graph,
	})
	if err != nil {
		t.Fatalf("build Reference Plan seed inventory: %v", err)
	}
	if result.OwnerSetHash != strings.Repeat("a", 64) || !slices.Equal(result.P1ScopeKeys, []string{sceneScope}) ||
		len(result.CharacterSeeds) != 1 || len(result.CharacterSeeds[0].StateOptions) != 2 || len(result.FixedTargetSeeds) != 4 {
		t.Fatalf("unexpected Reference Plan seed inventory: %#v", result)
	}
	if !slices.ContainsFunc(result.FixedTargetSeeds, func(seed storygraph.ReferencePlanFixedTargetSeed) bool {
		return seed.TargetKind == "scene_composition" && len(seed.CharacterDependencies) == 2 &&
			len(seed.FixedDependencyBusinessKeys) == 2
	}) {
		t.Fatalf("Scene composition seed lacks its complete production closure: %#v", result.FixedTargetSeeds)
	}

	reordered := value.Graph
	slices.Reverse(reordered.Nodes)
	slices.Reverse(reordered.Edges)
	replayed, err := storygraph.BuildReferencePlanSeedInventory(storygraph.ReferencePlanSeedInventoryInput{
		OwnerSetHash: strings.Repeat("a", 64), P1ScopeKeys: []string{sceneScope}, Graph: reordered,
	})
	if err != nil || !reflect.DeepEqual(replayed, result) {
		t.Fatalf("Reference Plan seed inventory is not deterministic: replay=%#v err=%v", replayed, err)
	}
}

func TestBuildReferencePlanSeedInventoryRejectsUnknownP1Scope(t *testing.T) {
	value := productionInteractionRelationFixture(t)
	addProductionLocationRelation(t, &value)
	removeReferencePlanProjection(&value)
	_, err := storygraph.BuildReferencePlanSeedInventory(storygraph.ReferencePlanSeedInventoryInput{
		OwnerSetHash: strings.Repeat("a", 64), P1ScopeKeys: []string{"scene:unknown"}, Graph: value.Graph,
	})
	if err == nil || !strings.Contains(err.Error(), "ambiguous_expected_reference_scope") {
		t.Fatalf("unknown P1 scope was not rejected: %v", err)
	}
}

func removeReferencePlanProjection(value *storygraph.ProductionOwnerSnapshot) {
	removed := make(map[string]struct{})
	kept := value.Graph.Nodes[:0]
	for _, node := range value.Graph.Nodes {
		if node.NodeType == storygraph.NodeTypeApprovedReferencePlanVersion || node.NodeType == storygraph.NodeTypeReferencePlanTarget {
			removed[node.StoryNodeKey] = struct{}{}
			continue
		}
		kept = append(kept, node)
	}
	value.Graph.Nodes = kept
	value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
		_, fromRemoved := removed[edge.FromNodeKey]
		_, toRemoved := removed[edge.ToNodeKey]
		return fromRemoved || toRemoved
	})
}
