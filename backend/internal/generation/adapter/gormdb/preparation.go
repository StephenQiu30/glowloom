package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	costgorm "github.com/StephenQiu30/lanverse/backend/internal/cost/adapter/gormdb"
	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	commandgorm "github.com/StephenQiu30/lanverse/backend/internal/platform/command/adapter/gormdb"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	quotagorm "github.com/StephenQiu30/lanverse/backend/internal/quota/adapter/gormdb"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type PreparationStore struct {
	database    *gorm.DB
	costConfig  costapp.Config
	quotaConfig quotaapp.Config
}

type preparationRepository struct{ database *gorm.DB }

func NewPreparationStore(database *gorm.DB, costConfig costapp.Config, quotaConfig quotaapp.Config) *PreparationStore {
	return &PreparationStore{database: database, costConfig: costConfig, quotaConfig: quotaConfig}
}

func (store *PreparationStore) WithinPreparationTransaction(
	ctx context.Context,
	operation func(application.PreparationRepository, application.CostPreparationOwner, application.QuotaPreparationOwner) error,
) error {
	if store == nil || store.database == nil || store.database.Config == nil || operation == nil {
		return errors.New("Generation preparation transaction is not configured")
	}
	if store.database.Config.DisableNestedTransaction {
		return errors.New("Generation preparation requires GORM nested transaction savepoints")
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		repo := &preparationRepository{database: transaction}
		costs := costapp.NewService(costgorm.New(transaction), store.costConfig)
		quotas := quotaapp.NewService(quotagorm.New(transaction), store.quotaConfig)
		return operation(repo, costs, quotas)
	})
}

func (repo *preparationRepository) AuthorizeProject(
	ctx context.Context,
	actor application.Actor,
	workspaceID, projectID string,
	write bool,
) error {
	return (&repository{database: repo.database}).AuthorizeProject(ctx, actor, workspaceID, projectID, write)
}

func (repo *preparationRepository) AuthorizeProviderProject(
	ctx context.Context,
	actor application.Actor,
	projectID string,
) (application.ProviderProjectScope, error) {
	return (&repository{database: repo.database}).AuthorizeProviderProject(ctx, actor, projectID)
}

func (repo *preparationRepository) FindGenerationTarget(
	ctx context.Context,
	targetID string,
) (domain.GenerationTarget, error) {
	return findGenerationTarget(ctx, repo.database, targetID)
}

func (repo *preparationRepository) LatestProjectProviderBindingForUpdate(
	ctx context.Context,
	workspaceID, projectID, purpose string,
) (domain.ProjectProviderBindingVersion, error) {
	return (&providerConfigurationRepository{database: repo.database}).LatestProjectProviderBindingForUpdate(
		ctx,
		workspaceID,
		projectID,
		purpose,
	)
}

func (repo *preparationRepository) FindProviderConnection(
	ctx context.Context,
	connectionID string,
) (domain.ProviderConnectionVersion, error) {
	return (&providerConfigurationRepository{database: repo.database}).FindProviderConnection(ctx, connectionID)
}

func (repo *preparationRepository) FindProviderCredential(
	ctx context.Context,
	credentialID string,
) (domain.ProviderCredentialVersion, error) {
	return (&providerConfigurationRepository{database: repo.database}).FindProviderCredential(ctx, credentialID)
}

func (repo *preparationRepository) FindProviderModelProfile(
	ctx context.Context,
	profileID string,
) (domain.ProviderModelProfileVersion, error) {
	return (&providerConfigurationRepository{database: repo.database}).FindProviderModelProfile(ctx, profileID)
}

func (repo *preparationRepository) ValidateWorkflowSource(
	ctx context.Context,
	actor application.Actor,
	workspaceID, projectID, workflowRunID, nodeRunID, inputHash string,
) error {
	workspace, project, runID, nodeID, err := parsePreparationIDs(workspaceID, projectID, workflowRunID, nodeRunID)
	if err != nil {
		return preparationConflict("Generation Workflow source is invalid")
	}
	var run model.WorkflowRun
	if err = repo.database.WithContext(ctx).Where(
		"id = ? AND workspace_id = ? AND project_id = ?", runID, workspace, project,
	).First(&run).Error; err != nil {
		return normalizePreparationSourceNotFound(err)
	}
	if run.CreatedBy.String() != actor.UserID || run.InitiatorTokenVersion != actor.TokenVersion {
		return preparationConflict("Generation Workflow initiator has drifted")
	}
	if run.Status != "RUNNING" && run.Status != "RETRYING" {
		return preparationConflict("Generation Workflow run is not executable")
	}
	var node model.NodeRunProjection
	if err = repo.database.WithContext(ctx).Where(
		"id = ? AND workspace_id = ? AND workflow_run_id = ?", nodeID, workspace, runID,
	).First(&node).Error; err != nil {
		return normalizePreparationSourceNotFound(err)
	}
	if node.RiskLevel != "external_ai" || (node.Status != "RUNNING" && node.Status != "RETRYING") ||
		node.ActiveClaimToken == nil || node.InputHash == nil || *node.InputHash != inputHash || len(node.Input) == 0 {
		return preparationConflict("Generation Workflow source is not executable")
	}
	_, _, persistedHash, parseErr := workflowdomain.ParseNodeInput(json.RawMessage(node.Input))
	if parseErr != nil || persistedHash != inputHash {
		return preparationConflict("Generation Workflow input facts have drifted")
	}
	return nil
}

