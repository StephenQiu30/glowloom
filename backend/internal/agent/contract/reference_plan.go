package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const (
	ReferencePlanSchemaSetContractID = "storygraph-reference-plan-schema-set-production"
	ReferencePlanInputContractID     = "reference-plan-input-production"
	ReferencePlanCandidateContractID = "reference-plan-candidate-production"
	ReferencePlanInputSchemaHash     = "cb02abe281b1fdcd08bf01ae2ed4dd681c9a09d07676c391c5b7f1f45883c8cd"
	ReferencePlanCandidateSchemaHash = "5aabb9ddcc340f43dc712cba37c6120a3500fc5946a9b6df28ab2c69ec03770b"
)

var referencePlanTargetKinds = []string{
	"character_appearance",
	"character_identity_anchor",
	"interaction_composition",
	"location_board",
	"prop_sheet",
	"scene_composition",
}

type ReferencePlanSchema struct {
	ContractID string `json:"contract_id"`
	SchemaHash string `json:"schema_hash"`
}

type ReferencePlanSchemaManifest struct {
	ContractID    string                `json:"contract_id"`
	Schemas       []ReferencePlanSchema `json:"schemas"`
	SchemaSetHash string                `json:"schema_set_hash"`
}

var referencePlanSchemas = []ReferencePlanSchema{
	{ContractID: ReferencePlanCandidateContractID, SchemaHash: ReferencePlanCandidateSchemaHash},
	{ContractID: ReferencePlanInputContractID, SchemaHash: ReferencePlanInputSchemaHash},
}

type ReferencePlanOwnerRef struct {
	WorkspaceID         string  `json:"workspace_id"`
	ProjectID           string  `json:"project_id"`
	OwnerKind           string  `json:"owner_kind"`
	VersionFamily       string  `json:"version_family"`
	OwnerLogicalID      string  `json:"owner_logical_id"`
	OwnerVersionID      string  `json:"owner_version_id"`
	OwnerRevision       int64   `json:"owner_revision"`
	OwnerContentHash    string  `json:"owner_content_hash"`
	FragmentKey         *string `json:"fragment_key"`
	FragmentContentHash *string `json:"fragment_content_hash"`
}

type ReferencePlanCharacterStateSeed struct {
	StateRef              ReferencePlanOwnerRef   `json:"state_ref"`
	AppearanceBusinessKey string                  `json:"appearance_business_key"`
	CoverageScopeKeys     []string                `json:"coverage_scope_keys"`
	OccurrenceRefs        []ReferencePlanOwnerRef `json:"occurrence_refs"`
}

type ReferencePlanCharacterSeed struct {
	AnchorBusinessKey string                            `json:"anchor_business_key"`
	IdentityRef       ReferencePlanOwnerRef             `json:"identity_ref"`
	SpecificationRef  ReferencePlanOwnerRef             `json:"specification_ref"`
	CoverageScopeKeys []string                          `json:"coverage_scope_keys"`
	StateOptions      []ReferencePlanCharacterStateSeed `json:"state_options"`
}

type ReferencePlanTargetOwnerRefs struct {
	Identity      []ReferencePlanOwnerRef `json:"identity"`
	Specification []ReferencePlanOwnerRef `json:"specification"`
	State         []ReferencePlanOwnerRef `json:"state"`
	Scene         []ReferencePlanOwnerRef `json:"scene"`
	Occurrence    []ReferencePlanOwnerRef `json:"occurrence"`
	Interaction   []ReferencePlanOwnerRef `json:"interaction"`
}

type ReferencePlanCharacterDependencySeed struct {
	AnchorBusinessKey     string                `json:"anchor_business_key"`
	StateRef              ReferencePlanOwnerRef `json:"state_ref"`
	AppearanceBusinessKey string                `json:"appearance_business_key"`
}

type ReferencePlanFixedTargetSeed struct {
	TargetBusinessKey           string                                 `json:"target_business_key"`
	TargetKind                  string                                 `json:"target_kind"`
	OwnerRefs                   ReferencePlanTargetOwnerRefs           `json:"owner_refs"`
	CoverageScopeKeys           []string                               `json:"coverage_scope_keys"`
	FixedDependencyBusinessKeys []string                               `json:"fixed_dependency_business_keys"`
	CharacterDependencies       []ReferencePlanCharacterDependencySeed `json:"character_dependencies"`
}

