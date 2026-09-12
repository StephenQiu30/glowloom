package generation

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	app "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	flow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
	"github.com/google/uuid"
)

type ReferenceCallObservationOwner interface {
	ReferenceCallExecutor
	Observe(context.Context, app.Actor, app.ClaimReferenceCallCommand) (gen.ReferenceCallState, error)
}

type ReferenceExecutionProgressQuery interface {
	Get(context.Context, app.Actor, string, string) (gen.ReferenceJobProgress, error)
}

type ReferenceObservationNodeExecutor struct {
	strict    *ReferenceCallNodeExecutor
	execution ReferenceCallObservationOwner
	progress  ReferenceExecutionProgressQuery
}

func NewReferenceObservationNodeExecutor(execution ReferenceCallObservationOwner, recovery ReferenceCallRecovery, media ReferenceStagedMediaMaterializer, progress ReferenceExecutionProgressQuery) (*ReferenceObservationNodeExecutor, error) {
	strict, err := NewReferenceCallNodeExecutor(execution, recovery, media)
	if err != nil {
		return nil, err
	}
	if progress == nil {
		return nil, errors.New("Reference execution progress Owner is required")
	}
	return &ReferenceObservationNodeExecutor{strict: strict, execution: execution, progress: progress}, nil
}

type referenceObservationConfig struct {
	ExecutionRef    gen.GenerationRevisionRef `json:"execution_ref"`
	JobHash         string                    `json:"job_hash"`
	PreviousCallKey string                    `json:"previous_call_key"`
}

