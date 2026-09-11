package agent_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestReferencePlanSchemaManifestMatchesAgentModels(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(current), "..", "fixtures", "agent", "storygraph-reference-plan-schemas.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, canonical, err := contract.DecodeReferencePlanSchemaManifest(raw)
	if err != nil || len(manifest.Schemas) != 2 || !bytes.Equal(append(canonical, '\n'), raw) {
		t.Fatalf("decode Reference Plan Schema manifest: %v", err)
	}
}

func TestReferencePlanCandidateCoversFrozenSeedInventory(t *testing.T) {
	input := referencePlanInput(t)
	candidate := referencePlanCandidate(input)
	if err := candidate.ValidateFor(input); err != nil {
		t.Fatalf("validate Reference Plan Candidate: %v", err)
	}
	raw := mustJSON(t, candidate)
	decoded, canonical, err := contract.DecodeReferencePlanCandidate(raw)
	if err != nil || len(canonical) == 0 || decoded.ReferenceTargetSeedRoot != input.ReferenceTargetSeedRoot {
		t.Fatalf("decode Reference Plan Candidate: %v", err)
	}
}

func TestReferencePlanCandidateRejectsSeedOrSpecificationDrift(t *testing.T) {
	input := referencePlanInput(t)
	tests := map[string]func(*contract.ReferencePlanCandidate){
		"unknown anchor state": func(value *contract.ReferencePlanCandidate) {
			value.AnchorSelections[0].SelectedStateRef.OwnerLogicalID = "state-unknown"
		},
		"missing target": func(value *contract.ReferencePlanCandidate) {
			value.TargetSpecifications = value.TargetSpecifications[1:]
		},
		"extra target": func(value *contract.ReferencePlanCandidate) {
			value.TargetSpecifications = append(value.TargetSpecifications, contract.ReferencePlanTargetSpecification{
				TargetBusinessKey: referencePlanBusinessKey("prop_sheet", "extra"), TargetKind: "prop_sheet",
				Fulfillment: "optional", DesignFocus: []string{"silhouette"},
				ForbiddenChanges: []string{"story_fact"}, DependsOnTargetBusinessKeys: []string{},
			})
		},
		"dependency drift": func(value *contract.ReferencePlanCandidate) {
			value.TargetSpecifications[0].DependsOnTargetBusinessKeys = []string{}
		},
		"purpose escape": func(value *contract.ReferencePlanCandidate) {
			value.TargetSpecifications[1].DesignFocus = []string{"invent_new_identity"}
		},
		"rank inversion": func(value *contract.ReferencePlanCandidate) {
			value.TargetSpecifications[1].Fulfillment = "optional"
			value.TargetSpecifications[0].Fulfillment = "required"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := referencePlanCandidate(input)
			mutate(&value)
			if err := value.ValidateFor(input); err == nil {
				t.Fatal("unsafe Reference Plan Candidate was accepted")
			}
		})
	}
}

func TestReferencePlanInputRejectsMutableOrIncompleteSeedInventory(t *testing.T) {
	input := referencePlanInput(t)
	input.ReferenceTargetSeedRoot = referencePlanHash("drift")
	if err := input.Validate(); err == nil {
		t.Fatal("drifted Reference Target seed root was accepted")
	}

	input = referencePlanInput(t)
	input.CharacterSeeds[0].StateOptions = input.CharacterSeeds[0].StateOptions[:1]
	if err := input.Validate(); err == nil {
		t.Fatal("incomplete Character State seed inventory was accepted")
	}
}

func TestReferencePlanCandidateRejectsUnknownFields(t *testing.T) {
	input := referencePlanInput(t)
	var value map[string]any
	if err := json.Unmarshal(mustJSON(t, referencePlanCandidate(input)), &value); err != nil {
		t.Fatal(err)
	}
	value["provider"] = "seedream"
	if _, _, err := contract.DecodeReferencePlanCandidate(mustJSON(t, value)); err == nil {
		t.Fatal("Provider field was accepted by Reference Plan Candidate")
	}
}

