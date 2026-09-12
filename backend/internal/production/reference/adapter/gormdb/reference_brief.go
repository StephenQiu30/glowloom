package gormdb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	platformowner "github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
	presetdomain "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
	referencedomain "github.com/StephenQiu30/lanverse/backend/internal/production/reference/domain"
)

func (store *Store) CompileReferenceBriefInput(
	ctx context.Context,
	workspaceID string,
	projectID string,
	targetBusinessKey string,
	stageRelease agentcontract.ReferenceBriefStageRelease,
) (agentcontract.ReferenceBriefInput, error) {
	if store == nil || store.database == nil {
		return agentcontract.ReferenceBriefInput{}, errors.New("Reference Brief facts store is unavailable")
	}
	workspace, workspaceErr := uuid.Parse(workspaceID)
	project, projectErr := uuid.Parse(projectID)
	if workspaceErr != nil || projectErr != nil || workspace == uuid.Nil || project == uuid.Nil || targetBusinessKey == "" {
		return agentcontract.ReferenceBriefInput{}, errors.New("invalid Reference Brief facts scope")
	}

	var compiled agentcontract.ReferenceBriefInput
	err := platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		plan, loadErr := loadCurrentReferenceBriefPlan(ctx, transaction, workspace, project)
		if loadErr != nil {
			return loadErr
		}
		target, loadErr := loadReferenceBriefTarget(
			ctx, transaction, workspace, project, plan, targetBusinessKey,
		)
		if loadErr != nil {
			return loadErr
		}
		if len(target.DependsOnTargetBusinessKeys) != 0 {
			return referencedomain.ErrReferenceBriefDependenciesNotReady
		}
		visualFoundationRef, styleRef, policyRef, loadErr := loadCurrentReferenceBriefPresetFacts(
			ctx, transaction, workspace, project, plan,
		)
		if loadErr != nil {
			return loadErr
		}
		if !reflect.DeepEqual(plan.EffectiveStyleSnapshot, styleRef) ||
			!reflect.DeepEqual(plan.EffectivePolicySnapshot, policyRef) {
			return errors.New("Reference Brief effective snapshot facts have drifted")
		}
		compiled, loadErr = compileBaseReferenceBriefInput(
			plan, target, visualFoundationRef, styleRef, policyRef, stageRelease,
		)
		return loadErr
	})
	return compiled, err
}

func (store *Store) CompileBaseReferenceBriefInputs(
	ctx context.Context,
	workspaceID string,
	projectID string,
	ownerReceiptID string,
	ownerReceiptHash string,
	stageRelease agentcontract.ReferenceBriefStageRelease,
) ([]agentcontract.ReferenceBriefInput, error) {
	if store == nil || store.database == nil {
		return nil, errors.New("Reference Brief facts store is unavailable")
	}
	workspace, workspaceErr := uuid.Parse(workspaceID)
	project, projectErr := uuid.Parse(projectID)
	receiptID, receiptErr := uuid.Parse(ownerReceiptID)
	if workspaceErr != nil || projectErr != nil || receiptErr != nil || workspace == uuid.Nil ||
		project == uuid.Nil || receiptID == uuid.Nil || len(ownerReceiptHash) != 64 {
		return nil, errors.New("invalid Reference Brief base wave scope")
	}

	var compiled []agentcontract.ReferenceBriefInput
	err := platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		plan, loadErr := loadCurrentReferenceBriefPlan(ctx, transaction, workspace, project)
		if loadErr != nil {
			return loadErr
		}
		if loadErr = validateReferenceBriefOwnerReceipt(
			ctx, transaction, workspace, project, receiptID, ownerReceiptHash, plan,
		); loadErr != nil {
			return loadErr
		}
		visualFoundationRef, styleRef, policyRef, loadErr := loadCurrentReferenceBriefPresetFacts(
			ctx, transaction, workspace, project, plan,
		)
		if loadErr != nil {
			return loadErr
		}
		if !reflect.DeepEqual(plan.EffectiveStyleSnapshot, styleRef) ||
			!reflect.DeepEqual(plan.EffectivePolicySnapshot, policyRef) {
			return errors.New("Reference Brief effective snapshot facts have drifted")
		}
		for _, targetRef := range plan.TargetVersionRefs {
			target, targetErr := loadReferenceBriefTarget(
				ctx, transaction, workspace, project, plan, targetRef.OwnerLogicalID,
			)
			if targetErr != nil {
				return targetErr
			}
			if target.ID != targetRef.OwnerVersionID || target.Revision != targetRef.OwnerRevision ||
				target.ContentHash != targetRef.OwnerContentHash {
				return errors.New("Reference Plan Target ref has drifted")
			}
			if len(target.DependsOnTargetBusinessKeys) != 0 || target.Fulfillment == "not_generated" {
				continue
			}
			input, compileErr := compileBaseReferenceBriefInput(
				plan, target, visualFoundationRef, styleRef, policyRef, stageRelease,
			)
			if compileErr != nil {
				return compileErr
			}
			compiled = append(compiled, input)
		}
		if len(compiled) == 0 {
			return errors.New("Reference Brief base wave is empty")
		}
		sort.Slice(compiled, func(left, right int) bool {
			return compiled[left].TargetBusinessKey < compiled[right].TargetBusinessKey
		})
		return nil
	})
	return compiled, err
}

