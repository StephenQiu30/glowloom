package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	costgorm "github.com/StephenQiu30/lanverse/backend/internal/cost/adapter/gormdb"
	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	quotagorm "github.com/StephenQiu30/lanverse/backend/internal/quota/adapter/gormdb"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
)

type ProviderStore struct {
	database    *gorm.DB
	costConfig  costapp.Config
	quotaConfig quotaapp.Config
}

type providerRepository struct{ *preparationRepository }

func NewProviderStore(database *gorm.DB, costConfig costapp.Config, quotaConfig quotaapp.Config) *ProviderStore {
	return &ProviderStore{database: database, costConfig: costConfig, quotaConfig: quotaConfig}
}

func (store *ProviderStore) WithinProviderTransaction(
	ctx context.Context,
	operation func(application.ProviderRepository, application.CostProviderOwner, application.QuotaProviderOwner) error,
) error {
	if store == nil || store.database == nil || store.database.Config == nil || operation == nil {
		return errors.New("Generation Provider transaction is not configured")
	}
	if store.database.Config.DisableNestedTransaction {
		return errors.New("Generation Provider transaction requires GORM nested transaction savepoints")
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		repo := &providerRepository{preparationRepository: &preparationRepository{database: transaction}}
		costs := costapp.NewService(costgorm.New(transaction), store.costConfig)
		quotas := quotaapp.NewService(quotagorm.New(transaction), store.quotaConfig)
		return operation(repo, costs, quotas)
	})
}

func (repo *providerRepository) LockProviderWorkspace(ctx context.Context, workspaceID string) error {
	return (&providerConfigurationRepository{database: repo.database}).LockProviderWorkspace(ctx, workspaceID)
}

func (repo *providerRepository) LatestProviderConnectionForUpdate(
	ctx context.Context,
	workspaceID, connectionKey string,
) (domain.ProviderConnectionVersion, error) {
	return (&providerConfigurationRepository{database: repo.database}).LatestProviderConnectionForUpdate(
		ctx,
		workspaceID,
		connectionKey,
	)
}

func (repo *providerRepository) LatestProviderModelProfileForUpdate(
	ctx context.Context,
	workspaceID, profileKey string,
) (domain.ProviderModelProfileVersion, error) {
	return (&providerConfigurationRepository{database: repo.database}).LatestProviderModelProfileForUpdate(
		ctx,
		workspaceID,
		profileKey,
	)
}

func (repo *providerRepository) FindProjectProviderBinding(
	ctx context.Context,
	bindingID string,
) (domain.ProjectProviderBindingVersion, error) {
	id, err := uuid.Parse(bindingID)
	if err != nil {
		return domain.ProjectProviderBindingVersion{}, application.ErrProjectProviderBindingNotFound
	}
	var record model.ProjectProviderBindingVersion
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return domain.ProjectProviderBindingVersion{}, normalizeProviderNotFound(
			err,
			application.ErrProjectProviderBindingNotFound,
		)
	}
	return projectProviderBindingDomain(record), nil
}

func (repo *providerRepository) FindRequestByIntent(
	ctx context.Context,
	intentID string,
) (domain.GenerationRequest, error) {
	id, err := uuid.Parse(intentID)
	if err != nil {
		return domain.GenerationRequest{}, application.ErrGenerationRequestNotFound
	}
	var record model.GenerationRequest
	if err = repo.database.WithContext(ctx).First(&record, "intent_id = ?", id).Error; err != nil {
		return domain.GenerationRequest{}, normalizeProviderNotFound(err, application.ErrGenerationRequestNotFound)
	}
	return generationRequestDomain(record), nil
}

func (repo *providerRepository) FindGenerationRequest(
	ctx context.Context,
	requestID string,
) (domain.GenerationRequest, error) {
	id, err := uuid.Parse(requestID)
	if err != nil {
		return domain.GenerationRequest{}, application.ErrGenerationRequestNotFound
	}
	var record model.GenerationRequest
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return domain.GenerationRequest{}, normalizeProviderNotFound(err, application.ErrGenerationRequestNotFound)
	}
	return generationRequestDomain(record), nil
}

