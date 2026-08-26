package gormdb

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func (repo *repository) PrepareStart(ctx context.Context, desired domain.StartPreparation) (domain.StartPreparation, error) {
	run, nodes, intent, err := startRecords(desired)
	if err != nil {
		return domain.StartPreparation{}, err
	}
	if err = repo.database.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
		Omit(clause.Associations).Create(&run).Error; err != nil {
		return domain.StartPreparation{}, err
	}
	if err = repo.database.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "workspace_id"}, {Name: "idempotency_key"}}, DoNothing: true,
	}).Omit(clause.Associations).Create(&intent).Error; err != nil {
		return domain.StartPreparation{}, err
	}
	var persistedIntent model.WorkflowStartIntent
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ? AND idempotency_key = ?", intent.WorkspaceID, intent.IdempotencyKey).
		First(&persistedIntent).Error; err != nil {
		return domain.StartPreparation{}, normalizeNotFound(err)
	}
	var persistedRun model.WorkflowRun
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&persistedRun, "id = ?", persistedIntent.WorkflowRunID).Error; err != nil {
		return domain.StartPreparation{}, normalizeNotFound(err)
	}
	if persistedIntent.CommandInputHash != intent.CommandInputHash || persistedIntent.TemporalInputHash != intent.TemporalInputHash {
		return repo.loadStartPreparation(ctx, persistedRun, persistedIntent)
	}
	if !sameRunIdentity(persistedRun, run) || !sameIntentIdentity(persistedIntent, intent) {
		return domain.StartPreparation{}, errors.New("persisted workflow start identity has drifted")
	}
	if len(nodes) > 0 {
		if err = repo.database.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "workflow_run_id"}, {Name: "node_id"}}, DoNothing: true,
		}).Omit(clause.Associations).Create(&nodes).Error; err != nil {
			return domain.StartPreparation{}, err
		}
	}
	prepared, err := repo.loadStartPreparation(ctx, persistedRun, persistedIntent)
	if err != nil {
		return domain.StartPreparation{}, err
	}
	if !sameNodeIdentities(prepared.Nodes, desired.Nodes) {
		return domain.StartPreparation{}, errors.New("persisted workflow node projection identity has drifted")
	}
	return prepared, nil
}

func (repo *repository) BeginStartAttempt(ctx context.Context, intentID string, now time.Time) (domain.StartPreparation, error) {
	id, err := uuid.Parse(intentID)
	if err != nil {
		return domain.StartPreparation{}, application.ErrNotFound
	}
	var intent model.WorkflowStartIntent
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&intent, "id = ?", id).Error; err != nil {
		return domain.StartPreparation{}, normalizeNotFound(err)
	}
	var run model.WorkflowRun
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", intent.WorkflowRunID).Error; err != nil {
		return domain.StartPreparation{}, normalizeNotFound(err)
	}
	if intent.Status == "completed" || intent.Status == "conflict" {
		return repo.loadStartPreparation(ctx, run, intent)
	}
	intent.AttemptNo++
	intent.Status = "pending"
	intent.Revision++
	intent.UpdatedAt = now.UTC()
	if err = repo.database.WithContext(ctx).Model(&model.WorkflowStartIntent{}).Where("id = ?", intent.ID).Updates(map[string]any{
		"attempt_no": intent.AttemptNo, "status": intent.Status, "revision": intent.Revision, "updated_at": intent.UpdatedAt,
	}).Error; err != nil {
		return domain.StartPreparation{}, err
	}
	run.Status, run.ProgressStage = "QUEUED", "start_pending"
	run.NextAction, run.Error = nil, nil
	run.Revision++
	run.UpdatedAt = now.UTC()
	if err = repo.database.WithContext(ctx).Model(&model.WorkflowRun{}).Where("id = ?", run.ID).Updates(map[string]any{
		"status": run.Status, "progress_stage": run.ProgressStage, "next_action": nil, "error": nil,
		"revision": run.Revision, "updated_at": run.UpdatedAt,
	}).Error; err != nil {
		return domain.StartPreparation{}, err
	}
	return repo.loadStartPreparation(ctx, run, intent)
}

