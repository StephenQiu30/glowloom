package maintenance_test

import (
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	maintenanceflow "github.com/StephenQiu30/glowloom-server/backend/internal/infra/maintenance/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/infra/maintenance/application"
)

func TestMaintenanceWorkflowsExecuteRegisteredActivities(t *testing.T) {
	tests := []struct {
		name   string
		kind   string
		outbox []int64
		inbox  []int64
	}{
		{"partitions", maintenanceflow.Partitions, nil, nil},
		{"outbox", maintenanceflow.Outbox, []int64{500, 0}, nil},
		{"processed", maintenanceflow.Inbox, nil, []int64{500, 0}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outbox := &outboxStore{dropped: 1, counts: tc.outbox}
			inbox := &inboxStore{counts: tc.inbox}
			service := application.NewService(outbox, inbox)
			next, err := maintenanceflow.Handlers(service)[tc.kind](t.Context(), taskruntime.Job{})
			if err != nil || next.IsZero() {
				t.Fatalf("maintenance task cursor: %v %v", next, err)
			}
		})
	}
}
