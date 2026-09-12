package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformowner "github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
)

const referenceBriefReadSetContractID = "reference-brief-read-set-production"

var ErrReferenceBriefDependenciesNotReady = errors.New("Reference Brief dependencies do not have formal AssetVersion selections")

type ReferenceBriefDependencyFact struct {
	Target                  ReferencePlanTargetVersion
	SelectedAssetVersionRef platformowner.VersionRef
}

type ReferenceBriefCompilationFacts struct {
	ApprovedPlan               ApprovedReferencePlanVersion
	Target                     ReferencePlanTargetVersion
	VisualFoundationVersionRef platformowner.VersionRef
	DependencyFacts            []ReferenceBriefDependencyFact
	StageRelease               agentcontract.ReferenceBriefStageRelease
}

type referenceBriefReadSet struct {
	ContractID                      string                                            `json:"contract_id"`
	ApprovedReferencePlanVersionRef agentcontract.ReferencePlanOwnerRef               `json:"approved_reference_plan_version_ref"`
	ReferencePlanTargetRef          agentcontract.ReferencePlanOwnerRef               `json:"reference_plan_target_ref"`
	VisualFoundationVersionRef      agentcontract.ReferencePlanOwnerRef               `json:"visual_foundation_version_ref"`
	EffectiveStyleSnapshotRef       agentcontract.ReferencePlanOwnerRef               `json:"effective_style_snapshot_ref"`
	EffectivePolicySnapshotRef      agentcontract.ReferencePlanOwnerRef               `json:"effective_policy_snapshot_ref"`
	DependencySelections            []agentcontract.ReferenceBriefDependencySelection `json:"dependency_selections"`
	SourceRefs                      agentcontract.ReferencePlanTargetOwnerRefs        `json:"source_refs"`
}

func CompileReferenceBriefInput(facts ReferenceBriefCompilationFacts) (agentcontract.ReferenceBriefInput, error) {
	if err := validateReferenceBriefCompilationFacts(facts); err != nil {
		return agentcontract.ReferenceBriefInput{}, err
	}
	planRef := referenceBriefOwnerRef(platformowner.VersionRef{
		WorkspaceID: facts.ApprovedPlan.WorkspaceID, ProjectID: facts.ApprovedPlan.ProjectID,
		OwnerKind: referencePlanOwnerKind, VersionFamily: referencePlanFamily,
		OwnerLogicalID: facts.ApprovedPlan.LogicalID, OwnerVersionID: facts.ApprovedPlan.ID,
		OwnerRevision: facts.ApprovedPlan.Revision, OwnerContentHash: facts.ApprovedPlan.ContentHash,
	})
	targetRef := referenceBriefTargetRef(facts.Target)
	visualRef := referenceBriefOwnerRef(facts.VisualFoundationVersionRef)
	styleRef := referenceBriefOwnerRef(facts.ApprovedPlan.EffectiveStyleSnapshot)
	policyRef := referenceBriefOwnerRef(facts.ApprovedPlan.EffectivePolicySnapshot)
	selections := make([]agentcontract.ReferenceBriefDependencySelection, len(facts.DependencyFacts))
	for index, dependency := range facts.DependencyFacts {
		selections[index] = agentcontract.ReferenceBriefDependencySelection{
			TargetVersionRef:        referenceBriefTargetRef(dependency.Target),
			SelectedAssetVersionRef: referenceBriefOwnerRef(dependency.SelectedAssetVersionRef),
		}
	}
	sourceRefs := cloneOwnerRefs(facts.Target.OwnerRefs)
	readSetRoot, err := referenceBriefReadSetRoot(referenceBriefReadSet{
		ContractID:                      referenceBriefReadSetContractID,
		ApprovedReferencePlanVersionRef: planRef, ReferencePlanTargetRef: targetRef,
		VisualFoundationVersionRef: visualRef, EffectiveStyleSnapshotRef: styleRef,
		EffectivePolicySnapshotRef: policyRef, DependencySelections: selections, SourceRefs: sourceRefs,
	})
	if err != nil {
		return agentcontract.ReferenceBriefInput{}, err
	}
	input := agentcontract.ReferenceBriefInput{
		WorkspaceID: facts.Target.WorkspaceID, ProjectID: facts.Target.ProjectID,
		ApprovedReferencePlanVersionRef: planRef, ReferencePlanTargetRef: targetRef,
		TargetBusinessKey: facts.Target.TargetBusinessKey, TargetKind: facts.Target.TargetKind,
		TargetFulfillment:          facts.Target.Fulfillment,
		VisualFoundationVersionRef: visualRef, EffectiveStyleSnapshotRef: styleRef,
		EffectivePolicySnapshotRef: policyRef, DependencySelections: selections,
		StageRelease: facts.StageRelease, TypedReadSetRoot: readSetRoot, SourceRefs: sourceRefs,
		DesignFocus:       append([]string(nil), facts.Target.Constraints.DesignFocus...),
		ForbiddenChanges:  append([]string(nil), facts.Target.Constraints.ForbiddenChanges...),
		RequiredViewRoles: agentcontract.ReferenceBriefRequiredViewRoles(facts.Target.TargetKind),
	}
	if err = input.Validate(); err != nil {
		return agentcontract.ReferenceBriefInput{}, err
	}
	return input, nil
}

