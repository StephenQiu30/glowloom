package application

import (
	"context"
	"errors"
	"strings"
	"time"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const compileOperation = "workflow_definition.compile"

var ErrNotFound = errors.New("workflow resource not found")

type Error struct {
	Code, Message, NextAction string
	Status                    int
}

func (value *Error) Error() string { return value.Message }

type Actor struct {
	UserID       string
	TokenVersion int
}

type RevisionSource interface {
	Resolve(context.Context, Actor, string) (domain.CompilationSource, error)
}

type Repository interface {
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	CreateReceipt(context.Context, platformcommand.Receipt) error
	EnsureCompilation(context.Context, domain.CompiledFacts) (domain.CompiledFacts, error)
	GetCompilation(context.Context, string, string) (domain.CompiledFacts, error)
	PrepareStart(context.Context, domain.StartPreparation) (domain.StartPreparation, error)
	BeginStartAttempt(context.Context, string, time.Time) (domain.StartPreparation, error)
	FinalizeStartAttempt(context.Context, domain.WorkflowRun, domain.StartIntent, domain.StartReceipt, int, int) error
}

type TransactionManager interface {
	WithinTransaction(context.Context, func(Repository) error) error
}

type Config struct {
	Now   func() time.Time
	NewID func() string
}

type Service struct {
	source       RevisionSource
	transactions TransactionManager
	contract     domain.CompilerContract
	config       Config
}

type CompileCommand struct {
	AuthoringRevisionID string
	IdempotencyKey      string
}

type compilationReceipt struct {
	DefinitionID       string `json:"definition_id"`
	RunInputSnapshotID string `json:"run_input_snapshot_id"`
}

func NewService(source RevisionSource, transactions TransactionManager, contract domain.CompilerContract, config Config) *Service {
	return &Service{source: source, transactions: transactions, contract: contract, config: config}
}

func (service *Service) Compile(ctx context.Context, actor Actor, command CompileCommand) (domain.CompiledFacts, error) {
	command.AuthoringRevisionID = strings.TrimSpace(command.AuthoringRevisionID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.AuthoringRevisionID == "" || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 ||
		service.source == nil || service.transactions == nil || service.config.Now == nil || service.config.NewID == nil {
		return domain.CompiledFacts{}, invalid("Invalid workflow compilation request")
	}
	source, err := service.source.Resolve(ctx, actor, command.AuthoringRevisionID)
	if err != nil {
		return domain.CompiledFacts{}, normalizeError(err)
	}
	compilation, err := domain.Compile(source, service.contract)
	if err != nil {
		return domain.CompiledFacts{}, invalid(err.Error())
	}
	var result domain.CompiledFacts
	err = service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		inputHash, hashErr := platformcommand.InputHash(command)
		if hashErr != nil {
			return hashErr
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, source.Revision.WorkspaceID, compileOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[compilationReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			result, replayErr = repo.GetCompilation(ctx, replayed.DefinitionID, replayed.RunInputSnapshotID)
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}

		now := service.config.Now().UTC()
		desired := domain.CompiledFacts{
			DefinitionID: service.config.NewID(), RunInputSnapshotID: service.config.NewID(),
			Compilation: compilation, CreatedBy: actor.UserID, CreatedAt: now,
		}
		var ensureErr error
		result, ensureErr = repo.EnsureCompilation(ctx, desired)
		if ensureErr != nil {
			return ensureErr
		}
		encoded, err := platformcommand.Result(compilationReceipt{
			DefinitionID: result.DefinitionID, RunInputSnapshotID: result.RunInputSnapshotID,
		})
		if err != nil {
			return err
		}
		return repo.CreateReceipt(ctx, platformcommand.Receipt{
			ID: service.config.NewID(), WorkspaceID: source.Revision.WorkspaceID, Operation: compileOperation,
			IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: result.DefinitionID,
			Result: encoded, CreatedBy: actor.UserID, CreatedAt: now,
		})
	})
	return result, normalizeError(err)
}

func invalid(message string) error {
	return &Error{Code: "validation_failed", Message: message, Status: 422}
}

func conflict(message string) error {
	return &Error{Code: "resource_conflict", Message: message, Status: 409}
}

func normalizeError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return &Error{Code: "not_found", Message: "Workflow resource not found", Status: 404}
	}
	return err
}
