package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	commandgorm "github.com/StephenQiu30/lanverse/backend/internal/platform/command/adapter/gormdb"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	presetdomain "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
	referenceapp "github.com/StephenQiu30/lanverse/backend/internal/production/reference/application"
	referencedomain "github.com/StephenQiu30/lanverse/backend/internal/production/reference/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type Store struct {
	database *gorm.DB
}

func NewStore(database *gorm.DB) *Store {
	return &Store{database: database}
}

func (store *Store) WithinVisualFoundationConfirmation(
	ctx context.Context,
	operation func(referenceapp.ConfirmationTransaction) error,
) error {
	if store == nil || store.database == nil {
		return errors.New("Visual Foundation confirmation store is unavailable")
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&confirmationTransaction{database: transaction})
	})
}

type confirmationTransaction struct {
	database *gorm.DB
}

func (transaction *confirmationTransaction) FindCommandReceipt(
	ctx context.Context,
	workspaceID, idempotencyKey string,
) (platformcommand.Receipt, error) {
	return commandgorm.Find(
		ctx, transaction.database, workspaceID,
		referencedomain.ConfirmVisualFoundationOperation, idempotencyKey,
	)
}

func (transaction *confirmationTransaction) ValidateReadSet(
	ctx context.Context,
	command referenceapp.ConfirmVisualFoundationCommand,
) error {
	workspaceID, projectID, actorID, gateInputID, decisionID, err := confirmationIDs(command)
	if err != nil {
		return fmt.Errorf("Visual Foundation confirmation identity: %w", referenceapp.ErrVisualFoundationConfirmationConflict)
	}
	var project model.Project
	if err = transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&project, "id = ?", projectID).Error; err != nil {
		return err
	}
	var membership model.Membership
	if err = transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		Where("workspace_id = ? AND user_id = ? AND status = ?", workspaceID, actorID, "active").
		First(&membership).Error; err != nil {
		return err
	}
	if project.WorkspaceID != workspaceID || project.Status != "active" ||
		(membership.Role != "owner" && membership.Role != "editor") {
		return fmt.Errorf("Visual Foundation confirmation project access: %w", referenceapp.ErrVisualFoundationConfirmationConflict)
	}
	var gateRecord model.WorkflowHumanGateInput
	if err = transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&gateRecord, "id = ?", gateInputID).Error; err != nil {
		return err
	}
	gate, _, decodeErr := workflowdomain.DecodeVisualFoundationScopeGateInput(json.RawMessage(gateRecord.Input))
	if decodeErr != nil || gateRecord.WorkspaceID != workspaceID || gateRecord.ProjectID != projectID ||
		gateRecord.InputHash != command.GateInputHash || gate.InputHash != command.GateInputHash ||
		len(gate.SemanticBlockers) != 0 || !containsDecision(gate.AllowedDecisions, "approved") ||
		gate.Subject.ProjectPresetSelection.SelectionID != command.Selection.ID ||
		gate.Subject.VisualFoundationCandidate.RevisionID != command.VisualCandidate.ID ||
		gate.Subject.VisualFoundationCandidate.Revision != command.VisualCandidate.Revision ||
		gate.Subject.VisualFoundationCandidate.RevisionHash != command.VisualCandidate.RevisionHash ||
		gate.Subject.VisualFoundationCandidate.ContentHash != command.VisualCandidate.ContentHash ||
		gate.Subject.ReferencePlanCandidate.RevisionID != command.ReferenceCandidate.ID ||
		gate.Subject.ReferencePlanCandidate.Revision != command.ReferenceCandidate.Revision ||
		gate.Subject.ReferencePlanCandidate.RevisionHash != command.ReferenceCandidate.RevisionHash ||
		gate.Subject.ReferencePlanCandidate.ContentHash != command.ReferenceCandidate.ContentHash ||
		gate.Subject.ReferenceTargetSeedRoot != command.ReferenceTargetSeedRoot ||
		gate.Subject.ConfirmedProductionWorld.OwnerSetHash != command.ProductionWorldOwnerSetHash ||
		!reflect.DeepEqual(gate.Subject.ExpectedReferenceTargetSet, command.ExpectedTargetSet) ||
		!sameExpectedHeads(gate.EffectPlan.AtomicStep.ExpectedHeads, command) {
		return fmt.Errorf("Visual Foundation confirmation Gate read set: %w", referenceapp.ErrVisualFoundationConfirmationConflict)
	}
	if err = transaction.validateDecision(ctx, workspaceID, projectID, actorID, gateRecord, decisionID); err != nil {
		return fmt.Errorf("Visual Foundation confirmation decision: %w", err)
	}
	if err = transaction.validateSelection(ctx, workspaceID, projectID, command); err != nil {
		return fmt.Errorf("Visual Foundation confirmation selection: %w", err)
	}
	if err = transaction.validateCandidate(ctx, workspaceID, projectID, command.VisualCandidate, "visual_foundation_candidate"); err != nil {
		return fmt.Errorf("Visual Foundation confirmation visual candidate: %w", err)
	}
	if err = transaction.validateCandidate(ctx, workspaceID, projectID, command.ReferenceCandidate, "reference_plan_candidate"); err != nil {
		return fmt.Errorf("Visual Foundation confirmation reference candidate: %w", err)
	}
	if err = transaction.validateReferenceProjection(ctx, command); err != nil {
		return fmt.Errorf("Visual Foundation confirmation projection: %w", err)
	}
	if err = transaction.validateExpectedHeads(ctx, projectID, command); err != nil {
		return fmt.Errorf("Visual Foundation confirmation expected Heads: %w", err)
	}
	return nil
}

