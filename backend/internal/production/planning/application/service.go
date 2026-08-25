package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
)

const (
	createPlanOperation            = "episode_plan.create"
	confirmPlanOperation           = "episode_plan.confirm"
	materializeOperation           = "episode_plan.materialize"
	publishCommitOperation         = "episode_plan.publish"
	acceptTaskOperation            = "episode_structure.accept_task"
	confirmStructureOperation      = "episode_structure.confirm"
	confirmStructureBatchOperation = "episode_structure.confirm_batch"
)

var (
	ErrNotFound  = errors.New("planning resource not found")
	dialogueLine = regexp.MustCompile(`^([^：:\n]{1,30})[：:](.*)$`)
)

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
	RevisionSource(context.Context, Actor, string, bool) (domain.Source, string, string, error)
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	CreateReceipt(context.Context, platformcommand.Receipt) error
	CreatePlan(context.Context, domain.Plan) error
	GetPlan(context.Context, Actor, string, bool) (domain.Plan, error)
	SavePlan(context.Context, domain.Plan) error
	ProjectImpact(context.Context, Actor, string, bool) (domain.Impact, error)
	HasConfirmedBible(context.Context, string, string) (bool, error)
	Materialize(context.Context, domain.Plan, domain.ImportCommit, []domain.Episode, []Version) error
	GetCommit(context.Context, Actor, string, bool) (domain.ImportCommit, error)
	GetPlanCommit(context.Context, Actor, string) (domain.ImportCommit, error)
	Publish(context.Context, domain.ImportCommit, []domain.Structure) error
	ListEpisodes(context.Context, Actor, string) ([]domain.Episode, error)
	GetEpisode(context.Context, Actor, string, bool) (domain.Episode, error)
	GetStructure(context.Context, Actor, string, bool) (domain.Structure, error)
	GetEpisodeStructure(context.Context, Actor, string) (domain.Structure, error)
	SaveStructure(context.Context, domain.Structure) error
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

type Version struct {
	ID, WorkspaceID, ProjectID, EpisodeID, DocumentRevisionID string
	VersionNo, SourceStart, SourceEnd                         int
	Content, ContentHash, Status, CreatedBy                   string
	CreatedAt                                                 time.Time
}
type View struct {
	Plan   domain.Plan
	Impact domain.Impact
}
type CreatePlanCommand struct {
	RevisionID, Strategy, IdempotencyKey string
	TargetDurationMS                     int
	RequestedEpisodeCount                *int
}
type ConfirmPlanCommand struct {
	PlanID, IdempotencyKey string
	ExpectedRevision       int
}
type ConfirmPlanResult struct {
	View    View
	Receipt platformcommand.Receipt
}
type PublishedStructureBatch struct {
	Commit      domain.ImportCommit
	Structures  []domain.Structure
	ContentHash string
}
type ConfirmStructureBatchCommand struct {
	CommitID, ExpectedContentHash, IdempotencyKey string
	ExpectedRevision                              int
}
type ConfirmStructureBatchResult struct {
	Batch   PublishedStructureBatch
	Receipt platformcommand.Receipt
}
type MaterializeCommand struct {
	PlanID, Mode, ExpectedActiveOrderHash, IdempotencyKey string
	ExpectedPlanRevision, ExpectedProjectRevision         int
}
type PublishCommand struct {
	CommitID, IdempotencyKey string
	ExpectedRevision         int
}
type StructureCommand struct {
	StructureID, IdempotencyKey string
	ExpectedRevision            int
}
type AcceptTaskCommand struct {
	StructureCommand
	TaskID string
}
type resourceReceipt struct {
	ID string `json:"id"`
}

func NewService(transactions TransactionManager, config Config) *Service {
	return &Service{transactions: transactions, config: config}
}