func validateReferenceBriefCompilationFacts(facts ReferenceBriefCompilationFacts) error {
	plan, target := facts.ApprovedPlan, facts.Target
	if plan.ContractID != approvedReferencePlanContractID || target.ContractID != referencePlanTargetContractID ||
		uuid.Validate(plan.ID) != nil || uuid.Validate(plan.LogicalID) != nil ||
		uuid.Validate(plan.WorkspaceID) != nil || uuid.Validate(plan.ProjectID) != nil ||
		plan.Revision < 1 || !referencePlanHashPattern.MatchString(plan.ContentHash) ||
		target.WorkspaceID != plan.WorkspaceID || target.ProjectID != plan.ProjectID ||
		target.PlanLogicalID != plan.LogicalID || target.PlanVersionID != plan.ID || target.Revision != plan.Revision ||
		target.ReviewDecisionID != plan.ReviewDecisionID || target.Fulfillment == "not_generated" ||
		target.Constraints.ProductionWorldOwnerSetHash != plan.ProductionWorldOwnerSetHash ||
		target.Constraints.ReferenceTargetSeedRoot != plan.ReferenceTargetSeedRoot ||
		!reflect.DeepEqual(target.EffectiveStyleSnapshot, plan.EffectiveStyleSnapshot) ||
		!reflect.DeepEqual(target.EffectivePolicySnapshot, plan.EffectivePolicySnapshot) ||
		!referenceBriefPlanContains(plan, referenceBriefTargetVersionRef(target)) ||
		validateReferenceBriefVisualRef(facts.VisualFoundationVersionRef, plan) != nil {
		return errors.New("invalid Reference Brief approved facts")
	}
	if len(facts.DependencyFacts) != len(target.DependsOnTargetBusinessKeys) {
		return errors.New("Reference Brief selected dependency set is incomplete")
	}
	for index, dependency := range facts.DependencyFacts {
		if dependency.Target.ContractID != referencePlanTargetContractID ||
			dependency.Target.TargetBusinessKey != target.DependsOnTargetBusinessKeys[index] ||
			dependency.Target.WorkspaceID != plan.WorkspaceID || dependency.Target.ProjectID != plan.ProjectID ||
			dependency.Target.PlanLogicalID != plan.LogicalID || dependency.Target.PlanVersionID != plan.ID ||
			dependency.Target.Revision != plan.Revision || dependency.Target.Fulfillment == "not_generated" ||
			!referenceBriefPlanContains(plan, referenceBriefTargetVersionRef(dependency.Target)) ||
			validateReferenceBriefAssetVersionRef(dependency.SelectedAssetVersionRef, plan) != nil {
			return errors.New("invalid Reference Brief selected dependency fact")
		}
		if index > 0 && facts.DependencyFacts[index-1].Target.TargetBusinessKey >= dependency.Target.TargetBusinessKey {
			return errors.New("Reference Brief selected dependencies are not sorted and unique")
		}
	}
	return nil
}

func validateReferenceBriefVisualRef(value platformowner.VersionRef, plan ApprovedReferencePlanVersion) error {
	if value.WorkspaceID != plan.WorkspaceID || value.ProjectID != plan.ProjectID ||
		value.OwnerKind != "preset" || value.VersionFamily != "preset_effective_set" ||
		value.OwnerLogicalID != plan.ProjectID || value.OwnerRevision != plan.Revision ||
		uuid.Validate(value.OwnerVersionID) != nil || !referencePlanHashPattern.MatchString(value.OwnerContentHash) ||
		slices.Contains([]string{plan.EffectiveStyleSnapshot.OwnerVersionID, plan.EffectivePolicySnapshot.OwnerVersionID}, value.OwnerVersionID) {
		return errors.New("invalid Reference Brief Visual Foundation Version ref")
	}
	return nil
}

func validateReferenceBriefAssetVersionRef(value platformowner.VersionRef, plan ApprovedReferencePlanVersion) error {
	if value.WorkspaceID != plan.WorkspaceID || value.ProjectID != plan.ProjectID ||
		value.OwnerKind != "asset" || value.VersionFamily != "asset_base_reference_set" ||
		value.OwnerLogicalID == "" || uuid.Validate(value.OwnerVersionID) != nil || value.OwnerRevision < 1 ||
		!referencePlanHashPattern.MatchString(value.OwnerContentHash) {
		return errors.New("invalid Reference Brief selected Asset Version ref")
	}
	return nil
}

func referenceBriefPlanContains(plan ApprovedReferencePlanVersion, expected platformowner.VersionRef) bool {
	return slices.ContainsFunc(plan.TargetVersionRefs, func(value platformowner.VersionRef) bool {
		return reflect.DeepEqual(value, expected)
	})
}

func referenceBriefTargetVersionRef(target ReferencePlanTargetVersion) platformowner.VersionRef {
	return platformowner.VersionRef{
		WorkspaceID: target.WorkspaceID, ProjectID: target.ProjectID,
		OwnerKind: referencePlanOwnerKind, VersionFamily: referencePlanFamily,
		OwnerLogicalID: target.TargetBusinessKey, OwnerVersionID: target.ID,
		OwnerRevision: target.Revision, OwnerContentHash: target.ContentHash,
	}
}

func referenceBriefTargetRef(target ReferencePlanTargetVersion) agentcontract.ReferencePlanOwnerRef {
	return referenceBriefOwnerRef(referenceBriefTargetVersionRef(target))
}

func referenceBriefOwnerRef(value platformowner.VersionRef) agentcontract.ReferencePlanOwnerRef {
	return agentcontract.ReferencePlanOwnerRef{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID,
		OwnerKind: value.OwnerKind, VersionFamily: value.VersionFamily,
		OwnerLogicalID: value.OwnerLogicalID, OwnerVersionID: value.OwnerVersionID,
		OwnerRevision: value.OwnerRevision, OwnerContentHash: value.OwnerContentHash,
	}
}

func referenceBriefReadSetRoot(value referenceBriefReadSet) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}