func compileBaseReferenceBriefInput(
	plan referencedomain.ApprovedReferencePlanVersion,
	target referencedomain.ReferencePlanTargetVersion,
	visualFoundationRef platformowner.VersionRef,
	styleRef platformowner.VersionRef,
	policyRef platformowner.VersionRef,
	stageRelease agentcontract.ReferenceBriefStageRelease,
) (agentcontract.ReferenceBriefInput, error) {
	if !reflect.DeepEqual(plan.EffectiveStyleSnapshot, styleRef) ||
		!reflect.DeepEqual(plan.EffectivePolicySnapshot, policyRef) ||
		len(target.DependsOnTargetBusinessKeys) != 0 {
		return agentcontract.ReferenceBriefInput{}, errors.New("invalid Reference Brief base facts")
	}
	return referencedomain.CompileReferenceBriefInput(referencedomain.ReferenceBriefCompilationFacts{
		ApprovedPlan: plan, Target: target, VisualFoundationVersionRef: visualFoundationRef,
		DependencyFacts: []referencedomain.ReferenceBriefDependencyFact{}, StageRelease: stageRelease,
	})
}

func validateReferenceBriefOwnerReceipt(
	ctx context.Context,
	database *gorm.DB,
	workspaceID uuid.UUID,
	projectID uuid.UUID,
	receiptID uuid.UUID,
	receiptHash string,
	plan referencedomain.ApprovedReferencePlanVersion,
) error {
	var receipt model.CommandReceipt
	if err := database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Where(
		"id = ? AND workspace_id = ? AND operation = ?",
		receiptID, workspaceID, referencedomain.ConfirmVisualFoundationOperation,
	).First(&receipt).Error; err != nil {
		return err
	}
	result, err := decodeStoredReferenceBriefFact[referencedomain.ConfirmVisualFoundationResult](receipt.Result)
	if err != nil {
		return err
	}
	rebuilt, err := referencedomain.CompleteConfirmVisualFoundationResult(result)
	if err != nil || !reflect.DeepEqual(rebuilt, result) {
		return errors.New("Reference Brief Gate 3 owner receipt content has drifted")
	}
	if result.CommandReceiptID != receipt.ID.String() || result.CommittedBy != receipt.CreatedBy.String() ||
		!result.CommittedAt.Equal(receipt.CreatedAt) {
		return errors.New("Reference Brief Gate 3 owner receipt identity has drifted")
	}
	if result.PlanVersionID != plan.ID || result.PlanRevision != plan.Revision ||
		result.PlanContentHash != plan.ContentHash || receipt.ResourceID.String() != plan.ID {
		return errors.New("Reference Brief Gate 3 owner receipt Plan has drifted")
	}
	if result.ReceiptContentHash != receiptHash {
		return errors.New("Reference Brief Gate 3 owner receipt hash has drifted")
	}
	if result.ReferenceCollectionReceipt.WorkspaceID != workspaceID.String() ||
		result.ReferenceCollectionReceipt.ProjectID != projectID.String() {
		return errors.New("Reference Brief Gate 3 owner receipt scope has drifted")
	}
	return nil
}

