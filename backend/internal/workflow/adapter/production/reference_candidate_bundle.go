package production

import (
	"context"
	"errors"

	genapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	flow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type ReferenceCandidateBundleOwner interface {
	Materialize(context.Context, genapp.Actor, genapp.ReferenceCandidateBundleCommand) (gen.ReferenceCandidateBundle, error)
}

func (executor *NodeExecutor) executeReferenceCandidateBundle(ctx context.Context, command flow.NodeExecutorCommand) (flow.NodeExecutorResult, error) {
	if executor.referenceBundles == nil {
		return flow.NodeExecutorResult{}, errors.New("reference candidate Bundle Owner unavailable")
	}
	input, _, hash, err := flow.BuildNodeInput(command.Input)
	var config struct{}
	if err != nil || hash != command.InputHash || canonical.Decode(input.Config, &config) != nil || len(input.Bindings) != 1 || len(input.FrozenInputs) != 1 || len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "bundle" || command.OutputPorts[0].ValueType != "reference_candidate_bundle" || !command.OutputPorts[0].Required || command.Attempt < 1 {
		return flow.NodeExecutorResult{}, errors.New("invalid reference candidate Bundle node")
	}
	binding := input.Bindings[0]
	frozen := input.FrozenInputs[0]
	if binding.Port != "review" || binding.ValueType != "vision_review_candidate" || binding.SourceKind != flow.NodeInputSourceNodeOutput || binding.ReferenceVersion != "1" || frozen.Kind != "reference_execution" || frozen.Version != "1" {
		return flow.NodeExecutorResult{}, errors.New("candidate Bundle requires exact review and Execution")
	}
	ownerCommand := genapp.ReferenceCandidateBundleCommand{WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, ExecutionRef: gen.GenerationRevisionRef{ID: frozen.ID, Revision: 1, ContentHash: frozen.Hash}, VisionReviewRef: gen.GenerationRevisionRef{ID: binding.ReferenceID, Revision: 1, ContentHash: binding.ContentHash}}
	if !ownerCommand.ExecutionRef.Valid() || !ownerCommand.VisionReviewRef.Valid() {
		return flow.NodeExecutorResult{}, errors.New("invalid candidate Bundle references")
	}
	value, err := executor.referenceBundles.Materialize(ctx, genapp.Actor{UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion}, ownerCommand)
	if err != nil {
		return flow.NodeExecutorResult{}, err
	}
	if value.WorkspaceID != command.WorkspaceID || value.ProjectID != command.ProjectID || value.ExecutionRef != ownerCommand.ExecutionRef || value.VisionReviewRef != ownerCommand.VisionReviewRef || !(gen.GenerationRevisionRef{ID: value.ID, Revision: 1, ContentHash: value.ContentHash}).Valid() {
		return flow.NodeExecutorResult{}, errors.New("candidate Bundle Owner returned unrelated identity")
	}
	output, _, _, err := flow.BuildNodeOutput(flow.NodeOutputSnapshot{SchemaVersion: flow.NodeOutputSchemaVersion, Bindings: []flow.NodeOutputBinding{{Port: "bundle", ValueType: "reference_candidate_bundle", ReferenceID: value.ID, ReferenceVersion: "1", ContentHash: value.ContentHash}}})
	return flow.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, err
}
