package maintenance_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/infra/maintenance/application"
)

type outboxStore struct {
	ensured []time.Time
	dropped int
	counts  []int64
	err     error
}

func (s *outboxStore) EnsurePartitions(_ context.Context, now time.Time) error {
	s.ensured = append(s.ensured, now)
	return s.err
}

func (s *outboxStore) DropEmptyPastPartitions(context.Context, time.Time) (int, error) {
	return s.dropped, s.err
}

func (s *outboxStore) PrunePublished(context.Context, time.Time, int) (int64, error) {
	if s.err != nil {
		return 0, s.err
	}
	n := s.counts[0]
	s.counts = s.counts[1:]
	return n, nil
}

type inboxStore struct {
	counts []int64
	err    error
}

func (s *inboxStore) PruneExpired(context.Context, time.Time, int) (int64, error) {
	if s.err != nil {
		return 0, s.err
	}
	n := s.counts[0]
	s.counts = s.counts[1:]
	return n, nil
}

func TestMaintenanceDrainsBatchesAndReportsProgress(t *testing.T) {
	now := time.Date(2026, time.September, 27, 0, 0, 0, 0, time.UTC)
	outbox := &outboxStore{dropped: 2, counts: []int64{2, 2, 1}}
	inbox := &inboxStore{counts: []int64{2, 0}}
	service := application.NewService(outbox, inbox)
	if dropped, err := service.MaintainPartitions(t.Context(), now); err != nil || dropped != 2 {
		t.Fatalf("maintain partitions = (%d, %v)", dropped, err)
	}
	if len(outbox.ensured) != 1 || !outbox.ensured[0].Equal(now) {
		t.Fatalf("ensure partitions times = %v", outbox.ensured)
	}
	var progress []int64
	if removed, err := service.PruneOutbox(t.Context(), now, 2, func(total int64) {
		progress = append(progress, total)
	}); err != nil || removed != 5 {
		t.Fatalf("prune outbox = (%d, %v)", removed, err)
	}
	if len(progress) != 3 || progress[0] != 2 || progress[1] != 4 || progress[2] != 5 {
		t.Fatalf("outbox progress = %v", progress)
	}
	if removed, err := service.PruneProcessedEvents(t.Context(), now, 2, nil); err != nil || removed != 2 {
		t.Fatalf("prune processed events = (%d, %v)", removed, err)
	}
}

func TestMaintenancePropagatesCancellationAndStoreFailure(t *testing.T) {
	want := errors.New("database unavailable")
	service := application.NewService(&outboxStore{err: want}, &inboxStore{err: want})
	if _, err := service.MaintainPartitions(t.Context(), time.Now()); !errors.Is(err, want) {
		t.Fatalf("partition maintenance error = %v", err)
	}
	if _, err := service.PruneOutbox(t.Context(), time.Now(), 100, nil); !errors.Is(err, want) {
		t.Fatalf("outbox cleanup error = %v", err)
	}
	if _, err := service.PruneProcessedEvents(t.Context(), time.Now(), 100, nil); !errors.Is(err, want) {
		t.Fatalf("processed cleanup error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := service.PruneOutbox(ctx, time.Now(), 100, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled cleanup error = %v", err)
	}
}