func (repo *preparationRepository) FindReceipt(
	ctx context.Context,
	workspaceID, operation, key string,
) (platformcommand.Receipt, error) {
	return commandgorm.Find(ctx, repo.database, workspaceID, operation, key)
}

func (repo *preparationRepository) FindReceiptByID(
	ctx context.Context,
	receiptID string,
) (platformcommand.Receipt, error) {
	return commandgorm.FindByID(ctx, repo.database, receiptID)
}

func (repo *preparationRepository) EnsureReceipt(
	ctx context.Context,
	receipt platformcommand.Receipt,
) (platformcommand.Receipt, error) {
	return commandgorm.Ensure(ctx, repo.database, receipt)
}

func (repo *preparationRepository) FindIntent(ctx context.Context, intentID string) (domain.Intent, error) {
	return repo.findIntent(ctx, intentID, false)
}

func (repo *preparationRepository) GetIntentForUpdate(ctx context.Context, intentID string) (domain.Intent, error) {
	return repo.findIntent(ctx, intentID, true)
}

func (repo *preparationRepository) findIntent(
	ctx context.Context,
	intentID string,
	forUpdate bool,
) (domain.Intent, error) {
	id, err := uuid.Parse(intentID)
	if err != nil {
		return domain.Intent{}, application.ErrIntentNotFound
	}
	query := repo.database.WithContext(ctx)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.GenerationIntent
	if err = query.First(&record, "id = ?", id).Error; err != nil {
		return domain.Intent{}, normalizePreparationIntentNotFound(err)
	}
	return intentDomain(record), nil
}

func (repo *preparationRepository) EnsureIntent(ctx context.Context, desired domain.Intent) (domain.Intent, error) {
	record, err := intentRecord(desired)
	if err != nil {
		return domain.Intent{}, err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "node_run_id"}}, DoNothing: true,
	}).Create(&record).Error; err != nil {
		return domain.Intent{}, err
	}
	var persisted model.GenerationIntent
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("node_run_id = ?", record.NodeRunID).First(&persisted).Error; err != nil {
		return domain.Intent{}, fmt.Errorf("load ensured Generation intent: %w", err)
	}
	return intentDomain(persisted), nil
}

func (repo *preparationRepository) UpdateIntent(
	ctx context.Context,
	desired domain.Intent,
	expectedRevision int64,
) (domain.Intent, error) {
	record, err := intentRecord(desired)
	if err != nil {
		return domain.Intent{}, err
	}
	updated := repo.database.WithContext(ctx).Model(&model.GenerationIntent{}).
		Where("id = ? AND revision = ?", record.ID, expectedRevision).
		Updates(map[string]any{
			"cost_estimate_id": record.CostEstimateID, "cost_reservation_id": record.CostReservationID,
			"quota_reservation_id":         record.QuotaReservationID,
			"cost_estimate_receipt_id":     record.CostEstimateReceiptID,
			"cost_reservation_receipt_id":  record.CostReservationReceiptID,
			"quota_reservation_receipt_id": record.QuotaReservationReceiptID,
			"cost_release_receipt_id":      record.CostReleaseReceiptID,
			"quota_release_receipt_id":     record.QuotaReleaseReceiptID,
			"cost_settlement_receipt_id":   record.CostSettlementReceiptID,
			"quota_consumption_receipt_id": record.QuotaConsumptionReceiptID,
			"generation_request_id":        record.GenerationRequestID,
			"provider_job_id":              record.ProviderJobID,
			"provider_call_set_hash":       record.ProviderCallSetHash,
			"status":                       record.Status, "claimant": record.Claimant, "claim_token": record.ClaimToken,
			"claim_expires_at": record.ClaimExpiresAt, "claim_fencing_version": record.ClaimFencingVersion,
			"cancelled_at": record.CancelledAt, "revision": record.Revision,
			"content_hash": record.ContentHash, "updated_at": record.UpdatedAt,
		})
	if updated.Error != nil {
		return domain.Intent{}, updated.Error
	}
	if updated.RowsAffected != 1 {
		return domain.Intent{}, preparationConflict("Generation intent revision has changed")
	}
	return desired, nil
}

