package agent_test

import (
	"reflect"
	"strings"
	"testing"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
)

func TestReferenceOutputCompilerConsumesSixFrozenBriefs(t *testing.T) {
	for _, kind := range []string{"character_identity_anchor", "character_appearance", "location_board", "prop_sheet", "scene_composition", "interaction_composition"} {
		t.Run(kind, func(t *testing.T) {
			document := referenceBriefCandidateDocument(t, kind)
			input, _, err := agentcontract.DecodeReferenceBriefInput(mustReferenceBriefJSON(t, referenceBriefInputDocument(document)))
			if err != nil {
				t.Fatal(err)
			}
			brief, _, err := agentcontract.DecodeReferenceBriefCandidate(mustReferenceBriefJSON(t, document))
			if err != nil {
				t.Fatal(err)
			}
			policies := make([]generationapp.ReferenceOutputSlotPolicy, len(input.RequiredViewRoles))
			for i, role := range input.RequiredViewRoles {
				policies[i] = generationapp.ReferenceOutputSlotPolicy{ViewRole: role, AllowedMediaTypes: []string{"image/png"}, AspectRatio: "1:1", MinWidth: 1024, MinHeight: 1024, MaxBytes: 8 << 20}
			}
			output, err := generationapp.CompileReferenceOutputContract(input, brief, 2, policies)
			if err != nil || len(output.Slots) != len(policies) {
				t.Fatalf("compile %s: %v", kind, err)
			}
			for _, slot := range output.Slots {
				if len(slot.SemanticRequirements) != len(brief.PositiveInstructions)+len(brief.LayoutRequirements)+len(brief.ScaleRequirements) ||
					slot.QCRubricRefs[0].ContentHash != brief.QCRubricRefs[0].ContentHash {
					t.Fatal("Brief semantics or QC lost")
				}
			}
			replayed, err := generationapp.CompileReferenceOutputContract(input, brief, 2, policies)
			if err != nil || !reflect.DeepEqual(replayed, output) {
				t.Fatalf("output drifted on replay: %v", err)
			}
			if _, err = generationapp.CompileReferenceOutputContract(input, brief, 2, policies[:len(policies)-1]); err == nil {
				t.Fatal("missing slot policy accepted")
			}
			policies[0].ViewRole = "reference_sheet"
			if _, err = generationapp.CompileReferenceOutputContract(input, brief, 2, policies); err == nil {
				t.Fatal("foreign policy accepted")
			}
			policies[0].ViewRole = input.RequiredViewRoles[0]
			brief.TypedReadSetRoot = strings.Repeat("f", 64)
			if _, err = generationapp.CompileReferenceOutputContract(input, brief, 2, policies); err == nil {
				t.Fatal("stale Brief accepted")
			}
		})
	}
}