func ValidateCurrentReferenceBriefInput(
	ctx context.Context,
	database *gorm.DB,
	input agentcontract.ReferenceBriefInput,
) error {
	if input.Validate() != nil {
		return errors.New("invalid Reference Brief input")
	}
	rebuilt, err := NewStore(database).CompileReferenceBriefInput(
		ctx,
		input.WorkspaceID,
		input.ProjectID,
		input.TargetBusinessKey,
		input.StageRelease,
	)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(rebuilt, input) {
		return errors.New("Reference Brief input has drifted from current facts")
	}
	return nil
}

func loadCurrentReferenceBriefPlan(
	ctx context.Context,
	database *gorm.DB,
	workspaceID uuid.UUID,
	projectID uuid.UUID,
) (referencedomain.ApprovedReferencePlanVersion, error) {
	var activation model.ProjectReferencePlanActivationHead
	if err := database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Where(
		"project_id = ? AND workspace_id = ?", projectID, workspaceID,
	).First(&activation).Error; err != nil {
		return referencedomain.ApprovedReferencePlanVersion{}, err
	}
	var record model.ApprovedReferencePlanVersion
	if err := database.WithContext(ctx).Where(
		"id = ? AND project_id = ? AND workspace_id = ?",
		activation.CurrentPlanVersionID, projectID, workspaceID,
	).First(&record).Error; err != nil {
		return referencedomain.ApprovedReferencePlanVersion{}, err
	}
	plan, err := decodeStoredReferenceBriefFact[referencedomain.ApprovedReferencePlanVersion](record.Content)
	if err != nil || verifyStoredReferenceBriefHash(record.Content, record.ContentHash) != nil ||
		plan.ContractID != "approved-reference-plan-production" ||
		plan.ID != record.ID.String() || plan.WorkspaceID != record.WorkspaceID.String() ||
		plan.ProjectID != record.ProjectID.String() || plan.LogicalID != record.LogicalID.String() ||
		plan.Revision != record.Revision || plan.CandidateRevisionID != record.CandidateRevisionID.String() ||
		plan.ProductionWorldOwnerSetHash != record.ProductionWorldOwnerSetHash ||
		plan.ExpectedTargetKeyRoot != record.ExpectedTargetKeyRoot ||
		plan.ReviewDecisionID != record.ReviewDecisionID.String() || plan.ContentHash != record.ContentHash ||
		plan.CreatedBy != record.CreatedBy.String() || !plan.CreatedAt.Equal(record.CreatedAt) ||
		activation.HeadRevision != plan.Revision || activation.CurrentPlanLogicalID != record.LogicalID ||
		activation.CurrentPlanContentHash != plan.ContentHash {
		return referencedomain.ApprovedReferencePlanVersion{}, errors.New("current Reference Plan fact has drifted")
	}
	var scopeHead model.ReferencePlanScopeHead
	if err = database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Where(
		"plan_logical_id = ? AND workspace_id = ? AND project_id = ?",
		record.LogicalID, workspaceID, projectID,
	).First(&scopeHead).Error; err != nil {
		return referencedomain.ApprovedReferencePlanVersion{}, err
	}
	if scopeHead.HeadRevision != plan.Revision || scopeHead.CurrentPlanVersionID != record.ID ||
		scopeHead.CurrentPlanContentHash != plan.ContentHash ||
		activation.HeadRevision != scopeHead.HeadRevision ||
		activation.CurrentPlanVersionID != scopeHead.CurrentPlanVersionID {
		return referencedomain.ApprovedReferencePlanVersion{}, errors.New("current Reference Plan Heads have drifted")
	}
	return plan, nil
}

