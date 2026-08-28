package production

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	planningapp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	planningdomain "github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
	storyboardapp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const (
	productionBibleConfirmOperation       = "production_bible.confirm"
	episodePlanConfirmOperation           = "episode_plan.confirm"
	episodePlanApplyOperation             = "episode_plan.apply"
	episodePlanningApplyOperation         = "episode_planning.apply"
	episodeStructureBatchConfirmOperation = "episode_structure.confirm_batch"
	storyboardFreezeIntentSetOperation    = "storyboard.freeze_intent_set"
)

type BibleOwner interface {
	Confirm(context.Context, bibleapp.Actor, bibleapp.ConfirmCommand) (bibleapp.ConfirmResult, error)
}

type PlanningConfirmationOwner interface {
	ConfirmPlan(context.Context, planningapp.Actor, planningapp.ConfirmPlanCommand) (planningapp.ConfirmPlanResult, error)
	ApplyEpisodePlan(context.Context, planningapp.Actor, planningapp.ApplyEpisodePlanCommand) (planningapp.ApplyEpisodePlanResult, error)
	ConfirmPublishedStructureBatch(context.Context, planningapp.Actor, planningapp.ConfirmStructureBatchCommand) (planningapp.ConfirmStructureBatchResult, error)
}

type EpisodePlanningOwner interface {
	ApplyEpisodePlanningCandidate(context.Context, planningapp.Actor, planningapp.ApplyEpisodePlanningCandidateCommand) (planningapp.ApplyEpisodePlanningCandidateResult, error)
}

type StoryboardSetOwner interface {
	FreezeIntentSet(context.Context, storyboardapp.Actor, storyboardapp.FreezeIntentSetCommand) (storyboardapp.FreezeIntentSetResult, error)
}

type Applier struct {
	bibles             BibleOwner
	plans              PlanningConfirmationOwner
	planningCandidates EpisodePlanningOwner
	storyboards        StoryboardSetOwner
}

func New(
	bibles BibleOwner,
	plans PlanningConfirmationOwner,
	planningCandidates EpisodePlanningOwner,
	storyboards StoryboardSetOwner,
) *Applier {
	return &Applier{bibles: bibles, plans: plans, planningCandidates: planningCandidates, storyboards: storyboards}
}

func (applier *Applier) ApplyHumanGateDecision(
	ctx context.Context,
	actor workflowapp.Actor,
	application domain.HumanGateOwnerApplication,
) (domain.HumanGateOwnerResult, error) {
	if applier == nil || strings.TrimSpace(actor.UserID) == "" ||
		(application.Decision != "approved" && application.Decision != "selected") ||
		strings.TrimSpace(application.Candidate.ReferenceID) == "" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported workflow human gate owner application")
	}
	if application.Executor == "gate.episode_plan_review" {
		return applier.applyEpisodePlan(ctx, actor, application)
	}
	if application.Executor == "gate.episode_structure_review" {
		return applier.applyEpisodeStructures(ctx, actor, application)
	}
	if application.Executor == "gate.storyboard_review" {
		return applier.freezeStoryboardIntents(ctx, actor, application)
	}
	if applier.bibles == nil || application.Executor != "gate.production_bible_review" ||
		application.Candidate.ValueType != "story_reconciliation_candidate" ||
		application.OutputPort != "bible" || application.OutputValueType != "production_bible_version" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported workflow human gate owner application")
	}
	expectedCandidateRevision, err := strconv.ParseInt(application.Candidate.ReferenceVersion, 10, 64)
	if err != nil || expectedCandidateRevision < 1 {
		return domain.HumanGateOwnerResult{}, errors.New("invalid production bible candidate revision")
	}
	var config struct {
		ExpectedVersion int `json:"expected_bible_version"`
	}
	if json.Unmarshal(application.NodeConfig, &config) != nil || config.ExpectedVersion < 1 {
		return domain.HumanGateOwnerResult{}, errors.New("invalid Production Bible Human Gate config")
	}
	var documentID, documentHash string
	for _, reference := range application.FrozenInputs {
		if reference.Kind != "script_revision" {
			continue
		}
		if documentID != "" {
			return domain.HumanGateOwnerResult{}, errors.New("Production Bible Human Gate has multiple script revisions")
		}
		documentID, documentHash = reference.ID, reference.Hash
	}
	if documentID == "" || len(documentHash) != 64 {
		return domain.HumanGateOwnerResult{}, errors.New("Production Bible Human Gate has no frozen script revision")
	}
	result, err := applier.bibles.Confirm(ctx, bibleapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, bibleapp.ConfirmCommand{
		CandidateRevisionID:       application.Candidate.ReferenceID,
		CandidateRevisionHash:     application.Candidate.ContentHash,
		ExpectedCandidateRevision: expectedCandidateRevision,
		DocumentRevisionID:        documentID, DocumentRevisionHash: documentHash,
		ExpectedVersion: config.ExpectedVersion, ReviewDecisionID: application.ReviewDecisionID,
		IdempotencyKey: "workflow-review:" + application.ReviewDecisionID,
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, normalizeOwnerApplyError(err)
	}
	if err = validateConfirmedBible(application, actor, result.Version, result.Receipt.Operation, result.Receipt.ResourceID,
		result.Receipt.WorkspaceID, result.Receipt.CreatedBy); err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	output, _, outputHash, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: application.OutputPort, ValueType: application.OutputValueType,
			ReferenceID: result.Version.ID, ReferenceVersion: strconv.Itoa(result.Version.Version),
			ContentHash: result.Version.ContentHash,
		}},
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, normalizeOwnerApplyError(err)
	}
	return domain.HumanGateOwnerResult{
		ReceiptID: result.Receipt.ID, Operation: result.Receipt.Operation, Output: output, OutputHash: outputHash,
	}, nil
}

