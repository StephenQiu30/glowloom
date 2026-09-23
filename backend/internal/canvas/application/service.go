package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

const operationName = "canvas.document.apply"

type Actor struct {
	UserID       string
	TokenVersion int
}

type Error struct {
	Code, Message, NextAction string
	Status                    int
}

func (value *Error) Error() string { return value.Message }

type Repository interface {
	ProjectScope(context.Context, Actor, string, bool) (string, error)
	FindDocument(context.Context, string, bool) (domain.Document, bool, error)
	SaveDocument(context.Context, string, domain.Document) error
	ValidateMediaVersion(context.Context, string, string, string) error
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	CreateReceipt(context.Context, platformcommand.Receipt) error
}

type TransactionManager interface {
	WithinTransaction(context.Context, func(Repository) error) error
}

type Service struct {
	transactions TransactionManager
	now          func() time.Time
	newID        func() string
}

func NewService(transactions TransactionManager, now func() time.Time, newID func() string) *Service {
	return &Service{transactions: transactions, now: now, newID: newID}
}

type ApplyCommand struct {
	ProjectID      string             `json:"project_id"`
	Operations     []domain.Operation `json:"operations"`
	IdempotencyKey string             `json:"idempotency_key"`
}

type ApplyResult struct {
	Document        domain.Document `json:"document"`
	AppliedRevision int             `json:"applied_revision"`
	Replayed        bool            `json:"replayed"`
}

type receiptResult struct {
	Revision int `json:"revision"`
}

func (service *Service) Get(ctx context.Context, actor Actor, projectID string) (domain.Document, error) {
	var result domain.Document
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		if _, err := repo.ProjectScope(ctx, actor, projectID, false); err != nil {
			return err
		}
		loaded, exists, err := repo.FindDocument(ctx, projectID, false)
		if err != nil {
			return err
		}
		if !exists {
			result = emptyDocument(projectID)
		} else {
			result = loaded
		}
		return nil
	})
	return result, err
}

func (service *Service) Apply(ctx context.Context, actor Actor, command ApplyCommand) (ApplyResult, error) {
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.transactions == nil || service.now == nil || service.newID == nil ||
		command.ProjectID == "" || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return ApplyResult{}, invalid("Invalid canvas operation request")
	}
	var result ApplyResult
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		workspaceID, err := repo.ProjectScope(ctx, actor, command.ProjectID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		receipt, err := repo.FindReceipt(ctx, workspaceID, operationName, command.IdempotencyKey)
		if err == nil {
			original, replayErr := platformcommand.Replay[receiptResult](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was used with a different canvas operation")
			}
			if replayErr != nil {
				return replayErr
			}
			current, exists, loadErr := repo.FindDocument(ctx, command.ProjectID, false)
			if loadErr != nil {
				return loadErr
			}
			if !exists || current.Revision < original.Revision {
				return errors.New("canvas receipt has no matching document revision")
			}
			result = ApplyResult{Document: current, AppliedRevision: original.Revision, Replayed: true}
			return nil
		}
		if !errors.Is(err, platformcommand.ErrReceiptNotFound) {
			return err
		}
		current, exists, err := repo.FindDocument(ctx, command.ProjectID, true)
		if err != nil {
			return err
		}
		if !exists {
			current = emptyDocument(command.ProjectID)
		}
		next, err := domain.ApplyOperations(current, command.Operations)
		if errors.Is(err, domain.ErrInvalid) {
			return invalid("Canvas operation is invalid")
		}
		if errors.Is(err, domain.ErrConflict) {
			return conflict("Canvas node changed; reload and review the latest version")
		}
		if err != nil {
			return err
		}
		for _, operation := range command.Operations {
			if operation.Kind != "create_node" && operation.Kind != "update_node" {
				continue
			}
			if operation.Node.MediaVersionID != "" {
				if err = repo.ValidateMediaVersion(ctx, workspaceID, operation.Node.MediaVersionID, operation.Node.Kind); err != nil {
					return err
				}
			}
		}
		if err = repo.SaveDocument(ctx, workspaceID, next); err != nil {
			return err
		}
		output, err := platformcommand.Result(receiptResult{Revision: next.Revision})
		if err != nil {
			return err
		}
		if err = repo.CreateReceipt(ctx, platformcommand.Receipt{
			ID: service.newID(), WorkspaceID: workspaceID, Operation: operationName,
			IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: command.ProjectID,
			Result: output, CreatedBy: actor.UserID, CreatedAt: service.now().UTC(),
		}); err != nil {
			return err
		}
		result = ApplyResult{Document: next, AppliedRevision: next.Revision}
		return nil
	})
	return result, err
}

func emptyDocument(projectID string) domain.Document {
	return domain.Document{SchemaVersion: domain.SchemaVersion, ProjectID: projectID, Nodes: []domain.Node{}, Connections: []domain.Connection{}, Tombstones: []string{}}
}

func invalid(message string) error {
	return &Error{Code: "invalid_request", Message: message, Status: 422}
}
func conflict(message string) error {
	return &Error{Code: "resource_conflict", Message: message, Status: 409, NextAction: "reload_canvas"}
}
