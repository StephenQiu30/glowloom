package execution

import (
	"context"
	"errors"

	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	planningapp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	storyboardapp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type HumanGateOwnerRouter struct {
	production workflowapp.HumanGateOwnerApplier
	generation workflowapp.HumanGateOwnerApplier
}

func NewHumanGateOwnerRouter(
	production workflowapp.HumanGateOwnerApplier,
	generation workflowapp.HumanGateOwnerApplier,
) (*HumanGateOwnerRouter, error) {
	if production == nil || generation == nil {
		return nil, errors.New("Workflow Human Gate owners are required")
	}
	return &HumanGateOwnerRouter{production: production, generation: generation}, nil
}

func (router *HumanGateOwnerRouter) ApplyHumanGateDecision(
	ctx context.Context,
	actor workflowapp.Actor,
	application domain.HumanGateOwnerApplication,
) (domain.HumanGateOwnerResult, error) {
	if router == nil || router.production == nil || router.generation == nil {
		return domain.HumanGateOwnerResult{}, errors.New("Workflow Human Gate owners are unavailable")
	}
	var (
		result domain.HumanGateOwnerResult
		err    error
	)
	switch application.Executor {
	case "gate.production_bible_review", "gate.episode_plan_review", "gate.episode_structure_review", "gate.storyboard_review":
		result, err = router.production.ApplyHumanGateDecision(ctx, actor, application)
	case "gate.generation_image_review":
		result, err = router.generation.ApplyHumanGateDecision(ctx, actor, application)
	default:
		return domain.HumanGateOwnerResult{}, &workflowapp.Error{
			Code: "resource_conflict", Message: "Unsupported Workflow Human Gate owner", Status: 409,
		}
	}
	return result, normalizeHumanGateOwnerError(err)
}

func normalizeHumanGateOwnerError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, bibleapp.ErrNotFound) || errors.Is(err, planningapp.ErrNotFound) ||
		errors.Is(err, storyboardapp.ErrNotFound) || errors.Is(err, generationapp.ErrNotFound) {
		return workflowapp.ErrNotFound
	}
	var bibleError *bibleapp.Error
	if errors.As(err, &bibleError) {
		return &workflowapp.Error{
			Code: bibleError.Code, Message: bibleError.Message, Status: bibleError.Status, NextAction: bibleError.NextAction,
		}
	}
	var planningError *planningapp.Error
	if errors.As(err, &planningError) {
		return &workflowapp.Error{
			Code: planningError.Code, Message: planningError.Message, Status: planningError.Status, NextAction: planningError.NextAction,
		}
	}
	var storyboardError *storyboardapp.Error
	if errors.As(err, &storyboardError) {
		return &workflowapp.Error{
			Code: storyboardError.Code, Message: storyboardError.Message, Status: storyboardError.Status, NextAction: storyboardError.NextAction,
		}
	}
	var generationError *generationapp.Error
	if errors.As(err, &generationError) {
		return &workflowapp.Error{
			Code: generationError.Code, Message: generationError.Message, Status: generationError.Status, NextAction: generationError.NextAction,
		}
	}
	return err
}

var _ workflowapp.HumanGateOwnerApplier = (*HumanGateOwnerRouter)(nil)
