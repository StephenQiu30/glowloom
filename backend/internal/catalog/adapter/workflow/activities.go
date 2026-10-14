package workflow

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
)

// Activities adapts the catalog's authorized reads and audited writes to flow.
type Activities struct {
	service  *application.CredentialTestService
	failures CheckFailureRecorder
}

// NewActivities injects the credential test service.
func NewActivities(service *application.CredentialTestService, failures CheckFailureRecorder) *Activities {
	return &Activities{service: service, failures: failures}
}

// LoadCredentialForTest reads only a still-active credential for the requester.
func (a *Activities) LoadCredentialForTest(ctx context.Context, input Input) (LoadedCredential, error) {
	request := application.CredentialTestRequest{
		TestID: input.TestID, ProviderID: input.ProviderID, CredentialID: input.CredentialID,
		ActorID: input.ActorID, OrgID: input.OrgID, RequestID: input.RequestID,
	}
	provider, credential, err := a.service.Load(ctx, request)
	if err != nil {
		return LoadedCredential{}, err
	}
	return LoadedCredential{
		ProviderID: provider.ID, ProviderKey: provider.Key, AdapterKey: provider.AdapterKey,
		CredentialID: credential.ID, KeyID: credential.KeyID, Ciphertext: credential.Ciphertext,
		Last4: credential.Last4,
	}, nil
}

// RecordCredentialTestResult writes the category only if the tested row remains current.
func (a *Activities) RecordCredentialTestResult(ctx context.Context, input RecordInput) error {
	return a.service.Record(ctx, application.CredentialTestObservation{
		CredentialTestRequest: application.CredentialTestRequest{
			TestID: input.TestID, ProviderID: input.ProviderID, CredentialID: input.CredentialID,
			ActorID: input.ActorID, OrgID: input.OrgID, RequestID: input.RequestID,
		},
		Result: input.Result, TestedAt: input.TestedAt, Last4: input.Last4,
		AuthResult: input.Evidence.AuthResult, AdapterResult: input.Evidence.AdapterResult, DiscoveryResult: input.Evidence.DiscoveryResult, Candidates: input.Evidence.Candidates,
	})
}

// CheckFailureRecorder closes checks whose workflow cannot produce fresh evidence.
type CheckFailureRecorder interface {
	FailProviderCheck(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) error
}

// FailCredentialCheck records a safe terminal outcome even after access revocation.
func (a *Activities) FailCredentialCheck(ctx context.Context, input Input) error {
	return a.failures.FailProviderCheck(ctx, input.TestID, input.ActorID, input.ProviderID, input.CredentialID)
}