func (repo *providerRepository) EnsureRequestJobAndCalls(
	ctx context.Context,
	request domain.GenerationRequest,
	job domain.ProviderJob,
	calls []domain.ProviderCall,
) (domain.GenerationRequest, domain.ProviderJob, []domain.ProviderCall, error) {
	requestRecord, err := generationRequestRecord(request)
	if err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, err
	}
	jobRecord, err := providerJobRecord(job)
	if err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, err
	}
	callRecords := make([]model.GenerationProviderCall, 0, len(calls))
	for _, call := range calls {
		record, recordErr := providerCallRecord(call)
		if recordErr != nil {
			return domain.GenerationRequest{}, domain.ProviderJob{}, nil, recordErr
		}
		callRecords = append(callRecords, record)
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&requestRecord).Error; err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, fmt.Errorf("create Generation request: %w", err)
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&jobRecord).Error; err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, fmt.Errorf("create Generation Provider job: %w", err)
	}
	if len(callRecords) == 0 {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, errors.New("Generation Provider job requires Calls")
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&callRecords).Error; err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, nil, fmt.Errorf("create Generation Provider Calls: %w", err)
	}
	persistedCalls := make([]domain.ProviderCall, len(callRecords))
	for index := range callRecords {
		persistedCalls[index] = providerCallDomain(callRecords[index])
	}
	return generationRequestDomain(requestRecord), providerJobDomain(jobRecord), persistedCalls, nil
}

func (repo *providerRepository) FindProviderJobByIntent(
	ctx context.Context,
	intentID string,
) (domain.ProviderJob, error) {
	id, err := uuid.Parse(intentID)
	if err != nil {
		return domain.ProviderJob{}, application.ErrProviderJobNotFound
	}
	var record model.GenerationProviderJob
	if err = repo.database.WithContext(ctx).First(&record, "intent_id = ?", id).Error; err != nil {
		return domain.ProviderJob{}, normalizeProviderNotFound(err, application.ErrProviderJobNotFound)
	}
	return providerJobDomain(record), nil
}

func (repo *providerRepository) GetIntentForProviderJobUpdate(
	ctx context.Context,
	jobID string,
) (domain.Intent, error) {
	id, err := uuid.Parse(jobID)
	if err != nil {
		return domain.Intent{}, application.ErrIntentNotFound
	}
	var record model.GenerationIntent
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&record, "provider_job_id = ?", id).Error; err != nil {
		return domain.Intent{}, normalizePreparationIntentNotFound(err)
	}
	return intentDomain(record), nil
}

func (repo *providerRepository) GetProviderJobForUpdate(
	ctx context.Context,
	jobID string,
) (domain.ProviderJob, error) {
	id, err := uuid.Parse(jobID)
	if err != nil {
		return domain.ProviderJob{}, application.ErrProviderJobNotFound
	}
	var record model.GenerationProviderJob
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&record, "id = ?", id).Error; err != nil {
		return domain.ProviderJob{}, normalizeProviderNotFound(err, application.ErrProviderJobNotFound)
	}
	return providerJobDomain(record), nil
}

func (repo *providerRepository) UpdateProviderJob(
	ctx context.Context,
	value domain.ProviderJob,
	expectedRevision int64,
) (domain.ProviderJob, error) {
	record, err := providerJobRecord(value)
	if err != nil {
		return domain.ProviderJob{}, err
	}
	updated := repo.database.WithContext(ctx).Model(&model.GenerationProviderJob{}).
		Where("id = ? AND revision = ?", record.ID, expectedRevision).
		Updates(map[string]any{
			"status": record.Status, "call_set_hash": record.CallSetHash,
			"dispatched_call_count": record.DispatchedCallCount,
			"succeeded_call_count":  record.SucceededCallCount, "failed_call_count": record.FailedCallCount,
			"revision": record.Revision, "content_hash": record.ContentHash, "updated_at": record.UpdatedAt,
		})
	if updated.Error != nil {
		return domain.ProviderJob{}, updated.Error
	}
	if updated.RowsAffected != 1 {
		return domain.ProviderJob{}, providerConflict("Generation Provider job revision has changed")
	}
	return value, nil
}

func (repo *providerRepository) ListProviderCalls(
	ctx context.Context,
	jobID string,
) ([]domain.ProviderCall, error) {
	id, err := uuid.Parse(jobID)
	if err != nil {
		return nil, application.ErrProviderJobNotFound
	}
	var records []model.GenerationProviderCall
	if err = repo.database.WithContext(ctx).Where("job_id = ?", id).Order("candidate_index ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]domain.ProviderCall, len(records))
	for index := range records {
		result[index] = providerCallDomain(records[index])
	}
	return result, nil
}

func (repo *providerRepository) GetProviderCallForUpdate(
	ctx context.Context,
	callID string,
) (domain.ProviderCall, error) {
	id, err := uuid.Parse(callID)
	if err != nil {
		return domain.ProviderCall{}, application.ErrProviderCallNotFound
	}
	var record model.GenerationProviderCall
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&record, "id = ?", id).Error; err != nil {
		return domain.ProviderCall{}, normalizeProviderNotFound(err, application.ErrProviderCallNotFound)
	}
	return providerCallDomain(record), nil
}

