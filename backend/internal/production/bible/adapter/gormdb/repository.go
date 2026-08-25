package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type Store struct{ database *gorm.DB }
type repository struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (store *Store) WithinTransaction(ctx context.Context, operation func(application.Repository) error) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&repository{database: transaction})
	})
}

func (repo *repository) RevisionInput(ctx context.Context, actor application.Actor, revisionID string, write bool) (application.RevisionInput, error) {
	id, err := uuid.Parse(revisionID)
	if err != nil {
		return application.RevisionInput{}, application.ErrNotFound
	}
	var revision model.DocumentRevision
	if err = repo.database.WithContext(ctx).First(&revision, "id = ?", id).Error; err != nil {
		return application.RevisionInput{}, normalizeNotFound(err)
	}
	var document model.ScriptDocument
	if err = repo.database.WithContext(ctx).First(&document, "id = ?", revision.DocumentID).Error; err != nil {
		return application.RevisionInput{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, document.ProjectID, write); err != nil {
		return application.RevisionInput{}, err
	}
	return application.RevisionInput{ID: revision.ID.String(), WorkspaceID: revision.WorkspaceID.String(), ProjectID: document.ProjectID.String(), NormalizedText: revision.NormalizedText, NormalizedHash: revision.NormalizedHash}, nil
}

func (repo *repository) FindReceipt(ctx context.Context, workspaceID, operation, key string) (platformcommand.Receipt, error) {
	id, err := uuid.Parse(workspaceID)
	if err != nil {
		return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
	}
	var record model.CommandReceipt
	if err = repo.database.WithContext(ctx).Where("workspace_id = ? AND operation = ? AND idempotency_key = ?", id, operation, key).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
		}
		return platformcommand.Receipt{}, err
	}
	return platformcommand.Receipt{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), Operation: record.Operation, IdempotencyKey: record.IdempotencyKey, InputHash: record.InputHash, ResourceID: record.ResourceID.String(), Result: append([]byte(nil), record.Result...), CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt}, nil
}

func (repo *repository) CreateReceipt(ctx context.Context, receipt platformcommand.Receipt) error {
	id, err := uuid.Parse(receipt.ID)
	if err != nil {
		return err
	}
	workspaceID, err := uuid.Parse(receipt.WorkspaceID)
	if err != nil {
		return err
	}
	resourceID, err := uuid.Parse(receipt.ResourceID)
	if err != nil {
		return err
	}
	createdBy, err := uuid.Parse(receipt.CreatedBy)
	if err != nil {
		return err
	}
	record := model.CommandReceipt{ID: id, WorkspaceID: workspaceID, Operation: receipt.Operation, IdempotencyKey: receipt.IdempotencyKey, InputHash: receipt.InputHash, ResourceID: resourceID, Result: datatypes.JSON(receipt.Result), CreatedBy: createdBy, CreatedAt: receipt.CreatedAt}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return &application.Error{Code: "resource_conflict", Message: "Idempotency key is already in use", Status: 409}
		}
		return err
	}
	return nil
}

func (repo *repository) CreateWorkflow(ctx context.Context, bible domain.Bible, invocation domain.Invocation) error {
	bibleRecord, err := bibleRecord(bible)
	if err != nil {
		return err
	}
	invocationRecord, err := invocationRecord(invocation)
	if err != nil {
		return err
	}
	scope, err := json.Marshal(map[string]any{"workspace_id": bible.WorkspaceID, "project_id": bible.ProjectID, "document_revision_id": bible.DocumentRevisionID})
	if err != nil {
		return err
	}
	task := model.WorkflowTask{ID: bibleRecord.TaskID, WorkspaceID: bibleRecord.WorkspaceID, TaskType: "production_bible", RequestType: "production_bible", RequestID: bibleRecord.ID, Scope: datatypes.JSON(scope), Status: "queued", ProgressStage: "queued", CancelStatus: "none", Revision: 1, CreatedAt: bible.CreatedAt, UpdatedAt: bible.UpdatedAt}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&task).Error; err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&bibleRecord).Error; err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&invocationRecord).Error
}

