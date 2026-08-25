package gormdb

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func (store *Store) PrepareControl(
	ctx context.Context,
	desired domain.ControlPreparation,
) (domain.ControlPreparation, error) {
	intentRecord, err := controlIntentRecord(desired.Intent)
	if err != nil {
		return domain.ControlPreparation{}, err
	}
	var prepared domain.ControlPreparation
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var existing model.WorkflowControlIntent
		existingErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND idempotency_key = ?", intentRecord.WorkspaceID, intentRecord.IdempotencyKey).
			First(&existing).Error
		if existingErr == nil {
			return loadControlPreparation(transaction, existing, &prepared)
		}
		if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return existingErr
		}
		var run model.WorkflowRun
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&run, "id = ?", intentRecord.WorkflowRunID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if run.WorkspaceID != intentRecord.WorkspaceID || run.Revision != intentRecord.ExpectedRunRevision ||
			!cancellableRunStatus(run.Status) {
			return &application.Error{Code: "resource_conflict", Message: "Workflow run is not cancellable at the expected revision", Status: 409}
		}
		intentRecord.TemporalWorkflowID = run.TemporalWorkflowID
		request, requestErr := application.NewControlRequest(controlIntentDomain(intentRecord))
		if requestErr != nil {
			return requestErr
		}
		intentRecord.InputHash = request.InputHash
		if createErr := transaction.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "workflow_run_id"}, {Name: "action"}}, DoNothing: true,
		}).Omit(clause.Associations).Create(&intentRecord).Error; createErr != nil {
			return createErr
		}
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workflow_run_id = ? AND action = ?", intentRecord.WorkflowRunID, intentRecord.Action).
			First(&intentRecord).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if !sameControlIntentIdentity(intentRecord, desired.Intent) {
			return errors.New("persisted workflow control intent identity has drifted")
		}
		prepared = domain.ControlPreparation{Run: runDomain(run), Intent: controlIntentDomain(intentRecord)}
		return nil
	})
	return prepared, err
}

func (store *Store) BeginControlAttempt(
	ctx context.Context,
	intentID string,
	now time.Time,
) (domain.ControlPreparation, error) {
	id, err := uuid.Parse(intentID)
	if err != nil {
		return domain.ControlPreparation{}, application.ErrNotFound
	}
	var prepared domain.ControlPreparation
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var intent model.WorkflowControlIntent
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&intent, "id = ?", id).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var run model.WorkflowRun
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", intent.WorkflowRunID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if intent.Status != "completed" && intent.Status != "conflict" {
			intent.Status = "pending"
			intent.AttemptNo++
			intent.Revision++
			intent.UpdatedAt = now.UTC()
			if updateErr := transaction.Model(&model.WorkflowControlIntent{}).Where("id = ?", intent.ID).Updates(map[string]any{
				"status": intent.Status, "attempt_no": intent.AttemptNo,
				"revision": intent.Revision, "updated_at": intent.UpdatedAt,
			}).Error; updateErr != nil {
				return updateErr
			}
		}
		prepared = domain.ControlPreparation{Run: runDomain(run), Intent: controlIntentDomain(intent)}
		return nil
	})
	return prepared, err
}

func (store *Store) FinalizeControlAttempt(ctx context.Context, finalization domain.ControlFinalization) error {
	intent, err := controlIntentRecord(finalization.Intent)
	if err != nil {
		return err
	}
	receipt, err := controlReceiptRecord(finalization.Receipt)
	if err != nil {
		return err
	}
	run, err := runRecord(finalization.Run)
	if err != nil {
		return err
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		runResult := transaction.Model(&model.WorkflowRun{}).
			Where("id = ? AND revision = ?", run.ID, finalization.ExpectedRunRevision).
			Updates(map[string]any{
				"status": run.Status, "progress_stage": run.ProgressStage, "next_action": run.NextAction,
				"error": run.Error, "revision": run.Revision, "updated_at": run.UpdatedAt,
			})
		if runResult.Error != nil {
			return runResult.Error
		}
		if runResult.RowsAffected != 1 {
			return &application.Error{Code: "resource_conflict", Message: "Workflow run changed before control receipt", Status: 409}
		}
		intentResult := transaction.Model(&model.WorkflowControlIntent{}).
			Where("id = ? AND revision = ?", intent.ID, finalization.ExpectedIntentRevision).
			Updates(map[string]any{
				"status": intent.Status, "attempt_no": intent.AttemptNo,
				"revision": intent.Revision, "updated_at": intent.UpdatedAt,
			})
		if intentResult.Error != nil {
			return intentResult.Error
		}
		if intentResult.RowsAffected != 1 {
			return &application.Error{Code: "resource_conflict", Message: "Workflow control intent changed before receipt", Status: 409}
		}
		if finalization.CancelNonTerminalNodeRuns {
			if updateErr := transaction.Model(&model.NodeRunProjection{}).
				Where("workflow_run_id = ? AND status IN ?", run.ID, []string{"QUEUED", "RUNNING", "WAITING_HUMAN", "RETRYING"}).
				Updates(map[string]any{
					"status": "CANCELLED", "active_claim_token": nil,
					"revision": gorm.Expr("revision + ?", 1), "updated_at": run.UpdatedAt,
				}).Error; updateErr != nil {
				return updateErr
			}
		}
		if createErr := transaction.Omit(clause.Associations).Create(&receipt).Error; createErr != nil {
			if errors.Is(createErr, gorm.ErrDuplicatedKey) {
				return &application.Error{Code: "resource_conflict", Message: "Workflow control attempt already has a receipt", Status: 409}
			}
			return createErr
		}
		return nil
	})
}

