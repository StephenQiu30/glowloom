package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
)

const (
	createBatchOperation  = "storyboard.create_batch"
	createSetOperation    = "storyboard.create_set"
	decideDraftOperation  = "storyboard.decide_draft"
	approveBatchOperation = "storyboard.approve_batch"
	applyBatchOperation   = "storyboard.apply_batch"
	createExportOperation = "storyboard.create_export"
)

var ErrNotFound = errors.New("storyboard resource not found")

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
type Repository interface {
	DraftInput(context.Context, Actor, string, bool) (domain.DraftInput, error)
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	CreateReceipt(context.Context, platformcommand.Receipt) error
	CreateWorkflow(context.Context, domain.Batch, domain.Invocation) error
	CreateSetWorkflow(context.Context, domain.DraftSet, []domain.Batch, []domain.Invocation) error
	GetSet(context.Context, Actor, string, bool) (domain.DraftSet, error)
	SaveSet(context.Context, domain.DraftSet) error
	GetBatch(context.Context, Actor, string, bool) (domain.Batch, error)
	GetLatestBatch(context.Context, Actor, string) (domain.Batch, error)
	SaveBatch(context.Context, domain.Batch) error
	CreateShots(context.Context, domain.Batch, []domain.Shot) error
	ListShots(context.Context, Actor, string) ([]domain.Shot, error)
	CreateExport(context.Context, domain.Export) error
	GetExport(context.Context, Actor, string) (domain.Export, error)
	GetLatestExport(context.Context, Actor, string) (domain.Export, error)
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
type CreateBatchCommand struct{ EpisodeID, IdempotencyKey string }
type StructureReference struct {
	EpisodeID, StructureID, ScriptVersionID string
}
type CreateSetCommand struct {
	StructureCommitID, StructureContentHash, IdempotencyKey string
	StructureRevision                                       int
	Structures                                              []StructureReference
}
type DecisionCommand struct {
	BatchID, ProposalKey, Action, IdempotencyKey string
	ExpectedRevision                             int
}
type RevisionCommand struct {
	BatchID, IdempotencyKey string
	ExpectedRevision        int
}
type ApplyCommand struct {
	RevisionCommand
	ExpectedOrderHash, ImpactHash string
}
type ExportCommand struct{ EpisodeID, ExpectedOrderHash, IdempotencyKey string }
type ApplyPreflight struct {
	BatchID               string
	BatchRevision         int
	OrderHash, ImpactHash string
	Created               int
}
type ExportPreflight struct {
	EpisodeID, OrderHash string
	Allowed              bool
	ShotCount            int
	Blockers             []map[string]string
}
type receipt struct {
	ID string `json:"id"`
}

func NewService(transactions TransactionManager, config Config) *Service {
	return &Service{transactions: transactions, config: config}
}

func (service *Service) CreateSet(ctx context.Context, actor Actor, command CreateSetCommand) (domain.DraftSet, error) {
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.StructureCommitID == "" || command.StructureRevision < 1 || len(command.StructureContentHash) != 64 ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 || len(command.Structures) == 0 || len(command.Structures) > 100 {
		return domain.DraftSet{}, invalid("Invalid storyboard draft set request")
	}
	var set domain.DraftSet
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		inputs := make([]domain.DraftInput, len(command.Structures))
		inputHashes := make([]string, len(command.Structures))
		seenEpisodes := make(map[string]struct{}, len(command.Structures))
		seenStructures := make(map[string]struct{}, len(command.Structures))
		for index, reference := range command.Structures {
			if reference.EpisodeID == "" || reference.StructureID == "" || reference.ScriptVersionID == "" {
				return invalid("Invalid storyboard structure reference")
			}
			if _, exists := seenEpisodes[reference.EpisodeID]; exists {
				return conflict("Storyboard draft set contains duplicate Episodes")
			}
			if _, exists := seenStructures[reference.StructureID]; exists {
				return conflict("Storyboard draft set contains duplicate Structures")
			}
			seenEpisodes[reference.EpisodeID], seenStructures[reference.StructureID] = struct{}{}, struct{}{}
			input, err := repo.DraftInput(ctx, actor, reference.EpisodeID, true)
			if err != nil {
				return err
			}
			if input.EpisodeID != reference.EpisodeID || input.StructureID != reference.StructureID ||
				input.ScriptVersionID != reference.ScriptVersionID {
				return conflict("Confirmed Episode Structure changed before storyboard drafting")
			}
			inputHash, err := platformcommand.InputHash(input)
			if err != nil {
				return err
			}
			inputs[index], inputHashes[index] = input, inputHash
		}
		workspaceID, projectID := inputs[0].WorkspaceID, inputs[0].ProjectID
		for _, input := range inputs[1:] {
			if input.WorkspaceID != workspaceID || input.ProjectID != projectID {
				return conflict("Storyboard draft set crosses project boundaries")
			}
		}
		inputHash, err := platformcommand.InputHash(struct {
			StructureCommitID    string
			StructureRevision    int
			StructureContentHash string
			BatchInputHashes     []string
		}{command.StructureCommitID, command.StructureRevision, command.StructureContentHash, inputHashes})
		if err != nil {
			return err
		}
		if found, receiptErr := repo.FindReceipt(ctx, workspaceID, createSetOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[receipt](found, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			set, replayErr = repo.GetSet(ctx, actor, replayed.ID, false)
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		now := service.config.Now().UTC()
		set = domain.DraftSet{
			ID: service.config.NewID(), WorkspaceID: workspaceID, ProjectID: projectID,
			StructureCommitID: command.StructureCommitID, StructureRevision: command.StructureRevision,
			StructureContentHash: command.StructureContentHash, Status: "queued", InputHash: inputHash,
			Batches: make([]domain.DraftSetBatch, len(inputs)), Revision: 1, CreatedBy: actor.UserID,
			CreatedAt: now, UpdatedAt: now,
		}
		batches := make([]domain.Batch, len(inputs))
		invocations := make([]domain.Invocation, len(inputs))
		for index, input := range inputs {
			batchID, taskID, invocationID := service.config.NewID(), service.config.NewID(), service.config.NewID()
			payload, err := storyboardPayload(input, batchID, taskID, invocationID, inputHashes[index])
			if err != nil {
				return err
			}
			batches[index] = domain.Batch{
				ID: batchID, WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID, EpisodeID: input.EpisodeID,
				StructureID: input.StructureID, ScriptVersionID: input.ScriptVersionID, TaskID: taskID,
				Status: "queued", InputHash: inputHashes[index], Candidate: domain.Candidate{Shots: []domain.DraftShot{}},
				Decisions: map[string]string{}, Revision: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now,
			}
			invocations[index] = domain.Invocation{
				ID: invocationID, WorkspaceID: input.WorkspaceID, RequestID: batchID, Kind: "storyboard_draft",
				InputHash: inputHashes[index], Payload: payload, Status: "queued", CreatedAt: now,
			}
			set.Batches[index] = domain.DraftSetBatch{
				BatchID: batchID, EpisodeID: input.EpisodeID, StructureID: input.StructureID,
				ScriptVersionID: input.ScriptVersionID, InputHash: inputHashes[index],
			}
		}
		if err = repo.CreateSetWorkflow(ctx, set, batches, invocations); err != nil {
			return err
		}
		result, err := platformcommand.Result(receipt{ID: set.ID})
		if err != nil {
			return err
		}
		return repo.CreateReceipt(ctx, platformcommand.Receipt{
			ID: service.config.NewID(), WorkspaceID: set.WorkspaceID, Operation: createSetOperation,
			IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: set.ID,
			Result: result, CreatedBy: actor.UserID, CreatedAt: now,
		})
	})
	return set, normalizeError(err)
}

func (service *Service) RefreshSet(ctx context.Context, actor Actor, setID string) (domain.DraftSet, error) {
	var value domain.DraftSet
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		set, err := repo.GetSet(ctx, actor, setID, true)
		if err != nil {
			return err
		}
		if set.Status != "queued" {
			value = set
			return nil
		}
		ready := true
		terminal := ""
		for index, reference := range set.Batches {
			batch, batchErr := repo.GetBatch(ctx, actor, reference.BatchID, false)
			if batchErr != nil {
				return batchErr
			}
			if batch.WorkspaceID != set.WorkspaceID || batch.ProjectID != set.ProjectID || batch.CreatedBy != set.CreatedBy ||
				batch.EpisodeID != reference.EpisodeID || batch.StructureID != reference.StructureID ||
				batch.ScriptVersionID != reference.ScriptVersionID || batch.InputHash != reference.InputHash {
				return conflict("Storyboard draft set batch has drifted")
			}
			switch batch.Status {
			case "queued", "running":
				ready = false
			case "needs_review", "approved", "applied":
				if batch.ResultHash == nil || len(*batch.ResultHash) != 64 {
					return conflict("Storyboard draft set result is incomplete")
				}
				resultHash := *batch.ResultHash
				set.Batches[index].ResultHash = &resultHash
			case "failed", "unknown", "cancelled":
				terminal = batch.Status
			default:
				return conflict("Storyboard draft set batch has an invalid status")
			}
		}
		if terminal == "" && !ready {
			value = set
			return nil
		}
		now := service.config.Now().UTC()
		if terminal != "" {
			set.Status = terminal
			set.ResultHash = nil
		} else {
			resultHash, hashErr := platformcommand.InputHash(set.Batches)
			if hashErr != nil {
				return hashErr
			}
			set.Status, set.ResultHash = "needs_review", &resultHash
		}
		set.Revision, set.UpdatedAt = set.Revision+1, now
		if err = repo.SaveSet(ctx, set); err != nil {
			return err
		}
		value = set
		return nil
	})
	return value, normalizeError(err)
}

