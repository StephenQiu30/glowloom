package operation_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/managed"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/domain"
)

type scopedIngestStore struct {
	managed.Store
	revoked      bool
	loads, texts int
	result       *application.ManagedResult
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
	if s.result != nil {
		return s.result, nil
	}
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

type artifactIngestFunc func(context.Context, application.ProviderDispatchIdentity, application.ManagedResult, *application.WorkflowControlDelivery) error

func (f artifactIngestFunc) IngestArtifacts(ctx context.Context, identity application.ProviderDispatchIdentity, result application.ManagedResult, original *application.WorkflowControlDelivery) error {
	return f(ctx, identity, result, original)
}

func TestManagedBinaryIngestRequiresOwnerAndPreservesFailureAuthority(t *testing.T) {
	for _, mode := range []string{"missing", "owner failure", "revoked during owner failure"} {
		t.Run(mode, func(t *testing.T) {
			id := uuid.New()
			original := application.ManagedResult{State: application.ManagedResultSucceeded, Artifacts: []application.ManagedArtifact{{Sequence: 1, Size: 3, MIME: "image/png", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}
			store := &scopedIngestStore{result: &original}
			failure := errors.New("synthetic Media unavailable")
			var owner managed.ArtifactIngest
			calls := 0
			wanted := failure
			if mode == "missing" {
				wanted = writefence.ErrUnavailable
			} else {
				owner = artifactIngestFunc(func(_ context.Context, identity application.ProviderDispatchIdentity, result application.ManagedResult, delivery *application.WorkflowControlDelivery) error {
					calls++
					if identity.OperationID != id || !reflect.DeepEqual(result, original) || delivery != nil {
						t.Fatal("original binary evidence changed at owner boundary")
					}
					if mode == "revoked during owner failure" {
						store.revoked = true
					}
					return failure
				})
				if mode == "revoked during owner failure" {
					wanted = application.ErrExecutionAuthorityRevoked
				}
			}
			err := managed.NewIngestActivities(store, nil, owner).Ingest(t.Context(), id)
			if !errors.Is(err, wanted) || store.texts != 0 || store.loads != 1 || (mode == "missing" && calls != 0) || (mode != "missing" && calls != 1) {
				t.Fatalf("binary owner bypass: mode=%s calls=%d store=%+v err=%v", mode, calls, store, err)
			}
		})
	}
}

func TestManagedActualConsumersUseInjectedAuthority(t *testing.T) {
	store := &scopedIngestStore{}
	ingest := operationLegacyIngest(nil, nil, nil, nil, store)
	if err := ingest.Ingest(t.Context(), uuid.New()); err != nil || store.loads != 1 || store.texts != 1 {
		t.Fatalf("injected ingest store not consumed: %v %+v", err, store)
	}
	store.revoked = true
	for _, run := range []func() error{func() error { return ingest.Ingest(t.Context(), uuid.New()) }, func() error {
		_, err := managed.NewActivities(store, nil, nil, nil, nil).Step(t.Context(), application.ManagedStep{OperationID: uuid.New(), Action: "submit"})
		return err
	}} {
		err := run()
		if !errors.Is(err, application.ErrExecutionAuthorityRevoked) {
			t.Fatalf("revoked actual consumer retried or dispatched: %v", err)
		}
	}
	if store.texts != 1 {
		t.Fatal("revoked consumer published text")
	}
}
