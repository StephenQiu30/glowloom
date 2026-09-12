package catalog

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	preset "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
)

const curatedRelease = "2026.09.12"

const (
	referenceQCPolicyHash           = "cd2edeb49a2e29803a62ce395f8ce7b5f3ff45f69706bb529278a2a648573aec"
	visualModelCapabilityPolicyHash = "145b1cf19e5f6fdd2ac676b98f1d66d55a3867a5d3ac1d041d44ea0009c308a4"
)

var curatedReleaseHashes = map[string]string{
	"chinese-fantasy-animation": "63694486dc920c64d5c0d109e728c1ce1670e1384fcb38f68f20e21df3330e5a",
	"cyberpunk-animation":       "a40c30ea7c284a7347ae6ed66c7f62014834ef375d32e8b1d609e59e55149a6c",
	"period-cinematic-realism":  "233bcc7ea1d1a612154ca711d9fe062f9397d728e163a64f2229af42ceefcd73",
	"urban-cinematic-realism":   "0a3953970ffa5ba4f414ef986d10638f01ba1defdd9d02765fe3a20ae0be9a0d",
}

type ImageQCPolicy struct {
	ContractID        string   `json:"contract_id"`
	Key               string   `json:"key"`
	AllowedMediaTypes []string `json:"allowed_media_types"`
	RequiredChecks    []string `json:"required_checks"`
	MaxImageBytes     int64    `json:"max_image_bytes"`
	ContentHash       string   `json:"content_hash"`
}

type ModelCapabilityPolicy struct {
	ContractID          string   `json:"contract_id"`
	Key                 string   `json:"key"`
	Capability          string   `json:"capability"`
	RequiredFeatures    []string `json:"required_features"`
	MaxImages           int      `json:"max_images"`
	MaxSingleImageBytes int64    `json:"max_single_image_bytes"`
	MaxTotalImageBytes  int64    `json:"max_total_image_bytes"`
	ContentHash         string   `json:"content_hash"`
}

func ReferenceQCPolicy() (ImageQCPolicy, error) {
	policy := ImageQCPolicy{
		ContractID: "preset-reference-qc-policy-production",
		Key:        "reference-qc",
		AllowedMediaTypes: []string{
			"image/jpeg",
			"image/png",
			"image/webp",
		},
		RequiredChecks: []string{
			"decodable_single_frame",
			"dimensions_match_receipt",
			"sha256_match",
			"target_view_complete",
		},
		MaxImageBytes: 10 << 20,
	}
	hash, err := catalogHash(policy)
	if err != nil {
		return ImageQCPolicy{}, err
	}
	if hash != referenceQCPolicyHash {
		return ImageQCPolicy{}, errors.New("curated reference QC policy has changed without a new identity")
	}
	policy.ContentHash = hash
	return policy, nil
}

func VisualModelCapabilityPolicy() (ModelCapabilityPolicy, error) {
	policy := ModelCapabilityPolicy{
		ContractID: "preset-visual-model-capability-policy-production",
		Key:        "visual-model-capability",
		Capability: "vision",
		RequiredFeatures: []string{
			"image_input",
			"strict_json_output",
		},
		MaxImages:           8,
		MaxSingleImageBytes: 10 << 20,
		MaxTotalImageBytes:  32 << 20,
	}
	hash, err := catalogHash(policy)
	if err != nil {
		return ModelCapabilityPolicy{}, err
	}
	if hash != visualModelCapabilityPolicyHash {
		return ModelCapabilityPolicy{}, errors.New("curated visual model policy has changed without a new identity")
	}
	policy.ContentHash = hash
	return policy, nil
}

