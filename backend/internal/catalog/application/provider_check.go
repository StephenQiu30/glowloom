package application

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ModelCandidate is a discovered model, not proof of account access or execution.
type ModelCandidate struct {
	ProviderModelID string          `json:"provider_model_id"`
	DisplayName     string          `json:"display_name"`
	Capability      string          `json:"capability"`
	Metadata        json.RawMessage `json:"metadata" swaggertype:"object"`
}

// ProviderCheck separates authentication, protocol and discovery evidence.
type ProviderCheck struct {
	Current         bool       `json:"current"`
	RequestID       *uuid.UUID `json:"request_id,omitempty"`
	NetworkResult   string     `json:"network_result,omitempty"`
	HTTPStatus      int        `json:"http_status,omitempty"`
	ElapsedMS       int64      `json:"elapsed_ms"`
	Attempts        int        `json:"attempts,omitempty"`
	ParentCheckID   *uuid.UUID `json:"parent_check_id,omitempty"`
	TrackingEndedAt *time.Time `json:"tracking_ended_at,omitempty"`
	TrackingNote    string     `json:"tracking_note,omitempty"`

	ModelID         *uuid.UUID       `json:"model_id,omitempty"`
	ModelVersionID  *uuid.UUID       `json:"model_version_id,omitempty"`
	ModelResult     string           `json:"model_result,omitempty"`
	TaskID          string           `json:"task_id,omitempty"`
	ID              uuid.UUID        `json:"id"`
	ProviderID      uuid.UUID        `json:"provider_id"`
	ConfigVersionID uuid.UUID        `json:"config_version_id"`
	CredentialID    uuid.UUID        `json:"credential_id"`
	Kind            string           `json:"kind"`
	Status          string           `json:"status"`
	AuthResult      string           `json:"auth_result"`
	AdapterResult   string           `json:"adapter_result"`
	DiscoveryResult string           `json:"discovery_result"`
	ErrorCode       *string          `json:"error_code"`
	CreatedAt       time.Time        `json:"created_at"`
	CompletedAt     *time.Time       `json:"completed_at"`
	Candidates      []ModelCandidate `json:"candidates" gorm:"-"`
}

// ProviderCheckPage contains bounded authorized diagnostic history.
type ProviderCheckPage struct {
	Items         []ProviderCheck `json:"items"`
	CurrentChecks []ProviderCheck `json:"current_checks"`
	HasMore       bool            `json:"has_more"`
}

// ProviderTestModel names a saved model and the validation route it requires.
type ProviderTestModel struct {
	OnboardingModel
	RequiresTrial bool `json:"requires_trial"`
}

// ProviderCheckReader returns authorized nonsecret check progress.
type ProviderCheckReader interface {
	ListProviderTestModels(context.Context, identityapp.Principal, uuid.UUID) ([]ProviderTestModel, error)
	CredentialTestRequestStore
	ReadProviderCheck(context.Context, identityapp.Principal, uuid.UUID) (ProviderCheck, error)
	ListProviderChecks(context.Context, identityapp.Principal, uuid.UUID, int) (ProviderCheckPage, error)
	EndProviderCheckTracking(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, string) (ProviderCheck, error)
}

// ProviderChecks reads the durable check receipt.
type ProviderChecks struct{ store ProviderCheckReader }

// NewProviderChecks injects the authorized check reader.
func NewProviderChecks(store ProviderCheckReader) *ProviderChecks {
	return &ProviderChecks{store: store}
}

// Read always checks platform authorization before querying check results.
func (s *ProviderChecks) Read(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (ProviderCheck, error) {
	if !IsPlatformAdmin(actor) {
		return ProviderCheck{}, identityapp.ErrForbidden
	}
	return s.store.ReadProviderCheck(ctx, actor, id)
}

// List reads a fixed-size page after checking current administrator authority.
func (s *ProviderChecks) List(ctx context.Context, actor identityapp.Principal, id uuid.UUID, offset int) (ProviderCheckPage, error) {
	if !IsPlatformAdmin(actor) {
		return ProviderCheckPage{}, identityapp.ErrForbidden
	}
	if offset < 0 || offset > 20000 {
		return ProviderCheckPage{}, ErrInvalidCredentialTest
	}
	return s.store.ListProviderChecks(ctx, actor, id, offset)
}

// EndTracking preserves the unknown result and original accounting evidence.
func (s *ProviderChecks) EndTracking(ctx context.Context, actor identityapp.Principal, id, request uuid.UUID, note string) (ProviderCheck, error) {
	if !IsPlatformAdmin(actor) {
		return ProviderCheck{}, identityapp.ErrForbidden
	}
	if request == uuid.Nil || len(note) < 1 || len(note) > 1000 {
		return ProviderCheck{}, ErrInvalidCredentialTest
	}
	return s.store.EndProviderCheckTracking(ctx, actor, id, request, note)
}

// Network schedules a key-free probe through the same durable request pipeline.
func (s *ProviderChecks) Network(ctx context.Context, actor identityapp.Principal, id uuid.UUID, key string) (CredentialTestAccepted, error) {
	return NewRequestCredentialTestCommand(s.store).ExecuteReachability(ctx, actor, id, key)
}

// Query schedules only a read of the original asynchronous supplier task.
func (s *ProviderChecks) Query(ctx context.Context, actor identityapp.Principal, id uuid.UUID, key string) (CredentialTestAccepted, error) {
	check, err := s.Read(ctx, actor, id)
	if err != nil {
		return CredentialTestAccepted{}, err
	}
	requestID, err := uuid.Parse(key)
	if err != nil || requestID == uuid.Nil || requestID.String() != key || check.Kind != "model_access" || check.TaskID == "" || check.ModelID == nil || check.ModelResult == "completed" || check.ModelResult == "rejected" {
		return CredentialTestAccepted{}, ErrInvalidCredentialTest
	}
	request := CredentialTestRequest{Kind: "model_query", ParentCheckID: id, ModelID: *check.ModelID, TestID: uuid.NewSHA1(requestID, []byte(actor.ID.String()+actor.OrgID.String()+check.ProviderID.String()+check.CredentialID.String())), ProviderID: check.ProviderID, CredentialID: check.CredentialID, ActorID: actor.ID, OrgID: actor.OrgID, RequestID: key}
	return s.store.RequestCredentialTest(ctx, actor, request)
}

// Models returns only models owned by the chosen saved connection.
func (s *ProviderChecks) Models(ctx context.Context, actor identityapp.Principal, id uuid.UUID) ([]ProviderTestModel, error) {
	if !IsPlatformAdmin(actor) {
		return nil, identityapp.ErrForbidden
	}
	return s.store.ListProviderTestModels(ctx, actor, id)
}

// ErrProviderCheckCapacity prevents checks from bypassing supplier dispatch limits.
var ErrProviderCheckCapacity = errors.New("provider check capacity exhausted")

// ProviderCheckUsageReader returns shared active and recent supplier dispatches.
type ProviderCheckUsageReader interface {
	ProviderUsage(context.Context, uuid.UUID) (int64, int64, error)
}
