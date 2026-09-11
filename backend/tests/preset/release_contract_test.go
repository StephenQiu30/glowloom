package preset_test

import (
	"encoding/json"
	"slices"
	"testing"

	preset "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
)

func TestPresetReleaseRoundTripsAnImmutableSixPurposeContract(t *testing.T) {
	release, encoded, err := preset.NewRelease(validPresetReleaseInput())
	if err != nil {
		t.Fatalf("new Preset release: %v", err)
	}
	if release.ContractID != preset.ReleaseContractID || len(release.ContentHash) != 64 || len(release.CapabilityManifest) != 6 ||
		len(release.PurposeProfiles) != 6 || len(release.FidelityInvariants) != 4 || len(release.WorldAdaptationRules) != 1 {
		t.Fatalf("unexpected Preset release: %#v", release)
	}
	decoded, canonical, err := preset.DecodeRelease(encoded)
	if err != nil {
		t.Fatalf("decode Preset release: %v", err)
	}
	if decoded.ContentHash != release.ContentHash || !slices.Equal(canonical, encoded) {
		t.Fatal("Preset release did not round-trip canonically")
	}
}

func TestPresetReleaseRejectsIncompleteCapabilityAndGovernance(t *testing.T) {
	tests := map[string]func(*preset.ReleaseInput){
		"missing NOTICE":          func(value *preset.ReleaseInput) { value.Provenance.NoticePath = "" },
		"missing capability":      func(value *preset.ReleaseInput) { value.CapabilityManifest = value.CapabilityManifest[:5] },
		"duplicate capability":    func(value *preset.ReleaseInput) { value.CapabilityManifest[1] = value.CapabilityManifest[0] },
		"wrong view roles":        func(value *preset.ReleaseInput) { value.CapabilityManifest[0].ViewRoles = []string{"front"} },
		"missing purpose profile": func(value *preset.ReleaseInput) { value.PurposeProfiles = value.PurposeProfiles[:5] },
		"invalid Skill hash":      func(value *preset.ReleaseInput) { value.SkillReleaseRefs[0].ContentHash = "latest" },
		"numeric release prefix":  func(value *preset.ReleaseInput) { value.Release = "v" + "1" },
		"invalid release date":    func(value *preset.ReleaseInput) { value.Release = "2026.02.31" },
		"missing fidelity invariant": func(value *preset.ReleaseInput) {
			value.FidelityInvariants = value.FidelityInvariants[:3]
		},
		"missing adaptation rule": func(value *preset.ReleaseInput) { value.WorldAdaptationRules = nil },
		"adaptation changes identity": func(value *preset.ReleaseInput) {
			value.WorldAdaptationRules[0].PreservedInvariantKeys = []string{"holder_relation", "scene_continuity", "story_fact"}
		},
		"automatic adaptation": func(value *preset.ReleaseInput) {
			value.WorldAdaptationRules[0].RequiresHumanDecision = false
		},
		"invalid adaptation scope": func(value *preset.ReleaseInput) {
			value.WorldAdaptationRules[0].ImpactScopeKinds = []string{"script_source"}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := validPresetReleaseInput()
			mutate(&input)
			if _, _, err := preset.NewRelease(input); err == nil {
				t.Fatal("invalid Preset release was accepted")
			}
		})
	}
}

func TestPresetReleaseRejectsUnknownFieldsAndHashDrift(t *testing.T) {
	_, encoded, err := preset.NewRelease(validPresetReleaseInput())
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err = json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	payload["provider"] = "latest"
	unknown, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = preset.DecodeRelease(unknown); err == nil {
		t.Fatal("unknown Preset release field was accepted")
	}
	delete(payload, "provider")
	payload["description"] = "drifted"
	drifted, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = preset.DecodeRelease(drifted); err == nil {
		t.Fatal("Preset release content hash drift was accepted")
	}
}

