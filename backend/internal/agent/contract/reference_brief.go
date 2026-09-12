package contract

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const (
	ReferenceBriefCandidateContractID = "reference-brief-candidate-production"
	ReferenceBriefCandidateSchemaHash = "d100072d39f4a5958d0126c8aa6342d67ef368457be5efb85caeb18e777e172f"
	ReferenceBriefInputContractID     = "reference-brief-input-production"
	ReferenceBriefInputSchemaHash     = "b4ce81ae76e11a273190e231ce82f44762e9c76e620869dc21618b44c03816db"
)

var referenceBriefIdentityInvariantSlots = []string{
	"body_shape",
	"facial_structure",
	"hair",
	"permanent_marks",
	"proportions",
}

var referenceBriefViewRoles = map[string][]string{
	"character_appearance":      {"back", "front", "profile"},
	"character_identity_anchor": {"back", "front", "profile"},
	"interaction_composition":   {"interaction_master"},
	"location_board":            {"empty_establishing", "material_scale_detail", "spatial_orientation"},
	"prop_sheet":                {"back", "front", "side", "state_detail"},
	"scene_composition":         {"composition_master"},
}

func ReferenceBriefRequiredViewRoles(targetKind string) []string {
	return append([]string(nil), referenceBriefViewRoles[targetKind]...)
}

type ReferenceBriefStageRelease struct {
	StageKey         string `json:"stage_key"`
	StageReleaseHash string `json:"stage_release_hash"`
}

type ReferenceBriefSourceDesignSlot struct {
	SlotKey           string `json:"slot_key"`
	SourceRequirement string `json:"source_requirement"`
	DesignRequirement string `json:"design_requirement"`
}

type ReferenceBriefQCRubricRef struct {
	ContractID  string `json:"contract_id"`
	ContentHash string `json:"content_hash"`
}

type ReferenceBriefDependencySelection struct {
	TargetVersionRef        ReferencePlanOwnerRef `json:"target_version_ref"`
	SelectedAssetVersionRef ReferencePlanOwnerRef `json:"selected_asset_version_ref"`
}

type ReferenceBriefInput struct {
	WorkspaceID                     string                              `json:"workspace_id"`
	ProjectID                       string                              `json:"project_id"`
	ApprovedReferencePlanVersionRef ReferencePlanOwnerRef               `json:"approved_reference_plan_version_ref"`
	ReferencePlanTargetRef          ReferencePlanOwnerRef               `json:"reference_plan_target_ref"`
	TargetBusinessKey               string                              `json:"target_business_key"`
	TargetKind                      string                              `json:"target_kind"`
	TargetFulfillment               string                              `json:"target_fulfillment"`
	VisualFoundationVersionRef      ReferencePlanOwnerRef               `json:"visual_foundation_version_ref"`
	EffectiveStyleSnapshotRef       ReferencePlanOwnerRef               `json:"effective_style_snapshot_ref"`
	EffectivePolicySnapshotRef      ReferencePlanOwnerRef               `json:"effective_policy_snapshot_ref"`
	DependencySelections            []ReferenceBriefDependencySelection `json:"dependency_selections"`
	StageRelease                    ReferenceBriefStageRelease          `json:"stage_release"`
	TypedReadSetRoot                string                              `json:"typed_read_set_root"`
	SourceRefs                      ReferencePlanTargetOwnerRefs        `json:"source_refs"`
	DesignFocus                     []string                            `json:"design_focus"`
	ForbiddenChanges                []string                            `json:"forbidden_changes"`
	RequiredViewRoles               []string                            `json:"required_view_roles"`
}

