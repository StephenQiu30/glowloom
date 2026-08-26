package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

const (
	createOperation  = "authoring_draft.create"
	updateOperation  = "authoring_draft.update"
	publishOperation = "authoring_revision.publish"
)

var ErrNotFound = errors.New("authoring resource not found")

type Error struct {
	Code, Message, NextAction string
	Status                    int
}

func (value *Error) Error() string { return value.Message }

type Actor struct {
	UserID       string
	TokenVersion int
}

type Repository interface {
	ProjectScope(context.Context, Actor, string, bool) (string, error)
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	CreateReceipt(context.Context, platformcommand.Receipt) error
	Catalog(context.Context, string, string) (string, domain.Catalog, error)
	VerifyFrozenInputs(context.Context, string, []domain.FrozenReference) error
	CreateDraft(context.Context, domain.Draft) error
	GetDraft(context.Context, Actor, string, bool) (domain.Draft, error)
	UpdateDraft(context.Context, domain.Draft, int) error
	CreateRevision(context.Context, domain.Draft, domain.Revision) error
	GetRevision(context.Context, Actor, string) (domain.Revision, error)
	AuthorizeRevisionExecution(context.Context, Actor, string) error
}

type TransactionManager interface {
	WithinTransaction(context.Context, func(Repository) error) error
}

type Config struct {
	Now   func() time.Time
	NewID func() string
}

type Service struct {
	transactions TransactionManager
	config       Config
}

type CreateCommand struct {
	ProjectID, AuthoringMode   string
	Graph                      domain.Graph
	Layout                     json.RawMessage
	FrozenInputs               []domain.FrozenReference
	CatalogKey, CatalogVersion string
	IdempotencyKey             string
}

type UpdateCommand struct {
	DraftID                    string
	ExpectedRevision           int
	Graph                      domain.Graph
	Layout                     json.RawMessage
	FrozenInputs               []domain.FrozenReference
	CatalogKey, CatalogVersion string
	IdempotencyKey             string
}

type PublishCommand struct {
	DraftID, IdempotencyKey string
	ExpectedRevision        int
}

type resourceReceipt struct {
	ResourceID string `json:"resource_id"`
}

func NewService(transactions TransactionManager, config Config) *Service {
	return &Service{transactions: transactions, config: config}
}

func (service *Service) Create(ctx context.Context, actor Actor, command CreateCommand) (domain.Draft, error) {
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.ProjectID == "" || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 || service.config.Now == nil || service.config.NewID == nil {
		return domain.Draft{}, invalid("Invalid authoring draft request")
	}
	var result domain.Draft
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		workspaceID, err := repo.ProjectScope(ctx, actor, command.ProjectID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, workspaceID, createOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[resourceReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			result, replayErr = repo.GetDraft(ctx, actor, replayed.ResourceID, false)
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		catalogID, catalog, err := repo.Catalog(ctx, command.CatalogKey, command.CatalogVersion)
		if err != nil {
			return err
		}
		if err = repo.VerifyFrozenInputs(ctx, command.ProjectID, command.FrozenInputs); err != nil {
			return err
		}
		snapshot, err := domain.PublishSnapshot(domain.DraftSnapshot{
			AuthoringMode: command.AuthoringMode, Graph: command.Graph, Layout: command.Layout, FrozenInputs: command.FrozenInputs,
		}, catalog)
		if err != nil {
			return invalid(err.Error())
		}
		now := service.config.Now().UTC()
		result = domain.Draft{
			ID: service.config.NewID(), WorkspaceID: workspaceID, ProjectID: command.ProjectID, CatalogID: catalogID,
			AuthoringMode: snapshot.AuthoringMode, Graph: snapshot.Graph, Layout: snapshot.Layout,
			FrozenInputs: snapshot.FrozenInputs, CatalogKey: catalog.Key, CatalogVersion: catalog.Version,
			Status: "active", Revision: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now,
		}
		if err = repo.CreateDraft(ctx, result); err != nil {
			return err
		}
		return createResourceReceipt(ctx, repo, service.config.NewID(), workspaceID, createOperation, command.IdempotencyKey, inputHash, result.ID, actor.UserID, now)
	})
	return result, normalizeError(err)
}

func (service *Service) Update(ctx context.Context, actor Actor, command UpdateCommand) (domain.Draft, error) {
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.DraftID == "" || command.ExpectedRevision < 1 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return domain.Draft{}, invalid("Invalid authoring draft update")
	}
	var result domain.Draft
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		draft, err := repo.GetDraft(ctx, actor, command.DraftID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, draft.WorkspaceID, updateOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[resourceReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			result, replayErr = repo.GetDraft(ctx, actor, replayed.ResourceID, false)
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if draft.Status != "active" || draft.Revision != command.ExpectedRevision {
			return conflict("Authoring draft changed before update")
		}
		catalogID, catalog, err := repo.Catalog(ctx, command.CatalogKey, command.CatalogVersion)
		if err != nil {
			return err
		}
		if err = repo.VerifyFrozenInputs(ctx, draft.ProjectID, command.FrozenInputs); err != nil {
			return err
		}
		snapshot, err := domain.PublishSnapshot(domain.DraftSnapshot{
			AuthoringMode: draft.AuthoringMode, Graph: command.Graph, Layout: command.Layout, FrozenInputs: command.FrozenInputs,
		}, catalog)
		if err != nil {
			return invalid(err.Error())
		}
		previousRevision := draft.Revision
		draft.CatalogID, draft.CatalogKey, draft.CatalogVersion = catalogID, catalog.Key, catalog.Version
		draft.Graph, draft.Layout, draft.FrozenInputs = snapshot.Graph, snapshot.Layout, snapshot.FrozenInputs
		draft.Revision++
		draft.UpdatedAt = service.config.Now().UTC()
		if err = repo.UpdateDraft(ctx, draft, previousRevision); err != nil {
			return err
		}
		if err = createResourceReceipt(ctx, repo, service.config.NewID(), draft.WorkspaceID, updateOperation, command.IdempotencyKey, inputHash, draft.ID, actor.UserID, draft.UpdatedAt); err != nil {
			return err
		}
		result = draft
		return nil
	})
	return result, normalizeError(err)
}