func CuratedReleases() ([]preset.Release, error) {
	qcPolicy, err := ReferenceQCPolicy()
	if err != nil {
		return nil, err
	}
	modelPolicy, err := VisualModelCapabilityPolicy()
	if err != nil {
		return nil, err
	}
	inputs := curatedReleaseInputs(qcPolicy.ContentHash, modelPolicy.ContentHash)
	releases := make([]preset.Release, 0, len(inputs))
	for _, input := range inputs {
		release, _, releaseErr := preset.NewRelease(input)
		if releaseErr != nil {
			return nil, releaseErr
		}
		releases = append(releases, release)
	}
	slices.SortFunc(releases, func(left, right preset.Release) int {
		if left.Key < right.Key {
			return -1
		}
		if left.Key > right.Key {
			return 1
		}
		return 0
	})
	for index := range releases {
		if index > 0 && releases[index-1].Key == releases[index].Key {
			return nil, errors.New("curated Preset catalog contains a duplicate release")
		}
		if expected, exists := curatedReleaseHashes[releases[index].Key]; !exists || expected != releases[index].ContentHash {
			return nil, errors.New("curated Preset release has changed without a new identity")
		}
	}
	return releases, nil
}

func FindCuratedRelease(key, release string) (preset.Release, bool, error) {
	releases, err := CuratedReleases()
	if err != nil {
		return preset.Release{}, false, err
	}
	for _, candidate := range releases {
		if candidate.Key == key && candidate.Release == release {
			return candidate, true, nil
		}
	}
	return preset.Release{}, false, nil
}

func catalogHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var material map[string]json.RawMessage
	if err = json.Unmarshal(raw, &material); err != nil {
		return "", err
	}
	delete(material, "content_hash")
	encoded, err := json.Marshal(material)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(encoded)
}