func referencePlanInput(t *testing.T) contract.ReferencePlanInput {
	t.Helper()
	workspaceID := "00000000-0000-0000-0000-000000000010"
	projectID := "00000000-0000-0000-0000-000000000001"
	character := referencePlanOwnerRef(workspaceID, projectID, "asset", "asset_identity_state_set", "character-1")
	characterSpec := referencePlanOwnerRef(workspaceID, projectID, "production/bible", "bible_production_world_set", "character-spec-1")
	stateA := referencePlanOwnerRef(workspaceID, projectID, "asset", "asset_identity_state_set", "state-a")
	stateB := referencePlanOwnerRef(workspaceID, projectID, "asset", "asset_identity_state_set", "state-b")
	sceneA := referencePlanOwnerRef(workspaceID, projectID, "production/planning", "planning_scene_set", "scene:00000000-0000-0000-0000-000000000101")
	sceneB := referencePlanOwnerRef(workspaceID, projectID, "production/planning", "planning_scene_set", "scene:00000000-0000-0000-0000-000000000102")
	occurrenceA := referencePlanOwnerRef(workspaceID, projectID, "production/planning", "planning_scene_set", "occurrence-a")
	occurrenceB := referencePlanOwnerRef(workspaceID, projectID, "production/planning", "planning_scene_set", "occurrence-b")
	location := referencePlanOwnerRef(workspaceID, projectID, "asset", "asset_identity_state_set", "location-1")
	locationSpec := referencePlanOwnerRef(workspaceID, projectID, "production/bible", "bible_production_world_set", "location-spec-1")
	locationState := referencePlanOwnerRef(workspaceID, projectID, "asset", "asset_identity_state_set", "location-state-1")
	locationOccurrence := referencePlanOwnerRef(workspaceID, projectID, "production/planning", "planning_scene_set", "location-occurrence-1")
	style := referencePlanOwnerRef(workspaceID, projectID, "preset", "preset_effective_set", "style-1")

	anchorKey := referencePlanBusinessKey("character_identity_anchor", "character-1")
	appearanceAKey := referencePlanBusinessKey("character_appearance", "character-1", "character-spec-1", "state-a")
	appearanceBKey := referencePlanBusinessKey("character_appearance", "character-1", "character-spec-1", "state-b")
	locationKey := referencePlanBusinessKey("location_board", "location-1", "location-spec-1", "location-state-1")
	sceneAKey := referencePlanBusinessKey("scene_composition", "scene-101")
	sceneBKey := referencePlanBusinessKey("scene_composition", "scene-102")

	input := contract.ReferencePlanInput{
		WorkspaceID: workspaceID, ProjectID: projectID,
		ProductionWorldOwnerSetHash:           referencePlanHash("owner-set"),
		P1ScopeKeys:                           []string{"scene:00000000-0000-0000-0000-000000000101", "scene:00000000-0000-0000-0000-000000000102"},
		VisualFoundationCandidateRevisionID:   "00000000-0000-0000-0000-000000000201",
		VisualFoundationCandidateRevisionHash: referencePlanHash("foundation-revision"),
		EffectiveStyleSnapshotRef:             style,
		CharacterSeeds: []contract.ReferencePlanCharacterSeed{{
			AnchorBusinessKey: anchorKey, IdentityRef: character, SpecificationRef: characterSpec,
			CoverageScopeKeys: []string{"scene:00000000-0000-0000-0000-000000000101", "scene:00000000-0000-0000-0000-000000000102"},
			StateOptions: []contract.ReferencePlanCharacterStateSeed{
				{StateRef: stateA, AppearanceBusinessKey: appearanceAKey, CoverageScopeKeys: []string{"scene:00000000-0000-0000-0000-000000000101"}, OccurrenceRefs: []contract.ReferencePlanOwnerRef{occurrenceA}},
				{StateRef: stateB, AppearanceBusinessKey: appearanceBKey, CoverageScopeKeys: []string{"scene:00000000-0000-0000-0000-000000000102"}, OccurrenceRefs: []contract.ReferencePlanOwnerRef{occurrenceB}},
			},
		}},
		FixedTargetSeeds: []contract.ReferencePlanFixedTargetSeed{
			{
				TargetBusinessKey: locationKey, TargetKind: "location_board",
				OwnerRefs:                   contract.ReferencePlanTargetOwnerRefs{Identity: []contract.ReferencePlanOwnerRef{location}, Specification: []contract.ReferencePlanOwnerRef{locationSpec}, State: []contract.ReferencePlanOwnerRef{locationState}, Scene: []contract.ReferencePlanOwnerRef{sceneA}, Occurrence: []contract.ReferencePlanOwnerRef{locationOccurrence}, Interaction: []contract.ReferencePlanOwnerRef{}},
				CoverageScopeKeys:           []string{"scene:00000000-0000-0000-0000-000000000101"},
				FixedDependencyBusinessKeys: []string{}, CharacterDependencies: []contract.ReferencePlanCharacterDependencySeed{},
			},
			{
				TargetBusinessKey: sceneAKey, TargetKind: "scene_composition",
				OwnerRefs:         contract.ReferencePlanTargetOwnerRefs{Identity: []contract.ReferencePlanOwnerRef{character, location}, Specification: []contract.ReferencePlanOwnerRef{characterSpec, locationSpec}, State: []contract.ReferencePlanOwnerRef{locationState, stateA}, Scene: []contract.ReferencePlanOwnerRef{sceneA}, Occurrence: []contract.ReferencePlanOwnerRef{locationOccurrence, occurrenceA}, Interaction: []contract.ReferencePlanOwnerRef{}},
				CoverageScopeKeys: []string{"scene:00000000-0000-0000-0000-000000000101"}, FixedDependencyBusinessKeys: []string{locationKey},
				CharacterDependencies: []contract.ReferencePlanCharacterDependencySeed{{AnchorBusinessKey: anchorKey, StateRef: stateA, AppearanceBusinessKey: appearanceAKey}},
			},
			{
				TargetBusinessKey: sceneBKey, TargetKind: "scene_composition",
				OwnerRefs:         contract.ReferencePlanTargetOwnerRefs{Identity: []contract.ReferencePlanOwnerRef{character}, Specification: []contract.ReferencePlanOwnerRef{characterSpec}, State: []contract.ReferencePlanOwnerRef{stateB}, Scene: []contract.ReferencePlanOwnerRef{sceneB}, Occurrence: []contract.ReferencePlanOwnerRef{occurrenceB}, Interaction: []contract.ReferencePlanOwnerRef{}},
				CoverageScopeKeys: []string{"scene:00000000-0000-0000-0000-000000000102"}, FixedDependencyBusinessKeys: []string{},
				CharacterDependencies: []contract.ReferencePlanCharacterDependencySeed{{AnchorBusinessKey: anchorKey, StateRef: stateB, AppearanceBusinessKey: appearanceBKey}},
			},
		},
		PurposeProfiles: referencePlanPurposeProfiles(),
	}
	root, err := input.ComputeSeedRoot()
	if err != nil {
		t.Fatal(err)
	}
	input.ReferenceTargetSeedRoot = root
	return input
}

