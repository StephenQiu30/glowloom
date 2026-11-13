package operation_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	task "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

type managedProbe struct {
	steps  []string
	result application.ManagedProgress
}

func (p *managedProbe) Step(_ context.Context, in application.ManagedStep) (application.ManagedProgress, error) {
	p.steps = append(p.steps, in.Action)
	return p.result, nil
}
func TestManagedTaskPersistsBeforeIngestAndSettlesOnce(t *testing.T) {
	for _, mode := range []string{"sync", "video", "recovered_sync"} {
		t.Run(mode, func(t *testing.T) {
			s := newStateStore()
			s.loaded.Provider.AdapterKey = "openrouter"
			s.loaded.Provider.Queue = "provider"
			s.loaded.Operation.Status = domain.StatusSubmitting
			p := &managedProbe{result: application.ManagedProgress{State: application.ManagedProgressSucceeded}}
			if mode == "video" {
				p.result.State = application.ManagedProgressAccepted
				p.result.TaskID = "original-task"
			}
			if mode == "recovered_sync" {
				at := time.Now()
				s.evidence.DispatchStartedAt = &at
			}
			h := s.handlers()
			task.RegisterProvider(h, s, p)
			run := func(k string) {
				_, err := h[k](t.Context(), taskruntime.Job{BusinessID: s.loaded.Operation.ID.String()})
				require.NoError(t, err)
			}
			run("operation.provider")
			require.Len(t, s.calls, 1)
			require.Len(t, s.completes, 1)
			require.Empty(t, s.settled)
			run("operation.run")
			if mode == "video" {
				p.result.State = application.ManagedProgressSucceeded
				run("operation.provider")
			}
			run("operation.run")
			s.evidence.Ingested = true
			s.cost.ActualCostMicros = 100
			run("operation.run")
			require.Len(t, s.settled, 1)
			require.EqualValues(t, 100, s.settled[0].ActualCostMicros)
			if mode == "recovered_sync" {
				require.Equal(t, []string{"recover"}, p.steps)
			} else {
				require.Equal(t, "submit", p.steps[0])
			}
		})
	}
}
func TestManagedTaskUnknownCostNeverReleasesOrResubmits(t *testing.T) {
	s := newStateStore()
	s.loaded.Operation.Status = domain.StatusSubmitting
	s.loaded.Provider.AdapterKey = "openrouter"
	s.loaded.Provider.Queue = "provider"
	s.completeErr = application.ErrProviderCostUnknown
	p := &managedProbe{result: application.ManagedProgress{State: application.ManagedProgressSucceeded}}
	h := s.handlers()
	task.RegisterProvider(h, s, p)
	_, err := h["operation.provider"](t.Context(), taskruntime.Job{BusinessID: s.loaded.Operation.ID.String()})
	require.NoError(t, err)
	require.Equal(t, domain.StatusUnknown, s.loaded.Operation.Status)
	require.Empty(t, s.settled)
	require.Equal(t, []string{"submit"}, p.steps)
}
func TestManagedTaskRetainsNotSubmittedClassification(t *testing.T) {
	s := newStateStore()
	s.loaded.Operation.Status = domain.StatusSubmitting
	s.loaded.Provider.AdapterKey = "volcengine_ark"
	p := &managedProbe{result: application.ManagedProgress{State: application.ManagedProgressNotSubmitted, ErrorCode: "invalid_provider_request"}}
	h := s.handlers()
	task.RegisterProvider(h, s, p)
	_, err := h["operation.provider"](t.Context(), taskruntime.Job{BusinessID: s.loaded.Operation.ID.String()})
	require.NoError(t, err)
	s.run(t, "operation.run")
	require.Equal(t, "provider_invalid_parameters", s.settled[0].FailureCode)
	require.Zero(t, s.settled[0].ActualCostMicros)
}
func TestManagedTaskCapacitySchedulesWaitingWithoutCallCompletion(t *testing.T) {
	s := newStateStore()
	s.loaded.Operation.Status = domain.StatusSubmitting
	s.loaded.Provider.AdapterKey = "openrouter"
	p := &managedProbe{result: application.ManagedProgress{State: application.ManagedProgressWaiting}}
	h := s.handlers()
	task.RegisterProvider(h, s, p)
	next, err := h["operation.provider"](t.Context(), taskruntime.Job{BusinessID: s.loaded.Operation.ID.String()})
	require.NoError(t, err)
	require.True(t, next.After(time.Now()))
	require.Empty(t, s.completes)
	require.Empty(t, s.settled)
}
func TestManagedTaskPollDeadlineEntersReconciliation(t *testing.T) {
	s := newStateStore()
	s.loaded.Operation.Status = domain.StatusSubmitted
	s.loaded.Provider.AdapterKey = "openrouter"
	id := "task-original"
	s.loaded.ProviderTaskID = &id
	at := time.Now().Add(-time.Hour)
	s.loaded.Operation.ConfirmedAt = &at
	s.evidence.DispatchStartedAt = &at
	p := &managedProbe{result: application.ManagedProgress{State: application.ManagedProgressUnknown}}
	h := s.handlers()
	task.RegisterProvider(h, s, p)
	_, err := h["operation.provider"](t.Context(), taskruntime.Job{BusinessID: s.loaded.Operation.ID.String()})
	require.NoError(t, err)
	require.Equal(t, domain.StatusReconciling, s.loaded.Operation.Status)
	require.Equal(t, []string{"query"}, p.steps)
	require.Empty(t, s.settled)
}

func TestManagedQueueWaitDoesNotConsumeProviderExecutionTime(t *testing.T) {
	s := newStateStore()
	s.loaded.Operation.Status = domain.StatusSubmitted
	s.loaded.Provider.AdapterKey = "volcengine_ark"
	id := "accepted-video"
	s.loaded.ProviderTaskID = &id
	queued, sent := time.Now().Add(-4*time.Hour), time.Now().Add(-time.Second)
	s.loaded.Operation.ConfirmedAt, s.evidence.DispatchStartedAt = &queued, &sent
	p := &managedProbe{result: application.ManagedProgress{State: application.ManagedProgressRunning}}
	h := s.handlers()
	task.RegisterProvider(h, s, p)
	next, err := h["operation.provider"](t.Context(), taskruntime.Job{BusinessID: s.loaded.Operation.ID.String()})
	require.NoError(t, err)
	require.Equal(t, domain.StatusSubmitted, s.loaded.Operation.Status)
	require.True(t, next.After(time.Now()))
	require.Equal(t, []string{"query"}, p.steps)
}
