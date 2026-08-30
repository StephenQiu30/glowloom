package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

const (
	reserveEstimateOperation    = "cost.reservation.reserve"
	settleReservationOperation  = "cost.reservation.settle"
	releaseReservationOperation = "cost.reservation.release"
)

type ReserveEstimateCommand struct {
	EstimateID, IdempotencyKey string
}

type SettleReservationCommand struct {
	ReservationID, UsageReceiptID, IdempotencyKey string
	SettledUnits                                  int64
}

type ReleaseReservationCommand struct {
	ReservationID, IdempotencyKey string
}

type ReservationResult struct {
	Reservation domain.Reservation
	LedgerEntry domain.LedgerEntry
	Receipt     platformcommand.Receipt
}

type ReservationView struct {
	Reservation   domain.Reservation
	LedgerEntries []domain.LedgerEntry
}

type reservationReceipt struct {
	Reservation domain.Reservation `json:"reservation"`
	LedgerEntry domain.LedgerEntry `json:"ledger_entry"`
}

type reservationHashInput struct {
	WorkspaceID, ProjectID, EstimateID, BudgetPolicyID, PriceQuoteID string
	Metric, SourceType, SourceID                                     string
	EstimatedUnits, SettledUnits                                     int64
	UnitAmount, ReservedAmount, SettledAmount, BudgetLimit, Currency string
	PriceQuoteRevision, BudgetPolicyRevision                         int64
	Status, UsageReceiptID                                           string
	Revision                                                         int64
	UpdatedBy                                                        string
}

type ledgerHashInput struct {
	WorkspaceID, ProjectID, ReservationID, EstimateID string
	EntryType                                         string
	Sequence                                          int64
	ReservedDelta, SettledDelta, Currency             string
	UsageReceiptID, CreatedBy                         string
}

