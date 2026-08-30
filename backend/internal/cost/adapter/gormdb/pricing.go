package gormdb

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	"github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

func (repo *repository) FindPriceQuote(ctx context.Context, quoteID string) (domain.PriceQuote, error) {
	id, err := uuid.Parse(quoteID)
	if err != nil {
		return domain.PriceQuote{}, application.ErrPriceQuoteNotFound
	}
	var record model.CostPriceQuote
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return domain.PriceQuote{}, normalizePriceQuoteNotFound(err)
	}
	return priceQuoteDomain(record), nil
}

func (repo *repository) FindModelProfileVersion(ctx context.Context, profileID string) (application.ModelProfileVersion, error) {
	id, err := uuid.Parse(profileID)
	if err != nil {
		return application.ModelProfileVersion{}, application.ErrModelProfileVersionNotFound
	}
	var record model.ProviderModelProfileVersion
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return application.ModelProfileVersion{}, normalizeProviderFactNotFound(err, application.ErrModelProfileVersionNotFound)
	}
	return application.ModelProfileVersion{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), BillingMetric: record.BillingMetric,
		State: record.State, ContentHash: record.ContentHash, Revision: record.Revision,
	}, nil
}

func (repo *repository) FindProviderBindingVersion(ctx context.Context, bindingID string) (application.ProviderBindingVersion, error) {
	id, err := uuid.Parse(bindingID)
	if err != nil {
		return application.ProviderBindingVersion{}, application.ErrProviderBindingVersionNotFound
	}
	var record model.ProjectProviderBindingVersion
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return application.ProviderBindingVersion{}, normalizeProviderFactNotFound(err, application.ErrProviderBindingVersionNotFound)
	}
	return application.ProviderBindingVersion{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		ModelProfileVersionID: record.ModelProfileVersionID.String(), Revision: record.Revision, ContentHash: record.ContentHash,
	}, nil
}

func (repo *repository) HasAnyPriceQuote(ctx context.Context, projectID string) (bool, error) {
	project, err := uuid.Parse(projectID)
	if err != nil {
		return false, application.ErrPriceQuoteNotFound
	}
	var count int64
	err = repo.database.WithContext(ctx).Model(&model.CostPriceQuote{}).Where("project_id = ?", project).Count(&count).Error
	return count > 0, err
}

func (repo *repository) FindCurrentPriceQuote(
	ctx context.Context,
	projectID, modelProfileVersionID string,
) (domain.PriceQuote, error) {
	return repo.findCurrentPriceQuote(ctx, projectID, modelProfileVersionID, false)
}

func (repo *repository) GetCurrentPriceQuoteForUpdate(
	ctx context.Context,
	projectID, modelProfileVersionID string,
) (domain.PriceQuote, error) {
	return repo.findCurrentPriceQuote(ctx, projectID, modelProfileVersionID, true)
}

func (repo *repository) findCurrentPriceQuote(
	ctx context.Context,
	projectID, modelProfileVersionID string,
	forUpdate bool,
) (domain.PriceQuote, error) {
	project, projectErr := uuid.Parse(projectID)
	profile, profileErr := uuid.Parse(modelProfileVersionID)
	if projectErr != nil || profileErr != nil {
		return domain.PriceQuote{}, application.ErrPriceQuoteNotFound
	}
	query := repo.database.WithContext(ctx).Where("project_id = ? AND model_profile_version_id = ?", project, profile).
		Order("revision DESC")
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.CostPriceQuote
	if err := query.First(&record).Error; err != nil {
		return domain.PriceQuote{}, normalizePriceQuoteNotFound(err)
	}
	return priceQuoteDomain(record), nil
}

func (repo *repository) EnsurePriceQuote(ctx context.Context, desired domain.PriceQuote) (domain.PriceQuote, error) {
	record, err := priceQuoteRecord(desired)
	if err != nil {
		return domain.PriceQuote{}, err
	}
	created := record
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "project_id"}, {Name: "model_profile_version_id"}, {Name: "revision"}}, DoNothing: true,
	}).Create(&created).Error; err != nil {
		return domain.PriceQuote{}, err
	}
	var persisted model.CostPriceQuote
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where(
		"project_id = ? AND model_profile_version_id = ? AND revision = ?",
		record.ProjectID, record.ModelProfileVersionID, record.Revision,
	).First(&persisted).Error; err != nil {
		return domain.PriceQuote{}, fmt.Errorf("load ensured cost price quote: %w", err)
	}
	return priceQuoteDomain(persisted), nil
}

func (repo *repository) FindEstimate(ctx context.Context, estimateID string) (domain.Estimate, error) {
	id, err := uuid.Parse(estimateID)
	if err != nil {
		return domain.Estimate{}, application.ErrEstimateNotFound
	}
	var record model.CostEstimate
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return domain.Estimate{}, normalizeEstimateNotFound(err)
	}
	return estimateDomain(record), nil
}

func (repo *repository) FindEstimateBySource(
	ctx context.Context,
	projectID, sourceType, sourceID string,
) (domain.Estimate, error) {
	project, projectErr := uuid.Parse(projectID)
	source, sourceErr := uuid.Parse(sourceID)
	if projectErr != nil || sourceErr != nil {
		return domain.Estimate{}, application.ErrEstimateNotFound
	}
	var record model.CostEstimate
	if err := repo.database.WithContext(ctx).Where(
		"project_id = ? AND source_type = ? AND source_id = ?", project, sourceType, source,
	).First(&record).Error; err != nil {
		return domain.Estimate{}, normalizeEstimateNotFound(err)
	}
	return estimateDomain(record), nil
}

