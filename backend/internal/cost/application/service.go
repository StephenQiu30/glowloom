package application

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

const setBudgetOperation = "cost.budget.set"

var (
	ErrBudgetNotFound = errors.New("cost budget policy not found")
	amountPattern     = regexp.MustCompile(`^(0|[1-9][0-9]{0,13})(\.[0-9]{1,6})?$`)
	currencyPattern   = regexp.MustCompile(`^[A-Z]{3}$`)
	maximumAmount     = decimal.RequireFromString("99999999999999.999999")
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

type ProjectScope struct {
	WorkspaceID, ProjectID string
}

type Repository interface {
	AuthorizeProject(context.Context, Actor, string, string) (ProjectScope, error)
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	EnsureReceipt(context.Context, platformcommand.Receipt) (platformcommand.Receipt, error)
	FindBudget(context.Context, string) (domain.BudgetPolicy, error)
	GetBudgetForUpdate(context.Context, string) (domain.BudgetPolicy, error)
	EnsureBudget(context.Context, domain.BudgetPolicy) (domain.BudgetPolicy, error)
	UpdateBudget(context.Context, domain.BudgetPolicy, int64) (domain.BudgetPolicy, error)
}

type TransactionManager interface {
	WithinCostTransaction(context.Context, func(Repository) error) error
}

type Config struct {
	Now   func() time.Time
	NewID func() string
}

type Service struct {
	transactions TransactionManager
	config       Config
}

type SetBudgetCommand struct {
	ProjectID, LimitAmount, Currency, IdempotencyKey string
	ExpectedRevision                                 int64
}

type BudgetResult struct {
	Policy  domain.BudgetPolicy
	Receipt platformcommand.Receipt
}

type budgetReceipt struct {
	Policy domain.BudgetPolicy `json:"policy"`
}

type budgetHashInput struct {
	WorkspaceID, ProjectID, LimitAmount, Currency string
	Revision                                      int64
}

func NewService(transactions TransactionManager, config Config) *Service {
	return &Service{transactions: transactions, config: config}
}

func (service *Service) SetBudget(ctx context.Context, actor Actor, command SetBudgetCommand) (BudgetResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.ProjectID = strings.TrimSpace(command.ProjectID)
	command.Currency = strings.TrimSpace(command.Currency)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	amount, amountErr := parseAmount(command.LimitAmount)
	if service == nil || service.transactions == nil || service.config.Now == nil || service.config.NewID == nil ||
		!validActor(actor) || !validUUID(command.ProjectID) || amountErr != nil ||
		!currencyPattern.MatchString(command.Currency) || command.ExpectedRevision < 0 ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return BudgetResult{}, invalid("Invalid project budget request")
	}
	command.LimitAmount = amount.StringFixed(6)
	inputHash, err := platformcommand.InputHash(struct {
		ActorID string
		Command SetBudgetCommand
	}{ActorID: actor.UserID, Command: command})
	if err != nil {
		return BudgetResult{}, err
	}
	now := service.config.Now().UTC()
	var result BudgetResult
	err = service.transactions.WithinCostTransaction(ctx, func(repo Repository) error {
		scope, authorizeErr := repo.AuthorizeProject(ctx, actor, command.ProjectID, "owner")
		if authorizeErr != nil {
			return authorizeErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, scope.WorkspaceID, setBudgetOperation, command.IdempotencyKey); findErr == nil {
			return replayBudget(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		current, findErr := repo.GetBudgetForUpdate(ctx, scope.ProjectID)
		if errors.Is(findErr, ErrBudgetNotFound) {
			if command.ExpectedRevision != 0 {
				return conflict("Project budget revision has changed")
			}
			policyID := strings.TrimSpace(service.config.NewID())
			if !validUUID(policyID) {
				return errors.New("cost budget policy identifier is invalid")
			}
			desired := domain.BudgetPolicy{
				ID: policyID, WorkspaceID: scope.WorkspaceID, ProjectID: scope.ProjectID,
				LimitAmount: amount, Currency: command.Currency, Revision: 1,
				CreatedBy: actor.UserID, UpdatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now,
			}
			desired.ContentHash, findErr = budgetContentHash(desired)
			if findErr != nil {
				return findErr
			}
			current, findErr = repo.EnsureBudget(ctx, desired)
			if findErr != nil {
				return findErr
			}
			if !domain.SameBudgetState(current, desired) {
				return platformcommand.ErrInputMismatch
			}
		} else if findErr != nil {
			return findErr
		} else {
			if validationErr := validateBudget(current); validationErr != nil {
				return validationErr
			}
			if current.WorkspaceID != scope.WorkspaceID || current.ProjectID != scope.ProjectID {
				return conflict("Project budget scope has drifted")
			}
			if current.Revision != command.ExpectedRevision {
				receipt, retryErr := repo.FindReceipt(ctx, scope.WorkspaceID, setBudgetOperation, command.IdempotencyKey)
				if retryErr == nil {
					return replayBudget(ctx, repo, receipt, inputHash, &result)
				}
				if !errors.Is(retryErr, platformcommand.ErrReceiptNotFound) {
					return retryErr
				}
				return conflict("Project budget revision has changed")
			}
			if !current.LimitAmount.Equal(amount) || current.Currency != command.Currency {
				desired := current
				desired.LimitAmount, desired.Currency = amount, command.Currency
				desired.Revision, desired.UpdatedBy, desired.UpdatedAt = current.Revision+1, actor.UserID, now
				desired.ContentHash, findErr = budgetContentHash(desired)
				if findErr != nil {
					return findErr
				}
				current, findErr = repo.UpdateBudget(ctx, desired, current.Revision)
				if findErr != nil {
					return findErr
				}
			}
		}
		return storeBudgetReceipt(ctx, repo, service.config.NewID, actor, command.IdempotencyKey, inputHash, current, now, &result)
	})
	return result, normalizeError(err)
}

func (service *Service) GetBudget(ctx context.Context, actor Actor, projectID string) (domain.BudgetPolicy, error) {
	actor.UserID, projectID = strings.TrimSpace(actor.UserID), strings.TrimSpace(projectID)
	if service == nil || service.transactions == nil || !validActor(actor) || !validUUID(projectID) {
		return domain.BudgetPolicy{}, invalid("Invalid project budget query")
	}
	var result domain.BudgetPolicy
	err := service.transactions.WithinCostTransaction(ctx, func(repo Repository) error {
		scope, authorizeErr := repo.AuthorizeProject(ctx, actor, projectID, "read")
		if authorizeErr != nil {
			return authorizeErr
		}
		policy, findErr := repo.FindBudget(ctx, scope.ProjectID)
		if findErr != nil {
			return findErr
		}
		if validationErr := validateBudget(policy); validationErr != nil {
			return validationErr
		}
		if policy.WorkspaceID != scope.WorkspaceID || policy.ProjectID != scope.ProjectID {
			return conflict("Project budget scope has drifted")
		}
		result = policy
		return nil
	})
	return result, normalizeError(err)
}

func parseAmount(raw string) (decimal.Decimal, error) {
	raw = strings.TrimSpace(raw)
	if !amountPattern.MatchString(raw) {
		return decimal.Zero, errors.New("invalid cost amount")
	}
	value, err := decimal.NewFromString(raw)
	if err != nil || value.IsNegative() || value.GreaterThan(maximumAmount) || !value.Round(6).Equal(value) {
		return decimal.Zero, errors.New("invalid cost amount")
	}
	return decimal.RequireFromString(value.StringFixed(6)), nil
}

func validateBudget(value domain.BudgetPolicy) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) || !validUUID(value.ProjectID) ||
		value.LimitAmount.IsNegative() || value.LimitAmount.GreaterThan(maximumAmount) ||
		!value.LimitAmount.Round(6).Equal(value.LimitAmount) || !currencyPattern.MatchString(value.Currency) ||
		value.Revision < 1 || len(value.ContentHash) != 64 || !validUUID(value.CreatedBy) || !validUUID(value.UpdatedBy) {
		return conflict("Project budget facts have drifted")
	}
	hash, err := budgetContentHash(value)
	if err != nil || hash != value.ContentHash {
		return conflict("Project budget facts have drifted")
	}
	return nil
}