type ReferenceBriefCandidate struct {
	WorkspaceID                     string                              `json:"workspace_id"`
	ProjectID                       string                              `json:"project_id"`
	ApprovedReferencePlanVersionRef ReferencePlanOwnerRef               `json:"approved_reference_plan_version_ref"`
	ReferencePlanTargetRef          ReferencePlanOwnerRef               `json:"reference_plan_target_ref"`
	TargetBusinessKey               string                              `json:"target_business_key"`
	TargetKind                      string                              `json:"target_kind"`
	VisualFoundationVersionRef      ReferencePlanOwnerRef               `json:"visual_foundation_version_ref"`
	EffectiveStyleSnapshotRef       ReferencePlanOwnerRef               `json:"effective_style_snapshot_ref"`
	EffectivePolicySnapshotRef      ReferencePlanOwnerRef               `json:"effective_policy_snapshot_ref"`
	DependencySelections            []ReferenceBriefDependencySelection `json:"dependency_selections"`
	StageRelease                    ReferenceBriefStageRelease          `json:"stage_release"`
	TypedReadSetRoot                string                              `json:"typed_read_set_root"`
	SourceDesignSlots               []ReferenceBriefSourceDesignSlot    `json:"source_design_slots"`
	PositiveInstructions            []string                            `json:"positive_instructions"`
	NegativeInstructions            []string                            `json:"negative_instructions"`
	RequiredViewRoles               []string                            `json:"required_view_roles"`
	LayoutRequirements              []string                            `json:"layout_requirements"`
	ScaleRequirements               []string                            `json:"scale_requirements"`
	RightsRequirements              []string                            `json:"rights_requirements"`
	ProvenanceRequirements          []string                            `json:"provenance_requirements"`
	QCRubricRefs                    []ReferenceBriefQCRubricRef         `json:"qc_rubric_refs"`
	SourceRefs                      ReferencePlanTargetOwnerRefs        `json:"source_refs"`
	Brief                           json.RawMessage                     `json:"brief"`
}

type CharacterIdentityAnchorBrief struct {
	TargetKind             string   `json:"target_kind"`
	IdentityInvariantSlots []string `json:"identity_invariant_slots"`
}

type CharacterAppearanceBrief struct {
	TargetKind             string   `json:"target_kind"`
	IdentityInvariantSlots []string `json:"identity_invariant_slots"`
	ApprovedVariableSlots  []string `json:"approved_variable_slots"`
}

type LocationBoardBrief struct {
	TargetKind          string   `json:"target_kind"`
	TopologyConstraints []string `json:"topology_constraints"`
	ScaleAnchors        []string `json:"scale_anchors"`
	MaterialSlots       []string `json:"material_slots"`
	OccupancyPolicy     string   `json:"occupancy_policy"`
}

type PropSheetBrief struct {
	TargetKind              string   `json:"target_kind"`
	PhysicalDimensions      string   `json:"physical_dimensions"`
	StructuralSlots         []string `json:"structural_slots"`
	StateSlots              []string `json:"state_slots"`
	ContentOrMechanismSlots []string `json:"content_or_mechanism_slots"`
	OccupancyPolicy         string   `json:"occupancy_policy"`
}

type SceneCompositionBrief struct {
	TargetKind         string   `json:"target_kind"`
	CompositionPurpose string   `json:"composition_purpose"`
	SpatialConstraints []string `json:"spatial_constraints"`
}

type InteractionCompositionBrief struct {
	TargetKind               string   `json:"target_kind"`
	HandSide                 string   `json:"hand_side"`
	GripOrContactPoint       string   `json:"grip_or_contact_point"`
	Orientation              string   `json:"orientation"`
	BodyPropScaleConstraints []string `json:"body_prop_scale_constraints"`
	TransferOrUseState       string   `json:"transfer_or_use_state"`
}

func DecodeReferenceBriefCandidate(raw json.RawMessage) (ReferenceBriefCandidate, json.RawMessage, error) {
	var value ReferenceBriefCandidate
	if decodeStrict(raw, &value) != nil || value.validate() != nil {
		return ReferenceBriefCandidate{}, nil, errors.New("invalid Reference Brief Candidate")
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return ReferenceBriefCandidate{}, nil, err
	}
	value.Brief, err = platformcanonical.JSON(value.Brief)
	if err != nil {
		return ReferenceBriefCandidate{}, nil, err
	}
	return value, json.RawMessage(canonical), nil
}

