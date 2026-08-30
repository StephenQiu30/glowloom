package temporal

import (
	"context"
	"errors"
	"time"

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
	FailRun(context.Context, domain.FailRunCommand) error
	CompleteRun(context.Context, domain.CompleteRunCommand) error
}

type ExecuteNodeActivityHandler func(
	context.Context,
	domain.NodeActivityCommand,
) (domain.NodeActivityResult, error)

func NewExecuteNodeActivityHandler(execute ExecuteNodeActivityHandler) ExecuteNodeActivityHandler {
	return func(ctx context.Context, command domain.NodeActivityCommand) (domain.NodeActivityResult, error) {
		heartbeatTimeout := activity.GetInfo(ctx).HeartbeatTimeout
		if heartbeatTimeout <= 0 {
			return execute(ctx, command)
		}

		activity.RecordHeartbeat(ctx)
		stop := make(chan struct{})
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			interval := heartbeatTimeout / 3
			if interval <= 0 {
				interval = heartbeatTimeout
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					activity.RecordHeartbeat(ctx)
				case <-ctx.Done():
					return
				case <-stop:
					return
				}
			}
		}()
		defer func() {
			close(stop)
			<-stopped
		}()

		return execute(ctx, command)
	}
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
	runtimeWorker.RegisterWorkflowWithOptions(
		ShotProductionWorkflow,
		workflow.RegisterOptions{Name: ShotProductionWorkflowName},
	)
	runtimeWorker.RegisterActivityWithOptions(
		activities.LoadExecutionPlan,
		activity.RegisterOptions{Name: LoadExecutionPlanActivityName},
	)
	runtimeWorker.RegisterActivityWithOptions(
		NewExecuteNodeActivityHandler(activities.ExecuteNode),
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
		activities.FailRun,
		activity.RegisterOptions{Name: FailRunActivityName},
	)
	runtimeWorker.RegisterActivityWithOptions(
		activities.CompleteRun,
		activity.RegisterOptions{Name: CompleteRunActivityName},
	)
	return runtimeWorker, nil
}

var _ RuntimeActivities = (*workflowapp.RuntimeService)(nil)
