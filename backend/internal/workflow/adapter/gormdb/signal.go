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

func (store *Store) PrepareSignal(ctx context.Context, desired domain.SignalPreparation) (domain.SignalPreparation, error) {
	applyRecord, err := signalApplyRecord(desired.ApplyReceipt)
	if err != nil {
		return domain.SignalPreparation{}, err
	}
	intentRecord, err := signalIntentRecord(desired.Intent, applyRecord.ID)
	if err != nil {
		return domain.SignalPreparation{}, err
	}
	var prepared domain.SignalPreparation
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var existingIntent model.WorkflowSignalIntent
		existingErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND idempotency_key = ?", intentRecord.WorkspaceID, intentRecord.IdempotencyKey).
			First(&existingIntent).Error
		if existingErr == nil {
			var existingApply model.WorkflowHumanGateApplyReceipt
			if loadErr := transaction.First(&existingApply, "id = ?", existingIntent.ApplyReceiptID).Error; loadErr != nil {
				return normalizeNotFound(loadErr)
			}
			prepared = domain.SignalPreparation{
				ApplyReceipt: signalApplyDomain(existingApply), Intent: signalIntentDomain(existingIntent),
			}
			return nil
		}
		if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return existingErr
		}
		var run model.WorkflowRun
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", intentRecord.WorkflowRunID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var node model.NodeRunProjection
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&node, "id = ?", intentRecord.NodeRunID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if run.WorkspaceID != intentRecord.WorkspaceID || node.WorkflowRunID != run.ID || node.Status != "WAITING_HUMAN" ||
			run.Status != "WAITING_HUMAN" || node.Revision != intentRecord.SubjectRevision {
			return errors.New("workflow human gate changed before signal preparation")
		}
		intentRecord.TemporalWorkflowID = run.TemporalWorkflowID
		temporary := signalIntentDomain(intentRecord)
		request, requestErr := application.NewSignalRequest(temporary)
		if requestErr != nil {
			return requestErr
		}
		intentRecord.InputHash = request.InputHash
		if createErr := transaction.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "review_decision_id"}}, DoNothing: true,
		}).Omit(clause.Associations).Create(&applyRecord).Error; createErr != nil {
			return createErr
		}
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("review_decision_id = ?", applyRecord.ReviewDecisionID).First(&applyRecord).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if !sameSignalApply(applyRecord, desired.ApplyReceipt) {
			return errors.New("persisted human gate apply receipt has drifted")
		}
		intentRecord.ApplyReceiptID = applyRecord.ID
		if createErr := transaction.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "workspace_id"}, {Name: "idempotency_key"}}, DoNothing: true,
		}).Omit(clause.Associations).Create(&intentRecord).Error; createErr != nil {
			return createErr
		}
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND idempotency_key = ?", intentRecord.WorkspaceID, intentRecord.IdempotencyKey).
			First(&intentRecord).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		prepared = domain.SignalPreparation{ApplyReceipt: signalApplyDomain(applyRecord), Intent: signalIntentDomain(intentRecord)}
		return nil
	})
	return prepared, err
}

func (store *Store) BeginSignalAttempt(ctx context.Context, intentID string, now time.Time) (domain.SignalPreparation, error) {
	id, err := uuid.Parse(intentID)
	if err != nil {
		return domain.SignalPreparation{}, application.ErrNotFound
	}
	var prepared domain.SignalPreparation
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var intent model.WorkflowSignalIntent
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&intent, "id = ?", id).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var apply model.WorkflowHumanGateApplyReceipt
		if loadErr := transaction.First(&apply, "id = ?", intent.ApplyReceiptID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if intent.Status != "completed" && intent.Status != "conflict" {
			intent.Status = "pending"
			intent.AttemptNo++
			intent.Revision++
			intent.UpdatedAt = now.UTC()
			if updateErr := transaction.Model(&model.WorkflowSignalIntent{}).Where("id = ?", intent.ID).Updates(map[string]any{
				"status": intent.Status, "attempt_no": intent.AttemptNo,
				"revision": intent.Revision, "updated_at": intent.UpdatedAt,
			}).Error; updateErr != nil {
				return updateErr
			}
		}
		prepared = domain.SignalPreparation{ApplyReceipt: signalApplyDomain(apply), Intent: signalIntentDomain(intent)}
		return nil
	})
	return prepared, err
}

func (store *Store) FinalizeSignalAttempt(
	ctx context.Context,
	intent domain.SignalIntent,
	receipt domain.SignalReceipt,
	expectedRevision int,
) error {
	intentRecord, err := signalIntentRecord(intent, uuid.Nil)
	if err != nil {
		return err
	}
	receiptRecord, err := signalReceiptRecord(receipt)
	if err != nil {
		return err
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		result := transaction.Model(&model.WorkflowSignalIntent{}).
			Where("id = ? AND revision = ?", intentRecord.ID, expectedRevision).
			Updates(map[string]any{
				"status": intentRecord.Status, "attempt_no": intentRecord.AttemptNo,
				"revision": intentRecord.Revision, "updated_at": intentRecord.UpdatedAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return &application.Error{Code: "resource_conflict", Message: "Workflow signal intent changed before receipt", Status: 409}
		}
		if createErr := transaction.Omit(clause.Associations).Create(&receiptRecord).Error; createErr != nil {
			if errors.Is(createErr, gorm.ErrDuplicatedKey) {
				return &application.Error{Code: "resource_conflict", Message: "Workflow signal attempt already has a receipt", Status: 409}
			}
			return createErr
		}
		return nil
	})
}

