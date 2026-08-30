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
	setPriceQuoteOperation  = "cost.price_quote.set"
	createEstimateOperation = "cost.estimate.create"
)

type SetPriceQuoteCommand struct {
	ProjectID, ModelProfileVersionID, ReservationUnitAmount, Currency, IdempotencyKey string
	ExpectedRevision                                                                  int64
}

type CreateEstimateCommand struct {
	ProjectID, ProviderBindingVersionID, ModelProfileVersionID string
	PriceQuoteID, Metric, SourceType, SourceID, IdempotencyKey string
	ProviderBindingRevision, ModelProfileRevision              int64
	ProviderBindingContentHash, ModelProfileContentHash        string
	PriceQuoteRevision                                         int64
	PriceQuoteContentHash                                      string
	Units                                                      int64
}

type PriceQuoteResult struct {
	Quote   domain.PriceQuote
	Receipt platformcommand.Receipt
}

type EstimateResult struct {
	Estimate domain.Estimate
	Receipt  platformcommand.Receipt
}

type priceQuoteReceipt struct {
	Quote domain.PriceQuote `json:"quote"`
}

type estimateReceipt struct {
	Estimate domain.Estimate `json:"estimate"`
}

type priceQuoteHashInput struct {
	WorkspaceID, ProjectID, ModelProfileVersionID, BillingMetric string
	ReservationUnitAmount, Currency                              string
	Revision                                                     int64
}

type estimateHashInput struct {
	WorkspaceID, ProjectID, BudgetPolicyID, PriceQuoteID    string
	ProviderBindingVersionID, ModelProfileVersionID, Metric string
	ProviderBindingContentHash, ModelProfileContentHash     string
	PriceQuoteContentHash                                   string
	SourceType, SourceID                                    string
	Units                                                   int64
	UnitAmount, TotalAmount, Currency                       string
	ProviderBindingRevision, ModelProfileRevision           int64
	PriceQuoteRevision, BudgetPolicyRevision                int64
	BudgetLimit                                             string
}

