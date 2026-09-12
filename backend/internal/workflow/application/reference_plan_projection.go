package application

import (
	"encoding/json"
	"errors"
	"slices"
	"sort"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	storygraphdomain "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

type ReferencePlanProjectedOwnerRefs struct {
	Identity      []agentcontract.ReferencePlanOwnerRef `json:"identity"`
	Specification []agentcontract.ReferencePlanOwnerRef `json:"specification"`
	State         []agentcontract.ReferencePlanOwnerRef `json:"state"`
	Scene         []agentcontract.ReferencePlanOwnerRef `json:"scene"`
	Occurrence    []agentcontract.ReferencePlanOwnerRef `json:"occurrence"`
	Interaction   []agentcontract.ReferencePlanOwnerRef `json:"interaction"`
}

type ReferencePlanCandidateConstraintProjection struct {
	ProductionWorldOwnerSetHash           string   `json:"production_world_owner_set_hash"`
	ReferenceTargetSeedRoot               string   `json:"reference_target_seed_root"`
	VisualFoundationCandidateRevisionID   string   `json:"visual_foundation_candidate_revision_id"`
	VisualFoundationCandidateRevisionHash string   `json:"visual_foundation_candidate_revision_hash"`
	PresetReleaseContentHash              string   `json:"preset_release_content_hash"`
	DesignFocus                           []string `json:"design_focus"`
	ForbiddenChanges                      []string `json:"forbidden_changes"`
}

type ReferencePlanTargetProjection struct {
	TargetBusinessKey           string                                     `json:"target_business_key"`
	TargetKind                  string                                     `json:"target_kind"`
	Fulfillment                 string                                     `json:"fulfillment"`
	OwnerRefs                   ReferencePlanProjectedOwnerRefs            `json:"owner_refs"`
	CoverageScopeKeys           []string                                   `json:"coverage_scope_keys"`
	DependsOnTargetBusinessKeys []string                                   `json:"depends_on_target_business_keys"`
	Constraints                 ReferencePlanCandidateConstraintProjection `json:"constraints"`
}

type ReferencePlanCandidateProjection struct {
	Candidate         agentcontract.ReferencePlanCandidate        `json:"candidate"`
	Targets           []ReferencePlanTargetProjection             `json:"targets"`
	ExpectedTargetSet storygraphdomain.ExpectedReferenceTargetSet `json:"expected_target_set"`
}

// BuildReferencePlanCandidateProjection restores the Backend-owned Owner refs,
// coverage, constraints, and dependencies that an Agent is not allowed to
// author. It creates no Effective Snapshot or approved Reference Plan.
func BuildReferencePlanCandidateProjection(
	input agentcontract.ReferencePlanInput,
	candidateBytes json.RawMessage,
) (ReferencePlanCandidateProjection, error) {
	candidate, _, err := agentcontract.DecodeReferencePlanCandidate(candidateBytes)
	if err != nil || candidate.ValidateFor(input) != nil {
		return ReferencePlanCandidateProjection{}, errors.New("invalid Reference Plan Candidate projection input")
	}
	specifications := make(map[string]agentcontract.ReferencePlanTargetSpecification, len(candidate.TargetSpecifications))
	for _, specification := range candidate.TargetSpecifications {
		specifications[specification.TargetBusinessKey] = specification
	}
	selected := make(map[string]agentcontract.ReferencePlanOwnerRef, len(candidate.AnchorSelections))
	for _, selection := range candidate.AnchorSelections {
		selected[selection.AnchorBusinessKey] = selection.SelectedStateRef
	}
	sceneByScope, err := referencePlanSceneRefsByScope(input.FixedTargetSeeds)
	if err != nil {
		return ReferencePlanCandidateProjection{}, err
	}
	targets := make([]ReferencePlanTargetProjection, 0, len(candidate.TargetSpecifications))
	for _, seed := range input.CharacterSeeds {
		selectedState, exists := selected[seed.AnchorBusinessKey]
		if !exists {
			return ReferencePlanCandidateProjection{}, errors.New("Reference Plan Anchor selection is incomplete")
		}
		for _, option := range seed.StateOptions {
			key := option.AppearanceBusinessKey
			kind := "character_appearance"
			if referencePlanProjectionOwnerRefEqual(option.StateRef, selectedState) {
				key = seed.AnchorBusinessKey
				kind = "character_identity_anchor"
			}
			specification, exists := specifications[key]
			if !exists {
				continue
			}
			targets = append(targets, referencePlanProjectionTarget(
				specification,
				ReferencePlanProjectedOwnerRefs{
					Identity:      []agentcontract.ReferencePlanOwnerRef{seed.IdentityRef},
					Specification: []agentcontract.ReferencePlanOwnerRef{seed.SpecificationRef},
					State:         []agentcontract.ReferencePlanOwnerRef{option.StateRef},
					Scene:         referencePlanProjectionSceneRefs(option.CoverageScopeKeys, sceneByScope),
					Occurrence:    append([]agentcontract.ReferencePlanOwnerRef(nil), option.OccurrenceRefs...),
					Interaction:   []agentcontract.ReferencePlanOwnerRef{},
				},
				option.CoverageScopeKeys,
				input.VisualFoundationCandidateRevisionID,
				input.VisualFoundationCandidateRevisionHash,
				input.ProductionWorldOwnerSetHash,
				input.ReferenceTargetSeedRoot,
				input.VisualFoundationCandidate.PresetReleaseContentHash,
			))
			if kind != specification.TargetKind {
				return ReferencePlanCandidateProjection{}, errors.New("Reference Plan Character target kind drifted")
			}
		}
	}
	for _, seed := range input.FixedTargetSeeds {
		specification, exists := specifications[seed.TargetBusinessKey]
		if !exists {
			return ReferencePlanCandidateProjection{}, errors.New("Reference Plan fixed Target is incomplete")
		}
		targets = append(targets, referencePlanProjectionTarget(
			specification,
			ReferencePlanProjectedOwnerRefs{
				Identity:      append([]agentcontract.ReferencePlanOwnerRef(nil), seed.OwnerRefs.Identity...),
				Specification: append([]agentcontract.ReferencePlanOwnerRef(nil), seed.OwnerRefs.Specification...),
				State:         append([]agentcontract.ReferencePlanOwnerRef(nil), seed.OwnerRefs.State...),
				Scene:         append([]agentcontract.ReferencePlanOwnerRef(nil), seed.OwnerRefs.Scene...),
				Occurrence:    append([]agentcontract.ReferencePlanOwnerRef(nil), seed.OwnerRefs.Occurrence...),
				Interaction:   append([]agentcontract.ReferencePlanOwnerRef(nil), seed.OwnerRefs.Interaction...),
			},
			seed.CoverageScopeKeys,
			input.VisualFoundationCandidateRevisionID,
			input.VisualFoundationCandidateRevisionHash,
			input.ProductionWorldOwnerSetHash,
			input.ReferenceTargetSeedRoot,
			input.VisualFoundationCandidate.PresetReleaseContentHash,
		))
	}
	sort.Slice(targets, func(left, right int) bool {
		return targets[left].TargetBusinessKey < targets[right].TargetBusinessKey
	})
	keys := make([]string, len(targets))
	for index, target := range targets {
		keys[index] = target.TargetBusinessKey
	}
	if len(targets) != len(specifications) || !slices.Equal(keys, referencePlanCandidateTargetKeys(candidate)) {
		return ReferencePlanCandidateProjection{}, errors.New("Reference Plan projected Target set drifted")
	}
	proof, err := storygraphdomain.BuildExpectedReferenceTargetSetProof(
		input.ProductionWorldOwnerSetHash, input.P1ScopeKeys, keys,
	)
	if err != nil {
		return ReferencePlanCandidateProjection{}, err
	}
	return ReferencePlanCandidateProjection{Candidate: candidate, Targets: targets, ExpectedTargetSet: proof}, nil
}

func ValidateReferencePlanCandidateProjection(
	input agentcontract.ReferencePlanInput,
	candidate json.RawMessage,
) error {
	_, err := BuildReferencePlanCandidateProjection(input, candidate)
	return err
}

func referencePlanProjectionTarget(
	specification agentcontract.ReferencePlanTargetSpecification,
	ownerRefs ReferencePlanProjectedOwnerRefs,
	coverage []string,
	visualFoundationCandidateRevisionID string,
	visualFoundationCandidateRevisionHash string,
	productionWorldOwnerSetHash string,
	referenceTargetSeedRoot string,
	presetReleaseContentHash string,
) ReferencePlanTargetProjection {
	return ReferencePlanTargetProjection{
		TargetBusinessKey: specification.TargetBusinessKey, TargetKind: specification.TargetKind,
		Fulfillment: specification.Fulfillment, OwnerRefs: ownerRefs,
		CoverageScopeKeys:           append([]string(nil), coverage...),
		DependsOnTargetBusinessKeys: append([]string(nil), specification.DependsOnTargetBusinessKeys...),
		Constraints: ReferencePlanCandidateConstraintProjection{
			ProductionWorldOwnerSetHash:           productionWorldOwnerSetHash,
			ReferenceTargetSeedRoot:               referenceTargetSeedRoot,
			VisualFoundationCandidateRevisionID:   visualFoundationCandidateRevisionID,
			VisualFoundationCandidateRevisionHash: visualFoundationCandidateRevisionHash,
			PresetReleaseContentHash:              presetReleaseContentHash,
			DesignFocus:                           append([]string(nil), specification.DesignFocus...),
			ForbiddenChanges:                      append([]string(nil), specification.ForbiddenChanges...),
		},
	}
}

func referencePlanSceneRefsByScope(
	seeds []agentcontract.ReferencePlanFixedTargetSeed,
) (map[string]agentcontract.ReferencePlanOwnerRef, error) {
	result := make(map[string]agentcontract.ReferencePlanOwnerRef)
	for _, seed := range seeds {
		if seed.TargetKind != "scene_composition" || len(seed.CoverageScopeKeys) != 1 || len(seed.OwnerRefs.Scene) != 1 {
			continue
		}
		result[seed.CoverageScopeKeys[0]] = seed.OwnerRefs.Scene[0]
	}
	return result, nil
}

func referencePlanProjectionSceneRefs(
	scopes []string,
	refs map[string]agentcontract.ReferencePlanOwnerRef,
) []agentcontract.ReferencePlanOwnerRef {
	result := make([]agentcontract.ReferencePlanOwnerRef, 0, len(scopes))
	for _, scope := range scopes {
		if value, exists := refs[scope]; exists {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(left, right int) bool {
		leftBytes, _ := json.Marshal(result[left])
		rightBytes, _ := json.Marshal(result[right])
		return string(leftBytes) < string(rightBytes)
	})
	return result
}

func referencePlanProjectionOwnerRefEqual(
	left agentcontract.ReferencePlanOwnerRef,
	right agentcontract.ReferencePlanOwnerRef,
) bool {
	leftBytes, _ := json.Marshal(left)
	rightBytes, _ := json.Marshal(right)
	return string(leftBytes) == string(rightBytes)
}

func referencePlanCandidateTargetKeys(candidate agentcontract.ReferencePlanCandidate) []string {
	result := make([]string, len(candidate.TargetSpecifications))
	for index, specification := range candidate.TargetSpecifications {
		result[index] = specification.TargetBusinessKey
	}
	return result
}
