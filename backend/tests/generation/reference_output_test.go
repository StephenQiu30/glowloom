package generation_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

func TestReferenceOutputContractPreservesIndependentViewsAndCanonicalIdentity(t *testing.T) {
	cases := map[string][]string{
		"character_identity_anchor": {"back", "front", "profile"},
		"character_appearance":      {"back", "front", "profile"},
		"location_board":            {"empty_establishing", "material_scale_detail", "spatial_orientation"},
		"prop_sheet":                {"back", "front", "side", "state_detail"},
		"scene_composition":         {"composition_master"},
		"interaction_composition":   {"interaction_master"},
	}
	for kind, roles := range cases {
		t.Run(kind, func(t *testing.T) {
			slots := referenceOutputSlots(roles)
			output, err := domain.BuildReferenceOutputContract(kind, 4, slots)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "character_identity_anchor" && output.ContentHash != "000e2355eedeadc80ce703b67f427723b693e36f9d6e9db53c4c0076c69467cc" {
				t.Fatalf("output hash golden drifted: %s", output.ContentHash)
			}
			if output.ContractID != "reference-output-production" || output.Modality != "image" ||
				output.CandidateBundleCount != 4 || output.BundleCompleteness != "all_required_slots" || len(output.Slots) != len(roles) {
				t.Fatalf("output contract=%#v", output)
			}
			for index, role := range roles {
				if output.Slots[index].SlotKey != role || output.Slots[index].ViewRole != role || !output.Slots[index].Required {
					t.Fatalf("required view %s was lost", role)
				}
			}
			slices.Reverse(slots)
			slots[0].SemanticRequirements = []string{"preserve scale", "preserve identity"}
			reordered, err := domain.BuildReferenceOutputContract(kind, 4, slots)
			if err != nil || !reflect.DeepEqual(output, reordered) {
				t.Fatalf("canonical output changed: %v", err)
			}
			raw, err := json.Marshal(output)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := domain.DecodeReferenceOutputContract(raw, kind)
			if err != nil || !reflect.DeepEqual(decoded, output) {
				t.Fatalf("roundtrip: %v", err)
			}
			slots[0].QCRubricRefs[0].ContentHash = strings.Repeat("b", 64)
			changed, err := domain.BuildReferenceOutputContract(kind, 4, slots)
			if err != nil || changed.ContentHash == output.ContentHash {
				t.Fatalf("QC not bound: %v", err)
			}
			if output.Slots[len(output.Slots)-1].QCRubricRefs[0].ContentHash != strings.Repeat("a", 64) {
				t.Fatal("constructor retained caller-owned slice")
			}
		})
	}
}

func TestReferenceOutputContractNormalizesSemanticUnicode(t *testing.T) {
	slots := referenceOutputSlots([]string{"composition_master"})
	slots[0].SemanticRequirements = []string{"caf\u00e9", "cafe\u0301"}
	output, err := domain.BuildReferenceOutputContract("scene_composition", 1, slots)
	if err != nil {
		t.Fatal(err)
	}
	slots[0].SemanticRequirements = []string{"caf\u00e9"}
	composed, err := domain.BuildReferenceOutputContract("scene_composition", 1, slots)
	if err != nil || !reflect.DeepEqual(output, composed) {
		t.Fatalf("equivalent Unicode changed output: %v", err)
	}
	raw, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = domain.DecodeReferenceOutputContract(raw, "scene_composition"); err != nil {
		t.Fatalf("normalized output cannot roundtrip: %v", err)
	}
}

