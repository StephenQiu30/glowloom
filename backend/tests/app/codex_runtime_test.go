package app_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/codexruntime"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

type localOperationReader struct{ loaded application.WorkflowOperation }

func (r localOperationReader) LoadWorkflowOperation(context.Context, uuid.UUID) (application.WorkflowOperation, error) {
	return r.loaded, nil
}

type localImageSubmitter struct {
	calls int
	input application.ProviderImageInput
}

func (s *localImageSubmitter) Submit(_ context.Context, input application.ProviderImageInput) (application.ProviderImageResult, error) {
	s.calls++
	s.input = input
	return application.ProviderImageResult{Outcome: "unknown", FailureCode: "uncertain"}, nil
}
func TestCodexWorkerReloadsFrozenPromptAndRejectsForeignIdentity(t *testing.T) {
	model, price, project, id := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	prompt := "stored prompt"
	loaded := application.WorkflowOperation{Operation: domain.Operation{ID: id, ProjectID: project, Capability: "image.generate", Mode: "text_to_image", OutputCount: 1, ModelProfileVersionID: &model, PriceRuleVersionID: &price}, Provider: application.WorkflowProvider{AdapterKey: "codex", Queue: "agent.codex", ProviderModelID: "fixture-model"}, ProviderRequestKey: "request", Inputs: []domain.OperationInput{{Role: "prompt", TextValue: &prompt}}}
	submitter := &localImageSubmitter{}
	activities := codexruntime.NewImageActivities(localOperationReader{loaded}, submitter)
	input := operationflow.ProviderSubmitInput{OperationID: id.String(), ProjectID: project.String(), ModelProfileVersionID: model.String(), PriceRuleVersionID: price.String(), ProviderModelID: "fixture-model", ProviderRequestKey: "request", Attempt: 1}
	forged := "untrusted override"
	input.Inputs = []operationflow.ProviderInput{{Role: "prompt", Text: &forged}}
	output, err := activities.Submit(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, operationflow.ProviderSubmitUnknown, output.Outcome)
	require.Equal(t, 1, submitter.calls)
	require.Equal(t, prompt, submitter.input.Prompt)
	input.ProjectID = uuid.NewString()
	output, err = activities.Submit(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, operationflow.ProviderSubmitNotSubmitted, output.Outcome)
	require.Equal(t, 1, submitter.calls)
}
