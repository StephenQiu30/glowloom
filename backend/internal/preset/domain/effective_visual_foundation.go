package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"time"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformowner "github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
)

const (
	effectiveVisualFoundationOwnerKind    = "preset"
	effectiveVisualFoundationFamily       = "preset_effective_set"
	effectiveVisualFoundationContractID   = "effective-visual-foundation-production"
	effectiveStyleSnapshotContractID      = "effective-style-snapshot-production"
	effectivePolicySnapshotContractID     = "effective-policy-snapshot-production"
	projectPresetBindingVersionContractID = "project-preset-binding-version-production"
)

var effectiveVisualFoundationHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// EffectiveVisualFoundationSetDraft freezes the exact Gate 3 inputs from which
// preset publishes its binding and effective snapshots.
type EffectiveVisualFoundationSetDraft struct {
	BindingVersionID            string
	StyleSnapshotID             string
	PolicySnapshotID            string
	Revision                    int64
	Selection                   ProjectSelection
	Release                     Release
	CandidateRevisionID         string
	CandidateRevision           int64
	CandidateRevisionHash       string
	CandidateContentHash        string
	Candidate                   json.RawMessage
	ProductionWorldOwnerSetHash string
	ReviewDecisionID            string
	CreatedBy                   string
	CreatedAt                   time.Time
}

type ProjectPresetBindingVersion struct {
	ID                       string    `json:"id"`
	ContractID               string    `json:"contract_id"`
	WorkspaceID              string    `json:"workspace_id"`
	ProjectID                string    `json:"project_id"`
	Revision                 int64     `json:"revision"`
	SelectionID              string    `json:"selection_id"`
	SelectionRevision        int64     `json:"selection_revision"`
	SelectionContentHash     string    `json:"selection_content_hash"`
	PresetKey                string    `json:"preset_key"`
	PresetRelease            string    `json:"preset_release"`
	PresetReleaseContentHash string    `json:"preset_release_content_hash"`
	ApplicationMode          string    `json:"application_mode"`
	TypedOverridesHash       string    `json:"typed_overrides_hash"`
	ReferenceAttachmentsHash string    `json:"reference_attachments_hash"`
	CandidateRevisionID      string    `json:"candidate_revision_id"`
	CandidateRevision        int64     `json:"candidate_revision"`
	CandidateRevisionHash    string    `json:"candidate_revision_hash"`
	CandidateContentHash     string    `json:"candidate_content_hash"`
	ReviewDecisionID         string    `json:"review_decision_id"`
	CreatedBy                string    `json:"created_by"`
	CreatedAt                time.Time `json:"created_at"`
	ContentHash              string    `json:"content_hash"`
}

type EffectiveStyleSnapshot struct {
	ID                       string                               `json:"id"`
	ContractID               string                               `json:"contract_id"`
	WorkspaceID              string                               `json:"workspace_id"`
	ProjectID                string                               `json:"project_id"`
	Revision                 int64                                `json:"revision"`
	CandidateRevisionID      string                               `json:"candidate_revision_id"`
	CandidateRevisionHash    string                               `json:"candidate_revision_hash"`
	PresetReleaseContentHash string                               `json:"preset_release_content_hash"`
	ApplicationMode          string                               `json:"application_mode"`
	VisualGrammar            VisualGrammar                        `json:"visual_grammar"`
	StylePolicy              agentcontract.VisualFoundationPolicy `json:"style_policy"`
	ReviewDecisionID         string                               `json:"review_decision_id"`
	CreatedBy                string                               `json:"created_by"`
	CreatedAt                time.Time                            `json:"created_at"`
	ContentHash              string                               `json:"content_hash"`
}

