package workflow_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	presetcatalog "github.com/StephenQiu30/lanverse/backend/internal/preset/catalog"
	presetdomain "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
	storygraphdomain "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
)

func TestCompileFaithfulVisualFoundationInputFromConfirmedWorld(t *testing.T) {
	candidate, source, world := faithfulVisualFoundationSources(t, false)
	release := curatedFaithfulRelease(t)

	input, canonical, err := workflowapp.CompileFaithfulVisualFoundationInput(
		workflowapp.FaithfulVisualFoundationInputCommand{
			World: world, Source: source, PresetRelease: release,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if input.WorkspaceID != candidate.WorkspaceID || input.ProjectID != candidate.ProjectID ||
		input.ProductionWorldOwnerSetHash != world.ProductionWorldOwnerSetHash ||
		input.PresetRelease.Key != release.Key || input.PresetRelease.Release != release.Release ||
		input.PresetRelease.ContentHash != release.ContentHash || input.ApplicationMode != "faithful" ||
		len(input.ConfirmedWorldRoots) != 3 || len(input.AdaptationRules) != len(release.WorldAdaptationRules) ||
		input.TypedOverrides == nil || len(input.TypedOverrides) != 0 ||
		input.ReferenceAttachments == nil || len(input.ReferenceAttachments) != 0 ||
		input.ConfirmedWorldFacts == nil || len(input.ConfirmedWorldFacts) != 0 ||
		input.DesignGaps == nil || len(input.DesignGaps) != 0 {
		t.Fatalf("faithful Visual Foundation input is incomplete: %#v", input)
	}
	decoded, decodedCanonical, err := agentcontract.DecodeVisualFoundationInput(canonical)
	if err != nil || decoded.TypedOverridesHash != input.TypedOverridesHash ||
		decoded.ReferenceAttachmentsHash != input.ReferenceAttachmentsHash || string(decodedCanonical) != string(canonical) {
		t.Fatalf("decode compiled Visual Foundation input: decoded=%#v err=%v", decoded, err)
	}
}

func TestCompileFaithfulVisualFoundationInputRejectsUnprovenOrUnresolvedWorld(t *testing.T) {
	_, source, world := faithfulVisualFoundationSources(t, false)
	release := curatedFaithfulRelease(t)

	driftedRoot := world
	for index := range driftedRoot.ConfirmedWorldRoots {
		if driftedRoot.ConfirmedWorldRoots[index].OwnerFamily == bibledomain.BibleProductionWorldFamily {
			driftedRoot.ConfirmedWorldRoots[index].CollectionRootHash = strings.Repeat("f", 64)
		}
	}
	if _, _, err := workflowapp.CompileFaithfulVisualFoundationInput(
		workflowapp.FaithfulVisualFoundationInputCommand{World: driftedRoot, Source: source, PresetRelease: release},
	); err == nil {
		t.Fatal("compiled a Visual Foundation input from a Bible root outside the confirmed Gate 2 source")
	}

	_, sourceWithGap, worldWithGap := faithfulVisualFoundationSources(t, true)
	if _, _, err := workflowapp.CompileFaithfulVisualFoundationInput(
		workflowapp.FaithfulVisualFoundationInputCommand{World: worldWithGap, Source: sourceWithGap, PresetRelease: release},
	); err == nil {
		t.Fatal("compiled the MVP faithful input while a Design Gap still requires an explicit visual-domain mapping")
	}

	driftedRelease := release
	driftedRelease.ContentHash = strings.Repeat("e", 64)
	if _, _, err := workflowapp.CompileFaithfulVisualFoundationInput(
		workflowapp.FaithfulVisualFoundationInputCommand{World: world, Source: source, PresetRelease: driftedRelease},
	); err == nil {
		t.Fatal("compiled a Visual Foundation input from a drifted Preset release")
	}
}

func faithfulVisualFoundationSources(
	t *testing.T,
	withDesignGap bool,
) (worlddomain.ProductionWorldCandidate, worlddomain.ConfirmedVisualFoundationSource, storygraphdomain.VisualFoundationWorldReadSet) {
	t.Helper()
	draft := productionWorldCandidateDraft(t)
	if withDesignGap {
		var production agentcontract.ProductionEntityFragmentCandidate
		if err := json.Unmarshal(draft.FrozenInput.ProductionEntityCandidate, &production); err != nil {
			t.Fatal(err)
		}
		gapKey := "gap_character_wardrobe_material"
		production.DesignGaps = []agentcontract.ProductionDesignGap{{
			GapKey: gapKey, SubjectKey: "character:linzhou", FieldKey: "wardrobe_material",
			MissingReason:            "剧本没有给出服装材质",
			SourceConstraints:        []agentcontract.SourceEvidenceSpan{},
			MutuallyExclusiveOptions: []string{"linen", "silk"},
			ImpactedSceneScopeKeys:   []string{draft.FrozenInput.StructureIdentitySet.SceneRefs[0].ScopeKey},
			AllowedResolutionSources: []string{"visual_foundation"},
		}}
		production.Entities[0].SpecificationSlots = append(
			production.Entities[0].SpecificationSlots,
			agentcontract.ProductionSemanticSlot{
				SlotKey: "wardrobe_material", Resolution: "unspecified_design_gap", DesignGapKey: &gapKey,
			},
		)
		draft.FrozenInput.ProductionEntityCandidate = mustJSON(t, production)
	}
	candidate, _, err := worlddomain.NewProductionWorldCandidate(draft)
	if err != nil {
		t.Fatal(err)
	}
	revisionHash := strings.Repeat("a", 64)
	version := visualFoundationBibleVersion(t, candidate, revisionHash)
	collection, err := bibledomain.BuildProductionWorldBibleCollection(version)
	if err != nil {
		t.Fatal(err)
	}
	source, err := worlddomain.NewConfirmedVisualFoundationSource(
		version, collection.CollectionRootHash, version.Candidate.VersionID, version.Candidate.Revision,
		revisionHash, candidate.ContentHash, candidate,
	)
	if err != nil {
		t.Fatal(err)
	}
	world := storygraphdomain.VisualFoundationWorldReadSet{
		WorkspaceID: candidate.WorkspaceID, ProjectID: candidate.ProjectID,
		StoryGraphVersionID: uuid.NewString(), StoryGraphContentHash: strings.Repeat("b", 64),
		ProductionWorldOwnerSetHash: strings.Repeat("c", 64),
		ConfirmedWorldRoots: []storygraphdomain.VisualFoundationWorldRoot{
			{OwnerFamily: "asset_identity_state_set", ScopeKey: "project:" + candidate.ProjectID, CollectionRootHash: strings.Repeat("1", 64)},
			{OwnerFamily: bibledomain.BibleProductionWorldFamily, ScopeKey: "project:" + candidate.ProjectID, CollectionRootHash: collection.CollectionRootHash},
			{OwnerFamily: "planning_scene_set", ScopeKey: candidate.SharedProof.PlanningEpisodeScopes[0].ScopeKey, CollectionRootHash: strings.Repeat("2", 64)},
		},
	}
	return candidate, source, world
}

func curatedFaithfulRelease(t *testing.T) presetdomain.Release {
	t.Helper()
	release, found, err := presetcatalog.FindCuratedRelease("urban-cinematic-realism", "2026.09.12")
	if err != nil || !found {
		t.Fatalf("load curated faithful Preset release: found=%v err=%v", found, err)
	}
	return release
}
