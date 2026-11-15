package microservices_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/managed"
	executionpg "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"
	executionapp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/execution/operation/domain"
	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/providerartifact"
)

type observedActivityStore struct {
	*executionpg.Store
	loaded  executionapp.WorkflowOperation
	config  json.RawMessage
	claimed bool
}

func (s *observedActivityStore) LoadWorkflowOperation(context.Context, uuid.UUID) (executionapp.WorkflowOperation, error) {
	return s.loaded, nil
}
func (s *observedActivityStore) ClaimProviderDispatch(context.Context, executionapp.ProviderDispatchIdentity) (executionapp.ProviderDispatchResult, error) {
	claimed := !s.claimed
	s.claimed = true
	return executionapp.ProviderDispatchResult{Claimed: claimed}, nil
}
func (s *observedActivityStore) LoadManagedCredential(context.Context, uuid.UUID) (executionapp.ManagedCredential, error) {
	return executionapp.ManagedCredential{ProviderID: uuid.New(), CredentialID: uuid.New(), Config: s.config, KeyID: "synthetic"}, nil
}

type noObservationStore struct{ managed.Store }

type observedActivityKeys struct{}

func (observedActivityKeys) Open(string, uuid.UUID, uuid.UUID, []byte) ([]byte, error) {
	return []byte(`{"api_key":"synthetic-not-real"}`), nil
}

type observedActivityProvider struct {
	calls  int
	result *executionapp.GenerationResult
}

func (p *observedActivityProvider) Submit(context.Context, executionapp.GenerationRequest, string) (executionapp.GenerationResult, error) {
	p.calls++
	if p.result != nil {
		return *p.result, nil
	}
	return executionapp.GenerationResult{Outcome: executionapp.ProviderOutcomeCompleted, State: executionapp.ProviderTaskSucceeded, Images: []executionapp.GeneratedImage{{Bytes: []byte("original provider bytes"), MediaType: "image/png"}}, Usage: json.RawMessage(`{"billable_requests":0,"original":9007199254740993}`)}, nil
}
func (*observedActivityProvider) Query(context.Context, catalogdomain.ProviderConfig, string, string) (executionapp.GenerationResult, error) {
	return executionapp.GenerationResult{}, errors.New("unexpected query")
}
func (*observedActivityProvider) Cancel(context.Context, catalogdomain.ProviderConfig, string, string) (executionapp.GenerationResult, error) {
	return executionapp.GenerationResult{}, errors.New("unexpected cancel")
}

type failObservedArtifact struct {
	executionapp.ArtifactDeliveryRepository
	check func() error
}

func (f *failObservedArtifact) PrepareArtifact(context.Context, executionapp.ProviderDispatchIdentity, executionapp.ManagedArtifact) (executionapp.ArtifactDeliveryRecord, error) {
	if err := f.check(); err != nil {
		return executionapp.ArtifactDeliveryRecord{}, err
	}
	return executionapp.ArtifactDeliveryRecord{}, errors.New("synthetic Media stage failure")
}

func (*failObservedArtifact) ArtifactDeliveries(context.Context, executionapp.ProviderDispatchIdentity) ([]executionapp.ArtifactDeliveryRecord, error) {
	return nil, nil
}

type unusedArtifactMedia struct{}

func (unusedArtifactMedia) StageArtifact(context.Context, contract.Request) (contract.Receipt, error) {
	return contract.Receipt{}, errors.New("unexpected Media call")
}
func (unusedArtifactMedia) ResolveArtifact(context.Context, contract.Request) (contract.Receipt, error) {
	return contract.Receipt{}, errors.New("unexpected Media resolve")
}
func (unusedArtifactMedia) OpenArtifact(context.Context, contract.Request) (io.ReadCloser, error) {
	return nil, errors.New("unexpected Media read")
}

