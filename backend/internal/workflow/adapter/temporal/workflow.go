package temporal

import (
	"strings"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const (
	EpisodeProductionWorkflowName  = "lanverse.episode-production"
	LoadExecutionPlanActivityName  = "lanverse.workflow.load-execution-plan"
	ExecuteNodeActivityName        = "lanverse.workflow.execute-node"
	OpenHumanGateActivityName      = "lanverse.workflow.open-human-gate"
	ApplyHumanGateActivityName     = "lanverse.workflow.apply-human-gate"
	CompleteRunActivityName        = "lanverse.workflow.complete-run"
	HumanGateSignalName            = "lanverse.workflow.human-gate-decision"
	workflowContractViolationError = "workflow_contract_violation"
)

type ExecutionPlan = workflowdomain.ExecutionPlan
type ExecutionNode = workflowdomain.ExecutionNode
type NodeActivityCommand = workflowdomain.NodeActivityCommand
type NodeActivityResult = workflowdomain.NodeActivityResult

type HumanGateSignal struct {
	WorkflowRunID  string `json:"workflow_run_id"`
	NodeRunID      string `json:"node_run_id"`
	SignalID       string `json:"signal_id"`
	SignalIntentID string `json:"signal_intent_id"`
	Decision       string `json:"decision"`
}

type ApplyHumanGateCommand = workflowdomain.ApplyHumanGateCommand
type CompleteRunCommand = workflowdomain.CompleteRunCommand

type RunResult struct {
	WorkflowRunID string `json:"workflow_run_id"`
	Status        string `json:"status"`
}

func EpisodeProductionWorkflow(ctx workflow.Context, request workflowdomain.StartRequest) (RunResult, error) {
	if !validEpisodeStart(request) {
		return RunResult{}, contractViolation("invalid episode workflow start input")
	}

	var plan ExecutionPlan
	loadContext := workflow.WithActivityOptions(ctx, shortActivityOptions("load-execution-plan"))
	if err := workflow.ExecuteActivity(loadContext, LoadExecutionPlanActivityName, request).Get(ctx, &plan); err != nil {
		return RunResult{}, err
	}
	if !validExecutionPlan(plan, request) {
		return RunResult{}, contractViolation("execution plan does not match start input")
	}

	signalChannel := workflow.GetSignalChannel(ctx, HumanGateSignalName)
	pendingSignals := make(map[string]HumanGateSignal)
	for _, node := range plan.Nodes {
		command := NodeActivityCommand{
			WorkflowRunID: request.WorkflowRunID, NodeRunID: node.NodeRunID,
			NodeID: node.NodeID, Executor: node.Executor, Attempt: 1,
		}
		if node.RiskLevel == "human_gate" {
			openContext := workflow.WithActivityOptions(ctx, shortActivityOptions("open-human-gate:"+node.NodeRunID))
			if err := workflow.ExecuteActivity(openContext, OpenHumanGateActivityName, command).Get(ctx, nil); err != nil {
				return RunResult{}, err
			}
			signal, err := awaitHumanGateSignal(ctx, signalChannel, pendingSignals, request.WorkflowRunID, node.NodeRunID)
			if err != nil {
				return RunResult{}, err
			}
			apply := ApplyHumanGateCommand{
				WorkflowRunID: request.WorkflowRunID, NodeRunID: node.NodeRunID, NodeID: node.NodeID,
				SignalIntentID: signal.SignalIntentID, Decision: signal.Decision,
			}
			applyContext := workflow.WithActivityOptions(ctx, shortActivityOptions("apply-human-gate:"+node.NodeRunID))
			if err = workflow.ExecuteActivity(applyContext, ApplyHumanGateActivityName, apply).Get(ctx, nil); err != nil {
				return RunResult{}, err
			}
			if signal.Decision != "APPROVED" && signal.Decision != "SELECTED" {
				return RunResult{}, contractViolation("human gate did not approve workflow continuation")
			}
			continue
		}

		activityContext := workflow.WithActivityOptions(ctx, nodeActivityOptions("execute-node:"+node.NodeRunID))
		var result NodeActivityResult
		if err := workflow.ExecuteActivity(activityContext, ExecuteNodeActivityName, command).Get(ctx, &result); err != nil {
			return RunResult{}, err
		}
		if result.Status != "SUCCEEDED" && result.Status != "CACHED" && result.Status != "SKIPPED" {
			return RunResult{}, contractViolation("node activity returned a non-terminal success status")
		}
	}

	completeContext := workflow.WithActivityOptions(ctx, shortActivityOptions("complete-run"))
	if err := workflow.ExecuteActivity(completeContext, CompleteRunActivityName, CompleteRunCommand{
		WorkflowRunID: request.WorkflowRunID,
	}).Get(ctx, nil); err != nil {
		return RunResult{}, err
	}
	return RunResult{WorkflowRunID: request.WorkflowRunID, Status: "SUCCEEDED"}, nil
}

func validEpisodeStart(request workflowdomain.StartRequest) bool {
	return validRequest(request) && request.WorkflowType == EpisodeProductionWorkflowName &&
		request.WorkflowTypeVersion == "1.0.0"
}

func validExecutionPlan(plan ExecutionPlan, request workflowdomain.StartRequest) bool {
	if plan.WorkflowRunID != request.WorkflowRunID || plan.DefinitionVersionID != request.DefinitionVersionID ||
		plan.RunInputSnapshotID != request.RunInputSnapshotID || plan.DefinitionContentHash != request.DefinitionContentHash ||
		plan.InputSnapshotHash != request.InputSnapshotHash || len(plan.Nodes) == 0 || len(plan.Nodes) > 500 {
		return false
	}
	seenRuns := make(map[string]struct{}, len(plan.Nodes))
	seenNodes := make(map[string]struct{}, len(plan.Nodes))
	for _, node := range plan.Nodes {
		if strings.TrimSpace(node.NodeRunID) == "" || strings.TrimSpace(node.NodeID) == "" || strings.TrimSpace(node.Executor) == "" ||
			(node.RiskLevel != "low" && node.RiskLevel != "external_ai" && node.RiskLevel != "human_gate") {
			return false
		}
		if _, exists := seenRuns[node.NodeRunID]; exists {
			return false
		}
		if _, exists := seenNodes[node.NodeID]; exists {
			return false
		}
		seenRuns[node.NodeRunID] = struct{}{}
		seenNodes[node.NodeID] = struct{}{}
	}
	return true
}

func awaitHumanGateSignal(
	ctx workflow.Context,
	channel workflow.ReceiveChannel,
	pending map[string]HumanGateSignal,
	workflowRunID string,
	nodeRunID string,
) (HumanGateSignal, error) {
	if signal, exists := pending[nodeRunID]; exists {
		delete(pending, nodeRunID)
		if validHumanGateSignal(signal, workflowRunID, nodeRunID) {
			return signal, nil
		}
		return HumanGateSignal{}, contractViolation("invalid human gate signal")
	}
	for {
		var signal HumanGateSignal
		selector := workflow.NewSelector(ctx)
		received := false
		selector.AddReceive(channel, func(channel workflow.ReceiveChannel, _ bool) {
			channel.Receive(ctx, &signal)
			received = true
		})
		selector.AddReceive(ctx.Done(), func(workflow.ReceiveChannel, bool) {})
		selector.Select(ctx)
		if ctx.Err() != nil {
			return HumanGateSignal{}, ctx.Err()
		}
		if !received {
			continue
		}
		if signal.WorkflowRunID != workflowRunID {
			return HumanGateSignal{}, contractViolation("human gate signal targets another workflow run")
		}
		if signal.NodeRunID != nodeRunID {
			pending[signal.NodeRunID] = signal
			continue
		}
		if !validHumanGateSignal(signal, workflowRunID, nodeRunID) {
			return HumanGateSignal{}, contractViolation("invalid human gate signal")
		}
		return signal, nil
	}
}

func validHumanGateSignal(signal HumanGateSignal, workflowRunID, nodeRunID string) bool {
	if signal.WorkflowRunID != workflowRunID || signal.NodeRunID != nodeRunID || strings.TrimSpace(signal.SignalID) == "" ||
		strings.TrimSpace(signal.SignalIntentID) == "" {
		return false
	}
	switch signal.Decision {
	case "APPROVED", "REJECTED", "CHANGES_REQUESTED", "SELECTED":
		return true
	default:
		return false
	}
}

func shortActivityOptions(activityID string) workflow.ActivityOptions {
	return workflow.ActivityOptions{
		ActivityID: activityID, StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{InitialInterval: time.Second, BackoffCoefficient: 2, MaximumInterval: 10 * time.Second, MaximumAttempts: 3},
	}
}

func nodeActivityOptions(activityID string) workflow.ActivityOptions {
	return workflow.ActivityOptions{
		ActivityID: activityID, StartToCloseTimeout: 10 * time.Minute, HeartbeatTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{InitialInterval: time.Second, BackoffCoefficient: 2, MaximumInterval: time.Minute, MaximumAttempts: 3},
	}
}

func contractViolation(message string) error {
	return temporal.NewNonRetryableApplicationError(message, workflowContractViolationError, nil)
}
