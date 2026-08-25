package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

const (
	createOperation  = "production_bible.create"
	confirmOperation = "production_bible.confirm"
	resumeOperation  = "production_bible.resume"
	decideOperation  = "production_bible.review_issue.decide"
	engineVersion    = "production-bible-agent-v1"
	promptVersion    = "production-bible-prompt-v1"
	schemaVersion    = "production-bible-schema-v1"
	harnessVersion   = "production-bible-harness-v1"
)

var ErrNotFound = errors.New("production bible not found")

type Error struct {
	Code, Message, NextAction string
	Status                    int
	Details                   map[string]any
}

func (value *Error) Error() string { return value.Message }

type Actor struct {
	UserID       string
	TokenVersion int
}

type RevisionInput struct {
	ID, WorkspaceID, ProjectID, NormalizedText, NormalizedHash string
}

type Repository interface {
	RevisionInput(context.Context, Actor, string, bool) (RevisionInput, error)
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	CreateReceipt(context.Context, platformcommand.Receipt) error
	CreateWorkflow(context.Context, domain.Bible, domain.Invocation) error
	GetBible(context.Context, Actor, string, bool) (domain.Bible, error)
	GetCurrentBible(context.Context, Actor, string) (domain.Bible, error)
	ConfirmBible(context.Context, domain.Bible) error
	UpdateReviewDecisions(context.Context, domain.Bible) error
	ResumeBible(context.Context, domain.Bible) error
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

type CreateCommand struct{ RevisionID, IdempotencyKey string }
type ConfirmCommand struct {
	BibleID, ExpectedResultHash, IdempotencyKey string
	ExpectedRevision                            int
}
type ConfirmResult struct {
	Bible   domain.Bible
	Receipt platformcommand.Receipt
}
type ResumeCommand struct {
	BibleID, IdempotencyKey string
	ExpectedRevision        int
}
type DecideReviewIssueCommand struct {
	BibleID, IssueKey, Action, IdempotencyKey string
	ExpectedRevision                          int
}

type bibleReceipt struct {
	BibleID string `json:"bible_id"`
}

func NewService(transactions TransactionManager, config Config) *Service {
	return &Service{transactions: transactions, config: config}
}

func (service *Service) Create(ctx context.Context, actor Actor, command CreateCommand) (domain.Bible, error) {
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.RevisionID == "" || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return domain.Bible{}, invalid("Invalid production bible request")
	}
	now := service.config.Now().UTC()
	var result domain.Bible
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		revision, err := repo.RevisionInput(ctx, actor, command.RevisionID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(struct{ RevisionID, NormalizedHash string }{revision.ID, revision.NormalizedHash})
		if err != nil {
			return err
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, revision.WorkspaceID, createOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[bibleReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			result, replayErr = repo.GetBible(ctx, actor, replayed.BibleID, false)
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}

		bibleID, taskID, invocationID := service.config.NewID(), service.config.NewID(), service.config.NewID()
		candidate := domain.Candidate{Entities: []domain.Entity{}, WorldEntries: []domain.WorldEntry{}, ReviewIssues: []domain.ReviewIssue{}}
		result = domain.Bible{
			ID: bibleID, WorkspaceID: revision.WorkspaceID, ProjectID: revision.ProjectID, DocumentRevisionID: revision.ID, TaskID: taskID,
			Status: "queued", InputHash: revision.NormalizedHash, EngineVersion: engineVersion, ModelName: "pending", PromptVersion: promptVersion,
			SchemaVersion: schemaVersion, HarnessVersion: harnessVersion, Candidate: candidate, ReviewDecisions: map[string]string{}, Revision: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now,
		}
		payload, err := json.Marshal(map[string]any{
			"bible_id": bibleID, "task_id": taskID, "workspace_id": revision.WorkspaceID, "project_id": revision.ProjectID,
			"document_revision_id": revision.ID, "normalized_text": revision.NormalizedText, "run_token": invocationID,
		})
		if err != nil {
			return err
		}
		invocation := domain.Invocation{ID: invocationID, WorkspaceID: revision.WorkspaceID, RequestID: bibleID, Kind: "production_bible", InputHash: revision.NormalizedHash, Payload: payload, Status: "queued", CreatedAt: now}
		if err = repo.CreateWorkflow(ctx, result, invocation); err != nil {
			return err
		}
		receiptResult, err := platformcommand.Result(bibleReceipt{BibleID: bibleID})
		if err != nil {
			return err
		}
		return repo.CreateReceipt(ctx, platformcommand.Receipt{ID: service.config.NewID(), WorkspaceID: revision.WorkspaceID, Operation: createOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: bibleID, Result: receiptResult, CreatedBy: actor.UserID, CreatedAt: now})
	})
	return result, normalizeError(err)
}

func (service *Service) Get(ctx context.Context, actor Actor, bibleID string) (domain.Bible, error) {
	var result domain.Bible
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		result, err = repo.GetBible(ctx, actor, bibleID, false)
		return err
	})
	return result, normalizeError(err)
}

func (service *Service) GetCurrent(ctx context.Context, actor Actor, projectID string) (domain.Bible, error) {
	var result domain.Bible
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		result, err = repo.GetCurrentBible(ctx, actor, projectID)
		return err
	})
	return result, normalizeError(err)
}

