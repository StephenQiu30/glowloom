package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	presetdomain "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
	storygraphdomain "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

// ReferencePlanVisualFoundationCandidateRevision is the exact persisted
// Candidate projection required to compile one Reference Plan invocation.
type ReferencePlanVisualFoundationCandidateRevision struct {
	ID                   string
	RevisionHash         string
	CandidateContentHash string
	Candidate            json.RawMessage
}

type ReferencePlanInputCommand struct {
	Inventory                storygraphdomain.ReferencePlanSeedInventory
	VisualFoundationRevision ReferencePlanVisualFoundationCandidateRevision
	PresetRelease            presetdomain.Release
}

// CompileReferencePlanInput combines only Backend-owned frozen facts. It does
// not publish an Effective Snapshot, write a Reference Plan, or call an Agent.
func CompileReferencePlanInput(
	command ReferencePlanInputCommand,
) (agentcontract.ReferencePlanInput, json.RawMessage, error) {
	visualFoundation, err := decodeReferencePlanVisualFoundationRevision(command.VisualFoundationRevision)
	if err != nil {
		return agentcontract.ReferencePlanInput{}, nil, err
	}
	if err = validateReferencePlanPreset(command.PresetRelease, visualFoundation); err != nil {
		return agentcontract.ReferencePlanInput{}, nil, err
	}
	input := agentcontract.ReferencePlanInput{
		WorkspaceID: visualFoundation.WorkspaceID, ProjectID: visualFoundation.ProjectID,
		ProductionWorldOwnerSetHash:           command.Inventory.OwnerSetHash,
		P1ScopeKeys:                           append([]string(nil), command.Inventory.P1ScopeKeys...),
		VisualFoundationCandidateRevisionID:   command.VisualFoundationRevision.ID,
		VisualFoundationCandidateRevisionHash: command.VisualFoundationRevision.RevisionHash,
		VisualFoundationCandidate:             visualFoundation,
		CharacterSeeds:                        referencePlanCharacterSeeds(command.Inventory.CharacterSeeds),
		FixedTargetSeeds:                      referencePlanFixedTargetSeeds(command.Inventory.FixedTargetSeeds),
		PurposeProfiles:                       referencePlanPurposeProfiles(command.PresetRelease.PurposeProfiles),
	}
	input.ReferenceTargetSeedRoot, err = input.ComputeSeedRoot()
	if err != nil {
		return agentcontract.ReferencePlanInput{}, nil, err
	}
	if err = input.Validate(); err != nil {
		return agentcontract.ReferencePlanInput{}, nil, fmt.Errorf("invalid frozen Reference Plan input: %w", err)
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return agentcontract.ReferencePlanInput{}, nil, err
	}
	decoded, canonical, err := agentcontract.DecodeReferencePlanInput(raw)
	if err != nil {
		return agentcontract.ReferencePlanInput{}, nil, fmt.Errorf("decode frozen Reference Plan input: %w", err)
	}
	return decoded, canonical, nil
}

func decodeReferencePlanVisualFoundationRevision(
	value ReferencePlanVisualFoundationCandidateRevision,
) (agentcontract.VisualFoundationCandidate, error) {
	identifier, err := uuid.Parse(value.ID)
	if err != nil || identifier == uuid.Nil || !visualFoundationHashPattern.MatchString(value.RevisionHash) ||
		!visualFoundationHashPattern.MatchString(value.CandidateContentHash) {
		return agentcontract.VisualFoundationCandidate{}, errors.New("invalid Visual Foundation Candidate revision")
	}
	candidate, canonical, err := agentcontract.DecodeVisualFoundationCandidate(value.Candidate)
	if err != nil {
		return agentcontract.VisualFoundationCandidate{}, errors.New("invalid Visual Foundation Candidate revision content")
	}
	contentHash, err := platformcanonical.Hash(canonical)
	if err != nil || contentHash != value.CandidateContentHash {
		return agentcontract.VisualFoundationCandidate{}, errors.New("Visual Foundation Candidate content has drifted")
	}
	return candidate, nil
}

func validateReferencePlanPreset(
	value presetdomain.Release,
	candidate agentcontract.VisualFoundationCandidate,
) error {
	rebuilt, _, err := presetdomain.NewRelease(value.ReleaseInput)
	if err != nil || !reflect.DeepEqual(rebuilt, value) || value.ContentHash != candidate.PresetReleaseContentHash ||
		!reflect.DeepEqual(value.FidelityInvariants, candidate.FidelityInvariants) {
		return errors.New("Reference Plan Preset release has drifted")
	}
	return nil
}

