package bootstrap

import (
	"errors"
	"time"

	"github.com/google/uuid"

	planningapp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	storyboardapp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
	workflowexecution "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/execution"
	workflowgeneration "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/generation"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflowreview "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/review"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
)

type WorkflowRuntimeRepository interface {
	workflowapp.RuntimeRepository
	workflowapp.NodeRuntimeRepository
	workflowapp.NodeCacheRuntimeRepository
	workflowapp.RunCompletionRepository
	workflowapp.RunFailureRepository
	workflowapp.HumanGateRepository
	workflowapp.HumanGateApplyRepository
}

func NewWorkflowRuntime(
	repository WorkflowRuntimeRepository,
	scripts workflowproduction.ScriptSource,
	evidence workflowproduction.SourceEvidenceOwner,
	stories workflowproduction.StoryAnalysisOwner,
	storyReviews workflowproduction.StoryReviewOwner,
	bibles workflowproduction.BibleCandidateOwner,
	projects *projectapp.Service,
	plans *planningapp.Service,
	planningOwners *planningapp.EpisodePlanningService,
	storygraphs *storygraphapp.Service,
	storyboards *storyboardapp.Service,
	reviews *reviewapp.Service,
	bindings workflowproduction.ShotImageWorkflowOwner,
	candidateSets workflowreview.CandidateSetSource,
	referenceTargets workflowgeneration.ReferenceTargetBuilder,
	preparations workflowgeneration.ReferencePreparation,
	providers workflowgeneration.ImageProvider,
	segments workflowproduction.EpisodeSegmentationOwner,
	episodes workflowproduction.EpisodeAnalysisOwner,
) (*workflowapp.RuntimeService, error) {
	if repository == nil || scripts == nil || evidence == nil || stories == nil || storyReviews == nil || bibles == nil || projects == nil || plans == nil || planningOwners == nil || storygraphs == nil || storyboards == nil || reviews == nil {
		return nil, errors.New("workflow runtime dependencies are required")
	}
	now := func() time.Time { return time.Now().UTC() }
	humanTasks := workflowapp.HumanTaskOpener(workflowreview.New(reviews))
	if candidateSets != nil {
		humanTasks = workflowreview.NewWithGeneration(reviews, candidateSets)
	}
	if (referenceTargets == nil) != (preparations == nil) || (referenceTargets == nil) != (providers == nil) {
		return nil, errors.New("reference asset workflow dependencies must be configured together")
	}
	executor := workflowapp.NodeExecutor(
		workflowproduction.NewNodeExecutor(scripts, evidence, stories, storyReviews, bibles, projects, plans, planningOwners, storygraphs, storyboards, bindings, segments, episodes),
	)
	if candidateSets != nil || referenceTargets != nil {
		materializer, _ := candidateSets.(workflowgeneration.ProviderOutputMaterializer)
		if referenceTargets != nil && materializer == nil {
			return nil, errors.New("reference asset output materializer is required")
		}
		var err error
		executor, err = workflowexecution.NewNodeExecutor(
			executor, workflowgeneration.NewNodeExecutor(candidateSets, referenceTargets, preparations, preparations, providers, materializer),
		)
		if err != nil {
			return nil, err
		}
	}
	return workflowapp.NewRuntimeService(repository, workflowapp.RuntimeConfig{
		Now: now, NewID: uuid.NewString,
		Executor:   executor,
		HumanTasks: humanTasks,
	}), nil
}