type EffectivePolicySnapshot struct {
	ID                          string                                  `json:"id"`
	ContractID                  string                                  `json:"contract_id"`
	WorkspaceID                 string                                  `json:"workspace_id"`
	ProjectID                   string                                  `json:"project_id"`
	Revision                    int64                                   `json:"revision"`
	EffectiveStyleSnapshotID    string                                  `json:"effective_style_snapshot_id"`
	EffectiveStyleSnapshotHash  string                                  `json:"effective_style_snapshot_hash"`
	ProductionWorldOwnerSetHash string                                  `json:"production_world_owner_set_hash"`
	FidelityInvariants          []string                                `json:"fidelity_invariants"`
	WorldAdaptations            []agentcontract.WorldAdaptationProposal `json:"world_adaptations"`
	WorldConflicts              []agentcontract.VisualWorldConflict     `json:"world_conflicts"`
	CreativeFillProposals       []agentcontract.CreativeFillProposal    `json:"creative_fill_proposals"`
	CapabilityManifest          []Capability                            `json:"capability_manifest"`
	PurposeProfiles             []PurposeProfile                        `json:"purpose_profiles"`
	SkillReleaseRefs            []ContentRef                            `json:"skill_release_refs"`
	QCPolicyRef                 ContentRef                              `json:"qc_policy_ref"`
	ModelCapabilityPolicyRef    ContentRef                              `json:"model_capability_policy_ref"`
	ReviewDecisionID            string                                  `json:"review_decision_id"`
	CreatedBy                   string                                  `json:"created_by"`
	CreatedAt                   time.Time                               `json:"created_at"`
	ContentHash                 string                                  `json:"content_hash"`
}

type EffectiveVisualFoundationSet struct {
	ContractID string                      `json:"contract_id"`
	Binding    ProjectPresetBindingVersion `json:"binding"`
	Style      EffectiveStyleSnapshot      `json:"style"`
	Policy     EffectivePolicySnapshot     `json:"policy"`
	Head       PresetEffectiveScopeHead    `json:"head"`
	Collection platformowner.Ref           `json:"collection"`
}

type PresetEffectiveScopeHead struct {
	ContractID                string `json:"contract_id"`
	WorkspaceID               string `json:"workspace_id"`
	ProjectID                 string `json:"project_id"`
	HeadRevision              int64  `json:"head_revision"`
	CurrentBindingVersionID   string `json:"current_binding_version_id"`
	CurrentBindingContentHash string `json:"current_binding_content_hash"`
	CurrentStyleSnapshotID    string `json:"current_style_snapshot_id"`
	CurrentStyleSnapshotHash  string `json:"current_style_snapshot_hash"`
	CurrentPolicySnapshotID   string `json:"current_policy_snapshot_id"`
	CurrentPolicySnapshotHash string `json:"current_policy_snapshot_hash"`
	CollectionRootHash        string `json:"collection_root_hash"`
	ContentHash               string `json:"content_hash"`
}