func referencePlanCharacterSeeds(
	values []storygraphdomain.ReferencePlanCharacterSeed,
) []agentcontract.ReferencePlanCharacterSeed {
	result := make([]agentcontract.ReferencePlanCharacterSeed, len(values))
	for index, value := range values {
		states := make([]agentcontract.ReferencePlanCharacterStateSeed, len(value.StateOptions))
		for stateIndex, state := range value.StateOptions {
			states[stateIndex] = agentcontract.ReferencePlanCharacterStateSeed{
				StateRef: referencePlanOwnerRef(state.StateRef), AppearanceBusinessKey: state.AppearanceBusinessKey,
				CoverageScopeKeys: append([]string(nil), state.CoverageScopeKeys...),
				OccurrenceRefs:    referencePlanOwnerRefs(state.OccurrenceRefs),
			}
		}
		result[index] = agentcontract.ReferencePlanCharacterSeed{
			AnchorBusinessKey: value.AnchorBusinessKey, IdentityRef: referencePlanOwnerRef(value.IdentityRef),
			SpecificationRef:  referencePlanOwnerRef(value.SpecificationRef),
			CoverageScopeKeys: append([]string(nil), value.CoverageScopeKeys...), StateOptions: states,
		}
	}
	return result
}

func referencePlanFixedTargetSeeds(
	values []storygraphdomain.ReferencePlanFixedTargetSeed,
) []agentcontract.ReferencePlanFixedTargetSeed {
	result := make([]agentcontract.ReferencePlanFixedTargetSeed, len(values))
	for index, value := range values {
		characterDependencies := make([]agentcontract.ReferencePlanCharacterDependencySeed, len(value.CharacterDependencies))
		for dependencyIndex, dependency := range value.CharacterDependencies {
			characterDependencies[dependencyIndex] = agentcontract.ReferencePlanCharacterDependencySeed{
				AnchorBusinessKey: dependency.AnchorBusinessKey, StateRef: referencePlanOwnerRef(dependency.StateRef),
				AppearanceBusinessKey: dependency.AppearanceBusinessKey,
			}
		}
		result[index] = agentcontract.ReferencePlanFixedTargetSeed{
			TargetBusinessKey: value.TargetBusinessKey, TargetKind: value.TargetKind,
			OwnerRefs: agentcontract.ReferencePlanTargetOwnerRefs{
				Identity:      referencePlanOwnerRefs(value.OwnerRefs.Identity),
				Specification: referencePlanOwnerRefs(value.OwnerRefs.Specification),
				State:         referencePlanOwnerRefs(value.OwnerRefs.State),
				Scene:         referencePlanOwnerRefs(value.OwnerRefs.Scene),
				Occurrence:    referencePlanOwnerRefs(value.OwnerRefs.Occurrence),
				Interaction:   referencePlanOwnerRefs(value.OwnerRefs.Interaction),
			},
			CoverageScopeKeys:           append([]string(nil), value.CoverageScopeKeys...),
			FixedDependencyBusinessKeys: referencePlanStrings(value.FixedDependencyBusinessKeys),
			CharacterDependencies:       characterDependencies,
		}
	}
	return result
}

func referencePlanPurposeProfiles(values []presetdomain.PurposeProfile) []agentcontract.ReferencePlanPurposeProfile {
	result := make([]agentcontract.ReferencePlanPurposeProfile, len(values))
	for index, value := range values {
		result[index] = agentcontract.ReferencePlanPurposeProfile{
			TargetKind: value.TargetKind, DesignFocus: append([]string(nil), value.DesignFocus...),
			ForbiddenChanges: append([]string(nil), value.ForbiddenChanges...),
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left].TargetKind < result[right].TargetKind })
	return result
}

func referencePlanStrings(values []string) []string {
	result := make([]string, len(values))
	copy(result, values)
	return result
}

func referencePlanOwnerRefs(values []storygraphdomain.OwnerRef) []agentcontract.ReferencePlanOwnerRef {
	result := make([]agentcontract.ReferencePlanOwnerRef, len(values))
	for index, value := range values {
		result[index] = referencePlanOwnerRef(value)
	}
	return result
}

func referencePlanOwnerRef(value storygraphdomain.OwnerRef) agentcontract.ReferencePlanOwnerRef {
	var fragmentKey, fragmentContentHash *string
	if value.FragmentKey != "" {
		fragmentKeyValue, fragmentHashValue := value.FragmentKey, value.FragmentContentHash
		fragmentKey, fragmentContentHash = &fragmentKeyValue, &fragmentHashValue
	}
	return agentcontract.ReferencePlanOwnerRef{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID,
		OwnerKind: value.OwnerKind, VersionFamily: value.VersionFamily, OwnerLogicalID: value.OwnerLogicalID,
		OwnerVersionID: value.OwnerVersionID, OwnerRevision: value.OwnerRevision,
		OwnerContentHash: value.OwnerContentHash, FragmentKey: fragmentKey, FragmentContentHash: fragmentContentHash,
	}
}
