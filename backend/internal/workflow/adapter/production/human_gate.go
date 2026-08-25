package production

import (
	"context"
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
	episodeStructureBatchConfirmOperation = "episode_structure.confirm_batch"
	storyboardApplySetOperation           = "storyboard.apply_set"
)

type BibleOwner interface {
	Confirm(context.Context, bibleapp.Actor, bibleapp.ConfirmCommand) (bibleapp.ConfirmResult, error)
}

type PlanningConfirmationOwner interface {
	ConfirmPlan(context.Context, planningapp.Actor, planningapp.ConfirmPlanCommand) (planningapp.ConfirmPlanResult, error)
	ConfirmPublishedStructureBatch(context.Context, planningapp.Actor, planningapp.ConfirmStructureBatchCommand) (planningapp.ConfirmStructureBatchResult, error)
}

type StoryboardSetOwner interface {
	ApplySet(context.Context, storyboardapp.Actor, storyboardapp.ApplySetCommand) (storyboardapp.ApplySetResult, error)
}

type Applier struct {
	bibles      BibleOwner
	plans       PlanningConfirmationOwner
	storyboards StoryboardSetOwner
}

func New(bibles BibleOwner, plans PlanningConfirmationOwner, storyboards StoryboardSetOwner) *Applier {
	return &Applier{bibles: bibles, plans: plans, storyboards: storyboards}
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
		return applier.applyStoryboardSet(ctx, actor, application)
	}
	if applier.bibles == nil || application.Executor != "gate.production_bible_review" ||
		application.Candidate.ValueType != "production_bible_candidate" ||
		application.OutputPort != "bible" || application.OutputValueType != "production_bible" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported workflow human gate owner application")
	}
	expectedRevision, err := strconv.Atoi(application.Candidate.ReferenceVersion)
	if err != nil || expectedRevision < 1 {
		return domain.HumanGateOwnerResult{}, errors.New("invalid production bible candidate revision")
	}
	result, err := applier.bibles.Confirm(ctx, bibleapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, bibleapp.ConfirmCommand{
		BibleID: application.Candidate.ReferenceID, ExpectedResultHash: application.Candidate.ContentHash,
		ExpectedRevision: expectedRevision, IdempotencyKey: "workflow-review:" + application.ReviewDecisionID,
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	if err = validateConfirmedBible(application, actor, result.Bible, result.Receipt.Operation, result.Receipt.ResourceID,
		result.Receipt.WorkspaceID, result.Receipt.CreatedBy); err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	output, _, outputHash, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: application.OutputPort, ValueType: application.OutputValueType,
			ReferenceID: result.Bible.ID, ReferenceVersion: strconv.Itoa(result.Bible.Revision),
			ContentHash: *result.Bible.ResultHash,
		}},
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	return domain.HumanGateOwnerResult{
		ReceiptID: result.Receipt.ID, Operation: result.Receipt.Operation, Output: output, OutputHash: outputHash,
	}, nil
}

func (applier *Applier) applyStoryboardSet(
	ctx context.Context,
	actor workflowapp.Actor,
	application domain.HumanGateOwnerApplication,
) (domain.HumanGateOwnerResult, error) {
	if applier.storyboards == nil || application.Candidate.ValueType != "storyboard_candidate" ||
		application.OutputPort != "storyboards" || application.OutputValueType != "storyboards" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported workflow human gate owner application")
	}
	expectedRevision, err := strconv.Atoi(application.Candidate.ReferenceVersion)
	if err != nil || expectedRevision < 1 || len(application.Candidate.ContentHash) != 64 {
		return domain.HumanGateOwnerResult{}, errors.New("invalid Storyboard Draft Set candidate")
	}
	result, err := applier.storyboards.ApplySet(ctx, storyboardapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, storyboardapp.ApplySetCommand{
		SetID: application.Candidate.ReferenceID, ExpectedRevision: expectedRevision,
		ExpectedCandidateHash: application.Candidate.ContentHash,
		IdempotencyKey:        "workflow-review:" + application.ReviewDecisionID,
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	set := result.Set
	if set.ID != application.Candidate.ReferenceID || set.WorkspaceID != application.WorkspaceID ||
		set.ProjectID != application.ProjectID || set.Status != "applied" || set.Revision != expectedRevision+1 ||
		set.ResultHash == nil || len(*set.ResultHash) != 64 || result.Receipt.Operation != storyboardApplySetOperation ||
		result.Receipt.ResourceID != set.ID || result.Receipt.WorkspaceID != application.WorkspaceID ||
		result.Receipt.CreatedBy != actor.UserID {
		return domain.HumanGateOwnerResult{}, errors.New("Storyboard owner result does not match workflow gate")
	}
	output, _, outputHash, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: application.OutputPort, ValueType: application.OutputValueType,
			ReferenceID: set.ID, ReferenceVersion: strconv.Itoa(set.Revision), ContentHash: *set.ResultHash,
		}},
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
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
		return domain.HumanGateOwnerResult{}, err
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

func validateConfirmedBible(
	application domain.HumanGateOwnerApplication,
	actor workflowapp.Actor,
	bible bibledomain.Bible,
	operation string,
	resourceID string,
	workspaceID string,
	createdBy string,
) error {
	if bible.Status != "confirmed" || bible.ID != application.Candidate.ReferenceID ||
		bible.WorkspaceID != application.WorkspaceID || bible.ProjectID != application.ProjectID ||
		bible.ResultHash == nil || *bible.ResultHash != application.Candidate.ContentHash ||
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