func (repo *repository) EnsureEstimate(ctx context.Context, desired domain.Estimate) (domain.Estimate, error) {
	record, err := estimateRecord(desired)
	if err != nil {
		return domain.Estimate{}, err
	}
	created := record
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "project_id"}, {Name: "source_type"}, {Name: "source_id"}}, DoNothing: true,
	}).Create(&created).Error; err != nil {
		return domain.Estimate{}, err
	}
	var persisted model.CostEstimate
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where(
		"project_id = ? AND source_type = ? AND source_id = ?", record.ProjectID, record.SourceType, record.SourceID,
	).First(&persisted).Error; err != nil {
		return domain.Estimate{}, fmt.Errorf("load ensured cost estimate: %w", err)
	}
	return estimateDomain(persisted), nil
}

func priceQuoteRecord(value domain.PriceQuote) (model.CostPriceQuote, error) {
	id, workspace, project, creator, err := parsePriceQuoteIDs(value)
	if err != nil {
		return model.CostPriceQuote{}, err
	}
	profile, err := uuid.Parse(value.ModelProfileVersionID)
	if err != nil {
		return model.CostPriceQuote{}, err
	}
	return model.CostPriceQuote{
		ID: id, WorkspaceID: workspace, ProjectID: project, ModelProfileVersionID: profile,
		BillingMetric: value.BillingMetric, ReservationUnitAmount: value.ReservationUnitAmount,
		Currency: value.Currency, Revision: value.Revision,
		ContentHash: value.ContentHash, CreatedBy: creator, CreatedAt: value.CreatedAt,
	}, nil
}

func priceQuoteDomain(value model.CostPriceQuote) domain.PriceQuote {
	return domain.PriceQuote{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		ModelProfileVersionID: value.ModelProfileVersionID.String(), BillingMetric: value.BillingMetric,
		ReservationUnitAmount: value.ReservationUnitAmount, Currency: value.Currency, Revision: value.Revision,
		ContentHash: value.ContentHash, CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt.UTC(),
	}
}

func estimateRecord(value domain.Estimate) (model.CostEstimate, error) {
	ids := []string{
		value.ID, value.WorkspaceID, value.ProjectID, value.BudgetPolicyID,
		value.PriceQuoteID, value.ProviderBindingVersionID, value.ModelProfileVersionID,
		value.SourceID, value.CreatedBy,
	}
	parsed := make([]uuid.UUID, len(ids))
	for index, raw := range ids {
		id, err := uuid.Parse(raw)
		if err != nil {
			return model.CostEstimate{}, err
		}
		parsed[index] = id
	}
	return model.CostEstimate{
		ID: parsed[0], WorkspaceID: parsed[1], ProjectID: parsed[2], BudgetPolicyID: parsed[3],
		PriceQuoteID: parsed[4], ProviderBindingVersionID: parsed[5],
		ProviderBindingRevision:    value.ProviderBindingRevision,
		ProviderBindingContentHash: value.ProviderBindingContentHash,
		ModelProfileVersionID:      parsed[6], ModelProfileRevision: value.ModelProfileRevision,
		ModelProfileContentHash: value.ModelProfileContentHash, PriceQuoteContentHash: value.PriceQuoteContentHash,
		Metric: value.Metric, SourceType: value.SourceType, SourceID: parsed[7],
		Units: value.Units, UnitAmount: value.UnitAmount, TotalAmount: value.TotalAmount, Currency: value.Currency,
		PriceQuoteRevision: value.PriceQuoteRevision, BudgetPolicyRevision: value.BudgetPolicyRevision,
		BudgetLimit: value.BudgetLimit, ContentHash: value.ContentHash, CreatedBy: parsed[8], CreatedAt: value.CreatedAt,
	}, nil
}

func estimateDomain(value model.CostEstimate) domain.Estimate {
	return domain.Estimate{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		BudgetPolicyID: value.BudgetPolicyID.String(), PriceQuoteID: value.PriceQuoteID.String(),
		ProviderBindingVersionID:   value.ProviderBindingVersionID.String(),
		ProviderBindingRevision:    value.ProviderBindingRevision,
		ProviderBindingContentHash: value.ProviderBindingContentHash,
		ModelProfileVersionID:      value.ModelProfileVersionID.String(), ModelProfileRevision: value.ModelProfileRevision,
		ModelProfileContentHash: value.ModelProfileContentHash, PriceQuoteContentHash: value.PriceQuoteContentHash,
		Metric: value.Metric, SourceType: value.SourceType, SourceID: value.SourceID.String(), Units: value.Units,
		UnitAmount: value.UnitAmount, TotalAmount: value.TotalAmount, Currency: value.Currency,
		PriceQuoteRevision: value.PriceQuoteRevision, BudgetPolicyRevision: value.BudgetPolicyRevision,
		BudgetLimit: value.BudgetLimit, ContentHash: value.ContentHash,
		CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt.UTC(),
	}
}

func parsePriceQuoteIDs(value domain.PriceQuote) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, error) {
	values := []string{value.ID, value.WorkspaceID, value.ProjectID, value.CreatedBy}
	parsed := make([]uuid.UUID, len(values))
	for index, raw := range values {
		id, err := uuid.Parse(raw)
		if err != nil {
			return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, err
		}
		parsed[index] = id
	}
	return parsed[0], parsed[1], parsed[2], parsed[3], nil
}

func normalizePriceQuoteNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrPriceQuoteNotFound
	}
	return err
}

func normalizeEstimateNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrEstimateNotFound
	}
	return err
}

func normalizeProviderFactNotFound(err, notFound error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound
	}
	return err
}