func curatedReleaseInputs(qcPolicyHash, modelPolicyHash string) []preset.ReleaseInput {
	return []preset.ReleaseInput{
		curatedReleaseInput(
			"urban-cinematic-realism", "都市电影写实", "cinematic-realism",
			"以自然材质、可信空间与克制电影光线表达当代故事。",
			preset.WorldDesignBasis{
				Era: "contemporary", Region: "script_defined", CivilizationLanguage: "grounded_urban",
				TechnologyOrMagicLanguage: "script_defined", ArchitectureLanguage: "functional_contemporary",
				WardrobeLanguage: "character_led_realism", PropLanguage: "functional_realism",
				MaterialSystem: "natural_wear", Motifs: []string{"controlled_reflection", "lived_in_detail"},
				AnachronismConstraints: []string{"preserve_script_era"},
			},
			preset.VisualGrammar{
				Medium: "cinematic_digital", Realism: "semi_photoreal", ShapeLanguage: "naturalistic",
				Proportion: "human_realistic", Palette: "motivated_neutral", Linework: "none",
				Texture: "material_specific", MaterialRendering: "physically_plausible",
				Lighting: "motivated_practical", Contrast: "controlled", Composition: "narrative_clarity",
				Camera: "grounded_cinematic", NegativeConstraints: []string{"artist_name", "protected_ip_imitation"},
			}, qcPolicyHash, modelPolicyHash,
		),
		curatedReleaseInput(
			"period-cinematic-realism", "古装电影写实", "period-realism",
			"以时代可信的建筑、服饰与旧化材质形成克制的古装电影质感。",
			preset.WorldDesignBasis{
				Era: "script_defined_period", Region: "script_defined", CivilizationLanguage: "historically_grounded",
				TechnologyOrMagicLanguage: "script_defined", ArchitectureLanguage: "regional_period_construction",
				WardrobeLanguage: "status_and_occupation_led", PropLanguage: "period_functional",
				MaterialSystem: "aged_natural_materials", Motifs: []string{"crafted_joinery", "weathered_surfaces"},
				AnachronismConstraints: []string{"exclude_unconfirmed_modern_objects", "preserve_script_era"},
			},
			preset.VisualGrammar{
				Medium: "cinematic_digital", Realism: "semi_photoreal", ShapeLanguage: "structural_naturalism",
				Proportion: "human_realistic", Palette: "earth_and_mineral", Linework: "none",
				Texture: "handcrafted_wear", MaterialRendering: "physically_plausible",
				Lighting: "natural_directional", Contrast: "measured", Composition: "layered_depth",
				Camera: "observational_cinematic", NegativeConstraints: []string{"artist_name", "protected_ip_imitation"},
			}, qcPolicyHash, modelPolicyHash,
		),
		curatedReleaseInput(
			"chinese-fantasy-animation", "国漫仙侠", "fantasy-animation",
			"以东方山水秩序、流动灵气与层叠服饰表达仙侠世界，同时保留人物和剧情事实。",
			preset.WorldDesignBasis{
				Era: "script_defined", Region: "script_defined", CivilizationLanguage: "eastern_fantasy",
				TechnologyOrMagicLanguage: "ritualized_spiritual_system", ArchitectureLanguage: "mountain_courtyard_verticality",
				WardrobeLanguage: "layered_flowing_silhouette", PropLanguage: "crafted_spiritual_function",
				MaterialSystem: "silk_stone_wood_and_energy", Motifs: []string{"cloud_rhythm", "mountain_axis", "seal_geometry"},
				AnachronismConstraints: []string{"explicit_decision_for_world_translation", "preserve_script_function"},
			},
			preset.VisualGrammar{
				Medium: "stylized_3d_animation", Realism: "semi_realistic", ShapeLanguage: "flowing_and_monumental",
				Proportion: "heroic_human", Palette: "mineral_with_luminous_accents", Linework: "selective",
				Texture: "silk_ink_stone", MaterialRendering: "stylized_physical",
				Lighting: "atmospheric_volume", Contrast: "luminous_depth", Composition: "landscape_hierarchy",
				Camera: "dynamic_cinematic", NegativeConstraints: []string{"artist_name", "protected_ip_imitation"},
			}, qcPolicyHash, modelPolicyHash,
		),
		curatedReleaseInput(
			"cyberpunk-animation", "赛博朋克动画", "cyberpunk-animation",
			"以高密度城市层级、可读发光界面和磨损工业材质表达近未来冲突。",
			preset.WorldDesignBasis{
				Era: "near_future", Region: "script_defined", CivilizationLanguage: "dense_networked_urban",
				TechnologyOrMagicLanguage: "visible_computational_infrastructure", ArchitectureLanguage: "layered_megastructure",
				WardrobeLanguage: "functional_techwear", PropLanguage: "modular_worn_technology",
				MaterialSystem: "industrial_composite_and_emissive", Motifs: []string{"cable_routing", "modular_panels", "signal_layers"},
				AnachronismConstraints: []string{"explicit_decision_for_technology_translation", "preserve_script_function"},
			},
			preset.VisualGrammar{
				Medium: "stylized_animation", Realism: "semi_realistic", ShapeLanguage: "angular_layered",
				Proportion: "human_realistic", Palette: "dark_neutral_with_signal_color", Linework: "graphic_selective",
				Texture: "worn_industrial", MaterialRendering: "stylized_physical",
				Lighting: "motivated_emissive", Contrast: "high_readability", Composition: "dense_but_legible",
				Camera: "kinetic_cinematic", NegativeConstraints: []string{"artist_name", "protected_ip_imitation"},
			}, qcPolicyHash, modelPolicyHash,
		),
	}
}

