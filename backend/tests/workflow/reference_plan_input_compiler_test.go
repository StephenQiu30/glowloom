package workflow_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	storygraphdomain "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
)

func TestCompileReferencePlanInputFromFrozenWorldAndCandidate(t *testing.T) {
	release := curatedFaithfulRelease(t)
	inventory := referencePlanInventoryFixture()
	revision := referencePlanVisualFoundationRevision(t, inventory, release.ContentHash)

	input, canonical, err := workflowapp.CompileReferencePlanInput(workflowapp.ReferencePlanInputCommand{
		Inventory: inventory, VisualFoundationRevision: revision, PresetRelease: release,
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.ProductionWorldOwnerSetHash != inventory.OwnerSetHash ||
		input.VisualFoundationCandidateRevisionID != revision.ID ||
		input.VisualFoundationCandidateRevisionHash != revision.RevisionHash ||
		input.VisualFoundationCandidate.StylePolicy.CameraRules[0] != "grounded_cinematic_camera" ||
		len(input.CharacterSeeds) != 1 || len(input.FixedTargetSeeds) != 1 || len(input.PurposeProfiles) != 6 ||
		input.PurposeProfiles[0].TargetKind != "character_appearance" ||
		input.PurposeProfiles[5].TargetKind != "scene_composition" || input.ReferenceTargetSeedRoot == "" {
		t.Fatalf("compiled Reference Plan input is incomplete: %#v", input)
	}
	decoded, decodedCanonical, err := agentcontract.DecodeReferencePlanInput(canonical)
	if err != nil || decoded.ReferenceTargetSeedRoot != input.ReferenceTargetSeedRoot ||
		!bytes.Equal(decodedCanonical, canonical) || bytes.Contains(canonical, []byte("effective_style_snapshot_ref")) {
		t.Fatalf("decode compiled Reference Plan input: decoded=%#v err=%v", decoded, err)
	}
}

func TestCompileReferencePlanInputRejectsCandidateOrPresetDrift(t *testing.T) {
	release := curatedFaithfulRelease(t)
	inventory := referencePlanInventoryFixture()
	revision := referencePlanVisualFoundationRevision(t, inventory, release.ContentHash)

	driftedContent := revision
	driftedContent.CandidateContentHash = strings.Repeat("f", 64)
	if _, _, err := workflowapp.CompileReferencePlanInput(workflowapp.ReferencePlanInputCommand{
		Inventory: inventory, VisualFoundationRevision: driftedContent, PresetRelease: release,
	}); err == nil {
		t.Fatal("compiled Reference Plan input from a drifted Candidate content hash")
	}

	driftedRelease := release
	driftedRelease.ContentHash = strings.Repeat("e", 64)
	if _, _, err := workflowapp.CompileReferencePlanInput(workflowapp.ReferencePlanInputCommand{
		Inventory: inventory, VisualFoundationRevision: revision, PresetRelease: driftedRelease,
	}); err == nil {
		t.Fatal("compiled Reference Plan input from a drifted Preset release")
	}

	driftedInventory := inventory
	driftedInventory.OwnerSetHash = strings.Repeat("d", 64)
	if _, _, err := workflowapp.CompileReferencePlanInput(workflowapp.ReferencePlanInputCommand{
		Inventory: driftedInventory, VisualFoundationRevision: revision, PresetRelease: release,
	}); err == nil {
		t.Fatal("compiled Reference Plan input from a different Production World Owner Set")
	}
}

func referencePlanVisualFoundationRevision(
	t *testing.T,
	inventory storygraphdomain.ReferencePlanSeedInventory,
	presetContentHash string,
) workflowapp.ReferencePlanVisualFoundationCandidateRevision {
	t.Helper()
	workspaceID := inventory.CharacterSeeds[0].IdentityRef.WorkspaceID
	projectID := inventory.CharacterSeeds[0].IdentityRef.ProjectID
	candidate := agentcontract.VisualFoundationCandidate{
		WorkspaceID: workspaceID, ProjectID: projectID,
		ProductionWorldOwnerSetHash: inventory.OwnerSetHash,
		PresetReleaseContentHash:    presetContentHash, ApplicationMode: "faithful",
		TypedOverridesHash:       referencePlanCompilerHash("typed-overrides"),
		ReferenceAttachmentsHash: referencePlanCompilerHash("reference-attachments"),
		FidelityInvariants:       []string{"character_identity", "holder_relation", "scene_continuity", "story_fact"},
		StylePolicy: agentcontract.VisualFoundationPolicy{
			PaletteRules: []string{"neutral_city_palette"}, MaterialRules: []string{"grounded_material_response"},
			LightingRules: []string{"motivated_cinematic_light"}, CameraRules: []string{"grounded_cinematic_camera"},
			ForbiddenChanges: []string{"character_identity", "holder_relation", "scene_continuity", "story_fact"},
		},
		WorldAdaptations: []agentcontract.WorldAdaptationProposal{},
		WorldConflicts:   []agentcontract.VisualWorldConflict{}, CreativeFillProposals: []agentcontract.CreativeFillProposal{},
	}
	raw, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	contentHash, err := agentcontract.ProductionCanonicalHash(raw)
	if err != nil {
		t.Fatal(err)
	}
	return workflowapp.ReferencePlanVisualFoundationCandidateRevision{
		ID: uuid.NewString(), RevisionHash: referencePlanCompilerHash("visual-foundation-revision"),
		CandidateContentHash: contentHash, Candidate: raw,
	}
}

func referencePlanInventoryFixture() storygraphdomain.ReferencePlanSeedInventory {
	workspaceID := "00000000-0000-0000-0000-000000000010"
	projectID := "00000000-0000-0000-0000-000000000001"
	scope := "scene:00000000-0000-0000-0000-000000000101"
	identity := referencePlanCompilerOwnerRef(workspaceID, projectID, "asset", "asset_identity_state_set", "character-1")
	specification := referencePlanCompilerOwnerRef(workspaceID, projectID, "production/bible", "bible_production_world_set", "character-spec-1")
	state := referencePlanCompilerOwnerRef(workspaceID, projectID, "asset", "asset_identity_state_set", "state-a")
	scene := referencePlanCompilerOwnerRef(workspaceID, projectID, "production/planning", "planning_scene_set", scope)
	occurrence := referencePlanCompilerOwnerRef(workspaceID, projectID, "production/planning", "planning_scene_set", "occurrence-a")
	anchorKey := referencePlanCompilerBusinessKey("character_identity_anchor", identity)
	appearanceKey := referencePlanCompilerBusinessKey("character_appearance", identity, specification, state)
	return storygraphdomain.ReferencePlanSeedInventory{
		OwnerSetHash: referencePlanCompilerHash("owner-set"), P1ScopeKeys: []string{scope},
		CharacterSeeds: []storygraphdomain.ReferencePlanCharacterSeed{{
			AnchorBusinessKey: anchorKey, IdentityRef: identity, SpecificationRef: specification,
			CoverageScopeKeys: []string{scope},
			StateOptions: []storygraphdomain.ReferencePlanCharacterStateSeed{{
				StateRef: state, AppearanceBusinessKey: appearanceKey,
				CoverageScopeKeys: []string{scope}, OccurrenceRefs: []storygraphdomain.OwnerRef{occurrence},
			}},
		}},
		FixedTargetSeeds: []storygraphdomain.ReferencePlanFixedTargetSeed{{
			TargetBusinessKey: referencePlanCompilerBusinessKey("scene_composition", scene),
			TargetKind:        "scene_composition",
			OwnerRefs: storygraphdomain.ReferencePlanTargetOwnerRefs{
				Identity: []storygraphdomain.OwnerRef{identity}, Specification: []storygraphdomain.OwnerRef{specification},
				State: []storygraphdomain.OwnerRef{state}, Scene: []storygraphdomain.OwnerRef{scene},
				Occurrence: []storygraphdomain.OwnerRef{occurrence}, Interaction: []storygraphdomain.OwnerRef{},
			},
			CoverageScopeKeys: []string{scope}, FixedDependencyBusinessKeys: []string{},
			CharacterDependencies: []storygraphdomain.ReferencePlanCharacterDependencySeed{{
				AnchorBusinessKey: anchorKey, StateRef: state, AppearanceBusinessKey: appearanceKey,
			}},
		}},
	}
}

func referencePlanCompilerOwnerRef(workspaceID, projectID, ownerKind, family, logicalID string) storygraphdomain.OwnerRef {
	return storygraphdomain.OwnerRef{
		WorkspaceID: workspaceID, ProjectID: projectID, OwnerKind: ownerKind, VersionFamily: family,
		OwnerLogicalID: logicalID, OwnerVersionID: "00000000-0000-0000-0000-000000000301",
		OwnerRevision: 1, OwnerContentHash: referencePlanCompilerHash(ownerKind + family + logicalID),
	}
}

func referencePlanCompilerBusinessKey(kind string, refs ...storygraphdomain.OwnerRef) string {
	value := []any{kind}
	for _, ref := range refs {
		value = append(value, []string{ref.OwnerKind, ref.VersionFamily, ref.OwnerLogicalID, ref.FragmentKey})
	}
	raw, _ := json.Marshal(value)
	return string(raw)
}

func referencePlanCompilerHash(seed string) string {
	value := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(value[:])
}