func (applier *Applier) freezeStoryboardIntents(
	ctx context.Context,
	actor workflowapp.Actor,
	application domain.HumanGateOwnerApplication,
) (domain.HumanGateOwnerResult, error) {
	if applier.storyboards == nil || application.Candidate.ValueType != "storyboard_intent_candidate_set" ||
		application.OutputPort != "intents" || application.OutputValueType != "approved_storyboard_intents" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported workflow human gate owner application")
	}
	expectedRevision, err := strconv.ParseInt(application.Candidate.ReferenceVersion, 10, 64)
	if err != nil || expectedRevision < 1 || len(application.Candidate.ContentHash) != 64 {
		return domain.HumanGateOwnerResult{}, errors.New("invalid Storyboard Intent Candidate")
	}
	result, err := applier.storyboards.FreezeIntentSet(ctx, storyboardapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, storyboardapp.FreezeIntentSetCommand{
		WorkspaceID: application.WorkspaceID, ProjectID: application.ProjectID,
		CandidateRevisionID:       application.Candidate.ReferenceID,
		CandidateRevisionHash:     application.Candidate.ContentHash,
		ExpectedCandidateRevision: expectedRevision, ReviewDecisionID: application.ReviewDecisionID,
		IdempotencyKey: "workflow-review:" + application.ReviewDecisionID,
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, normalizeOwnerApplyError(err)
	}
	set, approved := result.Set, result.Approved
	if approved.ID != result.Receipt.ID || approved.WorkspaceID != application.WorkspaceID ||
		approved.ProjectID != application.ProjectID || approved.DraftSetID != set.ID ||
		approved.CandidateRevisionID != application.Candidate.ReferenceID ||
		approved.CandidateRevisionHash != application.Candidate.ContentHash ||
		approved.CandidateRevision != expectedRevision || approved.ReviewDecisionID != application.ReviewDecisionID ||
		len(approved.Scenes) == 0 || len(approved.VisualRequirementsHash) != 64 || len(approved.ContentHash) != 64 ||
		set.WorkspaceID != application.WorkspaceID || set.ProjectID != application.ProjectID ||
		set.Status != "intent_frozen" || set.Revision != approved.DraftSetRevision+1 ||
		set.ResultHash == nil || *set.ResultHash != approved.ContentHash ||
		result.Receipt.Operation != storyboardFreezeIntentSetOperation || result.Receipt.ResourceID != set.ID ||
		result.Receipt.WorkspaceID != application.WorkspaceID || result.Receipt.CreatedBy != actor.UserID {
		return domain.HumanGateOwnerResult{}, errors.New("Storyboard owner result does not match workflow gate")
	}
	output, _, outputHash, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: application.OutputPort, ValueType: application.OutputValueType,
			ReferenceID: approved.ID, ReferenceVersion: "1", ContentHash: approved.ContentHash,
		}},
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, normalizeOwnerApplyError(err)
	}
	return domain.HumanGateOwnerResult{
		ReceiptID: result.Receipt.ID, Operation: result.Receipt.Operation, Output: output, OutputHash: outputHash,
	}, nil
}

