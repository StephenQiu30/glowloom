package application

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	preset "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
)

type BaseVisionReviewCompilationFacts struct {
	Target               ReferenceGenerationTarget
	Brief                agentapp.AcceptedReferenceBrief
	Style                preset.EffectiveStyleSnapshot
	Policy               preset.EffectivePolicySnapshot
	Bundles              domain.ReferenceBundleInputCollection
	Media                []domain.ReferenceStagedMedia
	CandidateBundleIndex int
	StageReleaseHash     string
}

// CompileBaseVisionReviewInput binds exact immutable facts, not mutable Heads or
// actor permissions. The caller must load and revalidate those in one transaction.
func CompileBaseVisionReviewInput(facts BaseVisionReviewCompilationFacts) (contract.VisionReviewInput, error) {
	target, brief, style, policy := facts.Target, facts.Brief, facts.Style, facts.Policy
	raw, err := json.Marshal(target)
	if err != nil {
		return contract.VisionReviewInput{}, err
	}
	if _, err := DecodeReferenceGenerationTarget(raw); err != nil {
		return contract.VisionReviewInput{}, err
	}
	if brief.Candidate.ValidateFor(brief.Input) != nil || target.ReferenceBriefRevisionRef != (ReferenceBriefRevisionRef{brief.RevisionID, brief.Revision, brief.RevisionHash, brief.ContentHash}) ||
		target.WorkspaceID != brief.Input.WorkspaceID || target.ProjectID != brief.Input.ProjectID || target.TargetKind != brief.Input.TargetKind || target.TargetBusinessKey != brief.Input.TargetBusinessKey ||
		!reflect.DeepEqual(target.ApprovedReferencePlanVersionRef, brief.Input.ApprovedReferencePlanVersionRef) || !reflect.DeepEqual(target.ReferencePlanTargetRef, brief.Input.ReferencePlanTargetRef) ||
		!reflect.DeepEqual(target.EffectiveStyleSnapshotRef, brief.Input.EffectiveStyleSnapshotRef) || !reflect.DeepEqual(target.EffectivePolicySnapshotRef, brief.Input.EffectivePolicySnapshotRef) || len(brief.Input.DependencySelections) != 0 {
		return contract.VisionReviewInput{}, errors.New("Vision Review accepted Brief or Target has drifted")
	}
	styleRef, policyRef := brief.Input.EffectiveStyleSnapshotRef, brief.Input.EffectivePolicySnapshotRef
	if style.ContractID != "effective-style-snapshot-production" || policy.ContractID != "effective-policy-snapshot-production" ||
		style.ID != styleRef.OwnerVersionID || style.Revision != styleRef.OwnerRevision || style.ContentHash != styleRef.OwnerContentHash ||
		policy.ID != policyRef.OwnerVersionID || policy.Revision != policyRef.OwnerRevision || policy.ContentHash != policyRef.OwnerContentHash ||
		style.WorkspaceID != target.WorkspaceID || policy.WorkspaceID != target.WorkspaceID || style.ProjectID != target.ProjectID || policy.ProjectID != target.ProjectID ||
		policy.EffectiveStyleSnapshotID != style.ID || policy.EffectiveStyleSnapshotHash != style.ContentHash ||
		validateVisionReviewSnapshotHash(style, style.ContentHash) != nil || validateVisionReviewSnapshotHash(policy, policy.ContentHash) != nil {
		return contract.VisionReviewInput{}, errors.New("Vision Review effective visual snapshots have drifted")
	}
	if preset.ValidateCapabilityManifest(policy.CapabilityManifest) != nil || !slices.ContainsFunc(policy.CapabilityManifest, func(value preset.Capability) bool {
		return value.TargetKind == target.TargetKind && slices.Equal(value.ViewRoles, brief.Input.RequiredViewRoles)
	}) {
		return contract.VisionReviewInput{}, errors.New("Vision Review target purpose is unsupported")
	}
	var profiles []preset.PurposeProfile
	for _, profile := range policy.PurposeProfiles {
		if profile.TargetKind == target.TargetKind {
			profiles = append(profiles, profile)
		}
	}
	if len(profiles) != 1 {
		return contract.VisionReviewInput{}, errors.New("Vision Review purpose profile is missing or ambiguous")
	}
	raw, err = json.Marshal(facts.Bundles)
	if err != nil {
		return contract.VisionReviewInput{}, err
	}
	bundles, err := domain.DecodeReferenceBundleInputs(raw)
	if err != nil {
		return contract.VisionReviewInput{}, err
	}
	if facts.CandidateBundleIndex < 0 || facts.CandidateBundleIndex >= len(bundles.Bundles) || len(bundles.Bundles) != target.OutputContract.CandidateBundleCount ||
		bundles.TargetRef != (domain.GenerationRevisionRef{ID: target.ID, Revision: target.Revision, ContentHash: target.ContentHash}) {
		return contract.VisionReviewInput{}, errors.New("Vision Review Bundle Target has drifted")
	}
	bundle := bundles.Bundles[facts.CandidateBundleIndex]
	if bundle.Admission.ValidateInternalReview() != nil || bundle.Input.GenerationRound != target.GenerationRound || bundle.Input.DependencyRootHash != target.DependencyRootHash ||
		bundle.Input.OutputContractRef != (domain.GenerationContractRef{ContractID: target.OutputContract.ContractID, ContentHash: target.OutputContract.ContentHash}) {
		return contract.VisionReviewInput{}, errors.New("Vision Review Bundle is not ready")
	}
	subject := contract.VisionReviewSubject{
		WorkspaceID: target.WorkspaceID, ProjectID: target.ProjectID, TargetKind: target.TargetKind, GenerationRound: target.GenerationRound,
		TargetRef: bundles.TargetRef, ExecutionRef: bundles.ExecutionRef, CandidateBundleIndex: facts.CandidateBundleIndex,
		BundleInputRef:  domain.GenerationActionRef{ID: bundle.Input.ID, ContentHash: bundle.Input.ContentHash},
		BriefRevisionID: brief.RevisionID, BriefRevisionHash: brief.RevisionHash, StageReleaseHash: facts.StageReleaseHash, InputHash: strings.Repeat("0", 64),
	}
	for _, slot := range bundle.Input.Slots {
		if slot.MediaRef == nil {
			return contract.VisionReviewInput{}, errors.New("Vision Review Bundle media is missing")
		}
		subject.Slots = append(subject.Slots, contract.VisionReviewSlot{SlotKey: slot.SlotKey, ViewRole: slot.ViewRole, MediaRef: *slot.MediaRef, SHA256: slot.MediaSHA256})
	}
	attachments, err := contract.BuildVisionReviewAttachments(subject, facts.Media)
	if err != nil {
		return contract.VisionReviewInput{}, err
	}
	return contract.BuildVisionReviewInput(contract.VisionReviewInput{
		Subject: subject, BriefInput: brief.Input, BriefCandidate: brief.Candidate, BriefContentHash: brief.ContentHash, Admission: bundle.Admission, Attachments: attachments,
		VisualContext: contract.VisionReviewVisualContext{
			ApplicationMode: style.ApplicationMode, ProductionWorldOwnerSetHash: policy.ProductionWorldOwnerSetHash,
			VisualGrammar: contract.VisionReviewVisualGrammar(style.VisualGrammar), StylePolicy: style.StylePolicy,
			FidelityInvariants:    slices.Clone(policy.FidelityInvariants),
			WorldAdaptations:      append([]contract.WorldAdaptationProposal{}, policy.WorldAdaptations...),
			WorldConflicts:        append([]contract.VisualWorldConflict{}, policy.WorldConflicts...),
			CreativeFillProposals: append([]contract.CreativeFillProposal{}, policy.CreativeFillProposals...),
			PurposeProfile:        contract.ReferencePlanPurposeProfile(profiles[0]), QCPolicyRef: contract.VisionReviewPolicyRef(policy.QCPolicyRef),
		},
	})
}

func validateVisionReviewSnapshotHash(value any, expected string) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var material map[string]json.RawMessage
	if err = json.Unmarshal(raw, &material); err != nil {
		return err
	}
	material["content_hash"] = json.RawMessage(`""`)
	raw, err = json.Marshal(material)
	if err != nil {
		return err
	}
	hash, err := canonical.Hash(raw)
	if err != nil || hash != expected {
		return errors.New("Vision Review snapshot content has drifted")
	}
	return nil
}
