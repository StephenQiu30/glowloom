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

func (repo *providerRepository) LatestProviderBindingForUpdate(
	ctx context.Context,
	workspaceID, projectID string,
) (domain.ProviderBinding, error) {
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return domain.ProviderBinding{}, application.ErrProviderBindingNotFound
	}
	project, err := uuid.Parse(projectID)
	if err != nil {
		return domain.ProviderBinding{}, application.ErrProviderBindingNotFound
	}
	var projectRecord model.Project
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND workspace_id = ?", project, workspace).First(&projectRecord).Error; err != nil {
		return domain.ProviderBinding{}, normalizeProviderNotFound(err, application.ErrProviderBindingNotFound)
	}
	var record model.GenerationProviderBindingVersion
	if err = repo.database.WithContext(ctx).
		Where("workspace_id = ? AND project_id = ? AND capability = ?", workspace, project, "generation.image").
		Order("revision DESC").First(&record).Error; err != nil {
		return domain.ProviderBinding{}, normalizeProviderNotFound(err, application.ErrProviderBindingNotFound)
	}
	return providerBindingDomain(record), nil
}

func (repo *providerRepository) FindProviderBinding(
	ctx context.Context,
	bindingID string,
) (domain.ProviderBinding, error) {
	id, err := uuid.Parse(bindingID)
	if err != nil {
		return domain.ProviderBinding{}, application.ErrProviderBindingNotFound
	}
	var record model.GenerationProviderBindingVersion
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return domain.ProviderBinding{}, normalizeProviderNotFound(err, application.ErrProviderBindingNotFound)
	}
	return providerBindingDomain(record), nil
}

func (repo *providerRepository) CreateProviderBinding(
	ctx context.Context,
	value domain.ProviderBinding,
) (domain.ProviderBinding, error) {
	record, err := providerBindingRecord(value)
	if err != nil {
		return domain.ProviderBinding{}, err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		return domain.ProviderBinding{}, fmt.Errorf("create Generation Provider binding: %w", err)
	}
	return providerBindingDomain(record), nil
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

func (repo *providerRepository) EnsureRequestAndJob(
	ctx context.Context,
	request domain.GenerationRequest,
	job domain.ProviderJob,
) (domain.GenerationRequest, domain.ProviderJob, error) {
	requestRecord, err := generationRequestRecord(request)
	if err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, err
	}
	jobRecord, err := providerJobRecord(job)
	if err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&requestRecord).Error; err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, fmt.Errorf("create Generation request: %w", err)
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&jobRecord).Error; err != nil {
		return domain.GenerationRequest{}, domain.ProviderJob{}, fmt.Errorf("create Generation Provider job: %w", err)
	}
	return generationRequestDomain(requestRecord), providerJobDomain(jobRecord), nil
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
			"provider_job_key": record.ProviderJobKey, "status": record.Status,
			"provider_receipt_id": record.ProviderReceiptID, "revision": record.Revision,
			"content_hash": record.ContentHash, "updated_at": record.UpdatedAt,
		})
	if updated.Error != nil {
		return domain.ProviderJob{}, updated.Error
	}
	if updated.RowsAffected != 1 {
		return domain.ProviderJob{}, providerConflict("Generation Provider job revision has changed")
	}
	return value, nil
}

func (repo *providerRepository) FindProviderResultReceiptByJob(
	ctx context.Context,
	jobID string,
) (domain.ProviderResultReceipt, error) {
	id, err := uuid.Parse(jobID)
	if err != nil {
		return domain.ProviderResultReceipt{}, application.ErrProviderResultReceiptNotFound
	}
	var record model.GenerationProviderResultReceipt
	if err = repo.database.WithContext(ctx).First(&record, "job_id = ?", id).Error; err != nil {
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
	if err = repo.database.WithContext(ctx).Where("job_id = ?", record.JobID).First(&persisted).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ProviderResultReceipt{}, providerConflict("Provider event is already bound to another job")
		}
		return domain.ProviderResultReceipt{}, err
	}
	return providerReceiptDomain(persisted)
}

func providerBindingRecord(value domain.ProviderBinding) (model.GenerationProviderBindingVersion, error) {
	ids, err := parseProviderUUIDs(value.ID, value.WorkspaceID, value.ProjectID, value.CreatedBy)
	if err != nil {
		return model.GenerationProviderBindingVersion{}, err
	}
	return model.GenerationProviderBindingVersion{
		ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], Capability: value.Capability,
		Revision: value.Revision, ProviderKey: value.ProviderKey, ModelKey: value.ModelKey,
		CredentialRef: value.CredentialRef, ContentHash: value.ContentHash,
		CreatedBy: ids[3], CreatedAt: value.CreatedAt.UTC(),
	}, nil
}

func providerBindingDomain(value model.GenerationProviderBindingVersion) domain.ProviderBinding {
	return domain.ProviderBinding{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		Capability: value.Capability, ProviderKey: value.ProviderKey, ModelKey: value.ModelKey,
		CredentialRef: value.CredentialRef, Revision: value.Revision, ContentHash: value.ContentHash,
		CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt.UTC(),
	}
}