func (executor *ReferenceObservationNodeExecutor) Execute(ctx context.Context, command flow.NodeExecutorCommand) (flow.NodeExecutorResult, error) {
	if executor == nil || executor.strict == nil || executor.execution == nil || executor.progress == nil {
		return flow.NodeExecutorResult{}, errors.New("Reference observation Owners are unavailable")
	}
	if command.Executor == "activity.reference_image_call" {
		return executor.strict.Execute(ctx, command)
	}
	summary := command.Executor == "activity.reference_execution_observation"
	if !summary && command.Executor != "activity.reference_call_observation" {
		return flow.NodeExecutorResult{}, errors.New("unsupported Reference observation executor")
	}
	if command.Attempt < 1 || command.InitiatorTokenVersion < 1 || strings.TrimSpace(command.IdempotencyKey) == "" {
		return flow.NodeExecutorResult{}, errors.New("invalid Reference observation boundary")
	}
	for _, id := range []string{command.WorkspaceID, command.ProjectID, command.InitiatorUserID, command.WorkflowRunID, command.NodeRunID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil || parsed.String() != id {
			return flow.NodeExecutorResult{}, errors.New("invalid Reference observation scope")
		}
	}
	var config referenceObservationConfig
	var key string
	if summary {
		if err := canonical.Decode(command.Input.Config, &config); err != nil {
			return flow.NodeExecutorResult{}, errors.New("invalid Reference summary config")
		}
	} else {
		var call struct {
			referenceObservationConfig
			CallKey string `json:"call_key"`
		}
		if err := canonical.Decode(command.Input.Config, &call); err != nil || !candidateSetHashPattern.MatchString(call.CallKey) {
			return flow.NodeExecutorResult{}, errors.New("invalid Reference call observation config")
		}
		config, key = call.referenceObservationConfig, call.CallKey
	}
	if !config.ExecutionRef.Valid() || !candidateSetHashPattern.MatchString(config.JobHash) {
		return flow.NodeExecutorResult{}, errors.New("invalid Reference observation identity")
	}
	input, _, hash, err := flow.BuildNodeInput(command.Input)
	port, valueType := "receipt", "reference_call_receipt"
	if summary {
		port, valueType = "execution", "reference_execution_observation"
	}
	if err != nil || hash != command.InputHash || len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != port || command.OutputPorts[0].ValueType != valueType || !command.OutputPorts[0].Required {
		return flow.NodeExecutorResult{}, errors.New("invalid Reference observation node contract")
	}
	if len(input.FrozenInputs) != 1 || input.FrozenInputs[0].Kind != "reference_execution" || input.FrozenInputs[0].ID != config.ExecutionRef.ID || input.FrozenInputs[0].Version != "1" || input.FrozenInputs[0].Hash != config.ExecutionRef.ContentHash {
		return flow.NodeExecutorResult{}, errors.New("Reference observation is not bound to its frozen Execution")
	}
	actor := app.Actor{UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion}
	progress, err := executor.progress.Get(ctx, actor, command.ProjectID, config.ExecutionRef.ID)
	if err != nil || progress.ExecutionRef != config.ExecutionRef || progress.JobHash != config.JobHash {
		return flow.NodeExecutorResult{}, errors.New("Reference observation Job has drifted")
	}
	if _, err := workflowapp.BuildReferenceExecutionGraph(progress); err != nil {
		return flow.NodeExecutorResult{}, errors.New("Reference observation Job is incomplete")
	}
	index := len(progress.Calls)
	if !summary {
		index = slices.IndexFunc(progress.Calls, func(call gen.ReferenceCallProgress) bool { return call.CallKey == key })
		if index < 0 {
			return flow.NodeExecutorResult{}, errors.New("Reference Call is outside Job")
		}
	}
	previous := ""
	if index > 0 {
		previous = progress.Calls[index-1].CallKey
	}
	if config.PreviousCallKey != previous || (previous == "" && len(input.Bindings) != 0) || (previous != "" && len(input.Bindings) != 1) {
		return flow.NodeExecutorResult{}, errors.New("Reference observation sequence has drifted")
	}
	callCommand := app.ClaimReferenceCallCommand{WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, ExecutionRef: config.ExecutionRef, ExpectedRevision: 1}
	observe := func(key string) (gen.ReferenceCallState, error) {
		read := callCommand
		read.CallKey = key
		state, err := executor.execution.Observe(ctx, actor, read)
		if err != nil {
			return gen.ReferenceCallState{}, errors.New("Reference outcome read failed")
		}
		if err := validateReferenceObservationState(state, read); err != nil {
			return gen.ReferenceCallState{}, err
		}
		return state, nil
	}
	if previous != "" {
		state, err := observe(previous)
		if err != nil || state.Receipt == nil || (state.Status != gen.ProviderCallSucceeded && state.Status != gen.ProviderCallFailed) {
			return flow.NodeExecutorResult{}, errors.New("Reference predecessor has no explicit outcome")
		}
		binding := input.Bindings[0]
		if binding.Port != "previous" || binding.ValueType != "reference_call_receipt" || binding.SourceKind != "node_output" || binding.SourcePort != "receipt" || binding.ReferenceVersion != "1" || binding.ReferenceID != state.Receipt.SubmissionToken || binding.ContentHash != state.Receipt.ContentHash {
			return flow.NodeExecutorResult{}, errors.New("Reference predecessor receipt has drifted")
		}
	}
	if progress.OutcomeUnknown > 0 {
		return referenceObservationAttention(), nil
	}
	if summary {
		if !progress.Terminal {
			return flow.NodeExecutorResult{}, errors.New("Reference execution still has unresolved Calls")
		}
		for _, call := range progress.Calls {
			if call.Status != gen.ProviderCallSucceeded {
				continue
			}
			state, err := observe(call.CallKey)
			if err != nil {
				return flow.NodeExecutorResult{}, err
			}
			if state.Status != call.Status || state.Revision != call.Revision || state.ContentHash != call.StateHash {
				return flow.NodeExecutorResult{}, errors.New("Reference summary outcome has drifted")
			}
			if err := executor.materialize(ctx, actor, state); err != nil {
				return flow.NodeExecutorResult{}, err
			}
		}
		return referenceObservationOutput(port, valueType, config.ExecutionRef.ID, progress.ContentHash)
	}
	callCommand.CallKey = key
	state, err := executor.execution.Execute(ctx, actor, callCommand)
	if err != nil {
		return flow.NodeExecutorResult{}, errors.New("Reference call observation execution failed")
	}
	if err := validateReferenceObservationState(state, callCommand); err != nil {
		return flow.NodeExecutorResult{}, err
	}
	if state.Status == gen.ProviderCallDispatching {
		dispatch := *state.Dispatch
		state, err = executor.strict.recovery.Expire(ctx, actor, app.ExpireReferenceCallCommand{WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, ExecutionRef: config.ExecutionRef, CallKey: key, SubmissionToken: dispatch.SubmissionToken})
		if err != nil || validateReferenceObservationState(state, callCommand) != nil || state.Dispatch == nil || *state.Dispatch != dispatch {
			return flow.NodeExecutorResult{}, errors.New("Reference observation recovery failed")
		}
	}
	switch state.Status {
	case gen.ProviderCallDispatching:
		return flow.NodeExecutorResult{Status: "RETRYING"}, nil
	case gen.ProviderCallOutcomeUnknown:
		return referenceObservationAttention(), nil
	case gen.ProviderCallSucceeded:
		if err := executor.materialize(ctx, actor, state); err != nil {
			return flow.NodeExecutorResult{}, err
		}
	case gen.ProviderCallFailed:
	default:
		return flow.NodeExecutorResult{}, errors.New("Reference Call has no explicit outcome")
	}
	return referenceObservationOutput(port, valueType, state.Receipt.SubmissionToken, state.Receipt.ContentHash)
}

