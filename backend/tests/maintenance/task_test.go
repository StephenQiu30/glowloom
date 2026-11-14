package maintenance_test

import (
	"context"
	"testing"
	"time"

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
		offset time.Duration
	}{
		{"outbox", maintenanceflow.Outbox, []int64{500, 0}, nil, 4 * time.Hour},
		{"processed", maintenanceflow.Inbox, nil, []int64{500, 0}, 4*time.Hour + 30*time.Minute},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outbox := &partitionDDLGuard{outboxStore: outboxStore{counts: tc.outbox}, t: t}
			inbox := &inboxStore{counts: tc.inbox}
			service := application.NewService(outbox, inbox)
			before := time.Now().UTC()
			next, err := maintenanceflow.Handlers(service)[tc.kind](t.Context(), taskruntime.Job{})
			if err != nil || !next.After(before) {
				t.Fatalf("maintenance task cursor: %v %v", next, err)
			}
			if next.Location() != time.UTC || next.Sub(next.Truncate(24*time.Hour)) != tc.offset {
				t.Fatalf("maintenance task must retain its daily UTC schedule: next=%v offset=%v", next, tc.offset)
			}
			if len(outbox.counts) != 0 || len(inbox.counts) != 0 {
				t.Fatalf("retention batches left: Outbox %v, Inbox %v", outbox.counts, inbox.counts)
			}
		})
	}
}

type partitionDDLGuard struct {
	outboxStore
	t *testing.T
}

func (s *partitionDDLGuard) EnsurePartitions(context.Context, time.Time) error {
	s.t.Fatal("ordinary maintenance must not create partitions")
	return nil
}

func (s *partitionDDLGuard) DropEmptyPastPartitions(context.Context, time.Time) (int, error) {
	s.t.Fatal("ordinary maintenance must not drop partitions")
	return 0, nil
}

func TestLegacyPartitionTaskCompletesWithoutDDLOrRescheduling(t *testing.T) {
	service := application.NewService(&partitionDDLGuard{t: t}, &inboxStore{})
	handler := maintenanceflow.Handlers(service)[maintenanceflow.Partitions]
	if handler == nil {
		t.Fatal("persisted partition tasks must remain claimable after restart")
	}
	for _, failures := range []int{0, 5} {
		next, err := handler(t.Context(), taskruntime.Job{
			Kind: maintenanceflow.Partitions, BusinessID: "periodic", Epoch: 2, Attempt: 2, Failures: failures,
		})
		if err != nil || !next.IsZero() {
			t.Fatalf("retired partition task must complete, not retry or reschedule: next=%v err=%v", next, err)
		}
	}
}