func (repo *repository) GetBible(ctx context.Context, actor application.Actor, bibleID string, forUpdate bool) (domain.Bible, error) {
	id, err := uuid.Parse(bibleID)
	if err != nil {
		return domain.Bible{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx).Where("id = ?", id)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.ProductionBible
	if err = query.First(&record).Error; err != nil {
		return domain.Bible{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, record.ProjectID, forUpdate); err != nil {
		return domain.Bible{}, err
	}
	return bibleDomain(record)
}

func (repo *repository) GetCurrentBible(ctx context.Context, actor application.Actor, projectID string) (domain.Bible, error) {
	id, err := uuid.Parse(projectID)
	if err != nil {
		return domain.Bible{}, application.ErrNotFound
	}
	if err = authorizeProject(ctx, repo.database, actor, id, false); err != nil {
		return domain.Bible{}, err
	}
	var record model.ProductionBible
	err = repo.database.WithContext(ctx).Where("project_id = ? AND status <> ?", id, "superseded").Order("created_at DESC").Order("id DESC").First(&record).Error
	if err != nil {
		return domain.Bible{}, normalizeNotFound(err)
	}
	return bibleDomain(record)
}

func (repo *repository) ConfirmBible(ctx context.Context, bible domain.Bible) error {
	record, err := bibleRecord(bible)
	if err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Model(&model.ProductionBible{}).Where("project_id = ? AND status = ? AND id <> ?", record.ProjectID, "confirmed", record.ID).Updates(map[string]any{"status": "superseded", "updated_at": record.UpdatedAt, "revision": gorm.Expr("revision + 1")}).Error; err != nil {
		return err
	}
	result := repo.database.WithContext(ctx).Model(&model.ProductionBible{}).Where("id = ?", record.ID).Updates(map[string]any{"status": record.Status, "confirmed_at": record.ConfirmedAt, "confirmed_by": record.ConfirmedBy, "revision": record.Revision, "updated_at": record.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return application.ErrNotFound
	}
	return nil
}

func (repo *repository) UpdateReviewDecisions(ctx context.Context, bible domain.Bible) error {
	id, err := uuid.Parse(bible.ID)
	if err != nil {
		return application.ErrNotFound
	}
	decisions, err := json.Marshal(bible.ReviewDecisions)
	if err != nil {
		return err
	}
	result := repo.database.WithContext(ctx).Model(&model.ProductionBible{}).Where("id = ?", id).Updates(map[string]any{
		"review_decisions": datatypes.JSON(decisions),
		"revision":         bible.Revision,
		"updated_at":       bible.UpdatedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return application.ErrNotFound
	}
	return nil
}

func (repo *repository) ResumeBible(ctx context.Context, bible domain.Bible) error {
	record, err := bibleRecord(bible)
	if err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Model(&model.ProductionBible{}).Where("id = ?", record.ID).Updates(map[string]any{"status": "queued", "result_hash": nil, "error": nil, "revision": record.Revision, "updated_at": record.UpdatedAt}).Error; err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Model(&model.WorkflowTask{}).Where("request_type = ? AND request_id = ?", "production_bible", record.ID).Updates(map[string]any{"status": "queued", "progress_stage": "queued", "error": nil, "next_action": nil, "revision": gorm.Expr("revision + 1"), "updated_at": record.UpdatedAt}).Error; err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Model(&model.AgentInvocation{}).Where("request_type = ? AND request_id = ?", "production_bible", record.ID).Updates(map[string]any{"status": "queued", "result_hash": nil, "error": nil, "started_at": nil, "completed_at": nil, "updated_at": record.UpdatedAt}).Error
}

func (store *Store) ClaimNext(ctx context.Context, now time.Time) (domain.Invocation, bool, error) {
	var result domain.Invocation
	found := false
	err := platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var record model.AgentInvocation
		err := transaction.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status = ? AND kind = ?", "queued", "production_bible").Order("created_at").First(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err = transaction.Model(&record).Updates(map[string]any{"status": "running", "attempts": gorm.Expr("attempts + 1"), "started_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if err = transaction.Model(&model.ProductionBible{}).Where("id = ?", record.RequestID).Updates(map[string]any{"status": "running", "checkpoint_stage": "agent_invocation", "checkpoint_revision": gorm.Expr("checkpoint_revision + 1"), "checkpoint_updated_at": now, "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error; err != nil {
			return err
		}
		if err = transaction.Model(&model.WorkflowTask{}).Where("request_type = ? AND request_id = ?", record.RequestType, record.RequestID).Updates(map[string]any{"status": "running", "progress_stage": "agent_invocation", "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error; err != nil {
			return err
		}
		record.Status, record.StartedAt, record.Attempts = "running", &now, record.Attempts+1
		result = invocationDomain(record)
		found = true
		return nil
	})
	return result, found, err
}

func (store *Store) InvocationSource(ctx context.Context, invocationID string) (string, error) {
	id, err := uuid.Parse(invocationID)
	if err != nil {
		return "", application.ErrNotFound
	}
	var invocation model.AgentInvocation
	if err = store.database.WithContext(ctx).First(&invocation, "id = ?", id).Error; err != nil {
		return "", normalizeNotFound(err)
	}
	var bible model.ProductionBible
	if err = store.database.WithContext(ctx).First(&bible, "id = ?", invocation.RequestID).Error; err != nil {
		return "", normalizeNotFound(err)
	}
	var revision model.DocumentRevision
	if err = store.database.WithContext(ctx).First(&revision, "id = ?", bible.DocumentRevisionID).Error; err != nil {
		return "", normalizeNotFound(err)
	}
	return revision.NormalizedText, nil
}

func (store *Store) CompleteInvocation(ctx context.Context, invocationID string, result contract.Result, now time.Time) error {
	id, err := uuid.Parse(invocationID)
	if err != nil {
		return application.ErrNotFound
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var invocation model.AgentInvocation
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&invocation, "id = ?", id).Error; err != nil {
			return normalizeNotFound(err)
		}
		if invocation.Status != "running" {
			return fmt.Errorf("agent invocation is not running")
		}
		if err := transaction.Model(&invocation).Updates(map[string]any{"status": "succeeded", "result_hash": result.ResultHash, "error": nil, "completed_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		stage := "candidate_ready"
		if err := transaction.Model(&model.ProductionBible{}).Where("id = ?", invocation.RequestID).Updates(map[string]any{"status": "needs_review", "result_hash": result.ResultHash, "candidate": datatypes.JSON(result.Candidate), "error": nil, "model_name": result.Executor.Model, "harness_version": result.Executor.Version, "checkpoint_stage": stage, "checkpoint_revision": gorm.Expr("checkpoint_revision + 1"), "checkpoint_updated_at": now, "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error; err != nil {
			return err
		}
		return transaction.Model(&model.WorkflowTask{}).Where("request_type = ? AND request_id = ?", invocation.RequestType, invocation.RequestID).Updates(map[string]any{"status": "succeeded", "progress_stage": stage, "error": nil, "next_action": nil, "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error
	})
}

func (store *Store) FailInvocation(ctx context.Context, invocationID, outcome, code, summary string, retryable bool, now time.Time) error {
	if outcome != "failed" && outcome != "unknown" {
		outcome = "unknown"
	}
	id, err := uuid.Parse(invocationID)
	if err != nil {
		return application.ErrNotFound
	}
	errorJSON, err := json.Marshal(map[string]any{"code": code, "summary": summary, "retryable": retryable})
	if err != nil {
		return err
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var invocation model.AgentInvocation
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&invocation, "id = ?", id).Error; err != nil {
			return normalizeNotFound(err)
		}
		if err := transaction.Model(&invocation).Updates(map[string]any{"status": outcome, "error": datatypes.JSON(errorJSON), "completed_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		nextAction := "retry_agent"
		if !retryable {
			nextAction = "review_input"
		}
		if err := transaction.Model(&model.ProductionBible{}).Where("id = ?", invocation.RequestID).Updates(map[string]any{"status": outcome, "error": datatypes.JSON(errorJSON), "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error; err != nil {
			return err
		}
		return transaction.Model(&model.WorkflowTask{}).Where("request_type = ? AND request_id = ?", invocation.RequestType, invocation.RequestID).Updates(map[string]any{"status": outcome, "progress_stage": "agent_result", "error": datatypes.JSON(errorJSON), "next_action": nextAction, "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error
	})
}

func authorizeProject(ctx context.Context, database *gorm.DB, actor application.Actor, projectID uuid.UUID, write bool) error {
	userID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return unauthenticated()
	}
	var user model.UserAccount
	if err = database.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil || user.Status != "active" || user.TokenVersion != actor.TokenVersion {
		return unauthenticated()
	}
	var project model.Project
	if err = database.WithContext(ctx).First(&project, "id = ?", projectID).Error; err != nil {
		return application.ErrNotFound
	}
	var workspace model.Workspace
	if err = database.WithContext(ctx).First(&workspace, "id = ?", project.WorkspaceID).Error; err != nil {
		return application.ErrNotFound
	}
	var membership model.Membership
	if err = database.WithContext(ctx).Where("workspace_id = ? AND user_id = ? AND status = ?", project.WorkspaceID, userID, "active").First(&membership).Error; err != nil {
		return application.ErrNotFound
	}
	if write && (membership.Role == "viewer" || workspace.Status != "active" || project.Status != "active") {
		return &application.Error{Code: "forbidden", Message: "Action is not allowed", Status: 403}
	}
	return nil
}

func bibleRecord(value domain.Bible) (model.ProductionBible, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.ProductionBible{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.ProductionBible{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.ProductionBible{}, err
	}
	revisionID, err := uuid.Parse(value.DocumentRevisionID)
	if err != nil {
		return model.ProductionBible{}, err
	}
	taskID, err := uuid.Parse(value.TaskID)
	if err != nil {
		return model.ProductionBible{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.ProductionBible{}, err
	}
	var confirmedBy *uuid.UUID
	if value.ConfirmedBy != nil {
		parsed, parseErr := uuid.Parse(*value.ConfirmedBy)
		if parseErr != nil {
			return model.ProductionBible{}, parseErr
		}
		confirmedBy = &parsed
	}
	candidate, err := json.Marshal(value.Candidate)
	if err != nil {
		return model.ProductionBible{}, err
	}
	reviewDecisions, err := json.Marshal(value.ReviewDecisions)
	if err != nil {
		return model.ProductionBible{}, err
	}
	return model.ProductionBible{ID: id, WorkspaceID: workspaceID, ProjectID: projectID, DocumentRevisionID: revisionID, TaskID: taskID, Status: value.Status, InputHash: value.InputHash, ResultHash: value.ResultHash, EngineVersion: value.EngineVersion, ModelName: value.ModelName, PromptVersion: value.PromptVersion, SchemaVersion: value.SchemaVersion, HarnessVersion: value.HarnessVersion, CheckpointStage: value.CheckpointStage, CheckpointRevision: value.CheckpointRevision, CheckpointUpdatedAt: value.CheckpointUpdatedAt, Candidate: datatypes.JSON(candidate), ReviewDecisions: datatypes.JSON(reviewDecisions), Error: datatypes.JSON(value.Error), Revision: value.Revision, ConfirmedAt: value.ConfirmedAt, ConfirmedBy: confirmedBy, CreatedBy: createdBy, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, nil
}

func bibleDomain(record model.ProductionBible) (domain.Bible, error) {
	candidate := domain.Candidate{Entities: []domain.Entity{}, WorldEntries: []domain.WorldEntry{}, ReviewIssues: []domain.ReviewIssue{}}
	if len(record.Candidate) > 0 {
		if err := json.Unmarshal(record.Candidate, &candidate); err != nil {
			return domain.Bible{}, err
		}
	}
	reviewDecisions := map[string]string{}
	if len(record.ReviewDecisions) > 0 {
		if err := json.Unmarshal(record.ReviewDecisions, &reviewDecisions); err != nil {
			return domain.Bible{}, err
		}
	}
	var confirmedBy *string
	if record.ConfirmedBy != nil {
		value := record.ConfirmedBy.String()
		confirmedBy = &value
	}
	return domain.Bible{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(), DocumentRevisionID: record.DocumentRevisionID.String(), TaskID: record.TaskID.String(), Status: record.Status, InputHash: record.InputHash, ResultHash: record.ResultHash, EngineVersion: record.EngineVersion, ModelName: record.ModelName, PromptVersion: record.PromptVersion, SchemaVersion: record.SchemaVersion, HarnessVersion: record.HarnessVersion, CheckpointStage: record.CheckpointStage, CheckpointRevision: record.CheckpointRevision, CheckpointUpdatedAt: record.CheckpointUpdatedAt, Candidate: candidate, ReviewDecisions: reviewDecisions, Error: append([]byte(nil), record.Error...), Revision: record.Revision, ConfirmedAt: record.ConfirmedAt, ConfirmedBy: confirmedBy, CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, nil
}

func invocationRecord(value domain.Invocation) (model.AgentInvocation, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	requestID, err := uuid.Parse(value.RequestID)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	return model.AgentInvocation{ID: id, WorkspaceID: workspaceID, RequestType: "production_bible", RequestID: requestID, Kind: value.Kind, InputHash: value.InputHash, Payload: datatypes.JSON(value.Payload), Status: value.Status, Attempts: value.Attempts, CreatedAt: value.CreatedAt, UpdatedAt: value.CreatedAt}, nil
}

func invocationDomain(record model.AgentInvocation) domain.Invocation {
	return domain.Invocation{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), RequestID: record.RequestID.String(), Kind: record.Kind, InputHash: record.InputHash, Payload: append([]byte(nil), record.Payload...), Status: record.Status, Attempts: record.Attempts, CreatedAt: record.CreatedAt}
}
func normalizeNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrNotFound
	}
	return err
}
func unauthenticated() error {
	return &application.Error{Code: "unauthenticated", Message: "Invalid credentials", Status: 401, NextAction: "login"}
}