func (service *Service) CreatePlan(ctx context.Context, actor Actor, command CreatePlanCommand) (View, error) {
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.Strategy != "explicit_markers" || command.TargetDurationMS < 15000 || command.TargetDurationMS > 600000 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return View{}, invalid("Only validated explicit episode markers are supported in this MVP")
	}
	var view View
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		source, workspaceID, projectID, err := repo.RevisionSource(ctx, actor, command.RevisionID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(struct {
			RevisionID, NormalizedHash, Strategy string
			TargetDurationMS                     int
			Requested                            *int
		}{source.DocumentRevisionID, source.NormalizedHash, command.Strategy, command.TargetDurationMS, command.RequestedEpisodeCount})
		if err != nil {
			return err
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, workspaceID, createPlanOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[resourceReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			view.Plan, replayErr = repo.GetPlan(ctx, actor, replayed.ID, false)
			if replayErr != nil {
				return replayErr
			}
			view.Impact, replayErr = repo.ProjectImpact(ctx, actor, view.Plan.ProjectID, false)
			view.Impact.ProjectedEpisodeCount += len(view.Plan.Proposals)
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		proposals, err := service.explicitProposals(source, command.TargetDurationMS)
		if err != nil {
			return err
		}
		if command.RequestedEpisodeCount != nil && *command.RequestedEpisodeCount != len(proposals) {
			return invalid("Requested episode count does not match explicit markers")
		}
		now, planID := service.config.Now().UTC(), service.config.NewID()
		for index := range proposals {
			proposals[index].ID, proposals[index].PlanID = service.config.NewID(), planID
		}
		plan := domain.Plan{ID: planID, WorkspaceID: workspaceID, ProjectID: projectID, DocumentRevisionID: source.DocumentRevisionID, Strategy: command.Strategy, Status: "review_ready", TargetDurationMS: command.TargetDurationMS, RequestedEpisodeCount: command.RequestedEpisodeCount, TotalEstimatedDurationMS: command.TargetDurationMS * len(proposals), InputHash: inputHash, EngineVersion: "explicit-markers-v1", SchemaVersion: "episode-plan-v1", Revision: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now, Proposals: proposals, Source: source}
		if err = repo.CreatePlan(ctx, plan); err != nil {
			return err
		}
		result, err := platformcommand.Result(resourceReceipt{ID: plan.ID})
		if err != nil {
			return err
		}
		if err = repo.CreateReceipt(ctx, platformcommand.Receipt{ID: service.config.NewID(), WorkspaceID: workspaceID, Operation: createPlanOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: plan.ID, Result: result, CreatedBy: actor.UserID, CreatedAt: now}); err != nil {
			return err
		}
		impact, err := repo.ProjectImpact(ctx, actor, projectID, false)
		if err != nil {
			return err
		}
		impact.ProjectedEpisodeCount += len(proposals)
		view = View{Plan: plan, Impact: impact}
		return nil
	})
	return view, normalizeError(err)
}

func (service *Service) GetPlan(ctx context.Context, actor Actor, planID string) (View, error) {
	var view View
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		view.Plan, err = repo.GetPlan(ctx, actor, planID, false)
		if err != nil {
			return err
		}
		view.Impact, err = repo.ProjectImpact(ctx, actor, view.Plan.ProjectID, false)
		view.Impact.ProjectedEpisodeCount += len(view.Plan.Proposals)
		return err
	})
	return view, normalizeError(err)
}

func (service *Service) GetImportCommitForPlan(ctx context.Context, actor Actor, planID string) (domain.ImportCommit, bool, error) {
	var commit domain.ImportCommit
	found := false
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		plan, err := repo.GetPlan(ctx, actor, planID, false)
		if err != nil {
			return err
		}
		commit, err = repo.GetPlanCommit(ctx, actor, plan.ID)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		found = err == nil
		return err
	})
	return commit, found, normalizeError(err)
}

func (service *Service) GetPublishedStructureBatch(ctx context.Context, actor Actor, planID string) (PublishedStructureBatch, error) {
	var batch PublishedStructureBatch
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		plan, err := repo.GetPlan(ctx, actor, planID, false)
		if err != nil {
			return err
		}
		commit, err := repo.GetPlanCommit(ctx, actor, plan.ID)
		if err != nil {
			return err
		}
		if plan.Status != "materialized" || commit.PlanID != plan.ID {
			return conflict("Episode structures are not published")
		}
		batch, err = publishedStructureBatch(ctx, repo, actor, commit, false)
		return err
	})
	return batch, normalizeError(err)
}

