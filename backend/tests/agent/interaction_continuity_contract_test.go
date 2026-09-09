package agent_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestInteractionContinuityRequiresThreeFrozenCandidateInputs(t *testing.T) {
	productionInput, productionCandidate := productionEntityContractFixture(t)
	productionRaw, err := json.Marshal(productionCandidate)
	if err != nil {
		t.Fatal(err)
	}
	bindingInput := contract.SceneOccurrenceBindingInput{
		ProductionEntityDerivationInput:       productionInput,
		ProductionEntityCandidateRevisionID:   uuid.NewString(),
		ProductionEntityCandidateRevisionHash: "b" + productionInput.SourceHash[1:],
		ProductionEntityCandidate:             productionRaw,
	}
	formalScene := productionInput.StructureIdentitySet.SceneRefs[0]
	bindingCandidate := contract.SceneBindingFragmentCandidate{
		SourceVersionID: productionInput.SourceVersionID, SourceHash: productionInput.SourceHash,
		StructureIdentitySetVersionID:         productionInput.StructureIdentitySetVersionID,
		StructureIdentitySetVersionHash:       productionInput.StructureIdentitySetVersionHash,
		SceneFactCandidateRevisionID:          productionInput.SceneFactCandidateRevisionID,
		SceneFactCandidateRevisionHash:        productionInput.SceneFactCandidateRevisionHash,
		ProductionEntityCandidateRevisionID:   bindingInput.ProductionEntityCandidateRevisionID,
		ProductionEntityCandidateRevisionHash: bindingInput.ProductionEntityCandidateRevisionHash,
		Scenes: []contract.SceneBindingFragment{{
			SceneScopeKey: formalScene.ScopeKey, SceneOwnerLogicalID: formalScene.SceneOwnerLogicalID,
			TemporarySceneID: formalScene.TemporarySceneID,
			SourceStart:      formalScene.SourceStart, SourceEnd: formalScene.SourceEnd,
			Dialogues: []contract.SceneDialogueFragment{}, Beats: []contract.SceneBeatFragment{},
			Occurrences: []contract.SceneOccurrenceFragment{{
				OccurrenceKey: "occurrence_scene_0001_0001", Order: 1,
				SubjectKind: "character", IdentityKey: "character:linzhou",
				StateKey: "state_character_linzhou_initial", OccurrenceRole: "actual",
				Evidence: contract.SourceEvidenceSpan{
					SourceStart: 0, SourceEnd: 2,
					TextHash: productionInput.SourceHash, ExactAnchor: productionInput.NormalizedText,
				},
			}},
		}},
		ReviewIssues: []contract.CandidateReviewIssue{},
	}
	bindingRaw, err := json.Marshal(bindingCandidate)
	if err != nil {
		t.Fatal(err)
	}
	input := contract.InteractionContinuityInput{
		SceneOccurrenceBindingInput:       bindingInput,
		SceneBindingCandidateRevisionID:   uuid.NewString(),
		SceneBindingCandidateRevisionHash: "c" + productionInput.SourceHash[1:],
		SceneBindingCandidate:             bindingRaw,
	}
	candidate := contract.InteractionContinuityCandidate{
		SourceVersionID: productionInput.SourceVersionID, SourceHash: productionInput.SourceHash,
		StructureIdentitySetVersionID:         productionInput.StructureIdentitySetVersionID,
		StructureIdentitySetVersionHash:       productionInput.StructureIdentitySetVersionHash,
		SceneFactCandidateRevisionID:          productionInput.SceneFactCandidateRevisionID,
		SceneFactCandidateRevisionHash:        productionInput.SceneFactCandidateRevisionHash,
		ProductionEntityCandidateRevisionID:   bindingInput.ProductionEntityCandidateRevisionID,
		ProductionEntityCandidateRevisionHash: bindingInput.ProductionEntityCandidateRevisionHash,
		SceneBindingCandidateRevisionID:       input.SceneBindingCandidateRevisionID,
		SceneBindingCandidateRevisionHash:     input.SceneBindingCandidateRevisionHash,
		SceneStoryTimes: []contract.SceneStoryTimeFragment{{
			SceneScopeKey: formalScene.ScopeKey, StoryTimeKey: "storytime:00000001",
		}},
		Interactions: []contract.InteractionFragment{}, Continuity: []contract.ContinuityFragment{},
		ReviewIssues: []contract.CandidateReviewIssue{},
	}
	raw, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err = contract.ValidateInteractionContinuityCandidate(raw, input); err != nil {
		t.Fatalf("validate Interaction/Continuity Candidate: %v", err)
	}

	input.SceneBindingCandidateRevisionHash = "f" + input.SceneBindingCandidateRevisionHash[1:]
	if err = contract.ValidateInteractionContinuityCandidate(raw, input); err == nil {
		t.Fatal("Interaction/Continuity accepted a drifted Scene Binding input")
	}
}