func TestSupplierObservationOriginalAcceptedTaskRecoversWithoutManifest(t *testing.T) {
	f, o := supplierObservationFixture(t)
	if err := f.admin.Exec(`DELETE FROM operation.provider_result WHERE operation_id=?`, o.Identity.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	o.Outcome = executionapp.ProviderOutcomeAccepted
	o.State = executionapp.ProviderTaskPending
	o.TaskID = "original accepted task"
	o.ImageCount = 0
	o.SupplierUsage = nil
	store := executionpg.NewStore(f.execution)
	if err := store.RecordSupplierObservation(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	results, err := executionapp.NewOwnedManagedResults(store, executionapp.NewArtifactDelivery(&failObservedArtifact{check: func() error { return errors.New("unexpected stage") }}, unusedArtifactMedia{}))
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := results.Recover(t.Context(), o.Identity)
	if err != nil || recovered == nil || recovered.State != executionapp.ManagedResultAccepted || recovered.TaskID != o.TaskID || recovered.Identity != o.Identity {
		t.Fatalf("original task recovery failed: %+v err=%v", recovered, err)
	}
}

func TestSupplierObservationActualActivityCommitsCostBeforeStageAndNeverResubmits(t *testing.T) {
	f, o := supplierObservationFixture(t)
	if err := f.admin.Exec(`DELETE FROM operation.provider_result WHERE operation_id=?`, o.Identity.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	spec, ok := catalogdomain.GetAdapterSpec("volcengine_ark")
	if !ok {
		t.Fatal("missing implemented OpenAI protocol")
	}
	config := catalogdomain.ProviderConfig{AdapterKey: "volcengine_ark", BaseURL: spec.AllowedBaseURL, Region: spec.AllowedRegion, Capabilities: []string{"image"}, TimeoutSeconds: 5, ConcurrencyLimit: 1, RateLimitPerMin: 1}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(config)
	store := &observedActivityStore{Store: executionpg.NewStore(f.execution), config: raw, loaded: executionapp.WorkflowOperation{ProviderRequestKey: o.Identity.RequestKey, Provider: executionapp.WorkflowProvider{AdapterKey: "volcengine_ark"}, Operation: operationdomain.Operation{ID: o.Identity.OperationID, ProjectID: o.Identity.ProjectID, ModelProfileVersionID: &o.Identity.ModelProfileVersionID, PriceRuleVersionID: &o.Identity.PriceRuleVersionID, Status: operationdomain.StatusSubmitting, Capability: "image.generate", Mode: "text_to_image", OutputCount: 1, Params: json.RawMessage(`{}`)}}}
	reached := false
	artifacts := &failObservedArtifact{check: func() error {
		var cost *int64
		var observation []byte
		if err := f.execution.Raw(`SELECT cost_micros,response_summary->'supplier_observation' FROM operation.provider_call WHERE operation_id=? AND action='submit'`, o.Identity.OperationID).Row().Scan(&cost, &observation); err != nil {
			return err
		}
		if cost == nil || *cost != 1234 || len(observation) == 0 {
			return errors.New("stage began before original liability committed")
		}
		reached = true
		return nil
	}}
	results, err := executionapp.NewOwnedManagedResults(store, executionapp.NewArtifactDelivery(artifacts, unusedArtifactMedia{}))
	if err != nil {
		t.Fatal(err)
	}
	provider := &observedActivityProvider{}
	missingPort := managed.NewActivities(noObservationStore{Store: store}, observedActivityKeys{}, provider, results, nil)
	if _, err := missingPort.Step(t.Context(), executionapp.ManagedStep{OperationID: o.Identity.OperationID, Action: "submit"}); !errors.Is(err, executionapp.ErrInvalidProviderCall) {
		t.Fatalf("missing mandatory observation port accepted: %v", err)
	}
	if provider.calls != 0 || store.claimed {
		t.Fatal("missing observation dependency discovered after dispatch")
	}
	activity := managed.NewActivities(store, observedActivityKeys{}, provider, results, nil)
	if _, err := activity.Step(t.Context(), executionapp.ManagedStep{OperationID: o.Identity.OperationID, Action: "submit"}); err == nil || err.Error() != "synthetic Media stage failure" {
		t.Fatalf("unexpected stage result: %v", err)
	}
	if !reached {
		t.Fatal("actual activity never reached stage")
	}
	if _, err := activity.Step(t.Context(), executionapp.ManagedStep{OperationID: o.Identity.OperationID, Action: "submit"}); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 {
		t.Fatalf("unknown stage reissued paid generation %d times", provider.calls)
	}
	recovered, err := store.ReadSupplierObservation(t.Context(), o.Identity)
	if err != nil || recovered == nil || recovered.ReceivedAt.IsZero() {
		t.Fatalf("original observation missing: %+v %v", recovered, err)
	}
}
