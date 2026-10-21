package workflow

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// TrialWorkflow owns each dispatch once. An activity crash is unknown, never a replayed POST.
func TrialWorkflow(ctx workflow.Context, input application.TrialInput) (application.TrialState, error) {
	state := application.TrialState{Fingerprint: input.Request.Fingerprint(), Kind: input.Request.Kind, Source: input.Request.Source, ProviderID: input.Request.ProviderID, Model: input.Request.Model, State: "running", Turns: []application.TrialTurn{}}
	if err := workflow.SetQueryHandler(ctx, "state", func() (application.TrialState, error) { return state, nil }); err != nil {
		return state, err
	}
	if input.OriginalFingerprint != "" {
		state.Fingerprint = input.OriginalFingerprint
	}
	messages := workflow.GetSignalChannel(ctx, "turn")
	cancelSignal := workflow.GetSignalChannel(ctx, "cancel")
	queue := "provider"
	if input.Request.Source == "codex" {
		queue = "agent.codex"
	}
	options := workflow.ActivityOptions{TaskQueue: queue, StartToCloseTimeout: 10 * time.Minute, ScheduleToStartTimeout: 30 * time.Second, WaitForCancellation: true, HeartbeatTimeout: 15 * time.Second, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 1}}
	for {
		turn := application.TrialTurn{RequestID: input.Request.RequestID, Prompt: input.Request.Prompt, TrialProgress: application.TrialProgress{State: "running"}}
		// Artifacts and workflow identity stay bound to the initial request; chat turns only change the prompt.
		if len(state.Turns) > 0 {
			turn.RequestID = input.Request.RequestID
		}
		state.Turns = append(state.Turns, turn)
		idx := len(state.Turns) - 1
		state.State = "running"
		if input.Action == "" {
			input.Action = "submit"
		}
		queryCount := 0
		for {
			actCtx, cancel := workflow.WithCancel(workflow.WithActivityOptions(ctx, options))
			future := workflow.ExecuteActivity(actCtx, "trial.Run", input)
			cancelled := false
			selector := workflow.NewSelector(ctx)
			selector.AddFuture(future, func(workflow.Future) {})
			selector.AddReceive(cancelSignal, func(c workflow.ReceiveChannel, _ bool) {
				var signal bool
				c.Receive(ctx, &signal)
				cancelled = true
				cancel()
			})
			selector.Select(ctx)
			var result application.TrialProgress
			err := future.Get(ctx, &result)
			cancel()
			if err != nil || cancelled {
				result = application.TrialProgress{State: "unknown", ErrorCode: "trial_dispatch_unknown", TaskID: input.TaskID, ConfigVersionID: input.ConfigVersionID, CredentialID: input.CredentialID}
			}
			if result.State == "pending" && result.TaskID != "" {
				queryCount++
				if queryCount > 240 {
					result.State = "unknown"
					result.ErrorCode = "trial_query_timeout"
					state.Turns[idx].TrialProgress = result
					state.State = "unknown"
					return state, nil
				}
				state.Turns[idx].TrialProgress = result
				input.Action = "query"
				input.TaskID = result.TaskID
				input.ConfigVersionID = result.ConfigVersionID
				input.CredentialID = result.CredentialID
				timer := workflow.NewTimer(ctx, 5*time.Second)
				wait := workflow.NewSelector(ctx)
				wait.AddFuture(timer, func(workflow.Future) {})
				wait.AddReceive(cancelSignal, func(c workflow.ReceiveChannel, _ bool) { var b bool; c.Receive(ctx, &b); cancelled = true })
				wait.Select(ctx)
				if !cancelled {
					continue
				}
				result.State = "unknown"
				result.ErrorCode = "trial_cancel_unconfirmed"
			}
			state.Turns[idx].TrialProgress = result
			state.State = result.State
			if result.ThreadID != "" {
				input.ThreadID = result.ThreadID
			}
			break
		}
		if input.Request.Kind != "chat" || state.State != "succeeded" || len(state.Turns) >= 32 {
			return state, nil
		}
		for {
			var next application.TrialMessage
			got := false
			selector := workflow.NewSelector(ctx)
			selector.AddReceive(messages, func(c workflow.ReceiveChannel, _ bool) { c.Receive(ctx, &next); got = true })
			selector.AddReceive(cancelSignal, func(c workflow.ReceiveChannel, _ bool) { var b bool; c.Receive(ctx, &b) })
			timerCtx, cancelTimer := workflow.WithCancel(ctx)
			selector.AddFuture(workflow.NewTimer(timerCtx, 24*time.Hour), func(workflow.Future) {})
			selector.Select(ctx)
			cancelTimer()
			if !got {
				state.State = "closed"
				return state, nil
			}
			duplicate := false
			for _, previous := range state.Turns {
				if previous.RequestID == next.RequestID {
					duplicate = true
				}
			}
			if duplicate {
				continue
			}
			candidate := input.Request
			candidate.RequestID = next.RequestID
			candidate.Prompt = next.Prompt
			if candidate.Validate() != nil {
				continue
			}
			input.Request.RequestID = next.RequestID
			input.Request.Prompt = next.Prompt
			input.Action = "submit"
			break
		}
	}
}
