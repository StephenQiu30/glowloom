package application

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

const compileOperation = "storygraph.compile"

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

var (
	ErrNotFound = errors.New("StoryGraph resource not found")
	ErrHeadCAS  = errors.New("StoryGraph head compare-and-swap failed")
)

type Error struct {
	Code, Message, NextAction string
	Status                    int
	Details                   map[string]any
}

func (value *Error) Error() string { return value.Message }

func IsStale(err error) bool {
	var value *Error
	return errors.As(err, &value) && value.Code == "stale_storygraph_head"
}

type Actor struct {
	UserID       string
	TokenVersion int
}

type Repository interface {
	LockPublication(context.Context, Actor, string) (storygraph.PublicationState, error)
	LoadOwnerSnapshot(context.Context, storygraph.PublicationState) (storygraph.OwnerSnapshot, error)
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	GetVersion(context.Context, string) (storygraph.Version, error)
	CreateVersion(context.Context, storygraph.Version) error
	SwitchHead(context.Context, storygraph.PublicationState, storygraph.Version) (storygraph.Head, error)
	CreateReceipt(context.Context, platformcommand.Receipt) error
	CreateOutbox(context.Context, storygraph.OutboxEvent) error
}

type TransactionManager interface {
	WithinSerializableTransaction(context.Context, func(Repository) error) error
}

type Config struct {
	Now   func() time.Time
	NewID func() string
}

type Service struct {
	transactions TransactionManager
	config       Config
}

type CompileCommand struct {
	ProjectID                  string `json:"project_id"`
	ExpectedHeadRevision       int64  `json:"expected_head_revision"`
	ExpectedCurrentContentHash string `json:"expected_current_content_hash"`
	IdempotencyKey             string `json:"idempotency_key"`
}

type CompileResult struct {
	Version storygraph.Version
	Head    storygraph.Head
	Receipt platformcommand.Receipt
}

type compileReceipt struct {
	VersionID string `json:"version_id"`
}

type publishedPayload struct {
	VersionID       string  `json:"version_id"`
	VersionNo       int64   `json:"version_no"`
	ParentVersionID *string `json:"parent_version_id,omitempty"`
	OwnerSetHash    string  `json:"owner_set_hash"`
	TopologyHash    string  `json:"topology_hash"`
	ContentHash     string  `json:"content_hash"`
}

func NewService(transactions TransactionManager, config Config) *Service {
	return &Service{transactions: transactions, config: config}
}