func DecodeReferenceBriefInput(raw json.RawMessage) (ReferenceBriefInput, json.RawMessage, error) {
	var value ReferenceBriefInput
	if decodeStrict(raw, &value) != nil || value.Validate() != nil {
		return ReferenceBriefInput{}, nil, errors.New("invalid Reference Brief input")
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return ReferenceBriefInput{}, nil, err
	}
	return value, json.RawMessage(canonical), nil
}

func (value ReferenceBriefInput) Validate() error {
	dependencyProbe := ReferenceBriefCandidate{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, TargetKind: value.TargetKind,
		DependencySelections: value.DependencySelections,
	}
	if !referencePlanUUID(value.WorkspaceID) || !referencePlanUUID(value.ProjectID) ||
		validateReferencePlanOwnerRef(value.ApprovedReferencePlanVersionRef, value.WorkspaceID, value.ProjectID) != nil ||
		validateReferencePlanOwnerRef(value.ReferencePlanTargetRef, value.WorkspaceID, value.ProjectID) != nil ||
		validateReferencePlanOwnerRef(value.VisualFoundationVersionRef, value.WorkspaceID, value.ProjectID) != nil ||
		validateReferencePlanOwnerRef(value.EffectiveStyleSnapshotRef, value.WorkspaceID, value.ProjectID) != nil ||
		validateReferencePlanOwnerRef(value.EffectivePolicySnapshotRef, value.WorkspaceID, value.ProjectID) != nil ||
		!validReferenceBriefOwnerFamilies(
			value.ApprovedReferencePlanVersionRef, value.ReferencePlanTargetRef, value.VisualFoundationVersionRef,
			value.EffectiveStyleSnapshotRef, value.EffectivePolicySnapshotRef,
		) || referencePlanBusinessKeyKind(value.TargetBusinessKey) != value.TargetKind ||
		value.ReferencePlanTargetRef.OwnerLogicalID != value.TargetBusinessKey ||
		!slices.Contains([]string{"optional", "required"}, value.TargetFulfillment) ||
		value.StageRelease.StageKey != "compile_reference_brief" || !hashPattern.MatchString(value.StageRelease.StageReleaseHash) ||
		!hashPattern.MatchString(value.TypedReadSetRoot) || validateReferenceBriefDependencies(dependencyProbe) != nil ||
		validateReferencePlanTargetOwnerRefs(value.SourceRefs, value.WorkspaceID, value.ProjectID) != nil ||
		validateReferenceBriefSourceRefs(value.TargetKind, value.SourceRefs) != nil ||
		!referencePlanSortedStrings(value.DesignFocus, true) || !referencePlanSortedStrings(value.ForbiddenChanges, true) ||
		!reflect.DeepEqual(value.RequiredViewRoles, referenceBriefViewRoles[value.TargetKind]) {
		return errors.New("invalid Reference Brief input")
	}
	return nil
}

