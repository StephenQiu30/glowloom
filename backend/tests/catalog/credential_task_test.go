package catalog_test

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"encoding/json"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	catalogflow "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func TestCredentialTaskSealsAgentBoundaryAndRecordsOnlySuccess(t *testing.T) {
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
			rec := &credentialSteps{}
			rec.load = func(_ context.Context, got catalogflow.Input) (catalogflow.LoadedCredential, error) {
				sequence = append(sequence, "load")
				if got != input {
					t.Fatalf("wrong load request: %+v", got)
				}
				return catalogflow.LoadedCredential{
					ProviderID: input.ProviderID, ProviderKey: "openrouter", AdapterKey: "openrouter",
					CredentialID: input.CredentialID, KeyID: "agent-2026", Ciphertext: []byte{0, 1, 2, 3}, Last4: "1234",
				}, nil
			}
			rec.probe = func(_ context.Context, got catalogflow.AgentTestInput) (catalogflow.AgentTestOutput, error) {
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
			}
			rec.record = func(_ context.Context, got catalogflow.RecordInput) error {
				sequence = append(sequence, "record")
				if got.Input != input || got.Result != domain.TestOK || got.TestedAt.IsZero() || got.Last4 != "1234" {
					t.Fatalf("wrong recorded result: %+v", got)
				}
				return nil
			}
			rec.fail = func(_ context.Context, got catalogflow.Input) error {
				if got != input {
					t.Fatal("wrong failed check identity")
				}
				sequence = append(sequence, "failed")
				return nil
			}
			raw, _ := json.Marshal(input)
			_, err := catalogflow.CredentialHandler(rec, rec)(context.Background(), taskruntime.Job{Kind: catalogflow.Credential, Reference: raw, BusinessID: input.TestID.String()})
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantRecord {
				if len(sequence) != 3 || sequence[0] != "load" || sequence[1] != "agent" || sequence[2] != "record" {
					t.Fatalf("wrong activity order: %v", sequence)
				}
			} else if len(sequence) != 3 || sequence[0] != "load" || sequence[1] != "agent" || sequence[2] != "failed" {
				t.Fatalf("failed probe recorded result: %v, %v", err, sequence)
			}
		})
	}
}

type credentialSteps struct {
	load   func(context.Context, catalogflow.Input) (catalogflow.LoadedCredential, error)
	probe  func(context.Context, catalogflow.AgentTestInput) (catalogflow.AgentTestOutput, error)
	record func(context.Context, catalogflow.RecordInput) error
	fail   func(context.Context, catalogflow.Input) error
}

func (s credentialSteps) LoadCredentialForTest(ctx context.Context, i catalogflow.Input) (catalogflow.LoadedCredential, error) {
	return s.load(ctx, i)
}
func (s credentialSteps) TestCredential(ctx context.Context, i catalogflow.AgentTestInput) (catalogflow.AgentTestOutput, error) {
	return s.probe(ctx, i)
}
func (s credentialSteps) TestReachability(context.Context, catalogflow.Input) error {
	return errors.New("unexpected reachability probe")
}
func (s credentialSteps) RecordCredentialTestResult(ctx context.Context, i catalogflow.RecordInput) error {
	return s.record(ctx, i)
}
func (s credentialSteps) FailCredentialCheck(ctx context.Context, i catalogflow.Input) error {
	return s.fail(ctx, i)
}