func (service *Service) CreateBatch(ctx context.Context, actor Actor, command CreateBatchCommand) (domain.Batch, error) {
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.EpisodeID == "" || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return domain.Batch{}, invalid("Invalid storyboard draft request")
	}
	var batch domain.Batch
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		input, err := repo.DraftInput(ctx, actor, command.EpisodeID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(input)
		if err != nil {
			return err
		}
		if found, receiptErr := repo.FindReceipt(ctx, input.WorkspaceID, createBatchOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[receipt](found, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			batch, replayErr = repo.GetBatch(ctx, actor, replayed.ID, false)
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		now, batchID, taskID, invocationID := service.config.Now().UTC(), service.config.NewID(), service.config.NewID(), service.config.NewID()
		payload, err := storyboardPayload(input, batchID, taskID, invocationID, inputHash)
		if err != nil {
			return err
		}
		batch = domain.Batch{ID: batchID, WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID, EpisodeID: input.EpisodeID, StructureID: input.StructureID, ScriptVersionID: input.ScriptVersionID, TaskID: taskID, Status: "queued", InputHash: inputHash, Candidate: domain.Candidate{Shots: []domain.DraftShot{}}, Decisions: map[string]string{}, Revision: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
		invocation := domain.Invocation{ID: invocationID, WorkspaceID: input.WorkspaceID, RequestID: batchID, Kind: "storyboard_draft", InputHash: inputHash, Payload: payload, Status: "queued", CreatedAt: now}
		if err = repo.CreateWorkflow(ctx, batch, invocation); err != nil {
			return err
		}
		result, err := platformcommand.Result(receipt{ID: batch.ID})
		if err != nil {
			return err
		}
		return repo.CreateReceipt(ctx, platformcommand.Receipt{ID: service.config.NewID(), WorkspaceID: batch.WorkspaceID, Operation: createBatchOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: batch.ID, Result: result, CreatedBy: actor.UserID, CreatedAt: now})
	})
	return batch, normalizeError(err)
}
func (service *Service) GetBatch(ctx context.Context, actor Actor, batchID string) (domain.Batch, error) {
	var value domain.Batch
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		value, err = repo.GetBatch(ctx, actor, batchID, false)
		return err
	})
	return value, normalizeError(err)
}
func (service *Service) GetLatestBatch(ctx context.Context, actor Actor, episodeID string) (domain.Batch, error) {
	var value domain.Batch
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		value, err = repo.GetLatestBatch(ctx, actor, episodeID)
		return err
	})
	return value, normalizeError(err)
}
func (service *Service) Decide(ctx context.Context, actor Actor, command DecisionCommand) (domain.Batch, error) {
	if command.Action != "accepted" || command.ExpectedRevision < 1 || command.ProposalKey == "" || strings.TrimSpace(command.IdempotencyKey) == "" {
		return domain.Batch{}, invalid("Only an explicit accepted decision is supported")
	}
	var value domain.Batch
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		batch, err := repo.GetBatch(ctx, actor, command.BatchID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		if found, receiptErr := repo.FindReceipt(ctx, batch.WorkspaceID, decideDraftOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[receipt](found, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			value, replayErr = repo.GetBatch(ctx, actor, replayed.ID, false)
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if batch.Status != "needs_review" || batch.Revision != command.ExpectedRevision {
			return conflict("Storyboard batch changed before the decision")
		}
		exists := false
		for _, shot := range batch.Candidate.Shots {
			if shot.ProposalKey == command.ProposalKey {
				exists = true
				break
			}
		}
		if !exists {
			return ErrNotFound
		}
		batch.Decisions[command.ProposalKey] = "accepted"
		now := service.config.Now().UTC()
		batch.Revision++
		batch.UpdatedAt = now
		if err = repo.SaveBatch(ctx, batch); err != nil {
			return err
		}
		result, err := platformcommand.Result(receipt{ID: batch.ID})
		if err != nil {
			return err
		}
		if err = repo.CreateReceipt(ctx, platformcommand.Receipt{ID: service.config.NewID(), WorkspaceID: batch.WorkspaceID, Operation: decideDraftOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: batch.ID, Result: result, CreatedBy: actor.UserID, CreatedAt: now}); err != nil {
			return err
		}
		value = batch
		return nil
	})
	return value, normalizeError(err)
}
func (service *Service) Approve(ctx context.Context, actor Actor, command RevisionCommand) (domain.Batch, error) {
	var value domain.Batch
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		batch, err := repo.GetBatch(ctx, actor, command.BatchID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		if found, receiptErr := repo.FindReceipt(ctx, batch.WorkspaceID, approveBatchOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[receipt](found, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			value, replayErr = repo.GetBatch(ctx, actor, replayed.ID, false)
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if batch.Status != "needs_review" || batch.Revision != command.ExpectedRevision {
			return conflict("Storyboard batch changed before approval")
		}
		for _, shot := range batch.Candidate.Shots {
			if batch.Decisions[shot.ProposalKey] != "accepted" {
				return conflict("Every storyboard draft must be accepted before approval")
			}
		}
		now := service.config.Now().UTC()
		batch.Status, batch.ApprovedAt, batch.ApprovedBy, batch.UpdatedAt = "approved", &now, &actor.UserID, now
		batch.Revision++
		if err = repo.SaveBatch(ctx, batch); err != nil {
			return err
		}
		result, err := platformcommand.Result(receipt{ID: batch.ID})
		if err != nil {
			return err
		}
		if err = repo.CreateReceipt(ctx, platformcommand.Receipt{ID: service.config.NewID(), WorkspaceID: batch.WorkspaceID, Operation: approveBatchOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: batch.ID, Result: result, CreatedBy: actor.UserID, CreatedAt: now}); err != nil {
			return err
		}
		value = batch
		return nil
	})
	return value, normalizeError(err)
}

func (service *Service) PreflightApply(ctx context.Context, actor Actor, batchID string, expectedRevision int) (ApplyPreflight, error) {
	batch, err := service.GetBatch(ctx, actor, batchID)
	if err != nil {
		return ApplyPreflight{}, err
	}
	if batch.Status != "approved" || batch.Revision != expectedRevision {
		return ApplyPreflight{}, conflict("Storyboard batch is not approved at the expected revision")
	}
	orderHash, impactHash, err := candidateImpact(batch)
	if err != nil {
		return ApplyPreflight{}, err
	}
	return ApplyPreflight{BatchID: batch.ID, BatchRevision: batch.Revision, OrderHash: orderHash, ImpactHash: impactHash, Created: len(batch.Candidate.Shots)}, nil
}
func (service *Service) Apply(ctx context.Context, actor Actor, command ApplyCommand) (domain.Batch, []domain.Shot, error) {
	var value domain.Batch
	var shots []domain.Shot
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		batch, err := repo.GetBatch(ctx, actor, command.BatchID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		if found, receiptErr := repo.FindReceipt(ctx, batch.WorkspaceID, applyBatchOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[receipt](found, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			value, replayErr = repo.GetBatch(ctx, actor, replayed.ID, false)
			if replayErr != nil {
				return replayErr
			}
			shots, replayErr = repo.ListShots(ctx, actor, value.EpisodeID)
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		orderHash, impactHash, err := candidateImpact(batch)
		if err != nil {
			return err
		}
		if batch.Status != "approved" || batch.Revision != command.ExpectedRevision || orderHash != command.ExpectedOrderHash || impactHash != command.ImpactHash {
			return conflict("Storyboard apply inputs changed after preflight")
		}
		now := service.config.Now().UTC()
		shots = make([]domain.Shot, len(batch.Candidate.Shots))
		for index, draft := range batch.Candidate.Shots {
			hash, hashErr := platformcommand.InputHash(draft)
			if hashErr != nil {
				return hashErr
			}
			shots[index] = domain.Shot{ID: service.config.NewID(), WorkspaceID: batch.WorkspaceID, ProjectID: batch.ProjectID, EpisodeID: batch.EpisodeID, BatchID: batch.ID, ProposalKey: draft.ProposalKey, Position: draft.Position, Title: draft.Title, NarrativeUnitIDs: draft.NarrativeUnitVersionIDs, Spec: draft.Spec, ContentHash: hash, Status: "active", Revision: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
		}
		batch.Status, batch.AppliedAt, batch.UpdatedAt = "applied", &now, now
		batch.Revision++
		if err = repo.CreateShots(ctx, batch, shots); err != nil {
			return err
		}
		result, err := platformcommand.Result(receipt{ID: batch.ID})
		if err != nil {
			return err
		}
		if err = repo.CreateReceipt(ctx, platformcommand.Receipt{ID: service.config.NewID(), WorkspaceID: batch.WorkspaceID, Operation: applyBatchOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: batch.ID, Result: result, CreatedBy: actor.UserID, CreatedAt: now}); err != nil {
			return err
		}
		value = batch
		return nil
	})
	return value, shots, normalizeError(err)
}
func (service *Service) ListShots(ctx context.Context, actor Actor, episodeID string) ([]domain.Shot, error) {
	var values []domain.Shot
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		values, err = repo.ListShots(ctx, actor, episodeID)
		return err
	})
	return values, normalizeError(err)
}

func (service *Service) PreflightExport(ctx context.Context, actor Actor, episodeID string) (ExportPreflight, error) {
	shots, err := service.ListShots(ctx, actor, episodeID)
	if err != nil {
		return ExportPreflight{}, err
	}
	result := ExportPreflight{EpisodeID: episodeID, Allowed: len(shots) > 0, ShotCount: len(shots), Blockers: []map[string]string{}}
	if len(shots) == 0 {
		result.Blockers = append(result.Blockers, map[string]string{"code": "SHOTS_REQUIRED", "summary": "请先原子写入正式分镜"})
		return result, nil
	}
	result.OrderHash, err = domain.OrderHash(shots)
	return result, err
}
func (service *Service) CreateExport(ctx context.Context, actor Actor, command ExportCommand) (domain.Export, error) {
	var value domain.Export
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		shots, err := repo.ListShots(ctx, actor, command.EpisodeID)
		if err != nil {
			return err
		}
		if len(shots) == 0 {
			return conflict("Formal storyboard shots are required before export")
		}
		orderHash, err := domain.OrderHash(shots)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(struct{ EpisodeID, OrderHash string }{command.EpisodeID, orderHash})
		if err != nil {
			return err
		}
		if found, receiptErr := repo.FindReceipt(ctx, shots[0].WorkspaceID, createExportOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[receipt](found, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			value, replayErr = repo.GetExport(ctx, actor, replayed.ID)
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if orderHash != command.ExpectedOrderHash {
			return conflict("Storyboard order changed after export preflight")
		}
		built, err := buildPackage(command.EpisodeID, inputHash, shots)
		if err != nil {
			return err
		}
		now := service.config.Now().UTC()
		value = domain.Export{ID: service.config.NewID(), WorkspaceID: shots[0].WorkspaceID, ProjectID: shots[0].ProjectID, EpisodeID: command.EpisodeID, Status: "succeeded", InputHash: inputHash, ContentHash: built.ContentHash, Manifest: built.Manifest, Files: built.Files, Package: built.Bytes, Revision: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
		if err = repo.CreateExport(ctx, value); err != nil {
			return err
		}
		result, err := platformcommand.Result(receipt{ID: value.ID})
		if err != nil {
			return err
		}
		return repo.CreateReceipt(ctx, platformcommand.Receipt{ID: service.config.NewID(), WorkspaceID: value.WorkspaceID, Operation: createExportOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: value.ID, Result: result, CreatedBy: actor.UserID, CreatedAt: now})
	})
	return value, normalizeError(err)
}
func (service *Service) GetExport(ctx context.Context, actor Actor, exportID string) (domain.Export, error) {
	var value domain.Export
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		value, err = repo.GetExport(ctx, actor, exportID)
		return err
	})
	return value, normalizeError(err)
}
func (service *Service) GetLatestExport(ctx context.Context, actor Actor, episodeID string) (domain.Export, error) {
	var value domain.Export
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		value, err = repo.GetLatestExport(ctx, actor, episodeID)
		return err
	})
	return value, normalizeError(err)
}

