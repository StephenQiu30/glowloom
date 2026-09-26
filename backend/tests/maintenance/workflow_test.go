package maintenance_test

import (
	"testing"

	"go.temporal.io/sdk/testsuite"

	maintenanceflow "github.com/StephenQiu30/lanverse/backend/internal/infra/maintenance/adapter/temporal"
	"github.com/StephenQiu30/lanverse/backend/internal/infra/maintenance/application"
)

func TestMaintenanceWorkflowsExecuteRegisteredActivities(t *testing.T) {
	tests := []struct {
		name     string
		workflow any
		outbox   []int64
		inbox    []int64
	}{
		{"partitions", maintenanceflow.OutboxPartitionMaintenanceWorkflow, nil, nil},
		{"outbox", maintenanceflow.OutboxCleanupWorkflow, []int64{2, 0}, nil},
		{"processed", maintenanceflow.ProcessedEventCleanupWorkflow, nil, []int64{2, 0}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outbox := &outboxStore{dropped: 1, counts: tc.outbox}
			inbox := &inboxStore{counts: tc.inbox}
			service := application.NewService(outbox, inbox)
			testSuite := &testsuite.WorkflowTestSuite{}
			env := testSuite.NewTestWorkflowEnvironment()
			env.RegisterActivity(maintenanceflow.NewActivities(service, 2))
			env.ExecuteWorkflow(tc.workflow)
			if err := env.GetWorkflowError(); err != nil {
				t.Fatalf("maintenance workflow: %v", err)
			}
			if !env.IsWorkflowCompleted() {
				t.Fatal("maintenance workflow did not complete")
			}
		})
	}
}