func (service *Service) SetPriceQuote(
	ctx context.Context,
	actor Actor,
	command SetPriceQuoteCommand,
) (PriceQuoteResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.ProjectID = strings.TrimSpace(command.ProjectID)
	command.ModelProfileVersionID = strings.TrimSpace(command.ModelProfileVersionID)
	command.Currency = strings.TrimSpace(command.Currency)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	unitAmount, amountErr := parseAmount(command.ReservationUnitAmount)
	if service == nil || service.transactions == nil || service.config.Now == nil || service.config.NewID == nil ||
		!validActor(actor) || !validUUID(command.ProjectID) || !validUUID(command.ModelProfileVersionID) ||
		amountErr != nil || !unitAmount.IsPositive() || !currencyPattern.MatchString(command.Currency) ||
		command.ExpectedRevision < 0 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return PriceQuoteResult{}, invalid("Invalid cost price quote request")
	}
	command.ReservationUnitAmount = unitAmount.StringFixed(6)
	inputHash, err := platformcommand.InputHash(struct {
		ActorID string
		Command SetPriceQuoteCommand
	}{ActorID: actor.UserID, Command: command})
	if err != nil {
		return PriceQuoteResult{}, err
	}
	now := service.config.Now().UTC()
	var result PriceQuoteResult
	err = service.transactions.WithinCostTransaction(ctx, func(repo Repository) error {
		scope, authorizeErr := repo.AuthorizeProject(ctx, actor, command.ProjectID, "owner")
		if authorizeErr != nil {
			return authorizeErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, scope.WorkspaceID, setPriceQuoteOperation, command.IdempotencyKey); findErr == nil {
			return replayPriceQuote(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		profile, profileErr := repo.FindModelProfileVersion(ctx, command.ModelProfileVersionID)
		if profileErr != nil {
			return profileErr
		}
		if profile.WorkspaceID != scope.WorkspaceID {
			return ErrModelProfileVersionNotFound
		}
		if validationErr := validateModelProfileVersion(profile); validationErr != nil {
			return validationErr
		}
		budget, budgetErr := repo.GetBudgetForUpdate(ctx, scope.ProjectID)
		if budgetErr != nil {
			return budgetErr
		}
		if validationErr := validateBudget(budget); validationErr != nil {
			return validationErr
		}
		if budget.WorkspaceID != scope.WorkspaceID || budget.Currency != command.Currency {
			return conflict("Price quote currency does not match project budget")
		}
		if receipt, findErr := repo.FindReceipt(ctx, scope.WorkspaceID, setPriceQuoteOperation, command.IdempotencyKey); findErr == nil {
			return replayPriceQuote(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		current, findErr := repo.GetCurrentPriceQuoteForUpdate(ctx, scope.ProjectID, profile.ID)
		if errors.Is(findErr, ErrPriceQuoteNotFound) {
			if command.ExpectedRevision != 0 {
				return conflict("Cost price quote revision has changed")
			}
			current, findErr = service.createPriceQuote(ctx, repo, actor, scope, command, unitAmount, 1, now)
			if findErr != nil {
				return findErr
			}
		} else if findErr != nil {
			return findErr
		} else {
			if validationErr := validatePriceQuote(current); validationErr != nil {
				return validationErr
			}
			if current.WorkspaceID != scope.WorkspaceID || current.ProjectID != scope.ProjectID ||
				current.Revision != command.ExpectedRevision {
				return conflict("Cost price quote revision has changed")
			}
			if current.ModelProfileVersionID != profile.ID || current.BillingMetric != profile.BillingMetric {
				return conflict("Cost price quote profile facts have drifted")
			}
			if !current.ReservationUnitAmount.Equal(unitAmount) || current.Currency != command.Currency {
				current, findErr = service.createPriceQuote(
					ctx, repo, actor, scope, command, unitAmount, current.Revision+1, now,
				)
				if findErr != nil {
					return findErr
				}
			}
		}
		return storePriceQuoteReceipt(
			ctx, repo, service.config.NewID, actor, command.IdempotencyKey, inputHash, current, now, &result,
		)
	})
	return result, normalizeError(err)
}

func (service *Service) createPriceQuote(
	ctx context.Context,
	repo Repository,
	actor Actor,
	scope ProjectScope,
	command SetPriceQuoteCommand,
	unitAmount decimal.Decimal,
	revision int64,
	now time.Time,
) (domain.PriceQuote, error) {
	quoteID := strings.TrimSpace(service.config.NewID())
	if !validUUID(quoteID) {
		return domain.PriceQuote{}, errors.New("cost price quote identifier is invalid")
	}
	desired := domain.PriceQuote{
		ID: quoteID, WorkspaceID: scope.WorkspaceID, ProjectID: scope.ProjectID,
		ModelProfileVersionID: command.ModelProfileVersionID,
		ReservationUnitAmount: unitAmount, Currency: command.Currency, Revision: revision,
		CreatedBy: actor.UserID, CreatedAt: now,
	}
	profile, err := repo.FindModelProfileVersion(ctx, command.ModelProfileVersionID)
	if err != nil {
		return domain.PriceQuote{}, err
	}
	desired.BillingMetric = profile.BillingMetric
	desired.ContentHash, err = priceQuoteContentHash(desired)
	if err != nil {
		return domain.PriceQuote{}, err
	}
	persisted, err := repo.EnsurePriceQuote(ctx, desired)
	if err != nil {
		return domain.PriceQuote{}, err
	}
	if !domain.SamePriceQuoteState(persisted, desired) {
		return domain.PriceQuote{}, platformcommand.ErrInputMismatch
	}
	return persisted, nil
}

func (service *Service) GetCurrentPriceQuote(
	ctx context.Context,
	actor Actor,
	projectID, modelProfileVersionID string,
) (domain.PriceQuote, error) {
	actor.UserID, projectID, modelProfileVersionID = strings.TrimSpace(actor.UserID), strings.TrimSpace(projectID), strings.TrimSpace(modelProfileVersionID)
	if service == nil || service.transactions == nil || !validActor(actor) || !validUUID(projectID) ||
		!validUUID(modelProfileVersionID) {
		return domain.PriceQuote{}, invalid("Invalid cost price quote query")
	}
	var result domain.PriceQuote
	err := service.transactions.WithinCostTransaction(ctx, func(repo Repository) error {
		scope, authorizeErr := repo.AuthorizeProject(ctx, actor, projectID, "read")
		if authorizeErr != nil {
			return authorizeErr
		}
		profile, findErr := repo.FindModelProfileVersion(ctx, modelProfileVersionID)
		if findErr != nil {
			return findErr
		}
		if profile.WorkspaceID != scope.WorkspaceID {
			return ErrModelProfileVersionNotFound
		}
		if validationErr := validateModelProfileVersion(profile); validationErr != nil {
			return validationErr
		}
		quote, findErr := repo.FindCurrentPriceQuote(ctx, scope.ProjectID, profile.ID)
		if findErr != nil {
			return findErr
		}
		if validationErr := validatePriceQuote(quote); validationErr != nil {
			return validationErr
		}
		if quote.WorkspaceID != scope.WorkspaceID || quote.ProjectID != scope.ProjectID ||
			quote.ModelProfileVersionID != profile.ID || quote.BillingMetric != profile.BillingMetric {
			return conflict("Cost price quote scope has drifted")
		}
		result = quote
		return nil
	})
	return result, normalizeError(err)
}

func (service *Service) CreateEstimate(
	ctx context.Context,
	actor Actor,
	command CreateEstimateCommand,
) (EstimateResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.ProjectID = strings.TrimSpace(command.ProjectID)
	command.ProviderBindingVersionID = strings.TrimSpace(command.ProviderBindingVersionID)
	command.ModelProfileVersionID = strings.TrimSpace(command.ModelProfileVersionID)
	command.PriceQuoteID = strings.TrimSpace(command.PriceQuoteID)
	command.ProviderBindingContentHash = strings.TrimSpace(command.ProviderBindingContentHash)
	command.ModelProfileContentHash = strings.TrimSpace(command.ModelProfileContentHash)
	command.PriceQuoteContentHash = strings.TrimSpace(command.PriceQuoteContentHash)
	command.Metric = strings.TrimSpace(command.Metric)
	command.SourceType = strings.TrimSpace(command.SourceType)
	command.SourceID = strings.TrimSpace(command.SourceID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.transactions == nil || service.config.Now == nil || service.config.NewID == nil ||
		!validActor(actor) || !validUUID(command.ProjectID) || !validUUID(command.ProviderBindingVersionID) ||
		!validUUID(command.ModelProfileVersionID) || !validUUID(command.PriceQuoteID) || command.PriceQuoteRevision < 1 ||
		command.ProviderBindingRevision < 1 || command.ModelProfileRevision < 1 ||
		len(command.ProviderBindingContentHash) != 64 || len(command.ModelProfileContentHash) != 64 ||
		len(command.PriceQuoteContentHash) != 64 ||
		!domain.IsBillingMetric(command.Metric) ||
		command.SourceType != domain.SourceGenerationIntent || !validUUID(command.SourceID) || command.Units < 1 ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return EstimateResult{}, invalid("Invalid cost estimate request")
	}
	inputHash, err := platformcommand.InputHash(struct {
		ActorID string
		Command CreateEstimateCommand
	}{ActorID: actor.UserID, Command: command})
	if err != nil {
		return EstimateResult{}, err
	}
	now := service.config.Now().UTC()
	var result EstimateResult
	err = service.transactions.WithinCostTransaction(ctx, func(repo Repository) error {
		scope, authorizeErr := repo.AuthorizeProject(ctx, actor, command.ProjectID, "write")
		if authorizeErr != nil {
			return authorizeErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, scope.WorkspaceID, createEstimateOperation, command.IdempotencyKey); findErr == nil {
			return replayEstimate(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		if existing, findErr := repo.FindEstimateBySource(ctx, scope.ProjectID, command.SourceType, command.SourceID); findErr == nil {
			return reuseEstimate(ctx, repo, service.config.NewID, actor, command, command.IdempotencyKey, inputHash, existing, now, &result)
		} else if !errors.Is(findErr, ErrEstimateNotFound) {
			return findErr
		}
		budget, budgetErr := repo.GetBudgetForUpdate(ctx, scope.ProjectID)
		if budgetErr != nil {
			return budgetErr
		}
		if validationErr := validateBudget(budget); validationErr != nil {
			return validationErr
		}
		if budget.WorkspaceID != scope.WorkspaceID {
			return conflict("Cost estimate budget scope has drifted")
		}
		if receipt, findErr := repo.FindReceipt(ctx, scope.WorkspaceID, createEstimateOperation, command.IdempotencyKey); findErr == nil {
			return replayEstimate(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		if existing, findErr := repo.FindEstimateBySource(ctx, scope.ProjectID, command.SourceType, command.SourceID); findErr == nil {
			return reuseEstimate(ctx, repo, service.config.NewID, actor, command, command.IdempotencyKey, inputHash, existing, now, &result)
		} else if !errors.Is(findErr, ErrEstimateNotFound) {
			return findErr
		}
		profile, quoteErr := repo.FindModelProfileVersion(ctx, command.ModelProfileVersionID)
		if quoteErr != nil {
			return quoteErr
		}
		if profile.WorkspaceID != scope.WorkspaceID {
			return ErrModelProfileVersionNotFound
		}
		if validationErr := validateModelProfileVersion(profile); validationErr != nil {
			return validationErr
		}
		if profile.BillingMetric != command.Metric ||
			profile.Revision != command.ModelProfileRevision || profile.ContentHash != command.ModelProfileContentHash {
			return conflict("Cost estimate provider profile facts have drifted")
		}
		binding, bindingErr := repo.FindProviderBindingVersion(ctx, command.ProviderBindingVersionID)
		if bindingErr != nil {
			return bindingErr
		}
		if binding.WorkspaceID != scope.WorkspaceID || binding.ProjectID != scope.ProjectID {
			return ErrProviderBindingVersionNotFound
		}
		if binding.ModelProfileVersionID != profile.ID || binding.Revision != command.ProviderBindingRevision ||
			binding.ContentHash != command.ProviderBindingContentHash {
			return conflict("Cost estimate provider binding facts have drifted")
		}
		quote, quoteErr := repo.GetCurrentPriceQuoteForUpdate(ctx, scope.ProjectID, profile.ID)
		if quoteErr != nil {
			return quoteErr
		}
		if validationErr := validatePriceQuote(quote); validationErr != nil {
			return validationErr
		}
		if quote.ID != command.PriceQuoteID || quote.WorkspaceID != scope.WorkspaceID ||
			quote.ProjectID != scope.ProjectID || quote.Currency != budget.Currency ||
			quote.ModelProfileVersionID != profile.ID || quote.BillingMetric != profile.BillingMetric ||
			quote.Revision != command.PriceQuoteRevision || quote.ContentHash != command.PriceQuoteContentHash {
			return conflict("Cost estimate price or budget scope has drifted")
		}
		totalAmount := quote.ReservationUnitAmount.Mul(decimal.NewFromInt(command.Units))
		if !totalAmount.IsPositive() || totalAmount.GreaterThan(maximumAmount) || !totalAmount.Round(6).Equal(totalAmount) {
			return conflict("Cost estimate exceeds supported amount")
		}
		estimateID := strings.TrimSpace(service.config.NewID())
		if !validUUID(estimateID) {
			return errors.New("cost estimate identifier is invalid")
		}
		desired := domain.Estimate{
			ID: estimateID, WorkspaceID: scope.WorkspaceID, ProjectID: scope.ProjectID,
			BudgetPolicyID: budget.ID, PriceQuoteID: quote.ID,
			ProviderBindingVersionID:   command.ProviderBindingVersionID,
			ProviderBindingRevision:    command.ProviderBindingRevision,
			ProviderBindingContentHash: command.ProviderBindingContentHash,
			ModelProfileVersionID:      command.ModelProfileVersionID,
			ModelProfileRevision:       command.ModelProfileRevision,
			ModelProfileContentHash:    command.ModelProfileContentHash,
			PriceQuoteContentHash:      command.PriceQuoteContentHash, Metric: command.Metric,
			SourceType: command.SourceType, SourceID: command.SourceID, Units: command.Units,
			UnitAmount: quote.ReservationUnitAmount, TotalAmount: totalAmount, Currency: quote.Currency,
			PriceQuoteRevision: quote.Revision, BudgetPolicyRevision: budget.Revision, BudgetLimit: budget.LimitAmount,
			CreatedBy: actor.UserID, CreatedAt: now,
		}
		desired.ContentHash, quoteErr = estimateContentHash(desired)
		if quoteErr != nil {
			return quoteErr
		}
		persisted, ensureErr := repo.EnsureEstimate(ctx, desired)
		if ensureErr != nil {
			return ensureErr
		}
		if !domain.SameEstimateState(persisted, desired) {
			return platformcommand.ErrInputMismatch
		}
		return storeEstimateReceipt(
			ctx, repo, service.config.NewID, actor, command.IdempotencyKey, inputHash, persisted, now, &result,
		)
	})
	return result, normalizeError(err)
}

func (service *Service) GetEstimate(ctx context.Context, actor Actor, estimateID string) (domain.Estimate, error) {
	actor.UserID, estimateID = strings.TrimSpace(actor.UserID), strings.TrimSpace(estimateID)
	if service == nil || service.transactions == nil || !validActor(actor) || !validUUID(estimateID) {
		return domain.Estimate{}, invalid("Invalid cost estimate query")
	}
	var result domain.Estimate
	err := service.transactions.WithinCostTransaction(ctx, func(repo Repository) error {
		estimate, findErr := repo.FindEstimate(ctx, estimateID)
		if findErr != nil {
			return findErr
		}
		if validationErr := validateEstimate(estimate); validationErr != nil {
			return validationErr
		}
		scope, authorizeErr := repo.AuthorizeProject(ctx, actor, estimate.ProjectID, "read")
		if authorizeErr != nil {
			return authorizeErr
		}
		if estimate.WorkspaceID != scope.WorkspaceID || estimate.ProjectID != scope.ProjectID {
			return conflict("Cost estimate scope has drifted")
		}
		result = estimate
		return nil
	})
	return result, normalizeError(err)
}

func reuseEstimate(
	ctx context.Context,
	repo Repository,
	newID func() string,
	actor Actor,
	command CreateEstimateCommand,
	key, inputHash string,
	estimate domain.Estimate,
	now time.Time,
	result *EstimateResult,
) error {
	if validateEstimate(estimate) != nil || estimate.ProjectID != command.ProjectID ||
		estimate.ProviderBindingVersionID != command.ProviderBindingVersionID ||
		estimate.ProviderBindingRevision != command.ProviderBindingRevision ||
		estimate.ProviderBindingContentHash != command.ProviderBindingContentHash ||
		estimate.ModelProfileVersionID != command.ModelProfileVersionID ||
		estimate.ModelProfileRevision != command.ModelProfileRevision ||
		estimate.ModelProfileContentHash != command.ModelProfileContentHash ||
		estimate.PriceQuoteID != command.PriceQuoteID || estimate.PriceQuoteRevision != command.PriceQuoteRevision ||
		estimate.PriceQuoteContentHash != command.PriceQuoteContentHash ||
		estimate.Metric != command.Metric ||
		estimate.SourceType != command.SourceType || estimate.SourceID != command.SourceID || estimate.Units != command.Units {
		return platformcommand.ErrInputMismatch
	}
	return storeEstimateReceipt(ctx, repo, newID, actor, key, inputHash, estimate, now, result)
}

func validatePriceQuote(value domain.PriceQuote) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) || !validUUID(value.ProjectID) ||
		!validUUID(value.ModelProfileVersionID) || !domain.IsBillingMetric(value.BillingMetric) ||
		!value.ReservationUnitAmount.IsPositive() ||
		value.ReservationUnitAmount.GreaterThan(maximumAmount) ||
		!value.ReservationUnitAmount.Round(6).Equal(value.ReservationUnitAmount) ||
		!currencyPattern.MatchString(value.Currency) || value.Revision < 1 || len(value.ContentHash) != 64 ||
		!validUUID(value.CreatedBy) {
		return conflict("Cost price quote facts have drifted")
	}
	hash, err := priceQuoteContentHash(value)
	if err != nil || hash != value.ContentHash {
		return conflict("Cost price quote facts have drifted")
	}
	return nil
}

func validateEstimate(value domain.Estimate) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) || !validUUID(value.ProjectID) ||
		!validUUID(value.BudgetPolicyID) || !validUUID(value.PriceQuoteID) ||
		!validUUID(value.ProviderBindingVersionID) || !validUUID(value.ModelProfileVersionID) ||
		value.ProviderBindingRevision < 1 || value.ModelProfileRevision < 1 ||
		len(value.ProviderBindingContentHash) != 64 || len(value.ModelProfileContentHash) != 64 ||
		len(value.PriceQuoteContentHash) != 64 ||
		!domain.IsBillingMetric(value.Metric) || value.SourceType != domain.SourceGenerationIntent ||
		!validUUID(value.SourceID) || value.Units < 1 || !value.UnitAmount.IsPositive() ||
		!value.TotalAmount.IsPositive() || value.UnitAmount.GreaterThan(maximumAmount) ||
		value.TotalAmount.GreaterThan(maximumAmount) || value.BudgetLimit.IsNegative() ||
		value.BudgetLimit.GreaterThan(maximumAmount) || !value.UnitAmount.Round(6).Equal(value.UnitAmount) ||
		!value.TotalAmount.Round(6).Equal(value.TotalAmount) || !value.BudgetLimit.Round(6).Equal(value.BudgetLimit) ||
		!value.UnitAmount.Mul(decimal.NewFromInt(value.Units)).Equal(value.TotalAmount) ||
		!currencyPattern.MatchString(value.Currency) || value.PriceQuoteRevision < 1 || value.BudgetPolicyRevision < 1 ||
		len(value.ContentHash) != 64 || !validUUID(value.CreatedBy) {
		return conflict("Cost estimate facts have drifted")
	}
	hash, err := estimateContentHash(value)
	if err != nil || hash != value.ContentHash {
		return conflict("Cost estimate facts have drifted")
	}
	return nil
}

func priceQuoteContentHash(value domain.PriceQuote) (string, error) {
	return platformcommand.InputHash(priceQuoteHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID,
		ModelProfileVersionID: value.ModelProfileVersionID, BillingMetric: value.BillingMetric,
		ReservationUnitAmount: value.ReservationUnitAmount.StringFixed(6), Currency: value.Currency, Revision: value.Revision,
	})
}

func estimateContentHash(value domain.Estimate) (string, error) {
	return platformcommand.InputHash(estimateHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID,
		BudgetPolicyID: value.BudgetPolicyID, PriceQuoteID: value.PriceQuoteID,
		ProviderBindingVersionID: value.ProviderBindingVersionID, ModelProfileVersionID: value.ModelProfileVersionID,
		ProviderBindingRevision:    value.ProviderBindingRevision,
		ProviderBindingContentHash: value.ProviderBindingContentHash,
		ModelProfileRevision:       value.ModelProfileRevision, ModelProfileContentHash: value.ModelProfileContentHash,
		PriceQuoteContentHash: value.PriceQuoteContentHash,
		Metric:                value.Metric, SourceType: value.SourceType, SourceID: value.SourceID, Units: value.Units,
		UnitAmount: value.UnitAmount.StringFixed(6), TotalAmount: value.TotalAmount.StringFixed(6), Currency: value.Currency,
		PriceQuoteRevision: value.PriceQuoteRevision, BudgetPolicyRevision: value.BudgetPolicyRevision,
		BudgetLimit: value.BudgetLimit.StringFixed(6),
	})
}

func validateModelProfileVersion(value ModelProfileVersion) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) || value.Revision < 1 ||
		value.State != "enabled" || !domain.IsBillingMetric(value.BillingMetric) || len(value.ContentHash) != 64 {
		return conflict("Provider model profile facts have drifted")
	}
	return nil
}