func storyboardPayload(input domain.DraftInput, batchID, taskID, runToken, inputHash string) (json.RawMessage, error) {
	units := make([]map[string]any, len(input.Units))
	for index, unit := range input.Units {
		units[index] = map[string]any{"unit_version_id": unit.ID, "position": unit.Position, "kind": unit.Kind, "exact_text": unit.Text, "required_for_coverage": unit.Required, "source_scene_id": unit.SceneID, "source_dialogue_id": unit.DialogueID}
	}
	return json.Marshal(map[string]any{"batch_id": batchID, "task_id": taskID, "input_hash": inputHash, "script_version_id": input.ScriptVersionID, "target_duration_ms": input.TargetDurationMS, "aspect_ratio": input.AspectRatio, "visual_style": input.VisualStyle, "units": units, "assets": []any{}, "production_bible_id": input.BibleID, "production_bible_revision": input.BibleRevision, "production_bible_result_hash": input.BibleResultHash, "world_entries": input.WorldEntries, "run_token": runToken})
}
func candidateImpact(batch domain.Batch) (string, string, error) {
	ordered := make([]struct {
		ProposalKey string
		Position    int
	}, len(batch.Candidate.Shots))
	for index, shot := range batch.Candidate.Shots {
		ordered[index] = struct {
			ProposalKey string
			Position    int
		}{shot.ProposalKey, shot.Position}
	}
	orderHash, err := platformcommand.InputHash(ordered)
	if err != nil {
		return "", "", err
	}
	impactHash, err := platformcommand.InputHash(struct {
		BatchID   string
		Revision  int
		OrderHash string
		Created   int
	}{batch.ID, batch.Revision, orderHash, len(ordered)})
	return orderHash, impactHash, err
}
func invalid(message string) error {
	return &Error{Code: "validation_failed", Message: message, Status: 422}
}
func conflict(message string) error {
	return &Error{Code: "resource_conflict", Message: message, Status: 409}
}
func normalizeError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return &Error{Code: "not_found", Message: "Storyboard resource not found", Status: 404}
	}
	return err
}