func (service *Service) ConfirmPublishedStructureBatch(
	ctx context.Context,
	actor Actor,
	command ConfirmStructureBatchCommand,
) (ConfirmStructureBatchResult, error) {
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if strings.TrimSpace(command.CommitID) == "" || command.ExpectedRevision < 1 ||
		len(command.ExpectedContentHash) != 64 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return ConfirmStructureBatchResult{}, invalid("Invalid Episode Structure batch confirmation request")
	}
	var result ConfirmStructureBatchResult
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		commit, err := repo.GetCommit(ctx, actor, command.CommitID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, commit.WorkspaceID, confirmStructureBatchOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[resourceReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			if replayed.ID != commit.ID {
				return conflict("Episode Structure batch receipt has drifted")
			}
			batch, batchErr := publishedStructureBatch(ctx, repo, actor, commit, true)
			if batchErr != nil {
				return batchErr
			}
			if batch.ContentHash != command.ExpectedContentHash {
				return conflict("Episode Structure batch content changed after confirmation")
			}
			for _, structure := range batch.Structures {
				if structure.Status != "confirmed" {
					return conflict("Episode Structure batch confirmation is incomplete")
				}
			}
			result = ConfirmStructureBatchResult{Batch: batch, Receipt: receipt}
			return nil
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if commit.Status != "published" || commit.Revision != command.ExpectedRevision {
			return conflict("Episode Structure batch changed before confirmation")
		}
		plan, err := repo.GetPlan(ctx, actor, commit.PlanID, false)
		if err != nil {
			return err
		}
		if plan.Status != "materialized" || plan.ID != commit.PlanID {
			return conflict("Episode Structure batch plan has drifted")
		}
		batch, err := publishedStructureBatch(ctx, repo, actor, commit, true)
		if err != nil {
			return err
		}
		if batch.ContentHash != command.ExpectedContentHash {
			return conflict("Episode Structure batch content changed before confirmation")
		}
		now := service.config.Now().UTC()
		for index := range batch.Structures {
			structure := &batch.Structures[index]
			if structure.Status != "needs_review" {
				return conflict("Episode Structure changed before batch confirmation")
			}
			if err = validateStructureTasks(*structure); err != nil {
				return err
			}
			confirmedBy := actor.UserID
			structure.Status, structure.ConfirmedAt, structure.ConfirmedBy, structure.UpdatedAt = "confirmed", &now, &confirmedBy, now
			structure.Revision++
			if err = repo.SaveStructure(ctx, *structure); err != nil {
				return err
			}
		}
		receiptResult, err := platformcommand.Result(resourceReceipt{ID: commit.ID})
		if err != nil {
			return err
		}
		receipt := platformcommand.Receipt{
			ID: service.config.NewID(), WorkspaceID: commit.WorkspaceID, Operation: confirmStructureBatchOperation,
			IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: commit.ID,
			Result: receiptResult, CreatedBy: actor.UserID, CreatedAt: now,
		}
		if err = repo.CreateReceipt(ctx, receipt); err != nil {
			return err
		}
		result = ConfirmStructureBatchResult{Batch: batch, Receipt: receipt}
		return nil
	})
	return result, normalizeError(err)
}

type structureReference struct {
	StructureID     string `json:"structure_id"`
	EpisodeID       string `json:"episode_id"`
	ScriptVersionID string `json:"script_version_id"`
	ResultHash      string `json:"result_hash"`
}