func (applier *Applier) applyEpisodeStructures(
	ctx context.Context,
	actor workflowapp.Actor,
	application domain.HumanGateOwnerApplication,
) (domain.HumanGateOwnerResult, error) {
	if application.Candidate.ValueType == "episode_planning_candidate_set" {
		return applier.applyEpisodePlanning(ctx, actor, application)
	}
	if applier.plans == nil || application.Candidate.ValueType != "episode_structure_candidate" ||
		application.OutputPort != "structures" || application.OutputValueType != "episode_structures" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported workflow human gate owner application")
	}
	expectedRevision, err := strconv.Atoi(application.Candidate.ReferenceVersion)
	if err != nil || expectedRevision < 1 || len(application.Candidate.ContentHash) != 64 {
		return domain.HumanGateOwnerResult{}, errors.New("invalid Episode Structure batch candidate")
	}
	result, err := applier.plans.ConfirmPublishedStructureBatch(ctx, planningapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, planningapp.ConfirmStructureBatchCommand{
		CommitID: application.Candidate.ReferenceID, ExpectedRevision: expectedRevision,
		ExpectedContentHash: application.Candidate.ContentHash,
		IdempotencyKey:      "workflow-review:" + application.ReviewDecisionID,
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, normalizeOwnerApplyError(err)
	}
	batch := result.Batch
	if batch.Commit.ID != application.Candidate.ReferenceID || batch.Commit.Status != "published" ||
		batch.Commit.Revision != expectedRevision || batch.Commit.WorkspaceID != application.WorkspaceID ||
		batch.Commit.ProjectID != application.ProjectID || batch.ContentHash != application.Candidate.ContentHash ||
		len(batch.Structures) == 0 || result.Receipt.Operation != episodeStructureBatchConfirmOperation ||
		result.Receipt.ResourceID != batch.Commit.ID || result.Receipt.WorkspaceID != application.WorkspaceID ||
		result.Receipt.CreatedBy != actor.UserID {
		return domain.HumanGateOwnerResult{}, errors.New("Episode Structure owner result does not match workflow gate")
	}
	for _, structure := range batch.Structures {
		if structure.WorkspaceID != application.WorkspaceID || structure.ProjectID != application.ProjectID ||
			structure.Status != "confirmed" || structure.ConfirmedBy == nil || *structure.ConfirmedBy != actor.UserID {
			return domain.HumanGateOwnerResult{}, errors.New("Episode Structure owner result does not match workflow gate")
		}
	}
	output, _, outputHash, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: application.OutputPort, ValueType: application.OutputValueType,
			ReferenceID: batch.Commit.ID, ReferenceVersion: strconv.Itoa(batch.Commit.Revision),
			ContentHash: batch.ContentHash,
		}},
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, normalizeOwnerApplyError(err)
	}
	return domain.HumanGateOwnerResult{
		ReceiptID: result.Receipt.ID, Operation: result.Receipt.Operation, Output: output, OutputHash: outputHash,
	}, nil
}

