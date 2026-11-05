// Package task executes accepted provider checks.
package task

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

// ErrInvalidCheck means the durable request or an activity result is unsafe.
var ErrInvalidCheck = errors.New("invalid credential check")

// Input identifies the credential and administrator who requested its test.
type Input struct {
	Kind         string    `json:"kind,omitempty"`
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
	OrgID       uuid.UUID       `json:"org_id"`
	RequestID   string          `json:"request_id"`
	TestID      uuid.UUID       `json:"test_id,omitempty"`
	ActorID     uuid.UUID       `json:"actor_id,omitempty"`
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
	HTTPStatus      int                          `json:"http_status,omitempty"`
	ElapsedMS       int64                        `json:"elapsed_ms,omitempty"`
	ModelResult     string                       `json:"model_result,omitempty"`
	TaskID          string                       `json:"task_id,omitempty"`
	Result          domain.TestResult            `json:"result"`
	AuthResult      string                       `json:"auth_result,omitempty"`
	AdapterResult   string                       `json:"adapter_result,omitempty"`
	DiscoveryResult string                       `json:"discovery_result,omitempty"`
	Candidates      []application.ModelCandidate `json:"candidates,omitempty"`
}

// RecordInput commits a completed test against its original credential.
type RecordInput struct {
	Evidence AgentTestOutput
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

func validResult(result domain.TestResult) bool {
	switch result {
	case domain.TestOK, domain.TestAuthFailed, domain.TestUnreachable, domain.TestTimeout, domain.TestUnsupported:
		return true
	default:
		return false
	}
}

// Credential identifies the provider credential-check task kind.
const Credential = "catalog.check"

// CredentialSteps owns durable credential-check execution and settlement.
type CredentialSteps interface {
	LoadCredentialForTest(context.Context, Input) (LoadedCredential, error)
	RecordCredentialTestResult(context.Context, RecordInput) error
	FailCredentialCheck(context.Context, Input) error
}

// Probe performs the configured provider credential check.
type Probe interface {
	TestCredential(context.Context, AgentTestInput) (AgentTestOutput, error)
	TestReachability(context.Context, Input) error
}

// CredentialHandler executes admitted credential checks through their owning steps.
func CredentialHandler(steps CredentialSteps, probe Probe) taskruntime.Handler {
	return taskruntime.WithReference(6*time.Minute, func(ctx context.Context, j taskruntime.Job, input Input) error {
		id, err := uuid.Parse(input.RequestID)
		if err != nil || input.TestID.String() != j.BusinessID || input.TestID == uuid.Nil || input.ActorID == uuid.Nil || input.OrgID == uuid.Nil || input.ProviderID == uuid.Nil || id.String() != input.RequestID {
			return ErrInvalidCheck
		}
		runErr := runCredential(ctx, steps, probe, input)
		if runErr != nil && ctx.Err() == nil {
			// A probe may have reached the provider. Record uncertainty against the
			// same check. No subsequent claim may create a second dispatch.
			return steps.FailCredentialCheck(ctx, input)
		}
		return runErr
	})
}
func runCredential(ctx context.Context, steps CredentialSteps, probe Probe, input Input) error {
	if input.CredentialID == uuid.Nil {
		return probe.TestReachability(ctx, input)
	}
	loaded, err := steps.LoadCredentialForTest(ctx, input)
	if err != nil {
		return err
	}
	if loaded.ProviderID != input.ProviderID || loaded.CredentialID != input.CredentialID || loaded.ProviderKey == "" || loaded.AdapterKey == "" || loaded.KeyID == "" || len(loaded.Ciphertext) == 0 || len(loaded.Last4) != 4 {
		return ErrInvalidCheck
	}
	tested, err := probe.TestCredential(ctx, AgentTestInput{TestID: input.TestID, ActorID: input.ActorID, OrgID: input.OrgID, RequestID: input.RequestID, ProviderID: loaded.ProviderID, ProviderKey: loaded.ProviderKey, AdapterKey: loaded.AdapterKey, Credential: AgentCredential{ID: loaded.CredentialID, KeyID: loaded.KeyID, Ciphertext: base64.StdEncoding.EncodeToString(loaded.Ciphertext)}})
	if err != nil {
		return err
	}
	if !validResult(tested.Result) {
		return ErrInvalidCheck
	}
	return steps.RecordCredentialTestResult(ctx, RecordInput{Input: input, Result: tested.Result, TestedAt: time.Now().UTC(), Last4: loaded.Last4, Evidence: tested})
}