func publishedStructureBatch(
	ctx context.Context,
	repo Repository,
	actor Actor,
	commit domain.ImportCommit,
	forUpdate bool,
) (PublishedStructureBatch, error) {
	if commit.Status != "published" || len(commit.Segments) == 0 {
		return PublishedStructureBatch{}, conflict("Episode structures are not published")
	}
	structures := make([]domain.Structure, len(commit.Segments))
	references := make([]structureReference, len(commit.Segments))
	seenStructures := make(map[string]struct{}, len(commit.Segments))
	seenEpisodes := make(map[string]struct{}, len(commit.Segments))
	for index, segment := range commit.Segments {
		if segment.PublishedVersionID == nil || *segment.PublishedVersionID == "" {
			return PublishedStructureBatch{}, conflict("Episode structure publication is incomplete")
		}
		structure, err := repo.GetEpisodeStructure(ctx, actor, segment.EpisodeID)
		if err != nil {
			return PublishedStructureBatch{}, err
		}
		if forUpdate {
			locked, lockErr := repo.GetStructure(ctx, actor, structure.ID, true)
			if lockErr != nil {
				return PublishedStructureBatch{}, lockErr
			}
			structure = locked
		}
		if structure.WorkspaceID != commit.WorkspaceID || structure.ProjectID != commit.ProjectID ||
			structure.EpisodeID != segment.EpisodeID || structure.ScriptVersionID != *segment.PublishedVersionID ||
			len(structure.ResultHash) != 64 || structure.Status == "superseded" {
			return PublishedStructureBatch{}, conflict("Episode structure publication has drifted")
		}
		if _, exists := seenStructures[structure.ID]; exists {
			return PublishedStructureBatch{}, conflict("Episode structure publication contains duplicates")
		}
		if _, exists := seenEpisodes[structure.EpisodeID]; exists {
			return PublishedStructureBatch{}, conflict("Episode structure publication contains duplicates")
		}
		seenStructures[structure.ID], seenEpisodes[structure.EpisodeID] = struct{}{}, struct{}{}
		structures[index] = structure
		references[index] = structureReference{
			StructureID: structure.ID, EpisodeID: structure.EpisodeID,
			ScriptVersionID: structure.ScriptVersionID, ResultHash: structure.ResultHash,
		}
	}
	contentHash, err := platformcommand.InputHash(references)
	if err != nil {
		return PublishedStructureBatch{}, err
	}
	return PublishedStructureBatch{Commit: commit, Structures: structures, ContentHash: contentHash}, nil
}

func (service *Service) ConfirmPlan(ctx context.Context, actor Actor, command ConfirmPlanCommand) (ConfirmPlanResult, error) {
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if strings.TrimSpace(command.PlanID) == "" || command.ExpectedRevision < 1 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return ConfirmPlanResult{}, invalid("Invalid episode plan confirmation")
	}
	var result ConfirmPlanResult
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		plan, err := repo.GetPlan(ctx, actor, command.PlanID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, plan.WorkspaceID, confirmPlanOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[resourceReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			result.View.Plan, replayErr = repo.GetPlan(ctx, actor, replayed.ID, false)
			if replayErr != nil {
				return replayErr
			}
			result.View.Impact, replayErr = repo.ProjectImpact(ctx, actor, result.View.Plan.ProjectID, false)
			result.View.Impact.ProjectedEpisodeCount += len(result.View.Plan.Proposals)
			result.Receipt = receipt
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if plan.Status != "review_ready" || plan.Revision != command.ExpectedRevision {
			return conflict("Episode plan changed before confirmation")
		}
		now := service.config.Now().UTC()
		plan.Status, plan.ConfirmedAt, plan.ConfirmedBy, plan.UpdatedAt = "confirmed", &now, &actor.UserID, now
		plan.Revision++
		for index := range plan.Proposals {
			plan.Proposals[index].IsLocked = true
		}
		if err = repo.SavePlan(ctx, plan); err != nil {
			return err
		}
		receiptResult, err := platformcommand.Result(resourceReceipt{ID: plan.ID})
		if err != nil {
			return err
		}
		receipt := platformcommand.Receipt{ID: service.config.NewID(), WorkspaceID: plan.WorkspaceID, Operation: confirmPlanOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: plan.ID, Result: receiptResult, CreatedBy: actor.UserID, CreatedAt: now}
		if err = repo.CreateReceipt(ctx, receipt); err != nil {
			return err
		}
		result.View.Plan = plan
		result.View.Impact, err = repo.ProjectImpact(ctx, actor, plan.ProjectID, false)
		result.View.Impact.ProjectedEpisodeCount += len(plan.Proposals)
		result.Receipt = receipt
		return err
	})
	return result, normalizeError(err)
}

