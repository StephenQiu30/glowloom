package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/quota/domain"
)

const (
	setPolicyOperation = "quota.policy.set_daily"
	reserveOperation   = "quota.reservation.reserve"
	consumeOperation   = "quota.reservation.consume"
	releaseOperation   = "quota.reservation.release"
)

var (
	ErrPolicyNotFound      = errors.New("quota policy not found")
	ErrCounterNotFound     = errors.New("quota counter not found")
	ErrReservationNotFound = errors.New("quota reservation not found")
)

type Error struct {
	Code, Message, NextAction string
	Status                    int
}

func (value *Error) Error() string { return value.Message }

func IsCode(err error, code string) bool {
	var typed *Error
	return errors.As(err, &typed) && typed.Code == code
}

type Actor struct {
	UserID       string
	TokenVersion int
}

type Repository interface {
	AuthorizeProject(context.Context, Actor, string, string, string) error
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	EnsureReceipt(context.Context, platformcommand.Receipt) (platformcommand.Receipt, error)
	FindPolicy(context.Context, string, string) (domain.Policy, error)
	GetPolicyForUpdate(context.Context, string, string) (domain.Policy, error)
	EnsurePolicy(context.Context, domain.Policy) (domain.Policy, error)
	UpdatePolicy(context.Context, domain.Policy, int64) (domain.Policy, error)
	FindCounter(context.Context, string, time.Time) (domain.Counter, error)
	GetCounterForUpdate(context.Context, string, time.Time) (domain.Counter, error)
	EnsureCounter(context.Context, domain.Counter) (domain.Counter, error)
	UpdateCounter(context.Context, domain.Counter, int64) (domain.Counter, error)
	FindReservationBySource(context.Context, string, time.Time, string, string) (domain.Reservation, error)
	GetReservation(context.Context, string) (domain.Reservation, error)
	GetReservationForUpdate(context.Context, string) (domain.Reservation, error)
	CreateReservation(context.Context, domain.Reservation) (domain.Reservation, error)
	UpdateReservation(context.Context, domain.Reservation, int64) (domain.Reservation, error)
	ListReservations(context.Context, string) ([]domain.Reservation, error)
}

type TransactionManager interface {
	WithinQuotaTransaction(context.Context, func(Repository) error) error
}

type Config struct {
	Now   func() time.Time
	NewID func() string
}

type Service struct {
	transactions TransactionManager
	config       Config
}

type SetDailyPolicyCommand struct {
	WorkspaceID, ProjectID, Metric, IdempotencyKey string
	LimitUnits, ExpectedRevision                   int64
}

type ReserveCommand struct {
	WorkspaceID, ProjectID, Metric string
	SourceType, SourceID           string
	Units                          int64
	IdempotencyKey                 string
}

type TransitionCommand struct {
	ReservationID, IdempotencyKey string
}

type PolicyResult struct {
	Policy  domain.Policy
	Receipt platformcommand.Receipt
}

type ReservationResult struct {
	Reservation domain.Reservation
	Receipt     platformcommand.Receipt
}

type policyReceipt struct {
	Policy domain.Policy `json:"policy"`
}

type reservationReceipt struct {
	Reservation domain.Reservation `json:"reservation"`
}

type policyHashInput struct {
	WorkspaceID, ProjectID, Metric, WindowKind string
	LimitUnits, Revision                       int64
}

type reservationHashInput struct {
	WorkspaceID, ProjectID, PolicyID, CounterID string
	Metric, SourceType, SourceID                string
	WindowStart, WindowEnd                      time.Time
	PolicyRevision, LimitUnits, Units           int64
}

func NewService(transactions TransactionManager, config Config) *Service {
	return &Service{transactions: transactions, config: config}
}

