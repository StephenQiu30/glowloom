package agent_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestSceneOccurrenceBindingRequiresExactFormalIdentityStateAndPresence(t *testing.T) {
	productionInput, productionCandidate := productionEntityContractFixture(t)
	productionRaw, err := json.Marshal(productionCandidate)
	if err != nil {
		t.Fatal(err)
	}
	input := contract.SceneOccurrenceBindingInput{
		ProductionEntityDerivationInput:       productionInput,
		ProductionEntityCandidateRevisionID:   uuid.NewString(),
		ProductionEntityCandidateRevisionHash: "b" + productionInput.SourceHash[1:],
		ProductionEntityCandidate:             productionRaw,
	}
	formalScene := productionInput.StructureIdentitySet.SceneRefs[0]
	candidate := contract.SceneBindingFragmentCandidate{
		SourceVersionID: productionInput.SourceVersionID, SourceHash: productionInput.SourceHash,
		StructureIdentitySetVersionID:         productionInput.StructureIdentitySetVersionID,
		StructureIdentitySetVersionHash:       productionInput.StructureIdentitySetVersionHash,
		SceneFactCandidateRevisionID:          productionInput.SceneFactCandidateRevisionID,
		SceneFactCandidateRevisionHash:        productionInput.SceneFactCandidateRevisionHash,
		ProductionEntityCandidateRevisionID:   input.ProductionEntityCandidateRevisionID,
		ProductionEntityCandidateRevisionHash: input.ProductionEntityCandidateRevisionHash,
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
	raw, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err = contract.ValidateSceneBindingFragmentCandidate(raw, input); err != nil {
		t.Fatalf("validate Scene binding Candidate: %v", err)
	}

	candidate.Scenes[0].Occurrences[0].OccurrenceRole = "mentioned_only"
	raw, err = json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err = contract.ValidateSceneBindingFragmentCandidate(raw, input); err == nil {
		t.Fatal("Scene binding accepted occurrence role drift")
	}
}
