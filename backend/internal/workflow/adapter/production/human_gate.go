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
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const (
	productionBibleConfirmOperation = "production_bible.confirm"
	episodePlanConfirmOperation     = "episode_plan.confirm"
)

type BibleOwner interface {
	Confirm(context.Context, bibleapp.Actor, bibleapp.ConfirmCommand) (bibleapp.ConfirmResult, error)
}

type EpisodePlanConfirmationOwner interface {
	ConfirmPlan(context.Context, planningapp.Actor, planningapp.ConfirmPlanCommand) (planningapp.ConfirmPlanResult, error)
}

type Applier struct {
	bibles BibleOwner
	plans  EpisodePlanConfirmationOwner
}

func New(bibles BibleOwner, plans EpisodePlanConfirmationOwner) *Applier {
	return &Applier{bibles: bibles, plans: plans}
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
