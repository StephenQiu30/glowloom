package workflow_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformowner "github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
	presetdomain "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
	referencedomain "github.com/StephenQiu30/lanverse/backend/internal/production/reference/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
)

func TestVisualReferenceOwnerContractsFreezeEffectiveSnapshotsAndExactTargets(t *testing.T) {
	draft := visualFoundationScopeSubjectDraft(t)
	createdAt := time.Date(2026, time.September, 12, 2, 0, 0, 0, time.UTC)
	decisionID, actorID := uuid.NewString(), uuid.NewString()
	visualSet, err := presetdomain.BuildEffectiveVisualFoundationSet(presetdomain.EffectiveVisualFoundationSetDraft{
		BindingVersionID: uuid.NewString(), StyleSnapshotID: uuid.NewString(), PolicySnapshotID: uuid.NewString(),
		Revision: 1, Selection: draft.ProjectPresetSelection, Release: draft.PresetRelease,
		CandidateRevisionID:         draft.VisualFoundationCandidate.RevisionID,
		CandidateRevision:           draft.VisualFoundationCandidate.Revision,
		CandidateRevisionHash:       draft.VisualFoundationCandidate.RevisionHash,
		CandidateContentHash:        draft.VisualFoundationCandidate.ContentHash,
		Candidate:                   draft.VisualFoundationCandidate.Candidate,
		ProductionWorldOwnerSetHash: draft.ReferencePlanInput.ProductionWorldOwnerSetHash,
		ReviewDecisionID:            decisionID, CreatedBy: actorID, CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if visualSet.Binding.SelectionID != draft.ProjectPresetSelection.ID ||
		visualSet.Binding.PresetReleaseContentHash != draft.PresetRelease.ContentHash ||
		visualSet.Style.CandidateRevisionID != draft.VisualFoundationCandidate.RevisionID ||
		visualSet.Policy.EffectiveStyleSnapshotID != visualSet.Style.ID ||
		visualSet.Policy.EffectiveStyleSnapshotHash != visualSet.Style.ContentHash ||
		len(visualSet.Binding.ContentHash) != 64 || len(visualSet.Style.ContentHash) != 64 ||
		len(visualSet.Policy.ContentHash) != 64 || len(visualSet.Collection.CollectionRootHash) != 64 ||
		len(visualSet.Collection.Members) != 2 ||
		visualSet.Head.CurrentBindingVersionID != visualSet.Binding.ID ||
		visualSet.Head.CurrentStyleSnapshotHash != visualSet.Style.ContentHash ||
		visualSet.Head.CurrentPolicySnapshotHash != visualSet.Policy.ContentHash ||
		visualSet.Head.CollectionRootHash != visualSet.Collection.CollectionRootHash {
		t.Fatalf("effective Visual Foundation set is incomplete: %#v", visualSet)
	}

	projection, err := workflowapp.BuildReferencePlanCandidateProjection(
		draft.ReferencePlanInput,
		draft.ReferencePlanCandidate.Candidate,
	)
	if err != nil {
		t.Fatal(err)
	}
	targets := make([]referencedomain.TargetDraft, len(projection.Targets))
	for index, target := range projection.Targets {
		targets[index] = referencedomain.TargetDraft{
			VersionID: uuid.NewString(), TargetBusinessKey: target.TargetBusinessKey,
			TargetKind: target.TargetKind, Fulfillment: target.Fulfillment,
			OwnerRefs: referenceOwnerRefs(target.OwnerRefs), CoverageScopeKeys: target.CoverageScopeKeys,
			DependsOnTargetBusinessKeys: target.DependsOnTargetBusinessKeys,
			Constraints:                 referenceConstraints(target.Constraints),
		}
	}
	plan, err := referencedomain.BuildApprovedReferencePlan(referencedomain.ApprovedReferencePlanDraft{
		PlanLogicalID: uuid.NewString(), PlanVersionID: uuid.NewString(), Revision: 1,
		WorkspaceID:                           draft.ConfirmedProductionWorld.WorkspaceID,
		ProjectID:                             draft.ConfirmedProductionWorld.ProjectID,
		CandidateRevisionID:                   draft.ReferencePlanCandidate.RevisionID,
		CandidateRevision:                     draft.ReferencePlanCandidate.Revision,
		CandidateRevisionHash:                 draft.ReferencePlanCandidate.RevisionHash,
		CandidateContentHash:                  draft.ReferencePlanCandidate.ContentHash,
		VisualFoundationCandidateRevisionID:   draft.VisualFoundationCandidate.RevisionID,
		VisualFoundationCandidateRevisionHash: draft.VisualFoundationCandidate.RevisionHash,
		PresetReleaseContentHash:              draft.PresetRelease.ContentHash,
		ProductionWorldOwnerSetHash:           draft.ReferencePlanInput.ProductionWorldOwnerSetHash,
		ReferenceTargetSeedRoot:               draft.ReferencePlanInput.ReferenceTargetSeedRoot,
		ExpectedTargetSet:                     projection.ExpectedTargetSet,
		EffectiveStyleSnapshot: platformowner.VersionRef{
			WorkspaceID: draft.ConfirmedProductionWorld.WorkspaceID,
			ProjectID:   draft.ConfirmedProductionWorld.ProjectID, OwnerKind: "preset",
			VersionFamily: "preset_effective_set", OwnerLogicalID: draft.ConfirmedProductionWorld.ProjectID,
			OwnerVersionID: visualSet.Style.ID, OwnerRevision: visualSet.Style.Revision,
			OwnerContentHash: visualSet.Style.ContentHash,
		},
		EffectivePolicySnapshot: platformowner.VersionRef{
			WorkspaceID: draft.ConfirmedProductionWorld.WorkspaceID,
			ProjectID:   draft.ConfirmedProductionWorld.ProjectID, OwnerKind: "preset",
			VersionFamily: "preset_effective_set", OwnerLogicalID: draft.ConfirmedProductionWorld.ProjectID,
			OwnerVersionID: visualSet.Policy.ID, OwnerRevision: visualSet.Policy.Revision,
			OwnerContentHash: visualSet.Policy.ContentHash,
		},
		Targets: targets, ReviewDecisionID: decisionID, CreatedBy: actorID, CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Version.CandidateRevisionID != draft.ReferencePlanCandidate.RevisionID ||
		plan.Version.ExpectedTargetKeyRoot != projection.ExpectedTargetSet.ExpectedTargetKeyRoot ||
		len(plan.Targets) != len(projection.Targets) || len(plan.Collection.Members) != len(plan.Targets)+1 ||
		plan.ScopeHead.CurrentPlanVersionID != plan.Version.ID ||
		plan.ActivationHead.CurrentPlanVersionID != plan.Version.ID ||
		plan.ScopeHead.CurrentPlanContentHash != plan.Version.ContentHash ||
		plan.ActivationHead.CurrentPlanContentHash != plan.Version.ContentHash {
		t.Fatalf("approved Reference Plan is incomplete: %#v", plan)
	}
	for index := range plan.Targets {
		if plan.Targets[index].TargetBusinessKey != projection.Targets[index].TargetBusinessKey ||
			!reflect.DeepEqual(plan.Targets[index].OwnerRefs, referenceOwnerRefs(projection.Targets[index].OwnerRefs)) ||
			len(plan.Targets[index].ContentHash) != 64 {
			t.Fatalf("approved Reference Target %d drifted: %#v", index, plan.Targets[index])
		}
	}
}

func TestVisualReferenceOwnerContractsRejectCandidateAndTargetDrift(t *testing.T) {
	draft := visualFoundationScopeSubjectDraft(t)
	invalidCandidate := append(json.RawMessage(nil), draft.VisualFoundationCandidate.Candidate...)
	var candidate agentcontract.VisualFoundationCandidate
	if err := json.Unmarshal(invalidCandidate, &candidate); err != nil {
		t.Fatal(err)
	}
	candidate.PresetReleaseContentHash = strings.Repeat("f", 64)
	invalidCandidate, _ = json.Marshal(candidate)
	if _, err := presetdomain.BuildEffectiveVisualFoundationSet(presetdomain.EffectiveVisualFoundationSetDraft{
		BindingVersionID: uuid.NewString(), StyleSnapshotID: uuid.NewString(), PolicySnapshotID: uuid.NewString(),
		Revision: 1, Selection: draft.ProjectPresetSelection, Release: draft.PresetRelease,
		CandidateRevisionID:   draft.VisualFoundationCandidate.RevisionID,
		CandidateRevision:     draft.VisualFoundationCandidate.Revision,
		CandidateRevisionHash: draft.VisualFoundationCandidate.RevisionHash,
		CandidateContentHash:  draft.VisualFoundationCandidate.ContentHash, Candidate: invalidCandidate,
		ProductionWorldOwnerSetHash: draft.ReferencePlanInput.ProductionWorldOwnerSetHash,
		ReviewDecisionID:            uuid.NewString(), CreatedBy: uuid.NewString(), CreatedAt: time.Now().UTC(),
	}); err == nil {
		t.Fatal("effective Visual Foundation owner accepted Candidate/Preset drift")
	}

	projection, err := workflowapp.BuildReferencePlanCandidateProjection(
		draft.ReferencePlanInput, draft.ReferencePlanCandidate.Candidate,
	)
	if err != nil {
		t.Fatal(err)
	}
	target := projection.Targets[0]
	if _, err = referencedomain.BuildApprovedReferencePlan(referencedomain.ApprovedReferencePlanDraft{
		PlanLogicalID: uuid.NewString(), PlanVersionID: uuid.NewString(), Revision: 1,
		WorkspaceID: draft.ConfirmedProductionWorld.WorkspaceID, ProjectID: draft.ConfirmedProductionWorld.ProjectID,
		CandidateRevisionID:                   draft.ReferencePlanCandidate.RevisionID,
		CandidateRevision:                     draft.ReferencePlanCandidate.Revision,
		CandidateRevisionHash:                 draft.ReferencePlanCandidate.RevisionHash,
		CandidateContentHash:                  draft.ReferencePlanCandidate.ContentHash,
		VisualFoundationCandidateRevisionID:   draft.VisualFoundationCandidate.RevisionID,
		VisualFoundationCandidateRevisionHash: draft.VisualFoundationCandidate.RevisionHash,
		PresetReleaseContentHash:              draft.PresetRelease.ContentHash,
		ProductionWorldOwnerSetHash:           draft.ReferencePlanInput.ProductionWorldOwnerSetHash,
		ReferenceTargetSeedRoot:               draft.ReferencePlanInput.ReferenceTargetSeedRoot,
		ExpectedTargetSet:                     projection.ExpectedTargetSet,
		EffectiveStyleSnapshot:                validPresetOwnerRef(t, draft.ConfirmedProductionWorld.WorkspaceID, draft.ConfirmedProductionWorld.ProjectID, "a"),
		EffectivePolicySnapshot:               validPresetOwnerRef(t, draft.ConfirmedProductionWorld.WorkspaceID, draft.ConfirmedProductionWorld.ProjectID, "b"),
		Targets: []referencedomain.TargetDraft{{
			VersionID: uuid.NewString(), TargetBusinessKey: target.TargetBusinessKey,
			TargetKind: target.TargetKind, Fulfillment: target.Fulfillment, OwnerRefs: referenceOwnerRefs(target.OwnerRefs),
			CoverageScopeKeys: target.CoverageScopeKeys, DependsOnTargetBusinessKeys: target.DependsOnTargetBusinessKeys,
			Constraints: referenceConstraints(target.Constraints),
		}},
		ReviewDecisionID: uuid.NewString(), CreatedBy: uuid.NewString(), CreatedAt: time.Now().UTC(),
	}); err == nil {
		t.Fatal("Approved Reference Plan accepted an incomplete expected Target set")
	}
}

func validPresetOwnerRef(t *testing.T, workspaceID, projectID, hashCharacter string) platformowner.VersionRef {
	t.Helper()
	return platformowner.VersionRef{
		WorkspaceID: workspaceID, ProjectID: projectID, OwnerKind: "preset",
		VersionFamily: "preset_effective_set", OwnerLogicalID: projectID,
		OwnerVersionID: uuid.NewString(), OwnerRevision: 1, OwnerContentHash: strings.Repeat(hashCharacter, 64),
	}
}

func referenceOwnerRefs(value workflowapp.ReferencePlanProjectedOwnerRefs) agentcontract.ReferencePlanTargetOwnerRefs {
	return agentcontract.ReferencePlanTargetOwnerRefs{
		Identity:      append([]agentcontract.ReferencePlanOwnerRef{}, value.Identity...),
		Specification: append([]agentcontract.ReferencePlanOwnerRef{}, value.Specification...),
		State:         append([]agentcontract.ReferencePlanOwnerRef{}, value.State...),
		Scene:         append([]agentcontract.ReferencePlanOwnerRef{}, value.Scene...),
		Occurrence:    append([]agentcontract.ReferencePlanOwnerRef{}, value.Occurrence...),
		Interaction:   append([]agentcontract.ReferencePlanOwnerRef{}, value.Interaction...),
	}
}

func referenceConstraints(value workflowapp.ReferencePlanCandidateConstraintProjection) referencedomain.TargetConstraints {
	return referencedomain.TargetConstraints{
		ProductionWorldOwnerSetHash:           value.ProductionWorldOwnerSetHash,
		ReferenceTargetSeedRoot:               value.ReferenceTargetSeedRoot,
		VisualFoundationCandidateRevisionID:   value.VisualFoundationCandidateRevisionID,
		VisualFoundationCandidateRevisionHash: value.VisualFoundationCandidateRevisionHash,
		PresetReleaseContentHash:              value.PresetReleaseContentHash,
		DesignFocus:                           value.DesignFocus, ForbiddenChanges: value.ForbiddenChanges,
	}
}