func intentRecord(value domain.Intent) (model.GenerationIntent, error) {
	ids := []string{
		value.ID, value.WorkspaceID, value.ProjectID, value.WorkflowRunID, value.NodeRunID, value.TargetID,
		value.BindingVersionID, value.ConnectionVersionID, value.CredentialVersionID,
		value.ModelProfileVersionID, value.PriceQuoteID, value.CreatedBy,
	}
	parsed := make([]uuid.UUID, len(ids))
	for index, raw := range ids {
		id, err := uuid.Parse(raw)
		if err != nil {
			return model.GenerationIntent{}, err
		}
		parsed[index] = id
	}
	costEstimateID, err := optionalPreparationUUID(value.CostEstimateID)
	if err != nil {
		return model.GenerationIntent{}, err
	}
	costReservationID, err := optionalPreparationUUID(value.CostReservationID)
	if err != nil {
		return model.GenerationIntent{}, err
	}
	quotaReservationID, err := optionalPreparationUUID(value.QuotaReservationID)
	if err != nil {
		return model.GenerationIntent{}, err
	}
	costEstimateReceiptID, err := optionalPreparationUUID(value.CostEstimateReceiptID)
	if err != nil {
		return model.GenerationIntent{}, err
	}
	costReservationReceiptID, err := optionalPreparationUUID(value.CostReservationReceiptID)
	if err != nil {
		return model.GenerationIntent{}, err
	}
	quotaReservationReceiptID, err := optionalPreparationUUID(value.QuotaReservationReceiptID)
	if err != nil {
		return model.GenerationIntent{}, err
	}
	costReleaseReceiptID, err := optionalPreparationUUID(value.CostReleaseReceiptID)
	if err != nil {
		return model.GenerationIntent{}, err
	}
	quotaReleaseReceiptID, err := optionalPreparationUUID(value.QuotaReleaseReceiptID)
	if err != nil {
		return model.GenerationIntent{}, err
	}
	costSettlementReceiptID, err := optionalPreparationUUID(value.CostSettlementReceiptID)
	if err != nil {
		return model.GenerationIntent{}, err
	}
	quotaConsumptionReceiptID, err := optionalPreparationUUID(value.QuotaConsumptionReceiptID)
	if err != nil {
		return model.GenerationIntent{}, err
	}
	generationRequestID, err := optionalPreparationUUID(value.GenerationRequestID)
	if err != nil {
		return model.GenerationIntent{}, err
	}
	providerJobID, err := optionalPreparationUUID(value.ProviderJobID)
	if err != nil {
		return model.GenerationIntent{}, err
	}
	claimToken, err := optionalPreparationUUID(optionalPreparationString(value.ClaimToken))
	if err != nil {
		return model.GenerationIntent{}, err
	}
	return model.GenerationIntent{
		ID: parsed[0], WorkspaceID: parsed[1], ProjectID: parsed[2], WorkflowRunID: parsed[3], NodeRunID: parsed[4],
		TargetID: parsed[5], TargetHash: value.TargetHash,
		BindingVersionID: parsed[6], BindingRevision: value.BindingRevision, BindingContentHash: value.BindingContentHash,
		ConnectionVersionID: parsed[7], CredentialVersionID: parsed[8], ModelProfileVersionID: parsed[9],
		ModelProfileRevision: value.ModelProfileRevision, ModelProfileContentHash: value.ModelProfileContentHash,
		PriceQuoteID: parsed[10], PriceQuoteRevision: value.PriceQuoteRevision,
		PriceQuoteContentHash: value.PriceQuoteContentHash, BillingMetric: value.BillingMetric,
		EstimatedUnits: value.EstimatedUnits,
		CostEstimateID: costEstimateID, CostReservationID: costReservationID, QuotaReservationID: quotaReservationID,
		CostEstimateReceiptID: costEstimateReceiptID, CostReservationReceiptID: costReservationReceiptID,
		QuotaReservationReceiptID: quotaReservationReceiptID, CostReleaseReceiptID: costReleaseReceiptID,
		QuotaReleaseReceiptID: quotaReleaseReceiptID, CostSettlementReceiptID: costSettlementReceiptID,
		QuotaConsumptionReceiptID: quotaConsumptionReceiptID, GenerationRequestID: generationRequestID,
		ProviderJobID: providerJobID, ProviderCallSetHash: optionalPreparationStringPointer(value.ProviderCallSetHash),
		Status: value.Status, Claimant: clonePreparationString(value.Claimant),
		ClaimToken: claimToken, ClaimExpiresAt: clonePreparationTime(value.ClaimExpiresAt),
		ClaimFencingVersion: value.ClaimFencingVersion, CancelledAt: clonePreparationTime(value.CancelledAt),
		Revision: value.Revision, ContentHash: value.ContentHash, CreatedBy: parsed[11],
		InitiatorTokenVersion: value.InitiatorTokenVersion, CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}, nil
}

