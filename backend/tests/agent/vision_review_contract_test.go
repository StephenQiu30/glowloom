package agent_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestVisionReviewCandidateFrozenBundleAndEvidence(t *testing.T) {
	raw, err := os.ReadFile("testdata/vision_review_candidate.json")
	if err != nil {
		t.Fatal(err)
	}
	candidate, canonical, err := contract.DecodeVisionReviewCandidate(raw)
	if err != nil || len(canonical) == 0 {
		t.Fatalf("decode review: %v", err)
	}
	if hash, err := contract.ProductionCanonicalHash(canonical); err != nil || hash != "a536d9ff3a5b82fbc3ce10ef02509bd5908710ef41c2919e14e045acfa86f340" {
		t.Fatalf("Vision Review canonical golden drifted: %s %v", hash, err)
	}
	if err := candidate.ValidateFor(candidate.Subject); err != nil {
		t.Fatal(err)
	}
	subject := candidate.Subject
	subject.InputHash = strings.Repeat("b", 64)
	if candidate.ValidateFor(subject) == nil {
		t.Fatal("accepted a different frozen input")
	}
	for _, field := range []string{"project_id", "brief_revision_hash", "stage_release_hash", "bundle_input_ref", "execution_ref", "media_sha256"} {
		var frozen contract.VisionReviewSubject
		encoded, _ := json.Marshal(candidate.Subject)
		if err := json.Unmarshal(encoded, &frozen); err != nil {
			t.Fatal(err)
		}
		switch field {
		case "project_id":
			frozen.ProjectID = "00000000-0000-4000-8000-000000000099"
		case "brief_revision_hash":
			frozen.BriefRevisionHash = strings.Repeat("b", 64)
		case "stage_release_hash":
			frozen.StageReleaseHash = strings.Repeat("b", 64)
		case "bundle_input_ref":
			frozen.BundleInputRef.ContentHash = strings.Repeat("b", 64)
		case "execution_ref":
			frozen.ExecutionRef.ContentHash = strings.Repeat("b", 64)
		case "media_sha256":
			frozen.Slots[0].SHA256 = strings.Repeat("d", 64)
		}
		if candidate.ValidateFor(frozen) == nil {
			t.Fatalf("accepted changed %s", field)
		}
	}
	for _, kind := range []string{"character_identity_anchor", "character_appearance", "location_board", "prop_sheet", "interaction_composition", "scene_composition"} {
		t.Run(kind, func(t *testing.T) {
			value := candidate
			value.Subject.TargetKind = kind
			value.Subject.Slots = nil
			for index, role := range contract.ReferenceBriefRequiredViewRoles(kind) {
				slot := candidate.Subject.Slots[0]
				slot.SlotKey = role
				slot.ViewRole = role
				slot.MediaRef.ID = []string{"00000000-0000-4000-8000-000000000021", "00000000-0000-4000-8000-000000000022", "00000000-0000-4000-8000-000000000023", "00000000-0000-4000-8000-000000000024"}[index]
				slot.SHA256 = strings.Repeat([]string{"a", "b", "c", "d"}[index], 64)
				value.Subject.Slots = append(value.Subject.Slots, slot)
			}
			value.Checks = append([]contract.VisionReviewCheck(nil), candidate.Checks...)
			for index := range value.Checks {
				value.Checks[index].Evidence = nil
				for _, slot := range value.Subject.Slots {
					value.Checks[index].Evidence = append(value.Checks[index].Evidence, contract.VisionReviewEvidence{SlotKey: slot.SlotKey, Region: contract.VisionReviewRegion{Width: 10000, Height: 10000}})
				}
			}
			encoded, _ := json.Marshal(value)
			if _, _, err := contract.DecodeVisionReviewCandidate(encoded); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestVisionReviewCandidateRejectsIncompleteAndUnauthorizedOutput(t *testing.T) {
	raw, err := os.ReadFile("testdata/vision_review_candidate.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"missing_category", "duplicate_category", "reordered", "unknown_slot", "missing_pass_view", "outside_region", "zero_area", "duplicate_region", "negative_confidence", "over_confidence", "pass_issue", "fail_without_evidence", "warn_without_advice", "unknown_status", "mixed_media", "wrong_view", "incomplete_views", "current_ref", "empty_input_hash", "selected", "extra_nested"} {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal(raw, &value); err != nil {
				t.Fatal(err)
			}
			subject := value["subject"].(map[string]any)
			checks := value["checks"].([]any)
			first := checks[0].(map[string]any)
			evidence := first["evidence"].([]any)
			region := evidence[0].(map[string]any)["region"].(map[string]any)
			slots := subject["slots"].([]any)
			switch name {
			case "missing_category":
				value["checks"] = checks[1:]
			case "duplicate_category":
				checks[1] = checks[0]
			case "reordered":
				checks[0], checks[1] = checks[1], checks[0]
			case "unknown_slot":
				evidence[0].(map[string]any)["slot_key"] = "other_bundle"
			case "missing_pass_view":
				first["evidence"] = evidence[1:]
			case "outside_region":
				region["x"] = 5000
			case "zero_area":
				region["width"] = 0
			case "duplicate_region":
				first["evidence"] = append(evidence, evidence[0])
			case "negative_confidence":
				first["confidence_bps"] = -1
			case "over_confidence":
				first["confidence_bps"] = 10001
			case "pass_issue":
				first["issue_code"] = "identity_drift"
			case "fail_without_evidence":
				first["status"] = "fail"
				first["issue_code"] = "identity_drift"
				first["recommendation"] = "修复身份"
				first["evidence"] = []any{}
			case "warn_without_advice":
				first["status"] = "warn"
				first["issue_code"] = "identity_uncertain"
			case "unknown_status":
				first["status"] = "eligible"
			case "mixed_media":
				slots[1].(map[string]any)["staged_media_ref"] = slots[0].(map[string]any)["staged_media_ref"]
			case "wrong_view":
				slots[0].(map[string]any)["view_role"] = "front"
			case "incomplete_views":
				subject["slots"] = slots[1:]
			case "current_ref":
				subject["execution_ref"].(map[string]any)["id"] = "current"
			case "empty_input_hash":
				subject["input_hash"] = ""
			case "selected":
				value["selected"] = true
			case "extra_nested":
				region["prompt"] = "override"
			}
			encoded, _ := json.Marshal(value)
			if _, _, err := contract.DecodeVisionReviewCandidate(encoded); err == nil {
				t.Fatal("accepted invalid review")
			}
		})
	}
	if _, _, err := contract.DecodeVisionReviewCandidate(append(raw, raw...)); err == nil {
		t.Fatal("accepted trailing JSON")
	}
	for _, field := range []string{"confidence_bps", "recommendation", "x"} {
		var document map[string]any
		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatal(err)
		}
		check := document["checks"].([]any)[0].(map[string]any)
		if field == "x" {
			delete(check["evidence"].([]any)[0].(map[string]any)["region"].(map[string]any), field)
		} else {
			delete(check, field)
		}
		encoded, _ := json.Marshal(document)
		if _, _, err := contract.DecodeVisionReviewCandidate(encoded); err == nil {
			t.Fatalf("accepted missing required field %s", field)
		}
	}
	duplicate := strings.TrimSpace(string(raw))
	duplicate = strings.TrimSuffix(duplicate, "}") + `,"contract_id":"vision-review-candidate-production"}`
	if _, _, err := contract.DecodeVisionReviewCandidate([]byte(duplicate)); err == nil {
		t.Fatal("accepted duplicate key")
	}
}