func generationRequestRecord(value domain.GenerationRequest) (model.GenerationRequest, error) {
	ids, err := parseProviderUUIDs(
		value.ID, value.WorkspaceID, value.ProjectID, value.IntentID, value.BindingID, value.CreatedBy,
	)
	if err != nil {
		return model.GenerationRequest{}, err
	}
	return model.GenerationRequest{
		ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], IntentID: ids[3], BindingID: ids[4],
		BindingRevision: value.BindingRevision, Capability: value.Capability, ProviderKey: value.ProviderKey,
		ModelKey: value.ModelKey, CredentialRef: value.CredentialRef, RequestKey: value.RequestKey,
		InputHash: value.InputHash, Units: value.Units, ContentHash: value.ContentHash,
		CreatedBy: ids[5], CreatedAt: value.CreatedAt.UTC(),
	}, nil
}

func generationRequestDomain(value model.GenerationRequest) domain.GenerationRequest {
	return domain.GenerationRequest{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		IntentID: value.IntentID.String(), BindingID: value.BindingID.String(), BindingRevision: value.BindingRevision,
		Capability: value.Capability, ProviderKey: value.ProviderKey, ModelKey: value.ModelKey,
		CredentialRef: value.CredentialRef, RequestKey: value.RequestKey, InputHash: value.InputHash,
		Units: value.Units, ContentHash: value.ContentHash, CreatedBy: value.CreatedBy.String(),
		CreatedAt: value.CreatedAt.UTC(),
	}
}

func providerJobRecord(value domain.ProviderJob) (model.GenerationProviderJob, error) {
	ids, err := parseProviderUUIDs(value.ID, value.WorkspaceID, value.ProjectID, value.IntentID, value.RequestID)
	if err != nil {
		return model.GenerationProviderJob{}, err
	}
	providerReceiptID, err := optionalPreparationUUID(value.ProviderReceiptID)
	if err != nil {
		return model.GenerationProviderJob{}, err
	}
	return model.GenerationProviderJob{
		ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], IntentID: ids[3], RequestID: ids[4],
		ProviderKey: value.ProviderKey, RequestKey: value.RequestKey,
		ProviderJobKey: optionalProviderStringPointer(value.ProviderJobKey), ProviderReceiptID: providerReceiptID,
		Status: value.Status, Revision: value.Revision, ContentHash: value.ContentHash,
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}, nil
}

func providerJobDomain(value model.GenerationProviderJob) domain.ProviderJob {
	return domain.ProviderJob{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		IntentID: value.IntentID.String(), RequestID: value.RequestID.String(), ProviderKey: value.ProviderKey,
		RequestKey: value.RequestKey, ProviderJobKey: providerOptionalString(value.ProviderJobKey),
		Status: value.Status, ProviderReceiptID: optionalPreparationUUIDString(value.ProviderReceiptID),
		Revision: value.Revision, ContentHash: value.ContentHash,
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}
}

func providerReceiptRecord(value domain.ProviderResultReceipt) (model.GenerationProviderResultReceipt, error) {
	ids, err := parseProviderUUIDs(value.ID, value.WorkspaceID, value.ProjectID, value.JobID, value.RequestID)
	if err != nil {
		return model.GenerationProviderResultReceipt{}, err
	}
	outputs, err := json.Marshal(value.Outputs)
	if err != nil {
		return model.GenerationProviderResultReceipt{}, err
	}
	return model.GenerationProviderResultReceipt{
		ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], JobID: ids[3], RequestID: ids[4],
		ProviderKey: value.ProviderKey, ProviderJobKey: optionalProviderStringPointer(value.ProviderJobKey),
		ProviderEventID: value.ProviderEventID, Status: value.Status, ActualUnits: value.ActualUnits,
		Outputs: outputs, FailureCode: optionalProviderStringPointer(value.FailureCode), ContentHash: value.ContentHash,
		OccurredAt: value.OccurredAt.UTC(), ReceivedAt: value.ReceivedAt.UTC(),
	}, nil
}

func providerReceiptDomain(value model.GenerationProviderResultReceipt) (domain.ProviderResultReceipt, error) {
	var outputs []domain.ProviderOutput
	if err := json.Unmarshal(value.Outputs, &outputs); err != nil {
		return domain.ProviderResultReceipt{}, err
	}
	if outputs == nil {
		outputs = []domain.ProviderOutput{}
	}
	return domain.ProviderResultReceipt{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		JobID: value.JobID.String(), RequestID: value.RequestID.String(), ProviderKey: value.ProviderKey,
		ProviderJobKey: providerOptionalString(value.ProviderJobKey), ProviderEventID: value.ProviderEventID,
		Status: value.Status, ActualUnits: value.ActualUnits, Outputs: outputs,
		FailureCode: providerOptionalString(value.FailureCode), ContentHash: value.ContentHash,
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
