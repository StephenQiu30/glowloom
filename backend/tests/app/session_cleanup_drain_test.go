package app_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type drainStore struct {
	identity.EmailStore
	run   func(context.Context) (int, error)
	calls atomic.Int64
}

func (s *drainStore) EmailTransaction(ctx context.Context, fn func(identity.EmailTx) error) error {
	s.calls.Add(1)
	return fn(drainTx{run: func() (int, error) { return s.run(ctx) }})
}

type drainTx struct {
	identity.EmailTx
	run func() (int, error)
}

func (t drainTx) CleanupExpiredRefresh(time.Time, int) (int, error) { return t.run() }
func awaitCleanupLog(t *testing.T, logs *observer.ObservedLogs, message string) observer.LoggedEntry {
	t.Helper()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		entries := logs.FilterMessage(message).All()
		if len(entries) > 0 {
			return entries[0]
		}
		select {
		case <-deadline.C:
			t.Fatal("cleanup observation missing: " + message)
		case <-ticker.C:
		}
	}
}
func TestSessionReceiptCleanupDrainsBacklog(t *testing.T) {
	drained := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	var remaining atomic.Int64
	remaining.Store(3501)
	store := &drainStore{run: func(context.Context) (int, error) {
		once.Do(func() { close(started) })
		n := int64(1000)
		if left := remaining.Load(); left < n {
			n = left
		}
		remaining.Add(-n)
		if remaining.Load() == 0 {
			close(drained)
		}
		return int(n), nil
	}}
	stop, err := identityruntime.StartSessionReceiptCleanupLoop(t.Context(), identity.NewSessionReceiptCleanup(store, time.Now), zap.NewNop(), 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("cleanup never started")
	}
	select {
	case <-drained:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("full batches slept instead of draining 3501-row backlog in one tick")
	}
	stop()
	if store.calls.Load() != 4 || remaining.Load() != 0 {
		t.Fatalf("partial batch must end drain: calls=%d left=%d", store.calls.Load(), remaining.Load())
	}
}
func TestSessionReceiptCleanupDrainBounds(t *testing.T) {
	for _, mode := range []string{"batch_limit", "budget_exhausted", "failed"} {
		t.Run(mode, func(t *testing.T) {
			core, logs := observer.New(zap.DebugLevel)
			var calls atomic.Int64
			var firstDeadline time.Time
			store := &drainStore{run: func(ctx context.Context) (int, error) {
				n := calls.Add(1)
				deadline, ok := ctx.Deadline()
				if !ok {
					t.Error("cleanup cycle lacks deadline")
				}
				if n == 1 {
					firstDeadline = deadline
				} else if !deadline.Equal(firstDeadline) {
					t.Error("cleanup batches did not share a bounded tick budget")
				}
				if mode == "budget_exhausted" && n == 2 {
					return 0, context.DeadlineExceeded
				}
				if mode == "failed" {
					return 0, errors.New("sensitive://credential-must-not-be-logged")
				}
				return 1000, nil
			}}
			stop, err := identityruntime.StartSessionReceiptCleanupLoop(t.Context(), identity.NewSessionReceiptCleanup(store, time.Now), zap.New(core), 200*time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			defer stop()
			entry := awaitCleanupLog(t, logs, "identity receipt cleanup incomplete")
			stop()
			fields := entry.ContextMap()
			if fields["reason"] != mode {
				t.Fatalf("reason=%v want %s", fields["reason"], mode)
			}
			wantClass := map[string]string{"batch_limit": "none", "budget_exhausted": "deadline", "failed": "store"}[mode]
			if fields["error_class"] != wantClass {
				t.Fatalf("error_class=%v want %s", fields["error_class"], wantClass)
			}
			expected := int64(32)
			if mode == "budget_exhausted" {
				expected = 2
			}
			if mode == "failed" {
				expected = 1
			}
			if calls.Load() != expected {
				t.Fatalf("bounded drain calls=%d want%d", calls.Load(), expected)
			}
			if mode != "failed" && fields["backlog"] != true {
				t.Fatalf("unobserved backlog: %v", fields)
			}
			for _, entry := range logs.All() {
				for _, field := range entry.Context {
					if field.String == "sensitive://credential-must-not-be-logged" {
						t.Fatal("raw error leaked")
					}
				}
			}
		})
	}
}

func TestSessionReceiptCleanupBudgetExpires(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	store := &drainStore{run: func(ctx context.Context) (int, error) { <-ctx.Done(); return 0, ctx.Err() }}
	stop, err := identityruntime.StartSessionReceiptCleanupLoop(t.Context(), identity.NewSessionReceiptCleanup(store, time.Now), zap.New(core), 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	entry := awaitCleanupLog(t, logs, "identity receipt cleanup incomplete")
	stop()
	fields := entry.ContextMap()
	if fields["reason"] != "budget_exhausted" || fields["error_class"] != "deadline" {
		t.Fatalf("actual budget cancellation not observed: %v", fields)
	}
}
