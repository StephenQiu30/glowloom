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

func (repo *repository) FindReservation(ctx context.Context, reservationID string) (domain.Reservation, error) {
	return repo.findReservation(ctx, reservationID, false)
}

func (repo *repository) GetReservationForUpdate(ctx context.Context, reservationID string) (domain.Reservation, error) {
	return repo.findReservation(ctx, reservationID, true)
}

func (repo *repository) findReservation(
	ctx context.Context,
	reservationID string,
	forUpdate bool,
) (domain.Reservation, error) {
	id, err := uuid.Parse(reservationID)
	if err != nil {
		return domain.Reservation{}, application.ErrReservationNotFound
	}
	query := repo.database.WithContext(ctx)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.CostReservation
	if err = query.First(&record, "id = ?", id).Error; err != nil {
		return domain.Reservation{}, normalizeReservationNotFound(err)
	}
	return reservationDomain(record), nil
}

func (repo *repository) FindReservationByEstimate(ctx context.Context, estimateID string) (domain.Reservation, error) {
	estimate, err := uuid.Parse(estimateID)
	if err != nil {
		return domain.Reservation{}, application.ErrReservationNotFound
	}
	var record model.CostReservation
	if err = repo.database.WithContext(ctx).Where("estimate_id = ?", estimate).First(&record).Error; err != nil {
		return domain.Reservation{}, normalizeReservationNotFound(err)
	}
	return reservationDomain(record), nil
}

func (repo *repository) EnsureReservation(ctx context.Context, desired domain.Reservation) (domain.Reservation, error) {
	record, err := reservationRecord(desired)
	if err != nil {
		return domain.Reservation{}, err
	}
	created := record
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "estimate_id"}}, DoNothing: true,
	}).Create(&created).Error; err != nil {
		return domain.Reservation{}, err
	}
	var persisted model.CostReservation
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where(
		"estimate_id = ?", record.EstimateID,
	).First(&persisted).Error; err != nil {
		return domain.Reservation{}, fmt.Errorf("load ensured cost reservation: %w", err)
	}
	return reservationDomain(persisted), nil
}

func (repo *repository) UpdateReservation(
	ctx context.Context,
	desired domain.Reservation,
	expectedRevision int64,
) (domain.Reservation, error) {
	record, err := reservationRecord(desired)
	if err != nil {
		return domain.Reservation{}, err
	}
	updated := repo.database.WithContext(ctx).Model(&model.CostReservation{}).
		Where("id = ? AND revision = ?", record.ID, expectedRevision).
		Updates(map[string]any{
			"settled_units": record.SettledUnits, "settled_amount": record.SettledAmount,
			"status": record.Status, "usage_receipt_id": record.UsageReceiptID,
			"revision": record.Revision, "content_hash": record.ContentHash, "updated_by": record.UpdatedBy,
			"settled_at": record.SettledAt, "released_at": record.ReleasedAt, "updated_at": record.UpdatedAt,
		})
	if updated.Error != nil {
		return domain.Reservation{}, updated.Error
	}
	if updated.RowsAffected != 1 {
		return domain.Reservation{}, conflict("Cost reservation revision has changed")
	}
	return desired, nil
}

func (repo *repository) ListReservations(ctx context.Context, projectID string) ([]domain.Reservation, error) {
	project, err := uuid.Parse(projectID)
	if err != nil {
		return nil, application.ErrReservationNotFound
	}
	var records []model.CostReservation
	if err = repo.database.WithContext(ctx).Where("project_id = ?", project).
		Order("created_at ASC, id ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]domain.Reservation, 0, len(records))
	for _, record := range records {
		result = append(result, reservationDomain(record))
	}
	return result, nil
}

func (repo *repository) FindLedgerEntry(
	ctx context.Context,
	reservationID, entryType string,
) (domain.LedgerEntry, error) {
	reservation, err := uuid.Parse(reservationID)
	if err != nil {
		return domain.LedgerEntry{}, application.ErrLedgerEntryNotFound
	}
	var record model.CostLedgerEntry
	if err = repo.database.WithContext(ctx).Where(
		"reservation_id = ? AND entry_type = ?", reservation, entryType,
	).First(&record).Error; err != nil {
		return domain.LedgerEntry{}, normalizeLedgerEntryNotFound(err)
	}
	return ledgerEntryDomain(record), nil
}

func (repo *repository) EnsureLedgerEntry(ctx context.Context, desired domain.LedgerEntry) (domain.LedgerEntry, error) {
	record, err := ledgerEntryRecord(desired)
	if err != nil {
		return domain.LedgerEntry{}, err
	}
	created := record
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "reservation_id"}, {Name: "entry_type"}}, DoNothing: true,
	}).Create(&created).Error; err != nil {
		return domain.LedgerEntry{}, err
	}
	var persisted model.CostLedgerEntry
	if err = repo.database.WithContext(ctx).Where(
		"reservation_id = ? AND entry_type = ?", record.ReservationID, record.EntryType,
	).First(&persisted).Error; err != nil {
		return domain.LedgerEntry{}, fmt.Errorf("load ensured cost ledger entry: %w", err)
	}
	return ledgerEntryDomain(persisted), nil
}

