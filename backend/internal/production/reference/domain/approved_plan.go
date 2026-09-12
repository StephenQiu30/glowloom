package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformowner "github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
	storygraphdomain "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

const (
	referencePlanOwnerKind            = "production/reference"
	referencePlanFamily               = "reference_plan_set"
	approvedReferencePlanContractID   = "approved-reference-plan-production"
	referencePlanTargetContractID     = "reference-plan-target-production"
	referencePlanScopeHeadContractID  = "reference-plan-scope-head-production"
	referencePlanActivationContractID = "project-reference-plan-activation-head-production"
)

var (
	referencePlanHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	referencePlanTargetKinds = []string{
		"character_appearance",
		"character_identity_anchor",
		"interaction_composition",
		"location_board",
		"prop_sheet",
		"scene_composition",
	}
)

type TargetConstraints struct {
	ProductionWorldOwnerSetHash           string   `json:"production_world_owner_set_hash"`
	ReferenceTargetSeedRoot               string   `json:"reference_target_seed_root"`
	VisualFoundationCandidateRevisionID   string   `json:"visual_foundation_candidate_revision_id"`
	VisualFoundationCandidateRevisionHash string   `json:"visual_foundation_candidate_revision_hash"`
	PresetReleaseContentHash              string   `json:"preset_release_content_hash"`
	DesignFocus                           []string `json:"design_focus"`
	ForbiddenChanges                      []string `json:"forbidden_changes"`
}

type TargetDraft struct {
	VersionID                   string
	TargetBusinessKey           string
	TargetKind                  string
	Fulfillment                 string
	OwnerRefs                   agentcontract.ReferencePlanTargetOwnerRefs
	CoverageScopeKeys           []string
	DependsOnTargetBusinessKeys []string
	Constraints                 TargetConstraints
}

type ApprovedReferencePlanDraft struct {
	PlanLogicalID                         string
	PlanVersionID                         string
	Revision                              int64
	WorkspaceID                           string
	ProjectID                             string
	CandidateRevisionID                   string
	CandidateRevision                     int64
	CandidateRevisionHash                 string
	CandidateContentHash                  string
	VisualFoundationCandidateRevisionID   string
	VisualFoundationCandidateRevisionHash string
	PresetReleaseContentHash              string
	ProductionWorldOwnerSetHash           string
	ReferenceTargetSeedRoot               string
	ExpectedTargetSet                     storygraphdomain.ExpectedReferenceTargetSet
	EffectiveStyleSnapshot                platformowner.VersionRef
	EffectivePolicySnapshot               platformowner.VersionRef
	Targets                               []TargetDraft
	ReviewDecisionID                      string
	CreatedBy                             string
	CreatedAt                             time.Time
}

type ReferencePlanTargetVersion struct {
	ID                          string                                     `json:"id"`
	ContractID                  string                                     `json:"contract_id"`
	WorkspaceID                 string                                     `json:"workspace_id"`
	ProjectID                   string                                     `json:"project_id"`
	PlanLogicalID               string                                     `json:"plan_logical_id"`
	PlanVersionID               string                                     `json:"plan_version_id"`
	Revision                    int64                                      `json:"revision"`
	TargetBusinessKey           string                                     `json:"target_business_key"`
	TargetKind                  string                                     `json:"target_kind"`
	Fulfillment                 string                                     `json:"fulfillment"`
	OwnerRefs                   agentcontract.ReferencePlanTargetOwnerRefs `json:"owner_refs"`
	CoverageScopeKeys           []string                                   `json:"coverage_scope_keys"`
	DependsOnTargetBusinessKeys []string                                   `json:"depends_on_target_business_keys"`
	Constraints                 TargetConstraints                          `json:"constraints"`
	EffectiveStyleSnapshot      platformowner.VersionRef                   `json:"effective_style_snapshot"`
	EffectivePolicySnapshot     platformowner.VersionRef                   `json:"effective_policy_snapshot"`
	ReviewDecisionID            string                                     `json:"review_decision_id"`
	CreatedBy                   string                                     `json:"created_by"`
	CreatedAt                   time.Time                                  `json:"created_at"`
	ContentHash                 string                                     `json:"content_hash"`
}