func (service *Service) SetDailyPolicy(
	ctx context.Context,
	actor Actor,
	command SetDailyPolicyCommand,
) (PolicyResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.ProjectID = strings.TrimSpace(command.ProjectID)
	command.Metric = strings.TrimSpace(command.Metric)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.transactions == nil || service.config.Now == nil || service.config.NewID == nil ||
		!validActor(actor) || !validUUID(command.WorkspaceID) || !validUUID(command.ProjectID) ||
		!domain.IsGenerationMetric(command.Metric) || command.LimitUnits < 1 || command.ExpectedRevision < 0 ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return PolicyResult{}, invalid("Invalid daily quota policy request")
	}
	inputHash, err := platformcommand.InputHash(struct {
		ActorID string
		Command SetDailyPolicyCommand
	}{ActorID: actor.UserID, Command: command})
	if err != nil {
		return PolicyResult{}, err
	}
	now := service.config.Now().UTC()
	windowStart, _ := domain.DailyWindow(now)
	var result PolicyResult
	err = service.transactions.WithinQuotaTransaction(ctx, func(repo Repository) error {
		if authorizeErr := repo.AuthorizeProject(ctx, actor, command.WorkspaceID, command.ProjectID, "owner"); authorizeErr != nil {
			return authorizeErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, command.WorkspaceID, setPolicyOperation, command.IdempotencyKey); findErr == nil {
			return replayPolicy(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		current, findErr := repo.GetPolicyForUpdate(ctx, command.ProjectID, command.Metric)
		if errors.Is(findErr, ErrPolicyNotFound) {
			if command.ExpectedRevision != 0 {
				return conflict("Daily quota policy revision has changed")
			}
			policyID := strings.TrimSpace(service.config.NewID())
			if !validUUID(policyID) {
				return errors.New("quota policy identifier is invalid")
			}
			desired := domain.Policy{
				ID: policyID, WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
				Metric: command.Metric, WindowKind: domain.WindowUTCDay, LimitUnits: command.LimitUnits, Revision: 1,
				CreatedBy: actor.UserID, UpdatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now,
			}
			desired.ContentHash, findErr = policyContentHash(desired)
			if findErr != nil {
				return findErr
			}
			current, findErr = repo.EnsurePolicy(ctx, desired)
			if findErr != nil {
				return findErr
			}
			if !domain.SamePolicyState(current, desired) {
				return platformcommand.ErrInputMismatch
			}
		} else if findErr != nil {
			return findErr
		} else {
			if validationErr := validatePolicy(current); validationErr != nil {
				return validationErr
			}
			if current.WorkspaceID != command.WorkspaceID || current.Revision != command.ExpectedRevision {
				return conflict("Daily quota policy revision has changed")
			}
			if current.LimitUnits != command.LimitUnits {
				counter, counterErr := repo.GetCounterForUpdate(ctx, current.ID, windowStart)
				if counterErr == nil {
					if validationErr := validateCurrentCounter(current, counter); validationErr != nil {
						return validationErr
					}
					reservations, listErr := repo.ListReservations(ctx, counter.ID)
					if listErr != nil {
						return listErr
					}
					if _, reconcileErr := reconcileUsage(current, counter, reservations); reconcileErr != nil {
						return reconcileErr
					}
					if counter.ReservedUnits+counter.ConsumedUnits > command.LimitUnits {
						return conflict("Daily quota limit is below current usage")
					}
				} else if !errors.Is(counterErr, ErrCounterNotFound) {
					return counterErr
				}
				desired := current
				desired.LimitUnits, desired.Revision = command.LimitUnits, current.Revision+1
				desired.UpdatedBy, desired.UpdatedAt = actor.UserID, now
				desired.ContentHash, findErr = policyContentHash(desired)
				if findErr != nil {
					return findErr
				}
				updated, updateErr := repo.UpdatePolicy(ctx, desired, current.Revision)
				if updateErr != nil {
					return updateErr
				}
				if counterErr == nil {
					previousCounterRevision := counter.Revision
					counter.PolicyRevision, counter.LimitUnits = updated.Revision, updated.LimitUnits
					counter.Revision, counter.UpdatedAt = counter.Revision+1, now
					if _, updateErr = repo.UpdateCounter(ctx, counter, previousCounterRevision); updateErr != nil {
						return updateErr
					}
				}
				current = updated
			}
		}
		return storePolicyReceipt(ctx, repo, service.config.NewID, actor, command.IdempotencyKey, inputHash, current, now, &result)
	})
	return result, normalizeError(err)
}

func (service *Service) Reserve(
	ctx context.Context,
	actor Actor,
	command ReserveCommand,
) (ReservationResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.ProjectID = strings.TrimSpace(command.ProjectID)
	command.Metric = strings.TrimSpace(command.Metric)
	command.SourceType = strings.TrimSpace(command.SourceType)
	command.SourceID = strings.TrimSpace(command.SourceID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.transactions == nil || service.config.Now == nil || service.config.NewID == nil ||
		!validActor(actor) || !validUUID(command.WorkspaceID) || !validUUID(command.ProjectID) ||
		!domain.IsGenerationMetric(command.Metric) || command.SourceType != "generation_intent" ||
		!validUUID(command.SourceID) || command.Units < 1 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return ReservationResult{}, invalid("Invalid quota reservation request")
	}
	inputHash, err := platformcommand.InputHash(struct {
		ActorID string
		Command ReserveCommand
	}{ActorID: actor.UserID, Command: command})
	if err != nil {
		return ReservationResult{}, err
	}
	now := service.config.Now().UTC()
	windowStart, windowEnd := domain.DailyWindow(now)
	var result ReservationResult
	err = service.transactions.WithinQuotaTransaction(ctx, func(repo Repository) error {
		if authorizeErr := repo.AuthorizeProject(ctx, actor, command.WorkspaceID, command.ProjectID, "write"); authorizeErr != nil {
			return authorizeErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, command.WorkspaceID, reserveOperation, command.IdempotencyKey); findErr == nil {
			return replayReservation(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		policy, loadErr := repo.GetPolicyForUpdate(ctx, command.ProjectID, command.Metric)
		if loadErr != nil {
			return loadErr
		}
		if loadErr = validatePolicy(policy); loadErr != nil || policy.WorkspaceID != command.WorkspaceID {
			if loadErr != nil {
				return loadErr
			}
			return conflict("Daily quota policy scope has drifted")
		}
		counterID := strings.TrimSpace(service.config.NewID())
		if !validUUID(counterID) {
			return errors.New("quota counter identifier is invalid")
		}
		counter, loadErr := repo.EnsureCounter(ctx, domain.Counter{
			ID: counterID, WorkspaceID: policy.WorkspaceID, ProjectID: policy.ProjectID, PolicyID: policy.ID,
			Metric: policy.Metric, WindowStart: windowStart, WindowEnd: windowEnd,
			PolicyRevision: policy.Revision, LimitUnits: policy.LimitUnits, ReservedUnits: 0, ConsumedUnits: 0,
			Revision: 1, CreatedAt: now, UpdatedAt: now,
		})
		if loadErr != nil {
			return fmt.Errorf("ensure daily quota counter: %w", loadErr)
		}
		if loadErr = validateCurrentCounter(policy, counter); loadErr != nil {
			return loadErr
		}
		reservations, loadErr := repo.ListReservations(ctx, counter.ID)
		if loadErr != nil {
			return loadErr
		}
		if _, loadErr = reconcileUsage(policy, counter, reservations); loadErr != nil {
			return loadErr
		}
		existing, findErr := repo.FindReservationBySource(ctx, policy.ID, windowStart, command.SourceType, command.SourceID)
		if findErr == nil {
			if existing.WorkspaceID != command.WorkspaceID || existing.ProjectID != command.ProjectID ||
				existing.Metric != command.Metric || existing.CounterID != counter.ID || existing.Units != command.Units ||
				validationReservation(existing) != nil {
				return platformcommand.ErrInputMismatch
			}
			return storeReservationReceipt(ctx, repo, service.config.NewID, actor, reserveOperation, command.IdempotencyKey, inputHash, existing, now, &result)
		}
		if !errors.Is(findErr, ErrReservationNotFound) {
			return findErr
		}
		if counter.ReservedUnits+counter.ConsumedUnits > counter.LimitUnits-command.Units {
			return exceeded()
		}
		reservationID := strings.TrimSpace(service.config.NewID())
		if !validUUID(reservationID) {
			return errors.New("quota reservation identifier is invalid")
		}
		desired := domain.Reservation{
			ID: reservationID, WorkspaceID: policy.WorkspaceID, ProjectID: policy.ProjectID,
			PolicyID: policy.ID, CounterID: counter.ID, Metric: policy.Metric,
			SourceType: command.SourceType, SourceID: command.SourceID, WindowStart: windowStart, WindowEnd: windowEnd,
			PolicyRevision: policy.Revision, LimitUnits: policy.LimitUnits, Units: command.Units,
			Status: domain.ReservationReserved, Revision: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now,
		}
		desired.BindingHash, loadErr = reservationBindingHash(desired)
		if loadErr != nil {
			return loadErr
		}
		persisted, createErr := repo.CreateReservation(ctx, desired)
		if createErr != nil {
			return fmt.Errorf("create quota reservation: %w", createErr)
		}
		previousCounterRevision := counter.Revision
		counter.ReservedUnits += command.Units
		counter.Revision, counter.UpdatedAt = counter.Revision+1, now
		if _, updateErr := repo.UpdateCounter(ctx, counter, previousCounterRevision); updateErr != nil {
			return fmt.Errorf("increment quota counter: %w", updateErr)
		}
		return storeReservationReceipt(ctx, repo, service.config.NewID, actor, reserveOperation, command.IdempotencyKey, inputHash, persisted, now, &result)
	})
	return result, normalizeError(err)
}

func (service *Service) Consume(ctx context.Context, actor Actor, command TransitionCommand) (ReservationResult, error) {
	return service.transition(ctx, actor, command, domain.ReservationConsumed, consumeOperation)
}

func (service *Service) Release(ctx context.Context, actor Actor, command TransitionCommand) (ReservationResult, error) {
	return service.transition(ctx, actor, command, domain.ReservationReleased, releaseOperation)
}

func (service *Service) GetReservation(ctx context.Context, actor Actor, reservationID string) (domain.Reservation, error) {
	actor.UserID, reservationID = strings.TrimSpace(actor.UserID), strings.TrimSpace(reservationID)
	if service == nil || service.transactions == nil || !validActor(actor) || !validUUID(reservationID) {
		return domain.Reservation{}, invalid("Invalid quota reservation query")
	}
	var result domain.Reservation
	err := service.transactions.WithinQuotaTransaction(ctx, func(repo Repository) error {
		snapshot, loadErr := repo.GetReservation(ctx, reservationID)
		if loadErr != nil {
			return loadErr
		}
		if authorizeErr := repo.AuthorizeProject(
			ctx, actor, snapshot.WorkspaceID, snapshot.ProjectID, "read",
		); authorizeErr != nil {
			return authorizeErr
		}
		policy, loadErr := repo.GetPolicyForUpdate(ctx, snapshot.ProjectID, snapshot.Metric)
		if loadErr != nil {
			return loadErr
		}
		if loadErr = validatePolicy(policy); loadErr != nil || policy.ID != snapshot.PolicyID ||
			policy.WorkspaceID != snapshot.WorkspaceID {
			if loadErr != nil {
				return loadErr
			}
			return conflict("Quota reservation policy binding has drifted")
		}
		counter, loadErr := repo.GetCounterForUpdate(ctx, snapshot.PolicyID, snapshot.WindowStart)
		if loadErr != nil {
			return loadErr
		}
		reservation, loadErr := repo.GetReservationForUpdate(ctx, reservationID)
		if loadErr != nil {
			return loadErr
		}
		if snapshot.BindingHash != reservation.BindingHash {
			return conflict("Quota reservation facts have drifted")
		}
		reservations, loadErr := repo.ListReservations(ctx, counter.ID)
		if loadErr != nil {
			return loadErr
		}
		if _, loadErr = reconcileUsage(policy, counter, reservations); loadErr != nil {
			return loadErr
		}
		if validationReservation(reservation) != nil || reservation.PolicyID != policy.ID ||
			reservation.CounterID != counter.ID {
			return conflict("Quota reservation facts have drifted")
		}
		result = reservation
		return nil
	})
	return result, normalizeError(err)
}

func (service *Service) transition(
	ctx context.Context,
	actor Actor,
	command TransitionCommand,
	desiredStatus string,
	operation string,
) (ReservationResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.ReservationID = strings.TrimSpace(command.ReservationID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.transactions == nil || service.config.Now == nil || service.config.NewID == nil ||
		!validActor(actor) || !validUUID(command.ReservationID) || command.IdempotencyKey == "" ||
		len(command.IdempotencyKey) > 200 {
		return ReservationResult{}, invalid("Invalid quota reservation transition")
	}
	inputHash, err := platformcommand.InputHash(struct {
		ActorID string
		Command TransitionCommand
	}{ActorID: actor.UserID, Command: command})
	if err != nil {
		return ReservationResult{}, err
	}
	now := service.config.Now().UTC()
	var result ReservationResult
	err = service.transactions.WithinQuotaTransaction(ctx, func(repo Repository) error {
		snapshot, loadErr := repo.GetReservation(ctx, command.ReservationID)
		if loadErr != nil {
			return loadErr
		}
		if authorizeErr := repo.AuthorizeProject(ctx, actor, snapshot.WorkspaceID, snapshot.ProjectID, "write"); authorizeErr != nil {
			return authorizeErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, snapshot.WorkspaceID, operation, command.IdempotencyKey); findErr == nil {
			return replayReservation(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		policy, loadErr := repo.GetPolicyForUpdate(ctx, snapshot.ProjectID, snapshot.Metric)
		if loadErr != nil {
			return loadErr
		}
		if loadErr = validatePolicy(policy); loadErr != nil || policy.ID != snapshot.PolicyID ||
			policy.WorkspaceID != snapshot.WorkspaceID {
			if loadErr != nil {
				return loadErr
			}
			return conflict("Quota reservation policy binding has drifted")
		}
		counter, loadErr := repo.GetCounterForUpdate(ctx, snapshot.PolicyID, snapshot.WindowStart)
		if loadErr != nil {
			return loadErr
		}
		reservation, loadErr := repo.GetReservationForUpdate(ctx, command.ReservationID)
		if loadErr != nil {
			return loadErr
		}
		if snapshot.BindingHash != reservation.BindingHash || validationReservation(reservation) != nil ||
			reservation.CounterID != counter.ID {
			return conflict("Quota reservation facts have drifted")
		}
		reservations, loadErr := repo.ListReservations(ctx, counter.ID)
		if loadErr != nil {
			return loadErr
		}
		if _, loadErr = reconcileUsage(policy, counter, reservations); loadErr != nil {
			return loadErr
		}
		if reservation.Status == desiredStatus {
			return storeReservationReceipt(ctx, repo, service.config.NewID, actor, operation, command.IdempotencyKey, inputHash, reservation, now, &result)
		}
		if reservation.Status != domain.ReservationReserved {
			return conflict("Quota reservation is already terminal")
		}
		if counter.ReservedUnits < reservation.Units {
			return conflict("Quota counter facts have drifted")
		}
		previousCounterRevision, previousReservationRevision := counter.Revision, reservation.Revision
		counter.ReservedUnits -= reservation.Units
		if desiredStatus == domain.ReservationConsumed {
			counter.ConsumedUnits += reservation.Units
			reservation.ConsumedAt = timePointer(now)
		} else {
			reservation.ReleasedAt = timePointer(now)
		}
		counter.Revision, counter.UpdatedAt = counter.Revision+1, now
		reservation.Status, reservation.Revision, reservation.UpdatedAt = desiredStatus, reservation.Revision+1, now
		if _, updateErr := repo.UpdateCounter(ctx, counter, previousCounterRevision); updateErr != nil {
			return updateErr
		}
		reservation, loadErr = repo.UpdateReservation(ctx, reservation, previousReservationRevision)
		if loadErr != nil {
			return loadErr
		}
		return storeReservationReceipt(ctx, repo, service.config.NewID, actor, operation, command.IdempotencyKey, inputHash, reservation, now, &result)
	})
	return result, normalizeError(err)
}

func (service *Service) GetDailyUsage(
	ctx context.Context,
	actor Actor,
	projectID string,
	metric string,
) (domain.Usage, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	projectID, metric = strings.TrimSpace(projectID), strings.TrimSpace(metric)
	if service == nil || service.transactions == nil || service.config.Now == nil ||
		!validActor(actor) || !validUUID(projectID) || !domain.IsGenerationMetric(metric) {
		return domain.Usage{}, invalid("Invalid daily quota query")
	}
	windowStart, windowEnd := domain.DailyWindow(service.config.Now())
	var usage domain.Usage
	err := service.transactions.WithinQuotaTransaction(ctx, func(repo Repository) error {
		policy, loadErr := repo.FindPolicy(ctx, projectID, metric)
		if loadErr != nil {
			return loadErr
		}
		if loadErr = repo.AuthorizeProject(ctx, actor, policy.WorkspaceID, policy.ProjectID, "read"); loadErr != nil {
			return loadErr
		}
		if loadErr = validatePolicy(policy); loadErr != nil {
			return loadErr
		}
		counter, loadErr := repo.FindCounter(ctx, policy.ID, windowStart)
		if errors.Is(loadErr, ErrCounterNotFound) {
			usage = domain.Usage{
				PolicyID: policy.ID, WorkspaceID: policy.WorkspaceID, ProjectID: policy.ProjectID, Metric: policy.Metric,
				WindowStart: windowStart, WindowEnd: windowEnd, PolicyRevision: policy.Revision,
				LimitUnits: policy.LimitUnits, AvailableUnits: policy.LimitUnits,
			}
			return nil
		}
		if loadErr != nil {
			return loadErr
		}
		if loadErr = validateCurrentCounter(policy, counter); loadErr != nil {
			return loadErr
		}
		reservations, loadErr := repo.ListReservations(ctx, counter.ID)
		if loadErr != nil {
			return loadErr
		}
		usage, loadErr = reconcileUsage(policy, counter, reservations)
		return loadErr
	})
	return usage, normalizeError(err)
}

func reconcileUsage(policy domain.Policy, counter domain.Counter, reservations []domain.Reservation) (domain.Usage, error) {
	if err := validatePolicy(policy); err != nil {
		return domain.Usage{}, err
	}
	if err := validateCounterScope(policy, counter); err != nil {
		return domain.Usage{}, err
	}
	var reserved, consumed int64
	seenSources := make(map[string]struct{}, len(reservations))
	for _, reservation := range reservations {
		if err := validationReservation(reservation); err != nil || reservation.CounterID != counter.ID ||
			reservation.PolicyID != policy.ID || reservation.WorkspaceID != policy.WorkspaceID ||
			reservation.ProjectID != policy.ProjectID || reservation.Metric != policy.Metric ||
			!reservation.WindowStart.Equal(counter.WindowStart) || !reservation.WindowEnd.Equal(counter.WindowEnd) ||
			reservation.PolicyRevision > counter.PolicyRevision ||
			(reservation.PolicyRevision == counter.PolicyRevision && reservation.LimitUnits != counter.LimitUnits) {
			return domain.Usage{}, conflict("Quota reservation facts have drifted")
		}
		sourceKey := reservation.SourceType + ":" + reservation.SourceID
		if _, duplicate := seenSources[sourceKey]; duplicate {
			return domain.Usage{}, conflict("Quota reservation source uniqueness has drifted")
		}
		seenSources[sourceKey] = struct{}{}
		switch reservation.Status {
		case domain.ReservationReserved:
			reserved += reservation.Units
		case domain.ReservationConsumed:
			consumed += reservation.Units
		case domain.ReservationReleased:
		default:
			return domain.Usage{}, conflict("Quota reservation state has drifted")
		}
	}
	if reserved != counter.ReservedUnits || consumed != counter.ConsumedUnits ||
		reserved+consumed > counter.LimitUnits {
		return domain.Usage{}, conflict("Quota counter facts have drifted")
	}
	return domain.Usage{
		PolicyID: policy.ID, CounterID: counter.ID, WorkspaceID: policy.WorkspaceID, ProjectID: policy.ProjectID,
		Metric: policy.Metric, WindowStart: counter.WindowStart, WindowEnd: counter.WindowEnd,
		PolicyRevision: counter.PolicyRevision, LimitUnits: counter.LimitUnits,
		ReservedUnits: reserved, ConsumedUnits: consumed, AvailableUnits: counter.LimitUnits - reserved - consumed,
	}, nil
}

func validatePolicy(value domain.Policy) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) || !validUUID(value.ProjectID) ||
		!domain.IsGenerationMetric(value.Metric) || value.WindowKind != domain.WindowUTCDay ||
		value.LimitUnits < 1 || value.Revision < 1 || len(value.ContentHash) != 64 {
		return conflict("Daily quota policy facts have drifted")
	}
	hash, err := policyContentHash(value)
	if err != nil || hash != value.ContentHash {
		return conflict("Daily quota policy facts have drifted")
	}
	return nil
}

func validateCounterScope(policy domain.Policy, value domain.Counter) error {
	windowStart, windowEnd := domain.DailyWindow(value.WindowStart)
	if !validUUID(value.ID) || value.WorkspaceID != policy.WorkspaceID || value.ProjectID != policy.ProjectID ||
		value.PolicyID != policy.ID || value.Metric != policy.Metric || !value.WindowStart.Equal(windowStart) ||
		!value.WindowEnd.Equal(windowEnd) || value.PolicyRevision < 1 || value.LimitUnits < 1 || value.ReservedUnits < 0 ||
		value.ConsumedUnits < 0 || value.ReservedUnits+value.ConsumedUnits > value.LimitUnits || value.Revision < 1 {
		return conflict("Quota counter facts have drifted")
	}
	return nil
}

func validateCurrentCounter(policy domain.Policy, value domain.Counter) error {
	if err := validateCounterScope(policy, value); err != nil {
		return err
	}
	if value.PolicyRevision != policy.Revision || value.LimitUnits != policy.LimitUnits {
		return conflict("Quota counter facts have drifted")
	}
	return nil
}

func validationReservation(value domain.Reservation) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) || !validUUID(value.ProjectID) ||
		!validUUID(value.PolicyID) || !validUUID(value.CounterID) || !domain.IsGenerationMetric(value.Metric) ||
		value.SourceType != "generation_intent" || !validUUID(value.SourceID) ||
		!value.WindowEnd.Equal(value.WindowStart.Add(24*time.Hour)) || value.PolicyRevision < 1 ||
		value.LimitUnits < 1 || value.Units < 1 || value.Units > value.LimitUnits || value.Revision < 1 {
		return conflict("Quota reservation facts have drifted")
	}
	if value.Status != domain.ReservationReserved && value.Status != domain.ReservationConsumed && value.Status != domain.ReservationReleased {
		return conflict("Quota reservation facts have drifted")
	}
	if (value.Status == domain.ReservationReserved && (value.ConsumedAt != nil || value.ReleasedAt != nil)) ||
		(value.Status == domain.ReservationConsumed && (value.ConsumedAt == nil || value.ReleasedAt != nil)) ||
		(value.Status == domain.ReservationReleased && (value.ConsumedAt != nil || value.ReleasedAt == nil)) {
		return conflict("Quota reservation terminal facts have drifted")
	}
	hash, err := reservationBindingHash(value)
	if err != nil || hash != value.BindingHash {
		return conflict("Quota reservation facts have drifted")
	}
	return nil
}

func policyContentHash(value domain.Policy) (string, error) {
	return platformcommand.InputHash(policyHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, Metric: value.Metric,
		WindowKind: value.WindowKind, LimitUnits: value.LimitUnits, Revision: value.Revision,
	})
}

func reservationBindingHash(value domain.Reservation) (string, error) {
	return platformcommand.InputHash(reservationHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, PolicyID: value.PolicyID, CounterID: value.CounterID,
		Metric: value.Metric, SourceType: value.SourceType, SourceID: value.SourceID,
		WindowStart: value.WindowStart.UTC(), WindowEnd: value.WindowEnd.UTC(), PolicyRevision: value.PolicyRevision,
		LimitUnits: value.LimitUnits, Units: value.Units,
	})
}

