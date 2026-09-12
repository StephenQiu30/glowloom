package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	presetdomain "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
	storygraphdomain "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

const VisualFoundationScopeSubjectSchemaVersion = "visual-foundation-scope-human-gate-subject-production"

type VisualFoundationScopeCandidateRevisionMaterial struct {
	RevisionID   string
	Revision     int64
	RevisionHash string
	ContentHash  string
	Candidate    json.RawMessage
}

type VisualFoundationScopeCandidateRevisionRef struct {
	StageKey      string `json:"stage_key"`
	CandidateType string `json:"candidate_type"`
	RevisionID    string `json:"revision_id"`
	Revision      int64  `json:"revision"`
	RevisionHash  string `json:"revision_hash"`
	ContentHash   string `json:"content_hash"`
}

type VisualFoundationScopeProductionWorldRef struct {
	WorkspaceID           string   `json:"workspace_id"`
	ProjectID             string   `json:"project_id"`
	StoryGraphVersionID   string   `json:"storygraph_version_id"`
	StoryGraphContentHash string   `json:"storygraph_content_hash"`
	OwnerSetHash          string   `json:"owner_set_hash"`
	P1ScopeKeys           []string `json:"p1_scope_keys"`
	ReadSetRoot           string   `json:"read_set_root"`
}

type VisualFoundationScopeProjectPresetSelectionRef struct {
	SelectionID     string `json:"selection_id"`
	Revision        int64  `json:"revision"`
	ContentHash     string `json:"content_hash"`
	ApplicationMode string `json:"application_mode"`
}

type VisualFoundationScopeSubject struct {
	SchemaVersion                string                                         `json:"schema_version"`
	ConfirmedProductionWorld     VisualFoundationScopeProductionWorldRef        `json:"confirmed_production_world"`
	PresetRelease                presetdomain.ProjectSelectionRelease           `json:"preset_release"`
	ProjectPresetSelection       VisualFoundationScopeProjectPresetSelectionRef `json:"project_preset_selection"`
	PresetCapabilityManifestRoot string                                         `json:"preset_capability_manifest_root"`
	VisualFoundationCandidate    VisualFoundationScopeCandidateRevisionRef      `json:"visual_foundation_candidate"`
	ReferencePlanCandidate       VisualFoundationScopeCandidateRevisionRef      `json:"reference_plan_candidate"`
	ReferenceTargetSeedRoot      string                                         `json:"reference_target_seed_root"`
	ExpectedReferenceTargetSet   storygraphdomain.ExpectedReferenceTargetSet    `json:"expected_reference_target_set"`
	ReadSetRoot                  string                                         `json:"read_set_root"`
}

type VisualFoundationScopeSubjectDraft struct {
	ConfirmedProductionWorld   storygraphdomain.ReferencePlanWorldReadSet
	ProjectPresetSelection     presetdomain.ProjectSelection
	PresetRelease              presetdomain.Release
	VisualFoundationCandidate  VisualFoundationScopeCandidateRevisionMaterial
	ReferencePlanInput         agentcontract.ReferencePlanInput
	ReferencePlanCandidate     VisualFoundationScopeCandidateRevisionMaterial
	ExpectedReferenceTargetSet storygraphdomain.ExpectedReferenceTargetSet
}