func validPresetReleaseInput() preset.ReleaseInput {
	targetKinds := []string{
		"character_identity_anchor", "character_appearance", "location_board", "prop_sheet",
		"interaction_composition", "scene_composition",
	}
	viewRoles := map[string][]string{
		"character_identity_anchor": {"back", "front", "profile"},
		"character_appearance":      {"back", "front", "profile"},
		"location_board":            {"empty_establishing", "material_scale_detail", "spatial_orientation"},
		"prop_sheet":                {"back", "front", "side", "state_detail"},
		"interaction_composition":   {"interaction_master"},
		"scene_composition":         {"composition_master"},
	}
	capabilities := make([]preset.Capability, 0, len(targetKinds))
	profiles := make([]preset.PurposeProfile, 0, len(targetKinds))
	for _, targetKind := range targetKinds {
		capabilities = append(capabilities, preset.Capability{TargetKind: targetKind, ViewRoles: viewRoles[targetKind]})
		profiles = append(profiles, preset.PurposeProfile{
			TargetKind: targetKind, DesignFocus: []string{"identity_fidelity", "visual_coherence"},
			ForbiddenChanges: []string{"production_fact_rewrite"},
		})
	}
	return preset.ReleaseInput{
		Key: "urban-cinematic-realism", Release: "2026.09.11", Label: "都市电影写实", Category: "cinematic-realism",
		Description: "以自然材质、可信空间与克制电影光线表达当代故事。", DefaultMode: "faithful",
		Provenance: preset.Provenance{
			Origin: "first_party", SourceURL: "https://github.com/StephenQiu30/lanverse",
			LicenseSPDX: "MIT", NoticePath: "backend/internal/preset/releases/NOTICE.md",
		},
		CapabilityManifest: capabilities,
		WorldDesignBasis: preset.WorldDesignBasis{
			Era: "contemporary", Region: "script_defined", CivilizationLanguage: "grounded_urban",
			TechnologyOrMagicLanguage: "script_defined", ArchitectureLanguage: "functional_contemporary",
			WardrobeLanguage: "character_led_realism", PropLanguage: "functional_realism", MaterialSystem: "natural_wear",
			Motifs: []string{"controlled_reflection", "lived_in_detail"}, AnachronismConstraints: []string{"preserve_script_era"},
		},
		VisualGrammar: preset.VisualGrammar{
			Medium: "cinematic_digital", Realism: "semi_photoreal", ShapeLanguage: "naturalistic", Proportion: "human_realistic",
			Palette: "motivated_neutral", Linework: "none", Texture: "material_specific", MaterialRendering: "physically_plausible",
			Lighting: "motivated_practical", Contrast: "controlled", Composition: "narrative_clarity", Camera: "grounded_cinematic",
			NegativeConstraints: []string{"artist_name", "protected_ip_imitation"},
		},
		FidelityInvariants: []string{"character_identity", "holder_relation", "scene_continuity", "story_fact"},
		WorldAdaptationRules: []preset.WorldAdaptationRule{
			{
				RuleKey: "translate-technology-language", SourceFactKind: "technology_or_magic",
				DesignDomain: "technology_or_magic", Directive: "translate_expression_without_changing_function_or_outcome",
				PreservedInvariantKeys: []string{"character_identity", "holder_relation", "scene_continuity", "story_fact"},
				ImpactScopeKinds:       []string{"asset", "interaction", "scene"}, RequiresHumanDecision: true,
			},
		},
		PurposeProfiles:  profiles,
		SkillReleaseRefs: []preset.ContentRef{{Owner: "agent/skill", Key: "build-storygraph", ContentHash: presetTestHash("skill")}},
		QCPolicyRef:      preset.ContentRef{Owner: "preset/policy", Key: "reference-qc", ContentHash: presetTestHash("qc")},
		ModelCapabilityPolicyRef: preset.ContentRef{
			Owner: "preset/policy", Key: "visual-model-capability", ContentHash: presetTestHash("model"),
		},
	}
}

func presetTestHash(seed string) string {
	values := map[string]string{
		"skill": "27d61a702e8a6a194584539259ddb82fde0f0c91ba2e85ae5008f82414b38bb8",
		"qc":    "1a7cf5730c6c81ae1ab6926963837c5452778140cdb34ba11de1f6158d515f86",
		"model": "d2dc9d7d3017302957b3ff00dfb0a17660f60142bbdbbcf821e071e5634a68b3",
	}
	return values[seed]
}