func storePriceQuoteReceipt(
	ctx context.Context,
	repo Repository,
	newID func() string,
	actor Actor,
	key, inputHash string,
	quote domain.PriceQuote,
	now time.Time,
	result *PriceQuoteResult,
) error {
	encoded, err := platformcommand.Result(priceQuoteReceipt{Quote: quote})
	if err != nil {
		return err
	}
	receiptID := strings.TrimSpace(newID())
	if !validUUID(receiptID) {
		return errors.New("cost price quote receipt identifier is invalid")
	}
	receipt, err := repo.EnsureReceipt(ctx, platformcommand.Receipt{
		ID: receiptID, WorkspaceID: quote.WorkspaceID, Operation: setPriceQuoteOperation,
		IdempotencyKey: key, InputHash: inputHash, ResourceID: quote.ID,
		Result: encoded, CreatedBy: actor.UserID, CreatedAt: now,
	})
	if err != nil {
		return err
	}
	*result = PriceQuoteResult{Quote: quote, Receipt: receipt}
	return nil
}

func replayPriceQuote(
	ctx context.Context,
	repo Repository,
	receipt platformcommand.Receipt,
	inputHash string,
	result *PriceQuoteResult,
) error {
	replayed, err := platformcommand.Replay[priceQuoteReceipt](receipt, inputHash)
	if err != nil {
		return err
	}
	if receipt.ResourceID != replayed.Quote.ID || validatePriceQuote(replayed.Quote) != nil {
		return platformcommand.ErrInputMismatch
	}
	persisted, err := repo.FindPriceQuote(ctx, replayed.Quote.ID)
	if err != nil || !domain.SamePriceQuoteState(persisted, replayed.Quote) || validatePriceQuote(persisted) != nil {
		return platformcommand.ErrInputMismatch
	}
	*result = PriceQuoteResult{Quote: replayed.Quote, Receipt: receipt}
	return nil
}