func (value ReferenceBriefCandidate) ValidateFor(input ReferenceBriefInput) error {
	if input.Validate() != nil || value.validate() != nil || value.WorkspaceID != input.WorkspaceID ||
		value.ProjectID != input.ProjectID ||
		!reflect.DeepEqual(value.ApprovedReferencePlanVersionRef, input.ApprovedReferencePlanVersionRef) ||
		!reflect.DeepEqual(value.ReferencePlanTargetRef, input.ReferencePlanTargetRef) ||
		value.TargetBusinessKey != input.TargetBusinessKey || value.TargetKind != input.TargetKind ||
		!reflect.DeepEqual(value.VisualFoundationVersionRef, input.VisualFoundationVersionRef) ||
		!reflect.DeepEqual(value.EffectiveStyleSnapshotRef, input.EffectiveStyleSnapshotRef) ||
		!reflect.DeepEqual(value.EffectivePolicySnapshotRef, input.EffectivePolicySnapshotRef) ||
		!reflect.DeepEqual(value.DependencySelections, input.DependencySelections) ||
		!reflect.DeepEqual(value.StageRelease, input.StageRelease) || value.TypedReadSetRoot != input.TypedReadSetRoot ||
		!reflect.DeepEqual(value.SourceRefs, input.SourceRefs) ||
		!reflect.DeepEqual(value.PositiveInstructions, input.DesignFocus) ||
		!reflect.DeepEqual(value.NegativeInstructions, input.ForbiddenChanges) ||
		!reflect.DeepEqual(value.RequiredViewRoles, input.RequiredViewRoles) {
		return errors.New("Reference Brief Candidate input fence has drifted")
	}
	return nil
}

func (value ReferenceBriefCandidate) validate() error {
	if !referencePlanUUID(value.WorkspaceID) || !referencePlanUUID(value.ProjectID) ||
		validateReferencePlanOwnerRef(value.ApprovedReferencePlanVersionRef, value.WorkspaceID, value.ProjectID) != nil ||
		validateReferencePlanOwnerRef(value.ReferencePlanTargetRef, value.WorkspaceID, value.ProjectID) != nil ||
		validateReferencePlanOwnerRef(value.VisualFoundationVersionRef, value.WorkspaceID, value.ProjectID) != nil ||
		validateReferencePlanOwnerRef(value.EffectiveStyleSnapshotRef, value.WorkspaceID, value.ProjectID) != nil ||
		validateReferencePlanOwnerRef(value.EffectivePolicySnapshotRef, value.WorkspaceID, value.ProjectID) != nil ||
		!validReferenceBriefOwnerFamilies(
			value.ApprovedReferencePlanVersionRef, value.ReferencePlanTargetRef, value.VisualFoundationVersionRef,
			value.EffectiveStyleSnapshotRef, value.EffectivePolicySnapshotRef,
		) || referencePlanBusinessKeyKind(value.TargetBusinessKey) != value.TargetKind ||
		value.ReferencePlanTargetRef.OwnerLogicalID != value.TargetBusinessKey ||
		value.StageRelease.StageKey != "compile_reference_brief" || !hashPattern.MatchString(value.StageRelease.StageReleaseHash) ||
		!hashPattern.MatchString(value.TypedReadSetRoot) || validateReferenceBriefDependencies(value) != nil ||
		validateReferenceBriefSourceDesignSlots(value.SourceDesignSlots) != nil ||
		!referencePlanSortedStrings(value.PositiveInstructions, true) ||
		!referencePlanSortedStrings(value.NegativeInstructions, true) ||
		!reflect.DeepEqual(value.RequiredViewRoles, referenceBriefViewRoles[value.TargetKind]) ||
		!referencePlanSortedStrings(value.LayoutRequirements, true) ||
		!referencePlanSortedStrings(value.ScaleRequirements, true) ||
		!referencePlanSortedStrings(value.RightsRequirements, true) ||
		!referencePlanSortedStrings(value.ProvenanceRequirements, true) ||
		validateReferenceBriefRubrics(value.QCRubricRefs) != nil ||
		validateReferencePlanTargetOwnerRefs(value.SourceRefs, value.WorkspaceID, value.ProjectID) != nil ||
		validateReferenceBriefSourceRefs(value.TargetKind, value.SourceRefs) != nil ||
		validateReferenceBriefPurpose(value.TargetKind, value.Brief) != nil {
		return errors.New("invalid Reference Brief Candidate")
	}
	return nil
}