func budgetContentHash(value domain.BudgetPolicy) (string, error) {
	return platformcommand.InputHash(budgetHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, LimitAmount: value.LimitAmount.StringFixed(6),
		Currency: value.Currency, Revision: value.Revision,
	})
}

func storeBudgetReceipt(
	ctx context.Context,
	repo Repository,
	newID func() string,
	actor Actor,
	key, inputHash string,
	policy domain.BudgetPolicy,
	now time.Time,
	result *BudgetResult,
) error {
	encoded, err := platformcommand.Result(budgetReceipt{Policy: policy})
	if err != nil {
		return err
	}
	receiptID := strings.TrimSpace(newID())
	if !validUUID(receiptID) {
		return errors.New("cost budget receipt identifier is invalid")
	}
	receipt, err := repo.EnsureReceipt(ctx, platformcommand.Receipt{
		ID: receiptID, WorkspaceID: policy.WorkspaceID, Operation: setBudgetOperation,
		IdempotencyKey: key, InputHash: inputHash, ResourceID: policy.ID,
		Result: encoded, CreatedBy: actor.UserID, CreatedAt: now,
	})
	if err != nil {
		return err
	}
	*result = BudgetResult{Policy: policy, Receipt: receipt}
	return nil
}

func replayBudget(ctx context.Context, repo Repository, receipt platformcommand.Receipt, inputHash string, result *BudgetResult) error {
	replayed, err := platformcommand.Replay[budgetReceipt](receipt, inputHash)
	if err != nil {
		return err
	}
	if receipt.ResourceID != replayed.Policy.ID || validateBudget(replayed.Policy) != nil {
		return platformcommand.ErrInputMismatch
	}
	current, err := repo.FindBudget(ctx, replayed.Policy.ProjectID)
	if err != nil || current.ID != replayed.Policy.ID || current.WorkspaceID != replayed.Policy.WorkspaceID ||
		validateBudget(current) != nil || current.Revision < replayed.Policy.Revision {
		return platformcommand.ErrInputMismatch
	}
	*result = BudgetResult{Policy: replayed.Policy, Receipt: receipt}
	return nil
}

func validActor(actor Actor) bool { return actor.TokenVersion > 0 && validUUID(actor.UserID) }

func validUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func normalizeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, platformcommand.ErrInputMismatch) {
		return conflict("Cost budget command or facts have drifted")
	}
	if errors.Is(err, ErrBudgetNotFound) {
		return notFound("Project budget is not set")
	}
	return err
}

func invalid(message string) error {
	return &Error{Code: "invalid_request", Message: message, Status: 422}
}

func conflict(message string) error {
	return &Error{Code: "state_conflict", Message: message, Status: 409}
}

func notFound(message string) error {
	return &Error{Code: "not_found", Message: message, Status: 404}
}