func (repo *providerRepository) UpdateProviderCall(
	ctx context.Context,
	value domain.ProviderCall,
	expectedRevision int64,
) (domain.ProviderCall, error) {
	record, err := providerCallRecord(value)
	if err != nil {
		return domain.ProviderCall{}, err
	}
	updated := repo.database.WithContext(ctx).Model(&model.GenerationProviderCall{}).
		Where("id = ? AND revision = ?", record.ID, expectedRevision).
		Updates(map[string]any{
			"status": record.Status, "local_failure_code": record.LocalFailureCode,
			"remote_request_id": record.RemoteRequestID, "remote_job_id": record.RemoteJobID,
			"dispatch_boundary_entered_at": record.DispatchBoundaryEnteredAt,
			"query_deadline_at":            record.QueryDeadlineAt,
			"remote_expires_at":            record.RemoteExpiresAt,
			"revision":                     record.Revision, "content_hash": record.ContentHash, "updated_at": record.UpdatedAt,
		})
	if updated.Error != nil {
		return domain.ProviderCall{}, updated.Error
	}
	if updated.RowsAffected != 1 {
		return domain.ProviderCall{}, providerConflict("Generation Provider Call revision has changed")
	}
	return value, nil
}

func (repo *providerRepository) ListProviderResultReceipts(
	ctx context.Context,
	jobID string,
) ([]domain.ProviderResultReceipt, error) {
	job, err := uuid.Parse(jobID)
	if err != nil {
		return nil, application.ErrProviderJobNotFound
	}
	var calls []model.GenerationProviderCall
	if err = repo.database.WithContext(ctx).Select("id", "candidate_index").
		Where("job_id = ?", job).Order("candidate_index ASC").Find(&calls).Error; err != nil {
		return nil, err
	}
	if len(calls) == 0 {
		return []domain.ProviderResultReceipt{}, nil
	}
	callIDs := make([]uuid.UUID, len(calls))
	for index := range calls {
		callIDs[index] = calls[index].ID
	}
	var records []model.GenerationProviderResultReceipt
	if err = repo.database.WithContext(ctx).Where("call_id IN ?", callIDs).Find(&records).Error; err != nil {
		return nil, err
	}
	byCall := make(map[uuid.UUID]model.GenerationProviderResultReceipt, len(records))
	for _, record := range records {
		byCall[record.CallID] = record
	}
	result := make([]domain.ProviderResultReceipt, 0, len(records))
	for _, call := range calls {
		record, exists := byCall[call.ID]
		if !exists {
			continue
		}
		receipt, decodeErr := providerReceiptDomain(record)
		if decodeErr != nil {
			return nil, decodeErr
		}
		result = append(result, receipt)
	}
	return result, nil
}

func (repo *providerRepository) FindProviderResultReceiptByCall(
	ctx context.Context,
	callID string,
) (domain.ProviderResultReceipt, error) {
	id, err := uuid.Parse(callID)
	if err != nil {
		return domain.ProviderResultReceipt{}, application.ErrProviderResultReceiptNotFound
	}
	var record model.GenerationProviderResultReceipt
	if err = repo.database.WithContext(ctx).First(&record, "call_id = ?", id).Error; err != nil {
		return domain.ProviderResultReceipt{}, normalizeProviderNotFound(err, application.ErrProviderResultReceiptNotFound)
	}
	return providerReceiptDomain(record)
}

func (repo *providerRepository) EnsureProviderResultReceipt(
	ctx context.Context,
	value domain.ProviderResultReceipt,
) (domain.ProviderResultReceipt, error) {
	record, err := providerReceiptRecord(value)
	if err != nil {
		return domain.ProviderResultReceipt{}, err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).
		Create(&record).Error; err != nil {
		return domain.ProviderResultReceipt{}, err
	}
	var persisted model.GenerationProviderResultReceipt
	if err = repo.database.WithContext(ctx).Where("call_id = ?", record.CallID).First(&persisted).Error; err != nil {
		return domain.ProviderResultReceipt{}, err
	}
	return providerReceiptDomain(persisted)
}