func intentDomain(value model.GenerationIntent) domain.Intent {
	return domain.Intent{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		WorkflowRunID: value.WorkflowRunID.String(), NodeRunID: value.NodeRunID.String(), TargetID: value.TargetID.String(),
		TargetHash: value.TargetHash, BindingVersionID: value.BindingVersionID.String(),
		BindingRevision: value.BindingRevision, BindingContentHash: value.BindingContentHash,
		ConnectionVersionID: value.ConnectionVersionID.String(), CredentialVersionID: value.CredentialVersionID.String(),
		ModelProfileVersionID: value.ModelProfileVersionID.String(), ModelProfileRevision: value.ModelProfileRevision,
		ModelProfileContentHash: value.ModelProfileContentHash, PriceQuoteID: value.PriceQuoteID.String(),
		PriceQuoteRevision: value.PriceQuoteRevision, PriceQuoteContentHash: value.PriceQuoteContentHash,
		BillingMetric: value.BillingMetric, EstimatedUnits: value.EstimatedUnits,
		CostEstimateID:            optionalPreparationUUIDString(value.CostEstimateID),
		CostReservationID:         optionalPreparationUUIDString(value.CostReservationID),
		QuotaReservationID:        optionalPreparationUUIDString(value.QuotaReservationID),
		CostEstimateReceiptID:     optionalPreparationUUIDString(value.CostEstimateReceiptID),
		CostReservationReceiptID:  optionalPreparationUUIDString(value.CostReservationReceiptID),
		QuotaReservationReceiptID: optionalPreparationUUIDString(value.QuotaReservationReceiptID),
		CostReleaseReceiptID:      optionalPreparationUUIDString(value.CostReleaseReceiptID),
		QuotaReleaseReceiptID:     optionalPreparationUUIDString(value.QuotaReleaseReceiptID),
		CostSettlementReceiptID:   optionalPreparationUUIDString(value.CostSettlementReceiptID),
		QuotaConsumptionReceiptID: optionalPreparationUUIDString(value.QuotaConsumptionReceiptID),
		GenerationRequestID:       optionalPreparationUUIDString(value.GenerationRequestID),
		ProviderJobID:             optionalPreparationUUIDString(value.ProviderJobID),
		ProviderCallSetHash:       optionalPreparationStringValue(value.ProviderCallSetHash),
		Status:                    value.Status, Claimant: clonePreparationString(value.Claimant),
		ClaimToken: optionalPreparationUUIDPointerString(value.ClaimToken), ClaimExpiresAt: clonePreparationTime(value.ClaimExpiresAt),
		ClaimFencingVersion: value.ClaimFencingVersion, CancelledAt: clonePreparationTime(value.CancelledAt),
		Revision: value.Revision, ContentHash: value.ContentHash, CreatedBy: value.CreatedBy.String(),
		InitiatorTokenVersion: value.InitiatorTokenVersion, CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}
}

func parsePreparationIDs(values ...string) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, error) {
	if len(values) != 4 {
		return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, errors.New("invalid Generation Workflow source identifiers")
	}
	parsed := make([]uuid.UUID, len(values))
	for index, value := range values {
		id, err := uuid.Parse(value)
		if err != nil {
			return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, err
		}
		parsed[index] = id
	}
	return parsed[0], parsed[1], parsed[2], parsed[3], nil
}

func optionalPreparationUUID(value string) (*uuid.UUID, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func optionalPreparationString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func optionalPreparationStringPointer(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	result := value
	return &result
}

func optionalPreparationStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func optionalPreparationUUIDString(value *uuid.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func optionalPreparationUUIDPointerString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	result := value.String()
	return &result
}

func clonePreparationString(value *string) *string {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func clonePreparationTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := value.UTC()
	return &result
}

func normalizePreparationIntentNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrIntentNotFound
	}
	return err
}

func normalizePreparationSourceNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return preparationConflict("Generation Workflow source was not found")
	}
	return err
}

func preparationConflict(message string) error {
	return &application.Error{Code: "state_conflict", Message: message, Status: 409}
}

var _ application.PreparationTransactionManager = (*PreparationStore)(nil)
var _ application.PreparationRepository = (*preparationRepository)(nil)