type ReferencePlanPurposeProfile struct {
	TargetKind       string   `json:"target_kind"`
	DesignFocus      []string `json:"design_focus"`
	ForbiddenChanges []string `json:"forbidden_changes"`
}

type ReferencePlanInput struct {
	WorkspaceID                           string                         `json:"workspace_id"`
	ProjectID                             string                         `json:"project_id"`
	ProductionWorldOwnerSetHash           string                         `json:"production_world_owner_set_hash"`
	P1ScopeKeys                           []string                       `json:"p1_scope_keys"`
	VisualFoundationCandidateRevisionID   string                         `json:"visual_foundation_candidate_revision_id"`
	VisualFoundationCandidateRevisionHash string                         `json:"visual_foundation_candidate_revision_hash"`
	VisualFoundationCandidate             VisualFoundationCandidate      `json:"visual_foundation_candidate"`
	CharacterSeeds                        []ReferencePlanCharacterSeed   `json:"character_seeds"`
	FixedTargetSeeds                      []ReferencePlanFixedTargetSeed `json:"fixed_target_seeds"`
	PurposeProfiles                       []ReferencePlanPurposeProfile  `json:"purpose_profiles"`
	ReferenceTargetSeedRoot               string                         `json:"reference_target_seed_root"`
}

type ReferencePlanAnchorSelection struct {
	AnchorBusinessKey string                `json:"anchor_business_key"`
	SelectedStateRef  ReferencePlanOwnerRef `json:"selected_state_ref"`
}

type ReferencePlanTargetSpecification struct {
	TargetBusinessKey           string   `json:"target_business_key"`
	TargetKind                  string   `json:"target_kind"`
	Fulfillment                 string   `json:"fulfillment"`
	DesignFocus                 []string `json:"design_focus"`
	ForbiddenChanges            []string `json:"forbidden_changes"`
	DependsOnTargetBusinessKeys []string `json:"depends_on_target_business_keys"`
}

type ReferencePlanCandidate struct {
	WorkspaceID                           string                             `json:"workspace_id"`
	ProjectID                             string                             `json:"project_id"`
	ProductionWorldOwnerSetHash           string                             `json:"production_world_owner_set_hash"`
	P1ScopeKeys                           []string                           `json:"p1_scope_keys"`
	VisualFoundationCandidateRevisionID   string                             `json:"visual_foundation_candidate_revision_id"`
	VisualFoundationCandidateRevisionHash string                             `json:"visual_foundation_candidate_revision_hash"`
	ReferenceTargetSeedRoot               string                             `json:"reference_target_seed_root"`
	AnchorSelections                      []ReferencePlanAnchorSelection     `json:"anchor_selections"`
	TargetSpecifications                  []ReferencePlanTargetSpecification `json:"target_specifications"`
}

func DecodeReferencePlanInput(raw json.RawMessage) (ReferencePlanInput, json.RawMessage, error) {
	var value ReferencePlanInput
	if decodeStrict(raw, &value) != nil || value.Validate() != nil {
		return ReferencePlanInput{}, nil, errors.New("invalid Reference Plan input")
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return ReferencePlanInput{}, nil, err
	}
	return value, json.RawMessage(canonical), nil
}

func DecodeReferencePlanSchemaManifest(raw json.RawMessage) (ReferencePlanSchemaManifest, json.RawMessage, error) {
	var value ReferencePlanSchemaManifest
	if decodeStrict(raw, &value) != nil || value.ContractID != ReferencePlanSchemaSetContractID ||
		!reflect.DeepEqual(value.Schemas, referencePlanSchemas) || !hashPattern.MatchString(value.SchemaSetHash) {
		return ReferencePlanSchemaManifest{}, nil, errors.New("invalid Reference Plan Schema manifest")
	}
	rawMaterial, err := json.Marshal(struct {
		ContractID string                `json:"contract_id"`
		Schemas    []ReferencePlanSchema `json:"schemas"`
	}{ContractID: value.ContractID, Schemas: value.Schemas})
	if err != nil {
		return ReferencePlanSchemaManifest{}, nil, err
	}
	hash, err := platformcanonical.Hash(rawMaterial)
	if err != nil || hash != value.SchemaSetHash {
		return ReferencePlanSchemaManifest{}, nil, errors.New("Reference Plan Schema set hash has drifted")
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return ReferencePlanSchemaManifest{}, nil, err
	}
	return value, json.RawMessage(canonical), nil
}

