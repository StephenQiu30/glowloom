package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
)

const (
	createBatchOperation     = "storyboard.create_batch"
	createSetOperation       = "storyboard.create_set"
	applySetOperation        = "storyboard.apply_set"
	createExportSetOperation = "storyboard.create_export_set"
	decideDraftOperation     = "storyboard.decide_draft"
	approveBatchOperation    = "storyboard.approve_batch"
	applyBatchOperation      = "storyboard.apply_batch"
	createExportOperation    = "storyboard.create_export"
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
	LockEpisode(context.Context, Actor, string) error
	CreateShots(context.Context, domain.Batch, []domain.Shot) error
	ListShots(context.Context, Actor, string) ([]domain.Shot, error)
	CreateExportSetWorkflow(context.Context, domain.ExportSet, []domain.Export) error
	GetExportSet(context.Context, Actor, string) (domain.ExportSet, error)
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
type ApplySetCommand struct {
	SetID, ExpectedCandidateHash, IdempotencyKey string
	ExpectedRevision                             int
}
type ApplySetResult struct {
	Set     domain.DraftSet
	Receipt platformcommand.Receipt
}
type ExportCommand struct{ EpisodeID, ExpectedOrderHash, IdempotencyKey string }
type CreateExportSetCommand struct {
	SetID, ExpectedResultHash, IdempotencyKey string
	ExpectedRevision                          int
}
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
		baselineOrderHashes := make([]string, len(command.Structures))
		seenEpisodes := make(map[string]struct{}, len(command.Structures))
		seenStructures := make(map[string]struct{}, len(command.Structures))
		for _, reference := range command.Structures {
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
		}
		firstInput, err := repo.DraftInput(ctx, actor, command.Structures[0].EpisodeID, true)
		if err != nil {
			return err
		}
		if found, receiptErr := repo.FindReceipt(ctx, firstInput.WorkspaceID, createSetOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[receipt](found, found.InputHash)
			if replayErr != nil {
				return replayErr
			}
			set, replayErr = repo.GetSet(ctx, actor, replayed.ID, false)
			if replayErr != nil {
				return replayErr
			}
			if found.ResourceID != set.ID || found.InputHash != set.InputHash || set.CreatedBy != actor.UserID ||
				set.WorkspaceID != firstInput.WorkspaceID || !draftSetMatchesCreateCommand(set, command) {
				return conflict("Storyboard draft set receipt has drifted")
			}
			return nil
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		for index, reference := range command.Structures {
			input := firstInput
			if index > 0 {
				input, err = repo.DraftInput(ctx, actor, reference.EpisodeID, true)
			}
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
			shots, err := repo.ListShots(ctx, actor, reference.EpisodeID)
			if err != nil {
				return err
			}
			baselineOrderHash, err := domain.OrderHash(shots)
			if err != nil {
				return err
			}
			inputs[index], inputHashes[index] = input, inputHash
			baselineOrderHashes[index] = baselineOrderHash
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
			BaselineOrderHashes  []string
		}{command.StructureCommitID, command.StructureRevision, command.StructureContentHash, inputHashes, baselineOrderHashes})
		if err != nil {
			return err
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
		executionPolicy, err := agentcontract.ExecutionPolicyFor("storyboard_draft")
		if err != nil {
			return err
		}
		encodedPolicy, err := json.Marshal(executionPolicy)
		if err != nil {
			return err
		}
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
				InputHash: inputHashes[index], ExecutionPolicy: append(json.RawMessage(nil), encodedPolicy...),
				Payload: payload, Status: "queued", CreatedAt: now,
			}
			set.Batches[index] = domain.DraftSetBatch{
				BatchID: batchID, EpisodeID: input.EpisodeID, StructureID: input.StructureID,
				ScriptVersionID: input.ScriptVersionID, InputHash: inputHashes[index],
				BaselineOrderHash: baselineOrderHashes[index],
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
			resultHash, hashErr := draftSetCandidateHash(set.Batches)
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

func (service *Service) ApplySet(ctx context.Context, actor Actor, command ApplySetCommand) (ApplySetResult, error) {
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.SetID == "" || command.ExpectedRevision < 1 || len(command.ExpectedCandidateHash) != 64 ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return ApplySetResult{}, invalid("Invalid storyboard draft set apply request")
	}
	var result ApplySetResult
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		set, err := repo.GetSet(ctx, actor, command.SetID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		candidateHash, err := draftSetCandidateHash(set.Batches)
		if err != nil {
			return err
		}
		if found, receiptErr := repo.FindReceipt(ctx, set.WorkspaceID, applySetOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[receipt](found, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			if replayed.ID != set.ID || set.Status != "applied" || set.Revision != command.ExpectedRevision+1 ||
				candidateHash != command.ExpectedCandidateHash || set.ResultHash == nil || len(*set.ResultHash) != 64 {
				return conflict("Storyboard draft set apply receipt has drifted")
			}
			result = ApplySetResult{Set: set, Receipt: found}
			return nil
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if set.Status != "needs_review" || set.Revision != command.ExpectedRevision || set.ResultHash == nil ||
			*set.ResultHash != command.ExpectedCandidateHash || candidateHash != command.ExpectedCandidateHash || len(set.Batches) == 0 {
			return conflict("Storyboard draft set changed before apply")
		}
		for _, reference := range set.Batches {
			if err = repo.LockEpisode(ctx, actor, reference.EpisodeID); err != nil {
				return err
			}
		}
		now := service.config.Now().UTC()
		formalReferences := make([]formalShotReference, 0)
		for _, reference := range set.Batches {
			batch, batchErr := repo.GetBatch(ctx, actor, reference.BatchID, true)
			if batchErr != nil {
				return batchErr
			}
			if batch.WorkspaceID != set.WorkspaceID || batch.ProjectID != set.ProjectID || batch.EpisodeID != reference.EpisodeID ||
				batch.StructureID != reference.StructureID || batch.ScriptVersionID != reference.ScriptVersionID ||
				batch.InputHash != reference.InputHash || batch.ResultHash == nil || reference.ResultHash == nil ||
				*batch.ResultHash != *reference.ResultHash || batch.Status != "approved" || batch.ApprovedBy == nil {
				return conflict("Every Storyboard Batch must be approved before Set apply")
			}
			for _, draft := range batch.Candidate.Shots {
				if batch.Decisions[draft.ProposalKey] != "accepted" {
					return conflict("Every storyboard draft must be accepted before Set apply")
				}
			}
			currentShots, listErr := repo.ListShots(ctx, actor, batch.EpisodeID)
			if listErr != nil {
				return listErr
			}
			baselineOrderHash, hashErr := domain.OrderHash(currentShots)
			if hashErr != nil {
				return hashErr
			}
			if baselineOrderHash != reference.BaselineOrderHash {
				return conflict("Formal storyboard Shots changed after Draft Set creation")
			}
			shots := make([]domain.Shot, len(batch.Candidate.Shots))
			for index, draft := range batch.Candidate.Shots {
				contentHash, contentErr := platformcommand.InputHash(draft)
				if contentErr != nil {
					return contentErr
				}
				shots[index] = domain.Shot{
					ID: service.config.NewID(), WorkspaceID: batch.WorkspaceID, ProjectID: batch.ProjectID,
					EpisodeID: batch.EpisodeID, BatchID: batch.ID, ProposalKey: draft.ProposalKey,
					Position: draft.Position, Title: draft.Title, NarrativeUnitIDs: draft.NarrativeUnitVersionIDs,
					Spec: draft.Spec, ContentHash: contentHash, Status: "active", Revision: 1,
					CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now,
				}
				formalReferences = append(formalReferences, formalShotReference{
					BatchID: batch.ID, EpisodeID: batch.EpisodeID, ShotID: shots[index].ID,
					Position: shots[index].Position, ContentHash: shots[index].ContentHash,
				})
			}
			batch.Status, batch.AppliedAt, batch.UpdatedAt = "applied", &now, now
			batch.Revision++
			if err = repo.CreateShots(ctx, batch, shots); err != nil {
				return err
			}
		}
		formalHash, err := platformcommand.InputHash(formalReferences)
		if err != nil {
			return err
		}
		set.Status, set.ResultHash, set.Revision, set.UpdatedAt = "applied", &formalHash, set.Revision+1, now
		if err = repo.SaveSet(ctx, set); err != nil {
			return err
		}
		receiptResult, err := platformcommand.Result(receipt{ID: set.ID})
		if err != nil {
			return err
		}
		ownerReceipt := platformcommand.Receipt{
			ID: service.config.NewID(), WorkspaceID: set.WorkspaceID, Operation: applySetOperation,
			IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: set.ID,
			Result: receiptResult, CreatedBy: actor.UserID, CreatedAt: now,
		}
		if err = repo.CreateReceipt(ctx, ownerReceipt); err != nil {
			return err
		}
		result = ApplySetResult{Set: set, Receipt: ownerReceipt}
		return nil
	})
	return result, normalizeError(err)
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
		executionPolicy, err := agentcontract.ExecutionPolicyFor("storyboard_draft")
		if err != nil {
			return err
		}
		encodedPolicy, err := json.Marshal(executionPolicy)
		if err != nil {
			return err
		}
		invocation := domain.Invocation{ID: invocationID, WorkspaceID: input.WorkspaceID, RequestID: batchID, Kind: "storyboard_draft", InputHash: inputHash, ExecutionPolicy: encodedPolicy, Payload: payload, Status: "queued", CreatedAt: now}
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
	var value ApplyPreflight
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		batch, err := repo.GetBatch(ctx, actor, batchID, false)
		if err != nil {
			return err
		}
		if batch.Status != "approved" || batch.Revision != expectedRevision {
			return conflict("Storyboard batch is not approved at the expected revision")
		}
		shots, err := repo.ListShots(ctx, actor, batch.EpisodeID)
		if err != nil {
			return err
		}
		baselineOrderHash, err := domain.OrderHash(shots)
		if err != nil {
			return err
		}
		orderHash, impactHash, err := candidateImpact(batch, baselineOrderHash)
		if err != nil {
			return err
		}
		value = ApplyPreflight{BatchID: batch.ID, BatchRevision: batch.Revision, OrderHash: orderHash, ImpactHash: impactHash, Created: len(batch.Candidate.Shots)}
		return nil
	})
	return value, normalizeError(err)
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
		if err = repo.LockEpisode(ctx, actor, batch.EpisodeID); err != nil {
			return err
		}
		currentShots, err := repo.ListShots(ctx, actor, batch.EpisodeID)
		if err != nil {
			return err
		}
		baselineOrderHash, err := domain.OrderHash(currentShots)
		if err != nil {
			return err
		}
		orderHash, impactHash, err := candidateImpact(batch, baselineOrderHash)
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

func (service *Service) CreateExportSet(
	ctx context.Context,
	actor Actor,
	command CreateExportSetCommand,
) (domain.ExportSet, error) {
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.SetID == "" || command.ExpectedRevision < 1 || len(command.ExpectedResultHash) != 64 ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return domain.ExportSet{}, invalid("Invalid Storyboard Export Set request")
	}
	var value domain.ExportSet
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		set, err := repo.GetSet(ctx, actor, command.SetID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(struct {
			SetID, ResultHash string
			Revision          int
		}{command.SetID, command.ExpectedResultHash, command.ExpectedRevision})
		if err != nil {
			return err
		}
		if found, receiptErr := repo.FindReceipt(ctx, set.WorkspaceID, createExportSetOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[receipt](found, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			value, replayErr = repo.GetExportSet(ctx, actor, replayed.ID)
			if replayErr != nil {
				return replayErr
			}
			if found.ResourceID != value.ID || found.CreatedBy != actor.UserID || value.CreatedBy != actor.UserID ||
				value.DraftSetID != command.SetID || value.DraftSetRevision != command.ExpectedRevision ||
				value.InputHash != inputHash || value.Status != "succeeded" || len(value.ContentHash) != 64 ||
				len(value.Exports) != len(set.Batches) || len(value.Exports) == 0 {
				return conflict("Storyboard Export Set receipt has drifted")
			}
			return nil
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if set.Status != "applied" || set.Revision != command.ExpectedRevision || set.ResultHash == nil ||
			*set.ResultHash != command.ExpectedResultHash || len(set.Batches) == 0 {
			return conflict("Applied Storyboard Draft Set changed before export")
		}
		for _, reference := range set.Batches {
			if err = repo.LockEpisode(ctx, actor, reference.EpisodeID); err != nil {
				return err
			}
		}
		snapshots := make([]exportEpisodeSnapshot, len(set.Batches))
		formalReferences := make([]formalShotReference, 0)
		for index, reference := range set.Batches {
			batch, batchErr := repo.GetBatch(ctx, actor, reference.BatchID, false)
			if batchErr != nil {
				return batchErr
			}
			if batch.WorkspaceID != set.WorkspaceID || batch.ProjectID != set.ProjectID || batch.Status != "applied" ||
				batch.EpisodeID != reference.EpisodeID || batch.StructureID != reference.StructureID ||
				batch.ScriptVersionID != reference.ScriptVersionID || batch.InputHash != reference.InputHash ||
				batch.ResultHash == nil || reference.ResultHash == nil || *batch.ResultHash != *reference.ResultHash {
				return conflict("Applied Storyboard Batch changed before export")
			}
			shots, listErr := repo.ListShots(ctx, actor, reference.EpisodeID)
			if listErr != nil {
				return listErr
			}
			if len(shots) == 0 {
				return conflict("Formal storyboard Shots are required for every exported Episode")
			}
			for _, shot := range shots {
				if shot.WorkspaceID != set.WorkspaceID || shot.ProjectID != set.ProjectID ||
					shot.EpisodeID != reference.EpisodeID || shot.BatchID != reference.BatchID || shot.Status != "active" {
					return conflict("Formal storyboard Shots changed after Draft Set apply")
				}
				formalReferences = append(formalReferences, formalShotReference{
					BatchID: reference.BatchID, EpisodeID: reference.EpisodeID, ShotID: shot.ID,
					Position: shot.Position, ContentHash: shot.ContentHash,
				})
			}
			orderHash, hashErr := domain.OrderHash(shots)
			if hashErr != nil {
				return hashErr
			}
			snapshots[index] = exportEpisodeSnapshot{reference: reference, shots: shots, orderHash: orderHash}
		}
		formalHash, err := platformcommand.InputHash(formalReferences)
		if err != nil {
			return err
		}
		if formalHash != command.ExpectedResultHash {
			return conflict("Formal storyboard Shots changed after Draft Set apply")
		}
		now := service.config.Now().UTC()
		value = domain.ExportSet{
			ID: service.config.NewID(), WorkspaceID: set.WorkspaceID, ProjectID: set.ProjectID,
			DraftSetID: set.ID, DraftSetRevision: set.Revision, Status: "succeeded", InputHash: inputHash,
			Exports: make([]domain.ExportSetReference, len(snapshots)), Revision: 1,
			CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now,
		}
		exports := make([]domain.Export, len(snapshots))
		contentReferences := make([]exportSetContentReference, len(snapshots))
		for index, snapshot := range snapshots {
			episodeInputHash, hashErr := platformcommand.InputHash(struct{ EpisodeID, OrderHash string }{
				snapshot.reference.EpisodeID, snapshot.orderHash,
			})
			if hashErr != nil {
				return hashErr
			}
			built, buildErr := buildPackage(snapshot.reference.EpisodeID, episodeInputHash, snapshot.shots)
			if buildErr != nil {
				return buildErr
			}
			exports[index] = domain.Export{
				ID: service.config.NewID(), WorkspaceID: set.WorkspaceID, ProjectID: set.ProjectID,
				ExportSetID: &value.ID, EpisodeID: snapshot.reference.EpisodeID, Status: "succeeded",
				InputHash: episodeInputHash, ContentHash: built.ContentHash, Manifest: built.Manifest,
				Files: built.Files, Package: built.Bytes, Revision: 1, CreatedBy: actor.UserID,
				CreatedAt: now, UpdatedAt: now,
			}
			value.Exports[index] = domain.ExportSetReference{
				ExportID: exports[index].ID, EpisodeID: exports[index].EpisodeID,
				OrderHash: snapshot.orderHash, ContentHash: exports[index].ContentHash,
			}
			contentReferences[index] = exportSetContentReference{
				EpisodeID: exports[index].EpisodeID, OrderHash: snapshot.orderHash,
				ContentHash: exports[index].ContentHash,
			}
		}
		value.ContentHash, err = platformcommand.InputHash(contentReferences)
		if err != nil {
			return err
		}
		if err = repo.CreateExportSetWorkflow(ctx, value, exports); err != nil {
			return err
		}
		result, err := platformcommand.Result(receipt{ID: value.ID})
		if err != nil {
			return err
		}
		return repo.CreateReceipt(ctx, platformcommand.Receipt{
			ID: service.config.NewID(), WorkspaceID: value.WorkspaceID, Operation: createExportSetOperation,
			IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: value.ID,
			Result: result, CreatedBy: actor.UserID, CreatedAt: now,
		})
	})
	return value, normalizeError(err)
}

func (service *Service) GetExportSet(ctx context.Context, actor Actor, exportSetID string) (domain.ExportSet, error) {
	var value domain.ExportSet
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		value, err = repo.GetExportSet(ctx, actor, exportSetID)
		return err
	})
	return value, normalizeError(err)
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