func validReferenceBriefOwnerFamilies(
	planRef ReferencePlanOwnerRef,
	targetRef ReferencePlanOwnerRef,
	visualFoundationRef ReferencePlanOwnerRef,
	styleRef ReferencePlanOwnerRef,
	policyRef ReferencePlanOwnerRef,
) bool {
	for _, ref := range []ReferencePlanOwnerRef{planRef, targetRef} {
		if ref.OwnerKind != "production/reference" || ref.VersionFamily != "reference_plan_set" || ref.FragmentKey != nil {
			return false
		}
	}
	for _, ref := range []ReferencePlanOwnerRef{visualFoundationRef, styleRef, policyRef} {
		if ref.OwnerKind != "preset" || ref.VersionFamily != "preset_effective_set" || ref.FragmentKey != nil {
			return false
		}
	}
	return true
}

func validateReferenceBriefDependencies(value ReferenceBriefCandidate) error {
	selections := value.DependencySelections
	if selections == nil {
		return errors.New("Reference Brief dependency selection is missing")
	}
	switch value.TargetKind {
	case "character_identity_anchor", "location_board", "prop_sheet":
		if len(selections) != 0 {
			return errors.New("Reference Brief base Target has dependencies")
		}
	case "character_appearance":
		if len(selections) != 1 || referencePlanBusinessKeyKind(selections[0].TargetVersionRef.OwnerLogicalID) != "character_identity_anchor" {
			return errors.New("Reference Brief Appearance lacks its exact Identity Anchor")
		}
	case "interaction_composition", "scene_composition":
		if len(selections) == 0 {
			return errors.New("Reference Brief Composition lacks selected base assets")
		}
	default:
		return errors.New("unknown Reference Brief purpose")
	}
	previous := ""
	for index, selection := range selections {
		dependencyKind := referencePlanBusinessKeyKind(selection.TargetVersionRef.OwnerLogicalID)
		if validateReferencePlanOwnerRef(selection.TargetVersionRef, value.WorkspaceID, value.ProjectID) != nil ||
			selection.TargetVersionRef.OwnerKind != "production/reference" ||
			selection.TargetVersionRef.VersionFamily != "reference_plan_set" || selection.TargetVersionRef.FragmentKey != nil ||
			!slices.Contains([]string{"character_appearance", "character_identity_anchor", "location_board", "prop_sheet"}, dependencyKind) ||
			validateReferencePlanOwnerRef(selection.SelectedAssetVersionRef, value.WorkspaceID, value.ProjectID) != nil ||
			selection.SelectedAssetVersionRef.OwnerKind != "asset" ||
			selection.SelectedAssetVersionRef.VersionFamily != "asset_base_reference_set" ||
			selection.SelectedAssetVersionRef.FragmentKey != nil ||
			index > 0 && previous >= selection.TargetVersionRef.OwnerLogicalID {
			return errors.New("invalid Reference Brief dependency selection")
		}
		previous = selection.TargetVersionRef.OwnerLogicalID
	}
	return nil
}

func validateReferenceBriefSourceRefs(targetKind string, refs ReferencePlanTargetOwnerRefs) error {
	if len(refs.Scene) == 0 || len(refs.Occurrence) == 0 || len(refs.Identity) == 0 ||
		len(refs.Specification) == 0 || len(refs.State) == 0 {
		return errors.New("Reference Brief source closure is incomplete")
	}
	switch targetKind {
	case "character_appearance", "character_identity_anchor", "location_board", "prop_sheet":
		if len(refs.Identity) != 1 || len(refs.Specification) != 1 || len(refs.State) != 1 || len(refs.Interaction) != 0 {
			return errors.New("Reference Brief base source closure is invalid")
		}
	case "interaction_composition":
		if len(refs.Scene) != 1 || len(refs.Interaction) != 1 {
			return errors.New("Reference Brief Interaction source closure is invalid")
		}
	case "scene_composition":
		if len(refs.Scene) != 1 {
			return errors.New("Reference Brief Scene source closure is invalid")
		}
	default:
		return errors.New("unknown Reference Brief source purpose")
	}
	return nil
}