func (service *Service) ReserveEstimate(
	ctx context.Context,
	actor Actor,
	command ReserveEstimateCommand,
) (ReservationResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.EstimateID = strings.TrimSpace(command.EstimateID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.transactions == nil || service.config.Now == nil || service.config.NewID == nil ||
		!validActor(actor) || !validUUID(command.EstimateID) || command.IdempotencyKey == "" ||
		len(command.IdempotencyKey) > 200 {
		return ReservationResult{}, invalid("Invalid cost reservation request")
	}
	inputHash, err := platformcommand.InputHash(struct {
		ActorID string
		Command ReserveEstimateCommand
	}{ActorID: actor.UserID, Command: command})
	if err != nil {
		return ReservationResult{}, err
	}
	now := service.config.Now().UTC()
	var result ReservationResult
	err = service.transactions.WithinCostTransaction(ctx, func(repo Repository) error {
		estimate, findErr := repo.FindEstimate(ctx, command.EstimateID)
		if findErr != nil {
			return findErr
		}
		if validationErr := validateEstimate(estimate); validationErr != nil {
			return validationErr
		}
		scope, authorizeErr := repo.AuthorizeProject(ctx, actor, estimate.ProjectID, "write")
		if authorizeErr != nil {
			return authorizeErr
		}
		if estimate.WorkspaceID != scope.WorkspaceID {
			return conflict("Cost estimate scope has drifted")
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, scope.WorkspaceID, reserveEstimateOperation, command.IdempotencyKey); receiptErr == nil {
			return replayCostReservation(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		budget, budgetErr := repo.GetBudgetForUpdate(ctx, scope.ProjectID)
		if budgetErr != nil {
			return budgetErr
		}
		if validationErr := validateBudget(budget); validationErr != nil {
			return validationErr
		}
		if budget.ID != estimate.BudgetPolicyID || budget.WorkspaceID != scope.WorkspaceID ||
			budget.Currency != estimate.Currency {
			return conflict("Cost reservation budget or currency has drifted")
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, scope.WorkspaceID, reserveEstimateOperation, command.IdempotencyKey); receiptErr == nil {
			return replayCostReservation(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		reservations, listErr := repo.ListReservations(ctx, scope.ProjectID)
		if listErr != nil {
			return listErr
		}
		reserved, settled, reconcileErr := reconcileBudgetUsage(budget, reservations)
		if reconcileErr != nil {
			return reconcileErr
		}
		if existing, existingErr := repo.FindReservationByEstimate(ctx, estimate.ID); existingErr == nil {
			return reuseReservedEstimate(
				ctx, repo, service.config.NewID, actor, command.IdempotencyKey, inputHash,
				estimate, existing, now, &result,
			)
		} else if !errors.Is(existingErr, ErrReservationNotFound) {
			return existingErr
		}
		if reserved.Add(settled).Add(estimate.TotalAmount).GreaterThan(budget.LimitAmount) {
			return budgetExceeded()
		}
		reservationID := strings.TrimSpace(service.config.NewID())
		if !validUUID(reservationID) {
			return errors.New("cost reservation identifier is invalid")
		}
		desired := domain.Reservation{
			ID: reservationID, WorkspaceID: scope.WorkspaceID, ProjectID: scope.ProjectID,
			EstimateID: estimate.ID, BudgetPolicyID: budget.ID, PriceQuoteID: estimate.PriceQuoteID,
			Metric: estimate.Metric, SourceType: estimate.SourceType, SourceID: estimate.SourceID,
			EstimatedUnits: estimate.Units, SettledUnits: 0, UnitAmount: estimate.UnitAmount,
			ReservedAmount: estimate.TotalAmount, SettledAmount: decimal.Zero,
			BudgetLimit: budget.LimitAmount, Currency: estimate.Currency,
			PriceQuoteRevision: estimate.PriceQuoteRevision, BudgetPolicyRevision: budget.Revision,
			Status: domain.ReservationReserved, Revision: 1, CreatedBy: actor.UserID, UpdatedBy: actor.UserID,
			CreatedAt: now, UpdatedAt: now,
		}
		desired.ContentHash, listErr = reservationContentHash(desired)
		if listErr != nil {
			return listErr
		}
		persisted, ensureErr := repo.EnsureReservation(ctx, desired)
		if ensureErr != nil {
			return ensureErr
		}
		if !domain.SameReservationState(persisted, desired) {
			return platformcommand.ErrInputMismatch
		}
		entry, entryErr := service.appendLedgerEntry(ctx, repo, actor, persisted, domain.LedgerReservationCreated, now)
		if entryErr != nil {
			return entryErr
		}
		return storeCostReservationReceipt(
			ctx, repo, service.config.NewID, actor, reserveEstimateOperation, command.IdempotencyKey,
			inputHash, persisted, entry, now, &result,
		)
	})
	return result, normalizeError(err)
}

func (service *Service) SettleReservation(
	ctx context.Context,
	actor Actor,
	command SettleReservationCommand,
) (ReservationResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.ReservationID = strings.TrimSpace(command.ReservationID)
	command.UsageReceiptID = strings.TrimSpace(command.UsageReceiptID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.transactions == nil || service.config.Now == nil || service.config.NewID == nil ||
		!validActor(actor) || !validUUID(command.ReservationID) || !validUUID(command.UsageReceiptID) ||
		command.SettledUnits < 1 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return ReservationResult{}, invalid("Invalid cost settlement request")
	}
	return service.transitionReservation(ctx, actor, command, ReleaseReservationCommand{}, domain.ReservationSettled)
}

func (service *Service) ReleaseReservation(
	ctx context.Context,
	actor Actor,
	command ReleaseReservationCommand,
) (ReservationResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.ReservationID = strings.TrimSpace(command.ReservationID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.transactions == nil || service.config.Now == nil || service.config.NewID == nil ||
		!validActor(actor) || !validUUID(command.ReservationID) || command.IdempotencyKey == "" ||
		len(command.IdempotencyKey) > 200 {
		return ReservationResult{}, invalid("Invalid cost release request")
	}
	return service.transitionReservation(ctx, actor, SettleReservationCommand{}, command, domain.ReservationReleased)
}

func (service *Service) transitionReservation(
	ctx context.Context,
	actor Actor,
	settle SettleReservationCommand,
	release ReleaseReservationCommand,
	desiredStatus string,
) (ReservationResult, error) {
	operation, reservationID, key := settleReservationOperation, settle.ReservationID, settle.IdempotencyKey
	var command any = settle
	if desiredStatus == domain.ReservationReleased {
		operation, reservationID, key, command = releaseReservationOperation, release.ReservationID, release.IdempotencyKey, release
	}
	inputHash, err := platformcommand.InputHash(struct {
		ActorID string
		Command any
	}{ActorID: actor.UserID, Command: command})
	if err != nil {
		return ReservationResult{}, err
	}
	now := service.config.Now().UTC()
	var result ReservationResult
	err = service.transactions.WithinCostTransaction(ctx, func(repo Repository) error {
		snapshot, findErr := repo.FindReservation(ctx, reservationID)
		if findErr != nil {
			return findErr
		}
		scope, authorizeErr := repo.AuthorizeProject(ctx, actor, snapshot.ProjectID, "write")
		if authorizeErr != nil {
			return authorizeErr
		}
		if snapshot.WorkspaceID != scope.WorkspaceID {
			return conflict("Cost reservation scope has drifted")
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, scope.WorkspaceID, operation, key); receiptErr == nil {
			return replayCostReservation(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		budget, budgetErr := repo.GetBudgetForUpdate(ctx, scope.ProjectID)
		if budgetErr != nil {
			return budgetErr
		}
		if validationErr := validateBudget(budget); validationErr != nil {
			return validationErr
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, scope.WorkspaceID, operation, key); receiptErr == nil {
			return replayCostReservation(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		reservation, lockErr := repo.GetReservationForUpdate(ctx, reservationID)
		if lockErr != nil {
			return lockErr
		}
		if !domain.SameReservationBinding(snapshot, reservation) || reservation.BudgetPolicyID != budget.ID ||
			reservation.WorkspaceID != budget.WorkspaceID || reservation.Currency != budget.Currency {
			return conflict("Cost reservation binding has drifted")
		}
		entries, listErr := repo.ListLedgerEntries(ctx, reservation.ID)
		if listErr != nil {
			return listErr
		}
		if validationErr := validateReservationLifecycle(reservation, entries); validationErr != nil {
			return validationErr
		}
		reservations, listErr := repo.ListReservations(ctx, scope.ProjectID)
		if listErr != nil {
			return listErr
		}
		if _, _, reconcileErr := reconcileBudgetUsage(budget, reservations); reconcileErr != nil {
			return reconcileErr
		}
		if reservation.Status == desiredStatus {
			entryType := domain.LedgerReservationSettled
			if desiredStatus == domain.ReservationReleased {
				entryType = domain.LedgerReservationReleased
			}
			entry, entryErr := repo.FindLedgerEntry(ctx, reservation.ID, entryType)
			if entryErr != nil {
				return entryErr
			}
			if desiredStatus == domain.ReservationSettled &&
				(reservation.SettledUnits != settle.SettledUnits ||
					reservation.UsageReceiptID == nil || *reservation.UsageReceiptID != settle.UsageReceiptID) {
				return conflict("Cost settlement facts differ from the terminal reservation")
			}
			return storeCostReservationReceipt(
				ctx, repo, service.config.NewID, actor, operation, key, inputHash, reservation, entry, now, &result,
			)
		}
		if reservation.Status != domain.ReservationReserved {
			return conflict("Cost reservation is already terminal")
		}
		previousRevision := reservation.Revision
		if desiredStatus == domain.ReservationSettled {
			if settle.SettledUnits > reservation.EstimatedUnits {
				return conflict("Settled units exceed the cost reservation")
			}
			usageReceiptID := settle.UsageReceiptID
			reservation.SettledUnits = settle.SettledUnits
			reservation.SettledAmount = reservation.UnitAmount.Mul(decimal.NewFromInt(settle.SettledUnits))
			reservation.UsageReceiptID = &usageReceiptID
			reservation.Status, reservation.SettledAt = domain.ReservationSettled, costTimePointer(now)
		} else {
			reservation.Status, reservation.ReleasedAt = domain.ReservationReleased, costTimePointer(now)
		}
		reservation.Revision, reservation.UpdatedBy, reservation.UpdatedAt = previousRevision+1, actor.UserID, now
		reservation.ContentHash, listErr = reservationContentHash(reservation)
		if listErr != nil {
			return listErr
		}
		updated, updateErr := repo.UpdateReservation(ctx, reservation, previousRevision)
		if updateErr != nil {
			return updateErr
		}
		entryType := domain.LedgerReservationSettled
		if desiredStatus == domain.ReservationReleased {
			entryType = domain.LedgerReservationReleased
		}
		entry, entryErr := service.appendLedgerEntry(ctx, repo, actor, updated, entryType, now)
		if entryErr != nil {
			return entryErr
		}
		return storeCostReservationReceipt(
			ctx, repo, service.config.NewID, actor, operation, key, inputHash, updated, entry, now, &result,
		)
	})
	return result, normalizeError(err)
}

func (service *Service) GetReservation(
	ctx context.Context,
	actor Actor,
	reservationID string,
) (ReservationView, error) {
	actor.UserID, reservationID = strings.TrimSpace(actor.UserID), strings.TrimSpace(reservationID)
	if service == nil || service.transactions == nil || !validActor(actor) || !validUUID(reservationID) {
		return ReservationView{}, invalid("Invalid cost reservation query")
	}
	var result ReservationView
	err := service.transactions.WithinCostTransaction(ctx, func(repo Repository) error {
		snapshot, findErr := repo.FindReservation(ctx, reservationID)
		if findErr != nil {
			return findErr
		}
		scope, authorizeErr := repo.AuthorizeProject(ctx, actor, snapshot.ProjectID, "read")
		if authorizeErr != nil {
			return authorizeErr
		}
		if snapshot.WorkspaceID != scope.WorkspaceID {
			return conflict("Cost reservation scope has drifted")
		}
		budget, budgetErr := repo.GetBudgetForUpdate(ctx, scope.ProjectID)
		if budgetErr != nil {
			return budgetErr
		}
		if validationErr := validateBudget(budget); validationErr != nil {
			return validationErr
		}
		reservation, findErr := repo.GetReservationForUpdate(ctx, reservationID)
		if findErr != nil {
			return findErr
		}
		if !domain.SameReservationBinding(snapshot, reservation) || reservation.BudgetPolicyID != budget.ID ||
			reservation.WorkspaceID != budget.WorkspaceID || reservation.Currency != budget.Currency {
			return conflict("Cost reservation binding has drifted")
		}
		entries, listErr := repo.ListLedgerEntries(ctx, reservation.ID)
		if listErr != nil {
			return listErr
		}
		if validationErr := validateReservationLifecycle(reservation, entries); validationErr != nil {
			return validationErr
		}
		result = ReservationView{Reservation: reservation, LedgerEntries: entries}
		return nil
	})
	return result, normalizeError(err)
}

func reuseReservedEstimate(
	ctx context.Context,
	repo Repository,
	newID func() string,
	actor Actor,
	key, inputHash string,
	estimate domain.Estimate,
	reservation domain.Reservation,
	now time.Time,
	result *ReservationResult,
) error {
	if validationErr := validateReservationEstimate(reservation, estimate); validationErr != nil {
		return validationErr
	}
	entries, err := repo.ListLedgerEntries(ctx, reservation.ID)
	if err != nil {
		return err
	}
	if validationErr := validateReservationLifecycle(reservation, entries); validationErr != nil {
		return validationErr
	}
	entry, err := ledgerByType(entries, domain.LedgerReservationCreated)
	if err != nil {
		return err
	}
	return storeCostReservationReceipt(
		ctx, repo, newID, actor, reserveEstimateOperation, key, inputHash, reservation, entry, now, result,
	)
}

func (service *Service) appendLedgerEntry(
	ctx context.Context,
	repo Repository,
	actor Actor,
	reservation domain.Reservation,
	entryType string,
	now time.Time,
) (domain.LedgerEntry, error) {
	entryID := strings.TrimSpace(service.config.NewID())
	if !validUUID(entryID) {
		return domain.LedgerEntry{}, errors.New("cost ledger entry identifier is invalid")
	}
	entry := domain.LedgerEntry{
		ID: entryID, WorkspaceID: reservation.WorkspaceID, ProjectID: reservation.ProjectID,
		ReservationID: reservation.ID, EstimateID: reservation.EstimateID, EntryType: entryType,
		Currency: reservation.Currency, CreatedBy: actor.UserID, CreatedAt: now,
	}
	switch entryType {
	case domain.LedgerReservationCreated:
		entry.Sequence, entry.ReservedDelta, entry.SettledDelta = 1, reservation.ReservedAmount, decimal.Zero
	case domain.LedgerReservationSettled:
		entry.Sequence, entry.ReservedDelta, entry.SettledDelta = 2, reservation.ReservedAmount.Neg(), reservation.SettledAmount
		entry.UsageReceiptID = reservation.UsageReceiptID
	case domain.LedgerReservationReleased:
		entry.Sequence, entry.ReservedDelta, entry.SettledDelta = 2, reservation.ReservedAmount.Neg(), decimal.Zero
	default:
		return domain.LedgerEntry{}, errors.New("cost ledger entry type is invalid")
	}
	var err error
	entry.ContentHash, err = ledgerContentHash(entry)
	if err != nil {
		return domain.LedgerEntry{}, err
	}
	persisted, err := repo.EnsureLedgerEntry(ctx, entry)
	if err != nil {
		return domain.LedgerEntry{}, err
	}
	if !domain.SameLedgerEntryState(persisted, entry) {
		return domain.LedgerEntry{}, platformcommand.ErrInputMismatch
	}
	return persisted, nil
}

func reconcileBudgetUsage(
	budget domain.BudgetPolicy,
	reservations []domain.Reservation,
) (decimal.Decimal, decimal.Decimal, error) {
	reserved, settled := decimal.Zero, decimal.Zero
	for _, reservation := range reservations {
		if validationErr := validateReservation(reservation); validationErr != nil {
			return decimal.Zero, decimal.Zero, validationErr
		}
		if reservation.WorkspaceID != budget.WorkspaceID || reservation.ProjectID != budget.ProjectID ||
			reservation.BudgetPolicyID != budget.ID || reservation.Currency != budget.Currency {
			return decimal.Zero, decimal.Zero, conflict("Cost reservation budget binding has drifted")
		}
		switch reservation.Status {
		case domain.ReservationReserved:
			reserved = reserved.Add(reservation.ReservedAmount)
		case domain.ReservationSettled:
			settled = settled.Add(reservation.SettledAmount)
		case domain.ReservationReleased:
		default:
			return decimal.Zero, decimal.Zero, conflict("Cost reservation status has drifted")
		}
	}
	if reserved.IsNegative() || settled.IsNegative() || reserved.Add(settled).GreaterThan(budget.LimitAmount) {
		return decimal.Zero, decimal.Zero, conflict("Project cost usage exceeds its budget facts")
	}
	return reserved, settled, nil
}

func validateReservationEstimate(reservation domain.Reservation, estimate domain.Estimate) error {
	if validateReservation(reservation) != nil || reservation.EstimateID != estimate.ID ||
		reservation.WorkspaceID != estimate.WorkspaceID || reservation.ProjectID != estimate.ProjectID ||
		reservation.BudgetPolicyID != estimate.BudgetPolicyID || reservation.PriceQuoteID != estimate.PriceQuoteID ||
		reservation.Metric != estimate.Metric || reservation.SourceType != estimate.SourceType ||
		reservation.SourceID != estimate.SourceID || reservation.EstimatedUnits != estimate.Units ||
		!reservation.UnitAmount.Equal(estimate.UnitAmount) || !reservation.ReservedAmount.Equal(estimate.TotalAmount) ||
		reservation.Currency != estimate.Currency || reservation.PriceQuoteRevision != estimate.PriceQuoteRevision ||
		reservation.BudgetPolicyRevision < estimate.BudgetPolicyRevision {
		return conflict("Cost reservation and estimate facts have drifted")
	}
	return nil
}

func validateReservation(value domain.Reservation) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) || !validUUID(value.ProjectID) ||
		!validUUID(value.EstimateID) || !validUUID(value.BudgetPolicyID) || !validUUID(value.PriceQuoteID) ||
		!domain.IsBillingMetric(value.Metric) || value.SourceType != domain.SourceGenerationIntent ||
		!validUUID(value.SourceID) || value.EstimatedUnits < 1 || value.SettledUnits < 0 ||
		value.SettledUnits > value.EstimatedUnits || !value.UnitAmount.IsPositive() || !value.ReservedAmount.IsPositive() ||
		value.SettledAmount.IsNegative() || value.BudgetLimit.IsNegative() ||
		value.UnitAmount.GreaterThan(maximumAmount) || value.ReservedAmount.GreaterThan(maximumAmount) ||
		value.SettledAmount.GreaterThan(maximumAmount) || value.BudgetLimit.GreaterThan(maximumAmount) ||
		!value.UnitAmount.Round(6).Equal(value.UnitAmount) || !value.ReservedAmount.Round(6).Equal(value.ReservedAmount) ||
		!value.SettledAmount.Round(6).Equal(value.SettledAmount) || !value.BudgetLimit.Round(6).Equal(value.BudgetLimit) ||
		!value.UnitAmount.Mul(decimal.NewFromInt(value.EstimatedUnits)).Equal(value.ReservedAmount) ||
		!value.UnitAmount.Mul(decimal.NewFromInt(value.SettledUnits)).Equal(value.SettledAmount) ||
		value.ReservedAmount.GreaterThan(value.BudgetLimit) || !currencyPattern.MatchString(value.Currency) ||
		value.PriceQuoteRevision < 1 || value.BudgetPolicyRevision < 1 || value.Revision < 1 ||
		len(value.ContentHash) != 64 || !validUUID(value.CreatedBy) || !validUUID(value.UpdatedBy) ||
		value.CreatedAt.IsZero() || value.UpdatedAt.Before(value.CreatedAt) {
		return conflict("Cost reservation facts have drifted")
	}
	switch value.Status {
	case domain.ReservationReserved:
		if value.Revision != 1 || value.SettledUnits != 0 || !value.SettledAmount.IsZero() || value.UsageReceiptID != nil ||
			value.SettledAt != nil || value.ReleasedAt != nil || !value.UpdatedAt.Equal(value.CreatedAt) {
			return conflict("Cost reservation terminal facts have drifted")
		}
	case domain.ReservationSettled:
		if value.Revision != 2 || value.SettledUnits < 1 || value.UsageReceiptID == nil ||
			!validUUID(*value.UsageReceiptID) || value.SettledAt == nil || value.ReleasedAt != nil ||
			!value.SettledAt.Equal(value.UpdatedAt) {
			return conflict("Cost reservation terminal facts have drifted")
		}
	case domain.ReservationReleased:
		if value.Revision != 2 || value.SettledUnits != 0 || !value.SettledAmount.IsZero() || value.UsageReceiptID != nil ||
			value.SettledAt != nil || value.ReleasedAt == nil || !value.ReleasedAt.Equal(value.UpdatedAt) {
			return conflict("Cost reservation terminal facts have drifted")
		}
	default:
		return conflict("Cost reservation status has drifted")
	}
	hash, err := reservationContentHash(value)
	if err != nil || hash != value.ContentHash {
		return conflict("Cost reservation facts have drifted")
	}
	return nil
}

func validateLedgerEntry(value domain.LedgerEntry) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) || !validUUID(value.ProjectID) ||
		!validUUID(value.ReservationID) || !validUUID(value.EstimateID) ||
		value.ReservedDelta.Abs().GreaterThan(maximumAmount) || value.SettledDelta.IsNegative() ||
		value.SettledDelta.GreaterThan(maximumAmount) || !value.ReservedDelta.Round(6).Equal(value.ReservedDelta) ||
		!value.SettledDelta.Round(6).Equal(value.SettledDelta) || !currencyPattern.MatchString(value.Currency) ||
		len(value.ContentHash) != 64 || !validUUID(value.CreatedBy) || value.CreatedAt.IsZero() {
		return conflict("Cost ledger entry facts have drifted")
	}
	switch value.EntryType {
	case domain.LedgerReservationCreated:
		if value.Sequence != 1 || !value.ReservedDelta.IsPositive() || !value.SettledDelta.IsZero() || value.UsageReceiptID != nil {
			return conflict("Cost ledger entry delta has drifted")
		}
	case domain.LedgerReservationSettled:
		if value.Sequence != 2 || !value.ReservedDelta.IsNegative() || !value.SettledDelta.IsPositive() ||
			value.UsageReceiptID == nil || !validUUID(*value.UsageReceiptID) {
			return conflict("Cost ledger entry delta has drifted")
		}
	case domain.LedgerReservationReleased:
		if value.Sequence != 2 || !value.ReservedDelta.IsNegative() || !value.SettledDelta.IsZero() || value.UsageReceiptID != nil {
			return conflict("Cost ledger entry delta has drifted")
		}
	default:
		return conflict("Cost ledger entry type has drifted")
	}
	hash, err := ledgerContentHash(value)
	if err != nil || hash != value.ContentHash {
		return conflict("Cost ledger entry facts have drifted")
	}
	return nil
}

func validateReservationLifecycle(reservation domain.Reservation, entries []domain.LedgerEntry) error {
	if err := validateReservation(reservation); err != nil {
		return err
	}
	expectedEntries := 1
	if reservation.Status != domain.ReservationReserved {
		expectedEntries = 2
	}
	if len(entries) != expectedEntries {
		return conflict("Cost ledger sequence has drifted")
	}
	created, err := ledgerByType(entries, domain.LedgerReservationCreated)
	if err != nil || created.Sequence != 1 || !created.ReservedDelta.Equal(reservation.ReservedAmount) ||
		!created.SettledDelta.IsZero() || created.CreatedBy != reservation.CreatedBy ||
		!created.CreatedAt.Equal(reservation.CreatedAt) {
		return conflict("Cost reservation creation ledger has drifted")
	}
	if err = validateLedgerBinding(reservation, created); err != nil {
		return err
	}
	if reservation.Status == domain.ReservationSettled {
		settled, findErr := ledgerByType(entries, domain.LedgerReservationSettled)
		if findErr != nil || !settled.ReservedDelta.Equal(reservation.ReservedAmount.Neg()) ||
			!settled.SettledDelta.Equal(reservation.SettledAmount) ||
			settled.UsageReceiptID == nil || reservation.UsageReceiptID == nil ||
			*settled.UsageReceiptID != *reservation.UsageReceiptID || settled.CreatedBy != reservation.UpdatedBy ||
			!settled.CreatedAt.Equal(reservation.UpdatedAt) {
			return conflict("Cost settlement ledger has drifted")
		}
		return validateLedgerBinding(reservation, settled)
	}
	if reservation.Status == domain.ReservationReleased {
		released, findErr := ledgerByType(entries, domain.LedgerReservationReleased)
		if findErr != nil || !released.ReservedDelta.Equal(reservation.ReservedAmount.Neg()) ||
			!released.SettledDelta.IsZero() || released.CreatedBy != reservation.UpdatedBy ||
			!released.CreatedAt.Equal(reservation.UpdatedAt) {
			return conflict("Cost release ledger has drifted")
		}
		return validateLedgerBinding(reservation, released)
	}
	return nil
}

func validateLedgerBinding(reservation domain.Reservation, entry domain.LedgerEntry) error {
	if err := validateLedgerEntry(entry); err != nil {
		return err
	}
	if entry.WorkspaceID != reservation.WorkspaceID || entry.ProjectID != reservation.ProjectID ||
		entry.ReservationID != reservation.ID || entry.EstimateID != reservation.EstimateID ||
		entry.Currency != reservation.Currency {
		return conflict("Cost ledger binding has drifted")
	}
	return nil
}

func ledgerByType(entries []domain.LedgerEntry, entryType string) (domain.LedgerEntry, error) {
	for _, entry := range entries {
		if entry.EntryType == entryType {
			return entry, nil
		}
	}
	return domain.LedgerEntry{}, ErrLedgerEntryNotFound
}

func reservationContentHash(value domain.Reservation) (string, error) {
	return platformcommand.InputHash(reservationHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, EstimateID: value.EstimateID,
		BudgetPolicyID: value.BudgetPolicyID, PriceQuoteID: value.PriceQuoteID,
		Metric: value.Metric, SourceType: value.SourceType, SourceID: value.SourceID,
		EstimatedUnits: value.EstimatedUnits, SettledUnits: value.SettledUnits,
		UnitAmount: value.UnitAmount.StringFixed(6), ReservedAmount: value.ReservedAmount.StringFixed(6),
		SettledAmount: value.SettledAmount.StringFixed(6), BudgetLimit: value.BudgetLimit.StringFixed(6),
		Currency: value.Currency, PriceQuoteRevision: value.PriceQuoteRevision,
		BudgetPolicyRevision: value.BudgetPolicyRevision, Status: value.Status,
		UsageReceiptID: optionalString(value.UsageReceiptID), Revision: value.Revision, UpdatedBy: value.UpdatedBy,
	})
}

func ledgerContentHash(value domain.LedgerEntry) (string, error) {
	return platformcommand.InputHash(ledgerHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID,
		ReservationID: value.ReservationID, EstimateID: value.EstimateID,
		EntryType: value.EntryType, Sequence: value.Sequence,
		ReservedDelta: value.ReservedDelta.StringFixed(6), SettledDelta: value.SettledDelta.StringFixed(6),
		Currency: value.Currency, UsageReceiptID: optionalString(value.UsageReceiptID), CreatedBy: value.CreatedBy,
	})
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func costTimePointer(value time.Time) *time.Time { return &value }

func storeCostReservationReceipt(
	ctx context.Context,
	repo Repository,
	newID func() string,
	actor Actor,
	operation, key, inputHash string,
	reservation domain.Reservation,
	entry domain.LedgerEntry,
	now time.Time,
	result *ReservationResult,
) error {
	encoded, err := platformcommand.Result(reservationReceipt{Reservation: reservation, LedgerEntry: entry})
	if err != nil {
		return err
	}
	receiptID := strings.TrimSpace(newID())
	if !validUUID(receiptID) {
		return errors.New("cost reservation receipt identifier is invalid")
	}
	receipt, err := repo.EnsureReceipt(ctx, platformcommand.Receipt{
		ID: receiptID, WorkspaceID: reservation.WorkspaceID, Operation: operation,
		IdempotencyKey: key, InputHash: inputHash, ResourceID: reservation.ID,
		Result: encoded, CreatedBy: actor.UserID, CreatedAt: now,
	})
	if err != nil {
		return err
	}
	*result = ReservationResult{Reservation: reservation, LedgerEntry: entry, Receipt: receipt}
	return nil
}

func replayCostReservation(
	ctx context.Context,
	repo Repository,
	receipt platformcommand.Receipt,
	inputHash string,
	result *ReservationResult,
) error {
	replayed, err := platformcommand.Replay[reservationReceipt](receipt, inputHash)
	if err != nil {
		return err
	}
	if receipt.ResourceID != replayed.Reservation.ID || validateReservation(replayed.Reservation) != nil ||
		validateLedgerBinding(replayed.Reservation, replayed.LedgerEntry) != nil {
		return platformcommand.ErrInputMismatch
	}
	current, err := repo.GetReservationForUpdate(ctx, replayed.Reservation.ID)
	if err != nil {
		return platformcommand.ErrInputMismatch
	}
	entries, err := repo.ListLedgerEntries(ctx, current.ID)
	if err != nil || validateReservationLifecycle(current, entries) != nil ||
		!domain.SameReservationBinding(current, replayed.Reservation) || current.Revision < replayed.Reservation.Revision {
		return platformcommand.ErrInputMismatch
	}
	persistedEntry, err := ledgerByType(entries, replayed.LedgerEntry.EntryType)
	if err != nil || persistedEntry.ID != replayed.LedgerEntry.ID ||
		!domain.SameLedgerEntryState(persistedEntry, replayed.LedgerEntry) {
		return platformcommand.ErrInputMismatch
	}
	*result = ReservationResult{
		Reservation: replayed.Reservation, LedgerEntry: replayed.LedgerEntry, Receipt: receipt,
	}
	return nil
}

func budgetExceeded() error {
	return &Error{
		Code: "budget_exceeded", Message: "Project cost budget is exhausted", Status: 409,
		NextAction: "increase_budget_or_reduce_scope",
	}
}