func (service *Service) Materialize(ctx context.Context, actor Actor, command MaterializeCommand) (domain.ImportCommit, error) {
	if command.Mode != "append_new" || command.ExpectedPlanRevision < 1 || command.ExpectedProjectRevision < 1 || len(command.ExpectedActiveOrderHash) != 64 || strings.TrimSpace(command.IdempotencyKey) == "" {
		return domain.ImportCommit{}, invalid("Invalid episode materialization request")
	}
	var commit domain.ImportCommit
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		plan, err := repo.GetPlan(ctx, actor, command.PlanID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, plan.WorkspaceID, materializeOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[resourceReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			commit, replayErr = repo.GetCommit(ctx, actor, replayed.ID, false)
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		impact, err := repo.ProjectImpact(ctx, actor, plan.ProjectID, true)
		if err != nil {
			return err
		}
		if plan.Status != "confirmed" || plan.Revision != command.ExpectedPlanRevision || impact.ProjectRevision != command.ExpectedProjectRevision || impact.ActiveOrderHash != command.ExpectedActiveOrderHash {
			return conflict("Episode materialization inputs changed")
		}
		confirmed, err := repo.HasConfirmedBible(ctx, plan.ProjectID, plan.DocumentRevisionID)
		if err != nil {
			return err
		}
		if !confirmed {
			return conflict("A confirmed Production Bible is required before episode materialization")
		}
		now, commitID := service.config.Now().UTC(), service.config.NewID()
		commit = domain.ImportCommit{ID: commitID, WorkspaceID: plan.WorkspaceID, ProjectID: plan.ProjectID, PlanID: plan.ID, Mode: "append_new", Status: "materialized", InputHash: inputHash, ExpectedProjectRevision: command.ExpectedProjectRevision, ExpectedActiveOrderHash: command.ExpectedActiveOrderHash, Revision: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now, Segments: []domain.Segment{}}
		episodes, versions := make([]domain.Episode, len(plan.Proposals)), make([]Version, len(plan.Proposals))
		text := []rune(plan.Source.NormalizedText)
		for index, proposal := range plan.Proposals {
			episodeID, versionID := service.config.NewID(), service.config.NewID()
			content := string(text[proposal.SourceStart:proposal.SourceEnd])
			episodes[index] = domain.Episode{ID: episodeID, WorkspaceID: plan.WorkspaceID, ProjectID: plan.ProjectID, Name: proposal.Title, Position: impact.ActiveEpisodeCount + index + 1, TargetDurationMS: proposal.EstimatedDurationMS, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now}
			versions[index] = Version{ID: versionID, WorkspaceID: plan.WorkspaceID, ProjectID: plan.ProjectID, EpisodeID: episodeID, DocumentRevisionID: plan.DocumentRevisionID, VersionNo: 1, SourceStart: proposal.SourceStart, SourceEnd: proposal.SourceEnd, Content: content, ContentHash: proposal.ContentHash, Status: "draft", CreatedBy: actor.UserID, CreatedAt: now}
			commit.Segments = append(commit.Segments, domain.Segment{ID: service.config.NewID(), ImportCommitID: commitID, ProposalID: proposal.ID, DocumentRevisionID: plan.DocumentRevisionID, EpisodeID: episodeID, SourceID: service.config.NewID(), DraftVersionID: versionID, Position: index + 1, SourceStart: proposal.SourceStart, SourceEnd: proposal.SourceEnd, SourceHash: proposal.ContentHash, Content: content})
		}
		plan.Status, plan.Revision, plan.UpdatedAt = "materialized", plan.Revision+1, now
		if err = repo.Materialize(ctx, plan, commit, episodes, versions); err != nil {
			return err
		}
		result, err := platformcommand.Result(resourceReceipt{ID: commit.ID})
		if err != nil {
			return err
		}
		return repo.CreateReceipt(ctx, platformcommand.Receipt{ID: service.config.NewID(), WorkspaceID: plan.WorkspaceID, Operation: materializeOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: commit.ID, Result: result, CreatedBy: actor.UserID, CreatedAt: now})
	})
	return commit, normalizeError(err)
}