func validateReferenceBriefSourceDesignSlots(values []ReferenceBriefSourceDesignSlot) error {
	if len(values) == 0 {
		return errors.New("Reference Brief source/design slots are missing")
	}
	previous := ""
	for index, value := range values {
		if !referencePlanStableText(value.SlotKey) || !referencePlanStableText(value.SourceRequirement) ||
			!referencePlanStableText(value.DesignRequirement) || index > 0 && previous >= value.SlotKey {
			return errors.New("invalid Reference Brief source/design slot")
		}
		previous = value.SlotKey
	}
	return nil
}

func validateReferenceBriefRubrics(values []ReferenceBriefQCRubricRef) error {
	if len(values) == 0 {
		return errors.New("Reference Brief QC rubric refs are missing")
	}
	previous := ""
	for index, value := range values {
		key := value.ContractID + "\x00" + value.ContentHash
		if !referencePlanStableText(value.ContractID) || !hashPattern.MatchString(value.ContentHash) || index > 0 && previous >= key {
			return errors.New("invalid Reference Brief QC rubric ref")
		}
		previous = key
	}
	return nil
}

func validateReferenceBriefPurpose(targetKind string, raw json.RawMessage) error {
	switch targetKind {
	case "character_identity_anchor":
		var value CharacterIdentityAnchorBrief
		if decodeStrict(raw, &value) != nil || value.TargetKind != targetKind ||
			!reflect.DeepEqual(value.IdentityInvariantSlots, referenceBriefIdentityInvariantSlots) {
			return errors.New("invalid Character Identity Anchor Brief")
		}
	case "character_appearance":
		var value CharacterAppearanceBrief
		if decodeStrict(raw, &value) != nil || value.TargetKind != targetKind ||
			!reflect.DeepEqual(value.IdentityInvariantSlots, referenceBriefIdentityInvariantSlots) ||
			!referencePlanSortedStrings(value.ApprovedVariableSlots, true) {
			return errors.New("invalid Character Appearance Brief")
		}
	case "location_board":
		var value LocationBoardBrief
		if decodeStrict(raw, &value) != nil || value.TargetKind != targetKind || value.OccupancyPolicy != "empty" ||
			!referencePlanSortedStrings(value.TopologyConstraints, true) || !referencePlanSortedStrings(value.ScaleAnchors, true) ||
			!referencePlanSortedStrings(value.MaterialSlots, true) {
			return errors.New("invalid Location Board Brief")
		}
	case "prop_sheet":
		var value PropSheetBrief
		if decodeStrict(raw, &value) != nil || value.TargetKind != targetKind || value.OccupancyPolicy != "no_hands_no_people" ||
			!referencePlanStableText(value.PhysicalDimensions) || !referencePlanSortedStrings(value.StructuralSlots, true) ||
			!referencePlanSortedStrings(value.StateSlots, true) || !referencePlanSortedStrings(value.ContentOrMechanismSlots, true) {
			return errors.New("invalid Prop Sheet Brief")
		}
	case "scene_composition":
		var value SceneCompositionBrief
		if decodeStrict(raw, &value) != nil || value.TargetKind != targetKind || !referencePlanStableText(value.CompositionPurpose) ||
			!referencePlanSortedStrings(value.SpatialConstraints, true) {
			return errors.New("invalid Scene Composition Brief")
		}
	case "interaction_composition":
		var value InteractionCompositionBrief
		if decodeStrict(raw, &value) != nil || value.TargetKind != targetKind ||
			!slices.Contains([]string{"both", "left", "right"}, value.HandSide) ||
			!referencePlanStableText(value.GripOrContactPoint) || !referencePlanStableText(value.Orientation) ||
			!referencePlanSortedStrings(value.BodyPropScaleConstraints, true) || !referencePlanStableText(value.TransferOrUseState) {
			return errors.New("invalid Interaction Composition Brief")
		}
	default:
		return errors.New("unknown Reference Brief purpose")
	}
	return nil
}
