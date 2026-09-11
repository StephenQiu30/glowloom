package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const ReleaseContractID = "preset-release-production"

var (
	releaseKeyPattern  = regexp.MustCompile(`^[a-z][a-z0-9-]{2,63}$`)
	releaseDatePattern = regexp.MustCompile(`^[0-9]{4}\.(0[1-9]|1[0-2])\.(0[1-9]|[12][0-9]|3[01])$`)
	stableIDPattern    = regexp.MustCompile(`^[a-z][a-z0-9/_-]{1,127}$`)
	spdxPattern        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+-]{0,63}$`)
	hashPattern        = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

var releaseTargetKinds = []string{
	"character_identity_anchor",
	"character_appearance",
	"location_board",
	"prop_sheet",
	"interaction_composition",
	"scene_composition",
}

var releaseViewRoles = map[string][]string{
	"character_identity_anchor": {"back", "front", "profile"},
	"character_appearance":      {"back", "front", "profile"},
	"location_board":            {"empty_establishing", "material_scale_detail", "spatial_orientation"},
	"prop_sheet":                {"back", "front", "side", "state_detail"},
	"interaction_composition":   {"interaction_master"},
	"scene_composition":         {"composition_master"},
}

var fidelityInvariantKeys = []string{
	"character_identity",
	"holder_relation",
	"scene_continuity",
	"story_fact",
}

type Provenance struct {
	Origin      string `json:"origin"`
	SourceURL   string `json:"source_url"`
	LicenseSPDX string `json:"license_spdx"`
	NoticePath  string `json:"notice_path"`
}

type Capability struct {
	TargetKind string   `json:"target_kind"`
	ViewRoles  []string `json:"view_roles"`
}

type WorldDesignBasis struct {
	Era                       string   `json:"era"`
	Region                    string   `json:"region"`
	CivilizationLanguage      string   `json:"civilization_language"`
	TechnologyOrMagicLanguage string   `json:"technology_or_magic_language"`
	ArchitectureLanguage      string   `json:"architecture_language"`
	WardrobeLanguage          string   `json:"wardrobe_language"`
	PropLanguage              string   `json:"prop_language"`
	MaterialSystem            string   `json:"material_system"`
	Motifs                    []string `json:"motifs"`
	AnachronismConstraints    []string `json:"anachronism_constraints"`
}

type VisualGrammar struct {
	Medium              string   `json:"medium"`
	Realism             string   `json:"realism"`
	ShapeLanguage       string   `json:"shape_language"`
	Proportion          string   `json:"proportion"`
	Palette             string   `json:"palette"`
	Linework            string   `json:"linework"`
	Texture             string   `json:"texture"`
	MaterialRendering   string   `json:"material_rendering"`
	Lighting            string   `json:"lighting"`
	Contrast            string   `json:"contrast"`
	Composition         string   `json:"composition"`
	Camera              string   `json:"camera"`
	NegativeConstraints []string `json:"negative_constraints"`
}

type PurposeProfile struct {
	TargetKind       string   `json:"target_kind"`
	DesignFocus      []string `json:"design_focus"`
	ForbiddenChanges []string `json:"forbidden_changes"`
}

type WorldAdaptationRule struct {
	RuleKey                string   `json:"rule_key"`
	SourceFactKind         string   `json:"source_fact_kind"`
	DesignDomain           string   `json:"design_domain"`
	Directive              string   `json:"directive"`
	PreservedInvariantKeys []string `json:"preserved_invariant_keys"`
	ImpactScopeKinds       []string `json:"impact_scope_kinds"`
	RequiresHumanDecision  bool     `json:"requires_human_decision"`
}

type ContentRef struct {
	Owner       string `json:"owner"`
	Key         string `json:"key"`
	ContentHash string `json:"content_hash"`
}

type ReleaseInput struct {
	Key                      string                `json:"key"`
	Release                  string                `json:"release"`
	Label                    string                `json:"label"`
	Category                 string                `json:"category"`
	Description              string                `json:"description"`
	DefaultMode              string                `json:"default_mode"`
	Provenance               Provenance            `json:"provenance"`
	CapabilityManifest       []Capability          `json:"capability_manifest"`
	WorldDesignBasis         WorldDesignBasis      `json:"world_design_basis"`
	VisualGrammar            VisualGrammar         `json:"visual_grammar"`
	FidelityInvariants       []string              `json:"fidelity_invariants"`
	WorldAdaptationRules     []WorldAdaptationRule `json:"world_adaptation_rules"`
	PurposeProfiles          []PurposeProfile      `json:"purpose_profiles"`
	SkillReleaseRefs         []ContentRef          `json:"skill_release_refs"`
	QCPolicyRef              ContentRef            `json:"qc_policy_ref"`
	ModelCapabilityPolicyRef ContentRef            `json:"model_capability_policy_ref"`
}

type Release struct {
	ContractID string `json:"contract_id"`
	ReleaseInput
	ContentHash string `json:"content_hash"`
}

func NewRelease(input ReleaseInput) (Release, json.RawMessage, error) {
	if err := validateReleaseInput(input); err != nil {
		return Release{}, nil, err
	}
	release := Release{ContractID: ReleaseContractID, ReleaseInput: input}
	hash, err := releaseHash(release)
	if err != nil {
		return Release{}, nil, err
	}
	release.ContentHash = hash
	encoded, err := encodeRelease(release)
	if err != nil {
		return Release{}, nil, err
	}
	return release, encoded, nil
}

func DecodeRelease(raw json.RawMessage) (Release, json.RawMessage, error) {
	var release Release
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&release); err != nil {
		return Release{}, nil, errors.New("invalid Preset release")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Release{}, nil, errors.New("invalid Preset release")
	}
	if release.ContractID != ReleaseContractID || !hashPattern.MatchString(release.ContentHash) || validateReleaseInput(release.ReleaseInput) != nil {
		return Release{}, nil, errors.New("invalid Preset release")
	}
	hash, err := releaseHash(release)
	if err != nil || hash != release.ContentHash {
		return Release{}, nil, errors.New("Preset release content hash has drifted")
	}
	encoded, err := encodeRelease(release)
	if err != nil {
		return Release{}, nil, err
	}
	return release, encoded, nil
}

func releaseHash(release Release) (string, error) {
	raw, err := json.Marshal(struct {
		ContractID string `json:"contract_id"`
		ReleaseInput
	}{ContractID: release.ContractID, ReleaseInput: release.ReleaseInput})
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func encodeRelease(release Release) (json.RawMessage, error) {
	raw, err := json.Marshal(release)
	if err != nil {
		return nil, err
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(canonical), nil
}

func validateReleaseInput(value ReleaseInput) error {
	if !releaseKeyPattern.MatchString(value.Key) || !validReleaseDate(value.Release) ||
		!stableText(value.Label) || !releaseKeyPattern.MatchString(value.Category) || !stableText(value.Description) ||
		!slices.Contains([]string{"faithful", "world_adaptation"}, value.DefaultMode) || validateProvenance(value.Provenance) != nil ||
		validateCapabilities(value.CapabilityManifest) != nil || validateWorldDesignBasis(value.WorldDesignBasis) != nil ||
		validateVisualGrammar(value.VisualGrammar) != nil || !slices.Equal(value.FidelityInvariants, fidelityInvariantKeys) ||
		validateWorldAdaptationRules(value.WorldAdaptationRules) != nil || validatePurposeProfiles(value.PurposeProfiles) != nil ||
		validateContentRefs(value.SkillReleaseRefs, true) != nil || validateContentRef(value.QCPolicyRef) != nil ||
		validateContentRef(value.ModelCapabilityPolicyRef) != nil {
		return errors.New("invalid Preset release contract")
	}
	return nil
}

func validateWorldAdaptationRules(values []WorldAdaptationRule) error {
	designDomains := []string{"architecture", "civilization", "material", "prop", "technology_or_magic", "wardrobe"}
	impactScopes := map[string]struct{}{
		"asset": {}, "interaction": {}, "reference_plan": {}, "scene": {}, "storyboard": {},
	}
	if len(values) == 0 {
		return errors.New("Preset release must declare world adaptation rules")
	}
	for index, value := range values {
		if !releaseKeyPattern.MatchString(value.RuleKey) || index > 0 && values[index-1].RuleKey >= value.RuleKey ||
			!slices.Contains(designDomains, value.SourceFactKind) || !slices.Contains(designDomains, value.DesignDomain) ||
			!stableText(value.Directive) || !slices.Equal(value.PreservedInvariantKeys, fidelityInvariantKeys) ||
			!value.RequiresHumanDecision || !sortedStableStrings(value.ImpactScopeKinds, true) {
			return errors.New("invalid Preset world adaptation rule")
		}
		for _, scope := range value.ImpactScopeKinds {
			if _, exists := impactScopes[scope]; !exists {
				return errors.New("invalid Preset world adaptation scope")
			}
		}
	}
	return nil
}

func validReleaseDate(value string) bool {
	if !releaseDatePattern.MatchString(value) {
		return false
	}
	parsed, err := time.Parse("2006.01.02", value)
	return err == nil && parsed.Format("2006.01.02") == value
}

func validateProvenance(value Provenance) error {
	parsed, err := url.Parse(value.SourceURL)
	cleanNotice := path.Clean(value.NoticePath)
	if !slices.Contains([]string{"first_party", "open_source"}, value.Origin) || err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		!spdxPattern.MatchString(value.LicenseSPDX) || cleanNotice != value.NoticePath || strings.HasPrefix(cleanNotice, "/") ||
		strings.HasPrefix(cleanNotice, "../") || !strings.HasSuffix(cleanNotice, ".md") {
		return errors.New("invalid Preset provenance")
	}
	return nil
}

func validateCapabilities(values []Capability) error {
	if len(values) != len(releaseTargetKinds) {
		return errors.New("Preset release must support every production target kind")
	}
	for index, targetKind := range releaseTargetKinds {
		if values[index].TargetKind != targetKind || !slices.Equal(values[index].ViewRoles, releaseViewRoles[targetKind]) {
			return errors.New("invalid Preset capability manifest")
		}
	}
	return nil
}

func validatePurposeProfiles(values []PurposeProfile) error {
	if len(values) != len(releaseTargetKinds) {
		return errors.New("Preset release must declare every purpose profile")
	}
	for index, targetKind := range releaseTargetKinds {
		value := values[index]
		if value.TargetKind != targetKind || !sortedStableStrings(value.DesignFocus, true) || !sortedStableStrings(value.ForbiddenChanges, true) {
			return errors.New("invalid Preset purpose profile")
		}
	}
	return nil
}

func validateWorldDesignBasis(value WorldDesignBasis) error {
	fields := []string{
		value.Era, value.Region, value.CivilizationLanguage, value.TechnologyOrMagicLanguage,
		value.ArchitectureLanguage, value.WardrobeLanguage, value.PropLanguage, value.MaterialSystem,
	}
	for _, field := range fields {
		if !stableText(field) {
			return errors.New("invalid Preset world design basis")
		}
	}
	if !sortedStableStrings(value.Motifs, true) || !sortedStableStrings(value.AnachronismConstraints, true) {
		return errors.New("invalid Preset world design basis")
	}
	return nil
}

func validateVisualGrammar(value VisualGrammar) error {
	fields := []string{
		value.Medium, value.Realism, value.ShapeLanguage, value.Proportion, value.Palette, value.Linework,
		value.Texture, value.MaterialRendering, value.Lighting, value.Contrast, value.Composition, value.Camera,
	}
	for _, field := range fields {
		if !stableText(field) {
			return errors.New("invalid Preset visual grammar")
		}
	}
	if !sortedStableStrings(value.NegativeConstraints, true) {
		return errors.New("invalid Preset visual grammar")
	}
	return nil
}

func validateContentRefs(values []ContentRef, requireNonempty bool) error {
	if values == nil || requireNonempty && len(values) == 0 {
		return errors.New("missing Preset content reference")
	}
	previous := ""
	for _, value := range values {
		if err := validateContentRef(value); err != nil {
			return err
		}
		key := value.Owner + "\x00" + value.Key
		if previous != "" && key <= previous {
			return errors.New("Preset content references are not sorted and unique")
		}
		previous = key
	}
	return nil
}

func validateContentRef(value ContentRef) error {
	if !stableIDPattern.MatchString(value.Owner) || !stableIDPattern.MatchString(value.Key) || !hashPattern.MatchString(value.ContentHash) {
		return errors.New("invalid Preset content reference")
	}
	return nil
}

func sortedStableStrings(values []string, requireNonempty bool) bool {
	if values == nil || requireNonempty && len(values) == 0 {
		return false
	}
	for index, value := range values {
		if !stableText(value) || index > 0 && strings.Compare(values[index-1], value) >= 0 {
			return false
		}
	}
	return true
}

func stableText(value string) bool {
	return strings.TrimSpace(value) != "" && strings.TrimSpace(value) == value && len(value) <= 500 && norm.NFC.IsNormalString(value)
}