func (applier *Applier) applyEpisodePlanning(
	ctx context.Context,
	actor workflowapp.Actor,
	application domain.HumanGateOwnerApplication,
) (domain.HumanGateOwnerResult, error) {
	if applier.planningCandidates == nil || application.OutputPort != "structures" ||
		application.OutputValueType != "planning_owner_set" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported Episode Planning Human Gate owner application")
	}
	expectedRevision, err := strconv.ParseInt(application.Candidate.ReferenceVersion, 10, 64)
	if err != nil || expectedRevision < 1 || len(application.Candidate.ContentHash) != 64 {
		return domain.HumanGateOwnerResult{}, errors.New("invalid Episode Planning Candidate revision")
	}
	result, err := applier.planningCandidates.ApplyEpisodePlanningCandidate(ctx, planningapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, planningapp.ApplyEpisodePlanningCandidateCommand{
		WorkspaceID: application.WorkspaceID, ProjectID: application.ProjectID,
		CandidateRevisionID:       application.Candidate.ReferenceID,
		CandidateRevisionHash:     application.Candidate.ContentHash,
		ExpectedCandidateRevision: expectedRevision, ReviewDecisionID: application.ReviewDecisionID,
		IdempotencyKey: "workflow-review:" + application.ReviewDecisionID,
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	set := result.Set
	if set.ID != result.Receipt.ID || set.WorkspaceID != application.WorkspaceID || set.ProjectID != application.ProjectID ||
		set.CandidateRevisionID != application.Candidate.ReferenceID ||
		set.CandidateRevisionHash != application.Candidate.ContentHash || set.CandidateRevision != expectedRevision ||
		set.ReviewDecisionID != application.ReviewDecisionID || len(set.ContentHash) != 64 || len(set.Structures) == 0 ||
		result.Receipt.Operation != episodePlanningApplyOperation ||
		result.Receipt.ResourceID != application.Candidate.ReferenceID || result.Receipt.WorkspaceID != application.WorkspaceID ||
		result.Receipt.CreatedBy != actor.UserID {
		return domain.HumanGateOwnerResult{}, errors.New("Episode Planning owner result does not match workflow gate")
	}
	output, _, outputHash, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: application.OutputPort, ValueType: application.OutputValueType,
			ReferenceID: set.ID, ReferenceVersion: "1", ContentHash: set.ContentHash,
		}},
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	return domain.HumanGateOwnerResult{
		ReceiptID: result.Receipt.ID, Operation: result.Receipt.Operation, Output: output, OutputHash: outputHash,
	}, nil
}

func (applier *Applier) applyEpisodePlan(
	ctx context.Context,
	actor workflowapp.Actor,
	application domain.HumanGateOwnerApplication,
) (domain.HumanGateOwnerResult, error) {
	if application.Candidate.ValueType == "episode_segmentation_candidate" {
		return applier.applyEpisodeSegmentation(ctx, actor, application)
	}
	if applier.plans == nil || application.Candidate.ValueType != "episode_plan_candidate" ||
		application.OutputPort != "episodes" || application.OutputValueType != "episode_plan" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported workflow human gate owner application")
	}
	expectedRevision, err := strconv.Atoi(application.Candidate.ReferenceVersion)
	if err != nil || expectedRevision < 1 {
		return domain.HumanGateOwnerResult{}, errors.New("invalid Episode Plan candidate revision")
	}
	result, err := applier.plans.ConfirmPlan(ctx, planningapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, planningapp.ConfirmPlanCommand{
		PlanID: application.Candidate.ReferenceID, ExpectedRevision: expectedRevision,
		IdempotencyKey: "workflow-review:" + application.ReviewDecisionID,
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	plan := result.View.Plan
	if err = validateConfirmedEpisodePlan(application, actor, plan, expectedRevision,
		result.Receipt.Operation, result.Receipt.ResourceID, result.Receipt.WorkspaceID, result.Receipt.CreatedBy); err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	output, _, outputHash, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: application.OutputPort, ValueType: application.OutputValueType,
			ReferenceID: plan.ID, ReferenceVersion: strconv.Itoa(plan.Revision), ContentHash: plan.InputHash,
		}},
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	return domain.HumanGateOwnerResult{
		ReceiptID: result.Receipt.ID, Operation: result.Receipt.Operation, Output: output, OutputHash: outputHash,
	}, nil
}