type ApprovedReferencePlanVersion struct {
	ID                          string                     `json:"id"`
	ContractID                  string                     `json:"contract_id"`
	WorkspaceID                 string                     `json:"workspace_id"`
	ProjectID                   string                     `json:"project_id"`
	LogicalID                   string                     `json:"logical_id"`
	Revision                    int64                      `json:"revision"`
	CandidateRevisionID         string                     `json:"candidate_revision_id"`
	CandidateRevision           int64                      `json:"candidate_revision"`
	CandidateRevisionHash       string                     `json:"candidate_revision_hash"`
	CandidateContentHash        string                     `json:"candidate_content_hash"`
	ProductionWorldOwnerSetHash string                     `json:"production_world_owner_set_hash"`
	ReferenceTargetSeedRoot     string                     `json:"reference_target_seed_root"`
	P1ScopeKeys                 []string                   `json:"p1_scope_keys"`
	ExpectedTargetKeyRoot       string                     `json:"expected_target_key_root"`
	TargetVersionRefs           []platformowner.VersionRef `json:"target_version_refs"`
	EffectiveStyleSnapshot      platformowner.VersionRef   `json:"effective_style_snapshot"`
	EffectivePolicySnapshot     platformowner.VersionRef   `json:"effective_policy_snapshot"`
	ReviewDecisionID            string                     `json:"review_decision_id"`
	CreatedBy                   string                     `json:"created_by"`
	CreatedAt                   time.Time                  `json:"created_at"`
	ContentHash                 string                     `json:"content_hash"`
}

type ReferencePlanScopeHead struct {
	ContractID             string `json:"contract_id"`
	WorkspaceID            string `json:"workspace_id"`
	ProjectID              string `json:"project_id"`
	PlanLogicalID          string `json:"plan_logical_id"`
	HeadRevision           int64  `json:"head_revision"`
	CurrentPlanVersionID   string `json:"current_plan_version_id"`
	CurrentPlanRevision    int64  `json:"current_plan_revision"`
	CurrentPlanContentHash string `json:"current_plan_content_hash"`
	ContentHash            string `json:"content_hash"`
}

type ProjectReferencePlanActivationHead struct {
	ContractID             string `json:"contract_id"`
	WorkspaceID            string `json:"workspace_id"`
	ProjectID              string `json:"project_id"`
	HeadRevision           int64  `json:"head_revision"`
	CurrentPlanLogicalID   string `json:"current_plan_logical_id"`
	CurrentPlanVersionID   string `json:"current_plan_version_id"`
	CurrentPlanRevision    int64  `json:"current_plan_revision"`
	CurrentPlanContentHash string `json:"current_plan_content_hash"`
	ContentHash            string `json:"content_hash"`
}

type ApprovedReferencePlanSet struct {
	Version        ApprovedReferencePlanVersion       `json:"version"`
	Targets        []ReferencePlanTargetVersion       `json:"targets"`
	ScopeHead      ReferencePlanScopeHead             `json:"scope_head"`
	ActivationHead ProjectReferencePlanActivationHead `json:"activation_head"`
	Collection     platformowner.Ref                  `json:"collection"`
}