func BuildEffectiveVisualFoundationSet(draft EffectiveVisualFoundationSetDraft) (EffectiveVisualFoundationSet, error) {
	selection, release, candidate, err := validateEffectiveVisualFoundationDraft(draft)
	if err != nil {
		return EffectiveVisualFoundationSet{}, err
	}
	binding := ProjectPresetBindingVersion{
		ID: draft.BindingVersionID, ContractID: projectPresetBindingVersionContractID,
		WorkspaceID: selection.WorkspaceID, ProjectID: selection.ProjectID, Revision: draft.Revision,
		SelectionID: selection.ID, SelectionRevision: selection.Revision, SelectionContentHash: selection.ContentHash,
		PresetKey: release.Key, PresetRelease: release.Release, PresetReleaseContentHash: release.ContentHash,
		ApplicationMode: selection.ApplicationMode, TypedOverridesHash: candidate.TypedOverridesHash,
		ReferenceAttachmentsHash: candidate.ReferenceAttachmentsHash,
		CandidateRevisionID:      draft.CandidateRevisionID, CandidateRevision: draft.CandidateRevision,
		CandidateRevisionHash: draft.CandidateRevisionHash, CandidateContentHash: draft.CandidateContentHash,
		ReviewDecisionID: draft.ReviewDecisionID, CreatedBy: draft.CreatedBy, CreatedAt: draft.CreatedAt,
	}
	binding.ContentHash, err = effectiveVisualFoundationHash(binding)
	if err != nil {
		return EffectiveVisualFoundationSet{}, err
	}
	style := EffectiveStyleSnapshot{
		ID: draft.StyleSnapshotID, ContractID: effectiveStyleSnapshotContractID,
		WorkspaceID: selection.WorkspaceID, ProjectID: selection.ProjectID, Revision: draft.Revision,
		CandidateRevisionID: draft.CandidateRevisionID, CandidateRevisionHash: draft.CandidateRevisionHash,
		PresetReleaseContentHash: release.ContentHash, ApplicationMode: selection.ApplicationMode,
		VisualGrammar: release.VisualGrammar, StylePolicy: candidate.StylePolicy,
		ReviewDecisionID: draft.ReviewDecisionID, CreatedBy: draft.CreatedBy, CreatedAt: draft.CreatedAt,
	}
	style.ContentHash, err = effectiveVisualFoundationHash(style)
	if err != nil {
		return EffectiveVisualFoundationSet{}, err
	}
	policy := EffectivePolicySnapshot{
		ID: draft.PolicySnapshotID, ContractID: effectivePolicySnapshotContractID,
		WorkspaceID: selection.WorkspaceID, ProjectID: selection.ProjectID, Revision: draft.Revision,
		EffectiveStyleSnapshotID: style.ID, EffectiveStyleSnapshotHash: style.ContentHash,
		ProductionWorldOwnerSetHash: candidate.ProductionWorldOwnerSetHash,
		FidelityInvariants:          append([]string(nil), candidate.FidelityInvariants...),
		WorldAdaptations:            append([]agentcontract.WorldAdaptationProposal(nil), candidate.WorldAdaptations...),
		WorldConflicts:              append([]agentcontract.VisualWorldConflict(nil), candidate.WorldConflicts...),
		CreativeFillProposals:       append([]agentcontract.CreativeFillProposal(nil), candidate.CreativeFillProposals...),
		CapabilityManifest:          append([]Capability(nil), release.CapabilityManifest...),
		PurposeProfiles:             append([]PurposeProfile(nil), release.PurposeProfiles...),
		SkillReleaseRefs:            append([]ContentRef(nil), release.SkillReleaseRefs...),
		QCPolicyRef:                 release.QCPolicyRef, ModelCapabilityPolicyRef: release.ModelCapabilityPolicyRef,
		ReviewDecisionID: draft.ReviewDecisionID, CreatedBy: draft.CreatedBy, CreatedAt: draft.CreatedAt,
	}
	policy.ContentHash, err = effectiveVisualFoundationHash(policy)
	if err != nil {
		return EffectiveVisualFoundationSet{}, err
	}
	collection, err := platformowner.Build(platformowner.Scope{
		WorkspaceID: selection.WorkspaceID, ProjectID: selection.ProjectID,
		OwnerKind: effectiveVisualFoundationOwnerKind, VersionFamily: effectiveVisualFoundationFamily,
		ScopeKind: "project", ScopeKey: "project:" + selection.ProjectID, ScopeRevision: draft.Revision,
	}, []platformowner.VersionRef{
		effectiveVisualFoundationVersionRef(style.ID, style.Revision, style.ContentHash, selection),
		effectiveVisualFoundationVersionRef(policy.ID, policy.Revision, policy.ContentHash, selection),
	})
	if err != nil {
		return EffectiveVisualFoundationSet{}, err
	}
	head := PresetEffectiveScopeHead{
		ContractID:  "preset-effective-scope-head-production",
		WorkspaceID: selection.WorkspaceID, ProjectID: selection.ProjectID, HeadRevision: draft.Revision,
		CurrentBindingVersionID: binding.ID, CurrentBindingContentHash: binding.ContentHash,
		CurrentStyleSnapshotID: style.ID, CurrentStyleSnapshotHash: style.ContentHash,
		CurrentPolicySnapshotID: policy.ID, CurrentPolicySnapshotHash: policy.ContentHash,
		CollectionRootHash: collection.CollectionRootHash,
	}
	head.ContentHash, err = effectiveVisualFoundationHash(head)
	if err != nil {
		return EffectiveVisualFoundationSet{}, err
	}
	return EffectiveVisualFoundationSet{
		ContractID: effectiveVisualFoundationContractID, Binding: binding,
		Style: style, Policy: policy, Head: head, Collection: collection,
	}, nil
}