func containsDecision(decisions []string, expected string) bool {
	for _, decision := range decisions {
		if decision == expected {
			return true
		}
	}
	return false
}

func (transaction *confirmationTransaction) validateDecision(
	ctx context.Context,
	workspaceID, projectID, actorID uuid.UUID,
	gate model.WorkflowHumanGateInput,
	decisionID uuid.UUID,
) error {
	var decision model.ReviewDecision
	if err := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&decision, "id = ?", decisionID).Error; err != nil {
		return err
	}
	var task model.HumanTask
	if err := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&task, "id = ?", decision.HumanTaskID).Error; err != nil {
		return err
	}
	if decision.WorkspaceID != workspaceID || decision.CreatedBy != actorID || decision.Decision != "approved" ||
		task.WorkspaceID != workspaceID || task.ProjectID != projectID || task.SubjectID != gate.ID ||
		task.NodeRunID != gate.NodeRunID || task.Status != "COMPLETED" ||
		decision.SubjectRevision != task.SubjectRevision || decision.SubjectHash != task.SubjectHash ||
		task.SubjectHash != gate.InputHash {
		return referenceapp.ErrVisualFoundationConfirmationConflict
	}
	return nil
}

func (transaction *confirmationTransaction) validateSelection(
	ctx context.Context,
	workspaceID, projectID uuid.UUID,
	command referenceapp.ConfirmVisualFoundationCommand,
) error {
	selectionID, err := uuid.Parse(command.Selection.ID)
	if err != nil {
		return referenceapp.ErrVisualFoundationConfirmationConflict
	}
	var record model.ProjectPresetSelection
	if err = transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&record, "id = ?", selectionID).Error; err != nil {
		return err
	}
	var head model.ProjectPresetSelectionHead
	if err = transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&head, "project_id = ?", projectID).Error; err != nil {
		return err
	}
	selection, _, decodeErr := presetdomain.DecodeProjectSelection(json.RawMessage(record.Selection))
	release, _, releaseErr := presetdomain.NewRelease(command.Release.ReleaseInput)
	if decodeErr != nil || releaseErr != nil || !reflect.DeepEqual(selection, command.Selection) ||
		!reflect.DeepEqual(release, command.Release) || record.WorkspaceID != workspaceID ||
		record.ProjectID != projectID || record.ContentHash != selection.ContentHash ||
		head.WorkspaceID != workspaceID || head.CurrentSelectionID != record.ID ||
		head.CurrentContentHash != record.ContentHash || head.Revision != record.Revision {
		return referenceapp.ErrVisualFoundationConfirmationConflict
	}
	return nil
}