func NewVisualFoundationScopeSubject(
	draft VisualFoundationScopeSubjectDraft,
) (VisualFoundationScopeSubject, json.RawMessage, error) {
	selection, err := validateVisualFoundationScopeSelection(draft.ProjectPresetSelection)
	if err != nil {
		return VisualFoundationScopeSubject{}, nil, err
	}
	presetCapabilityManifestRoot, err := validateVisualFoundationScopeRelease(selection, draft.PresetRelease)
	if err != nil {
		return VisualFoundationScopeSubject{}, nil, err
	}
	visualCandidate, visualRef, err := visualFoundationScopeVisualCandidate(draft.VisualFoundationCandidate)
	if err != nil {
		return VisualFoundationScopeSubject{}, nil, err
	}
	referenceCandidate, referenceRef, err := visualFoundationScopeReferenceCandidate(
		draft.ReferencePlanCandidate,
		draft.ReferencePlanInput,
	)
	if err != nil {
		return VisualFoundationScopeSubject{}, nil, err
	}
	if err = validateVisualFoundationScopeLineage(
		draft,
		selection,
		visualCandidate,
		referenceCandidate,
	); err != nil {
		return VisualFoundationScopeSubject{}, nil, err
	}
	worldRef, err := visualFoundationScopeProductionWorldRef(draft.ConfirmedProductionWorld)
	if err != nil {
		return VisualFoundationScopeSubject{}, nil, err
	}
	value := VisualFoundationScopeSubject{
		SchemaVersion:            VisualFoundationScopeSubjectSchemaVersion,
		ConfirmedProductionWorld: worldRef,
		PresetRelease:            selection.PresetRelease,
		ProjectPresetSelection: VisualFoundationScopeProjectPresetSelectionRef{
			SelectionID: selection.ID, Revision: selection.Revision, ContentHash: selection.ContentHash,
			ApplicationMode: selection.ApplicationMode,
		},
		PresetCapabilityManifestRoot: presetCapabilityManifestRoot,
		VisualFoundationCandidate:    visualRef,
		ReferencePlanCandidate:       referenceRef,
		ReferenceTargetSeedRoot:      draft.ReferencePlanInput.ReferenceTargetSeedRoot,
		ExpectedReferenceTargetSet:   cloneExpectedReferenceTargetSet(draft.ExpectedReferenceTargetSet),
	}
	value.ReadSetRoot, err = visualFoundationScopeHash(visualFoundationScopeReadSetMaterial(value))
	if err != nil {
		return VisualFoundationScopeSubject{}, nil, err
	}
	return encodeVisualFoundationScopeSubject(value)
}

func DecodeVisualFoundationScopeSubject(
	raw json.RawMessage,
) (VisualFoundationScopeSubject, json.RawMessage, error) {
	var value VisualFoundationScopeSubject
	if err := decodeVisualFoundationScopeStrict(raw, &value); err != nil {
		return VisualFoundationScopeSubject{}, nil, errors.New("invalid Gate 3 Visual Foundation Scope Subject")
	}
	if err := validateVisualFoundationScopeSubject(value); err != nil {
		return VisualFoundationScopeSubject{}, nil, err
	}
	return encodeVisualFoundationScopeSubject(value)
}

func validateVisualFoundationScopeLineage(
	draft VisualFoundationScopeSubjectDraft,
	selection presetdomain.ProjectSelection,
	visual agentcontract.VisualFoundationCandidate,
	reference agentcontract.ReferencePlanCandidate,
) error {
	world := draft.ConfirmedProductionWorld
	input := draft.ReferencePlanInput
	if input.Validate() != nil || validateVisualFoundationScopeInventoryInput(world.Inventory, input) != nil ||
		world.WorkspaceID != selection.WorkspaceID || world.ProjectID != selection.ProjectID ||
		world.WorkspaceID != input.WorkspaceID || world.ProjectID != input.ProjectID ||
		world.Inventory.OwnerSetHash != input.ProductionWorldOwnerSetHash ||
		!reflect.DeepEqual(world.Inventory.P1ScopeKeys, input.P1ScopeKeys) ||
		visual.WorkspaceID != world.WorkspaceID || visual.ProjectID != world.ProjectID ||
		visual.ProductionWorldOwnerSetHash != world.Inventory.OwnerSetHash ||
		visual.PresetReleaseContentHash != selection.PresetRelease.ContentHash ||
		visual.ApplicationMode != selection.ApplicationMode ||
		!reflect.DeepEqual(visual, input.VisualFoundationCandidate) ||
		input.VisualFoundationCandidateRevisionID != draft.VisualFoundationCandidate.RevisionID ||
		input.VisualFoundationCandidateRevisionHash != draft.VisualFoundationCandidate.RevisionHash ||
		reference.WorkspaceID != world.WorkspaceID || reference.ProjectID != world.ProjectID ||
		reference.ProductionWorldOwnerSetHash != world.Inventory.OwnerSetHash ||
		reference.VisualFoundationCandidateRevisionID != draft.VisualFoundationCandidate.RevisionID ||
		reference.VisualFoundationCandidateRevisionHash != draft.VisualFoundationCandidate.RevisionHash ||
		reference.ReferenceTargetSeedRoot != input.ReferenceTargetSeedRoot {
		return errors.New("Gate 3 Visual Foundation Scope lineage has drifted")
	}
	keys := make([]string, len(reference.TargetSpecifications))
	for index, target := range reference.TargetSpecifications {
		keys[index] = target.TargetBusinessKey
	}
	expected, err := storygraphdomain.BuildExpectedReferenceTargetSetProof(
		input.ProductionWorldOwnerSetHash,
		input.P1ScopeKeys,
		keys,
	)
	if err != nil || !reflect.DeepEqual(expected, draft.ExpectedReferenceTargetSet) {
		return errors.New("Gate 3 expected Reference Target set has drifted")
	}
	return nil
}

