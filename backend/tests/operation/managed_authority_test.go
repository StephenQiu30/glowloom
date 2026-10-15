package operation_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/managed"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

type scopedIngestStore struct {
	managed.Store
	revoked      bool
	loads, texts int
}

func (s *scopedIngestStore) LoadWorkflowOperation(context.Context, uuid.UUID) (application.WorkflowOperation, error) {
	s.loads++
	if s.revoked {
		return application.WorkflowOperation{}, application.ErrExecutionAuthorityRevoked
	}
	model, price := uuid.New(), uuid.New()
	return application.WorkflowOperation{Operation: domain.Operation{Status: domain.StatusIngesting, ModelProfileVersionID: &model, PriceRuleVersionID: &price}, Provider: application.WorkflowProvider{Moderation: "provider"}}, nil
}
func (s *scopedIngestStore) ReadManagedResult(context.Context, application.ProviderDispatchIdentity) (*application.ManagedResult, error) {
	return &application.ManagedResult{State: "succeeded", Text: "rollback-free text fixture"}, nil
}
func (s *scopedIngestStore) IngestManagedText(context.Context, application.ProviderDispatchIdentity) error {
	s.texts++
	return nil
}
func (s *scopedIngestStore) AuthorizeWorker(context.Context, uuid.UUID, bool) error {
	if s.revoked {
		return application.ErrExecutionAuthorityRevoked
	}
	return nil
}
func TestManagedActualConsumersUseInjectedAuthority(t *testing.T) {
	store := &scopedIngestStore{}
	ingest := managed.NewScopedIngestActivities(nil, nil, nil, nil, store)
	if err := ingest.Ingest(t.Context(), uuid.New()); err != nil || store.loads != 1 || store.texts != 1 {
		t.Fatalf("injected ingest store not consumed: %v %+v", err, store)
	}
	store.revoked = true
	for _, run := range []func() error{func() error { return ingest.Ingest(t.Context(), uuid.New()) }, func() error {
		_, err := managed.NewActivities(store, nil, nil, nil, nil, nil).Step(t.Context(), application.ManagedStep{OperationID: uuid.New(), Action: "submit"})
		return err
	}} {
		err := run()
		var appErr *temporal.ApplicationError
		if !errors.As(err, &appErr) || !appErr.NonRetryable() || appErr.Type() != "execution_authority_revoked" {
			t.Fatalf("revoked actual consumer retried or dispatched: %v", err)
		}
	}
	if store.texts != 1 {
		t.Fatal("revoked consumer published text")
	}
}