func (transaction *confirmationTransaction) validateCandidate(
	ctx context.Context,
	workspaceID, projectID uuid.UUID,
	candidate referenceapp.CandidateRevision,
	candidateType string,
) error {
	candidateID, err := uuid.Parse(candidate.ID)
	if err != nil {
		return referenceapp.ErrVisualFoundationConfirmationConflict
	}
	var record model.SceneAnalysisCandidateRevision
	if err = transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&record, "id = ?", candidateID).Error; err != nil {
		return err
	}
	var head model.SceneAnalysisCandidateHead
	if err = transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&head, "stage_instance_key = ?", record.StageInstanceKey).Error; err != nil {
		return err
	}
	contentHash, hashErr := platformcanonical.Hash(json.RawMessage(record.Candidate))
	candidateHash, candidateHashErr := platformcanonical.Hash(candidate.Candidate)
	if hashErr != nil || candidateHashErr != nil || record.WorkspaceID != workspaceID || record.ProjectID != projectID ||
		record.CandidateType != candidateType || record.RevisionNo != candidate.Revision ||
		record.CandidateRevisionHash != candidate.RevisionHash ||
		record.CandidateContentHash != candidate.ContentHash || contentHash != candidate.ContentHash ||
		candidateHash != candidate.ContentHash ||
		head.WorkspaceID != workspaceID || head.ProjectID != projectID ||
		head.CurrentRevisionID != record.ID ||
		head.CurrentCandidateRevisionHash != record.CandidateRevisionHash ||
		head.Revision != record.RevisionNo {
		return referenceapp.ErrVisualFoundationConfirmationConflict
	}
	return nil
}

func (transaction *confirmationTransaction) validateReferenceProjection(
	ctx context.Context,
	command referenceapp.ConfirmVisualFoundationCommand,
) error {
	referenceID, _ := uuid.Parse(command.ReferenceCandidate.ID)
	var record model.SceneAnalysisCandidateRevision
	if err := transaction.database.WithContext(ctx).First(&record, "id = ?", referenceID).Error; err != nil {
		return err
	}
	var invocation model.SceneAnalysisInvocationRecord
	if err := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&invocation, "id = ?", record.SourceInvocationID).Error; err != nil {
		return err
	}
	var payload agentcontract.ReferencePlanPayload
	if json.Unmarshal(invocation.Payload, &payload) != nil || payload.Validate() != nil ||
		invocation.StageKey != agentcontract.ReferencePlanStageKey || invocation.Status != "accepted" {
		return referenceapp.ErrVisualFoundationConfirmationConflict
	}
	projection, err := workflowapp.BuildReferencePlanCandidateProjection(
		payload.StageInput, json.RawMessage(record.Candidate),
	)
	if err != nil || !reflect.DeepEqual(projection.ExpectedTargetSet, command.ExpectedTargetSet) ||
		!reflect.DeepEqual(targetDrafts(projection), command.Targets) {
		return referenceapp.ErrVisualFoundationConfirmationConflict
	}
	return nil
}

func (transaction *confirmationTransaction) validateExpectedHeads(
	ctx context.Context,
	projectID uuid.UUID,
	command referenceapp.ConfirmVisualFoundationCommand,
) error {
	var presetHead model.PresetEffectiveScopeHead
	presetErr := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&presetHead, "project_id = ?", projectID).Error
	referenceLogicalID, _ := uuid.Parse(command.ExpectedReferenceHead.LogicalID)
	var referenceHead model.ReferencePlanScopeHead
	referenceErr := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&referenceHead, "plan_logical_id = ?", referenceLogicalID).Error
	if command.ExpectedPresetHead.Revision == 0 {
		if !errors.Is(presetErr, gorm.ErrRecordNotFound) || !errors.Is(referenceErr, gorm.ErrRecordNotFound) {
			return referenceapp.ErrVisualFoundationConfirmationConflict
		}
		var activation model.ProjectReferencePlanActivationHead
		activationErr := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&activation, "project_id = ?", projectID).Error
		if !errors.Is(activationErr, gorm.ErrRecordNotFound) {
			return referenceapp.ErrVisualFoundationConfirmationConflict
		}
		return nil
	}
	if presetErr != nil || referenceErr != nil ||
		presetHead.HeadRevision != command.ExpectedPresetHead.Revision ||
		presetHead.HeadContentHash != command.ExpectedPresetHead.ContentHash ||
		referenceHead.HeadRevision != command.ExpectedReferenceHead.Revision ||
		referenceHead.HeadContentHash != command.ExpectedReferenceHead.ContentHash {
		return referenceapp.ErrVisualFoundationConfirmationConflict
	}
	var activation model.ProjectReferencePlanActivationHead
	if err := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&activation, "project_id = ?", projectID).Error; err != nil {
		return err
	}
	if activation.HeadRevision != referenceHead.HeadRevision ||
		activation.CurrentPlanLogicalID != referenceHead.PlanLogicalID ||
		activation.CurrentPlanVersionID != referenceHead.CurrentPlanVersionID ||
		activation.CurrentPlanContentHash != referenceHead.CurrentPlanContentHash {
		return referenceapp.ErrVisualFoundationConfirmationConflict
	}
	return nil
}