func (service *Service) Publish(ctx context.Context, actor Actor, command PublishCommand) (domain.ImportCommit, error) {
	var commit domain.ImportCommit
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		commit, err = repo.GetCommit(ctx, actor, command.CommitID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, commit.WorkspaceID, publishCommitOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[resourceReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			commit, replayErr = repo.GetCommit(ctx, actor, replayed.ID, false)
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if commit.Status != "materialized" || commit.Revision != command.ExpectedRevision {
			return conflict("Episode import commit changed before publication")
		}
		plan, err := repo.GetPlan(ctx, actor, commit.PlanID, false)
		if err != nil {
			return err
		}
		confirmed, err := repo.HasConfirmedBible(ctx, commit.ProjectID, plan.DocumentRevisionID)
		if err != nil {
			return err
		}
		if !confirmed {
			return conflict("A confirmed Production Bible is required before episode publication")
		}
		now := service.config.Now().UTC()
		structures := make([]domain.Structure, len(commit.Segments))
		for index := range commit.Segments {
			segment := &commit.Segments[index]
			publishedID := segment.DraftVersionID
			segment.PublishedVersionID = &publishedID
			scenes := service.extractScenes(segment.Content)
			hash, hashErr := platformcommand.InputHash(scenes)
			if hashErr != nil {
				return hashErr
			}
			structures[index] = domain.Structure{ID: service.config.NewID(), WorkspaceID: commit.WorkspaceID, ProjectID: commit.ProjectID, EpisodeID: segment.EpisodeID, ScriptVersionID: segment.DraftVersionID, Status: "needs_review", ResultHash: hash, Revision: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now, Scenes: scenes}
		}
		commit.Status, commit.Revision, commit.UpdatedAt = "published", commit.Revision+1, now
		if err = repo.Publish(ctx, commit, structures); err != nil {
			return err
		}
		result, err := platformcommand.Result(resourceReceipt{ID: commit.ID})
		if err != nil {
			return err
		}
		return repo.CreateReceipt(ctx, platformcommand.Receipt{ID: service.config.NewID(), WorkspaceID: commit.WorkspaceID, Operation: publishCommitOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: commit.ID, Result: result, CreatedBy: actor.UserID, CreatedAt: now})
	})
	return commit, normalizeError(err)
}

func (service *Service) ListEpisodes(ctx context.Context, actor Actor, projectID string) ([]domain.Episode, error) {
	var values []domain.Episode
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		values, err = repo.ListEpisodes(ctx, actor, projectID)
		return err
	})
	return values, normalizeError(err)
}
func (service *Service) GetEpisode(ctx context.Context, actor Actor, episodeID string) (domain.Episode, error) {
	var value domain.Episode
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		value, err = repo.GetEpisode(ctx, actor, episodeID, false)
		return err
	})
	return value, normalizeError(err)
}
func (service *Service) GetEpisodeStructure(ctx context.Context, actor Actor, episodeID string) (domain.Structure, error) {
	var value domain.Structure
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		value, err = repo.GetEpisodeStructure(ctx, actor, episodeID)
		return err
	})
	return value, normalizeError(err)
}

