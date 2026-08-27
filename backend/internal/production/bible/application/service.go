package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

const (
	createOperation  = "production_bible.create"
	confirmOperation = "production_bible.confirm"
	resumeOperation  = "production_bible.resume"
	decideOperation  = "production_bible.review_issue.decide"
	engineVersion    = "storygraph-stage-agent-v1"
	promptVersion    = "build-storygraph-prompt-v1"
	schemaVersion    = "storygraph-candidate-schema-v1"
	harnessVersion   = "storygraph-stage-harness-v1"
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
	VersionNo                                                  int
}

type Repository interface {
	RevisionInput(context.Context, Actor, string, bool) (RevisionInput, error)
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	CreateReceipt(context.Context, platformcommand.Receipt) error
	CreateWorkflow(context.Context, domain.Bible, domain.Invocation) error
	GetBible(context.Context, Actor, string, bool) (domain.Bible, error)
	GetCurrentBible(context.Context, Actor, string) (domain.Bible, error)
	CandidateConfirmation(context.Context, Actor, ConfirmCommand, bool) (CandidateConfirmation, error)
	GetBibleVersion(context.Context, Actor, string) (domain.ProductionBibleVersion, error)
	CreateBibleVersion(context.Context, domain.ProductionBibleVersion) error
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
	CandidateRevisionID, CandidateRevisionHash string
	DocumentRevisionID, DocumentRevisionHash   string
	ReviewDecisionID, IdempotencyKey           string
	ExpectedCandidateRevision                  int64
	ExpectedVersion                            int
}
type ConfirmResult struct {
	Version domain.ProductionBibleVersion
	Receipt platformcommand.Receipt
}