func referencePlanCandidate(input contract.ReferencePlanInput) contract.ReferencePlanCandidate {
	anchorKey := input.CharacterSeeds[0].AnchorBusinessKey
	stateA := input.CharacterSeeds[0].StateOptions[0]
	appearanceBKey := input.CharacterSeeds[0].StateOptions[1].AppearanceBusinessKey
	locationKey := input.FixedTargetSeeds[0].TargetBusinessKey
	sceneAKey := input.FixedTargetSeeds[1].TargetBusinessKey
	sceneBKey := input.FixedTargetSeeds[2].TargetBusinessKey
	return contract.ReferencePlanCandidate{
		WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID,
		ProductionWorldOwnerSetHash:           input.ProductionWorldOwnerSetHash,
		P1ScopeKeys:                           append([]string(nil), input.P1ScopeKeys...),
		VisualFoundationCandidateRevisionID:   input.VisualFoundationCandidateRevisionID,
		VisualFoundationCandidateRevisionHash: input.VisualFoundationCandidateRevisionHash,
		ReferenceTargetSeedRoot:               input.ReferenceTargetSeedRoot,
		AnchorSelections:                      []contract.ReferencePlanAnchorSelection{{AnchorBusinessKey: anchorKey, SelectedStateRef: stateA.StateRef}},
		TargetSpecifications: []contract.ReferencePlanTargetSpecification{
			{TargetBusinessKey: appearanceBKey, TargetKind: "character_appearance", Fulfillment: "optional", DesignFocus: []string{"state_readability"}, ForbiddenChanges: []string{"story_fact"}, DependsOnTargetBusinessKeys: []string{anchorKey}},
			{TargetBusinessKey: anchorKey, TargetKind: "character_identity_anchor", Fulfillment: "required", DesignFocus: []string{"identity_readability", "silhouette"}, ForbiddenChanges: []string{"story_fact"}, DependsOnTargetBusinessKeys: []string{}},
			{TargetBusinessKey: locationKey, TargetKind: "location_board", Fulfillment: "required", DesignFocus: []string{"spatial_readability"}, ForbiddenChanges: []string{"story_fact"}, DependsOnTargetBusinessKeys: []string{}},
			{TargetBusinessKey: sceneAKey, TargetKind: "scene_composition", Fulfillment: "required", DesignFocus: []string{"composition"}, ForbiddenChanges: []string{"story_fact"}, DependsOnTargetBusinessKeys: []string{anchorKey, locationKey}},
			{TargetBusinessKey: sceneBKey, TargetKind: "scene_composition", Fulfillment: "optional", DesignFocus: []string{"composition"}, ForbiddenChanges: []string{"story_fact"}, DependsOnTargetBusinessKeys: []string{appearanceBKey}},
		},
	}
}