func loadControlPreparation(
	transaction *gorm.DB,
	intent model.WorkflowControlIntent,
	prepared *domain.ControlPreparation,
) error {
	var run model.WorkflowRun
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", intent.WorkflowRunID).Error; err != nil {
		return normalizeNotFound(err)
	}
	*prepared = domain.ControlPreparation{Run: runDomain(run), Intent: controlIntentDomain(intent)}
	return nil
}

func controlIntentRecord(value domain.ControlIntent) (model.WorkflowControlIntent, error) {
	id, workspaceID, runID, createdBy, err := controlIDs(value.ID, value.WorkspaceID, value.WorkflowRunID, value.CreatedBy)
	if err != nil {
		return model.WorkflowControlIntent{}, err
	}
	controlID, err := uuid.Parse(value.ControlID)
	if err != nil {
		return model.WorkflowControlIntent{}, err
	}
	return model.WorkflowControlIntent{
		ID: id, WorkspaceID: workspaceID, WorkflowRunID: runID,
		IdempotencyKey: value.IdempotencyKey, CommandInputHash: value.CommandInputHash,
		TemporalWorkflowID: value.TemporalWorkflowID, ControlID: controlID,
		InputHash: value.InputHash, Action: value.Action, ExpectedRunRevision: value.ExpectedRunRevision,
		Status: value.Status, AttemptNo: value.AttemptNo, Revision: value.Revision,
		CreatedBy: createdBy, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func controlReceiptRecord(value domain.ControlReceipt) (model.WorkflowControlReceipt, error) {
	id, workspaceID, intentID, runID, err := controlIDs(value.ID, value.WorkspaceID, value.ControlIntentID, value.WorkflowRunID)
	if err != nil {
		return model.WorkflowControlReceipt{}, err
	}
	controlID, err := uuid.Parse(value.ControlID)
	if err != nil {
		return model.WorkflowControlReceipt{}, err
	}
	return model.WorkflowControlReceipt{
		ID: id, WorkspaceID: workspaceID, ControlIntentID: intentID, WorkflowRunID: runID,
		AttemptNo: value.AttemptNo, Outcome: value.Outcome, ControlID: controlID,
		ExpectedInputHash: value.ExpectedInputHash, ObservedInputHash: value.ObservedInputHash, CreatedAt: value.CreatedAt,
	}, nil
}

func controlIntentDomain(value model.WorkflowControlIntent) domain.ControlIntent {
	return domain.ControlIntent{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), WorkflowRunID: value.WorkflowRunID.String(),
		IdempotencyKey: value.IdempotencyKey, CommandInputHash: value.CommandInputHash,
		TemporalWorkflowID: value.TemporalWorkflowID, ControlID: value.ControlID.String(),
		InputHash: value.InputHash, Action: value.Action, ExpectedRunRevision: value.ExpectedRunRevision,
		Status: value.Status, AttemptNo: value.AttemptNo, Revision: value.Revision,
		CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func sameControlIntentIdentity(record model.WorkflowControlIntent, desired domain.ControlIntent) bool {
	return record.ID.String() == desired.ID && record.WorkspaceID.String() == desired.WorkspaceID &&
		record.WorkflowRunID.String() == desired.WorkflowRunID && record.CommandInputHash == desired.CommandInputHash &&
		record.ControlID.String() == desired.ControlID && record.Action == desired.Action &&
		record.ExpectedRunRevision == desired.ExpectedRunRevision && record.CreatedBy.String() == desired.CreatedBy
}

func cancellableRunStatus(status string) bool {
	switch status {
	case "QUEUED", "RUNNING", "WAITING_HUMAN", "RETRYING", "PAUSED", "NEEDS_ATTENTION":
		return true
	default:
		return false
	}
}

func controlIDs(values ...string) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, error) {
	ids := make([]uuid.UUID, len(values))
	for index, value := range values {
		parsed, err := uuid.Parse(value)
		if err != nil {
			return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, err
		}
		ids[index] = parsed
	}
	return ids[0], ids[1], ids[2], ids[3], nil
}

var _ application.ControlRepository = (*Store)(nil)
