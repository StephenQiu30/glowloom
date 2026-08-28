package application

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	costdomain "github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
	quotadomain "github.com/StephenQiu30/lanverse/backend/internal/quota/domain"
)

const (
	prepareIntentOperation = "generation.intent.prepare"
	claimIntentOperation   = "generation.intent.claim"
	cancelIntentOperation  = "generation.intent.cancel"
)

var (
	ErrIntentNotFound           = errors.New("generation intent not found")
	ErrGenerationTargetNotFound = errors.New("GenerationTarget not found")
	intentHashPattern           = regexp.MustCompile(`^[0-9a-f]{64}$`)
	claimantPattern             = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]{2,119}$`)
)

type CostPreparationOwner interface {
	CreateEstimate(context.Context, costapp.Actor, costapp.CreateEstimateCommand) (costapp.EstimateResult, error)
	ReserveEstimate(context.Context, costapp.Actor, costapp.ReserveEstimateCommand) (costapp.ReservationResult, error)
	ReleaseReservation(context.Context, costapp.Actor, costapp.ReleaseReservationCommand) (costapp.ReservationResult, error)
	GetEstimate(context.Context, costapp.Actor, string) (costdomain.Estimate, error)
	GetReservation(context.Context, costapp.Actor, string) (costapp.ReservationView, error)
}

type QuotaPreparationOwner interface {
	Reserve(context.Context, quotaapp.Actor, quotaapp.ReserveCommand) (quotaapp.ReservationResult, error)
	Release(context.Context, quotaapp.Actor, quotaapp.TransitionCommand) (quotaapp.ReservationResult, error)
	GetReservation(context.Context, quotaapp.Actor, string) (quotadomain.Reservation, error)
}

type PreparationRepository interface {
	AuthorizeProject(context.Context, Actor, string, string, bool) error
	ValidateWorkflowSource(context.Context, Actor, string, string, string, string, string) error
	FindGenerationTarget(context.Context, string) (domain.GenerationTarget, error)
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	FindReceiptByID(context.Context, string) (platformcommand.Receipt, error)
	EnsureReceipt(context.Context, platformcommand.Receipt) (platformcommand.Receipt, error)
	FindIntent(context.Context, string) (domain.Intent, error)
	GetIntentForUpdate(context.Context, string) (domain.Intent, error)
	EnsureIntent(context.Context, domain.Intent) (domain.Intent, error)
	UpdateIntent(context.Context, domain.Intent, int64) (domain.Intent, error)
}

type PreparationTransactionManager interface {
	WithinPreparationTransaction(
		context.Context,
		func(PreparationRepository, CostPreparationOwner, QuotaPreparationOwner) error,
	) error
}

type PreparationConfig struct {
	Now      func() time.Time
	NewID    func() string
	ClaimTTL time.Duration
}

type PreparationService struct {
	transactions PreparationTransactionManager
	config       PreparationConfig
}

type PrepareImageGenerationCommand struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	WorkflowInputHash, TargetID, TargetHash          string
	IdempotencyKey                                   string
	Units                                            int64
}

type AcquireExecutionClaimCommand struct {
	IntentID, Claimant, IdempotencyKey string
}

type CancelPreparedIntentCommand struct {
	IntentID, IdempotencyKey string
}

type IntentView struct {
	Intent           domain.Intent
	CostEstimate     costdomain.Estimate
	CostReservation  costdomain.Reservation
	QuotaReservation quotadomain.Reservation
}

type PreparationResult struct {
	IntentView
	Receipt platformcommand.Receipt
}

type ExecutionClaimResult struct {
	Intent        domain.Intent
	Authorization domain.ExecutionAuthorization
	Receipt       platformcommand.Receipt
}

type CancellationResult struct {
	Intent           domain.Intent
	CostReservation  costdomain.Reservation
	QuotaReservation quotadomain.Reservation
	Receipt          platformcommand.Receipt
}

type intentReceipt struct {
	Intent domain.Intent `json:"intent"`
}

type claimReceipt struct {
	Intent        domain.Intent                 `json:"intent"`
	Authorization domain.ExecutionAuthorization `json:"authorization"`
}

type cancellationReceipt struct {
	Intent           domain.Intent           `json:"intent"`
	CostReservation  costdomain.Reservation  `json:"cost_reservation"`
	QuotaReservation quotadomain.Reservation `json:"quota_reservation"`
}

type intentHashInput struct {
	ID, WorkspaceID, ProjectID, WorkflowRunID, NodeRunID  string
	TargetID, Metric, TargetHash                          string
	Units                                                 int64
	CostEstimateID, CostReservationID, QuotaReservationID string
	CostEstimateReceiptID, CostReservationReceiptID       string
	QuotaReservationReceiptID, CostReleaseReceiptID       string
	QuotaReleaseReceiptID, CostSettlementReceiptID        string
	QuotaConsumptionReceiptID                             string
	GenerationRequestID, ProviderJobID, ProviderReceiptID string
	Status, Claimant, ClaimToken                          string
	ClaimExpiresAt, CancelledAt                           string
	ClaimFencingVersion, Revision                         int64
	CreatedBy                                             string
	InitiatorTokenVersion                                 int
}

func NewPreparationService(transactions PreparationTransactionManager, config PreparationConfig) *PreparationService {
	return &PreparationService{transactions: transactions, config: config}
}

func (service *PreparationService) PrepareImageGeneration(
	ctx context.Context,
	actor Actor,
	command PrepareImageGenerationCommand,
) (PreparationResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.ProjectID = strings.TrimSpace(command.ProjectID)
	command.WorkflowRunID = strings.TrimSpace(command.WorkflowRunID)
	command.NodeRunID = strings.TrimSpace(command.NodeRunID)
	command.WorkflowInputHash = strings.TrimSpace(command.WorkflowInputHash)
	command.TargetID = strings.TrimSpace(command.TargetID)
	command.TargetHash = strings.TrimSpace(command.TargetHash)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.transactions == nil || service.config.Now == nil || service.config.NewID == nil ||
		!validPreparationActor(actor) || !validUUID(command.WorkspaceID) || !validUUID(command.ProjectID) ||
		!validUUID(command.WorkflowRunID) || !validUUID(command.NodeRunID) ||
		!intentHashPattern.MatchString(command.WorkflowInputHash) || !validUUID(command.TargetID) ||
		!intentHashPattern.MatchString(command.TargetHash) || command.Units < 1 ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return PreparationResult{}, invalid("Invalid image generation preparation")
	}
	inputHash, err := platformcommand.InputHash(struct {
		ActorID string
		Command PrepareImageGenerationCommand
	}{ActorID: actor.UserID, Command: command})
	if err != nil {
		return PreparationResult{}, err
	}
	now := service.config.Now().UTC()
	var result PreparationResult
	err = service.transactions.WithinPreparationTransaction(ctx, func(
		repo PreparationRepository,
		costs CostPreparationOwner,
		quotas QuotaPreparationOwner,
	) error {
		if authorizeErr := repo.AuthorizeProject(ctx, actor, command.WorkspaceID, command.ProjectID, true); authorizeErr != nil {
			return authorizeErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, command.WorkspaceID, prepareIntentOperation, command.IdempotencyKey); findErr == nil {
			return service.replayPreparation(ctx, repo, costs, quotas, actor, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		if sourceErr := repo.ValidateWorkflowSource(
			ctx, actor, command.WorkspaceID, command.ProjectID, command.WorkflowRunID, command.NodeRunID, command.WorkflowInputHash,
		); sourceErr != nil {
			return sourceErr
		}
		target, targetErr := repo.FindGenerationTarget(ctx, command.TargetID)
		if targetErr != nil {
			return targetErr
		}
		if domain.ValidateGenerationTarget(target) != nil || target.WorkspaceID != command.WorkspaceID ||
			target.ProjectID != command.ProjectID || target.TargetHash != command.TargetHash || target.CreatedBy != actor.UserID {
			return conflict("GenerationTarget binding has drifted")
		}
		intentID := strings.TrimSpace(service.config.NewID())
		if !validUUID(intentID) {
			return errors.New("generation intent identifier is invalid")
		}
		desired := domain.Intent{
			ID: intentID, WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
			WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
			TargetID: target.ID, Metric: costdomain.MetricGenerationImage, TargetHash: target.TargetHash, Units: command.Units,
			Status: domain.IntentPreparing, ClaimFencingVersion: 0, Revision: 1,
			CreatedBy: actor.UserID, InitiatorTokenVersion: actor.TokenVersion,
			CreatedAt: now, UpdatedAt: now,
		}
		desired.ContentHash, err = intentContentHash(desired)
		if err != nil {
			return err
		}
		intent, ensureErr := repo.EnsureIntent(ctx, desired)
		if ensureErr != nil {
			return ensureErr
		}
		if !domain.SameIntentBinding(intent, desired) {
			return platformcommand.ErrInputMismatch
		}
		if intent.Status != domain.IntentPreparing {
			if intent.Status != domain.IntentPrepared {
				return conflict("Generation intent is no longer preparable")
			}
			view, viewErr := service.loadIntentView(ctx, repo, costs, quotas, actor, intent)
			if viewErr != nil {
				return viewErr
			}
			return service.storePreparationReceipt(
				ctx, repo, actor, command.IdempotencyKey, inputHash, view, now, &result,
			)
		}
		if intent.ID != desired.ID || !domain.SameIntentState(intent, desired) {
			return platformcommand.ErrInputMismatch
		}
		costActor := costapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}
		estimateResult, ownerErr := costs.CreateEstimate(ctx, costActor, costapp.CreateEstimateCommand{
			ProjectID: command.ProjectID, Metric: costdomain.MetricGenerationImage,
			SourceType: costdomain.SourceGenerationIntent, SourceID: intent.ID, Units: command.Units,
			IdempotencyKey: preparationOwnerKey(intent.ID, "cost-estimate"),
		})
		if ownerErr != nil {
			return ownerErr
		}
		costReservationResult, ownerErr := costs.ReserveEstimate(ctx, costActor, costapp.ReserveEstimateCommand{
			EstimateID:     estimateResult.Estimate.ID,
			IdempotencyKey: preparationOwnerKey(intent.ID, "cost-reserve"),
		})
		if ownerErr != nil {
			return ownerErr
		}
		quotaResult, ownerErr := quotas.Reserve(ctx, quotaapp.Actor{
			UserID: actor.UserID, TokenVersion: actor.TokenVersion,
		}, quotaapp.ReserveCommand{
			WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
			Metric: quotadomain.MetricGenerationImage, SourceType: "generation_intent", SourceID: intent.ID,
			Units: command.Units, IdempotencyKey: preparationOwnerKey(intent.ID, "quota-reserve"),
		})
		if ownerErr != nil {
			return ownerErr
		}
		intent.CostEstimateID = estimateResult.Estimate.ID
		intent.CostReservationID = costReservationResult.Reservation.ID
		intent.QuotaReservationID = quotaResult.Reservation.ID
		intent.CostEstimateReceiptID = estimateResult.Receipt.ID
		intent.CostReservationReceiptID = costReservationResult.Receipt.ID
		intent.QuotaReservationReceiptID = quotaResult.Receipt.ID
		intent.Status, intent.Revision, intent.UpdatedAt = domain.IntentPrepared, 2, now
		intent.ContentHash, ownerErr = intentContentHash(intent)
		if ownerErr != nil {
			return ownerErr
		}
		intent, ownerErr = repo.UpdateIntent(ctx, intent, 1)
		if ownerErr != nil {
			return ownerErr
		}
		view := IntentView{
			Intent: intent, CostEstimate: estimateResult.Estimate,
			CostReservation: costReservationResult.Reservation, QuotaReservation: quotaResult.Reservation,
		}
		if ownerErr = validateIntentView(view); ownerErr != nil {
			return ownerErr
		}
		if ownerErr = service.validateOwnerReceipts(ctx, repo, view.Intent); ownerErr != nil {
			return ownerErr
		}
		return service.storePreparationReceipt(
			ctx, repo, actor, command.IdempotencyKey, inputHash, view, now, &result,
		)
	})
	return result, normalizePreparationError(err)
}

func (service *PreparationService) AcquireExecutionClaim(
	ctx context.Context,
	command AcquireExecutionClaimCommand,
) (ExecutionClaimResult, error) {
	command.IntentID = strings.TrimSpace(command.IntentID)
	command.Claimant = strings.TrimSpace(command.Claimant)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.transactions == nil || service.config.Now == nil || service.config.NewID == nil ||
		service.config.ClaimTTL <= 0 || service.config.ClaimTTL > 15*time.Minute || !validUUID(command.IntentID) ||
		!claimantPattern.MatchString(command.Claimant) || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return ExecutionClaimResult{}, invalid("Invalid Generation execution claim")
	}
	inputHash, err := platformcommand.InputHash(command)
	if err != nil {
		return ExecutionClaimResult{}, err
	}
	now := service.config.Now().UTC()
	var result ExecutionClaimResult
	err = service.transactions.WithinPreparationTransaction(ctx, func(
		repo PreparationRepository,
		costs CostPreparationOwner,
		quotas QuotaPreparationOwner,
	) error {
		intent, loadErr := repo.GetIntentForUpdate(ctx, command.IntentID)
		if loadErr != nil {
			return loadErr
		}
		actor := intentActor(intent)
		if loadErr = repo.AuthorizeProject(ctx, actor, intent.WorkspaceID, intent.ProjectID, true); loadErr != nil {
			return loadErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, intent.WorkspaceID, claimIntentOperation, command.IdempotencyKey); findErr == nil {
			return service.replayClaim(ctx, repo, costs, quotas, actor, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		if validateErr := validateIntent(intent); validateErr != nil {
			return validateErr
		}
		if intent.Status != domain.IntentPrepared {
			return conflict("Generation intent is not available to claim")
		}
		if _, loadErr = service.loadIntentView(ctx, repo, costs, quotas, actor, intent); loadErr != nil {
			return loadErr
		}
		claimToken := strings.TrimSpace(service.config.NewID())
		if !validUUID(claimToken) {
			return errors.New("generation execution claim token is invalid")
		}
		expiresAt := now.Add(service.config.ClaimTTL)
		intent.Status, intent.Claimant, intent.ClaimToken = domain.IntentClaimed, stringPointer(command.Claimant), stringPointer(claimToken)
		intent.ClaimExpiresAt, intent.ClaimFencingVersion = timePointer(expiresAt), 1
		intent.Revision, intent.UpdatedAt = intent.Revision+1, now
		intent.ContentHash, loadErr = intentContentHash(intent)
		if loadErr != nil {
			return loadErr
		}
		intent, loadErr = repo.UpdateIntent(ctx, intent, intent.Revision-1)
		if loadErr != nil {
			return loadErr
		}
		authorization := authorizationForIntent(intent)
		return service.storeClaimReceipt(
			ctx, repo, actor, command.IdempotencyKey, inputHash, intent, authorization, now, &result,
		)
	})
	return result, normalizePreparationError(err)
}

func (service *PreparationService) CancelPreparedIntent(
	ctx context.Context,
	actor Actor,
	command CancelPreparedIntentCommand,
) (CancellationResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.IntentID = strings.TrimSpace(command.IntentID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.transactions == nil || service.config.Now == nil || service.config.NewID == nil ||
		!validPreparationActor(actor) || !validUUID(command.IntentID) || command.IdempotencyKey == "" ||
		len(command.IdempotencyKey) > 200 {
		return CancellationResult{}, invalid("Invalid Generation cancellation")
	}
	inputHash, err := platformcommand.InputHash(struct {
		ActorID string
		Command CancelPreparedIntentCommand
	}{ActorID: actor.UserID, Command: command})
	if err != nil {
		return CancellationResult{}, err
	}
	now := service.config.Now().UTC()
	var result CancellationResult
	err = service.transactions.WithinPreparationTransaction(ctx, func(
		repo PreparationRepository,
		costs CostPreparationOwner,
		quotas QuotaPreparationOwner,
	) error {
		intent, loadErr := repo.GetIntentForUpdate(ctx, command.IntentID)
		if loadErr != nil {
			return loadErr
		}
		if intent.CreatedBy != actor.UserID || intent.InitiatorTokenVersion != actor.TokenVersion {
			return conflict("Generation cancellation initiator has drifted")
		}
		if loadErr = repo.AuthorizeProject(ctx, actor, intent.WorkspaceID, intent.ProjectID, true); loadErr != nil {
			return loadErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, intent.WorkspaceID, cancelIntentOperation, command.IdempotencyKey); findErr == nil {
			return service.replayCancellation(ctx, repo, costs, quotas, actor, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		if validateErr := validateIntent(intent); validateErr != nil {
			return validateErr
		}
		if intent.Status != domain.IntentPrepared {
			return conflict("Generation intent is not cancellable")
		}
		preparedView, loadErr := service.loadIntentView(ctx, repo, costs, quotas, actor, intent)
		if loadErr != nil {
			return loadErr
		}
		costResult, loadErr := costs.ReleaseReservation(ctx, costapp.Actor{
			UserID: actor.UserID, TokenVersion: actor.TokenVersion,
		}, costapp.ReleaseReservationCommand{
			ReservationID:  intent.CostReservationID,
			IdempotencyKey: preparationOwnerKey(intent.ID, "cost-release"),
		})
		if loadErr != nil {
			return loadErr
		}
		quotaResult, loadErr := quotas.Release(ctx, quotaapp.Actor{
			UserID: actor.UserID, TokenVersion: actor.TokenVersion,
		}, quotaapp.TransitionCommand{
			ReservationID:  intent.QuotaReservationID,
			IdempotencyKey: preparationOwnerKey(intent.ID, "quota-release"),
		})
		if loadErr != nil {
			return loadErr
		}
		intent.Status, intent.CancelledAt = domain.IntentCancelled, timePointer(now)
		intent.CostReleaseReceiptID, intent.QuotaReleaseReceiptID = costResult.Receipt.ID, quotaResult.Receipt.ID
		intent.Revision, intent.UpdatedAt = intent.Revision+1, now
		intent.ContentHash, loadErr = intentContentHash(intent)
		if loadErr != nil {
			return loadErr
		}
		intent, loadErr = repo.UpdateIntent(ctx, intent, intent.Revision-1)
		if loadErr != nil {
			return loadErr
		}
		preparedView.Intent = intent
		preparedView.CostReservation = costResult.Reservation
		preparedView.QuotaReservation = quotaResult.Reservation
		if loadErr = validateIntentView(preparedView); loadErr != nil {
			return loadErr
		}
		cancelled := CancellationResult{
			Intent: intent, CostReservation: costResult.Reservation, QuotaReservation: quotaResult.Reservation,
		}
		if loadErr = service.validateOwnerReceipts(ctx, repo, intent); loadErr != nil {
			return loadErr
		}
		return service.storeCancellationReceipt(
			ctx, repo, actor, command.IdempotencyKey, inputHash, cancelled, now, &result,
		)
	})
	return result, normalizePreparationError(err)
}

func (service *PreparationService) GetIntent(ctx context.Context, actor Actor, intentID string) (IntentView, error) {
	actor.UserID, intentID = strings.TrimSpace(actor.UserID), strings.TrimSpace(intentID)
	if service == nil || service.transactions == nil || !validPreparationActor(actor) || !validUUID(intentID) {
		return IntentView{}, invalid("Invalid Generation intent query")
	}
	var result IntentView
	err := service.transactions.WithinPreparationTransaction(ctx, func(
		repo PreparationRepository,
		costs CostPreparationOwner,
		quotas QuotaPreparationOwner,
	) error {
		intent, loadErr := repo.FindIntent(ctx, intentID)
		if loadErr != nil {
			return loadErr
		}
		if loadErr = repo.AuthorizeProject(ctx, actor, intent.WorkspaceID, intent.ProjectID, false); loadErr != nil {
			return loadErr
		}
		result, loadErr = service.loadIntentView(ctx, repo, costs, quotas, actor, intent)
		return loadErr
	})
	return result, normalizePreparationError(err)
}

func (service *PreparationService) VerifyExecutionAuthorization(
	ctx context.Context,
	authorization domain.ExecutionAuthorization,
) error {
	authorization.IntentID = strings.TrimSpace(authorization.IntentID)
	authorization.ClaimToken = strings.TrimSpace(authorization.ClaimToken)
	authorization.TargetID = strings.TrimSpace(authorization.TargetID)
	authorization.TargetHash = strings.TrimSpace(authorization.TargetHash)
	authorization.CostReservationID = strings.TrimSpace(authorization.CostReservationID)
	authorization.QuotaReservationID = strings.TrimSpace(authorization.QuotaReservationID)
	if service == nil || service.transactions == nil || service.config.Now == nil ||
		!validUUID(authorization.IntentID) || !validUUID(authorization.ClaimToken) ||
		!validUUID(authorization.TargetID) ||
		!validUUID(authorization.CostReservationID) || !validUUID(authorization.QuotaReservationID) ||
		!intentHashPattern.MatchString(authorization.TargetHash) || authorization.ClaimFencingVersion < 1 ||
		authorization.IntentRevision < 1 || authorization.Units < 1 || authorization.ExpiresAt.IsZero() {
		return invalid("Invalid Generation execution authorization")
	}
	err := service.transactions.WithinPreparationTransaction(ctx, func(
		repo PreparationRepository,
		costs CostPreparationOwner,
		quotas QuotaPreparationOwner,
	) error {
		intent, loadErr := repo.GetIntentForUpdate(ctx, authorization.IntentID)
		if loadErr != nil {
			return loadErr
		}
		if loadErr = validateIntent(intent); loadErr != nil {
			return loadErr
		}
		actor := intentActor(intent)
		if loadErr = repo.AuthorizeProject(ctx, actor, intent.WorkspaceID, intent.ProjectID, true); loadErr != nil {
			return loadErr
		}
		if intent.Status != domain.IntentClaimed || intent.ClaimToken == nil || intent.ClaimExpiresAt == nil ||
			*intent.ClaimToken != authorization.ClaimToken || !intent.ClaimExpiresAt.Equal(authorization.ExpiresAt) ||
			intent.TargetID != authorization.TargetID || intent.TargetHash != authorization.TargetHash ||
			intent.CostReservationID != authorization.CostReservationID ||
			intent.QuotaReservationID != authorization.QuotaReservationID ||
			intent.ClaimFencingVersion != authorization.ClaimFencingVersion ||
			intent.Revision != authorization.IntentRevision || intent.Units != authorization.Units {
			return conflict("Generation execution authorization has drifted")
		}
		if !service.config.Now().UTC().Before(*intent.ClaimExpiresAt) {
			return authorizationExpired()
		}
		_, loadErr = service.loadIntentView(ctx, repo, costs, quotas, actor, intent)
		return loadErr
	})
	return normalizePreparationError(err)
}

func (service *PreparationService) loadIntentView(
	ctx context.Context,
	repo PreparationRepository,
	costs CostPreparationOwner,
	quotas QuotaPreparationOwner,
	actor Actor,
	intent domain.Intent,
) (IntentView, error) {
	if err := validateIntent(intent); err != nil || intent.Status == domain.IntentPreparing {
		if err != nil {
			return IntentView{}, err
		}
		return IntentView{}, conflict("Generation intent preparation is incomplete")
	}
	target, err := repo.FindGenerationTarget(ctx, intent.TargetID)
	if err != nil {
		return IntentView{}, err
	}
	if domain.ValidateGenerationTarget(target) != nil || target.WorkspaceID != intent.WorkspaceID ||
		target.ProjectID != intent.ProjectID || target.TargetHash != intent.TargetHash || target.CreatedBy != intent.CreatedBy {
		return IntentView{}, conflict("GenerationTarget binding has drifted")
	}
	costActor := costapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}
	estimate, err := costs.GetEstimate(ctx, costActor, intent.CostEstimateID)
	if err != nil {
		return IntentView{}, err
	}
	costView, err := costs.GetReservation(ctx, costActor, intent.CostReservationID)
	if err != nil {
		return IntentView{}, err
	}
	quotaReservation, err := quotas.GetReservation(ctx, quotaapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, intent.QuotaReservationID)
	if err != nil {
		return IntentView{}, err
	}
	view := IntentView{
		Intent: intent, CostEstimate: estimate,
		CostReservation: costView.Reservation, QuotaReservation: quotaReservation,
	}
	if err = validateIntentView(view); err != nil {
		return IntentView{}, err
	}
	if err = service.validateOwnerReceipts(ctx, repo, intent); err != nil {
		return IntentView{}, err
	}
	return view, nil
}

func validateIntentView(view IntentView) error {
	intent, estimate := view.Intent, view.CostEstimate
	if validateIntent(intent) != nil || estimate.ID != intent.CostEstimateID ||
		estimate.WorkspaceID != intent.WorkspaceID || estimate.ProjectID != intent.ProjectID ||
		estimate.SourceType != costdomain.SourceGenerationIntent || estimate.SourceID != intent.ID ||
		estimate.Metric != intent.Metric || estimate.Units != intent.Units ||
		view.CostReservation.ID != intent.CostReservationID ||
		view.CostReservation.EstimateID != estimate.ID || view.CostReservation.SourceID != intent.ID ||
		view.CostReservation.WorkspaceID != intent.WorkspaceID || view.CostReservation.ProjectID != intent.ProjectID ||
		view.QuotaReservation.ID != intent.QuotaReservationID || view.QuotaReservation.SourceType != "generation_intent" ||
		view.QuotaReservation.SourceID != intent.ID || view.QuotaReservation.WorkspaceID != intent.WorkspaceID ||
		view.QuotaReservation.ProjectID != intent.ProjectID || view.QuotaReservation.Metric != intent.Metric ||
		view.QuotaReservation.Units != intent.Units {
		return conflict("Generation intent Owner bindings have drifted")
	}
	switch intent.Status {
	case domain.IntentPrepared, domain.IntentClaimed, domain.IntentDispatching,
		domain.IntentSubmitted, domain.IntentOutcomeUnknown:
		if view.CostReservation.Status != costdomain.ReservationReserved ||
			view.QuotaReservation.Status != quotadomain.ReservationReserved {
			return conflict("Generation intent reservations are no longer executable")
		}
	case domain.IntentCancelled, domain.IntentFailed:
		if view.CostReservation.Status != costdomain.ReservationReleased ||
			view.QuotaReservation.Status != quotadomain.ReservationReleased {
			return conflict("Generation cancellation reservations have drifted")
		}
	case domain.IntentSucceeded:
		if view.CostReservation.Status != costdomain.ReservationSettled ||
			view.QuotaReservation.Status != quotadomain.ReservationConsumed {
			return conflict("Generation terminal reservations have drifted")
		}
	default:
		return conflict("Generation intent state is not readable")
	}
	return nil
}

func (service *PreparationService) validateOwnerReceipts(
	ctx context.Context,
	repo PreparationRepository,
	intent domain.Intent,
) error {
	checks := []struct {
		id, operation, key, resource string
	}{
		{intent.CostEstimateReceiptID, "cost.estimate.create", preparationOwnerKey(intent.ID, "cost-estimate"), intent.CostEstimateID},
		{intent.CostReservationReceiptID, "cost.reservation.reserve", preparationOwnerKey(intent.ID, "cost-reserve"), intent.CostReservationID},
		{intent.QuotaReservationReceiptID, "quota.reservation.reserve", preparationOwnerKey(intent.ID, "quota-reserve"), intent.QuotaReservationID},
	}
	if intent.Status == domain.IntentCancelled || intent.Status == domain.IntentFailed {
		checks = append(checks,
			struct{ id, operation, key, resource string }{intent.CostReleaseReceiptID, "cost.reservation.release", preparationOwnerKey(intent.ID, "cost-release"), intent.CostReservationID},
			struct{ id, operation, key, resource string }{intent.QuotaReleaseReceiptID, "quota.reservation.release", preparationOwnerKey(intent.ID, "quota-release"), intent.QuotaReservationID},
		)
	}
	if intent.Status == domain.IntentSucceeded {
		checks = append(checks,
			struct{ id, operation, key, resource string }{intent.CostSettlementReceiptID, "cost.reservation.settle", preparationOwnerKey(intent.ID, "cost-settle"), intent.CostReservationID},
			struct{ id, operation, key, resource string }{intent.QuotaConsumptionReceiptID, "quota.reservation.consume", preparationOwnerKey(intent.ID, "quota-consume"), intent.QuotaReservationID},
		)
	}
	for _, check := range checks {
		receipt, err := repo.FindReceiptByID(ctx, check.id)
		if err != nil || receipt.WorkspaceID != intent.WorkspaceID || receipt.Operation != check.operation ||
			receipt.IdempotencyKey != check.key || receipt.ResourceID != check.resource || receipt.CreatedBy != intent.CreatedBy {
			return conflict("Generation intent Owner receipt bindings have drifted")
		}
	}
	return nil
}

func validateIntent(value domain.Intent) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) || !validUUID(value.ProjectID) ||
		!validUUID(value.WorkflowRunID) || !validUUID(value.NodeRunID) || !validUUID(value.TargetID) ||
		value.Metric != costdomain.MetricGenerationImage || !intentHashPattern.MatchString(value.TargetHash) ||
		value.Units < 1 || !validUUID(value.CreatedBy) || value.InitiatorTokenVersion < 1 ||
		value.Revision < 1 || len(value.ContentHash) != 64 || value.CreatedAt.IsZero() ||
		value.UpdatedAt.Before(value.CreatedAt) {
		return conflict("Generation intent facts have drifted")
	}
	ownerRefsValid := validUUID(value.CostEstimateID) && validUUID(value.CostReservationID) &&
		validUUID(value.QuotaReservationID) && validUUID(value.CostEstimateReceiptID) &&
		validUUID(value.CostReservationReceiptID) && validUUID(value.QuotaReservationReceiptID)
	providerRefsValid := validUUID(value.GenerationRequestID) && validUUID(value.ProviderJobID)
	terminalProviderRefsValid := providerRefsValid && validUUID(value.ProviderReceiptID)
	switch value.Status {
	case domain.IntentPreparing:
		if value.Revision != 1 || ownerRefsValid || value.CostEstimateID != "" || value.CostReservationID != "" ||
			value.QuotaReservationID != "" || value.CostEstimateReceiptID != "" ||
			value.CostReservationReceiptID != "" || value.QuotaReservationReceiptID != "" ||
			value.CostReleaseReceiptID != "" || value.QuotaReleaseReceiptID != "" ||
			value.CostSettlementReceiptID != "" || value.QuotaConsumptionReceiptID != "" ||
			value.GenerationRequestID != "" || value.ProviderJobID != "" || value.ProviderReceiptID != "" ||
			value.Claimant != nil || value.ClaimToken != nil || value.ClaimExpiresAt != nil ||
			value.ClaimFencingVersion != 0 || value.CancelledAt != nil || !value.UpdatedAt.Equal(value.CreatedAt) {
			return conflict("Generation intent facts have drifted")
		}
	case domain.IntentPrepared:
		if value.Revision != 2 || !ownerRefsValid || value.CostReleaseReceiptID != "" || value.QuotaReleaseReceiptID != "" ||
			value.CostSettlementReceiptID != "" || value.QuotaConsumptionReceiptID != "" ||
			value.GenerationRequestID != "" || value.ProviderJobID != "" || value.ProviderReceiptID != "" ||
			value.Claimant != nil || value.ClaimToken != nil || value.ClaimExpiresAt != nil ||
			value.ClaimFencingVersion != 0 || value.CancelledAt != nil {
			return conflict("Generation intent facts have drifted")
		}
	case domain.IntentClaimed:
		if value.Revision != 3 || !ownerRefsValid || value.CostReleaseReceiptID != "" || value.QuotaReleaseReceiptID != "" ||
			value.CostSettlementReceiptID != "" || value.QuotaConsumptionReceiptID != "" ||
			value.GenerationRequestID != "" || value.ProviderJobID != "" || value.ProviderReceiptID != "" ||
			value.Claimant == nil || !claimantPattern.MatchString(*value.Claimant) ||
			value.ClaimToken == nil || !validUUID(*value.ClaimToken) || value.ClaimExpiresAt == nil ||
			!value.ClaimExpiresAt.After(value.UpdatedAt) || value.ClaimFencingVersion != 1 || value.CancelledAt != nil {
			return conflict("Generation intent facts have drifted")
		}
	case domain.IntentDispatching, domain.IntentSubmitted, domain.IntentOutcomeUnknown:
		if value.Revision < 4 || !ownerRefsValid || !providerRefsValid || value.ProviderReceiptID != "" ||
			value.CostReleaseReceiptID != "" || value.QuotaReleaseReceiptID != "" ||
			value.CostSettlementReceiptID != "" || value.QuotaConsumptionReceiptID != "" ||
			value.Claimant == nil || !claimantPattern.MatchString(*value.Claimant) ||
			value.ClaimToken == nil || !validUUID(*value.ClaimToken) || value.ClaimExpiresAt == nil ||
			value.ClaimFencingVersion != 1 || value.CancelledAt != nil {
			return conflict("Generation intent facts have drifted")
		}
	case domain.IntentSucceeded:
		if value.Revision < 5 || !ownerRefsValid || !terminalProviderRefsValid ||
			!validUUID(value.CostSettlementReceiptID) || !validUUID(value.QuotaConsumptionReceiptID) ||
			value.CostReleaseReceiptID != "" || value.QuotaReleaseReceiptID != "" ||
			value.Claimant == nil || !claimantPattern.MatchString(*value.Claimant) ||
			value.ClaimToken == nil || !validUUID(*value.ClaimToken) || value.ClaimExpiresAt == nil ||
			value.ClaimFencingVersion != 1 || value.CancelledAt != nil {
			return conflict("Generation intent facts have drifted")
		}
	case domain.IntentFailed:
		if value.Revision < 5 || !ownerRefsValid || !terminalProviderRefsValid ||
			!validUUID(value.CostReleaseReceiptID) || !validUUID(value.QuotaReleaseReceiptID) ||
			value.CostSettlementReceiptID != "" || value.QuotaConsumptionReceiptID != "" ||
			value.Claimant == nil || !claimantPattern.MatchString(*value.Claimant) ||
			value.ClaimToken == nil || !validUUID(*value.ClaimToken) || value.ClaimExpiresAt == nil ||
			value.ClaimFencingVersion != 1 || value.CancelledAt != nil {
			return conflict("Generation intent facts have drifted")
		}
	case domain.IntentCancelled:
		if value.Revision != 3 || !ownerRefsValid || !validUUID(value.CostReleaseReceiptID) ||
			!validUUID(value.QuotaReleaseReceiptID) || value.CostSettlementReceiptID != "" ||
			value.QuotaConsumptionReceiptID != "" || value.GenerationRequestID != "" ||
			value.ProviderJobID != "" || value.ProviderReceiptID != "" ||
			value.Claimant != nil || value.ClaimToken != nil ||
			value.ClaimExpiresAt != nil || value.ClaimFencingVersion != 0 || value.CancelledAt == nil ||
			!value.CancelledAt.Equal(value.UpdatedAt) {
			return conflict("Generation intent facts have drifted")
		}
	default:
		return conflict("Generation intent facts have drifted")
	}
	hash, err := intentContentHash(value)
	if err != nil || hash != value.ContentHash {
		return conflict("Generation intent facts have drifted")
	}
	return nil
}

func intentContentHash(value domain.Intent) (string, error) {
	return platformcommand.InputHash(intentHashInput{
		ID: value.ID, WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID,
		WorkflowRunID: value.WorkflowRunID, NodeRunID: value.NodeRunID,
		TargetID: value.TargetID, Metric: value.Metric, TargetHash: value.TargetHash, Units: value.Units,
		CostEstimateID: value.CostEstimateID, CostReservationID: value.CostReservationID,
		QuotaReservationID:        value.QuotaReservationID,
		CostEstimateReceiptID:     value.CostEstimateReceiptID,
		CostReservationReceiptID:  value.CostReservationReceiptID,
		QuotaReservationReceiptID: value.QuotaReservationReceiptID,
		CostReleaseReceiptID:      value.CostReleaseReceiptID, QuotaReleaseReceiptID: value.QuotaReleaseReceiptID,
		CostSettlementReceiptID:   value.CostSettlementReceiptID,
		QuotaConsumptionReceiptID: value.QuotaConsumptionReceiptID,
		GenerationRequestID:       value.GenerationRequestID, ProviderJobID: value.ProviderJobID,
		ProviderReceiptID: value.ProviderReceiptID,
		Status:            value.Status, Claimant: optionalString(value.Claimant), ClaimToken: optionalString(value.ClaimToken),
		ClaimExpiresAt: optionalTime(value.ClaimExpiresAt), CancelledAt: optionalTime(value.CancelledAt),
		ClaimFencingVersion: value.ClaimFencingVersion, Revision: value.Revision,
		CreatedBy: value.CreatedBy, InitiatorTokenVersion: value.InitiatorTokenVersion,
	})
}

func (service *PreparationService) storePreparationReceipt(
	ctx context.Context,
	repo PreparationRepository,
	actor Actor,
	key, inputHash string,
	view IntentView,
	now time.Time,
	result *PreparationResult,
) error {
	encoded, err := platformcommand.Result(intentReceipt{Intent: view.Intent})
	if err != nil {
		return err
	}
	receipt, err := service.ensureReceipt(ctx, repo, actor, view.Intent.WorkspaceID, prepareIntentOperation,
		key, inputHash, view.Intent.ID, encoded, now)
	if err != nil {
		return err
	}
	*result = PreparationResult{IntentView: view, Receipt: receipt}
	return nil
}

func (service *PreparationService) replayPreparation(
	ctx context.Context,
	repo PreparationRepository,
	costs CostPreparationOwner,
	quotas QuotaPreparationOwner,
	actor Actor,
	receipt platformcommand.Receipt,
	inputHash string,
	result *PreparationResult,
) error {
	replayed, err := platformcommand.Replay[intentReceipt](receipt, inputHash)
	if err != nil || receipt.ResourceID != replayed.Intent.ID || validateIntent(replayed.Intent) != nil ||
		replayed.Intent.Status != domain.IntentPrepared {
		return platformcommand.ErrInputMismatch
	}
	persisted, err := repo.FindIntent(ctx, replayed.Intent.ID)
	if err != nil || !intentProgressedFromPreparation(persisted, replayed.Intent) {
		return platformcommand.ErrInputMismatch
	}
	view, err := service.loadIntentView(ctx, repo, costs, quotas, actor, persisted)
	if err != nil {
		return err
	}
	*result = PreparationResult{IntentView: view, Receipt: receipt}
	return nil
}

func (service *PreparationService) storeClaimReceipt(
	ctx context.Context,
	repo PreparationRepository,
	actor Actor,
	key, inputHash string,
	intent domain.Intent,
	authorization domain.ExecutionAuthorization,
	now time.Time,
	result *ExecutionClaimResult,
) error {
	encoded, err := platformcommand.Result(claimReceipt{Intent: intent, Authorization: authorization})
	if err != nil {
		return err
	}
	receipt, err := service.ensureReceipt(ctx, repo, actor, intent.WorkspaceID, claimIntentOperation,
		key, inputHash, intent.ID, encoded, now)
	if err != nil {
		return err
	}
	*result = ExecutionClaimResult{Intent: intent, Authorization: authorization, Receipt: receipt}
	return nil
}

func (service *PreparationService) replayClaim(
	ctx context.Context,
	repo PreparationRepository,
	costs CostPreparationOwner,
	quotas QuotaPreparationOwner,
	actor Actor,
	receipt platformcommand.Receipt,
	inputHash string,
	result *ExecutionClaimResult,
) error {
	replayed, err := platformcommand.Replay[claimReceipt](receipt, inputHash)
	if err != nil || receipt.ResourceID != replayed.Intent.ID || validateIntent(replayed.Intent) != nil ||
		replayed.Intent.Status != domain.IntentClaimed || replayed.Authorization != authorizationForIntent(replayed.Intent) {
		return platformcommand.ErrInputMismatch
	}
	persisted, err := repo.FindIntent(ctx, replayed.Intent.ID)
	if err != nil || !intentProgressedFromClaim(persisted, replayed.Intent) {
		return platformcommand.ErrInputMismatch
	}
	if _, err = service.loadIntentView(ctx, repo, costs, quotas, actor, persisted); err != nil {
		return err
	}
	*result = ExecutionClaimResult{Intent: persisted, Authorization: replayed.Authorization, Receipt: receipt}
	return nil
}

func intentProgressedFromPreparation(current, prepared domain.Intent) bool {
	if validateIntent(current) != nil || current.ID != prepared.ID || current.Revision < prepared.Revision ||
		!domain.SameIntentBinding(current, prepared) ||
		current.CostEstimateID != prepared.CostEstimateID ||
		current.CostReservationID != prepared.CostReservationID ||
		current.QuotaReservationID != prepared.QuotaReservationID ||
		current.CostEstimateReceiptID != prepared.CostEstimateReceiptID ||
		current.CostReservationReceiptID != prepared.CostReservationReceiptID ||
		current.QuotaReservationReceiptID != prepared.QuotaReservationReceiptID ||
		!current.CreatedAt.Equal(prepared.CreatedAt) || current.UpdatedAt.Before(prepared.UpdatedAt) {
		return false
	}
	switch current.Status {
	case domain.IntentPrepared, domain.IntentClaimed, domain.IntentDispatching,
		domain.IntentSubmitted, domain.IntentOutcomeUnknown, domain.IntentSucceeded,
		domain.IntentFailed, domain.IntentCancelled:
		return true
	default:
		return false
	}
}

func intentProgressedFromClaim(current, claimed domain.Intent) bool {
	if !intentProgressedFromPreparation(current, claimed) ||
		!sameIntentStringPointer(current.Claimant, claimed.Claimant) ||
		!sameIntentStringPointer(current.ClaimToken, claimed.ClaimToken) ||
		!sameIntentTimePointer(current.ClaimExpiresAt, claimed.ClaimExpiresAt) ||
		current.ClaimFencingVersion != claimed.ClaimFencingVersion {
		return false
	}
	switch current.Status {
	case domain.IntentClaimed, domain.IntentDispatching, domain.IntentSubmitted,
		domain.IntentOutcomeUnknown, domain.IntentSucceeded, domain.IntentFailed:
		return true
	default:
		return false
	}
}

func sameIntentStringPointer(left, right *string) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func sameIntentTimePointer(left, right *time.Time) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && left.Equal(*right))
}

func (service *PreparationService) storeCancellationReceipt(
	ctx context.Context,
	repo PreparationRepository,
	actor Actor,
	key, inputHash string,
	value CancellationResult,
	now time.Time,
	result *CancellationResult,
) error {
	encoded, err := platformcommand.Result(cancellationReceipt{
		Intent: value.Intent, CostReservation: value.CostReservation, QuotaReservation: value.QuotaReservation,
	})
	if err != nil {
		return err
	}
	receipt, err := service.ensureReceipt(ctx, repo, actor, value.Intent.WorkspaceID, cancelIntentOperation,
		key, inputHash, value.Intent.ID, encoded, now)
	if err != nil {
		return err
	}
	value.Receipt = receipt
	*result = value
	return nil
}

func (service *PreparationService) replayCancellation(
	ctx context.Context,
	repo PreparationRepository,
	costs CostPreparationOwner,
	quotas QuotaPreparationOwner,
	actor Actor,
	receipt platformcommand.Receipt,
	inputHash string,
	result *CancellationResult,
) error {
	replayed, err := platformcommand.Replay[cancellationReceipt](receipt, inputHash)
	if err != nil || receipt.ResourceID != replayed.Intent.ID || validateIntent(replayed.Intent) != nil ||
		replayed.Intent.Status != domain.IntentCancelled {
		return platformcommand.ErrInputMismatch
	}
	persisted, err := repo.FindIntent(ctx, replayed.Intent.ID)
	if err != nil || !domain.SameIntentState(persisted, replayed.Intent) {
		return platformcommand.ErrInputMismatch
	}
	view, err := service.loadIntentView(ctx, repo, costs, quotas, actor, persisted)
	if err != nil || view.CostReservation.ID != replayed.CostReservation.ID ||
		view.QuotaReservation.ID != replayed.QuotaReservation.ID ||
		view.CostReservation.Status != replayed.CostReservation.Status ||
		view.QuotaReservation.Status != replayed.QuotaReservation.Status {
		return platformcommand.ErrInputMismatch
	}
	*result = CancellationResult{
		Intent: persisted, CostReservation: view.CostReservation,
		QuotaReservation: view.QuotaReservation, Receipt: receipt,
	}
	return nil
}

func (service *PreparationService) ensureReceipt(
	ctx context.Context,
	repo PreparationRepository,
	actor Actor,
	workspaceID, operation, key, inputHash, resourceID string,
	encoded []byte,
	now time.Time,
) (platformcommand.Receipt, error) {
	receiptID := strings.TrimSpace(service.config.NewID())
	if !validUUID(receiptID) {
		return platformcommand.Receipt{}, errors.New("generation intent receipt identifier is invalid")
	}
	return repo.EnsureReceipt(ctx, platformcommand.Receipt{
		ID: receiptID, WorkspaceID: workspaceID, Operation: operation, IdempotencyKey: key,
		InputHash: inputHash, ResourceID: resourceID, Result: encoded,
		CreatedBy: actor.UserID, CreatedAt: now,
	})
}

func authorizationForIntent(intent domain.Intent) domain.ExecutionAuthorization {
	if intent.ClaimToken == nil || intent.ClaimExpiresAt == nil {
		return domain.ExecutionAuthorization{}
	}
	return domain.ExecutionAuthorization{
		IntentID: intent.ID, ClaimToken: *intent.ClaimToken, TargetID: intent.TargetID, TargetHash: intent.TargetHash,
		CostReservationID: intent.CostReservationID, QuotaReservationID: intent.QuotaReservationID,
		ClaimFencingVersion: intent.ClaimFencingVersion, IntentRevision: intent.Revision,
		Units: intent.Units, ExpiresAt: *intent.ClaimExpiresAt,
	}
}

func preparationOwnerKey(intentID, suffix string) string {
	return "generation-intent:" + intentID + ":" + suffix
}

func intentActor(intent domain.Intent) Actor {
	return Actor{UserID: intent.CreatedBy, TokenVersion: intent.InitiatorTokenVersion}
}

func validPreparationActor(actor Actor) bool {
	return actor.TokenVersion > 0 && validUUID(actor.UserID)
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func optionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func stringPointer(value string) *string { return &value }

func timePointer(value time.Time) *time.Time { return &value }

func normalizePreparationError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, platformcommand.ErrInputMismatch) {
		return conflict("Generation intent command or binding has drifted")
	}
	if errors.Is(err, ErrIntentNotFound) {
		return notFound("Generation intent not found")
	}
	if errors.Is(err, ErrGenerationTargetNotFound) {
		return notFound("GenerationTarget not found")
	}
	var costError *costapp.Error
	if errors.As(err, &costError) {
		return &Error{Code: costError.Code, Message: costError.Message, NextAction: costError.NextAction, Status: costError.Status}
	}
	var quotaError *quotaapp.Error
	if errors.As(err, &quotaError) {
		return &Error{Code: quotaError.Code, Message: quotaError.Message, NextAction: quotaError.NextAction, Status: quotaError.Status}
	}
	return err
}

func authorizationExpired() error {
	return &Error{
		Code: "execution_authorization_expired", Message: "Generation execution authorization has expired",
		Status: 409, NextAction: "reconcile_generation_intent",
	}
}