func curatedReleaseInput(
	key, label, category, description string,
	world preset.WorldDesignBasis,
	visual preset.VisualGrammar,
	qcPolicyHash, modelPolicyHash string,
) preset.ReleaseInput {
	return preset.ReleaseInput{
		Key: key, Release: curatedRelease, Label: label, Category: category, Description: description,
		DefaultMode: "faithful",
		Provenance: preset.Provenance{
			Origin: "first_party", SourceURL: "https://github.com/StephenQiu30/lanverse",
			LicenseSPDX: "MIT", NoticePath: "backend/internal/preset/catalog/NOTICE.md",
		},
		CapabilityManifest:       curatedCapabilities(),
		WorldDesignBasis:         world,
		VisualGrammar:            visual,
		FidelityInvariants:       []string{"character_identity", "holder_relation", "scene_continuity", "story_fact"},
		WorldAdaptationRules:     curatedWorldAdaptationRules(),
		PurposeProfiles:          curatedPurposeProfiles(),
		SkillReleaseRefs:         []preset.ContentRef{{Owner: "agent/skill", Key: "build-storygraph", ContentHash: contract.StoryGraphSkillBundleHash}},
		QCPolicyRef:              preset.ContentRef{Owner: "preset/policy", Key: "reference-qc", ContentHash: qcPolicyHash},
		ModelCapabilityPolicyRef: preset.ContentRef{Owner: "preset/policy", Key: "visual-model-capability", ContentHash: modelPolicyHash},
	}
}

func curatedCapabilities() []preset.Capability {
	return []preset.Capability{
		{TargetKind: "character_identity_anchor", ViewRoles: []string{"back", "front", "profile"}},
		{TargetKind: "character_appearance", ViewRoles: []string{"back", "front", "profile"}},
		{TargetKind: "location_board", ViewRoles: []string{"empty_establishing", "material_scale_detail", "spatial_orientation"}},
		{TargetKind: "prop_sheet", ViewRoles: []string{"back", "front", "side", "state_detail"}},
		{TargetKind: "interaction_composition", ViewRoles: []string{"interaction_master"}},
		{TargetKind: "scene_composition", ViewRoles: []string{"composition_master"}},
	}
}

func curatedPurposeProfiles() []preset.PurposeProfile {
	targetKinds := []string{
		"character_identity_anchor",
		"character_appearance",
		"location_board",
		"prop_sheet",
		"interaction_composition",
		"scene_composition",
	}
	profiles := make([]preset.PurposeProfile, 0, len(targetKinds))
	for _, targetKind := range targetKinds {
		profiles = append(profiles, preset.PurposeProfile{
			TargetKind:       targetKind,
			DesignFocus:      []string{"identity_fidelity", "production_fact_fidelity", "visual_coherence"},
			ForbiddenChanges: []string{"protected_ip_imitation", "script_fact_rewrite"},
		})
	}
	return profiles
}

func curatedWorldAdaptationRules() []preset.WorldAdaptationRule {
	invariants := []string{"character_identity", "holder_relation", "scene_continuity", "story_fact"}
	return []preset.WorldAdaptationRule{
		{
			RuleKey: "translate-architecture-language", SourceFactKind: "architecture", DesignDomain: "architecture",
			Directive:              "translate_visual_language_without_changing_place_function_or_scene_continuity",
			PreservedInvariantKeys: append([]string(nil), invariants...),
			ImpactScopeKinds:       []string{"asset", "reference_plan", "scene", "storyboard"}, RequiresHumanDecision: true,
		},
		{
			RuleKey: "translate-technology-language", SourceFactKind: "technology_or_magic", DesignDomain: "technology_or_magic",
			Directive:              "translate_expression_without_changing_function_holder_or_story_outcome",
			PreservedInvariantKeys: append([]string(nil), invariants...),
			ImpactScopeKinds:       []string{"asset", "interaction", "reference_plan", "scene", "storyboard"}, RequiresHumanDecision: true,
		},
		{
			RuleKey: "translate-wardrobe-language", SourceFactKind: "wardrobe", DesignDomain: "wardrobe",
			Directive:              "translate_visual_language_without_changing_character_identity_or_confirmed_appearance_state",
			PreservedInvariantKeys: append([]string(nil), invariants...),
			ImpactScopeKinds:       []string{"asset", "reference_plan", "scene", "storyboard"}, RequiresHumanDecision: true,
		},
	}
}