func validateReferenceObservationState(state gen.ReferenceCallState, command app.ClaimReferenceCallCommand) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if _, err = gen.DecodeReferenceCallState(raw); err != nil || state.CallKey != command.CallKey {
		return errors.New("Reference observation state has drifted")
	}
	if state.Receipt != nil && (state.Receipt.WorkspaceID != command.WorkspaceID || state.Receipt.ProjectID != command.ProjectID || state.Receipt.Call.ExecutionRef != command.ExecutionRef) {
		return errors.New("Reference observation receipt has drifted")
	}
	return nil
}

func (executor *ReferenceObservationNodeExecutor) materialize(ctx context.Context, actor app.Actor, state gen.ReferenceCallState) error {
	receipt := state.Receipt
	if state.Status != gen.ProviderCallSucceeded || receipt == nil {
		return errors.New("Reference media requires a successful receipt")
	}
	media, err := executor.strict.media.Materialize(ctx, actor, app.MaterializeReferenceStagedMediaCommand{WorkspaceID: receipt.WorkspaceID, ProjectID: receipt.ProjectID, ExecutionRef: receipt.Call.ExecutionRef, CallKey: state.CallKey, ReceiptRef: gen.GenerationActionRef{ID: receipt.SubmissionToken, ContentHash: receipt.ContentHash}})
	if err != nil {
		return errors.New("Reference observed media materialization failed")
	}
	raw, err := json.Marshal(media)
	if err != nil {
		return err
	}
	if _, err := gen.DecodeReferenceStagedMedia(raw); err != nil || (media.State != "ready_for_review" && media.State != "rejected") {
		return errors.New("Reference observed media has no validated outcome")
	}
	expected, err := gen.NewReferenceStagedMedia(*receipt, media.ObjectStoreRef)
	initial, identityErr := gen.InitialReferenceStagedMedia(media)
	if err != nil || identityErr != nil || expected.ContentHash != initial.ContentHash {
		return errors.New("Reference observed media identity has drifted")
	}
	return nil
}

func referenceObservationOutput(port, valueType, id, hash string) (flow.NodeExecutorResult, error) {
	output, _, _, err := flow.BuildNodeOutput(flow.NodeOutputSnapshot{SchemaVersion: flow.NodeOutputSchemaVersion, Bindings: []flow.NodeOutputBinding{{Port: port, ValueType: valueType, ReferenceID: id, ReferenceVersion: "1", ContentHash: hash}}})
	if err != nil {
		return flow.NodeExecutorResult{}, err
	}
	return flow.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func referenceObservationAttention() flow.NodeExecutorResult {
	return flow.NodeExecutorResult{Status: flow.NodeActivityNeedsAttention, ErrorCode: flow.ProviderOutcomeUnknownErrorCode, NextAction: flow.ManualProviderReconciliationNextAction}
}