func validateVisualFoundationScopeInventoryInput(
	inventory storygraphdomain.ReferencePlanSeedInventory,
	input agentcontract.ReferencePlanInput,
) error {
	raw, err := json.Marshal(inventory)
	if err != nil {
		return err
	}
	var normalized struct {
		OwnerSetHash     string                                       `json:"owner_set_hash"`
		P1ScopeKeys      []string                                     `json:"p1_scope_keys"`
		CharacterSeeds   []agentcontract.ReferencePlanCharacterSeed   `json:"character_seeds"`
		FixedTargetSeeds []agentcontract.ReferencePlanFixedTargetSeed `json:"fixed_target_seeds"`
	}
	if err = decodeVisualFoundationScopeStrict(raw, &normalized); err != nil ||
		normalized.OwnerSetHash != input.ProductionWorldOwnerSetHash ||
		!reflect.DeepEqual(normalized.P1ScopeKeys, input.P1ScopeKeys) ||
		!reflect.DeepEqual(normalized.CharacterSeeds, input.CharacterSeeds) ||
		!reflect.DeepEqual(normalized.FixedTargetSeeds, input.FixedTargetSeeds) {
		return errors.New("Gate 3 Reference Plan seed inventory has drifted")
	}
	return nil
}

func validateVisualFoundationScopeSelection(
	value presetdomain.ProjectSelection,
) (presetdomain.ProjectSelection, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return presetdomain.ProjectSelection{}, err
	}
	decoded, _, err := presetdomain.DecodeProjectSelection(raw)
	if err != nil || !reflect.DeepEqual(decoded, value) {
		return presetdomain.ProjectSelection{}, errors.New("invalid Gate 3 Project Preset Selection")
	}
	return decoded, nil
}

func validateVisualFoundationScopeRelease(
	selection presetdomain.ProjectSelection,
	release presetdomain.Release,
) (string, error) {
	return validateVisualFoundationScopeReleaseRef(selection.PresetRelease, release)
}

func validateVisualFoundationScopeReleaseRef(
	releaseRef presetdomain.ProjectSelectionRelease,
	release presetdomain.Release,
) (string, error) {
	raw, err := json.Marshal(release)
	if err != nil {
		return "", err
	}
	decoded, _, err := presetdomain.DecodeRelease(raw)
	if err != nil || !reflect.DeepEqual(decoded, release) ||
		releaseRef.Key != release.Key || releaseRef.Release != release.Release ||
		releaseRef.ContentHash != release.ContentHash {
		return "", errors.New("invalid Gate 3 Preset release")
	}
	return visualFoundationScopeHash(release.CapabilityManifest)
}

func visualFoundationScopeVisualCandidate(
	material VisualFoundationScopeCandidateRevisionMaterial,
) (agentcontract.VisualFoundationCandidate, VisualFoundationScopeCandidateRevisionRef, error) {
	if err := validateVisualFoundationScopeCandidateMaterial(material); err != nil {
		return agentcontract.VisualFoundationCandidate{}, VisualFoundationScopeCandidateRevisionRef{}, err
	}
	candidate, canonical, err := agentcontract.DecodeVisualFoundationCandidate(material.Candidate)
	contentHash, hashErr := visualFoundationScopeCanonicalHash(canonical)
	if err != nil || hashErr != nil || contentHash != material.ContentHash {
		return agentcontract.VisualFoundationCandidate{}, VisualFoundationScopeCandidateRevisionRef{},
			errors.New("Gate 3 Visual Foundation Candidate has drifted")
	}
	return candidate, visualFoundationScopeCandidateRef(
		agentcontract.VisualFoundationStageKey,
		"visual_foundation_candidate",
		material,
	), nil
}

func visualFoundationScopeReferenceCandidate(
	material VisualFoundationScopeCandidateRevisionMaterial,
	input agentcontract.ReferencePlanInput,
) (agentcontract.ReferencePlanCandidate, VisualFoundationScopeCandidateRevisionRef, error) {
	if err := validateVisualFoundationScopeCandidateMaterial(material); err != nil {
		return agentcontract.ReferencePlanCandidate{}, VisualFoundationScopeCandidateRevisionRef{}, err
	}
	candidate, canonical, err := agentcontract.DecodeReferencePlanCandidate(material.Candidate)
	contentHash, hashErr := visualFoundationScopeCanonicalHash(canonical)
	if err != nil || candidate.ValidateFor(input) != nil || hashErr != nil || contentHash != material.ContentHash {
		return agentcontract.ReferencePlanCandidate{}, VisualFoundationScopeCandidateRevisionRef{},
			errors.New("Gate 3 Reference Plan Candidate has drifted")
	}
	return candidate, visualFoundationScopeCandidateRef(
		agentcontract.ReferencePlanStageKey,
		"reference_plan_candidate",
		material,
	), nil
}