func BuildApprovedReferencePlan(draft ApprovedReferencePlanDraft) (ApprovedReferencePlanSet, error) {
	if err := validateApprovedReferencePlanDraft(draft); err != nil {
		return ApprovedReferencePlanSet{}, err
	}
	targetDrafts := append([]TargetDraft(nil), draft.Targets...)
	sort.Slice(targetDrafts, func(left, right int) bool {
		return targetDrafts[left].TargetBusinessKey < targetDrafts[right].TargetBusinessKey
	})
	keys := make([]string, len(targetDrafts))
	targets := make([]ReferencePlanTargetVersion, len(targetDrafts))
	targetRefs := make([]platformowner.VersionRef, len(targetDrafts))
	for index, targetDraft := range targetDrafts {
		keys[index] = targetDraft.TargetBusinessKey
		target := ReferencePlanTargetVersion{
			ID: targetDraft.VersionID, ContractID: referencePlanTargetContractID,
			WorkspaceID: draft.WorkspaceID, ProjectID: draft.ProjectID,
			PlanLogicalID: draft.PlanLogicalID, PlanVersionID: draft.PlanVersionID, Revision: draft.Revision,
			TargetBusinessKey: targetDraft.TargetBusinessKey, TargetKind: targetDraft.TargetKind,
			Fulfillment: targetDraft.Fulfillment, OwnerRefs: cloneOwnerRefs(targetDraft.OwnerRefs),
			CoverageScopeKeys:           append([]string{}, targetDraft.CoverageScopeKeys...),
			DependsOnTargetBusinessKeys: append([]string{}, targetDraft.DependsOnTargetBusinessKeys...),
			Constraints:                 cloneTargetConstraints(targetDraft.Constraints),
			EffectiveStyleSnapshot:      draft.EffectiveStyleSnapshot,
			EffectivePolicySnapshot:     draft.EffectivePolicySnapshot,
			ReviewDecisionID:            draft.ReviewDecisionID, CreatedBy: draft.CreatedBy, CreatedAt: draft.CreatedAt,
		}
		var err error
		target.ContentHash, err = referencePlanHash(target)
		if err != nil {
			return ApprovedReferencePlanSet{}, err
		}
		targets[index] = target
		targetRefs[index] = referencePlanVersionRef(
			draft, target.TargetBusinessKey, target.ID, target.Revision, target.ContentHash,
		)
	}
	if !slices.Equal(keys, draft.ExpectedTargetSet.ExpectedTargetBusinessKeys) {
		return ApprovedReferencePlanSet{}, errors.New("Reference Plan target set differs from expected set")
	}
	proof, err := storygraphdomain.BuildExpectedReferenceTargetSetProof(
		draft.ProductionWorldOwnerSetHash, draft.ExpectedTargetSet.P1ScopeKeys, keys,
	)
	if err != nil || !reflect.DeepEqual(proof, draft.ExpectedTargetSet) {
		return ApprovedReferencePlanSet{}, errors.New("Reference Plan expected target proof has drifted")
	}
	version := ApprovedReferencePlanVersion{
		ID: draft.PlanVersionID, ContractID: approvedReferencePlanContractID,
		WorkspaceID: draft.WorkspaceID, ProjectID: draft.ProjectID,
		LogicalID: draft.PlanLogicalID, Revision: draft.Revision,
		CandidateRevisionID: draft.CandidateRevisionID, CandidateRevision: draft.CandidateRevision,
		CandidateRevisionHash: draft.CandidateRevisionHash, CandidateContentHash: draft.CandidateContentHash,
		ProductionWorldOwnerSetHash: draft.ProductionWorldOwnerSetHash,
		ReferenceTargetSeedRoot:     draft.ReferenceTargetSeedRoot,
		P1ScopeKeys:                 append([]string(nil), draft.ExpectedTargetSet.P1ScopeKeys...),
		ExpectedTargetKeyRoot:       draft.ExpectedTargetSet.ExpectedTargetKeyRoot,
		TargetVersionRefs:           targetRefs,
		EffectiveStyleSnapshot:      draft.EffectiveStyleSnapshot,
		EffectivePolicySnapshot:     draft.EffectivePolicySnapshot,
		ReviewDecisionID:            draft.ReviewDecisionID, CreatedBy: draft.CreatedBy, CreatedAt: draft.CreatedAt,
	}
	version.ContentHash, err = referencePlanHash(version)
	if err != nil {
		return ApprovedReferencePlanSet{}, err
	}
	scopeHead := ReferencePlanScopeHead{
		ContractID:  referencePlanScopeHeadContractID,
		WorkspaceID: draft.WorkspaceID, ProjectID: draft.ProjectID, PlanLogicalID: draft.PlanLogicalID,
		HeadRevision: draft.Revision, CurrentPlanVersionID: version.ID,
		CurrentPlanRevision: version.Revision, CurrentPlanContentHash: version.ContentHash,
	}
	scopeHead.ContentHash, err = referencePlanHash(scopeHead)
	if err != nil {
		return ApprovedReferencePlanSet{}, err
	}
	activationHead := ProjectReferencePlanActivationHead{
		ContractID:  referencePlanActivationContractID,
		WorkspaceID: draft.WorkspaceID, ProjectID: draft.ProjectID, HeadRevision: draft.Revision,
		CurrentPlanLogicalID: version.LogicalID, CurrentPlanVersionID: version.ID,
		CurrentPlanRevision: version.Revision, CurrentPlanContentHash: version.ContentHash,
	}
	activationHead.ContentHash, err = referencePlanHash(activationHead)
	if err != nil {
		return ApprovedReferencePlanSet{}, err
	}
	members := append([]platformowner.VersionRef(nil), targetRefs...)
	members = append(members, referencePlanVersionRef(
		draft, version.LogicalID, version.ID, version.Revision, version.ContentHash,
	))
	collection, err := platformowner.Build(platformowner.Scope{
		WorkspaceID: draft.WorkspaceID, ProjectID: draft.ProjectID,
		OwnerKind: referencePlanOwnerKind, VersionFamily: referencePlanFamily,
		ScopeKind: "reference_plan", ScopeKey: "reference-plan:" + draft.PlanLogicalID,
		ScopeRevision: draft.Revision,
	}, members)
	if err != nil {
		return ApprovedReferencePlanSet{}, err
	}
	return ApprovedReferencePlanSet{
		Version: version, Targets: targets, ScopeHead: scopeHead,
		ActivationHead: activationHead, Collection: collection,
	}, nil
}