func TestReferenceOutputContractRejectsIncompleteViewsAndUnsafePolicy(t *testing.T) {
	for name, mutate := range map[string]func([]domain.ReferenceOutputSlot) []domain.ReferenceOutputSlot{
		"missing view":   func(s []domain.ReferenceOutputSlot) []domain.ReferenceOutputSlot { return s[:2] },
		"duplicate view": func(s []domain.ReferenceOutputSlot) []domain.ReferenceOutputSlot { s[1] = s[0]; return s },
		"composite sheet": func(s []domain.ReferenceOutputSlot) []domain.ReferenceOutputSlot {
			s[0].ViewRole = "reference_sheet"
			return s
		},
		"optional view": func(s []domain.ReferenceOutputSlot) []domain.ReferenceOutputSlot { s[0].Required = false; return s },
		"foreign slot key": func(s []domain.ReferenceOutputSlot) []domain.ReferenceOutputSlot {
			s[0].SlotKey = "material_scale_detail"
			return s
		},
		"invalid media": func(s []domain.ReferenceOutputSlot) []domain.ReferenceOutputSlot {
			s[0].AllowedMediaTypes = []string{"image/svg+xml"}
			return s
		},
		"duplicate media": func(s []domain.ReferenceOutputSlot) []domain.ReferenceOutputSlot {
			s[0].AllowedMediaTypes = []string{"image/png", "image/png"}
			return s
		},
		"zero dimension":      func(s []domain.ReferenceOutputSlot) []domain.ReferenceOutputSlot { s[0].MinWidth = 0; return s },
		"unbounded dimension": func(s []domain.ReferenceOutputSlot) []domain.ReferenceOutputSlot { s[0].MinHeight = 8193; return s },
		"mutable ratio":       func(s []domain.ReferenceOutputSlot) []domain.ReferenceOutputSlot { s[0].AspectRatio = "auto"; return s },
		"unreduced ratio":     func(s []domain.ReferenceOutputSlot) []domain.ReferenceOutputSlot { s[0].AspectRatio = "2:2"; return s },
		"unbounded bytes":     func(s []domain.ReferenceOutputSlot) []domain.ReferenceOutputSlot { s[0].MaxBytes = 11 << 20; return s },
		"no semantics": func(s []domain.ReferenceOutputSlot) []domain.ReferenceOutputSlot {
			s[0].SemanticRequirements = nil
			return s
		},
		"invalid QC": func(s []domain.ReferenceOutputSlot) []domain.ReferenceOutputSlot {
			s[0].QCRubricRefs[0].ContentHash = "latest"
			return s
		},
		"no QC": func(s []domain.ReferenceOutputSlot) []domain.ReferenceOutputSlot { s[0].QCRubricRefs = nil; return s },
	} {
		t.Run(name, func(t *testing.T) {
			slots := mutate(referenceOutputSlots([]string{"back", "front", "profile"}))
			if _, err := domain.BuildReferenceOutputContract("character_identity_anchor", 1, slots); err == nil {
				t.Fatal("invalid output accepted")
			}
		})
	}
	for _, count := range []int{-1, 0, 5} {
		if _, err := domain.BuildReferenceOutputContract("character_identity_anchor", count, referenceOutputSlots([]string{"back", "front", "profile"})); err == nil {
			t.Fatalf("count %d accepted", count)
		}
	}
	slots := referenceOutputSlots([]string{"back", "front", "side", "state_detail"})
	for i := range slots {
		slots[i].MaxBytes = 10 << 20
	}
	if _, err := domain.BuildReferenceOutputContract("prop_sheet", 1, slots); err == nil {
		t.Fatal("whole Bundle exceeded Vision budget")
	}
}

func TestReferenceOutputDecoderRejectsTamperingAndUnknownFields(t *testing.T) {
	output, err := domain.BuildReferenceOutputContract("character_identity_anchor", 1, referenceOutputSlots([]string{"back", "front", "profile"}))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	for name, encoded := range map[string]string{
		"hash drift":         strings.Replace(string(raw), `"candidate_bundle_count":1`, `"candidate_bundle_count":2`, 1),
		"provider field":     strings.Replace(string(raw), `{`, `{"provider":"openai",`, 1),
		"duplicate key":      strings.Replace(string(raw), `{`, `{"candidate_bundle_count":1,`, 1),
		"slot unknown field": strings.Replace(string(raw), `"required":true`, `"required":true,"prompt":"free text"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.DecodeReferenceOutputContract([]byte(encoded), "character_identity_anchor"); err == nil {
				t.Fatal("invalid output decoded")
			}
		})
	}
	if _, err := domain.DecodeReferenceOutputContract(raw, "location_board"); err == nil {
		t.Fatal("wrong purpose accepted")
	}
}

func referenceOutputSlots(roles []string) []domain.ReferenceOutputSlot {
	slots := make([]domain.ReferenceOutputSlot, len(roles))
	for i, role := range roles {
		slots[i] = domain.ReferenceOutputSlot{
			SlotKey: role, ViewRole: role, Required: true,
			AllowedMediaTypes: []string{"image/png"}, AspectRatio: "1:1", MinWidth: 1024, MinHeight: 1024, MaxBytes: 8 << 20,
			SemanticRequirements: []string{"preserve identity", "preserve scale"},
			QCRubricRefs:         []domain.ReferenceOutputQCRubricRef{{ContractID: "reference-visual-qc-production", ContentHash: strings.Repeat("a", 64)}},
		}
	}
	return slots
}