func (service *Service) AcceptTask(ctx context.Context, actor Actor, command AcceptTaskCommand) (domain.Structure, error) {
	var value domain.Structure
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		structure, err := repo.GetStructure(ctx, actor, command.StructureID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, structure.WorkspaceID, acceptTaskOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[resourceReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			value, replayErr = repo.GetStructure(ctx, actor, replayed.ID, false)
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if structure.Status != "needs_review" || structure.Revision != command.ExpectedRevision {
			return conflict("Episode structure changed before task decision")
		}
		found := false
		for sceneIndex := range structure.Scenes {
			for taskIndex := range structure.Scenes[sceneIndex].Tasks {
				if structure.Scenes[sceneIndex].Tasks[taskIndex].ID == command.TaskID {
					structure.Scenes[sceneIndex].Tasks[taskIndex].Status = "accepted"
					found = true
				}
			}
		}
		if !found {
			return ErrNotFound
		}
		now := service.config.Now().UTC()
		structure.Revision++
		structure.UpdatedAt = now
		if err = repo.SaveStructure(ctx, structure); err != nil {
			return err
		}
		result, err := platformcommand.Result(resourceReceipt{ID: structure.ID})
		if err != nil {
			return err
		}
		if err = repo.CreateReceipt(ctx, platformcommand.Receipt{ID: service.config.NewID(), WorkspaceID: structure.WorkspaceID, Operation: acceptTaskOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: structure.ID, Result: result, CreatedBy: actor.UserID, CreatedAt: now}); err != nil {
			return err
		}
		value = structure
		return nil
	})
	return value, normalizeError(err)
}

func (service *Service) ConfirmStructure(ctx context.Context, actor Actor, command StructureCommand) (domain.Structure, error) {
	var value domain.Structure
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		structure, err := repo.GetStructure(ctx, actor, command.StructureID, true)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		if receipt, receiptErr := repo.FindReceipt(ctx, structure.WorkspaceID, confirmStructureOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[resourceReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			value, replayErr = repo.GetStructure(ctx, actor, replayed.ID, false)
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if structure.Status != "needs_review" || structure.Revision != command.ExpectedRevision {
			return conflict("Episode structure changed before confirmation")
		}
		if err = validateStructureTasks(structure); err != nil {
			return err
		}
		now := service.config.Now().UTC()
		structure.Status, structure.ConfirmedAt, structure.ConfirmedBy, structure.UpdatedAt = "confirmed", &now, &actor.UserID, now
		structure.Revision++
		if err = repo.SaveStructure(ctx, structure); err != nil {
			return err
		}
		result, err := platformcommand.Result(resourceReceipt{ID: structure.ID})
		if err != nil {
			return err
		}
		if err = repo.CreateReceipt(ctx, platformcommand.Receipt{ID: service.config.NewID(), WorkspaceID: structure.WorkspaceID, Operation: confirmStructureOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: structure.ID, Result: result, CreatedBy: actor.UserID, CreatedAt: now}); err != nil {
			return err
		}
		value = structure
		return nil
	})
	return value, normalizeError(err)
}

func validateStructureTasks(structure domain.Structure) error {
	for _, scene := range structure.Scenes {
		if len(scene.Tasks) < 1 || len(scene.Tasks) > 4 {
			return conflict("Each scene must contain one to four production tasks")
		}
		hasBreakdown := false
		for _, task := range scene.Tasks {
			if task.Kind == "shot_breakdown" {
				hasBreakdown = true
			}
			if task.Required && task.Status != "accepted" {
				return conflict("Required production tasks must be accepted")
			}
		}
		if !hasBreakdown {
			return conflict("Each scene requires a shot breakdown task")
		}
	}
	return nil
}

func (service *Service) explicitProposals(source domain.Source, targetDuration int) ([]domain.Proposal, error) {
	markers := make([]int, 0)
	for index, block := range source.Blocks {
		if block.Kind == "episode_marker" {
			markers = append(markers, index)
		}
	}
	if len(markers) == 0 {
		return nil, invalid("No explicit episode markers were found")
	}
	text := []rune(source.NormalizedText)
	proposals := make([]domain.Proposal, len(markers))
	for index, markerIndex := range markers {
		marker := source.Blocks[markerIndex]
		nextBlockIndex, sourceEnd := len(source.Blocks), len(text)
		if index+1 < len(markers) {
			nextBlockIndex = markers[index+1]
			sourceEnd = source.Blocks[nextBlockIndex].SourceStart
		}
		endBlock := source.Blocks[nextBlockIndex-1]
		title := "第 " + integerText(index+1) + " 集"
		for _, block := range source.Blocks[markerIndex+1 : nextBlockIndex] {
			candidate := strings.TrimSpace(string(text[block.SourceStart:block.SourceEnd]))
			if candidate == "" {
				continue
			}
			if strings.HasPrefix(candidate, "《") && strings.HasSuffix(candidate, "》") {
				title = strings.TrimSuffix(strings.TrimPrefix(candidate, "《"), "》")
			}
			break
		}
		content := string(text[marker.SourceStart:sourceEnd])
		hash := sha256.Sum256([]byte(content))
		proposals[index] = domain.Proposal{Position: index + 1, Title: title, StartBlockID: marker.ID, EndBlockID: endBlock.ID, StartBlockPosition: marker.Position, EndBlockPosition: endBlock.Position, SourceStart: marker.SourceStart, SourceEnd: sourceEnd, ContentHash: hex.EncodeToString(hash[:]), EstimatedDurationMS: targetDuration, Reason: "检测到连续分集标记，边界已锚定到不可变原稿。", Confidence: 1, BoundaryEvidence: map[string]any{"strategy": "explicit_markers", "episode_number": index + 1}}
	}
	return proposals, nil
}