func generationRequestRecord(value domain.GenerationRequest) (model.GenerationRequest, error) {
	ids, err := parseProviderUUIDs(
		value.ID, value.WorkspaceID, value.ProjectID, value.IntentID, value.TargetID, value.BindingID,
		value.ConnectionVersionID, value.CredentialVersionID, value.ModelProfileVersionID, value.PriceQuoteID,
		value.CreatedBy,
	)
	if err != nil {
		return model.GenerationRequest{}, err
	}
	return model.GenerationRequest{
		ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], IntentID: ids[3], TargetID: ids[4], BindingID: ids[5],
		BindingRevision: value.BindingRevision, BindingContentHash: value.BindingContentHash,
		Purpose: value.Purpose, ProviderKey: value.ProviderKey, ExternalModelID: value.ExternalModelID,
		ConnectionVersionID: ids[6], CredentialVersionID: ids[7], ModelProfileVersionID: ids[8],
		ModelProfileRevision: value.ModelProfileRevision, ModelProfileContentHash: value.ModelProfileContentHash,
		PriceQuoteID: ids[9], PriceQuoteRevision: value.PriceQuoteRevision,
		PriceQuoteContentHash: value.PriceQuoteContentHash, BillingMetric: value.BillingMetric,
		RequestKey: value.RequestKey, TargetHash: value.TargetHash, EstimatedUnits: value.EstimatedUnits,
		ContentHash: value.ContentHash, CreatedBy: ids[10], CreatedAt: value.CreatedAt.UTC(),
	}, nil
}

func generationRequestDomain(value model.GenerationRequest) domain.GenerationRequest {
	return domain.GenerationRequest{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		IntentID: value.IntentID.String(), TargetID: value.TargetID.String(), BindingID: value.BindingID.String(),
		BindingRevision: value.BindingRevision, BindingContentHash: value.BindingContentHash,
		Purpose: value.Purpose, ProviderKey: value.ProviderKey, ExternalModelID: value.ExternalModelID,
		ConnectionVersionID: value.ConnectionVersionID.String(), CredentialVersionID: value.CredentialVersionID.String(),
		ModelProfileVersionID: value.ModelProfileVersionID.String(), ModelProfileRevision: value.ModelProfileRevision,
		ModelProfileContentHash: value.ModelProfileContentHash, PriceQuoteID: value.PriceQuoteID.String(),
		PriceQuoteRevision: value.PriceQuoteRevision, PriceQuoteContentHash: value.PriceQuoteContentHash,
		BillingMetric: value.BillingMetric, RequestKey: value.RequestKey, TargetHash: value.TargetHash,
		EstimatedUnits: value.EstimatedUnits, ContentHash: value.ContentHash, CreatedBy: value.CreatedBy.String(),
		CreatedAt: value.CreatedAt.UTC(),
	}
}

func providerJobRecord(value domain.ProviderJob) (model.GenerationProviderJob, error) {
	ids, err := parseProviderUUIDs(value.ID, value.WorkspaceID, value.ProjectID, value.IntentID, value.RequestID)
	if err != nil {
		return model.GenerationProviderJob{}, err
	}
	return model.GenerationProviderJob{
		ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], IntentID: ids[3], RequestID: ids[4],
		ProviderKey: value.ProviderKey, RequestKey: value.RequestKey, Status: value.Status,
		CallCount: value.CallCount, DispatchedCallCount: value.DispatchedCallCount,
		SucceededCallCount: value.SucceededCallCount, FailedCallCount: value.FailedCallCount,
		CallSetHash: value.CallSetHash, Revision: value.Revision, ContentHash: value.ContentHash,
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}, nil
}

func providerJobDomain(value model.GenerationProviderJob) domain.ProviderJob {
	return domain.ProviderJob{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		IntentID: value.IntentID.String(), RequestID: value.RequestID.String(), ProviderKey: value.ProviderKey,
		RequestKey: value.RequestKey, Status: value.Status, CallCount: value.CallCount,
		DispatchedCallCount: value.DispatchedCallCount, SucceededCallCount: value.SucceededCallCount,
		FailedCallCount: value.FailedCallCount, CallSetHash: value.CallSetHash,
		Revision: value.Revision, ContentHash: value.ContentHash,
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}
}