func (service *Service) Compile(ctx context.Context, actor Actor, command CompileCommand) (CompileResult, error) {
	command.ProjectID = strings.TrimSpace(command.ProjectID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.ProjectID == "" || command.ExpectedHeadRevision < 0 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 ||
		(command.ExpectedHeadRevision == 0 && command.ExpectedCurrentContentHash != "") ||
		(command.ExpectedHeadRevision > 0 && !hashPattern.MatchString(command.ExpectedCurrentContentHash)) {
		return CompileResult{}, invalid("Invalid StoryGraph compilation request")
	}
	inputHash, err := platformcommand.InputHash(command)
	if err != nil {
		return CompileResult{}, err
	}
	var result CompileResult
	err = service.transactions.WithinSerializableTransaction(ctx, func(repo Repository) error {
		state, lockErr := repo.LockPublication(ctx, actor, command.ProjectID)
		if lockErr != nil {
			return lockErr
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, state.WorkspaceID, compileOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[compileReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			version, replayErr := repo.GetVersion(ctx, replayed.VersionID)
			if replayErr != nil {
				return replayErr
			}
			if version.WorkspaceID != state.WorkspaceID || version.ProjectID != state.ProjectID {
				return ErrNotFound
			}
			result = CompileResult{Version: version, Head: headFromVersion(version), Receipt: receipt}
			return nil
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if state.HeadRevision != command.ExpectedHeadRevision || state.CurrentContentHash != command.ExpectedCurrentContentHash {
			result.Head = headFromState(state)
			return stale(state)
		}
		snapshot, snapshotErr := repo.LoadOwnerSnapshot(ctx, state)
		if snapshotErr != nil {
			return snapshotErr
		}
		compiled, compileErr := storygraph.CompileOwnerSnapshot(snapshot)
		if compileErr != nil {
			return invalid(compiledErrorMessage(compileErr))
		}
		now := service.config.Now().UTC()
		version := newVersion(service.config.NewID(), actor.UserID, now, state, compiled)
		if createErr := repo.CreateVersion(ctx, version); createErr != nil {
			return createErr
		}
		head, switchErr := repo.SwitchHead(ctx, state, version)
		if switchErr != nil {
			return switchErr
		}
		receiptResult, resultErr := platformcommand.Result(compileReceipt{VersionID: version.ID})
		if resultErr != nil {
			return resultErr
		}
		receipt := platformcommand.Receipt{
			ID: service.config.NewID(), WorkspaceID: state.WorkspaceID, Operation: compileOperation,
			IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: version.ID,
			Result: receiptResult, CreatedBy: actor.UserID, CreatedAt: now,
		}
		if createErr := repo.CreateReceipt(ctx, receipt); createErr != nil {
			return createErr
		}
		payload := publishedPayload{
			VersionID: version.ID, VersionNo: version.VersionNo, ParentVersionID: version.ParentVersionID,
			OwnerSetHash: version.OwnerSetHash, TopologyHash: version.TopologyHash, ContentHash: version.ContentHash,
		}
		encodedPayload, encodeErr := json.Marshal(payload)
		if encodeErr != nil {
			return encodeErr
		}
		payloadHash, hashErr := storygraph.HashCanonicalValue(payload)
		if hashErr != nil {
			return hashErr
		}
		event := storygraph.OutboxEvent{
			ID: service.config.NewID(), EventType: "StoryGraphVersionPublished", EventVersion: 1,
			WorkspaceID: state.WorkspaceID, ProjectID: state.ProjectID,
			AggregateKind: "storygraph", AggregateID: state.ProjectID, AggregateRevision: version.VersionNo,
			SourceReceiptID: receipt.ID, Payload: encodedPayload, PayloadHash: payloadHash,
			Status: "pending", Attempts: 0, OccurredAt: now, CreatedAt: now,
		}
		if createErr := repo.CreateOutbox(ctx, event); createErr != nil {
			return createErr
		}
		result = CompileResult{Version: version, Head: head, Receipt: receipt}
		return nil
	})
	return result, normalizeError(err)
}

func newVersion(id, createdBy string, now time.Time, state storygraph.PublicationState, compiled storygraph.CompiledOwnerSnapshot) storygraph.Version {
	var parentVersionID, parentContentHash *string
	if state.HeadRevision > 0 {
		versionID, contentHash := state.CurrentVersionID, state.CurrentContentHash
		parentVersionID, parentContentHash = &versionID, &contentHash
	}
	return storygraph.Version{
		ID: id, WorkspaceID: state.WorkspaceID, ProjectID: state.ProjectID,
		VersionNo: state.HeadRevision + 1, ParentVersionID: parentVersionID, ParentContentHash: parentContentHash,
		SourceRevisionID: compiled.SourceRevisionID, SourceRevisionHash: compiled.SourceRevisionHash,
		OwnerHeads: compiled.OwnerHeads, OwnerSetHash: compiled.OwnerSetHash,
		SchemaVersion: compiled.Graph.SchemaVersion, Nodes: compiled.Graph.Nodes, Edges: compiled.Graph.Edges,
		TopologyHash: compiled.Graph.TopologyHash, ContentHash: compiled.Graph.ContentHash,
		Status: "published", PublishedAt: now, CreatedBy: createdBy, CreatedAt: now,
	}
}

func headFromState(state storygraph.PublicationState) storygraph.Head {
	return storygraph.Head{
		WorkspaceID: state.WorkspaceID, ProjectID: state.ProjectID,
		CurrentVersionID: state.CurrentVersionID, CurrentContentHash: state.CurrentContentHash,
		Revision: state.HeadRevision,
	}
}

func headFromVersion(version storygraph.Version) storygraph.Head {
	return storygraph.Head{
		WorkspaceID: version.WorkspaceID, ProjectID: version.ProjectID,
		CurrentVersionID: version.ID, CurrentContentHash: version.ContentHash,
		Revision: version.VersionNo, UpdatedAt: version.PublishedAt,
	}
}

func stale(state storygraph.PublicationState) error {
	return &Error{
		Code: "stale_storygraph_head", Message: "StoryGraph head changed before publication", Status: 409,
		NextAction: "Reload the current StoryGraph head and compile from its exact revision and content hash",
		Details: map[string]any{
			"current_version_id":    state.CurrentVersionID,
			"current_content_hash":  state.CurrentContentHash,
			"current_head_revision": state.HeadRevision,
		},
	}
}

func invalid(message string) error {
	return &Error{Code: "invalid_storygraph", Message: message, Status: 422}
}

func conflict(message string) error {
	return &Error{Code: "resource_conflict", Message: message, Status: 409}
}

func compiledErrorMessage(err error) string {
	if err == nil {
		return "StoryGraph compilation failed"
	}
	return err.Error()
}

func normalizeError(err error) error {
	if errors.Is(err, ErrHeadCAS) {
		return &Error{
			Code: "stale_storygraph_head", Message: "StoryGraph head changed before publication", Status: 409,
			NextAction: "Reload the current StoryGraph head before retrying compilation",
		}
	}
	if errors.Is(err, ErrNotFound) {
		return &Error{Code: "not_found", Message: "StoryGraph resource not found", Status: 404}
	}
	return err
}
