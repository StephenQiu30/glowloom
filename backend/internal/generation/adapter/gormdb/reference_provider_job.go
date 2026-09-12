package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

func (repo *referenceExecutionRepository) PublishReferenceProviderJob(ctx context.Context, workspace, project string, job domain.ReferenceProviderJob, calls []domain.ReferenceProviderCall) error {
	execution, err := repo.FindReferenceExecution(ctx, workspace, project, job.ExecutionRef.ID)
	if err != nil {
		return err
	}
	if job.ExecutionRef != (domain.GenerationRevisionRef{ID: execution.ID, Revision: execution.Revision, ContentHash: execution.ContentHash}) {
		return errors.New("Reference Provider job execution identity has drifted")
	}
	if err = validateReferenceProviderCallSet(job, calls); err != nil {
		return err
	}
	raw, err := json.Marshal(job)
	if err != nil {
		return err
	}
	// Scope identities have already been strictly decoded by the execution reader.
	workspaceID, err := uuid.Parse(workspace)
	if err != nil {
		return err
	}
	projectID, err := uuid.Parse(project)
	if err != nil {
		return err
	}
	executionID, err := uuid.Parse(execution.ID)
	if err != nil {
		return err
	}
	record := model.GenerationReferenceProviderJob{ExecutionID: executionID, WorkspaceID: workspaceID, ProjectID: projectID, ExecutionHash: execution.ContentHash, CallSetRoot: job.CallSetRoot, ContentHash: job.ContentHash, Content: raw}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		return err
	}
	records := make([]model.GenerationReferenceProviderCall, len(calls))
	for i, call := range calls {
		raw, err := json.Marshal(call)
		if err != nil {
			return err
		}
		state, err := domain.NewReferenceCallState(call.CallKey)
		if err != nil {
			return err
		}
		stateRaw, err := json.Marshal(state)
		if err != nil {
			return err
		}
		records[i] = model.GenerationReferenceProviderCall{CallKey: call.CallKey, ExecutionID: executionID, WorkspaceID: workspaceID, ProjectID: projectID, BundleIndex: call.BundleIndex, SlotKey: call.SlotKey, CompiledRequestHash: call.CompiledRequestHash, Content: raw, Status: state.Status, Revision: state.Revision, StateContent: stateRaw, StateHash: state.ContentHash}
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&records).Error
}

func (repo *referenceExecutionRepository) FindReferenceProviderJob(ctx context.Context, workspace, project, executionID string) (domain.ReferenceProviderJob, []domain.ReferenceProviderCall, error) {
	var record model.GenerationReferenceProviderJob
	if err := repo.database.WithContext(ctx).Where("execution_id = ? AND workspace_id = ? AND project_id = ?", executionID, workspace, project).First(&record).Error; err != nil {
		return domain.ReferenceProviderJob{}, nil, err
	}
	job, err := domain.DecodeReferenceProviderJob(json.RawMessage(record.Content))
	if err != nil {
		return domain.ReferenceProviderJob{}, nil, err
	}
	if job.ExecutionRef.ID != record.ExecutionID.String() || job.ExecutionRef.ContentHash != record.ExecutionHash || job.CallSetRoot != record.CallSetRoot || job.ContentHash != record.ContentHash {
		return domain.ReferenceProviderJob{}, nil, errors.New("persisted Reference Provider job identity has drifted")
	}
	var records []model.GenerationReferenceProviderCall
	// Read the full membership by execution, including incorrectly scoped rows,
	// then validate scope. Never filter a corrupt member out of the expected set.
	limits := domain.DefaultReferenceGenerationLimits()
	if err = repo.database.WithContext(ctx).Where("execution_id = ?", executionID).Order("bundle_index, slot_key").Limit(limits.MaxBundles*limits.MaxSlots + 1).Find(&records).Error; err != nil {
		return domain.ReferenceProviderJob{}, nil, err
	}
	calls := make([]domain.ReferenceProviderCall, len(records))
	for i, item := range records {
		call, err := domain.DecodeReferenceProviderCall(json.RawMessage(item.Content))
		if err != nil {
			return domain.ReferenceProviderJob{}, nil, err
		}
		if item.WorkspaceID != record.WorkspaceID || item.ProjectID != record.ProjectID || call.ExecutionRef != job.ExecutionRef || call.CallKey != item.CallKey || call.BundleIndex != item.BundleIndex || call.SlotKey != item.SlotKey || call.CompiledRequestHash != item.CompiledRequestHash {
			return domain.ReferenceProviderJob{}, nil, errors.New("persisted Reference Provider call identity has drifted")
		}
		if _, err := referenceCallStateFromRecord(item); err != nil {
			return domain.ReferenceProviderJob{}, nil, err
		}
		calls[i] = call
	}
	// Canonical identity uses Go string ordering, not the database's locale.
	sort.Slice(calls, func(i, j int) bool {
		if calls[i].BundleIndex != calls[j].BundleIndex {
			return calls[i].BundleIndex < calls[j].BundleIndex
		}
		return calls[i].SlotKey < calls[j].SlotKey
	})
	if err = validateReferenceProviderCallSet(job, calls); err != nil {
		return domain.ReferenceProviderJob{}, nil, err
	}
	return job, calls, nil
}

func validateReferenceProviderCallSet(job domain.ReferenceProviderJob, calls []domain.ReferenceProviderCall) error {
	inputs := make([]domain.ReferenceProviderCallInput, len(calls))
	for i, call := range calls {
		inputs[i] = call.ReferenceProviderCallInput
	}
	expected, expectedCalls, err := domain.BuildReferenceProviderJob(job.ExecutionRef, inputs)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(job, expected) || !reflect.DeepEqual(calls, expectedCalls) {
		return errors.New("Reference Provider job has an incomplete or inconsistent call set")
	}
	return nil
}
