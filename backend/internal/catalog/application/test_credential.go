package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// ErrInvalidCredentialTest means a test request or observation is incomplete.
var ErrInvalidCredentialTest = errors.New("invalid credential test")

// CredentialTestRequest binds one authorized test to its original credential.
type CredentialTestRequest struct {
	TestID       uuid.UUID
	ProviderID   uuid.UUID
	CredentialID uuid.UUID
	ActorID      uuid.UUID
	OrgID        uuid.UUID
	RequestID    string
}

// CredentialTestObservation is the Agent's safe result, with no provider body.
type CredentialTestObservation struct {
	CredentialTestRequest
	Result   domain.TestResult
	TestedAt time.Time
	Last4    string
}

// CredentialTestStore checks live administrator rights and owns test writes.
type CredentialTestStore interface {
	LoadForCredentialTest(context.Context, CredentialTestRequest) (domain.Provider, domain.Credential, error)
	RecordCredentialTest(context.Context, CredentialTestObservation, []identityapp.OutboxEvent) error
}

// CredentialTestService enforces the encrypted read and safe audited write.
type CredentialTestService struct {
	store CredentialTestStore
}

// NewCredentialTestService injects the catalog store used by flow activities.
func NewCredentialTestService(store CredentialTestStore) *CredentialTestService {
	return &CredentialTestService{store: store}
}

// Load rejects stale and unauthorized credentials before they reach the Agent.
func (s *CredentialTestService) Load(ctx context.Context, request CredentialTestRequest) (domain.Provider, domain.Credential, error) {
	if s == nil || s.store == nil || !validTestRequest(request) {
		return domain.Provider{}, domain.Credential{}, ErrInvalidCredentialTest
	}
	provider, credential, err := s.store.LoadForCredentialTest(ctx, request)
	if err != nil {
		return domain.Provider{}, domain.Credential{}, fmt.Errorf("load credential for test: %w", err)
	}
	if provider.ID != request.ProviderID || provider.Status != domain.ProviderActive ||
		credential.ID != request.CredentialID || credential.ProviderID != provider.ID ||
		credential.Status != domain.CredentialActive || credential.Validate() != nil {
		return domain.Provider{}, domain.Credential{}, ErrInvalidCredentialTest
	}
	return provider, credential, nil
}

// Record commits the category and safe events against the original credential.
func (s *CredentialTestService) Record(ctx context.Context, observation CredentialTestObservation) error {
	if s == nil || s.store == nil || !validTestRequest(observation.CredentialTestRequest) ||
		!utf8.ValidString(observation.Last4) || utf8.RuneCountInString(observation.Last4) != 4 {
		return ErrInvalidCredentialTest
	}
	probe := domain.Credential{Status: domain.CredentialActive}
	if err := probe.RecordTest(observation.Result, observation.TestedAt); err != nil {
		return err
	}
	events, err := credentialTestEvents(observation)
	if err != nil {
		return fmt.Errorf("build credential test events: %w", err)
	}
	if err := s.store.RecordCredentialTest(ctx, observation, events); err != nil {
		return fmt.Errorf("record credential test: %w", err)
	}
	return nil
}

func validTestRequest(request CredentialTestRequest) bool {
	requestID, err := uuid.Parse(request.RequestID)
	return request.TestID != uuid.Nil && request.ProviderID != uuid.Nil && request.CredentialID != uuid.Nil &&
		request.ActorID != uuid.Nil && request.OrgID != uuid.Nil && err == nil && requestID.String() == request.RequestID
}

func credentialTestEvents(observation CredentialTestObservation) ([]identityapp.OutboxEvent, error) {
	changedID := uuid.NewSHA1(observation.TestID, []byte("catalog.credential_changed.v1"))
	auditID := uuid.NewSHA1(observation.TestID, []byte("audit.recorded.v1"))
	changed, err := json.Marshal(map[string]any{
		"event_id": changedID, "event_type": credentialChangedTopic,
		"occurred_at": observation.TestedAt.UTC(), "org_id": observation.OrgID,
		"actor":     map[string]any{"kind": "user", "id": observation.ActorID},
		"aggregate": map[string]any{"type": "provider_credential", "id": observation.CredentialID},
		"data": map[string]any{
			"change": "tested", "provider_id": observation.ProviderID, "result": observation.Result,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode credential test change: %w", err)
	}
	audit, err := json.Marshal(map[string]any{
		"event_id": auditID, "event_type": auditTopic,
		"occurred_at": observation.TestedAt.UTC(), "org_id": observation.OrgID,
		"actor":     map[string]any{"kind": "user", "id": observation.ActorID},
		"aggregate": map[string]any{"type": "audit", "id": auditID},
		"data": map[string]any{
			"action":     "credential.tested",
			"object":     map[string]any{"type": "provider_credential", "id": observation.CredentialID},
			"after":      map[string]any{"last4": observation.Last4},
			"request_id": observation.RequestID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode credential test audit: %w", err)
	}
	key := observation.OrgID.String()
	return []identityapp.OutboxEvent{
		{ID: changedID, Topic: credentialChangedTopic, PartitionKey: key, Payload: changed},
		{ID: auditID, Topic: auditTopic, PartitionKey: key, Payload: audit},
	}, nil
}