func storePolicyReceipt(
	ctx context.Context,
	repo Repository,
	newID func() string,
	actor Actor,
	key, inputHash string,
	policy domain.Policy,
	now time.Time,
	result *PolicyResult,
) error {
	encoded, err := platformcommand.Result(policyReceipt{Policy: policy})
	if err != nil {
		return err
	}
	receiptID := strings.TrimSpace(newID())
	if !validUUID(receiptID) {
		return errors.New("quota policy receipt identifier is invalid")
	}
	receipt, err := repo.EnsureReceipt(ctx, platformcommand.Receipt{
		ID: receiptID, WorkspaceID: policy.WorkspaceID, Operation: setPolicyOperation, IdempotencyKey: key,
		InputHash: inputHash, ResourceID: policy.ID, Result: encoded, CreatedBy: actor.UserID, CreatedAt: now,
	})
	if err != nil {
		return err
	}
	*result = PolicyResult{Policy: policy, Receipt: receipt}
	return nil
}

func storeReservationReceipt(
	ctx context.Context,
	repo Repository,
	newID func() string,
	actor Actor,
	operation, key, inputHash string,
	reservation domain.Reservation,
	now time.Time,
	result *ReservationResult,
) error {
	encoded, err := platformcommand.Result(reservationReceipt{Reservation: reservation})
	if err != nil {
		return err
	}
	receiptID := strings.TrimSpace(newID())
	if !validUUID(receiptID) {
		return errors.New("quota reservation receipt identifier is invalid")
	}
	receipt, err := repo.EnsureReceipt(ctx, platformcommand.Receipt{
		ID: receiptID, WorkspaceID: reservation.WorkspaceID, Operation: operation, IdempotencyKey: key,
		InputHash: inputHash, ResourceID: reservation.ID, Result: encoded, CreatedBy: actor.UserID, CreatedAt: now,
	})
	if err != nil {
		return err
	}
	*result = ReservationResult{Reservation: reservation, Receipt: receipt}
	return nil
}