func (repo *repository) ListLedgerEntries(ctx context.Context, reservationID string) ([]domain.LedgerEntry, error) {
	reservation, err := uuid.Parse(reservationID)
	if err != nil {
		return nil, application.ErrLedgerEntryNotFound
	}
	var records []model.CostLedgerEntry
	if err = repo.database.WithContext(ctx).Where("reservation_id = ?", reservation).
		Order("sequence ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]domain.LedgerEntry, 0, len(records))
	for _, record := range records {
		result = append(result, ledgerEntryDomain(record))
	}
	return result, nil
}

func reservationRecord(value domain.Reservation) (model.CostReservation, error) {
	ids := []string{
		value.ID, value.WorkspaceID, value.ProjectID, value.EstimateID, value.BudgetPolicyID,
		value.PriceQuoteID, value.SourceID, value.CreatedBy, value.UpdatedBy,
	}
	parsed := make([]uuid.UUID, len(ids))
	for index, raw := range ids {
		id, err := uuid.Parse(raw)
		if err != nil {
			return model.CostReservation{}, err
		}
		parsed[index] = id
	}
	usageReceiptID, err := optionalUUID(value.UsageReceiptID)
	if err != nil {
		return model.CostReservation{}, err
	}
	return model.CostReservation{
		ID: parsed[0], WorkspaceID: parsed[1], ProjectID: parsed[2], EstimateID: parsed[3],
		BudgetPolicyID: parsed[4], PriceQuoteID: parsed[5], Metric: value.Metric,
		SourceType: value.SourceType, SourceID: parsed[6], EstimatedUnits: value.EstimatedUnits,
		SettledUnits: value.SettledUnits, UnitAmount: value.UnitAmount, ReservedAmount: value.ReservedAmount,
		SettledAmount: value.SettledAmount, BudgetLimit: value.BudgetLimit, Currency: value.Currency,
		PriceQuoteRevision: value.PriceQuoteRevision, BudgetPolicyRevision: value.BudgetPolicyRevision,
		Status: value.Status, UsageReceiptID: usageReceiptID, Revision: value.Revision, ContentHash: value.ContentHash,
		CreatedBy: parsed[7], UpdatedBy: parsed[8], SettledAt: value.SettledAt, ReleasedAt: value.ReleasedAt,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func reservationDomain(value model.CostReservation) domain.Reservation {
	return domain.Reservation{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		EstimateID: value.EstimateID.String(), BudgetPolicyID: value.BudgetPolicyID.String(),
		PriceQuoteID: value.PriceQuoteID.String(), Metric: value.Metric, SourceType: value.SourceType,
		SourceID: value.SourceID.String(), EstimatedUnits: value.EstimatedUnits, SettledUnits: value.SettledUnits,
		UnitAmount: value.UnitAmount, ReservedAmount: value.ReservedAmount, SettledAmount: value.SettledAmount,
		BudgetLimit: value.BudgetLimit, Currency: value.Currency, PriceQuoteRevision: value.PriceQuoteRevision,
		BudgetPolicyRevision: value.BudgetPolicyRevision, Status: value.Status,
		UsageReceiptID: optionalUUIDString(value.UsageReceiptID), Revision: value.Revision, ContentHash: value.ContentHash,
		CreatedBy: value.CreatedBy.String(), UpdatedBy: value.UpdatedBy.String(), SettledAt: value.SettledAt,
		ReleasedAt: value.ReleasedAt, CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}
}

func ledgerEntryRecord(value domain.LedgerEntry) (model.CostLedgerEntry, error) {
	ids := []string{
		value.ID, value.WorkspaceID, value.ProjectID, value.ReservationID, value.EstimateID, value.CreatedBy,
	}
	parsed := make([]uuid.UUID, len(ids))
	for index, raw := range ids {
		id, err := uuid.Parse(raw)
		if err != nil {
			return model.CostLedgerEntry{}, err
		}
		parsed[index] = id
	}
	usageReceiptID, err := optionalUUID(value.UsageReceiptID)
	if err != nil {
		return model.CostLedgerEntry{}, err
	}
	return model.CostLedgerEntry{
		ID: parsed[0], WorkspaceID: parsed[1], ProjectID: parsed[2], ReservationID: parsed[3],
		EstimateID: parsed[4], EntryType: value.EntryType, Sequence: value.Sequence,
		ReservedDelta: value.ReservedDelta, SettledDelta: value.SettledDelta, Currency: value.Currency,
		UsageReceiptID: usageReceiptID, ContentHash: value.ContentHash, CreatedBy: parsed[5], CreatedAt: value.CreatedAt,
	}, nil
}

func ledgerEntryDomain(value model.CostLedgerEntry) domain.LedgerEntry {
	return domain.LedgerEntry{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		ReservationID: value.ReservationID.String(), EstimateID: value.EstimateID.String(),
		EntryType: value.EntryType, Sequence: value.Sequence, ReservedDelta: value.ReservedDelta,
		SettledDelta: value.SettledDelta, Currency: value.Currency,
		UsageReceiptID: optionalUUIDString(value.UsageReceiptID), ContentHash: value.ContentHash,
		CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt.UTC(),
	}
}

func optionalUUID(value *string) (*uuid.UUID, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := uuid.Parse(*value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func optionalUUIDString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	result := value.String()
	return &result
}

func normalizeReservationNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrReservationNotFound
	}
	return err
}

func normalizeLedgerEntryNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrLedgerEntryNotFound
	}
	return err
}