func signalApplyRecord(value domain.HumanGateApplyReceipt) (model.WorkflowHumanGateApplyReceipt, error) {
	id, workspaceID, runID, nodeID, taskID, decisionID, createdBy, err := signalIDs(
		value.ID, value.WorkspaceID, value.WorkflowRunID, value.NodeRunID, value.HumanTaskID, value.ReviewDecisionID, value.CreatedBy,
	)
	if err != nil {
		return model.WorkflowHumanGateApplyReceipt{}, err
	}
	return model.WorkflowHumanGateApplyReceipt{
		ID: id, WorkspaceID: workspaceID, WorkflowRunID: runID, NodeRunID: nodeID,
		HumanTaskID: taskID, ReviewDecisionID: decisionID, SubjectRevision: value.SubjectRevision,
		Decision: value.Decision, CreatedBy: createdBy, CreatedAt: value.CreatedAt,
	}, nil
}

func signalIntentRecord(value domain.SignalIntent, applyReceiptID uuid.UUID) (model.WorkflowSignalIntent, error) {
	id, workspaceID, runID, nodeID, taskID, decisionID, createdBy, err := signalIDs(
		value.ID, value.WorkspaceID, value.WorkflowRunID, value.NodeRunID, value.HumanTaskID, value.ReviewDecisionID, value.CreatedBy,
	)
	if err != nil {
		return model.WorkflowSignalIntent{}, err
	}
	return model.WorkflowSignalIntent{
		ID: id, WorkspaceID: workspaceID, WorkflowRunID: runID, NodeRunID: nodeID,
		HumanTaskID: taskID, ReviewDecisionID: decisionID, ApplyReceiptID: applyReceiptID,
		IdempotencyKey: value.IdempotencyKey, CommandInputHash: value.CommandInputHash,
		TemporalWorkflowID: value.TemporalWorkflowID, SignalID: value.SignalID, InputHash: value.InputHash,
		Decision: value.Decision, SubjectRevision: value.SubjectRevision, Status: value.Status,
		AttemptNo: value.AttemptNo, Revision: value.Revision, CreatedBy: createdBy,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func signalReceiptRecord(value domain.SignalReceipt) (model.WorkflowSignalReceipt, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.WorkflowSignalReceipt{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.WorkflowSignalReceipt{}, err
	}
	intentID, err := uuid.Parse(value.SignalIntentID)
	if err != nil {
		return model.WorkflowSignalReceipt{}, err
	}
	runID, err := uuid.Parse(value.WorkflowRunID)
	if err != nil {
		return model.WorkflowSignalReceipt{}, err
	}
	return model.WorkflowSignalReceipt{
		ID: id, WorkspaceID: workspaceID, SignalIntentID: intentID, WorkflowRunID: runID,
		AttemptNo: value.AttemptNo, Outcome: value.Outcome, SignalID: value.SignalID,
		ExpectedInputHash: value.ExpectedInputHash, ObservedInputHash: value.ObservedInputHash, CreatedAt: value.CreatedAt,
	}, nil
}

func signalApplyDomain(value model.WorkflowHumanGateApplyReceipt) domain.HumanGateApplyReceipt {
	return domain.HumanGateApplyReceipt{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), WorkflowRunID: value.WorkflowRunID.String(),
		NodeRunID: value.NodeRunID.String(), HumanTaskID: value.HumanTaskID.String(), ReviewDecisionID: value.ReviewDecisionID.String(),
		SubjectRevision: value.SubjectRevision, Decision: value.Decision, CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt,
	}
}

func signalIntentDomain(value model.WorkflowSignalIntent) domain.SignalIntent {
	return domain.SignalIntent{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), WorkflowRunID: value.WorkflowRunID.String(),
		NodeRunID: value.NodeRunID.String(), HumanTaskID: value.HumanTaskID.String(), ReviewDecisionID: value.ReviewDecisionID.String(),
		IdempotencyKey: value.IdempotencyKey, CommandInputHash: value.CommandInputHash,
		TemporalWorkflowID: value.TemporalWorkflowID, SignalID: value.SignalID, InputHash: value.InputHash,
		Decision: value.Decision, SubjectRevision: value.SubjectRevision, Status: value.Status,
		AttemptNo: value.AttemptNo, Revision: value.Revision, CreatedBy: value.CreatedBy.String(),
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func sameSignalApply(record model.WorkflowHumanGateApplyReceipt, desired domain.HumanGateApplyReceipt) bool {
	return record.ID.String() == desired.ID && record.WorkspaceID.String() == desired.WorkspaceID &&
		record.WorkflowRunID.String() == desired.WorkflowRunID && record.NodeRunID.String() == desired.NodeRunID &&
		record.HumanTaskID.String() == desired.HumanTaskID && record.ReviewDecisionID.String() == desired.ReviewDecisionID &&
		record.SubjectRevision == desired.SubjectRevision && record.Decision == desired.Decision &&
		record.CreatedBy.String() == desired.CreatedBy
}

func signalIDs(values ...string) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, error) {
	ids := make([]uuid.UUID, len(values))
	for index, value := range values {
		parsed, err := uuid.Parse(value)
		if err != nil {
			return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, err
		}
		ids[index] = parsed
	}
	return ids[0], ids[1], ids[2], ids[3], ids[4], ids[5], ids[6], nil
}

var _ application.SignalRepository = (*Store)(nil)
