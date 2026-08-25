package temporal

import (
	"context"
	"errors"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type RuntimeActivities interface {
	LoadExecutionPlan(context.Context, domain.StartRequest) (domain.ExecutionPlan, error)
	ExecuteNode(context.Context, domain.NodeActivityCommand) (domain.NodeActivityResult, error)
	OpenHumanGate(context.Context, domain.NodeActivityCommand) error
	ApplyHumanGate(context.Context, domain.ApplyHumanGateCommand) error
	CompleteRun(context.Context, domain.CompleteRunCommand) error
}

func (runtime *Client) NewWorker(activities RuntimeActivities) (worker.Worker, error) {
	if runtime == nil || runtime.client == nil || runtime.taskQueue == "" || activities == nil {
		return nil, errors.New("invalid Temporal worker configuration")
	}
	runtimeWorker := worker.New(runtime.client, runtime.taskQueue, worker.Options{})
	runtimeWorker.RegisterWorkflowWithOptions(
		EpisodeProductionWorkflow,
		workflow.RegisterOptions{Name: EpisodeProductionWorkflowName},
	)
	runtimeWorker.RegisterActivityWithOptions(
		activities.LoadExecutionPlan,
		activity.RegisterOptions{Name: LoadExecutionPlanActivityName},
	)
	runtimeWorker.RegisterActivityWithOptions(
		activities.ExecuteNode,
		activity.RegisterOptions{Name: ExecuteNodeActivityName},
	)
	runtimeWorker.RegisterActivityWithOptions(
		activities.OpenHumanGate,
		activity.RegisterOptions{Name: OpenHumanGateActivityName},
	)
	runtimeWorker.RegisterActivityWithOptions(
		activities.ApplyHumanGate,
		activity.RegisterOptions{Name: ApplyHumanGateActivityName},
	)
	runtimeWorker.RegisterActivityWithOptions(
		activities.CompleteRun,
		activity.RegisterOptions{Name: CompleteRunActivityName},
	)
	return runtimeWorker, nil
}

var _ RuntimeActivities = (*workflowapp.RuntimeService)(nil)