func validateEffectiveVisualFoundationDraft(draft EffectiveVisualFoundationSetDraft) (ProjectSelection, Release, agentcontract.VisualFoundationCandidate, error) {
	for _, identifier := range []string{
		draft.BindingVersionID, draft.StyleSnapshotID, draft.PolicySnapshotID,
		draft.CandidateRevisionID, draft.ReviewDecisionID, draft.CreatedBy,
	} {
		if parsed, err := uuid.Parse(identifier); err != nil || parsed == uuid.Nil {
			return ProjectSelection{}, Release{}, agentcontract.VisualFoundationCandidate{}, errors.New("invalid effective Visual Foundation identity")
		}
	}
	if draft.Revision < 1 || draft.CandidateRevision < 1 ||
		draft.CreatedAt.IsZero() || draft.CreatedAt.Location() != time.UTC ||
		!effectiveVisualFoundationHashPattern.MatchString(draft.CandidateRevisionHash) ||
		!effectiveVisualFoundationHashPattern.MatchString(draft.CandidateContentHash) ||
		!effectiveVisualFoundationHashPattern.MatchString(draft.ProductionWorldOwnerSetHash) {
		return ProjectSelection{}, Release{}, agentcontract.VisualFoundationCandidate{}, errors.New("invalid effective Visual Foundation lineage")
	}
	selectionRaw, err := json.Marshal(draft.Selection)
	if err != nil {
		return ProjectSelection{}, Release{}, agentcontract.VisualFoundationCandidate{}, err
	}
	selection, _, err := DecodeProjectSelection(selectionRaw)
	if err != nil || !reflect.DeepEqual(selection, draft.Selection) {
		return ProjectSelection{}, Release{}, agentcontract.VisualFoundationCandidate{}, errors.New("Project Preset selection has drifted")
	}
	releaseRaw, err := json.Marshal(draft.Release)
	if err != nil {
		return ProjectSelection{}, Release{}, agentcontract.VisualFoundationCandidate{}, err
	}
	release, _, err := DecodeRelease(releaseRaw)
	if err != nil || !reflect.DeepEqual(release, draft.Release) ||
		selection.PresetRelease.Key != release.Key || selection.PresetRelease.Release != release.Release ||
		selection.PresetRelease.ContentHash != release.ContentHash {
		return ProjectSelection{}, Release{}, agentcontract.VisualFoundationCandidate{}, errors.New("Preset release has drifted")
	}
	candidate, canonicalCandidate, err := agentcontract.DecodeVisualFoundationCandidate(draft.Candidate)
	if err != nil {
		return ProjectSelection{}, Release{}, agentcontract.VisualFoundationCandidate{}, err
	}
	candidateHash, err := platformcanonical.Hash(canonicalCandidate)
	if err != nil || candidateHash != draft.CandidateContentHash ||
		candidate.WorkspaceID != selection.WorkspaceID || candidate.ProjectID != selection.ProjectID ||
		candidate.ProductionWorldOwnerSetHash != draft.ProductionWorldOwnerSetHash ||
		candidate.PresetReleaseContentHash != release.ContentHash || candidate.ApplicationMode != selection.ApplicationMode {
		return ProjectSelection{}, Release{}, agentcontract.VisualFoundationCandidate{}, errors.New("Visual Foundation Candidate lineage has drifted")
	}
	return selection, release, candidate, nil
}

func effectiveVisualFoundationHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func effectiveVisualFoundationVersionRef(id string, revision int64, contentHash string, selection ProjectSelection) platformowner.VersionRef {
	return platformowner.VersionRef{
		WorkspaceID: selection.WorkspaceID, ProjectID: selection.ProjectID,
		OwnerKind: effectiveVisualFoundationOwnerKind, VersionFamily: effectiveVisualFoundationFamily,
		OwnerLogicalID: selection.ProjectID, OwnerVersionID: id,
		OwnerRevision: revision, OwnerContentHash: contentHash,
	}
}