func (transaction *confirmationTransaction) PersistConfirmation(
	ctx context.Context,
	value referenceapp.ConfirmationPersistence,
) error {
	if err := transaction.createPresetSet(ctx, value.PresetSet); err != nil {
		return err
	}
	if err := transaction.createReferenceSet(ctx, value.ReferenceSet); err != nil {
		return err
	}
	if err := transaction.createCollectionReceipt(ctx, value.PresetReceipt); err != nil {
		return err
	}
	if err := transaction.createCollectionReceipt(ctx, value.ReferenceReceipt); err != nil {
		return err
	}
	if err := commandgorm.Create(ctx, transaction.database, value.CommandReceipt); err != nil {
		return err
	}
	ids, err := parseUUIDs(
		value.Outbox.ID, value.Outbox.WorkspaceID, value.Outbox.ProjectID,
		value.Outbox.AggregateID, value.Outbox.SourceReceiptID,
	)
	if err != nil {
		return err
	}
	return transaction.database.WithContext(ctx).Omit(clause.Associations).Create(&model.OutboxEvent{
		ID: ids[0], EventType: referencedomain.VisualFoundationConfirmedEvent, EventVersion: 1,
		WorkspaceID: ids[1], ProjectID: ids[2], AggregateKind: "approved_reference_plan",
		AggregateID: ids[3].String(), AggregateRevision: value.Outbox.AggregateRevision,
		SourceReceiptID: ids[4], Payload: datatypes.JSON(value.Outbox.Payload),
		PayloadHash: value.Outbox.PayloadHash, Status: "pending",
		OccurredAt: value.Outbox.OccurredAt, CreatedAt: value.Outbox.OccurredAt,
	}).Error
}

func (transaction *confirmationTransaction) createPresetSet(
	ctx context.Context,
	value presetdomain.EffectiveVisualFoundationSet,
) error {
	ids, err := parseUUIDs(
		value.Binding.ID, value.Binding.WorkspaceID, value.Binding.ProjectID,
		value.Binding.SelectionID, value.Binding.CandidateRevisionID,
		value.Binding.ReviewDecisionID, value.Binding.CreatedBy,
	)
	if err != nil {
		return err
	}
	bindingJSON, err := json.Marshal(value.Binding)
	if err != nil {
		return err
	}
	binding := model.ProjectPresetBindingVersion{
		ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], Revision: value.Binding.Revision,
		SelectionID: ids[3], CandidateRevisionID: ids[4], ReviewDecisionID: ids[5],
		Content: datatypes.JSON(bindingJSON), ContentHash: value.Binding.ContentHash,
		CreatedBy: ids[6], CreatedAt: value.Binding.CreatedAt,
	}
	styleID, _ := uuid.Parse(value.Style.ID)
	styleJSON, err := json.Marshal(value.Style)
	if err != nil {
		return err
	}
	style := model.EffectiveStyleSnapshot{
		ID: styleID, WorkspaceID: ids[1], ProjectID: ids[2], Revision: value.Style.Revision,
		CandidateRevisionID: ids[4], ReviewDecisionID: ids[5],
		Content: datatypes.JSON(styleJSON), ContentHash: value.Style.ContentHash,
		CreatedBy: ids[6], CreatedAt: value.Style.CreatedAt,
	}
	policyID, _ := uuid.Parse(value.Policy.ID)
	policyJSON, err := json.Marshal(value.Policy)
	if err != nil {
		return err
	}
	policy := model.EffectivePolicySnapshot{
		ID: policyID, WorkspaceID: ids[1], ProjectID: ids[2], Revision: value.Policy.Revision,
		EffectiveStyleSnapshotID: styleID, ReviewDecisionID: ids[5],
		Content: datatypes.JSON(policyJSON), ContentHash: value.Policy.ContentHash,
		CreatedBy: ids[6], CreatedAt: value.Policy.CreatedAt,
	}
	for _, record := range []any{&binding, &style, &policy} {
		if err = transaction.database.WithContext(ctx).Omit(clause.Associations).Create(record).Error; err != nil {
			return err
		}
	}
	head := model.PresetEffectiveScopeHead{
		ProjectID: ids[2], WorkspaceID: ids[1], HeadRevision: value.Head.HeadRevision,
		BindingVersionID: ids[0], StyleSnapshotID: styleID, PolicySnapshotID: policyID,
		CollectionRootHash: value.Head.CollectionRootHash,
		HeadContentHash:    value.Head.ContentHash, UpdatedAt: value.Binding.CreatedAt,
	}
	return transaction.replacePresetHead(ctx, head)
}