func (repo *repository) FinalizeStartAttempt(
	ctx context.Context,
	run domain.WorkflowRun,
	intent domain.StartIntent,
	receipt domain.StartReceipt,
	expectedRunRevision, expectedIntentRevision int,
) error {
	runRecord, err := runRecord(run)
	if err != nil {
		return err
	}
	intentRecord, err := startIntentRecord(intent)
	if err != nil {
		return err
	}
	receiptRecord, err := startReceiptRecord(receipt)
	if err != nil {
		return err
	}
	runResult := repo.database.WithContext(ctx).Model(&model.WorkflowRun{}).
		Where("id = ? AND revision = ?", runRecord.ID, expectedRunRevision).
		Updates(map[string]any{
			"status": runRecord.Status, "progress_stage": runRecord.ProgressStage,
			"next_action": runRecord.NextAction, "error": runRecord.Error,
			"revision": runRecord.Revision, "updated_at": runRecord.UpdatedAt,
		})
	if runResult.Error != nil {
		return runResult.Error
	}
	if runResult.RowsAffected != 1 {
		return &application.Error{Code: "resource_conflict", Message: "Workflow run changed before start receipt", Status: 409}
	}
	intentResult := repo.database.WithContext(ctx).Model(&model.WorkflowStartIntent{}).
		Where("id = ? AND revision = ?", intentRecord.ID, expectedIntentRevision).
		Updates(map[string]any{
			"status": intentRecord.Status, "attempt_no": intentRecord.AttemptNo,
			"revision": intentRecord.Revision, "updated_at": intentRecord.UpdatedAt,
		})
	if intentResult.Error != nil {
		return intentResult.Error
	}
	if intentResult.RowsAffected != 1 {
		return &application.Error{Code: "resource_conflict", Message: "Workflow start intent changed before receipt", Status: 409}
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&receiptRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return &application.Error{Code: "resource_conflict", Message: "Workflow start attempt already has a receipt", Status: 409}
		}
		return err
	}
	return nil
}

func (repo *repository) loadStartPreparation(
	ctx context.Context,
	run model.WorkflowRun,
	intent model.WorkflowStartIntent,
) (domain.StartPreparation, error) {
	var nodes []model.NodeRunProjection
	if err := repo.database.WithContext(ctx).Where("workflow_run_id = ?", run.ID).Order("node_id ASC").Find(&nodes).Error; err != nil {
		return domain.StartPreparation{}, err
	}
	result := domain.StartPreparation{Run: runDomain(run), Intent: startIntentDomain(intent)}
	result.Nodes = make([]domain.NodeRunProjection, 0, len(nodes))
	for _, node := range nodes {
		result.Nodes = append(result.Nodes, nodeRunDomain(node))
	}
	return result, nil
}

func startRecords(value domain.StartPreparation) (model.WorkflowRun, []model.NodeRunProjection, model.WorkflowStartIntent, error) {
	run, err := runRecord(value.Run)
	if err != nil {
		return model.WorkflowRun{}, nil, model.WorkflowStartIntent{}, err
	}
	nodes := make([]model.NodeRunProjection, 0, len(value.Nodes))
	for _, node := range value.Nodes {
		record, nodeErr := nodeRunRecord(node)
		if nodeErr != nil {
			return model.WorkflowRun{}, nil, model.WorkflowStartIntent{}, nodeErr
		}
		nodes = append(nodes, record)
	}
	intent, err := startIntentRecord(value.Intent)
	return run, nodes, intent, err
}