func validateApprovedReferencePlanDraft(draft ApprovedReferencePlanDraft) error {
	for _, identifier := range []string{
		draft.PlanLogicalID, draft.PlanVersionID, draft.WorkspaceID, draft.ProjectID,
		draft.CandidateRevisionID, draft.VisualFoundationCandidateRevisionID,
		draft.ReviewDecisionID, draft.CreatedBy,
	} {
		if parsed, err := uuid.Parse(identifier); err != nil || parsed == uuid.Nil {
			return errors.New("invalid Approved Reference Plan identity")
		}
	}
	if draft.Revision < 1 || draft.CandidateRevision < 1 ||
		draft.CreatedAt.IsZero() || draft.CreatedAt.Location() != time.UTC ||
		!referencePlanHashPattern.MatchString(draft.CandidateRevisionHash) ||
		!referencePlanHashPattern.MatchString(draft.CandidateContentHash) ||
		!referencePlanHashPattern.MatchString(draft.VisualFoundationCandidateRevisionHash) ||
		!referencePlanHashPattern.MatchString(draft.PresetReleaseContentHash) ||
		!referencePlanHashPattern.MatchString(draft.ProductionWorldOwnerSetHash) ||
		!referencePlanHashPattern.MatchString(draft.ReferenceTargetSeedRoot) ||
		draft.ExpectedTargetSet.OwnerSetHash != draft.ProductionWorldOwnerSetHash ||
		len(draft.Targets) == 0 {
		return errors.New("invalid Approved Reference Plan lineage")
	}
	if err := validatePresetSnapshotRef(draft.EffectiveStyleSnapshot, draft); err != nil {
		return err
	}
	if err := validatePresetSnapshotRef(draft.EffectivePolicySnapshot, draft); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(draft.Targets))
	coverage := make(map[string]struct{})
	for _, target := range draft.Targets {
		if _, duplicate := seen[target.TargetBusinessKey]; duplicate {
			return errors.New("duplicate Reference Plan target business key")
		}
		seen[target.TargetBusinessKey] = struct{}{}
		for _, scope := range target.CoverageScopeKeys {
			coverage[scope] = struct{}{}
		}
		if err := validateTargetDraft(target, draft); err != nil {
			return err
		}
	}
	for _, target := range draft.Targets {
		for _, dependency := range target.DependsOnTargetBusinessKeys {
			if _, exists := seen[dependency]; !exists {
				return errors.New("Reference Plan target dependency is outside the Plan")
			}
		}
	}
	coverageKeys := make([]string, 0, len(coverage))
	for key := range coverage {
		coverageKeys = append(coverageKeys, key)
	}
	sort.Strings(coverageKeys)
	if !slices.Equal(coverageKeys, draft.ExpectedTargetSet.P1ScopeKeys) {
		return errors.New("Reference Plan target coverage differs from p1 scope")
	}
	return nil
}

func validateTargetDraft(target TargetDraft, draft ApprovedReferencePlanDraft) error {
	if parsed, err := uuid.Parse(target.VersionID); err != nil || parsed == uuid.Nil ||
		strings.TrimSpace(target.TargetBusinessKey) == "" ||
		!slices.Contains(referencePlanTargetKinds, target.TargetKind) ||
		!slices.Contains([]string{"not_generated", "optional", "required"}, target.Fulfillment) ||
		!sortedStrings(target.CoverageScopeKeys, true) ||
		!sortedStrings(target.DependsOnTargetBusinessKeys, false) ||
		!sortedStrings(target.Constraints.DesignFocus, true) ||
		!sortedStrings(target.Constraints.ForbiddenChanges, true) {
		return errors.New("invalid Reference Plan target")
	}
	var businessKey []json.RawMessage
	if err := platformcanonical.Decode([]byte(target.TargetBusinessKey), &businessKey); err != nil || len(businessKey) == 0 {
		return errors.New("invalid Reference Plan target business key")
	}
	var keyKind string
	if err := json.Unmarshal(businessKey[0], &keyKind); err != nil || keyKind != target.TargetKind {
		return errors.New("Reference Plan target kind drifted")
	}
	if target.Constraints.ProductionWorldOwnerSetHash != draft.ProductionWorldOwnerSetHash ||
		target.Constraints.ReferenceTargetSeedRoot != draft.ReferenceTargetSeedRoot ||
		target.Constraints.VisualFoundationCandidateRevisionID != draft.VisualFoundationCandidateRevisionID ||
		target.Constraints.VisualFoundationCandidateRevisionHash != draft.VisualFoundationCandidateRevisionHash ||
		target.Constraints.PresetReleaseContentHash != draft.PresetReleaseContentHash {
		return errors.New("Reference Plan target constraints have drifted")
	}
	if err := validateTargetOwnerRefs(target.OwnerRefs, draft.WorkspaceID, draft.ProjectID); err != nil {
		return err
	}
	return nil
}