func (transaction *confirmationTransaction) createReferenceSet(
	ctx context.Context,
	value referencedomain.ApprovedReferencePlanSet,
) error {
	ids, err := parseUUIDs(
		value.Version.ID, value.Version.WorkspaceID, value.Version.ProjectID,
		value.Version.LogicalID, value.Version.CandidateRevisionID,
		value.Version.ReviewDecisionID, value.Version.CreatedBy,
	)
	if err != nil {
		return err
	}
	versionJSON, _ := json.Marshal(value.Version)
	version := model.ApprovedReferencePlanVersion{
		ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], LogicalID: ids[3],
		Revision: value.Version.Revision, CandidateRevisionID: ids[4],
		ProductionWorldOwnerSetHash: value.Version.ProductionWorldOwnerSetHash,
		ExpectedTargetKeyRoot:       value.Version.ExpectedTargetKeyRoot,
		ReviewDecisionID:            ids[5], Content: datatypes.JSON(versionJSON),
		ContentHash: value.Version.ContentHash, CreatedBy: ids[6], CreatedAt: value.Version.CreatedAt,
	}
	if err = transaction.database.WithContext(ctx).Omit(clause.Associations).Create(&version).Error; err != nil {
		return err
	}
	targets := make([]model.ReferencePlanTargetVersion, len(value.Targets))
	for index, target := range value.Targets {
		targetID, parseErr := uuid.Parse(target.ID)
		if parseErr != nil {
			return parseErr
		}
		content, marshalErr := json.Marshal(target)
		if marshalErr != nil {
			return marshalErr
		}
		targets[index] = model.ReferencePlanTargetVersion{
			ID: targetID, WorkspaceID: ids[1], ProjectID: ids[2], PlanVersionID: ids[0],
			Revision: target.Revision, TargetBusinessKey: target.TargetBusinessKey,
			TargetKind: target.TargetKind, Fulfillment: target.Fulfillment,
			Content: datatypes.JSON(content), ContentHash: target.ContentHash,
			CreatedBy: ids[6], CreatedAt: target.CreatedAt,
		}
	}
	if err = transaction.database.WithContext(ctx).Omit(clause.Associations).Create(&targets).Error; err != nil {
		return err
	}
	scopeHead := model.ReferencePlanScopeHead{
		PlanLogicalID: ids[3], WorkspaceID: ids[1], ProjectID: ids[2],
		HeadRevision: value.ScopeHead.HeadRevision, CurrentPlanVersionID: ids[0],
		CurrentPlanContentHash: value.ScopeHead.CurrentPlanContentHash,
		CollectionRootHash:     value.Collection.CollectionRootHash,
		HeadContentHash:        value.ScopeHead.ContentHash, UpdatedAt: value.Version.CreatedAt,
	}
	if err = transaction.replaceReferenceHead(ctx, scopeHead); err != nil {
		return err
	}
	activation := model.ProjectReferencePlanActivationHead{
		ProjectID: ids[2], WorkspaceID: ids[1], HeadRevision: value.ActivationHead.HeadRevision,
		CurrentPlanLogicalID: ids[3], CurrentPlanVersionID: ids[0],
		CurrentPlanContentHash: value.ActivationHead.CurrentPlanContentHash,
		HeadContentHash:        value.ActivationHead.ContentHash, UpdatedAt: value.Version.CreatedAt,
	}
	return transaction.replaceActivationHead(ctx, activation)
}