func loadReferenceBriefTarget(
	ctx context.Context,
	database *gorm.DB,
	workspaceID uuid.UUID,
	projectID uuid.UUID,
	plan referencedomain.ApprovedReferencePlanVersion,
	targetBusinessKey string,
) (referencedomain.ReferencePlanTargetVersion, error) {
	planID, err := uuid.Parse(plan.ID)
	if err != nil {
		return referencedomain.ReferencePlanTargetVersion{}, errors.New("invalid current Reference Plan identity")
	}
	var record model.ReferencePlanTargetVersion
	if err = database.WithContext(ctx).Where(
		"plan_version_id = ? AND target_business_key = ? AND workspace_id = ? AND project_id = ?",
		planID, targetBusinessKey, workspaceID, projectID,
	).First(&record).Error; err != nil {
		return referencedomain.ReferencePlanTargetVersion{}, err
	}
	target, err := decodeStoredReferenceBriefFact[referencedomain.ReferencePlanTargetVersion](record.Content)
	if err != nil || verifyStoredReferenceBriefHash(record.Content, record.ContentHash) != nil ||
		target.ContractID != "reference-plan-target-production" ||
		target.ID != record.ID.String() || target.WorkspaceID != record.WorkspaceID.String() ||
		target.ProjectID != record.ProjectID.String() || target.PlanVersionID != record.PlanVersionID.String() ||
		target.Revision != record.Revision || target.TargetBusinessKey != record.TargetBusinessKey ||
		target.TargetKind != record.TargetKind || target.Fulfillment != record.Fulfillment ||
		target.ContentHash != record.ContentHash || target.CreatedBy != record.CreatedBy.String() ||
		!target.CreatedAt.Equal(record.CreatedAt) {
		return referencedomain.ReferencePlanTargetVersion{}, errors.New("Reference Plan Target fact has drifted")
	}
	return target, nil
}