func storeEstimateReceipt(
	ctx context.Context,
	repo Repository,
	newID func() string,
	actor Actor,
	key, inputHash string,
	estimate domain.Estimate,
	now time.Time,
	result *EstimateResult,
) error {
	encoded, err := platformcommand.Result(estimateReceipt{Estimate: estimate})
	if err != nil {
		return err
	}
	receiptID := strings.TrimSpace(newID())
	if !validUUID(receiptID) {
		return errors.New("cost estimate receipt identifier is invalid")
	}
	receipt, err := repo.EnsureReceipt(ctx, platformcommand.Receipt{
		ID: receiptID, WorkspaceID: estimate.WorkspaceID, Operation: createEstimateOperation,
		IdempotencyKey: key, InputHash: inputHash, ResourceID: estimate.ID,
		Result: encoded, CreatedBy: actor.UserID, CreatedAt: now,
	})
	if err != nil {
		return err
	}
	*result = EstimateResult{Estimate: estimate, Receipt: receipt}
	return nil
}

func replayEstimate(
	ctx context.Context,
	repo Repository,
	receipt platformcommand.Receipt,
	inputHash string,
	result *EstimateResult,
) error {
	replayed, err := platformcommand.Replay[estimateReceipt](receipt, inputHash)
	if err != nil {
		return err
	}
	if receipt.ResourceID != replayed.Estimate.ID || validateEstimate(replayed.Estimate) != nil {
		return platformcommand.ErrInputMismatch
	}
	persisted, err := repo.FindEstimate(ctx, replayed.Estimate.ID)
	if err != nil || !domain.SameEstimateState(persisted, replayed.Estimate) || validateEstimate(persisted) != nil {
		return platformcommand.ErrInputMismatch
	}
	*result = EstimateResult{Estimate: replayed.Estimate, Receipt: receipt}
	return nil
}