func providerCallRecord(value domain.ProviderCall) (model.GenerationProviderCall, error) {
	ids, err := parseProviderUUIDs(value.ID, value.WorkspaceID, value.ProjectID, value.JobID)
	if err != nil {
		return model.GenerationProviderCall{}, err
	}
	return model.GenerationProviderCall{
		ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], JobID: ids[3],
		CandidateIndex: value.CandidateIndex, CallKey: value.CallKey, RequestHash: value.RequestHash,
		RequestedOutputCount: value.RequestedOutputCount, Status: value.Status,
		LocalFailureCode:          optionalProviderStringPointer(value.LocalFailureCode),
		RemoteRequestID:           optionalProviderStringPointer(value.RemoteRequestID),
		RemoteJobID:               optionalProviderStringPointer(value.RemoteJobID),
		DispatchBoundaryEnteredAt: clonePreparationTime(value.DispatchBoundaryEnteredAt),
		QueryDeadlineAt:           clonePreparationTime(value.QueryDeadlineAt),
		RemoteExpiresAt:           clonePreparationTime(value.RemoteExpiresAt),
		Revision:                  value.Revision, ContentHash: value.ContentHash,
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}, nil
}

func providerCallDomain(value model.GenerationProviderCall) domain.ProviderCall {
	return domain.ProviderCall{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		JobID: value.JobID.String(), CandidateIndex: value.CandidateIndex, CallKey: value.CallKey,
		RequestHash: value.RequestHash, RequestedOutputCount: value.RequestedOutputCount, Status: value.Status,
		LocalFailureCode: providerOptionalString(value.LocalFailureCode),
		RemoteRequestID:  providerOptionalString(value.RemoteRequestID), RemoteJobID: providerOptionalString(value.RemoteJobID),
		DispatchBoundaryEnteredAt: clonePreparationTime(value.DispatchBoundaryEnteredAt),
		QueryDeadlineAt:           clonePreparationTime(value.QueryDeadlineAt),
		RemoteExpiresAt:           clonePreparationTime(value.RemoteExpiresAt),
		Revision:                  value.Revision, ContentHash: value.ContentHash,
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}
}

func providerReceiptRecord(value domain.ProviderResultReceipt) (model.GenerationProviderResultReceipt, error) {
	ids, err := parseProviderUUIDs(value.ID, value.WorkspaceID, value.ProjectID, value.CallID)
	if err != nil {
		return model.GenerationProviderResultReceipt{}, err
	}
	output, err := json.Marshal(value.Output)
	if err != nil {
		return model.GenerationProviderResultReceipt{}, err
	}
	usage, err := json.Marshal(value.ProviderUsageObservation)
	if err != nil {
		return model.GenerationProviderResultReceipt{}, err
	}
	return model.GenerationProviderResultReceipt{
		ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], CallID: ids[3],
		ProviderEventID: optionalProviderStringPointer(value.ProviderEventID), Status: value.Status,
		OutputCount: value.OutputCount, Output: output, FailureCode: optionalProviderStringPointer(value.FailureCode),
		ProviderUsageObservation: usage, ProviderUsageHash: value.ProviderUsageHash,
		ContentHash: value.ContentHash, OccurredAt: value.OccurredAt.UTC(), ReceivedAt: value.ReceivedAt.UTC(),
	}, nil
}

func providerReceiptDomain(value model.GenerationProviderResultReceipt) (domain.ProviderResultReceipt, error) {
	var output *domain.ProviderOutput
	if err := json.Unmarshal(value.Output, &output); err != nil {
		return domain.ProviderResultReceipt{}, err
	}
	var usage domain.ProviderUsageObservation
	if err := json.Unmarshal(value.ProviderUsageObservation, &usage); err != nil {
		return domain.ProviderResultReceipt{}, err
	}
	return domain.ProviderResultReceipt{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		CallID: value.CallID.String(), ProviderEventID: providerOptionalString(value.ProviderEventID),
		Status: value.Status, OutputCount: value.OutputCount, Output: output,
		FailureCode: providerOptionalString(value.FailureCode), ProviderUsageObservation: usage,
		ProviderUsageHash: value.ProviderUsageHash, ContentHash: value.ContentHash,
		OccurredAt: value.OccurredAt.UTC(), ReceivedAt: value.ReceivedAt.UTC(),
	}, nil
}

func parseProviderUUIDs(values ...string) ([]uuid.UUID, error) {
	parsed := make([]uuid.UUID, len(values))
	for index, value := range values {
		id, err := uuid.Parse(value)
		if err != nil {
			return nil, err
		}
		parsed[index] = id
	}
	return parsed, nil
}

func optionalProviderStringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func providerOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func normalizeProviderNotFound(err, notFound error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound
	}
	return err
}

func providerConflict(message string) error {
	return &application.Error{Code: "state_conflict", Message: message, Status: 409}
}

var _ application.ProviderTransactionManager = (*ProviderStore)(nil)
var _ application.ProviderRepository = (*providerRepository)(nil)