func replayPolicy(
	ctx context.Context,
	repo Repository,
	receipt platformcommand.Receipt,
	inputHash string,
	result *PolicyResult,
) error {
	replayed, err := platformcommand.Replay[policyReceipt](receipt, inputHash)
	if err != nil {
		return err
	}
	if receipt.ResourceID != replayed.Policy.ID || validatePolicy(replayed.Policy) != nil {
		return platformcommand.ErrInputMismatch
	}
	current, err := repo.FindPolicy(ctx, replayed.Policy.ProjectID, replayed.Policy.Metric)
	if err != nil || current.ID != replayed.Policy.ID || current.WorkspaceID != replayed.Policy.WorkspaceID ||
		validatePolicy(current) != nil {
		return platformcommand.ErrInputMismatch
	}
	*result = PolicyResult{Policy: replayed.Policy, Receipt: receipt}
	return nil
}

func replayReservation(
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
	if receipt.ResourceID != replayed.Reservation.ID || validationReservation(replayed.Reservation) != nil {
		return platformcommand.ErrInputMismatch
	}
	current, err := repo.GetReservation(ctx, replayed.Reservation.ID)
	if err != nil || validationReservation(current) != nil ||
		!domain.SameReservationBinding(current, replayed.Reservation) {
		return platformcommand.ErrInputMismatch
	}
	*result = ReservationResult{Reservation: replayed.Reservation, Receipt: receipt}
	return nil
}

func validActor(actor Actor) bool { return actor.TokenVersion > 0 && validUUID(actor.UserID) }

func validUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func timePointer(value time.Time) *time.Time { return &value }

func normalizeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, platformcommand.ErrInputMismatch) {
		return conflict("Quota command or binding has drifted")
	}
	if errors.Is(err, ErrPolicyNotFound) {
		return notFound("Daily quota policy not found")
	}
	if errors.Is(err, ErrCounterNotFound) || errors.Is(err, ErrReservationNotFound) {
		return notFound("Quota reservation not found")
	}
	return err
}

func invalid(message string) error {
	return &Error{Code: "invalid_request", Message: message, Status: 422}
}

func conflict(message string) error {
	return &Error{Code: "state_conflict", Message: message, Status: 409}
}

func exceeded() error {
	return &Error{Code: "quota_exceeded", Message: "Daily image quota is exhausted", Status: 409, NextAction: "wait_for_next_window"}
}

func notFound(message string) error {
	return &Error{Code: "not_found", Message: message, Status: 404}
}
