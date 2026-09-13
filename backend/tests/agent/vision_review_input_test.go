package agent_test

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestVisionReviewInputSharedCanonicalIdentity(t *testing.T) {
	raw, err := os.ReadFile("testdata/vision_review_input.json")
	if err != nil {
		t.Fatal(err)
	}
	input, err := contract.DecodeVisionReviewInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := contract.DecodeVisionReviewInput(encoded); err != nil {
		t.Fatal(err)
	}
	if input.Subject.StageReleaseHash == input.BriefInput.StageRelease.StageReleaseHash {
		t.Fatal("review Release replaced the Brief Release")
	}
	if input.Subject.InputHash != "9702a0c7bd5822c091597da38ab2c76a21b9f49ac03795dc11ba607cbcf09999" {
		t.Fatal("shared input identity changed")
	}
	rebuilt, err := contract.BuildVisionReviewInput(input)
	if err != nil || !reflect.DeepEqual(rebuilt, input) {
		t.Fatalf("deterministic rebuild: %v", err)
	}
	input.VisualContext.VisualGrammar.NegativeConstraints[0] = "mutated"
	if rebuilt.VisualContext.VisualGrammar.NegativeConstraints[0] == "mutated" {
		t.Fatal("input retained mutable Owner slices")
	}
}

func TestVisionReviewInputBasePurposesAndDependencyClosure(t *testing.T) {
	raw, err := os.ReadFile("testdata/vision_review_input.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"character_identity_anchor", "location_board", "prop_sheet", "character_appearance", "interaction_composition", "scene_composition"} {
		t.Run(kind, func(t *testing.T) {
			input, err := contract.DecodeVisionReviewInput(raw)
			if err != nil {
				t.Fatal(err)
			}
			candidateDocument := referenceBriefCandidateDocument(t, kind)
			candidateRaw := mustReferenceBriefJSON(t, candidateDocument)
			input.BriefCandidate, _, err = contract.DecodeReferenceBriefCandidate(candidateRaw)
			if err != nil {
				t.Fatal(err)
			}
			input.BriefInput, _, err = contract.DecodeReferenceBriefInput(mustReferenceBriefJSON(t, referenceBriefInputDocument(candidateDocument)))
			if err != nil {
				t.Fatal(err)
			}
			input.BriefContentHash, err = contract.ProductionCanonicalHash(candidateRaw)
			if err != nil {
				t.Fatal(err)
			}
			input.Subject.WorkspaceID, input.Subject.ProjectID, input.Subject.TargetKind = input.BriefInput.WorkspaceID, input.BriefInput.ProjectID, kind
			input.VisualContext.PurposeProfile.TargetKind = kind
			sample := input.Attachments[0]
			input.Attachments, input.Subject.Slots = nil, nil
			for index, role := range contract.ReferenceBriefRequiredViewRoles(kind) {
				attachment := sample
				attachment.Slot.SlotKey, attachment.Slot.ViewRole = role, role
				attachment.Slot.MediaRef.ID = fmt.Sprintf("00000000-0000-4000-8000-%012d", index+21)
				attachment.ProviderReceiptRef.ID = attachment.Slot.MediaRef.ID
				attachment.Slot.SHA256 = strings.Repeat(string("abcd"[index]), 64)
				input.Subject.Slots = append(input.Subject.Slots, attachment.Slot)
				input.Attachments = append(input.Attachments, attachment)
			}
			_, err = contract.BuildVisionReviewInput(input)
			if kind == "character_appearance" || strings.HasSuffix(kind, "_composition") {
				if err == nil || !strings.Contains(err.Error(), "dependency assets") {
					t.Fatalf("dependent purpose was silently downgraded: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestVisionReviewInputRejectsDriftAndForgedShape(t *testing.T) {
	for _, field := range []string{"contract_id", "input_hash", "scope", "round", "brief_hash", "brief_content", "admission", "selection", "publication", "rights", "grammar", "policy", "purpose", "empty_rules", "missing_attachment", "digest", "missing_boolean", "null_boolean", "unknown", "private_path"} {
		t.Run(field, func(t *testing.T) {
			raw, err := os.ReadFile("testdata/vision_review_input.json")
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			if err := json.Unmarshal(raw, &value); err != nil {
				t.Fatal(err)
			}
			subject := value["subject"].(map[string]any)
			visual := value["visual_context"].(map[string]any)
			admission := value["admission"].(map[string]any)
			switch field {
			case "contract_id":
				value["contract_id"] = "other"
			case "input_hash":
				subject["input_hash"] = "invalid"
			case "scope":
				subject["project_id"] = "00000000-0000-4000-8000-000000000099"
			case "round":
				subject["generation_round"] = 2
			case "brief_hash":
				value["brief_content_hash"] = subject["stage_release_hash"]
			case "brief_content":
				value["brief_candidate"].(map[string]any)["positive_instructions"] = []any{"changed"}
			case "admission":
				admission["internal_review_ready"] = false
			case "selection":
				admission["selection_ready"] = true
			case "publication":
				admission["publication_ready"] = true
			case "rights":
				admission["formal_use_blockers"] = []any{"vision_review_required"}
			case "grammar":
				visual["visual_grammar"].(map[string]any)["medium"] = ""
			case "policy":
				visual["style_policy"].(map[string]any)["forbidden_changes"] = []any{}
			case "purpose":
				visual["purpose_profile"].(map[string]any)["target_kind"] = "prop_sheet"
			case "empty_rules":
				visual["visual_grammar"].(map[string]any)["negative_constraints"] = []any{}
			case "missing_attachment":
				value["attachments"] = value["attachments"].([]any)[:1]
			case "digest":
				value["attachments"].([]any)[0].(map[string]any)["slot"].(map[string]any)["sha256"] = subject["stage_release_hash"]
			case "missing_boolean":
				delete(admission, "selection_ready")
			case "null_boolean":
				admission["selection_ready"] = nil
			case "unknown":
				value["approved"] = true
			case "private_path":
				value["attachments"].([]any)[0].(map[string]any)["path"] = "/private/image.png"
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := contract.DecodeVisionReviewInput(encoded); err == nil {
				t.Fatal("accepted input drift")
			}
			if field == "input_hash" {
				return
			}
			// Re-signing an invalid shape must not turn it into a valid input.
			delete(subject, "input_hash")
			material, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			hash, err := contract.ProductionCanonicalHash(material)
			if err != nil {
				t.Fatal(err)
			}
			subject["input_hash"] = hash
			encoded, err = json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := contract.DecodeVisionReviewInput(encoded); err == nil {
				t.Fatal("re-signing bypassed input policy")
			}
		})
	}
}