func validateVisualFoundationScopeCandidateMaterial(
	value VisualFoundationScopeCandidateRevisionMaterial,
) error {
	if _, err := uuid.Parse(value.RevisionID); err != nil || value.Revision < 1 ||
		!nodeOutputContentHashPattern.MatchString(value.RevisionHash) ||
		!nodeOutputContentHashPattern.MatchString(value.ContentHash) || len(value.Candidate) == 0 {
		return errors.New("invalid Gate 3 Candidate revision")
	}
	return nil
}

func visualFoundationScopeCandidateRef(
	stageKey string,
	candidateType string,
	material VisualFoundationScopeCandidateRevisionMaterial,
) VisualFoundationScopeCandidateRevisionRef {
	return VisualFoundationScopeCandidateRevisionRef{
		StageKey: stageKey, CandidateType: candidateType,
		RevisionID: material.RevisionID, Revision: material.Revision,
		RevisionHash: material.RevisionHash, ContentHash: material.ContentHash,
	}
}

func visualFoundationScopeProductionWorldRef(
	value storygraphdomain.ReferencePlanWorldReadSet,
) (VisualFoundationScopeProductionWorldRef, error) {
	if _, err := uuid.Parse(value.WorkspaceID); err != nil {
		return VisualFoundationScopeProductionWorldRef{}, errors.New("invalid Gate 3 Production World")
	}
	if _, err := uuid.Parse(value.ProjectID); err != nil {
		return VisualFoundationScopeProductionWorldRef{}, errors.New("invalid Gate 3 Production World")
	}
	if _, err := uuid.Parse(value.StoryGraphVersionID); err != nil ||
		!nodeOutputContentHashPattern.MatchString(value.StoryGraphContentHash) ||
		!nodeOutputContentHashPattern.MatchString(value.Inventory.OwnerSetHash) {
		return VisualFoundationScopeProductionWorldRef{}, errors.New("invalid Gate 3 Production World")
	}
	readSetRoot, err := visualFoundationScopeHash(value)
	if err != nil {
		return VisualFoundationScopeProductionWorldRef{}, err
	}
	return VisualFoundationScopeProductionWorldRef{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID,
		StoryGraphVersionID: value.StoryGraphVersionID, StoryGraphContentHash: value.StoryGraphContentHash,
		OwnerSetHash: value.Inventory.OwnerSetHash,
		P1ScopeKeys:  append([]string(nil), value.Inventory.P1ScopeKeys...), ReadSetRoot: readSetRoot,
	}, nil
}

func validateVisualFoundationScopeSubject(value VisualFoundationScopeSubject) error {
	if value.SchemaVersion != VisualFoundationScopeSubjectSchemaVersion ||
		!nodeOutputContentHashPattern.MatchString(value.ReadSetRoot) ||
		!nodeOutputContentHashPattern.MatchString(value.ConfirmedProductionWorld.ReadSetRoot) ||
		!nodeOutputContentHashPattern.MatchString(value.ConfirmedProductionWorld.StoryGraphContentHash) ||
		!nodeOutputContentHashPattern.MatchString(value.ConfirmedProductionWorld.OwnerSetHash) ||
		!nodeOutputContentHashPattern.MatchString(value.PresetRelease.ContentHash) ||
		!nodeOutputContentHashPattern.MatchString(value.ProjectPresetSelection.ContentHash) ||
		!nodeOutputContentHashPattern.MatchString(value.PresetCapabilityManifestRoot) ||
		!nodeOutputContentHashPattern.MatchString(value.ReferenceTargetSeedRoot) ||
		value.VisualFoundationCandidate.StageKey != agentcontract.VisualFoundationStageKey ||
		value.VisualFoundationCandidate.CandidateType != "visual_foundation_candidate" ||
		value.ReferencePlanCandidate.StageKey != agentcontract.ReferencePlanStageKey ||
		value.ReferencePlanCandidate.CandidateType != "reference_plan_candidate" ||
		validateVisualFoundationScopeCandidateRef(value.VisualFoundationCandidate) != nil ||
		validateVisualFoundationScopeCandidateRef(value.ReferencePlanCandidate) != nil ||
		value.ExpectedReferenceTargetSet.OwnerSetHash != value.ConfirmedProductionWorld.OwnerSetHash ||
		!reflect.DeepEqual(value.ExpectedReferenceTargetSet.P1ScopeKeys, value.ConfirmedProductionWorld.P1ScopeKeys) {
		return errors.New("invalid Gate 3 Visual Foundation Scope Subject")
	}
	for _, identifier := range []string{
		value.ConfirmedProductionWorld.WorkspaceID,
		value.ConfirmedProductionWorld.ProjectID,
		value.ConfirmedProductionWorld.StoryGraphVersionID,
		value.ProjectPresetSelection.SelectionID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Gate 3 Visual Foundation Scope Subject")
		}
	}
	expected, err := storygraphdomain.BuildExpectedReferenceTargetSetProof(
		value.ExpectedReferenceTargetSet.OwnerSetHash,
		value.ExpectedReferenceTargetSet.P1ScopeKeys,
		value.ExpectedReferenceTargetSet.ExpectedTargetBusinessKeys,
	)
	if err != nil || !reflect.DeepEqual(expected, value.ExpectedReferenceTargetSet) {
		return errors.New("invalid Gate 3 expected Reference Target set")
	}
	readSetRoot, err := visualFoundationScopeHash(visualFoundationScopeReadSetMaterial(value))
	if err != nil || readSetRoot != value.ReadSetRoot {
		return errors.New("Gate 3 Visual Foundation Scope read set has drifted")
	}
	return nil
}