type formalShotReference struct {
	BatchID, EpisodeID, ShotID, ContentHash string
	Position                                int
}

type exportEpisodeSnapshot struct {
	reference domain.DraftSetBatch
	shots     []domain.Shot
	orderHash string
}

type exportSetContentReference struct {
	EpisodeID, OrderHash, ContentHash string
}

func draftSetCandidateHash(batches []domain.DraftSetBatch) (string, error) {
	return platformcommand.InputHash(batches)
}

func draftSetMatchesCreateCommand(set domain.DraftSet, command CreateSetCommand) bool {
	if set.StructureCommitID != command.StructureCommitID || set.StructureRevision != command.StructureRevision ||
		set.StructureContentHash != command.StructureContentHash || len(set.Batches) != len(command.Structures) {
		return false
	}
	for index, reference := range command.Structures {
		batch := set.Batches[index]
		if batch.EpisodeID != reference.EpisodeID || batch.StructureID != reference.StructureID ||
			batch.ScriptVersionID != reference.ScriptVersionID {
			return false
		}
	}
	return true
}

func candidateImpact(batch domain.Batch, baselineOrderHash string) (string, string, error) {
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
		BatchID, OrderHash, BaselineOrderHash string
		Revision, Created                     int
	}{batch.ID, orderHash, baselineOrderHash, batch.Revision, len(ordered)})
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