func (service *Service) extractScenes(content string) []domain.Scene {
	type line struct {
		text       string
		start, end int
	}
	segments := strings.SplitAfter(content, "\n")
	lines := make([]line, 0, len(segments))
	offset := 0
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		raw := strings.TrimSuffix(segment, "\n")
		count := utf8.RuneCountInString(raw)
		lines = append(lines, line{raw, offset, offset + count})
		offset += utf8.RuneCountInString(segment)
	}
	starts := []int{}
	for index, item := range lines {
		trimmed := strings.TrimSpace(item.text)
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(trimmed, "内景") || strings.HasPrefix(trimmed, "外景") || strings.HasPrefix(upper, "INT.") || strings.HasPrefix(upper, "EXT.") {
			starts = append(starts, index)
		}
	}
	if len(starts) == 0 {
		starts = append(starts, 0)
	}
	scenes := make([]domain.Scene, len(starts))
	for index, startIndex := range starts {
		endIndex := len(lines)
		if index+1 < len(starts) {
			endIndex = starts[index+1]
		}
		start, end := lines[startIndex].start, utf8.RuneCountInString(content)
		if endIndex < len(lines) {
			end = lines[endIndex].start
		}
		heading := strings.TrimSpace(lines[startIndex].text)
		if heading == "" {
			heading = "未命名场景"
		}
		scene := domain.Scene{ID: service.config.NewID(), Heading: heading, Position: index + 1, SourceStart: start, SourceEnd: end, Dialogues: []domain.Dialogue{}, NarrativeUnits: []domain.NarrativeUnit{}, Tasks: []domain.ProductionTask{{ID: service.config.NewID(), Kind: "shot_breakdown", Label: "将本场拆解为可执行分镜", Status: "pending", Required: true}}}
		for _, item := range lines[startIndex+1 : endIndex] {
			trimmed := strings.TrimSpace(item.text)
			if trimmed == "" {
				continue
			}
			if match := dialogueLine.FindStringSubmatch(trimmed); match != nil {
				scene.Dialogues = append(scene.Dialogues, domain.Dialogue{ID: service.config.NewID(), Speaker: strings.TrimSpace(match[1]), Text: strings.TrimSpace(match[2]), SourceStart: item.start, SourceEnd: item.end})
			} else {
				scene.NarrativeUnits = append(scene.NarrativeUnits, domain.NarrativeUnit{ID: service.config.NewID(), Kind: "action", Text: trimmed, SourceStart: item.start, SourceEnd: item.end})
			}
		}
		scenes[index] = scene
	}
	return scenes
}

func integerText(value int) string {
	const digits = "0123456789"
	if value < 10 {
		return string(digits[value])
	}
	return "多"
}
func invalid(message string) error {
	return &Error{Code: "validation_failed", Message: message, Status: 422}
}
func conflict(message string) error {
	return &Error{Code: "resource_conflict", Message: message, Status: 409}
}
func normalizeError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return &Error{Code: "not_found", Message: "Planning resource not found", Status: 404}
	}
	return err
}