func loadCurrentReferenceBriefPresetFacts(
	ctx context.Context,
	database *gorm.DB,
	workspaceID uuid.UUID,
	projectID uuid.UUID,
	plan referencedomain.ApprovedReferencePlanVersion,
) (platformowner.VersionRef, platformowner.VersionRef, platformowner.VersionRef, error) {
	var head model.PresetEffectiveScopeHead
	if err := database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Where(
		"project_id = ? AND workspace_id = ?", projectID, workspaceID,
	).First(&head).Error; err != nil {
		return platformowner.VersionRef{}, platformowner.VersionRef{}, platformowner.VersionRef{}, err
	}
	var bindingRecord model.ProjectPresetBindingVersion
	var styleRecord model.EffectiveStyleSnapshot
	var policyRecord model.EffectivePolicySnapshot
	if err := database.WithContext(ctx).Where("id = ?", head.BindingVersionID).First(&bindingRecord).Error; err != nil {
		return platformowner.VersionRef{}, platformowner.VersionRef{}, platformowner.VersionRef{}, err
	}
	if err := database.WithContext(ctx).Where("id = ?", head.StyleSnapshotID).First(&styleRecord).Error; err != nil {
		return platformowner.VersionRef{}, platformowner.VersionRef{}, platformowner.VersionRef{}, err
	}
	if err := database.WithContext(ctx).Where("id = ?", head.PolicySnapshotID).First(&policyRecord).Error; err != nil {
		return platformowner.VersionRef{}, platformowner.VersionRef{}, platformowner.VersionRef{}, err
	}
	binding, err := decodeStoredReferenceBriefFact[presetdomain.ProjectPresetBindingVersion](bindingRecord.Content)
	if err != nil || verifyStoredReferenceBriefHash(bindingRecord.Content, bindingRecord.ContentHash) != nil ||
		binding.ContractID != "project-preset-binding-version-production" ||
		binding.ID != bindingRecord.ID.String() || binding.WorkspaceID != bindingRecord.WorkspaceID.String() ||
		binding.ProjectID != bindingRecord.ProjectID.String() || binding.Revision != bindingRecord.Revision ||
		binding.SelectionID != bindingRecord.SelectionID.String() ||
		binding.CandidateRevisionID != bindingRecord.CandidateRevisionID.String() ||
		binding.ReviewDecisionID != bindingRecord.ReviewDecisionID.String() ||
		binding.ContentHash != bindingRecord.ContentHash || binding.CreatedBy != bindingRecord.CreatedBy.String() ||
		!binding.CreatedAt.Equal(bindingRecord.CreatedAt) {
		return platformowner.VersionRef{}, platformowner.VersionRef{}, platformowner.VersionRef{}, errors.New("current Preset Binding fact has drifted")
	}
	style, err := decodeStoredReferenceBriefFact[presetdomain.EffectiveStyleSnapshot](styleRecord.Content)
	if err != nil || verifyStoredReferenceBriefHash(styleRecord.Content, styleRecord.ContentHash) != nil ||
		style.ContractID != "effective-style-snapshot-production" ||
		style.ID != styleRecord.ID.String() || style.WorkspaceID != styleRecord.WorkspaceID.String() ||
		style.ProjectID != styleRecord.ProjectID.String() || style.Revision != styleRecord.Revision ||
		style.CandidateRevisionID != styleRecord.CandidateRevisionID.String() ||
		style.ReviewDecisionID != styleRecord.ReviewDecisionID.String() ||
		style.ContentHash != styleRecord.ContentHash || style.CreatedBy != styleRecord.CreatedBy.String() ||
		!style.CreatedAt.Equal(styleRecord.CreatedAt) {
		return platformowner.VersionRef{}, platformowner.VersionRef{}, platformowner.VersionRef{}, errors.New("current Effective Style fact has drifted")
	}
	policy, err := decodeStoredReferenceBriefFact[presetdomain.EffectivePolicySnapshot](policyRecord.Content)
	if err != nil || verifyStoredReferenceBriefHash(policyRecord.Content, policyRecord.ContentHash) != nil ||
		policy.ContractID != "effective-policy-snapshot-production" ||
		policy.ID != policyRecord.ID.String() || policy.WorkspaceID != policyRecord.WorkspaceID.String() ||
		policy.ProjectID != policyRecord.ProjectID.String() || policy.Revision != policyRecord.Revision ||
		policy.EffectiveStyleSnapshotID != styleRecord.ID.String() ||
		policy.ReviewDecisionID != policyRecord.ReviewDecisionID.String() ||
		policy.ContentHash != policyRecord.ContentHash || policy.CreatedBy != policyRecord.CreatedBy.String() ||
		!policy.CreatedAt.Equal(policyRecord.CreatedAt) {
		return platformowner.VersionRef{}, platformowner.VersionRef{}, platformowner.VersionRef{}, errors.New("current Effective Policy fact has drifted")
	}
	if head.HeadRevision != plan.Revision || binding.Revision != head.HeadRevision ||
		style.Revision != head.HeadRevision || policy.Revision != head.HeadRevision ||
		binding.WorkspaceID != plan.WorkspaceID || binding.ProjectID != plan.ProjectID ||
		style.WorkspaceID != plan.WorkspaceID || style.ProjectID != plan.ProjectID ||
		policy.WorkspaceID != plan.WorkspaceID || policy.ProjectID != plan.ProjectID ||
		policy.EffectiveStyleSnapshotHash != style.ContentHash {
		return platformowner.VersionRef{}, platformowner.VersionRef{}, platformowner.VersionRef{}, errors.New("current Preset Effective Head has drifted")
	}
	base := platformowner.VersionRef{
		WorkspaceID: plan.WorkspaceID, ProjectID: plan.ProjectID,
		OwnerKind: "preset", VersionFamily: "preset_effective_set", OwnerLogicalID: plan.ProjectID,
	}
	visual := base
	visual.OwnerVersionID, visual.OwnerRevision, visual.OwnerContentHash = binding.ID, binding.Revision, binding.ContentHash
	styleRef := base
	styleRef.OwnerVersionID, styleRef.OwnerRevision, styleRef.OwnerContentHash = style.ID, style.Revision, style.ContentHash
	policyRef := base
	policyRef.OwnerVersionID, policyRef.OwnerRevision, policyRef.OwnerContentHash = policy.ID, policy.Revision, policy.ContentHash
	return visual, styleRef, policyRef, nil
}

func decodeStoredReferenceBriefFact[T any](raw []byte) (T, error) {
	var value T
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, errors.New("invalid stored Reference Brief fact")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return value, errors.New("invalid stored Reference Brief fact")
	}
	return value, nil
}

func verifyStoredReferenceBriefHash(raw []byte, expected string) error {
	var material map[string]json.RawMessage
	if err := json.Unmarshal(raw, &material); err != nil {
		return err
	}
	material["content_hash"] = json.RawMessage(`""`)
	encoded, err := json.Marshal(material)
	if err != nil {
		return err
	}
	hash, err := platformcanonical.Hash(encoded)
	if err != nil {
		return err
	}
	if hash != expected {
		return errors.New("stored Reference Brief fact content hash has drifted")
	}
	return nil
}
