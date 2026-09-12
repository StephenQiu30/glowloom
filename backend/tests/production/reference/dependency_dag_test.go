package reference_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	referencedomain "github.com/StephenQiu30/lanverse/backend/internal/production/reference/domain"
)

func TestReferenceTargetDependencyDAGBuildsDeterministicExecutionWaves(t *testing.T) {
	planID := uuid.NewString()
	anchor := referenceTarget(planID, "character_identity_anchor", "anchor", "required")
	appearance := referenceTarget(planID, "character_appearance", "appearance", "required", anchor.TargetBusinessKey)
	location := referenceTarget(planID, "location_board", "location", "required")
	prop := referenceTarget(planID, "prop_sheet", "prop", "required")
	interaction := referenceTarget(
		planID, "interaction_composition", "interaction", "required",
		appearance.TargetBusinessKey, prop.TargetBusinessKey,
	)
	scene := referenceTarget(
		planID, "scene_composition", "scene", "required",
		appearance.TargetBusinessKey, location.TargetBusinessKey, prop.TargetBusinessKey,
	)
	targets := []referencedomain.ReferencePlanTargetVersion{scene, prop, appearance, interaction, anchor, location}

	graph, err := referencedomain.BuildReferenceTargetDependencyDAG(planID, targets)
	if err != nil {
		t.Fatal(err)
	}
	if graph.SchemaVersion != referencedomain.ReferenceTargetDependencyDAGSchema ||
		graph.PlanVersionID != planID || len(graph.ContentHash) != 64 || len(graph.Nodes) != 6 || len(graph.Waves) != 3 {
		t.Fatalf("Reference Target dependency DAG is incomplete: %#v", graph)
	}
	wantWaves := [][]string{
		{anchor.TargetBusinessKey, location.TargetBusinessKey, prop.TargetBusinessKey},
		{appearance.TargetBusinessKey},
		{interaction.TargetBusinessKey, scene.TargetBusinessKey},
	}
	for index := range wantWaves {
		if !reflect.DeepEqual(graph.Waves[index].TargetBusinessKeys, wantWaves[index]) {
			t.Fatalf("wave %d = %v want %v", index, graph.Waves[index].TargetBusinessKeys, wantWaves[index])
		}
	}
	reversed := append([]referencedomain.ReferencePlanTargetVersion(nil), targets...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	replayed, err := referencedomain.BuildReferenceTargetDependencyDAG(planID, reversed)
	if err != nil || !reflect.DeepEqual(replayed, graph) {
		t.Fatalf("reordered Target input changed dependency DAG: got=%#v want=%#v err=%v", replayed, graph, err)
	}
}

func TestReferenceTargetDependencyDAGRejectsCyclesAndInvalidExecutionDependencies(t *testing.T) {
	planID := uuid.NewString()
	anchor := referenceTarget(planID, "character_identity_anchor", "anchor", "required")
	appearance := referenceTarget(planID, "character_appearance", "appearance", "required", anchor.TargetBusinessKey)
	scene := referenceTarget(planID, "scene_composition", "scene", "required", appearance.TargetBusinessKey)

	cyclicAnchor := anchor
	cyclicAnchor.DependsOnTargetBusinessKeys = []string{scene.TargetBusinessKey}
	if _, err := referencedomain.BuildReferenceTargetDependencyDAG(
		planID, []referencedomain.ReferencePlanTargetVersion{cyclicAnchor, appearance, scene},
	); err == nil {
		t.Fatal("Reference Target dependency DAG accepted a dependency cycle")
	}

	missing := scene
	missing.DependsOnTargetBusinessKeys = []string{`["prop_sheet","missing"]`}
	if _, err := referencedomain.BuildReferenceTargetDependencyDAG(
		planID, []referencedomain.ReferencePlanTargetVersion{anchor, appearance, missing},
	); err == nil {
		t.Fatal("Reference Target dependency DAG accepted a dependency outside the Plan")
	}

	notGenerated := referenceTarget(planID, "prop_sheet", "unused-prop", "not_generated")
	scene.DependsOnTargetBusinessKeys = []string{appearance.TargetBusinessKey, notGenerated.TargetBusinessKey}
	if _, err := referencedomain.BuildReferenceTargetDependencyDAG(
		planID, []referencedomain.ReferencePlanTargetVersion{anchor, appearance, notGenerated, scene},
	); err == nil {
		t.Fatal("Reference Target dependency DAG accepted a non-executable dependency")
	}
}

func referenceTarget(
	planID string,
	targetKind string,
	name string,
	fulfillment string,
	dependencies ...string,
) referencedomain.ReferencePlanTargetVersion {
	return referencedomain.ReferencePlanTargetVersion{
		ID: uuid.NewString(), PlanVersionID: planID,
		TargetBusinessKey: `["` + targetKind + `","` + name + `"]`, TargetKind: targetKind,
		Fulfillment: fulfillment, DependsOnTargetBusinessKeys: dependencies,
		ContentHash: strings.Repeat("a", 64),
	}
}