func TestVisionReviewNotAssessableIsExplicit(t *testing.T) {
	raw, err := os.ReadFile("testdata/vision_review_candidate.json")
	if err != nil {
		t.Fatal(err)
	}
	value, _, err := contract.DecodeVisionReviewCandidate(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"warn", "fail"} {
		value.Checks[0].Status = status
		value.Checks[0].IssueCode = "identity_drift"
		value.Checks[0].Recommendation = "核对人物身份特征"
		encoded, _ := json.Marshal(value)
		decoded, _, err := contract.DecodeVisionReviewCandidate(encoded)
		if err != nil || decoded.Checks[0].Status != status {
			t.Fatalf("lost issue status %s: %v", status, err)
		}
	}
	check := &value.Checks[1]
	check.Status = "not_assessable"
	check.ConfidenceBPS = 0
	check.IssueCode = "contact_not_visible"
	check.Recommendation = "提供可见接触点的视图"
	check.Evidence = []contract.VisionReviewEvidence{}
	encoded, _ := json.Marshal(value)
	decoded, _, err := contract.DecodeVisionReviewCandidate(encoded)
	if err != nil || decoded.Checks[1].Status != "not_assessable" {
		t.Fatalf("lost explicit uncertainty: %v", err)
	}
	check.ConfidenceBPS = 9000
	encoded, _ = json.Marshal(value)
	if _, _, err = contract.DecodeVisionReviewCandidate(encoded); err == nil {
		t.Fatal("not_assessable claimed confidence")
	}
}