func referencePlanPurposeProfiles() []contract.ReferencePlanPurposeProfile {
	return []contract.ReferencePlanPurposeProfile{
		{TargetKind: "character_appearance", DesignFocus: []string{"state_readability"}, ForbiddenChanges: []string{"story_fact"}},
		{TargetKind: "character_identity_anchor", DesignFocus: []string{"identity_readability", "silhouette"}, ForbiddenChanges: []string{"story_fact"}},
		{TargetKind: "interaction_composition", DesignFocus: []string{"contact_geometry"}, ForbiddenChanges: []string{"story_fact"}},
		{TargetKind: "location_board", DesignFocus: []string{"spatial_readability"}, ForbiddenChanges: []string{"story_fact"}},
		{TargetKind: "prop_sheet", DesignFocus: []string{"state_readability"}, ForbiddenChanges: []string{"story_fact"}},
		{TargetKind: "scene_composition", DesignFocus: []string{"composition"}, ForbiddenChanges: []string{"story_fact"}},
	}
}

func referencePlanOwnerRef(workspaceID, projectID, ownerKind, family, logicalID string) contract.ReferencePlanOwnerRef {
	return contract.ReferencePlanOwnerRef{
		WorkspaceID: workspaceID, ProjectID: projectID, OwnerKind: ownerKind, VersionFamily: family,
		OwnerLogicalID: logicalID, OwnerVersionID: "00000000-0000-0000-0000-000000000301",
		OwnerRevision: 1, OwnerContentHash: referencePlanHash(ownerKind + family + logicalID),
	}
}

func referencePlanBusinessKey(kind string, parts ...string) string {
	value := []any{kind}
	for _, part := range parts {
		value = append(value, []string{"owner", "family", part, ""})
	}
	raw, _ := json.Marshal(value)
	return string(raw)
}

func referencePlanHash(seed string) string {
	value := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(value[:])
}
