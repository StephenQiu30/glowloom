package catalog_test

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"

	catalogflow "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/workflow"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
)

func TestCredentialTestWorkflowSealsAgentBoundaryAndRecordsOnlySuccess(t *testing.T) {
	input := catalogflow.Input{
		TestID: uuid.New(), ProviderID: uuid.New(), CredentialID: uuid.New(),
		ActorID: uuid.New(), OrgID: uuid.New(), RequestID: uuid.NewString(),
	}
	for _, tc := range []struct {
		name       string
		agentError error
		wantRecord bool
	}{
		{name: "successful probe", wantRecord: true},
		{name: "failed probe", agentError: errors.New("probe unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var sequence []string
			env := (&testsuite.WorkflowTestSuite{}).NewTestWorkflowEnvironment()
			env.RegisterActivityWithOptions(func(_ context.Context, got catalogflow.Input) (catalogflow.LoadedCredential, error) {
				sequence = append(sequence, "load")
				if got != input {
					t.Fatalf("wrong load request: %+v", got)
				}
				return catalogflow.LoadedCredential{
					ProviderID: input.ProviderID, ProviderKey: "openrouter", AdapterKey: "openrouter",
					CredentialID: input.CredentialID, KeyID: "agent-2026", Ciphertext: []byte{0, 1, 2, 3}, Last4: "1234",
				}, nil
			}, activity.RegisterOptions{Name: "LoadCredentialForTest"})
			env.RegisterActivityWithOptions(func(_ context.Context, got catalogflow.AgentTestInput) (catalogflow.AgentTestOutput, error) {
				sequence = append(sequence, "agent")
				if got.ProviderID != input.ProviderID || got.ProviderKey != "openrouter" ||
					got.AdapterKey != "openrouter" || got.Credential.ID != input.CredentialID ||
					got.Credential.KeyID != "agent-2026" ||
					got.Credential.Ciphertext != base64.StdEncoding.EncodeToString([]byte{0, 1, 2, 3}) {
					t.Fatalf("unsafe agent input: %+v", got)
				}
				if tc.agentError != nil {
					return catalogflow.AgentTestOutput{}, tc.agentError
				}
				return catalogflow.AgentTestOutput{Result: domain.TestOK}, nil
			}, activity.RegisterOptions{Name: "provider.test_credential"})
			env.RegisterActivityWithOptions(func(_ context.Context, got catalogflow.RecordInput) error {
				sequence = append(sequence, "record")
				if got.Input != input || got.Result != domain.TestOK || got.TestedAt.IsZero() || got.Last4 != "1234" {
					t.Fatalf("wrong recorded result: %+v", got)
				}
				return nil
			}, activity.RegisterOptions{Name: "RecordCredentialTestResult"})
			env.ExecuteWorkflow(catalogflow.CredentialTestWorkflow, input)
			if tc.wantRecord {
				if err := env.GetWorkflowError(); err != nil {
					t.Fatalf("workflow failed: %v", err)
				}
				var output catalogflow.Output
				if err := env.GetWorkflowResult(&output); err != nil || output.CredentialID != input.CredentialID || output.Result != domain.TestOK {
					t.Fatalf("wrong workflow output %+v: %v", output, err)
				}
				if len(sequence) != 3 || sequence[0] != "load" || sequence[1] != "agent" || sequence[2] != "record" {
					t.Fatalf("wrong activity order: %v", sequence)
				}
			} else if env.GetWorkflowError() == nil || len(sequence) != 2 || sequence[0] != "load" || sequence[1] != "agent" {
				t.Fatalf("failed probe recorded result: %v, %v", env.GetWorkflowError(), sequence)
			}
		})
	}
}
