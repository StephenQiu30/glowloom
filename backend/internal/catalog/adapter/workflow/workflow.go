// Package workflow adapts catalog credential checks to Temporal activities.
package workflow

import (
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
)

// ErrInvalidTestWorkflow means the durable request or an activity result is unsafe.
var ErrInvalidTestWorkflow = errors.New("invalid credential test workflow")

// Input identifies the credential and administrator who requested its test.
type Input struct {
	TestID       uuid.UUID `json:"test_id"`
	ProviderID   uuid.UUID `json:"provider_id"`
	CredentialID uuid.UUID `json:"credential_id"`
	ActorID      uuid.UUID `json:"actor_id"`
	OrgID        uuid.UUID `json:"org_id"`
	RequestID    string    `json:"request_id"`
}

// LoadedCredential contains encrypted bytes read by the Go database activity.
type LoadedCredential struct {
	ProviderID   uuid.UUID `json:"provider_id"`
	ProviderKey  string    `json:"provider_key"`
	AdapterKey   string    `json:"adapter_key"`
	CredentialID uuid.UUID `json:"credential_id"`
	KeyID        string    `json:"key_id"`
	Ciphertext   []byte    `json:"ciphertext"`
	Last4        string    `json:"last4"`
}

// AgentTestInput is the reviewed JSON contract for provider.test_credential.
type AgentTestInput struct {
	ProviderID  uuid.UUID       `json:"provider_id"`
	ProviderKey string          `json:"provider_key"`
	AdapterKey  string          `json:"adapter_key"`
	Credential  AgentCredential `json:"credential"`
}

// AgentCredential carries only the encrypted envelope and its key identity.
type AgentCredential struct {
	ID         uuid.UUID `json:"id"`
	KeyID      string    `json:"key_id"`
	Ciphertext string    `json:"ciphertext"`
}

// AgentTestOutput is the safe result category returned by the Agent.
type AgentTestOutput struct {
	Result domain.TestResult `json:"result"`
}

// RecordInput commits a completed test against its original credential.
type RecordInput struct {
	Input
	Result   domain.TestResult `json:"result"`
	TestedAt time.Time         `json:"tested_at"`
	Last4    string            `json:"last4"`
}

// Output is the safe workflow result; it contains no key identity or ciphertext.
type Output struct {
	CredentialID uuid.UUID         `json:"credential_id"`
	Result       domain.TestResult `json:"result"`
	TestedAt     time.Time         `json:"tested_at"`
}

// CredentialTestWorkflow loads the current sealed credential, probes it once,
// then atomically records a safe result and its events.
func CredentialTestWorkflow(ctx workflow.Context, input Input) (Output, error) {
	requestID, err := uuid.Parse(input.RequestID)
	if input.TestID == uuid.Nil || input.ProviderID == uuid.Nil || input.CredentialID == uuid.Nil ||
		input.ActorID == uuid.Nil || input.OrgID == uuid.Nil || err != nil || requestID.String() != input.RequestID {
		return Output{}, ErrInvalidTestWorkflow
	}
	flowOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval: time.Second,
			MaximumAttempts: 3,
		},
	}
	flowCtx := workflow.WithActivityOptions(ctx, flowOptions)
	var loaded LoadedCredential
	if err := workflow.ExecuteActivity(flowCtx, "LoadCredentialForTest", input).Get(flowCtx, &loaded); err != nil {
		return Output{}, err
	}
	if loaded.ProviderID != input.ProviderID || loaded.CredentialID != input.CredentialID ||
		loaded.ProviderKey == "" || loaded.AdapterKey == "" || loaded.KeyID == "" ||
		len(loaded.Ciphertext) == 0 || len(loaded.Last4) != 4 {
		return Output{}, ErrInvalidTestWorkflow
	}
	agentInput := AgentTestInput{
		ProviderID: loaded.ProviderID, ProviderKey: loaded.ProviderKey, AdapterKey: loaded.AdapterKey,
		Credential: AgentCredential{
			ID: loaded.CredentialID, KeyID: loaded.KeyID,
			Ciphertext: base64.StdEncoding.EncodeToString(loaded.Ciphertext),
		},
	}
	agentCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		TaskQueue: "agent", StartToCloseTimeout: 15 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 1},
	})
	var tested AgentTestOutput
	if err := workflow.ExecuteActivity(agentCtx, "provider.test_credential", agentInput).Get(agentCtx, &tested); err != nil {
		return Output{}, err
	}
	if !validResult(tested.Result) {
		return Output{}, ErrInvalidTestWorkflow
	}
	record := RecordInput{
		Input: input, Result: tested.Result, TestedAt: workflow.Now(ctx).UTC(), Last4: loaded.Last4,
	}
	if err := workflow.ExecuteActivity(flowCtx, "RecordCredentialTestResult", record).Get(flowCtx, nil); err != nil {
		return Output{}, err
	}
	return Output{CredentialID: input.CredentialID, Result: tested.Result, TestedAt: record.TestedAt}, nil
}

func validResult(result domain.TestResult) bool {
	switch result {
	case domain.TestOK, domain.TestAuthFailed, domain.TestUnreachable, domain.TestTimeout, domain.TestUnsupported:
		return true
	default:
		return false
	}
}

// Register installs the credential test workflow and Go database activities.
func Register(w worker.Worker, activities *Activities) {
	w.RegisterWorkflow(CredentialTestWorkflow)
	w.RegisterActivity(activities)
}
