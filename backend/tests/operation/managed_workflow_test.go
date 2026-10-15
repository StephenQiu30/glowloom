package operation_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

func TestManagedWorkflowPersistsBeforeIngestAndSettlesOnce(t *testing.T) {
	for _, kind := range []string{"sync", "video", "recovered_sync", "unknown_cost"} {
		t.Run(kind, func(t *testing.T) {
			loaded := workflowFixture()
			loaded.Provider.AdapterKey = "openrouter"
			loaded.Provider.Queue = "provider"
			loaded.Provider.SupportsCancel = false
			loaded.Provider.Moderation = "provider"
			id := loaded.Operation.ID
			suite := &testsuite.WorkflowTestSuite{}
			env := suite.NewTestWorkflowEnvironment()
			registerOperationMockActivities(env)
			env.RegisterActivityWithOptions(func(context.Context, string) (application.ProviderCost, error) {
				return application.ProviderCost{}, nil
			}, activity.RegisterOptions{Name: "flow.LoadProviderCost"})
			env.RegisterActivityWithOptions(func(context.Context, application.ManagedStep) (application.ManagedProgress, error) {
				return application.ManagedProgress{}, nil
			}, activity.RegisterOptions{Name: "provider.managed"})
			env.RegisterActivityWithOptions(func(context.Context, uuid.UUID) error { return nil }, activity.RegisterOptions{Name: "media.IngestManaged"})
			env.OnActivity("flow.LoadOperation", mock.Anything, id.String()).Return(loaded, nil).Once()
			env.OnActivity("flow.CheckConsent", mock.Anything, id.String()).Return(nil).Once()
			env.OnActivity("flow.Transition", mock.Anything, mock.Anything).Return(func(_ context.Context, in application.TransitionInput) (domain.Status, error) { return in.To, nil })
			env.OnActivity("flow.BeginProviderCall", mock.Anything, mock.Anything).Return(nil)
			if kind == "unknown_cost" {
				env.OnActivity("flow.CompleteProviderCall", mock.Anything, mock.MatchedBy(func(in application.CompleteProviderCallInput) bool { return in.Managed })).Return(temporal.NewNonRetryableApplicationError("missing usage", "provider_cost_unknown", nil)).Once()
				env.RegisterDelayedCallback(func() { env.SignalWorkflow("resolve_manual", operationflow.ManualResolution{Outcome: "retry"}) }, time.Minute)
			}
			env.OnActivity("flow.CompleteProviderCall", mock.Anything, mock.Anything).Return(nil)
			switch kind {
			case "sync", "unknown_cost":
				env.OnActivity("provider.managed", mock.Anything, application.ManagedStep{OperationID: id, Action: "submit"}).Return(application.ManagedProgress{State: "succeeded", OutputCount: 1}, nil).Once()
			case "recovered_sync":
				env.OnActivity("provider.managed", mock.Anything, application.ManagedStep{OperationID: id, Action: "submit"}).Return(application.ManagedProgress{State: "unknown"}, nil).Once()
				env.OnActivity("provider.managed", mock.Anything, application.ManagedStep{OperationID: id, Action: "recover"}).Return(application.ManagedProgress{State: "succeeded", OutputCount: 1}, nil).Once()
			case "video":
				env.OnActivity("provider.managed", mock.Anything, application.ManagedStep{OperationID: id, Action: "submit"}).Return(application.ManagedProgress{State: "accepted", TaskID: "task-fixture"}, nil).Once()
				env.OnActivity("provider.managed", mock.Anything, application.ManagedStep{OperationID: id, Action: "recover"}).Return(application.ManagedProgress{State: "accepted", TaskID: "task-fixture"}, nil).Once()
				env.OnActivity("provider.managed", mock.Anything, application.ManagedStep{OperationID: id, Action: "query"}).Return(application.ManagedProgress{State: "succeeded", TaskID: "task-fixture", OutputCount: 1}, nil).Once()
			}
			env.OnActivity("media.IngestManaged", mock.Anything, id).Return(nil).Once()
			env.OnActivity("flow.LoadProviderCost", mock.Anything, id.String()).Return(application.ProviderCost{ActualCostMicros: 100, HasSubmission: true}, nil).Once()
			env.OnActivity("flow.SettleOperation", mock.Anything, mock.MatchedBy(func(in operationflow.SettlementInput) bool {
				return in.To == domain.StatusCompleted && in.ActualCostMicros == 100
			})).Return(nil).Once()
			env.SetTestTimeout(time.Second * 10)
			env.ExecuteWorkflow(operationflow.OperationWorkflow, operationflow.OperationInput{OperationID: id.String()})
			require.NoError(t, env.GetWorkflowError())
			env.AssertExpectations(t)
		})
	}
}

