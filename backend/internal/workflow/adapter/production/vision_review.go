package production

import (
	"context"
	"errors"
	"strconv"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	genapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	flow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type VisionReviewInputSource interface {
	CompileBaseVisionReviewInput(context.Context, genapp.Actor, string, string, int, string) (contract.VisionReviewInput, error)
}
type VisionReviewOwner interface {
	Execute(context.Context, agentapp.ExecuteVisionReviewCommand) (agentapp.VisionReviewExecutionState, error)
}
type VisionReviewDependencies struct {
	Inputs           VisionReviewInputSource
	Execution        VisionReviewOwner
	StageReleaseHash string
}

func (executor *NodeExecutor) executeVisionReview(ctx context.Context, command flow.NodeExecutorCommand) (flow.NodeExecutorResult, error) {
	d := executor.visionReview
	if d == nil || d.Inputs == nil || d.Execution == nil || !workflowContentHashPattern.MatchString(d.StageReleaseHash) {
		return flow.NodeExecutorResult{}, errors.New("Vision Review Workflow dependencies are unavailable")
	}
	input, _, hash, err := flow.BuildNodeInput(command.Input)
	if err != nil || hash != command.InputHash || len(input.Bindings) != 0 || len(input.FrozenInputs) != 1 || len(command.OutputPorts) != 1 ||
		command.OutputPorts[0].Key != "candidate" || command.OutputPorts[0].ValueType != "vision_review_candidate" || !command.OutputPorts[0].Required || command.Attempt < 1 {
		return flow.NodeExecutorResult{}, errors.New("invalid Vision Review node contract")
	}
	var config struct {
		ExecutionRef gen.GenerationRevisionRef `json:"execution_ref"`
		BundleIndex  *int                      `json:"bundle_index"`
	}
	if canonical.Decode(input.Config, &config) != nil || !config.ExecutionRef.Valid() || config.BundleIndex == nil || *config.BundleIndex < 0 {
		return flow.NodeExecutorResult{}, errors.New("invalid Vision Review node config")
	}
	frozen := input.FrozenInputs[0]
	if frozen.Kind != "reference_execution" || frozen.ID != config.ExecutionRef.ID || frozen.Version != "1" || frozen.Hash != config.ExecutionRef.ContentHash {
		return flow.NodeExecutorResult{}, errors.New("Vision Review Execution is not frozen")
	}
	compiled, err := d.Inputs.CompileBaseVisionReviewInput(ctx, genapp.Actor{UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion}, command.ProjectID, config.ExecutionRef.ID, *config.BundleIndex, d.StageReleaseHash)
	if err != nil {
		return flow.NodeExecutorResult{}, err
	}
	if compiled.Subject.ExecutionRef != config.ExecutionRef || compiled.Subject.WorkspaceID != command.WorkspaceID || compiled.Subject.ProjectID != command.ProjectID || compiled.Subject.StageReleaseHash != d.StageReleaseHash || compiled.Subject.CandidateBundleIndex != *config.BundleIndex {
		return flow.NodeExecutorResult{}, errors.New("Vision Review compiled input escaped its node")
	}
	state, err := d.Execution.Execute(ctx, agentapp.ExecuteVisionReviewCommand{WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID, UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion, Input: compiled})
	if err != nil {
		return flow.NodeExecutorResult{}, err
	}
	switch state.Status {
	case "running":
		return flow.NodeExecutorResult{Status: "RETRYING"}, nil
	case "rejected", "outcome_unknown":
		return flow.NodeExecutorResult{Status: flow.NodeActivityNeedsAttention, ErrorCode: flow.VisionReviewUnconfirmedErrorCode, NextAction: flow.ManualVisionReviewNextAction}, nil
	case "accepted":
		candidate := state.Candidate
		if candidate.WorkspaceID != command.WorkspaceID || candidate.ProjectID != command.ProjectID || candidate.StageKey != contract.VisionReviewStageKey || candidate.CandidateType != "vision_review_candidate" || candidate.Revision != 1 || !workflowContentHashPattern.MatchString(candidate.CandidateRevisionHash) {
			return flow.NodeExecutorResult{}, errors.New("Vision Review candidate does not match Workflow scope")
		}
		output, _, _, err := flow.BuildNodeOutput(flow.NodeOutputSnapshot{SchemaVersion: flow.NodeOutputSchemaVersion, Bindings: []flow.NodeOutputBinding{{Port: "candidate", ValueType: "vision_review_candidate", ReferenceID: candidate.ID, ReferenceVersion: strconv.FormatInt(candidate.Revision, 10), ContentHash: candidate.CandidateRevisionHash}}})
		return flow.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, err
	default:
		return flow.NodeExecutorResult{}, errors.New("invalid Vision Review execution state")
	}
}