func validateVisualFoundationScopeCandidateRef(value VisualFoundationScopeCandidateRevisionRef) error {
	if _, err := uuid.Parse(value.RevisionID); err != nil || value.Revision < 1 ||
		!nodeOutputContentHashPattern.MatchString(value.RevisionHash) ||
		!nodeOutputContentHashPattern.MatchString(value.ContentHash) {
		return errors.New("invalid Gate 3 Candidate revision reference")
	}
	return nil
}

func visualFoundationScopeReadSetMaterial(value VisualFoundationScopeSubject) any {
	return struct {
		ConfirmedProductionWorld     VisualFoundationScopeProductionWorldRef        `json:"confirmed_production_world"`
		PresetRelease                presetdomain.ProjectSelectionRelease           `json:"preset_release"`
		ProjectPresetSelection       VisualFoundationScopeProjectPresetSelectionRef `json:"project_preset_selection"`
		PresetCapabilityManifestRoot string                                         `json:"preset_capability_manifest_root"`
		VisualFoundationCandidate    VisualFoundationScopeCandidateRevisionRef      `json:"visual_foundation_candidate"`
		ReferencePlanCandidate       VisualFoundationScopeCandidateRevisionRef      `json:"reference_plan_candidate"`
		ReferenceTargetSeedRoot      string                                         `json:"reference_target_seed_root"`
		ExpectedReferenceTargetSet   storygraphdomain.ExpectedReferenceTargetSet    `json:"expected_reference_target_set"`
	}{
		value.ConfirmedProductionWorld,
		value.PresetRelease,
		value.ProjectPresetSelection,
		value.PresetCapabilityManifestRoot,
		value.VisualFoundationCandidate,
		value.ReferencePlanCandidate,
		value.ReferenceTargetSeedRoot,
		value.ExpectedReferenceTargetSet,
	}
}

func cloneExpectedReferenceTargetSet(
	value storygraphdomain.ExpectedReferenceTargetSet,
) storygraphdomain.ExpectedReferenceTargetSet {
	return storygraphdomain.ExpectedReferenceTargetSet{
		OwnerSetHash:               value.OwnerSetHash,
		P1ScopeKeys:                append([]string(nil), value.P1ScopeKeys...),
		ExpectedTargetBusinessKeys: append([]string(nil), value.ExpectedTargetBusinessKeys...),
		ExpectedTargetKeyRoot:      value.ExpectedTargetKeyRoot,
	}
}

func encodeVisualFoundationScopeSubject(
	value VisualFoundationScopeSubject,
) (VisualFoundationScopeSubject, json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return VisualFoundationScopeSubject{}, nil, err
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return VisualFoundationScopeSubject{}, nil, err
	}
	return value, json.RawMessage(canonical), nil
}

func decodeVisualFoundationScopeStrict(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("multiple Gate 3 JSON values")
	}
	return nil
}

func visualFoundationScopeHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func visualFoundationScopeCanonicalHash(raw json.RawMessage) (string, error) {
	return platformcanonical.Hash(raw)
}