func runRecord(value domain.WorkflowRun) (model.WorkflowRun, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.WorkflowRun{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.WorkflowRun{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.WorkflowRun{}, err
	}
	revisionID, err := uuid.Parse(value.AuthoringRevisionID)
	if err != nil {
		return model.WorkflowRun{}, err
	}
	definitionID, err := uuid.Parse(value.DefinitionVersionID)
	if err != nil {
		return model.WorkflowRun{}, err
	}
	snapshotID, err := uuid.Parse(value.RunInputSnapshotID)
	if err != nil {
		return model.WorkflowRun{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.WorkflowRun{}, err
	}
	var sourceWorkflowRunID *uuid.UUID
	if value.SourceWorkflowRunID != nil {
		parsed, parseErr := uuid.Parse(*value.SourceWorkflowRunID)
		if parseErr != nil || value.RerunRootNodeID == nil || strings.TrimSpace(*value.RerunRootNodeID) == "" {
			return model.WorkflowRun{}, errors.New("invalid workflow rerun source identity")
		}
		sourceWorkflowRunID = &parsed
	} else if value.RerunRootNodeID != nil {
		return model.WorkflowRun{}, errors.New("invalid workflow rerun root identity")
	}
	return model.WorkflowRun{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, AuthoringRevisionID: revisionID,
		WorkflowDefinitionVersionID: definitionID, RunInputSnapshotID: snapshotID,
		TemporalWorkflowID: value.TemporalWorkflowID, StartInputHash: value.StartInputHash,
		SourceWorkflowRunID: sourceWorkflowRunID, RerunRootNodeID: cloneStringPointer(value.RerunRootNodeID),
		Status: value.Status, ProgressStage: value.ProgressStage, NextAction: value.NextAction,
		Error: datatypes.JSON(value.Error), PausedFromStatus: value.PausedFromStatus,
		PausedFromProgressStage: value.PausedFromProgressStage,
		Revision:                value.Revision, CreatedBy: createdBy, InitiatorTokenVersion: value.InitiatorTokenVersion,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func nodeRunRecord(value domain.NodeRunProjection) (model.NodeRunProjection, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.NodeRunProjection{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.NodeRunProjection{}, err
	}
	runID, err := uuid.Parse(value.WorkflowRunID)
	if err != nil {
		return model.NodeRunProjection{}, err
	}
	var activeClaimToken *uuid.UUID
	if value.ActiveClaimToken != nil {
		parsed, parseErr := uuid.Parse(*value.ActiveClaimToken)
		if parseErr != nil {
			return model.NodeRunProjection{}, parseErr
		}
		activeClaimToken = &parsed
	}
	var reusedFromNodeRunID *uuid.UUID
	if value.ReusedFromNodeRunID != nil {
		parsed, parseErr := uuid.Parse(*value.ReusedFromNodeRunID)
		if parseErr != nil {
			return model.NodeRunProjection{}, parseErr
		}
		reusedFromNodeRunID = &parsed
	}
	return model.NodeRunProjection{
		ID: id, WorkspaceID: workspaceID, WorkflowRunID: runID, NodeID: value.NodeID,
		DefinitionKey: value.DefinitionKey, DefinitionVersion: value.DefinitionVersion,
		Executor: value.Executor, RiskLevel: value.RiskLevel, Status: value.Status, Attempt: value.Attempt,
		ActiveClaimToken: activeClaimToken, ReusedFromNodeRunID: reusedFromNodeRunID,
		Input: datatypes.JSON(value.Input), InputHash: nodeInputHashPointer(value.InputHash), CacheKey: nodeCacheKeyPointer(value.CacheKey),
		Output: datatypes.JSON(value.Output), OutputHash: nodeOutputHashPointer(value.OutputHash),
		Revision:  value.Revision,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func startIntentRecord(value domain.StartIntent) (model.WorkflowStartIntent, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.WorkflowStartIntent{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.WorkflowStartIntent{}, err
	}
	runID, err := uuid.Parse(value.WorkflowRunID)
	if err != nil {
		return model.WorkflowStartIntent{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.WorkflowStartIntent{}, err
	}
	return model.WorkflowStartIntent{
		ID: id, WorkspaceID: workspaceID, WorkflowRunID: runID, IdempotencyKey: value.IdempotencyKey,
		CommandInputHash: value.CommandInputHash, TemporalInputHash: value.TemporalInputHash,
		Status: value.Status, AttemptNo: value.AttemptNo, Revision: value.Revision,
		CreatedBy: createdBy, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func startReceiptRecord(value domain.StartReceipt) (model.WorkflowStartReceipt, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.WorkflowStartReceipt{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.WorkflowStartReceipt{}, err
	}
	intentID, err := uuid.Parse(value.StartIntentID)
	if err != nil {
		return model.WorkflowStartReceipt{}, err
	}
	runID, err := uuid.Parse(value.WorkflowRunID)
	if err != nil {
		return model.WorkflowStartReceipt{}, err
	}
	return model.WorkflowStartReceipt{
		ID: id, WorkspaceID: workspaceID, StartIntentID: intentID, WorkflowRunID: runID,
		AttemptNo: value.AttemptNo, Outcome: value.Outcome, TemporalWorkflowID: value.TemporalWorkflowID,
		ExpectedInputHash: value.ExpectedInputHash, ObservedInputHash: value.ObservedInputHash, CreatedAt: value.CreatedAt,
	}, nil
}

func runDomain(value model.WorkflowRun) domain.WorkflowRun {
	var sourceWorkflowRunID *string
	if value.SourceWorkflowRunID != nil {
		parsed := value.SourceWorkflowRunID.String()
		sourceWorkflowRunID = &parsed
	}
	return domain.WorkflowRun{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		AuthoringRevisionID: value.AuthoringRevisionID.String(), DefinitionVersionID: value.WorkflowDefinitionVersionID.String(),
		RunInputSnapshotID: value.RunInputSnapshotID.String(), TemporalWorkflowID: value.TemporalWorkflowID,
		StartInputHash: value.StartInputHash, Status: value.Status, ProgressStage: value.ProgressStage,
		SourceWorkflowRunID: sourceWorkflowRunID, RerunRootNodeID: cloneStringPointer(value.RerunRootNodeID),
		NextAction: value.NextAction, Error: append([]byte(nil), value.Error...),
		PausedFromStatus:        cloneStringPointer(value.PausedFromStatus),
		PausedFromProgressStage: cloneStringPointer(value.PausedFromProgressStage), Revision: value.Revision,
		CreatedBy: value.CreatedBy.String(), InitiatorTokenVersion: value.InitiatorTokenVersion,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func nodeRunDomain(value model.NodeRunProjection) domain.NodeRunProjection {
	var claimToken *string
	if value.ActiveClaimToken != nil {
		token := value.ActiveClaimToken.String()
		claimToken = &token
	}
	var reusedFromNodeRunID *string
	if value.ReusedFromNodeRunID != nil {
		reused := value.ReusedFromNodeRunID.String()
		reusedFromNodeRunID = &reused
	}
	return domain.NodeRunProjection{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), WorkflowRunID: value.WorkflowRunID.String(),
		NodeID: value.NodeID, DefinitionKey: value.DefinitionKey, DefinitionVersion: value.DefinitionVersion,
		Executor: value.Executor, RiskLevel: value.RiskLevel, Status: value.Status, Attempt: value.Attempt,
		ActiveClaimToken: claimToken, ReusedFromNodeRunID: reusedFromNodeRunID,
		Input: append([]byte(nil), value.Input...), InputHash: nodeInputHashValue(value.InputHash), CacheKey: nodeCacheKeyValue(value.CacheKey),
		Output: append([]byte(nil), value.Output...), OutputHash: nodeOutputHashValue(value.OutputHash),
		Revision:  value.Revision,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func nodeOutputHashPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func nodeInputHashPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func nodeCacheKeyPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func nodeOutputHashValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func nodeInputHashValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func nodeCacheKeyValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func startIntentDomain(value model.WorkflowStartIntent) domain.StartIntent {
	return domain.StartIntent{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), WorkflowRunID: value.WorkflowRunID.String(),
		IdempotencyKey: value.IdempotencyKey, CommandInputHash: value.CommandInputHash,
		TemporalInputHash: value.TemporalInputHash, Status: value.Status, AttemptNo: value.AttemptNo,
		Revision: value.Revision, CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func sameRunIdentity(left, right model.WorkflowRun) bool {
	return left.ID == right.ID && left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.AuthoringRevisionID == right.AuthoringRevisionID &&
		left.WorkflowDefinitionVersionID == right.WorkflowDefinitionVersionID && left.RunInputSnapshotID == right.RunInputSnapshotID &&
		left.TemporalWorkflowID == right.TemporalWorkflowID && left.StartInputHash == right.StartInputHash &&
		equalOptionalUUID(left.SourceWorkflowRunID, right.SourceWorkflowRunID) &&
		equalOptionalString(left.RerunRootNodeID, right.RerunRootNodeID) &&
		left.CreatedBy == right.CreatedBy && left.InitiatorTokenVersion == right.InitiatorTokenVersion
}

func equalOptionalUUID(left, right *uuid.UUID) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func equalOptionalString(left, right *string) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func sameIntentIdentity(left, right model.WorkflowStartIntent) bool {
	return left.ID == right.ID && left.WorkspaceID == right.WorkspaceID && left.WorkflowRunID == right.WorkflowRunID &&
		left.IdempotencyKey == right.IdempotencyKey && left.CommandInputHash == right.CommandInputHash &&
		left.TemporalInputHash == right.TemporalInputHash
}

func sameNodeIdentities(left, right []domain.NodeRunProjection) bool {
	if len(left) != len(right) {
		return false
	}
	leftCopy := append([]domain.NodeRunProjection(nil), left...)
	rightCopy := append([]domain.NodeRunProjection(nil), right...)
	slices.SortFunc(leftCopy, func(a, b domain.NodeRunProjection) int { return strings.Compare(a.NodeID, b.NodeID) })
	slices.SortFunc(rightCopy, func(a, b domain.NodeRunProjection) int { return strings.Compare(a.NodeID, b.NodeID) })
	for index := range leftCopy {
		if leftCopy[index].ID != rightCopy[index].ID || leftCopy[index].WorkspaceID != rightCopy[index].WorkspaceID ||
			leftCopy[index].WorkflowRunID != rightCopy[index].WorkflowRunID || leftCopy[index].NodeID != rightCopy[index].NodeID ||
			leftCopy[index].DefinitionKey != rightCopy[index].DefinitionKey || leftCopy[index].DefinitionVersion != rightCopy[index].DefinitionVersion ||
			leftCopy[index].Executor != rightCopy[index].Executor || leftCopy[index].RiskLevel != rightCopy[index].RiskLevel ||
			!equalOptionalString(leftCopy[index].ReusedFromNodeRunID, rightCopy[index].ReusedFromNodeRunID) {
			return false
		}
	}
	return true
}
