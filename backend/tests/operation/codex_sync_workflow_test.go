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

type codexTaskProbe struct {
	calls  int
	result task.ProviderSubmitOutput
}

func (p *codexTaskProbe) Submit(context.Context, task.ProviderSubmitInput) (task.ProviderSubmitOutput, error) {
	p.calls++
	return p.result, nil
}
func TestCodexTaskUnknownKeepsReservationWithoutQueryOrMockReview(t *testing.T) {
	s := newStateStore()
	s.loaded.Provider.AdapterKey = "codex"
	s.loaded.Provider.Queue = "agent.codex"
	s.loaded.Operation.Status = domain.StatusSubmitting
	p := &codexTaskProbe{result: task.ProviderSubmitOutput{Outcome: task.ProviderSubmitUnknown}}
	h := s.handlers()
	task.RegisterCodex(h, s, p)
	for n := 0; n < 2; n++ {
		_, err := h["operation.codex"](t.Context(), taskruntime.Job{BusinessID: s.loaded.Operation.ID.String()})
		require.NoError(t, err)
	}
	s.run(t, "operation.run")
	s.run(t, "operation.run")
	s.run(t, "operation.run")
	require.Equal(t, domain.StatusManual, s.loaded.Operation.Status)
	require.Empty(t, s.settled)
	require.Equal(t, 1, p.calls)
	require.Equal(t, 1, s.recoveries)
}
func TestCodexTaskCrashAfterDispatchNeverSubmitsAgain(t *testing.T) {
	s := newStateStore()
	s.loaded.Provider.AdapterKey = "codex"
	s.loaded.Provider.Queue = "agent.codex"
	s.loaded.Operation.Status = domain.StatusSubmitting
	at := time.Now()
	s.evidence.DispatchStartedAt = &at
	p := &codexTaskProbe{}
	h := s.handlers()
	task.RegisterCodex(h, s, p)
	_, err := h["operation.codex"](t.Context(), taskruntime.Job{BusinessID: s.loaded.Operation.ID.String()})
	require.NoError(t, err)
	require.Zero(t, p.calls)
	require.Equal(t, application.ProviderCallStateUnknown, s.evidence.SubmitState)
}
func TestCodexTaskQueuedCancellationRequiresDefiniteNoSend(t *testing.T) {
	s := newStateStore()
	s.loaded.Provider.AdapterKey = "codex"
	s.loaded.Operation.Status = domain.StatusSubmitting
	s.evidence.SubmitState = application.ProviderCallStateNotSubmitted
	s.evidence.DispatchCancelled = true
	s.run(t, "operation.run")
	s.run(t, "operation.run")
	require.Equal(t, domain.StatusCancelled, s.loaded.Operation.Status)
	require.Zero(t, s.settled[0].ActualCostMicros)
}
func TestCodexTaskExecutedReceiptRejectsNotExecutedAndConsultsOriginal(t *testing.T) {
	s := newStateStore()
	s.loaded.Provider.AdapterKey = "codex"
	s.loaded.Operation.Status = domain.StatusManual
	s.run(t, "operation.run")
	require.Empty(t, s.settled)
	require.Equal(t, 1, s.recoveries)
}