type CandidateConfirmation struct {
	WorkspaceID, ProjectID                     string
	DocumentRevisionID, DocumentRevisionHash   string
	CandidateRevisionID, CandidateRevisionHash string
	CandidateContentHash                       string
	CandidateRevisionNo                        int64
	Snapshot                                   json.RawMessage
	NextVersion                                int
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

type bibleVersionReceipt struct {
	VersionID string `json:"version_id"`
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
		stageInput, err := json.Marshal(map[string]any{
			"bible_id": bibleID, "task_id": taskID, "workspace_id": revision.WorkspaceID, "project_id": revision.ProjectID,
			"document_revision_id": revision.ID, "normalized_text": revision.NormalizedText, "run_token": invocationID,
		})
		if err != nil {
			return err
		}
		shardEnd := utf8.RuneCountInString(revision.NormalizedText)
		shardKey := fmt.Sprintf("script:%d:%d", 0, shardEnd)
		manifestMaterial, err := json.Marshal(struct {
			Stage, ShardKey, SourceHash string
			AbsoluteStart, AbsoluteEnd  int
		}{"analyze_story", shardKey, revision.NormalizedHash, 0, shardEnd})
		if err != nil {
			return err
		}
		manifestHash, err := agentcontract.CanonicalHash(manifestMaterial)
		if err != nil {
			return err
		}
		start := 0
		stageInvocation, err := agentcontract.NewStageInvocation(
			invocationID,
			agentcontract.StoryGraphDefinition().ExecutionPolicy(),
			agentcontract.StageInvocationPayload{
				Stage: "analyze_story", ShardKey: shardKey, WorkspaceID: revision.WorkspaceID,
				ProjectID: revision.ProjectID,
				SourceRefs: []agentcontract.StageSourceRef{{
					OwnerKind: "script_revision", OwnerLogicalID: revision.ID,
					OwnerVersionID: revision.ID, Revision: int64(revision.VersionNo), ContentHash: revision.NormalizedHash,
				}},
				UpstreamCandidates: []agentcontract.StageUpstreamCandidateRef{},
				ShardManifestRef:   agentcontract.ShardManifestRef{ManifestID: taskID, Version: 1, Hash: manifestHash},
				Shard:              agentcontract.InvocationShard{Kind: "script_slice", Key: shardKey, TreePath: "0", AbsoluteStart: &start, AbsoluteEnd: &shardEnd},
				StageInput:         stageInput,
			},
		)
		if err != nil {
			return err
		}
		encodedPolicy, err := json.Marshal(stageInvocation.ExecutionPolicy)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(stageInvocation.Payload)
		if err != nil {
			return err
		}
		stageInstanceKey, err := stageInvocation.StageInstanceKey()
		if err != nil {
			return err
		}
		invocation := domain.Invocation{
			ID: invocationID, WorkspaceID: revision.WorkspaceID, RequestID: bibleID,
			Kind: "storygraph_stage", Stage: stageInvocation.Payload.Stage, ShardKey: shardKey,
			InputHash: stageInvocation.InputHash, StageInstanceKey: stageInstanceKey, ManifestHash: manifestHash,
			ExecutionPolicy: encodedPolicy, Payload: payload, Status: "queued", CreatedAt: now,
		}
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
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.CandidateRevisionID = strings.TrimSpace(command.CandidateRevisionID)
	command.CandidateRevisionHash = strings.ToLower(strings.TrimSpace(command.CandidateRevisionHash))
	command.DocumentRevisionID = strings.TrimSpace(command.DocumentRevisionID)
	command.DocumentRevisionHash = strings.ToLower(strings.TrimSpace(command.DocumentRevisionHash))
	command.ReviewDecisionID = strings.TrimSpace(command.ReviewDecisionID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.transactions == nil || service.config.Now == nil || service.config.NewID == nil ||
		actor.UserID == "" || actor.TokenVersion < 1 || command.ExpectedCandidateRevision < 1 ||
		command.ExpectedVersion < 1 || len(command.CandidateRevisionHash) != 64 ||
		len(command.DocumentRevisionHash) != 64 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return ConfirmResult{}, invalid("Invalid production bible confirmation")
	}
	for _, identifier := range []string{
		actor.UserID, command.CandidateRevisionID, command.DocumentRevisionID, command.ReviewDecisionID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return ConfirmResult{}, invalid("Invalid production bible confirmation")
		}
	}
	var result ConfirmResult
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		scope, err := repo.CandidateConfirmation(ctx, actor, command, false)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, scope.WorkspaceID, confirmOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[bibleVersionReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			result.Version, replayErr = repo.GetBibleVersion(ctx, actor, replayed.VersionID)
			result.Receipt = receipt
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		locked, err := repo.CandidateConfirmation(ctx, actor, command, true)
		if err != nil {
			return err
		}
		if !sameCandidateConfirmation(locked, scope) || locked.NextVersion != command.ExpectedVersion {
			return conflict("Production bible changed before confirmation")
		}
		now := service.config.Now().UTC()
		version, err := domain.NewProductionBibleVersion(domain.ProductionBibleVersionInput{
			ID: service.config.NewID(), WorkspaceID: locked.WorkspaceID, ProjectID: locked.ProjectID,
			DocumentRevisionID: locked.DocumentRevisionID, DocumentRevisionHash: locked.DocumentRevisionHash,
			CandidateRevisionID: locked.CandidateRevisionID, CandidateRevisionNo: locked.CandidateRevisionNo,
			CandidateRevisionHash: locked.CandidateRevisionHash, CandidateContentHash: locked.CandidateContentHash,
			Version: locked.NextVersion, ReviewDecisionID: command.ReviewDecisionID,
			Snapshot: locked.Snapshot, CreatedBy: actor.UserID, CreatedAt: now,
		})
		if err != nil {
			return err
		}
		if err = repo.CreateBibleVersion(ctx, version); err != nil {
			return err
		}
		receiptResult, err := platformcommand.Result(bibleVersionReceipt{VersionID: version.ID})
		if err != nil {
			return err
		}
		receipt := platformcommand.Receipt{ID: service.config.NewID(), WorkspaceID: version.WorkspaceID, Operation: confirmOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: version.ID, Result: receiptResult, CreatedBy: actor.UserID, CreatedAt: now}
		if err = repo.CreateReceipt(ctx, receipt); err != nil {
			return err
		}
		result = ConfirmResult{Version: version, Receipt: receipt}
		return nil
	})
	return result, normalizeError(err)
}

func sameCandidateConfirmation(left, right CandidateConfirmation) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.DocumentRevisionID == right.DocumentRevisionID && left.DocumentRevisionHash == right.DocumentRevisionHash &&
		left.CandidateRevisionID == right.CandidateRevisionID && left.CandidateRevisionHash == right.CandidateRevisionHash &&
		left.CandidateContentHash == right.CandidateContentHash && left.CandidateRevisionNo == right.CandidateRevisionNo &&
		bytes.Equal(left.Snapshot, right.Snapshot)
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