func (transaction *confirmationTransaction) createCollectionReceipt(
	ctx context.Context,
	value referencedomain.VisualScopeCollectionReceipt,
) error {
	ids, err := parseUUIDs(
		value.ID, value.CommandID, value.WorkspaceID, value.ProjectID,
		value.GateInputID, value.ReviewDecisionID, value.CommittedBy,
	)
	if err != nil {
		return err
	}
	collection, err := json.Marshal(value.Collection)
	if err != nil {
		return err
	}
	return transaction.database.WithContext(ctx).Omit(clause.Associations).
		Create(&model.VisualFoundationScopeCollectionReceipt{
			ID: ids[0], CommandID: ids[1], WorkspaceID: ids[2], ProjectID: ids[3],
			GateInputID: ids[4], ReviewDecisionID: ids[5],
			OwnerKind: value.Collection.OwnerKind, VersionFamily: value.Collection.VersionFamily,
			Collection: datatypes.JSON(collection), CollectionRootHash: value.Collection.CollectionRootHash,
			ReceiptContentHash: value.ReceiptContentHash,
			CommittedBy:        ids[6], CommittedAt: value.CommittedAt,
		}).Error
}

func (transaction *confirmationTransaction) replacePresetHead(ctx context.Context, head model.PresetEffectiveScopeHead) error {
	if head.HeadRevision == 1 {
		return transaction.database.WithContext(ctx).Omit(clause.Associations).Create(&head).Error
	}
	result := transaction.database.WithContext(ctx).Model(&model.PresetEffectiveScopeHead{}).
		Where("project_id = ? AND head_revision = ?", head.ProjectID, head.HeadRevision-1).
		Updates(map[string]any{
			"head_revision": head.HeadRevision, "binding_version_id": head.BindingVersionID,
			"style_snapshot_id": head.StyleSnapshotID, "policy_snapshot_id": head.PolicySnapshotID,
			"collection_root_hash": head.CollectionRootHash, "head_content_hash": head.HeadContentHash,
			"updated_at": head.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return referenceapp.ErrVisualFoundationConfirmationConflict
	}
	return nil
}

func (transaction *confirmationTransaction) replaceReferenceHead(ctx context.Context, head model.ReferencePlanScopeHead) error {
	if head.HeadRevision == 1 {
		return transaction.database.WithContext(ctx).Omit(clause.Associations).Create(&head).Error
	}
	result := transaction.database.WithContext(ctx).Model(&model.ReferencePlanScopeHead{}).
		Where("plan_logical_id = ? AND head_revision = ?", head.PlanLogicalID, head.HeadRevision-1).
		Updates(map[string]any{
			"head_revision": head.HeadRevision, "current_plan_version_id": head.CurrentPlanVersionID,
			"current_plan_content_hash": head.CurrentPlanContentHash,
			"collection_root_hash":      head.CollectionRootHash, "head_content_hash": head.HeadContentHash,
			"updated_at": head.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return referenceapp.ErrVisualFoundationConfirmationConflict
	}
	return nil
}

func (transaction *confirmationTransaction) replaceActivationHead(
	ctx context.Context,
	head model.ProjectReferencePlanActivationHead,
) error {
	if head.HeadRevision == 1 {
		return transaction.database.WithContext(ctx).Omit(clause.Associations).Create(&head).Error
	}
	result := transaction.database.WithContext(ctx).Model(&model.ProjectReferencePlanActivationHead{}).
		Where("project_id = ? AND head_revision = ?", head.ProjectID, head.HeadRevision-1).
		Updates(map[string]any{
			"head_revision": head.HeadRevision, "current_plan_logical_id": head.CurrentPlanLogicalID,
			"current_plan_version_id":   head.CurrentPlanVersionID,
			"current_plan_content_hash": head.CurrentPlanContentHash,
			"head_content_hash":         head.HeadContentHash, "updated_at": head.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return referenceapp.ErrVisualFoundationConfirmationConflict
	}
	return nil
}

func confirmationIDs(command referenceapp.ConfirmVisualFoundationCommand) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, error) {
	values, err := parseUUIDs(
		command.WorkspaceID, command.ProjectID, command.ActorID,
		command.GateInputID, command.ReviewDecisionID,
	)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, err
	}
	return values[0], values[1], values[2], values[3], values[4], nil
}

func parseUUIDs(values ...string) ([]uuid.UUID, error) {
	result := make([]uuid.UUID, len(values))
	for index, value := range values {
		parsed, err := uuid.Parse(value)
		if err != nil || parsed == uuid.Nil {
			return nil, errors.New("invalid Visual Foundation persistence identity")
		}
		result[index] = parsed
	}
	return result, nil
}

func sameExpectedHeads(
	values []workflowdomain.HumanGateExpectedHead,
	command referenceapp.ConfirmVisualFoundationCommand,
) bool {
	if len(values) != 2 {
		return false
	}
	return values[0].OwnerKind == command.ExpectedPresetHead.OwnerKind &&
		values[0].LogicalID == command.ExpectedPresetHead.LogicalID &&
		values[0].Revision == command.ExpectedPresetHead.Revision &&
		values[0].ContentHash == command.ExpectedPresetHead.ContentHash &&
		values[1].OwnerKind == command.ExpectedReferenceHead.OwnerKind &&
		values[1].LogicalID == command.ExpectedReferenceHead.LogicalID &&
		values[1].Revision == command.ExpectedReferenceHead.Revision &&
		values[1].ContentHash == command.ExpectedReferenceHead.ContentHash
}

func targetDrafts(projection workflowapp.ReferencePlanCandidateProjection) []referencedomain.TargetDraft {
	result := make([]referencedomain.TargetDraft, len(projection.Targets))
	for index, target := range projection.Targets {
		result[index] = referencedomain.TargetDraft{
			TargetBusinessKey: target.TargetBusinessKey, TargetKind: target.TargetKind,
			Fulfillment: target.Fulfillment,
			OwnerRefs: agentcontract.ReferencePlanTargetOwnerRefs{
				Identity:      append([]agentcontract.ReferencePlanOwnerRef{}, target.OwnerRefs.Identity...),
				Specification: append([]agentcontract.ReferencePlanOwnerRef{}, target.OwnerRefs.Specification...),
				State:         append([]agentcontract.ReferencePlanOwnerRef{}, target.OwnerRefs.State...),
				Scene:         append([]agentcontract.ReferencePlanOwnerRef{}, target.OwnerRefs.Scene...),
				Occurrence:    append([]agentcontract.ReferencePlanOwnerRef{}, target.OwnerRefs.Occurrence...),
				Interaction:   append([]agentcontract.ReferencePlanOwnerRef{}, target.OwnerRefs.Interaction...),
			},
			CoverageScopeKeys:           append([]string(nil), target.CoverageScopeKeys...),
			DependsOnTargetBusinessKeys: append([]string(nil), target.DependsOnTargetBusinessKeys...),
			Constraints: referencedomain.TargetConstraints{
				ProductionWorldOwnerSetHash:           target.Constraints.ProductionWorldOwnerSetHash,
				ReferenceTargetSeedRoot:               target.Constraints.ReferenceTargetSeedRoot,
				VisualFoundationCandidateRevisionID:   target.Constraints.VisualFoundationCandidateRevisionID,
				VisualFoundationCandidateRevisionHash: target.Constraints.VisualFoundationCandidateRevisionHash,
				PresetReleaseContentHash:              target.Constraints.PresetReleaseContentHash,
				DesignFocus:                           append([]string(nil), target.Constraints.DesignFocus...),
				ForbiddenChanges:                      append([]string(nil), target.Constraints.ForbiddenChanges...),
			},
		}
	}
	return result
}

var _ referenceapp.ConfirmationStore = (*Store)(nil)