func (service *Service) Publish(ctx context.Context, actor Actor, command PublishCommand) (domain.Revision, error) {
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.DraftID == "" || command.ExpectedRevision < 1 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return domain.Revision{}, invalid("Invalid authoring publish request")
	}
	var result domain.Revision
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		draft, err := repo.GetDraft(ctx, actor, command.DraftID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, draft.WorkspaceID, publishOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[resourceReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			result, replayErr = repo.GetRevision(ctx, actor, replayed.ResourceID)
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if draft.Status != "active" || draft.Revision != command.ExpectedRevision {
			return conflict("Authoring draft changed before publish")
		}
		if draft.CurrentPublishedRevisionID != nil {
			current, currentErr := repo.GetRevision(ctx, actor, *draft.CurrentPublishedRevisionID)
			if currentErr != nil {
				return currentErr
			}
			if current.RevisionNo == draft.Revision {
				return conflict("Authoring draft revision is already published")
			}
		}
		catalogID, catalog, err := repo.Catalog(ctx, draft.CatalogKey, draft.CatalogVersion)
		if err != nil {
			return err
		}
		if catalogID != draft.CatalogID {
			return conflict("Authoring catalog binding changed")
		}
		if err = repo.VerifyFrozenInputs(ctx, draft.ProjectID, draft.FrozenInputs); err != nil {
			return err
		}
		snapshot, err := domain.PublishSnapshot(domain.DraftSnapshot{
			AuthoringMode: draft.AuthoringMode, Graph: draft.Graph, Layout: draft.Layout, FrozenInputs: draft.FrozenInputs,
		}, catalog)
		if err != nil {
			return invalid(err.Error())
		}
		now := service.config.Now().UTC()
		result = domain.Revision{
			ID: service.config.NewID(), WorkspaceID: draft.WorkspaceID, ProjectID: draft.ProjectID,
			DraftID: draft.ID, CatalogID: catalogID, RevisionNo: draft.Revision,
			RevisionSnapshot: snapshot, CreatedBy: actor.UserID, CreatedAt: now,
		}
		if err = repo.CreateRevision(ctx, draft, result); err != nil {
			return err
		}
		return createResourceReceipt(ctx, repo, service.config.NewID(), draft.WorkspaceID, publishOperation, command.IdempotencyKey, inputHash, result.ID, actor.UserID, now)
	})
	return result, normalizeError(err)
}

func (service *Service) CompilationSource(ctx context.Context, actor Actor, revisionID string) (domain.Revision, domain.Catalog, error) {
	if strings.TrimSpace(revisionID) == "" {
		return domain.Revision{}, domain.Catalog{}, invalid("Invalid authoring revision request")
	}
	var revision domain.Revision
	var catalog domain.Catalog
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		if err = repo.AuthorizeRevisionExecution(ctx, actor, revisionID); err != nil {
			return err
		}
		revision, err = repo.GetRevision(ctx, actor, revisionID)
		if err != nil {
			return err
		}
		catalogID, resolved, err := repo.Catalog(ctx, revision.CatalogKey, revision.CatalogVersion)
		if err != nil {
			return err
		}
		if catalogID != revision.CatalogID || resolved.ContentHash != revision.CatalogHash ||
			resolved.ExecutionHash != revision.CatalogExecutionHash {
			return conflict("Authoring revision catalog binding changed")
		}
		if err = repo.VerifyFrozenInputs(ctx, revision.ProjectID, revision.FrozenInputs); err != nil {
			return err
		}
		catalog = resolved
		return nil
	})
	return revision, catalog, normalizeError(err)
}

func createResourceReceipt(
	ctx context.Context,
	repo Repository,
	id, workspaceID, operation, idempotencyKey, inputHash, resourceID, createdBy string,
	now time.Time,
) error {
	encoded, err := platformcommand.Result(resourceReceipt{ResourceID: resourceID})
	if err != nil {
		return err
	}
	return repo.CreateReceipt(ctx, platformcommand.Receipt{
		ID: id, WorkspaceID: workspaceID, Operation: operation, IdempotencyKey: idempotencyKey,
		InputHash: inputHash, ResourceID: resourceID, Result: encoded, CreatedBy: createdBy, CreatedAt: now,
	})
}

func invalid(message string) error {
	return &Error{Code: "validation_failed", Message: message, Status: 422}
}

func conflict(message string) error {
	return &Error{Code: "resource_conflict", Message: message, Status: 409}
}

func normalizeError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return &Error{Code: "not_found", Message: "Authoring resource not found", Status: 404}
	}
	return err
}
