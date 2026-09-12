package generation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"

	app "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	flow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type ReferenceCallExecutor interface {
	Execute(context.Context, app.Actor, app.ClaimReferenceCallCommand) (gen.ReferenceCallState, error)
}

type ReferenceCallRecovery interface {
	Expire(context.Context, app.Actor, app.ExpireReferenceCallCommand) (gen.ReferenceCallState, error)
}

type ReferenceStagedMediaMaterializer interface {
	Materialize(context.Context, app.Actor, app.MaterializeReferenceStagedMediaCommand) (gen.ReferenceStagedMedia, error)
}

type ReferenceCallNodeExecutor struct {
	execution ReferenceCallExecutor
	recovery  ReferenceCallRecovery
	media     ReferenceStagedMediaMaterializer
}

func NewReferenceCallNodeExecutor(execution ReferenceCallExecutor, recovery ReferenceCallRecovery, media ReferenceStagedMediaMaterializer) (*ReferenceCallNodeExecutor, error) {
	if execution == nil || recovery == nil || media == nil {
		return nil, errors.New("Reference call execution, recovery and staged media owners are required")
	}
	return &ReferenceCallNodeExecutor{execution: execution, recovery: recovery, media: media}, nil
}

func (executor *ReferenceCallNodeExecutor) Execute(ctx context.Context, command flow.NodeExecutorCommand) (flow.NodeExecutorResult, error) {
	if executor == nil || executor.execution == nil || executor.recovery == nil || executor.media == nil || command.Executor != "activity.reference_image_call" || command.Attempt < 1 || command.InitiatorTokenVersion < 1 || strings.TrimSpace(command.IdempotencyKey) == "" {
		return flow.NodeExecutorResult{}, errors.New("invalid Reference call workflow boundary")
	}
	for _, id := range []string{command.WorkspaceID, command.ProjectID, command.InitiatorUserID, command.WorkflowRunID, command.NodeRunID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil || parsed.String() != id {
			return flow.NodeExecutorResult{}, errors.New("invalid Reference call workflow scope")
		}
	}
	// Decode the original config before normalization so duplicate keys cannot be
	// silently collapsed into a different request by generic node normalization.
	var config struct {
		ExecutionRef gen.GenerationRevisionRef `json:"execution_ref"`
		CallKey      string                    `json:"call_key"`
	}
	if err := canonical.Decode(command.Input.Config, &config); err != nil || !config.ExecutionRef.Valid() || !candidateSetHashPattern.MatchString(config.CallKey) {
		return flow.NodeExecutorResult{}, errors.New("invalid Reference call workflow config")
	}
	input, _, hash, err := flow.BuildNodeInput(command.Input)
	if err != nil || hash != command.InputHash || len(input.Bindings) != 0 || len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "receipt" || command.OutputPorts[0].ValueType != "reference_call_receipt" || !command.OutputPorts[0].Required {
		return flow.NodeExecutorResult{}, errors.New("invalid Reference call node contract")
	}
	actor := app.Actor{UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion}
	state, err := executor.execution.Execute(ctx, actor, app.ClaimReferenceCallCommand{WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, ExecutionRef: config.ExecutionRef, CallKey: config.CallKey, ExpectedRevision: 1})
	if err != nil {
		return flow.NodeExecutorResult{}, errors.New("Reference call execution failed")
	}
	validate := func(state gen.ReferenceCallState) error {
		raw, err := json.Marshal(state)
		if err != nil {
			return err
		}
		if _, err = gen.DecodeReferenceCallState(raw); err != nil {
			return err
		}
		if state.CallKey != config.CallKey {
			return errors.New("foreign Reference call state")
		}
		if state.Receipt != nil && (state.Receipt.WorkspaceID != command.WorkspaceID || state.Receipt.ProjectID != command.ProjectID || state.Receipt.Call.ExecutionRef != config.ExecutionRef) {
			return errors.New("foreign Reference call receipt")
		}
		return nil
	}
	if err := validate(state); err != nil {
		return flow.NodeExecutorResult{}, errors.New("Reference call state failed verification")
	}
	if state.Status == gen.ProviderCallDispatching {
		dispatch := *state.Dispatch
		state, err = executor.recovery.Expire(ctx, actor, app.ExpireReferenceCallCommand{WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, ExecutionRef: config.ExecutionRef, CallKey: config.CallKey, SubmissionToken: state.Dispatch.SubmissionToken})
		if err != nil {
			return flow.NodeExecutorResult{}, errors.New("Reference call recovery failed")
		}
		if err := validate(state); err != nil {
			return flow.NodeExecutorResult{}, errors.New("Reference call recovery state failed verification")
		}
		if state.Dispatch == nil || *state.Dispatch != dispatch {
			return flow.NodeExecutorResult{}, errors.New("Reference call recovery changed dispatch identity")
		}
	}
	switch state.Status {
	case gen.ProviderCallDispatching:
		return flow.NodeExecutorResult{Status: "RETRYING"}, nil
	case gen.ProviderCallOutcomeUnknown:
		return flow.NodeExecutorResult{Status: flow.NodeActivityNeedsAttention, ErrorCode: flow.ProviderOutcomeUnknownErrorCode, NextAction: flow.ManualProviderReconciliationNextAction}, nil
	case gen.ProviderCallSucceeded:
		media, err := executor.media.Materialize(ctx, actor, app.MaterializeReferenceStagedMediaCommand{WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, ExecutionRef: config.ExecutionRef, CallKey: config.CallKey, ReceiptRef: gen.GenerationActionRef{ID: state.Receipt.SubmissionToken, ContentHash: state.Receipt.ContentHash}})
		if err != nil {
			return flow.NodeExecutorResult{}, errors.New("Reference staged media materialization failed")
		}
		raw, err := json.Marshal(media)
		if err != nil {
			return flow.NodeExecutorResult{}, errors.New("Reference staged media encoding failed")
		}
		if _, err := gen.DecodeReferenceStagedMedia(raw); err != nil || media.State != "ready_for_review" {
			return flow.NodeExecutorResult{}, errors.New("Reference staged media is not ready for review")
		}
		expected, err := gen.NewReferenceStagedMedia(*state.Receipt, media.ObjectStoreRef)
		initial, identityErr := gen.InitialReferenceStagedMedia(media)
		if err != nil || identityErr != nil || initial.ContentHash != expected.ContentHash {
			return flow.NodeExecutorResult{}, errors.New("Reference staged media differs from receipt")
		}
		output, _, _, err := flow.BuildNodeOutput(flow.NodeOutputSnapshot{SchemaVersion: flow.NodeOutputSchemaVersion, Bindings: []flow.NodeOutputBinding{{Port: "receipt", ValueType: "reference_call_receipt", ReferenceID: state.Receipt.SubmissionToken, ReferenceVersion: "1", ContentHash: state.Receipt.ContentHash}}})
		if err != nil {
			return flow.NodeExecutorResult{}, err
		}
		return flow.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
	default:
		return flow.NodeExecutorResult{}, errors.New("Reference call did not produce a staged receipt")
	}
}

var _ workflowapp.NodeExecutor = (*ReferenceCallNodeExecutor)(nil)
