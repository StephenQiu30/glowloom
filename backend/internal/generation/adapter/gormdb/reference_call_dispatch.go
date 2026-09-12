package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func (store *Store) WithinReferenceCallDispatch(ctx context.Context, operation func(application.ReferenceCallDispatchRepository) error) error {
	if store == nil || store.database == nil || store.database.Config == nil || operation == nil || store.database.DisableNestedTransaction {
		return errors.New("Reference dispatch requires a database and nested transaction savepoints")
	}
	return platformdatabase.WithinSerializableTransaction(ctx, store.database, func(tx *gorm.DB) error {
		targets := &referenceTargetRepository{referenceAuthorizationRepository{repository{database: tx}}}
		return operation(&referenceExecutionRepository{referenceExecutionAuthorizationRepository{targets, &providerConfigurationRepository{database: tx}}, tx})
	})
}

func (store *Store) WithinReferenceCallExecution(ctx context.Context, operation func(application.ReferenceCallDispatchRepository) error) error {
	if store == nil || store.database == nil || store.database.Statement == nil {
		return errors.New("Reference execution requires a standalone database transaction")
	}
	if _, nested := store.database.Statement.ConnPool.(gorm.TxCommitter); nested {
		return errors.New("Reference execution cannot use an outer transaction or savepoint")
	}
	return store.WithinReferenceCallDispatch(ctx, operation)
}

func referenceCallStateFromRecord(record model.GenerationReferenceProviderCall) (domain.ReferenceCallState, error) {
	state, err := domain.DecodeReferenceCallState(json.RawMessage(record.StateContent))
	if err != nil {
		return domain.ReferenceCallState{}, err
	}
	if state.CallKey != record.CallKey || state.Status != record.Status || state.Revision != record.Revision || state.ContentHash != record.StateHash {
		return domain.ReferenceCallState{}, errors.New("Reference call runtime columns have drifted")
	}
	if state.Receipt != nil && (state.Receipt.WorkspaceID != record.WorkspaceID.String() || state.Receipt.ProjectID != record.ProjectID.String() || state.Receipt.Call.ExecutionRef.ID != record.ExecutionID.String()) {
		return domain.ReferenceCallState{}, errors.New("Reference receipt scope differs from persisted call")
	}
	if state.Dispatch == nil {
		if record.SubmissionToken != nil || record.DispatchedAt != nil {
			return domain.ReferenceCallState{}, errors.New("pending Reference call has dispatch metadata")
		}
	} else if record.SubmissionToken == nil || record.DispatchedAt == nil || record.SubmissionToken.String() != state.Dispatch.SubmissionToken || !record.DispatchedAt.Equal(state.Dispatch.DispatchedAt) {
		return domain.ReferenceCallState{}, errors.New("Reference call dispatch metadata has drifted")
	}
	return state, nil
}

func (repo *referenceExecutionRepository) FindReferenceCallState(ctx context.Context, workspace, project, executionID, key string) (domain.ReferenceCallState, error) {
	var record model.GenerationReferenceProviderCall
	if err := repo.database.WithContext(ctx).Where("workspace_id = ? AND project_id = ? AND execution_id = ? AND call_key = ?", workspace, project, executionID, key).First(&record).Error; err != nil {
		return domain.ReferenceCallState{}, err
	}
	call, err := domain.DecodeReferenceProviderCall(json.RawMessage(record.Content))
	if err != nil {
		return domain.ReferenceCallState{}, err
	}
	if call.CallKey != record.CallKey || call.ExecutionRef.ID != record.ExecutionID.String() || call.BundleIndex != record.BundleIndex || call.SlotKey != record.SlotKey || call.CompiledRequestHash != record.CompiledRequestHash {
		return domain.ReferenceCallState{}, errors.New("Reference call invocation identity has drifted")
	}
	return referenceCallStateFromRecord(record)
}

func (repo *referenceExecutionRepository) UpdateReferenceCallState(ctx context.Context, workspace, project, executionID string, before, after domain.ReferenceCallState) error {
	if err := domain.ValidateReferenceCallTransition(before, after); err != nil {
		return err
	}
	if after.Receipt != nil && (after.Receipt.WorkspaceID != workspace || after.Receipt.ProjectID != project || after.Receipt.Call.ExecutionRef.ID != executionID) {
		return errors.New("Reference receipt scope differs from call update")
	}
	raw, err := json.Marshal(after)
	if err != nil {
		return err
	}
	token, err := uuid.Parse(after.Dispatch.SubmissionToken)
	if err != nil {
		return err
	}
	updated := repo.database.WithContext(ctx).Model(&model.GenerationReferenceProviderCall{}).
		Where("workspace_id = ? AND project_id = ? AND execution_id = ? AND call_key = ? AND status = ? AND revision = ? AND state_hash = ?", workspace, project, executionID, before.CallKey, before.Status, before.Revision, before.ContentHash).
		UpdateColumns(map[string]any{"status": after.Status, "revision": after.Revision, "state_content": datatypes.JSON(raw), "state_hash": after.ContentHash, "submission_token": token, "dispatched_at": after.Dispatch.DispatchedAt})
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected != 1 {
		return application.ErrReferenceCallStateConflict
	}
	return nil
}

func (repo *referenceExecutionRepository) ReferenceCallDispatchUsage(ctx context.Context, workspace string, start, end time.Time) (application.ReferenceCallDispatchUsage, error) {
	var usage application.ReferenceCallDispatchUsage
	if start.IsZero() || !end.After(start) {
		return usage, errors.New("invalid Reference call dispatch accounting window")
	}
	if err := repo.database.WithContext(ctx).Model(&model.GenerationReferenceProviderCall{}).Where("workspace_id = ? AND status IN ?", workspace, []string{domain.ProviderCallDispatching, domain.ProviderCallOutcomeUnknown}).Count(&usage.Unresolved).Error; err != nil {
		return usage, err
	}
	if err := repo.database.WithContext(ctx).Model(&model.GenerationReferenceProviderCall{}).Where("workspace_id = ? AND dispatched_at >= ? AND dispatched_at < ?", workspace, start, end).Count(&usage.Daily).Error; err != nil {
		return usage, err
	}
	return usage, nil
}

var _ application.ReferenceCallDispatchTransactions = (*Store)(nil)
var _ application.ReferenceCallExecutionTransactions = (*Store)(nil)
