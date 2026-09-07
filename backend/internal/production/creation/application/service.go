package application

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
	"github.com/google/uuid"
)

var ErrNotFound = errors.New("creation run not found")
var ErrNoDelivery = errors.New("no creation delivery ready")
var ErrLeaseLost = errors.New("creation delivery lease lost")

type Error struct {
	Code   string
	Status int
}

func (e *Error) Error() string              { return e.Code }
func Problem(code string, status int) error { return &Error{Code: code, Status: status} }

type Actor struct {
	UserID       string
	TokenVersion int
}
type CreateCommand struct {
	ProjectID          string
	DocumentRevisionID string `json:"document_revision_id"`
	SourceHash         string `json:"source_hash"`
	IdempotencyKey     string `json:"idempotency_key"`
}
type Repository interface {
	Authorize(context.Context, Actor, string, bool) (string, error)
	Source(context.Context, string, string) (domain.Source, error)
	Find(context.Context, Actor, string, string) (domain.Run, error)
	Get(context.Context, string) (domain.Run, error)
	Create(context.Context, domain.Run) error
	List(context.Context, string, int) ([]domain.Run, error)
	Retry(context.Context, string, int64, int, time.Time) error
}
type Transactions interface {
	WithinTransaction(context.Context, func(Repository) error) error
}
type Config struct {
	Endpoint string
	Now      func() time.Time
	NewID    func() string
}
type Service struct {
	transactions Transactions
	config       Config
}

func NewService(transactions Transactions, config Config) *Service {
	return &Service{transactions: transactions, config: config}
}

func (service *Service) Create(ctx context.Context, actor Actor, input CreateCommand) (domain.Run, error) {
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if !validID(input.ProjectID) || !validID(input.DocumentRevisionID) || !validID(actor.UserID) || actor.TokenVersion < 1 || !validHash(input.SourceHash) || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 200 {
		return domain.Run{}, Problem("validation_failed", 422)
	}
	inputHash, err := command.InputHash(input)
	if err != nil {
		return domain.Run{}, err
	}
	var result domain.Run
	err = service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		workspaceID, err := repo.Authorize(ctx, actor, input.ProjectID, true)
		if err != nil {
			return err
		}
		found, err := repo.Find(ctx, actor, input.ProjectID, input.IdempotencyKey)
		if err == nil {
			if found.InputHash != inputHash {
				return Problem("idempotency_conflict", 409)
			}
			result = found
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		if service.config.Endpoint == "" {
			return Problem("creation_unavailable", 503)
		}
		source, err := repo.Source(ctx, input.ProjectID, input.DocumentRevisionID)
		if err != nil {
			return err
		}
		if source.ContentHash != input.SourceHash {
			return Problem("source_hash_drift", 409)
		}
		now := service.config.Now().UTC()
		id := service.config.NewID()
		envelope := domain.Command{Schema: "creation-command-production", CommandID: id, RunID: id, WorkspaceID: workspaceID, ProjectID: input.ProjectID, ActorID: actor.UserID, Source: source, FlowType: domain.FlowType, WorkflowID: "lanverse:creation:" + id}
		payloadHash, err := PayloadHash(envelope)
		if err != nil {
			return err
		}
		result = domain.Run{Command: envelope, InputHash: inputHash, PayloadHash: payloadHash, IdempotencyKey: input.IdempotencyKey, Endpoint: service.config.Endpoint, TokenVersion: actor.TokenVersion, Status: domain.Queued, Revision: 1, CreatedAt: now, UpdatedAt: now}
		return repo.Create(ctx, result)
	})
	return result, err
}
func (service *Service) Get(ctx context.Context, actor Actor, id string) (domain.Run, error) {
	if !validID(id) {
		return domain.Run{}, Problem("validation_failed", 422)
	}
	var result domain.Run
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		run, err := repo.Get(ctx, id)
		if err != nil {
			return err
		}
		if _, err = repo.Authorize(ctx, actor, run.Command.ProjectID, false); err != nil {
			return err
		}
		result = run
		return nil
	})
	return result, err
}
func (service *Service) List(ctx context.Context, actor Actor, projectID string, limit int) ([]domain.Run, error) {
	if !validID(projectID) || limit < 1 || limit > 100 {
		return nil, Problem("validation_failed", 422)
	}
	var result []domain.Run
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		if _, err := repo.Authorize(ctx, actor, projectID, false); err != nil {
			return err
		}
		var err error
		result, err = repo.List(ctx, projectID, limit)
		return err
	})
	return result, err
}
func (service *Service) Retry(ctx context.Context, actor Actor, id string, revision int64) (domain.Run, error) {
	if !validID(id) || revision < 1 {
		return domain.Run{}, Problem("validation_failed", 422)
	}
	var result domain.Run
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		run, err := repo.Get(ctx, id)
		if err != nil {
			return err
		}
		if _, err = repo.Authorize(ctx, actor, run.Command.ProjectID, true); err != nil {
			return err
		}
		if actor.UserID != run.Command.ActorID {
			return Problem("forbidden", 403)
		}
		if run.Revision != revision {
			return Problem("revision_conflict", 409)
		}
		if run.Status != domain.Blocked {
			return Problem("delivery_not_blocked", 409)
		}
		if err = repo.Retry(ctx, id, revision, actor.TokenVersion, service.config.Now().UTC()); err != nil {
			return err
		}
		result, err = repo.Get(ctx, id)
		return err
	})
	return result, err
}

func PayloadHash(value domain.Command) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return canonical.Hash(raw)
}
func ValidateAcceptance(run domain.Run, receipt domain.Acceptance) error {
	if receipt.Schema != "creation-acceptance-production" || receipt.CommandID != run.Command.CommandID || receipt.RunID != run.Command.RunID || receipt.PayloadHash != run.PayloadHash || receipt.FlowType != run.Command.FlowType || receipt.WorkflowID != run.Command.WorkflowID || !validID(receipt.ReceiptID) || receipt.AcceptedAt.IsZero() {
		return Problem("agent_receipt_mismatch", 502)
	}
	return nil
}
func validID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed != uuid.Nil && parsed.String() == value
}
func validHash(value string) bool {
	_, err := hex.DecodeString(value)
	return len(value) == 64 && err == nil && strings.ToLower(value) == value
}