func (service *Service) Confirm(ctx context.Context, actor Actor, command ConfirmCommand) (ConfirmResult, error) {
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.ExpectedRevision < 1 || len(command.ExpectedResultHash) != 64 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return ConfirmResult{}, invalid("Invalid production bible confirmation")
	}
	var result ConfirmResult
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		bible, err := repo.GetBible(ctx, actor, command.BibleID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, bible.WorkspaceID, confirmOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[bibleReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			result.Bible, replayErr = repo.GetBible(ctx, actor, replayed.BibleID, false)
			result.Receipt = receipt
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if bible.Status != "needs_review" || bible.ResultHash == nil || *bible.ResultHash != command.ExpectedResultHash || bible.Revision != command.ExpectedRevision {
			return conflict("Production bible changed before confirmation")
		}
		for _, issue := range bible.Candidate.ReviewIssues {
			if issue.Severity == "blocking" && bible.ReviewDecisions[issue.IssueKey] != "accepted" {
				return conflict("Blocking review issues must be resolved before confirmation")
			}
		}
		now := service.config.Now().UTC()
		bible.Status, bible.ConfirmedAt, bible.ConfirmedBy, bible.UpdatedAt = "confirmed", &now, &actor.UserID, now
		bible.Revision++
		if err = repo.ConfirmBible(ctx, bible); err != nil {
			return err
		}
		receiptResult, err := platformcommand.Result(bibleReceipt{BibleID: bible.ID})
		if err != nil {
			return err
		}
		receipt := platformcommand.Receipt{ID: service.config.NewID(), WorkspaceID: bible.WorkspaceID, Operation: confirmOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: bible.ID, Result: receiptResult, CreatedBy: actor.UserID, CreatedAt: now}
		if err = repo.CreateReceipt(ctx, receipt); err != nil {
			return err
		}
		result = ConfirmResult{Bible: bible, Receipt: receipt}
		return nil
	})
	return result, normalizeError(err)
}

func (service *Service) DecideReviewIssue(ctx context.Context, actor Actor, command DecideReviewIssueCommand) (domain.Bible, error) {
	command.IssueKey = strings.TrimSpace(command.IssueKey)
	command.Action = strings.TrimSpace(command.Action)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.BibleID == "" || command.IssueKey == "" || !oneOf(command.Action, "accepted", "rejected") || command.ExpectedRevision < 1 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return domain.Bible{}, invalid("Invalid production bible review decision")
	}
	var result domain.Bible
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		bible, err := repo.GetBible(ctx, actor, command.BibleID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, bible.WorkspaceID, decideOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[bibleReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			result, replayErr = repo.GetBible(ctx, actor, replayed.BibleID, false)
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if bible.Status != "needs_review" || bible.Revision != command.ExpectedRevision {
			return conflict("Production bible changed before review decision")
		}
		found := false
		for _, issue := range bible.Candidate.ReviewIssues {
			if issue.IssueKey == command.IssueKey {
				found = true
				break
			}
		}
		if !found {
			return invalid("Production bible review issue does not exist")
		}
		if bible.ReviewDecisions == nil {
			bible.ReviewDecisions = map[string]string{}
		}
		bible.ReviewDecisions[command.IssueKey] = command.Action
		now := service.config.Now().UTC()
		bible.Revision++
		bible.UpdatedAt = now
		if err = repo.UpdateReviewDecisions(ctx, bible); err != nil {
			return err
		}
		receiptResult, err := platformcommand.Result(bibleReceipt{BibleID: bible.ID})
		if err != nil {
			return err
		}
		if err = repo.CreateReceipt(ctx, platformcommand.Receipt{ID: service.config.NewID(), WorkspaceID: bible.WorkspaceID, Operation: decideOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: bible.ID, Result: receiptResult, CreatedBy: actor.UserID, CreatedAt: now}); err != nil {
			return err
		}
		result = bible
		return nil
	})
	return result, normalizeError(err)
}

func (service *Service) Resume(ctx context.Context, actor Actor, command ResumeCommand) (domain.Bible, error) {
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.ExpectedRevision < 1 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return domain.Bible{}, invalid("Invalid production bible resume request")
	}
	var result domain.Bible
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		bible, err := repo.GetBible(ctx, actor, command.BibleID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, bible.WorkspaceID, resumeOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[bibleReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			result, replayErr = repo.GetBible(ctx, actor, replayed.BibleID, false)
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if !oneOf(bible.Status, "failed", "unknown", "cancelled") || bible.Revision != command.ExpectedRevision {
			return conflict("Production bible cannot be resumed from its current state")
		}
		now := service.config.Now().UTC()
		bible.Status, bible.UpdatedAt = "queued", now
		bible.Error, bible.ResultHash = nil, nil
		bible.Revision++
		if err = repo.ResumeBible(ctx, bible); err != nil {
			return err
		}
		receiptResult, err := platformcommand.Result(bibleReceipt{BibleID: bible.ID})
		if err != nil {
			return err
		}
		if err = repo.CreateReceipt(ctx, platformcommand.Receipt{ID: service.config.NewID(), WorkspaceID: bible.WorkspaceID, Operation: resumeOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: bible.ID, Result: receiptResult, CreatedBy: actor.UserID, CreatedAt: now}); err != nil {
			return err
		}
		result = bible
		return nil
	})
	return result, normalizeError(err)
}

func invalid(message string) error {
	return &Error{Code: "validation_failed", Message: message, Status: 422}
}
func conflict(message string) error {
	return &Error{Code: "resource_conflict", Message: message, Status: 409}
}
func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
func normalizeError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return &Error{Code: "not_found", Message: "Production bible not found", Status: 404}
	}
	return err
}