func DecodeReferencePlanCandidate(raw json.RawMessage) (ReferencePlanCandidate, json.RawMessage, error) {
	var value ReferencePlanCandidate
	if decodeStrict(raw, &value) != nil || value.validateShape() != nil {
		return ReferencePlanCandidate{}, nil, errors.New("invalid Reference Plan Candidate")
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return ReferencePlanCandidate{}, nil, err
	}
	return value, json.RawMessage(canonical), nil
}

func (value ReferencePlanInput) ComputeSeedRoot() (string, error) {
	raw, err := json.Marshal(struct {
		ContractID       string                         `json:"contract_id"`
		CharacterSeeds   []ReferencePlanCharacterSeed   `json:"character_seeds"`
		FixedTargetSeeds []ReferencePlanFixedTargetSeed `json:"fixed_target_seeds"`
	}{
		ContractID:     "reference-target-seed-inventory-production",
		CharacterSeeds: value.CharacterSeeds, FixedTargetSeeds: value.FixedTargetSeeds,
	})
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func (value ReferencePlanInput) Validate() error {
	if !referencePlanUUID(value.WorkspaceID) || !referencePlanUUID(value.ProjectID) ||
		!referencePlanUUID(value.VisualFoundationCandidateRevisionID) ||
		!hashPattern.MatchString(value.ProductionWorldOwnerSetHash) ||
		!hashPattern.MatchString(value.VisualFoundationCandidateRevisionHash) ||
		!hashPattern.MatchString(value.ReferenceTargetSeedRoot) ||
		validateReferencePlanScopes(value.P1ScopeKeys) != nil ||
		value.CharacterSeeds == nil || len(value.FixedTargetSeeds) == 0 || value.PurposeProfiles == nil ||
		value.VisualFoundationCandidate.validateShape() != nil ||
		value.VisualFoundationCandidate.WorkspaceID != value.WorkspaceID ||
		value.VisualFoundationCandidate.ProjectID != value.ProjectID ||
		value.VisualFoundationCandidate.ProductionWorldOwnerSetHash != value.ProductionWorldOwnerSetHash ||
		value.VisualFoundationCandidate.ApplicationMode == "faithful" && len(value.VisualFoundationCandidate.WorldAdaptations) != 0 ||
		validateReferencePlanPurposeProfiles(value.PurposeProfiles) != nil {
		return errors.New("invalid Reference Plan input")
	}
	anchors, appearances, err := validateReferencePlanCharacterSeeds(value)
	if err != nil {
		return err
	}
	fixed, sceneScopes, err := validateReferencePlanFixedSeeds(value, anchors, appearances)
	if err != nil {
		return err
	}
	if !slices.Equal(sceneScopes, value.P1ScopeKeys) {
		return errors.New("invalid Reference Plan fixed target seed inventory")
	}
	for key := range anchors {
		if _, exists := fixed[key]; exists {
			return errors.New("duplicate Reference Plan target seed key")
		}
	}
	for key := range appearances {
		if _, exists := fixed[key]; exists {
			return errors.New("duplicate Reference Plan target seed key")
		}
	}
	root, rootErr := value.ComputeSeedRoot()
	if rootErr != nil || root != value.ReferenceTargetSeedRoot {
		return errors.New("Reference Target seed root has drifted")
	}
	return nil
}

func (value ReferencePlanCandidate) validateShape() error {
	if !referencePlanUUID(value.WorkspaceID) || !referencePlanUUID(value.ProjectID) ||
		!referencePlanUUID(value.VisualFoundationCandidateRevisionID) ||
		!hashPattern.MatchString(value.ProductionWorldOwnerSetHash) ||
		!hashPattern.MatchString(value.VisualFoundationCandidateRevisionHash) ||
		!hashPattern.MatchString(value.ReferenceTargetSeedRoot) ||
		validateReferencePlanScopes(value.P1ScopeKeys) != nil || value.AnchorSelections == nil || len(value.TargetSpecifications) == 0 {
		return errors.New("invalid Reference Plan Candidate")
	}
	previous := ""
	for index, selection := range value.AnchorSelections {
		if referencePlanBusinessKeyKind(selection.AnchorBusinessKey) != "character_identity_anchor" ||
			validateReferencePlanOwnerRef(selection.SelectedStateRef, value.WorkspaceID, value.ProjectID) != nil ||
			index > 0 && previous >= selection.AnchorBusinessKey {
			return errors.New("invalid Reference Plan Anchor selection")
		}
		previous = selection.AnchorBusinessKey
	}
	previous = ""
	for index, specification := range value.TargetSpecifications {
		if referencePlanBusinessKeyKind(specification.TargetBusinessKey) != specification.TargetKind ||
			!slices.Contains(referencePlanTargetKinds, specification.TargetKind) ||
			!slices.Contains([]string{"not_generated", "optional", "required"}, specification.Fulfillment) ||
			!referencePlanSortedStrings(specification.DesignFocus, true) ||
			!referencePlanSortedStrings(specification.ForbiddenChanges, true) ||
			!referencePlanSortedStrings(specification.DependsOnTargetBusinessKeys, false) ||
			index > 0 && previous >= specification.TargetBusinessKey {
			return errors.New("invalid Reference Plan Target specification")
		}
		previous = specification.TargetBusinessKey
	}
	return nil
}

func (value ReferencePlanCandidate) ValidateFor(input ReferencePlanInput) error {
	if input.Validate() != nil || value.validateShape() != nil || value.WorkspaceID != input.WorkspaceID ||
		value.ProjectID != input.ProjectID || value.ProductionWorldOwnerSetHash != input.ProductionWorldOwnerSetHash ||
		!slices.Equal(value.P1ScopeKeys, input.P1ScopeKeys) ||
		value.VisualFoundationCandidateRevisionID != input.VisualFoundationCandidateRevisionID ||
		value.VisualFoundationCandidateRevisionHash != input.VisualFoundationCandidateRevisionHash ||
		value.ReferenceTargetSeedRoot != input.ReferenceTargetSeedRoot {
		return errors.New("Reference Plan Candidate input lineage drifted")
	}
	selectedStates, expected, err := referencePlanExpectedTargets(input, value.AnchorSelections)
	if err != nil || len(value.TargetSpecifications) != len(expected) {
		return errors.New("Reference Plan Candidate target set differs from frozen seeds")
	}
	profiles := make(map[string]ReferencePlanPurposeProfile, len(input.PurposeProfiles))
	for _, profile := range input.PurposeProfiles {
		profiles[profile.TargetKind] = profile
	}
	specifications := make(map[string]ReferencePlanTargetSpecification, len(value.TargetSpecifications))
	for _, specification := range value.TargetSpecifications {
		kind, exists := expected[specification.TargetBusinessKey]
		profile := profiles[specification.TargetKind]
		if !exists || kind != specification.TargetKind ||
			!referencePlanSubset(specification.DesignFocus, profile.DesignFocus) ||
			!slices.Equal(specification.ForbiddenChanges, profile.ForbiddenChanges) {
			return errors.New("Reference Plan Candidate target specification escapes its frozen seed or PurposeProfile")
		}
		specifications[specification.TargetBusinessKey] = specification
	}
	for _, specification := range value.TargetSpecifications {
		expectedDependencies, dependencyErr := referencePlanExpectedDependencies(input, selectedStates, specification)
		if dependencyErr != nil || !slices.Equal(specification.DependsOnTargetBusinessKeys, expectedDependencies) {
			return errors.New("Reference Plan Candidate dependency set differs from frozen seeds")
		}
		for _, dependencyKey := range expectedDependencies {
			dependency, exists := specifications[dependencyKey]
			if !exists || referencePlanFulfillmentRank(dependency.Fulfillment) < referencePlanFulfillmentRank(specification.Fulfillment) {
				return errors.New("Reference Plan Candidate dependency fulfillment rank is invalid")
			}
		}
	}
	return nil
}

func validateReferencePlanCharacterSeeds(value ReferencePlanInput) (map[string]ReferencePlanCharacterSeed, map[string]ReferencePlanCharacterStateSeed, error) {
	anchors := make(map[string]ReferencePlanCharacterSeed, len(value.CharacterSeeds))
	appearances := make(map[string]ReferencePlanCharacterStateSeed)
	previous := ""
	for index, seed := range value.CharacterSeeds {
		if referencePlanBusinessKeyKind(seed.AnchorBusinessKey) != "character_identity_anchor" ||
			index > 0 && previous >= seed.AnchorBusinessKey || validateReferencePlanScopes(seed.CoverageScopeKeys) != nil ||
			!referencePlanSubset(seed.CoverageScopeKeys, value.P1ScopeKeys) || len(seed.StateOptions) == 0 ||
			validateReferencePlanOwnerRef(seed.IdentityRef, value.WorkspaceID, value.ProjectID) != nil ||
			validateReferencePlanOwnerRef(seed.SpecificationRef, value.WorkspaceID, value.ProjectID) != nil {
			return nil, nil, errors.New("invalid Reference Plan Character seed")
		}
		previous = seed.AnchorBusinessKey
		statePrevious := ""
		coverage := make(map[string]struct{})
		occurrences := make(map[string]struct{})
		for stateIndex, option := range seed.StateOptions {
			stateKey := option.StateRef.sortKey()
			if validateReferencePlanOwnerRef(option.StateRef, value.WorkspaceID, value.ProjectID) != nil ||
				stateIndex > 0 && statePrevious >= stateKey ||
				referencePlanBusinessKeyKind(option.AppearanceBusinessKey) != "character_appearance" ||
				validateReferencePlanScopes(option.CoverageScopeKeys) != nil ||
				!referencePlanSubset(option.CoverageScopeKeys, seed.CoverageScopeKeys) || len(option.OccurrenceRefs) == 0 ||
				validateReferencePlanRefs(option.OccurrenceRefs, value.WorkspaceID, value.ProjectID) != nil {
				return nil, nil, errors.New("invalid Reference Plan Character State seed")
			}
			statePrevious = stateKey
			for _, scope := range option.CoverageScopeKeys {
				coverage[scope] = struct{}{}
			}
			for _, occurrence := range option.OccurrenceRefs {
				key := occurrence.sortKey()
				if _, duplicate := occurrences[key]; duplicate {
					return nil, nil, errors.New("duplicate Reference Plan Character occurrence seed")
				}
				occurrences[key] = struct{}{}
			}
			if _, duplicate := appearances[option.AppearanceBusinessKey]; duplicate {
				return nil, nil, errors.New("duplicate Reference Plan Appearance seed")
			}
			appearances[option.AppearanceBusinessKey] = option
		}
		if !slices.Equal(referencePlanSetKeys(coverage), seed.CoverageScopeKeys) {
			return nil, nil, errors.New("Reference Plan Character seed coverage is incomplete")
		}
		anchors[seed.AnchorBusinessKey] = seed
	}
	return anchors, appearances, nil
}

func validateReferencePlanFixedSeeds(value ReferencePlanInput, anchors map[string]ReferencePlanCharacterSeed, appearances map[string]ReferencePlanCharacterStateSeed) (map[string]ReferencePlanFixedTargetSeed, []string, error) {
	fixed := make(map[string]ReferencePlanFixedTargetSeed, len(value.FixedTargetSeeds))
	previous := ""
	sceneScopes := make(map[string]struct{})
	for index, seed := range value.FixedTargetSeeds {
		if !slices.Contains([]string{"interaction_composition", "location_board", "prop_sheet", "scene_composition"}, seed.TargetKind) ||
			referencePlanBusinessKeyKind(seed.TargetBusinessKey) != seed.TargetKind || index > 0 && previous >= seed.TargetBusinessKey ||
			validateReferencePlanScopes(seed.CoverageScopeKeys) != nil || !referencePlanSubset(seed.CoverageScopeKeys, value.P1ScopeKeys) ||
			validateReferencePlanTargetOwnerRefs(seed.OwnerRefs, value.WorkspaceID, value.ProjectID) != nil ||
			!referencePlanSortedStrings(seed.FixedDependencyBusinessKeys, false) || seed.CharacterDependencies == nil {
			return nil, nil, errors.New("invalid Reference Plan fixed target seed")
		}
		previous = seed.TargetBusinessKey
		if !slices.Equal(seed.CoverageScopeKeys, referencePlanSceneScopes(seed.OwnerRefs.Scene)) {
			return nil, nil, errors.New("Reference Plan fixed target coverage differs from Scene refs")
		}
		switch seed.TargetKind {
		case "location_board", "prop_sheet":
			if len(seed.OwnerRefs.Identity) != 1 || len(seed.OwnerRefs.Specification) != 1 || len(seed.OwnerRefs.State) != 1 ||
				len(seed.OwnerRefs.Occurrence) == 0 || len(seed.FixedDependencyBusinessKeys) != 0 || len(seed.CharacterDependencies) != 0 {
				return nil, nil, errors.New("invalid Reference Plan base target seed")
			}
		case "scene_composition":
			if len(seed.OwnerRefs.Scene) != 1 || len(seed.OwnerRefs.Identity) == 0 || len(seed.OwnerRefs.Specification) == 0 ||
				len(seed.OwnerRefs.State) == 0 || len(seed.OwnerRefs.Occurrence) == 0 {
				return nil, nil, errors.New("invalid Reference Plan Scene seed")
			}
			sceneScopes[seed.CoverageScopeKeys[0]] = struct{}{}
		case "interaction_composition":
			if len(seed.OwnerRefs.Scene) != 1 || len(seed.OwnerRefs.Interaction) != 1 || len(seed.OwnerRefs.Occurrence) < 2 {
				return nil, nil, errors.New("invalid Reference Plan Interaction seed")
			}
		}
		if err := validateReferencePlanCharacterDependencies(seed.CharacterDependencies, anchors, appearances); err != nil {
			return nil, nil, err
		}
		fixed[seed.TargetBusinessKey] = seed
	}
	for _, seed := range value.FixedTargetSeeds {
		for _, dependency := range seed.FixedDependencyBusinessKeys {
			base, exists := fixed[dependency]
			if !exists || !slices.Contains([]string{"location_board", "prop_sheet"}, base.TargetKind) {
				return nil, nil, errors.New("Reference Plan fixed dependency does not name a base target seed")
			}
		}
	}
	return fixed, referencePlanSetKeys(sceneScopes), nil
}

func validateReferencePlanCharacterDependencies(values []ReferencePlanCharacterDependencySeed, anchors map[string]ReferencePlanCharacterSeed, appearances map[string]ReferencePlanCharacterStateSeed) error {
	previous := ""
	for index, value := range values {
		seed, exists := anchors[value.AnchorBusinessKey]
		appearance, appearanceExists := appearances[value.AppearanceBusinessKey]
		key := value.AnchorBusinessKey + "\x00" + value.StateRef.sortKey()
		if !exists || !appearanceExists || !reflect.DeepEqual(appearance.StateRef, value.StateRef) ||
			index > 0 && previous >= key || !slices.ContainsFunc(seed.StateOptions, func(option ReferencePlanCharacterStateSeed) bool {
			return option.AppearanceBusinessKey == value.AppearanceBusinessKey && reflect.DeepEqual(option.StateRef, value.StateRef)
		}) {
			return errors.New("invalid Reference Plan Character dependency seed")
		}
		previous = key
	}
	return nil
}

func validateReferencePlanTargetOwnerRefs(value ReferencePlanTargetOwnerRefs, workspaceID, projectID string) error {
	for _, refs := range [][]ReferencePlanOwnerRef{value.Identity, value.Specification, value.State, value.Scene, value.Occurrence, value.Interaction} {
		if refs == nil || validateReferencePlanRefs(refs, workspaceID, projectID) != nil {
			return errors.New("invalid Reference Plan Target Owner refs")
		}
	}
	return nil
}

func validateReferencePlanPurposeProfiles(values []ReferencePlanPurposeProfile) error {
	if len(values) != len(referencePlanTargetKinds) {
		return errors.New("Reference Plan PurposeProfiles are incomplete")
	}
	for index, profile := range values {
		if profile.TargetKind != referencePlanTargetKinds[index] || !referencePlanSortedStrings(profile.DesignFocus, true) ||
			!referencePlanSortedStrings(profile.ForbiddenChanges, true) {
			return errors.New("invalid Reference Plan PurposeProfile")
		}
	}
	return nil
}

func referencePlanExpectedTargets(input ReferencePlanInput, selections []ReferencePlanAnchorSelection) (map[string]ReferencePlanOwnerRef, map[string]string, error) {
	if len(selections) != len(input.CharacterSeeds) {
		return nil, nil, errors.New("Reference Plan Anchor selections are incomplete")
	}
	selected := make(map[string]ReferencePlanOwnerRef, len(selections))
	expected := make(map[string]string)
	for index, seed := range input.CharacterSeeds {
		selection := selections[index]
		if selection.AnchorBusinessKey != seed.AnchorBusinessKey {
			return nil, nil, errors.New("Reference Plan Anchor selections differ from frozen seeds")
		}
		matched := false
		for _, option := range seed.StateOptions {
			if reflect.DeepEqual(option.StateRef, selection.SelectedStateRef) {
				matched = true
				continue
			}
			expected[option.AppearanceBusinessKey] = "character_appearance"
		}
		if !matched {
			return nil, nil, errors.New("Reference Plan Anchor selected a non-occurring State")
		}
		selected[seed.AnchorBusinessKey] = selection.SelectedStateRef
		expected[seed.AnchorBusinessKey] = "character_identity_anchor"
	}
	for _, seed := range input.FixedTargetSeeds {
		expected[seed.TargetBusinessKey] = seed.TargetKind
	}
	return selected, expected, nil
}

func referencePlanExpectedDependencies(input ReferencePlanInput, selected map[string]ReferencePlanOwnerRef, specification ReferencePlanTargetSpecification) ([]string, error) {
	for _, seed := range input.CharacterSeeds {
		for _, option := range seed.StateOptions {
			if option.AppearanceBusinessKey == specification.TargetBusinessKey {
				return []string{seed.AnchorBusinessKey}, nil
			}
		}
	}
	for _, seed := range input.FixedTargetSeeds {
		if seed.TargetBusinessKey != specification.TargetBusinessKey {
			continue
		}
		if !slices.Contains([]string{"scene_composition", "interaction_composition"}, seed.TargetKind) || specification.Fulfillment == "not_generated" {
			return []string{}, nil
		}
		dependencies := append([]string(nil), seed.FixedDependencyBusinessKeys...)
		for _, dependency := range seed.CharacterDependencies {
			selectedState, exists := selected[dependency.AnchorBusinessKey]
			if !exists {
				return nil, errors.New("Reference Plan Character dependency lacks an Anchor selection")
			}
			key := dependency.AppearanceBusinessKey
			if reflect.DeepEqual(selectedState, dependency.StateRef) {
				key = dependency.AnchorBusinessKey
			}
			dependencies = append(dependencies, key)
		}
		sort.Strings(dependencies)
		dependencies = slices.Compact(dependencies)
		return dependencies, nil
	}
	return []string{}, nil
}

func validateReferencePlanOwnerRef(value ReferencePlanOwnerRef, workspaceID, projectID string) error {
	if !referencePlanUUID(value.WorkspaceID) || !referencePlanUUID(value.ProjectID) || !referencePlanUUID(value.OwnerVersionID) ||
		value.WorkspaceID != workspaceID || value.ProjectID != projectID || !referencePlanStableText(value.OwnerKind) ||
		!referencePlanStableText(value.VersionFamily) || !referencePlanStableText(value.OwnerLogicalID) ||
		value.OwnerRevision < 1 || value.OwnerRevision > 9007199254740991 || !hashPattern.MatchString(value.OwnerContentHash) ||
		(value.FragmentKey == nil) != (value.FragmentContentHash == nil) {
		return errors.New("invalid Reference Plan Owner ref")
	}
	if value.FragmentKey != nil && (!referencePlanStableText(*value.FragmentKey) || !hashPattern.MatchString(*value.FragmentContentHash)) {
		return errors.New("invalid Reference Plan Owner ref")
	}
	return nil
}

func validateReferencePlanRefs(values []ReferencePlanOwnerRef, workspaceID, projectID string) error {
	previous := ""
	for index, value := range values {
		if validateReferencePlanOwnerRef(value, workspaceID, projectID) != nil || index > 0 && previous >= value.sortKey() {
			return errors.New("Reference Plan Owner refs are not sorted and unique")
		}
		previous = value.sortKey()
	}
	return nil
}

func (value ReferencePlanOwnerRef) sortKey() string {
	fragment := ""
	if value.FragmentKey != nil {
		fragment = *value.FragmentKey
	}
	return strings.Join([]string{value.OwnerKind, value.VersionFamily, value.OwnerLogicalID, fragment, value.OwnerVersionID}, "\x00")
}

func referencePlanBusinessKeyKind(value string) string {
	canonical, err := platformcanonical.JSON([]byte(value))
	if err != nil || !bytes.Equal(canonical, []byte(value)) {
		return ""
	}
	var parts []json.RawMessage
	if json.Unmarshal(canonical, &parts) != nil || len(parts) < 2 {
		return ""
	}
	var kind string
	if json.Unmarshal(parts[0], &kind) != nil || !slices.Contains(referencePlanTargetKinds, kind) {
		return ""
	}
	expectedRefs := map[string]int{
		"character_appearance": 3, "character_identity_anchor": 1,
		"interaction_composition": 1, "location_board": 3, "prop_sheet": 3, "scene_composition": 1,
	}[kind]
	if len(parts) != expectedRefs+1 {
		return ""
	}
	for _, raw := range parts[1:] {
		var ref []string
		if json.Unmarshal(raw, &ref) != nil || len(ref) != 4 || !referencePlanStableText(ref[0]) ||
			!referencePlanStableText(ref[1]) || !referencePlanStableText(ref[2]) ||
			ref[3] != "" && !referencePlanStableText(ref[3]) {
			return ""
		}
	}
	return kind
}

func validateReferencePlanScopes(values []string) error {
	if !referencePlanSortedStrings(values, true) {
		return errors.New("Reference Plan scopes must be non-empty, sorted, and unique")
	}
	for _, value := range values {
		if !strings.HasPrefix(value, "scene:") || !referencePlanStableText(value) {
			return errors.New("Reference Plan scope must name a Scene")
		}
	}
	return nil
}

func referencePlanSceneScopes(refs []ReferencePlanOwnerRef) []string {
	values := make([]string, len(refs))
	for index, ref := range refs {
		values[index] = ref.OwnerLogicalID
	}
	sort.Strings(values)
	return slices.Compact(values)
}

func referencePlanSortedStrings(values []string, nonempty bool) bool {
	if values == nil || nonempty && len(values) == 0 {
		return false
	}
	for index, value := range values {
		if !referencePlanStableText(value) || index > 0 && values[index-1] >= value {
			return false
		}
	}
	return true
}

func referencePlanSubset(subset, superset []string) bool {
	allowed := make(map[string]struct{}, len(superset))
	for _, value := range superset {
		allowed[value] = struct{}{}
	}
	for _, value := range subset {
		if _, exists := allowed[value]; !exists {
			return false
		}
	}
	return true
}

func referencePlanSetKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func referencePlanFulfillmentRank(value string) int {
	return slices.Index([]string{"not_generated", "optional", "required"}, value)
}

func referencePlanUUID(value string) bool {
	identifier, err := uuid.Parse(value)
	return err == nil && identifier != uuid.Nil && identifier.String() == value
}

func referencePlanStableText(value string) bool {
	if strings.TrimSpace(value) == "" || !norm.NFC.IsNormalString(value) {
		return false
	}
	for _, character := range value {
		if character == unicode.MaxASCII || character < ' ' {
			return false
		}
	}
	return true
}