func (applier *Applier) applyEpisodeSegmentation(
	ctx context.Context,
	actor workflowapp.Actor,
	application domain.HumanGateOwnerApplication,
) (domain.HumanGateOwnerResult, error) {
	if applier.plans == nil || application.OutputPort != "episodes" || application.OutputValueType != "episode_set" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported Episode segmentation Human Gate owner application")
	}
	expectedRevision, err := strconv.ParseInt(application.Candidate.ReferenceVersion, 10, 64)
	if err != nil || expectedRevision < 1 || len(application.Candidate.ContentHash) != 64 {
		return domain.HumanGateOwnerResult{}, errors.New("invalid Episode segmentation Candidate revision")
	}
	result, err := applier.plans.ApplyEpisodePlan(ctx, planningapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, planningapp.ApplyEpisodePlanCommand{
		CandidateRevisionID: application.Candidate.ReferenceID, CandidateRevisionHash: application.Candidate.ContentHash,
		ExpectedCandidateRevision: expectedRevision, ReviewDecisionID: application.ReviewDecisionID,
		IdempotencyKey: "workflow-review:" + application.ReviewDecisionID,
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	set := result.Set
	if set.ID != result.Receipt.ID || set.WorkspaceID != application.WorkspaceID || set.ProjectID != application.ProjectID ||
		set.CandidateRevisionID != application.Candidate.ReferenceID ||
		set.CandidateRevisionHash != application.Candidate.ContentHash || set.CandidateRevision != expectedRevision ||
		len(set.ContentHash) != 64 || len(set.Episodes) == 0 || result.Receipt.Operation != episodePlanApplyOperation ||
		result.Receipt.ResourceID != application.Candidate.ReferenceID || result.Receipt.WorkspaceID != application.WorkspaceID ||
		result.Receipt.CreatedBy != actor.UserID {
		return domain.HumanGateOwnerResult{}, errors.New("Episode Plan owner result does not match workflow gate")
	}
	output, _, outputHash, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: application.OutputPort, ValueType: application.OutputValueType,
			ReferenceID: set.ID, ReferenceVersion: "1", ContentHash: set.ContentHash,
		}},
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	return domain.HumanGateOwnerResult{
		ReceiptID: result.Receipt.ID, Operation: result.Receipt.Operation, Output: output, OutputHash: outputHash,
	}, nil
}

func normalizeOwnerApplyError(err error) error {
	var workflowError *workflowapp.Error
	if errors.As(err, &workflowError) {
		return err
	}
	var bibleError *bibleapp.Error
	if errors.As(err, &bibleError) {
		return &workflowapp.Error{
			Code: bibleError.Code, Message: bibleError.Message,
			NextAction: bibleError.NextAction, Status: bibleError.Status,
		}
	}
	var planningError *planningapp.Error
	if errors.As(err, &planningError) {
		return &workflowapp.Error{
			Code: planningError.Code, Message: planningError.Message,
			NextAction: planningError.NextAction, Status: planningError.Status,
		}
	}
	var storyboardError *storyboardapp.Error
	if errors.As(err, &storyboardError) {
		return &workflowapp.Error{
			Code: storyboardError.Code, Message: storyboardError.Message,
			NextAction: storyboardError.NextAction, Status: storyboardError.Status,
		}
	}
	return err
}

func validateConfirmedBible(
	application domain.HumanGateOwnerApplication,
	actor workflowapp.Actor,
	bible bibledomain.ProductionBibleVersion,
	operation string,
	resourceID string,
	workspaceID string,
	createdBy string,
) error {
	if bible.ID == application.Candidate.ReferenceID || bible.CandidateRevisionID != application.Candidate.ReferenceID ||
		bible.CandidateRevisionHash != application.Candidate.ContentHash || bible.WorkspaceID != application.WorkspaceID ||
		bible.ProjectID != application.ProjectID || bible.ReviewDecisionID != application.ReviewDecisionID ||
		operation != productionBibleConfirmOperation || resourceID != bible.ID ||
		workspaceID != application.WorkspaceID || createdBy != actor.UserID {
		return errors.New("production bible owner result does not match workflow gate")
	}
	return nil
}

func validateConfirmedEpisodePlan(
	application domain.HumanGateOwnerApplication,
	actor workflowapp.Actor,
	plan planningdomain.Plan,
	expectedRevision int,
	operation string,
	resourceID string,
	workspaceID string,
	createdBy string,
) error {
	if plan.Status != "confirmed" || plan.ID != application.Candidate.ReferenceID || plan.Revision != expectedRevision+1 ||
		plan.WorkspaceID != application.WorkspaceID || plan.ProjectID != application.ProjectID ||
		plan.InputHash != application.Candidate.ContentHash || operation != episodePlanConfirmOperation ||
		resourceID != plan.ID || workspaceID != application.WorkspaceID || createdBy != actor.UserID {
		return errors.New("Episode Plan owner result does not match workflow gate")
	}
	return nil
}

var _ workflowapp.HumanGateOwnerApplier = (*Applier)(nil)