func validatePresetSnapshotRef(ref platformowner.VersionRef, draft ApprovedReferencePlanDraft) error {
	if ref.WorkspaceID != draft.WorkspaceID || ref.ProjectID != draft.ProjectID ||
		ref.OwnerKind != "preset" || ref.VersionFamily != "preset_effective_set" ||
		ref.OwnerLogicalID != draft.ProjectID || ref.OwnerRevision != draft.Revision ||
		uuid.Validate(ref.OwnerVersionID) != nil || !referencePlanHashPattern.MatchString(ref.OwnerContentHash) {
		return errors.New("invalid effective Preset snapshot ref")
	}
	return nil
}

func validateTargetOwnerRefs(refs agentcontract.ReferencePlanTargetOwnerRefs, workspaceID, projectID string) error {
	groups := [][]agentcontract.ReferencePlanOwnerRef{
		refs.Identity, refs.Specification, refs.State, refs.Scene, refs.Occurrence, refs.Interaction,
	}
	for _, group := range groups {
		previous := ""
		for index, ref := range group {
			if ref.WorkspaceID != workspaceID || ref.ProjectID != projectID ||
				strings.TrimSpace(ref.OwnerKind) == "" || strings.TrimSpace(ref.VersionFamily) == "" ||
				strings.TrimSpace(ref.OwnerLogicalID) == "" || uuid.Validate(ref.OwnerVersionID) != nil ||
				ref.OwnerRevision < 1 || !referencePlanHashPattern.MatchString(ref.OwnerContentHash) ||
				(ref.FragmentKey == nil) != (ref.FragmentContentHash == nil) {
				return errors.New("invalid Reference Plan target Owner ref")
			}
			if ref.FragmentContentHash != nil && !referencePlanHashPattern.MatchString(*ref.FragmentContentHash) {
				return errors.New("invalid Reference Plan target fragment ref")
			}
			raw, err := json.Marshal(ref)
			if err != nil || index > 0 && previous >= string(raw) {
				return errors.New("unsorted Reference Plan target Owner refs")
			}
			previous = string(raw)
		}
	}
	return nil
}

func sortedStrings(values []string, requireNonEmpty bool) bool {
	if requireNonEmpty && len(values) == 0 {
		return false
	}
	for index, value := range values {
		if strings.TrimSpace(value) == "" || index > 0 && values[index-1] >= value {
			return false
		}
	}
	return true
}

func cloneOwnerRefs(value agentcontract.ReferencePlanTargetOwnerRefs) agentcontract.ReferencePlanTargetOwnerRefs {
	return agentcontract.ReferencePlanTargetOwnerRefs{
		Identity:      append([]agentcontract.ReferencePlanOwnerRef{}, value.Identity...),
		Specification: append([]agentcontract.ReferencePlanOwnerRef{}, value.Specification...),
		State:         append([]agentcontract.ReferencePlanOwnerRef{}, value.State...),
		Scene:         append([]agentcontract.ReferencePlanOwnerRef{}, value.Scene...),
		Occurrence:    append([]agentcontract.ReferencePlanOwnerRef{}, value.Occurrence...),
		Interaction:   append([]agentcontract.ReferencePlanOwnerRef{}, value.Interaction...),
	}
}

func cloneTargetConstraints(value TargetConstraints) TargetConstraints {
	value.DesignFocus = append([]string{}, value.DesignFocus...)
	value.ForbiddenChanges = append([]string{}, value.ForbiddenChanges...)
	return value
}

func referencePlanHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func referencePlanVersionRef(draft ApprovedReferencePlanDraft, logicalID, versionID string, revision int64, contentHash string) platformowner.VersionRef {
	return platformowner.VersionRef{
		WorkspaceID: draft.WorkspaceID, ProjectID: draft.ProjectID,
		OwnerKind: referencePlanOwnerKind, VersionFamily: referencePlanFamily,
		OwnerLogicalID: logicalID, OwnerVersionID: versionID,
		OwnerRevision: revision, OwnerContentHash: contentHash,
	}
}
