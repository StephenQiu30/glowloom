package contract

import (
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const (
	VisualFoundationSchemaSetContractID = "storygraph-visual-foundation-schema-set-production"
	VisualFoundationCandidateContractID = "visual-foundation-candidate-production"
	VisualFoundationCandidateSchemaHash = "2c4b612079acb4874032e0e8358a272bd71cce45e6473209dda21cef29e40c4c"
	VisualFoundationInputContractID     = "visual-foundation-input-production"
	VisualFoundationInputSchemaHash     = "4fe72c85b00f4f3348c6bb16e908d74365c873bddd4b0d5c3e919a67b00793df"
)

type VisualFoundationSchema struct {
	ContractID string `json:"contract_id"`
	SchemaHash string `json:"schema_hash"`
}

type VisualFoundationSchemaManifest struct {
	ContractID    string                   `json:"contract_id"`
	SchemaSetHash string                   `json:"schema_set_hash"`
	Schemas       []VisualFoundationSchema `json:"schemas"`
}

var visualFoundationSchemas = []VisualFoundationSchema{
	{ContractID: VisualFoundationCandidateContractID, SchemaHash: VisualFoundationCandidateSchemaHash},
	{ContractID: VisualFoundationInputContractID, SchemaHash: VisualFoundationInputSchemaHash},
}

var (
	visualFoundationReleaseKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{2,63}$`)
	visualFoundationReleasePattern    = regexp.MustCompile(`^[0-9]{4}\.(0[1-9]|1[0-2])\.(0[1-9]|[12][0-9]|3[01])$`)
	visualFoundationRuleKeyPattern    = regexp.MustCompile(`^[a-z][a-z0-9-]{2,63}$`)
	visualFoundationOverridePattern   = regexp.MustCompile(`^override_[a-z0-9_]{1,120}$`)
	visualFoundationFactPattern       = regexp.MustCompile(`^fact_[a-z0-9_]{1,120}$`)
	visualFoundationGapPattern        = regexp.MustCompile(`^gap_[a-z0-9_]{1,120}$`)
	visualFoundationMappingPattern    = regexp.MustCompile(`^mapping_[a-z0-9_]{1,120}$`)
	visualFoundationConflictPattern   = regexp.MustCompile(`^conflict_[a-z0-9_]{1,120}$`)
)

var visualFoundationFidelityInvariants = []string{
	"character_identity",
	"holder_relation",
	"scene_continuity",
	"story_fact",
}

type ConfirmedWorldRoot struct {
	OwnerFamily        string `json:"owner_family"`
	ScopeKey           string `json:"scope_key"`
	CollectionRootHash string `json:"collection_root_hash"`
}

type VisualFoundationPresetReleaseIdentity struct {
	Key         string `json:"key"`
	Release     string `json:"release"`
	ContentHash string `json:"content_hash"`
}

type PresetAdaptationRuleSnapshot struct {
	RuleKey                string   `json:"rule_key"`
	SourceFactKind         string   `json:"source_fact_kind"`
	DesignDomain           string   `json:"design_domain"`
	Directive              string   `json:"directive"`
	PreservedInvariantKeys []string `json:"preserved_invariant_keys"`
	ImpactScopeKinds       []string `json:"impact_scope_kinds"`
}

type VisualGrammarSnapshot struct {
	Palette             string   `json:"palette"`
	MaterialRendering   string   `json:"material_rendering"`
	Lighting            string   `json:"lighting"`
	Camera              string   `json:"camera"`
	NegativeConstraints []string `json:"negative_constraints"`
}

type TypedVisualOverride struct {
	OverrideKey         string `json:"override_key"`
	ScopeKind           string `json:"scope_kind"`
	ScopeKey            string `json:"scope_key"`
	DesignDomain        string `json:"design_domain"`
	Value               string `json:"value"`
	CreatorDecisionHash string `json:"creator_decision_hash"`
}

type VisualReferenceAttachment struct {
	AttachmentID  string `json:"attachment_id"`
	ObjectKey     string `json:"object_key"`
	ContentHash   string `json:"content_hash"`
	MediaType     string `json:"media_type"`
	RightsBasis   string `json:"rights_basis"`
	RightsRefHash string `json:"rights_ref_hash"`
}

type ConfirmedWorldFact struct {
	FactKey           string   `json:"fact_key"`
	FactKind          string   `json:"fact_kind"`
	ContentHash       string   `json:"content_hash"`
	AffectedScopeKeys []string `json:"affected_scope_keys"`
}

type VisualDesignGap struct {
	GapKey               string   `json:"gap_key"`
	DesignDomain         string   `json:"design_domain"`
	SourceConstraintHash string   `json:"source_constraint_hash"`
	AffectedScopeKeys    []string `json:"affected_scope_keys"`
}

type VisualFoundationInput struct {
	WorkspaceID                 string                                `json:"workspace_id"`
	ProjectID                   string                                `json:"project_id"`
	ProductionWorldOwnerSetHash string                                `json:"production_world_owner_set_hash"`
	ConfirmedWorldRoots         []ConfirmedWorldRoot                  `json:"confirmed_world_roots"`
	PresetRelease               VisualFoundationPresetReleaseIdentity `json:"preset_release"`
	ApplicationMode             string                                `json:"application_mode"`
	FidelityInvariants          []string                              `json:"fidelity_invariants"`
	AdaptationRules             []PresetAdaptationRuleSnapshot        `json:"adaptation_rules"`
	VisualGrammar               VisualGrammarSnapshot                 `json:"visual_grammar"`
	TypedOverrides              []TypedVisualOverride                 `json:"typed_overrides"`
	TypedOverridesHash          string                                `json:"typed_overrides_hash"`
	ReferenceAttachments        []VisualReferenceAttachment           `json:"reference_attachments"`
	ReferenceAttachmentsHash    string                                `json:"reference_attachments_hash"`
	ConfirmedWorldFacts         []ConfirmedWorldFact                  `json:"confirmed_world_facts"`
	DesignGaps                  []VisualDesignGap                     `json:"design_gaps"`
}

type VisualFoundationPolicy struct {
	PaletteRules     []string `json:"palette_rules"`
	MaterialRules    []string `json:"material_rules"`
	LightingRules    []string `json:"lighting_rules"`
	CameraRules      []string `json:"camera_rules"`
	ForbiddenChanges []string `json:"forbidden_changes"`
}

type WorldAdaptationProposal struct {
	MappingKey             string   `json:"mapping_key"`
	SourceFactKey          string   `json:"source_fact_key"`
	PresetRuleKey          string   `json:"preset_rule_key"`
	DesignValue            string   `json:"design_value"`
	PreservedInvariantKeys []string `json:"preserved_invariant_keys"`
	AffectedScopeKeys      []string `json:"affected_scope_keys"`
	DecisionStatus         string   `json:"decision_status"`
}

type VisualWorldConflict struct {
	ConflictKey       string   `json:"conflict_key"`
	SourceFactKey     string   `json:"source_fact_key"`
	PresetRuleKey     string   `json:"preset_rule_key"`
	Severity          string   `json:"severity"`
	Summary           string   `json:"summary"`
	AffectedScopeKeys []string `json:"affected_scope_keys"`
	ResolutionStatus  string   `json:"resolution_status"`
}

type CreativeFillProposal struct {
	GapKey            string   `json:"gap_key"`
	DesignDomain      string   `json:"design_domain"`
	Proposal          string   `json:"proposal"`
	AffectedScopeKeys []string `json:"affected_scope_keys"`
	DecisionStatus    string   `json:"decision_status"`
}

type VisualFoundationCandidate struct {
	WorkspaceID                 string                    `json:"workspace_id"`
	ProjectID                   string                    `json:"project_id"`
	ProductionWorldOwnerSetHash string                    `json:"production_world_owner_set_hash"`
	PresetReleaseContentHash    string                    `json:"preset_release_content_hash"`
	ApplicationMode             string                    `json:"application_mode"`
	TypedOverridesHash          string                    `json:"typed_overrides_hash"`
	ReferenceAttachmentsHash    string                    `json:"reference_attachments_hash"`
	FidelityInvariants          []string                  `json:"fidelity_invariants"`
	StylePolicy                 VisualFoundationPolicy    `json:"style_policy"`
	WorldAdaptations            []WorldAdaptationProposal `json:"world_adaptations"`
	WorldConflicts              []VisualWorldConflict     `json:"world_conflicts"`
	CreativeFillProposals       []CreativeFillProposal    `json:"creative_fill_proposals"`
}

func DecodeVisualFoundationInput(raw json.RawMessage) (VisualFoundationInput, json.RawMessage, error) {
	var value VisualFoundationInput
	if decodeStrict(raw, &value) != nil || value.Validate() != nil {
		return VisualFoundationInput{}, nil, errors.New("invalid Visual Foundation input")
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return VisualFoundationInput{}, nil, err
	}
	return value, json.RawMessage(canonical), nil
}

func DecodeVisualFoundationSchemaManifest(raw json.RawMessage) (VisualFoundationSchemaManifest, json.RawMessage, error) {
	var value VisualFoundationSchemaManifest
	if decodeStrict(raw, &value) != nil || value.ContractID != VisualFoundationSchemaSetContractID ||
		!reflect.DeepEqual(value.Schemas, visualFoundationSchemas) || !hashPattern.MatchString(value.SchemaSetHash) {
		return VisualFoundationSchemaManifest{}, nil, errors.New("invalid Visual Foundation Schema manifest")
	}
	for _, schema := range value.Schemas {
		if !hashPattern.MatchString(schema.SchemaHash) {
			return VisualFoundationSchemaManifest{}, nil, errors.New("invalid Visual Foundation Schema manifest")
		}
	}
	hash, err := visualFoundationCollectionHash(struct {
		ContractID string                   `json:"contract_id"`
		Schemas    []VisualFoundationSchema `json:"schemas"`
	}{ContractID: value.ContractID, Schemas: value.Schemas})
	if err != nil || hash != value.SchemaSetHash {
		return VisualFoundationSchemaManifest{}, nil, errors.New("Visual Foundation Schema set hash has drifted")
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return VisualFoundationSchemaManifest{}, nil, err
	}
	return value, json.RawMessage(canonical), nil
}

func DecodeVisualFoundationCandidate(raw json.RawMessage) (VisualFoundationCandidate, json.RawMessage, error) {
	var value VisualFoundationCandidate
	if decodeStrict(raw, &value) != nil || value.validateShape() != nil {
		return VisualFoundationCandidate{}, nil, errors.New("invalid Visual Foundation Candidate")
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return VisualFoundationCandidate{}, nil, err
	}
	return value, json.RawMessage(canonical), nil
}

func (value VisualFoundationInput) Validate() error {
	workspaceID, workspaceErr := uuid.Parse(value.WorkspaceID)
	projectID, projectErr := uuid.Parse(value.ProjectID)
	if workspaceErr != nil || projectErr != nil || workspaceID == uuid.Nil || projectID == uuid.Nil ||
		!hashPattern.MatchString(value.ProductionWorldOwnerSetHash) ||
		value.ConfirmedWorldRoots == nil || value.FidelityInvariants == nil || value.AdaptationRules == nil ||
		value.TypedOverrides == nil || value.ReferenceAttachments == nil ||
		value.ConfirmedWorldFacts == nil || value.DesignGaps == nil ||
		!reflect.DeepEqual(value.FidelityInvariants, visualFoundationFidelityInvariants) ||
		!validVisualFoundationMode(value.ApplicationMode) || validateVisualFoundationPreset(value.PresetRelease) != nil ||
		validateConfirmedWorldRoots(value.ProjectID, value.ConfirmedWorldRoots) != nil ||
		validateAdaptationRuleSnapshots(value.AdaptationRules) != nil || validateVisualGrammarSnapshot(value.VisualGrammar) != nil ||
		validateTypedVisualOverrides(value.ProjectID, value.TypedOverrides) != nil ||
		validateVisualReferenceAttachments(value.ReferenceAttachments) != nil ||
		validateConfirmedWorldFacts(value.ConfirmedWorldFacts) != nil || validateVisualDesignGaps(value.DesignGaps) != nil {
		return errors.New("invalid Visual Foundation input")
	}
	overrideHash, err := visualFoundationCollectionHash(value.TypedOverrides)
	if err != nil || overrideHash != value.TypedOverridesHash {
		return errors.New("typed visual override set hash drifted")
	}
	attachmentHash, err := visualFoundationCollectionHash(value.ReferenceAttachments)
	if err != nil || attachmentHash != value.ReferenceAttachmentsHash {
		return errors.New("visual reference attachment set hash drifted")
	}
	return nil
}

func (value VisualFoundationCandidate) validateShape() error {
	if identifier, err := uuid.Parse(value.WorkspaceID); err != nil || identifier == uuid.Nil {
		return errors.New("invalid Visual Foundation Candidate workspace")
	}
	if identifier, err := uuid.Parse(value.ProjectID); err != nil || identifier == uuid.Nil {
		return errors.New("invalid Visual Foundation Candidate project")
	}
	if !hashPattern.MatchString(value.ProductionWorldOwnerSetHash) ||
		!hashPattern.MatchString(value.PresetReleaseContentHash) ||
		!hashPattern.MatchString(value.TypedOverridesHash) ||
		!hashPattern.MatchString(value.ReferenceAttachmentsHash) ||
		value.FidelityInvariants == nil || value.WorldAdaptations == nil || value.WorldConflicts == nil ||
		value.CreativeFillProposals == nil ||
		!validVisualFoundationMode(value.ApplicationMode) ||
		!reflect.DeepEqual(value.FidelityInvariants, visualFoundationFidelityInvariants) ||
		validateVisualFoundationPolicy(value.StylePolicy) != nil ||
		validateWorldAdaptationProposals(value.WorldAdaptations) != nil ||
		validateVisualWorldConflicts(value.WorldConflicts) != nil ||
		validateCreativeFillProposals(value.CreativeFillProposals) != nil {
		return errors.New("invalid Visual Foundation Candidate")
	}
	return nil
}

func (value VisualFoundationCandidate) ValidateFor(input VisualFoundationInput) error {
	if input.Validate() != nil || value.validateShape() != nil || value.WorkspaceID != input.WorkspaceID ||
		value.ProjectID != input.ProjectID || value.ProductionWorldOwnerSetHash != input.ProductionWorldOwnerSetHash ||
		value.PresetReleaseContentHash != input.PresetRelease.ContentHash || value.ApplicationMode != input.ApplicationMode ||
		value.TypedOverridesHash != input.TypedOverridesHash || value.ReferenceAttachmentsHash != input.ReferenceAttachmentsHash ||
		!reflect.DeepEqual(value.FidelityInvariants, input.FidelityInvariants) {
		return errors.New("Visual Foundation Candidate input lineage drifted")
	}
	if value.ApplicationMode == "faithful" && len(value.WorldAdaptations) != 0 {
		return errors.New("faithful Visual Foundation cannot adapt confirmed world facts")
	}
	facts := make(map[string]ConfirmedWorldFact, len(input.ConfirmedWorldFacts))
	for _, fact := range input.ConfirmedWorldFacts {
		facts[fact.FactKey] = fact
	}
	rules := make(map[string]PresetAdaptationRuleSnapshot, len(input.AdaptationRules))
	for _, rule := range input.AdaptationRules {
		rules[rule.RuleKey] = rule
	}
	for _, proposal := range value.WorldAdaptations {
		fact, factExists := facts[proposal.SourceFactKey]
		rule, ruleExists := rules[proposal.PresetRuleKey]
		if !factExists || !ruleExists || fact.FactKind != rule.SourceFactKind ||
			!slices.Equal(proposal.AffectedScopeKeys, fact.AffectedScopeKeys) {
			return errors.New("world adaptation is outside the frozen fact and Preset rule")
		}
	}
	for _, conflict := range value.WorldConflicts {
		fact, factExists := facts[conflict.SourceFactKey]
		_, ruleExists := rules[conflict.PresetRuleKey]
		if !factExists || !ruleExists || !slices.Equal(conflict.AffectedScopeKeys, fact.AffectedScopeKeys) {
			return errors.New("world conflict is outside the frozen fact and Preset rule")
		}
	}
	if len(value.CreativeFillProposals) != len(input.DesignGaps) {
		return errors.New("Visual Foundation Candidate must cover every frozen Design Gap")
	}
	for index, proposal := range value.CreativeFillProposals {
		gap := input.DesignGaps[index]
		if proposal.GapKey != gap.GapKey || proposal.DesignDomain != gap.DesignDomain ||
			!slices.Equal(proposal.AffectedScopeKeys, gap.AffectedScopeKeys) {
			return errors.New("creative fill proposal drifted from its frozen Design Gap")
		}
	}
	return nil
}

func validateConfirmedWorldRoots(projectID string, values []ConfirmedWorldRoot) error {
	if len(values) < 3 {
		return errors.New("Visual Foundation input lacks confirmed world roots")
	}
	keys := make([]string, len(values))
	projectScope := "project:" + projectID
	projectFamilies := []string{}
	planningCount := 0
	for index, value := range values {
		if !hashPattern.MatchString(value.CollectionRootHash) {
			return errors.New("invalid confirmed world root")
		}
		keys[index] = value.OwnerFamily + "\x00" + value.ScopeKey + "\x00" + value.CollectionRootHash
		switch value.OwnerFamily {
		case "asset_identity_state_set", "bible_production_world_set":
			if value.ScopeKey != projectScope {
				return errors.New("confirmed world root crosses its project scope")
			}
			projectFamilies = append(projectFamilies, value.OwnerFamily)
		case "planning_scene_set":
			if !validVisualFoundationScope(value.ScopeKey, "episode") {
				return errors.New("invalid planning world root")
			}
			planningCount++
		default:
			return errors.New("invalid confirmed world root family")
		}
	}
	if !sortedUnique(keys) || !slices.Equal(projectFamilies, []string{"asset_identity_state_set", "bible_production_world_set"}) || planningCount == 0 {
		return errors.New("confirmed world roots are incomplete or unordered")
	}
	return nil
}

func validateVisualFoundationPreset(value VisualFoundationPresetReleaseIdentity) error {
	if !visualFoundationReleaseKeyPattern.MatchString(value.Key) ||
		!visualFoundationReleasePattern.MatchString(value.Release) || !hashPattern.MatchString(value.ContentHash) {
		return errors.New("invalid Visual Foundation Preset release")
	}
	return nil
}

func validateAdaptationRuleSnapshots(values []PresetAdaptationRuleSnapshot) error {
	keys := make([]string, len(values))
	for index, value := range values {
		keys[index] = value.RuleKey
		if !visualFoundationRuleKeyPattern.MatchString(value.RuleKey) || !validVisualDesignDomain(value.SourceFactKind) ||
			!validVisualDesignDomain(value.DesignDomain) || !validVisualText(value.Directive, 800) ||
			!reflect.DeepEqual(value.PreservedInvariantKeys, visualFoundationFidelityInvariants) ||
			!sortedUnique(value.ImpactScopeKinds) {
			return errors.New("invalid Preset adaptation rule snapshot")
		}
		for _, scope := range value.ImpactScopeKinds {
			if !slices.Contains([]string{"asset", "interaction", "reference_plan", "scene", "storyboard"}, scope) {
				return errors.New("invalid Preset adaptation impact kind")
			}
		}
	}
	if len(values) == 0 || !sortedUnique(keys) {
		return errors.New("Preset adaptation rules must be non-empty, sorted, and unique")
	}
	return nil
}

func validateVisualGrammarSnapshot(value VisualGrammarSnapshot) error {
	for _, field := range []string{value.Palette, value.MaterialRendering, value.Lighting, value.Camera} {
		if !validVisualText(field, 800) {
			return errors.New("invalid visual grammar snapshot")
		}
	}
	if len(value.NegativeConstraints) == 0 || !sortedUnique(value.NegativeConstraints) {
		return errors.New("invalid visual grammar constraints")
	}
	return nil
}

func validateTypedVisualOverrides(projectID string, values []TypedVisualOverride) error {
	keys := make([]string, len(values))
	for index, value := range values {
		keys[index] = value.OverrideKey
		if !visualFoundationOverridePattern.MatchString(value.OverrideKey) ||
			!slices.Contains([]string{"asset", "project"}, value.ScopeKind) ||
			!validVisualFoundationScope(value.ScopeKey, value.ScopeKind) || !validVisualDesignDomain(value.DesignDomain) ||
			!validVisualText(value.Value, 1200) || !hashPattern.MatchString(value.CreatorDecisionHash) ||
			value.ScopeKind == "project" && value.ScopeKey != "project:"+projectID {
			return errors.New("invalid typed visual override")
		}
	}
	if !sort.StringsAreSorted(keys) || len(keys) != len(slices.Compact(append([]string(nil), keys...))) {
		return errors.New("typed visual overrides must be sorted and unique")
	}
	return nil
}

func validateVisualReferenceAttachments(values []VisualReferenceAttachment) error {
	keys := make([]string, len(values))
	for index, value := range values {
		keys[index] = value.ObjectKey
		identifier, err := uuid.Parse(value.AttachmentID)
		parts := strings.Split(value.ObjectKey, "/")
		if err != nil || identifier == uuid.Nil || !validVisualObjectKey(value.ObjectKey, parts) ||
			!hashPattern.MatchString(value.ContentHash) ||
			!slices.Contains([]string{"image/jpeg", "image/png", "image/webp"}, value.MediaType) ||
			!slices.Contains([]string{"licensed", "owned", "public_domain"}, value.RightsBasis) ||
			!hashPattern.MatchString(value.RightsRefHash) {
			return errors.New("invalid visual reference attachment")
		}
	}
	if !sort.StringsAreSorted(keys) || len(keys) != len(slices.Compact(append([]string(nil), keys...))) {
		return errors.New("visual reference attachments must be sorted and unique")
	}
	return nil
}

func validateConfirmedWorldFacts(values []ConfirmedWorldFact) error {
	keys := make([]string, len(values))
	for index, value := range values {
		keys[index] = value.FactKey
		if !visualFoundationFactPattern.MatchString(value.FactKey) || !validVisualDesignDomain(value.FactKind) ||
			!hashPattern.MatchString(value.ContentHash) || !validVisualSceneScopes(value.AffectedScopeKeys) {
			return errors.New("invalid confirmed world fact")
		}
	}
	if !sort.StringsAreSorted(keys) || len(keys) != len(slices.Compact(append([]string(nil), keys...))) {
		return errors.New("confirmed world facts must be sorted and unique")
	}
	return nil
}

func validateVisualDesignGaps(values []VisualDesignGap) error {
	keys := make([]string, len(values))
	for index, value := range values {
		keys[index] = value.GapKey
		if !visualFoundationGapPattern.MatchString(value.GapKey) || !validVisualDesignDomain(value.DesignDomain) ||
			!hashPattern.MatchString(value.SourceConstraintHash) || !validVisualSceneScopes(value.AffectedScopeKeys) {
			return errors.New("invalid Visual Design Gap")
		}
	}
	if !sort.StringsAreSorted(keys) || len(keys) != len(slices.Compact(append([]string(nil), keys...))) {
		return errors.New("Visual Design Gaps must be sorted and unique")
	}
	return nil
}

func validateVisualFoundationPolicy(value VisualFoundationPolicy) error {
	for _, rules := range [][]string{value.PaletteRules, value.MaterialRules, value.LightingRules, value.CameraRules} {
		if len(rules) == 0 || !sortedUnique(rules) {
			return errors.New("Visual Foundation policy rules must be non-empty, sorted, and unique")
		}
		for _, rule := range rules {
			if !validVisualText(rule, 800) {
				return errors.New("invalid Visual Foundation policy rule")
			}
		}
	}
	if value.ForbiddenChanges == nil || !reflect.DeepEqual(value.ForbiddenChanges, visualFoundationFidelityInvariants) {
		return errors.New("Visual Foundation policy cannot relax fidelity invariants")
	}
	return nil
}

func validateWorldAdaptationProposals(values []WorldAdaptationProposal) error {
	keys := make([]string, len(values))
	for index, value := range values {
		keys[index] = value.MappingKey
		if !visualFoundationMappingPattern.MatchString(value.MappingKey) || !visualFoundationFactPattern.MatchString(value.SourceFactKey) ||
			!visualFoundationRuleKeyPattern.MatchString(value.PresetRuleKey) || !validVisualText(value.DesignValue, 1200) ||
			!reflect.DeepEqual(value.PreservedInvariantKeys, visualFoundationFidelityInvariants) ||
			!validVisualSceneScopes(value.AffectedScopeKeys) || value.DecisionStatus != "needs_creator_decision" {
			return errors.New("invalid world adaptation proposal")
		}
	}
	if !sort.StringsAreSorted(keys) || len(keys) != len(slices.Compact(append([]string(nil), keys...))) {
		return errors.New("world adaptations must be sorted and unique")
	}
	return nil
}

func validateVisualWorldConflicts(values []VisualWorldConflict) error {
	keys := make([]string, len(values))
	for index, value := range values {
		keys[index] = value.ConflictKey
		if !visualFoundationConflictPattern.MatchString(value.ConflictKey) || !visualFoundationFactPattern.MatchString(value.SourceFactKey) ||
			!visualFoundationRuleKeyPattern.MatchString(value.PresetRuleKey) ||
			!slices.Contains([]string{"blocking", "warning"}, value.Severity) || !validVisualText(value.Summary, 800) ||
			!validVisualSceneScopes(value.AffectedScopeKeys) || value.ResolutionStatus != "needs_creator_decision" {
			return errors.New("invalid Visual World Conflict")
		}
	}
	if !sort.StringsAreSorted(keys) || len(keys) != len(slices.Compact(append([]string(nil), keys...))) {
		return errors.New("Visual World Conflicts must be sorted and unique")
	}
	return nil
}

func validateCreativeFillProposals(values []CreativeFillProposal) error {
	keys := make([]string, len(values))
	for index, value := range values {
		keys[index] = value.GapKey
		if !visualFoundationGapPattern.MatchString(value.GapKey) || !validVisualDesignDomain(value.DesignDomain) ||
			!validVisualText(value.Proposal, 1200) || !validVisualSceneScopes(value.AffectedScopeKeys) ||
			value.DecisionStatus != "needs_creator_decision" {
			return errors.New("invalid creative fill proposal")
		}
	}
	if !sort.StringsAreSorted(keys) || len(keys) != len(slices.Compact(append([]string(nil), keys...))) {
		return errors.New("creative fill proposals must be sorted and unique")
	}
	return nil
}

func visualFoundationCollectionHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func validVisualDesignDomain(value string) bool {
	return slices.Contains([]string{"architecture", "civilization", "material", "prop", "technology_or_magic", "wardrobe"}, value)
}

func validVisualFoundationMode(value string) bool {
	return value == "faithful" || value == "world_adaptation"
}

func validVisualFoundationScope(value, kind string) bool {
	prefix, identifier, found := strings.Cut(value, ":")
	parsed, err := uuid.Parse(identifier)
	return found && prefix == kind && err == nil && parsed != uuid.Nil
}

func validVisualSceneScopes(values []string) bool {
	if !sortedUnique(values) {
		return false
	}
	for _, value := range values {
		if !validVisualFoundationScope(value, "scene") {
			return false
		}
	}
	return true
}

func validVisualObjectKey(value string, parts []string) bool {
	if value == "" || len(value) > 512 || strings.HasPrefix(value, "/") || strings.Contains(value, `\`) {
		return false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func validVisualText(value string, maxRunes int) bool {
	return value != "" && value == strings.TrimSpace(value) && utf8.RuneCountInString(value) <= maxRunes
}
