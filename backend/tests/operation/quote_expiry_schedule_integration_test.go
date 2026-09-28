package operation_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationflow "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func TestQuoteExpiryScheduleInstallsIdempotentlyOnLocalTemporal(t *testing.T) {
	addr, namespace := os.Getenv("LV_TEST_TEMPORAL_ADDR"), os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if addr == "" || namespace == "" {
		t.Skip("set LV_TEST_TEMPORAL_ADDR and LV_TEST_TEMPORAL_NAMESPACE")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	workflowClient, err := client.Dial(client.Options{HostPort: addr, Namespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(workflowClient.Close)
	prefix := "quote-expiry-test-" + uuid.NewString() + "-"
	installer := operationflow.NewQuoteExpiryScheduleInstaller(workflowClient.ScheduleClient(), prefix)
	handle := workflowClient.ScheduleClient().GetHandle(ctx, prefix+operationflow.QuoteExpiryScheduleID)
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := handle.Delete(cleanupCtx); err != nil {
			t.Errorf("delete quote expiry test schedule: %v", err)
		}
	})
	if err := installer.Install(ctx); err != nil {
		t.Fatalf("install quote expiry schedule: %v", err)
	}
	if err := installer.Install(ctx); err != nil {
		t.Fatalf("repeat quote expiry schedule installation: %v", err)
	}
	description, err := handle.Describe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	action, ok := description.Schedule.Action.(*client.ScheduleWorkflowAction)
	if !ok || action.Workflow != "QuoteExpiryWorkflow" || action.TaskQueue != "flow" ||
		len(description.Schedule.Spec.Intervals) != 1 ||
		description.Schedule.Spec.Intervals[0].Every != 5*time.Minute ||
		description.Schedule.Policy.Overlap != enums.SCHEDULE_OVERLAP_POLICY_SKIP ||
		!description.Schedule.Policy.PauseOnFailure {
		t.Fatalf("quote expiry schedule contract = %+v", description.Schedule)
	}
}

func TestScheduledQuoteExpiryRunsAgainstLocalPostgresAndTemporal(t *testing.T) {
	addr, namespace := os.Getenv("LV_TEST_TEMPORAL_ADDR"), os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if os.Getenv("LV_TEST_OPERATION_STORE_DB_DSN") == "" || addr == "" || namespace == "" {
		t.Skip("set isolated operation PostgreSQL and local Temporal test variables")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	item := quotedSnapshotItem(projectID, 1)
	created := time.Now().UTC().Add(-20 * time.Minute)
	expires := created.Add(15 * time.Minute)
	item.Operation.CreateTime = created
	item.Operation.QuoteExpiresAt = &expires
	store := pgoperation.NewStore(database)
	if err := store.CreateQuoteSnapshot(ctx, actor, nil, []pgoperation.QuoteItem{item}); err != nil {
		t.Fatal(err)
	}
	workflowClient, err := client.Dial(client.Options{HostPort: addr, Namespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(workflowClient.Close)
	queue := "quote-expiry-test-" + uuid.NewString()
	flowWorker := worker.New(workflowClient, queue, worker.Options{})
	operationflow.RegisterQuoteExpiry(flowWorker,
		operationflow.NewQuoteExpiryActivities(operationapp.NewQuoteExpiryService(store, 10)))
	if err := flowWorker.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(flowWorker.Stop)
	scheduleID := "quote-expiry-test-" + uuid.NewString()
	handle, err := workflowClient.ScheduleClient().Create(ctx, client.ScheduleOptions{
		ID:   scheduleID,
		Spec: client.ScheduleSpec{Intervals: []client.ScheduleIntervalSpec{{Every: 3 * time.Second}}},
		Action: &client.ScheduleWorkflowAction{
			ID: scheduleID, Workflow: operationflow.QuoteExpiryWorkflow, TaskQueue: queue,
		},
		Overlap: enums.SCHEDULE_OVERLAP_POLICY_SKIP,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := handle.Delete(cleanupCtx); err != nil {
			t.Errorf("delete quote expiry execution schedule: %v", err)
		}
	})
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		description, err := handle.Describe(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, action := range description.Info.RecentActions {
			if action.StartWorkflowResult == nil {
				continue
			}
			run := action.StartWorkflowResult
			if err := workflowClient.GetWorkflow(ctx, run.WorkflowID, run.FirstExecutionRunID).Get(ctx, nil); err != nil {
				t.Fatalf("scheduled quote expiry workflow: %v", err)
			}
			got, err := store.FindOperation(ctx, actor, projectID, item.Operation.ID)
			if err != nil || got.Status != domain.StatusExpired {
				t.Fatalf("scheduled expired quote = %+v, %v", got, err)
			}
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for scheduled quote expiry: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}