func TestManagedWorkflowRoutesRevocationToFinancialClosure(t *testing.T) {
	for _, point := range []string{"submitted", "succeeded", "ingesting", "completed", "reconciling"} {
		t.Run(point, func(t *testing.T) {
			loaded := workflowFixture()
			loaded.Provider.AdapterKey = "openrouter"
			loaded.Provider.Queue = "provider"
			loaded.Provider.SupportsCancel = false
			id := loaded.Operation.ID
			env := (&testsuite.WorkflowTestSuite{}).NewTestWorkflowEnvironment()
			registerOperationMockActivities(env)
			register := func(name string, fn any) { env.RegisterActivityWithOptions(fn, activity.RegisterOptions{Name: name}) }
			register("provider.managed", func(context.Context, application.ManagedStep) (application.ManagedProgress, error) {
				return application.ManagedProgress{}, nil
			})
			register("media.IngestManaged", func(context.Context, uuid.UUID) error { return nil })
			register("flow.LoadProviderCost", func(context.Context, string) (application.ProviderCost, error) {
				return application.ProviderCost{ActualCostMicros: 7, HasSubmission: true}, nil
			})
			register("flow.ResolveRevokedOperation", func(context.Context, string) error { return nil })
			revoked := temporal.NewNonRetryableApplicationError("authority revoked", "execution_authority_revoked", nil)
			env.OnActivity("flow.LoadOperation", mock.Anything, id.String()).Return(loaded, nil).Once()
			env.OnActivity("flow.CheckConsent", mock.Anything, id.String()).Return(nil).Once()
			env.OnActivity("flow.BeginProviderCall", mock.Anything, mock.Anything).Return(nil)
			env.OnActivity("flow.CompleteProviderCall", mock.Anything, mock.Anything).Return(nil)
			env.OnActivity("flow.Transition", mock.Anything, mock.Anything).Return(func(_ context.Context, in application.TransitionInput) (domain.Status, error) {
				if string(in.To) == point {
					return "", revoked
				}
				return in.To, nil
			})
			state := "succeeded"
			task := ""
			if point == "submitted" {
				state, task = "accepted", "fixture-task"
			}
			if point == "reconciling" {
				state = "unknown"
			}
			env.OnActivity("provider.managed", mock.Anything, application.ManagedStep{OperationID: id, Action: "submit"}).Return(application.ManagedProgress{State: state, TaskID: task}, nil).Once()
			if point == "reconciling" {
				env.OnActivity("provider.managed", mock.Anything, application.ManagedStep{OperationID: id, Action: "recover"}).Return(application.ManagedProgress{State: "unknown"}, nil).Once()
			}
			if point == "completed" {
				env.OnActivity("media.IngestManaged", mock.Anything, id).Return(nil).Once()
				env.OnActivity("flow.SettleOperation", mock.Anything, mock.Anything).Return(revoked).Once()
			}
			env.OnActivity("flow.ResolveRevokedOperation", mock.Anything, id.String()).Return(nil).Once()
			env.SetTestTimeout(10 * time.Second)
			env.ExecuteWorkflow(operationflow.OperationWorkflow, operationflow.OperationInput{OperationID: id.String()})
			require.NoError(t, env.GetWorkflowError())
			env.AssertExpectations(t)
		})
	}
}
