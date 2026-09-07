package creation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
	"github.com/google/uuid"
)

type deliveryStore struct {
	delivery     domain.Delivery
	status, code string
	receipt      *domain.Acceptance
	authorized   bool
}

func (s *deliveryStore) Claim(context.Context, time.Time, time.Duration) (domain.Delivery, error) {
	return s.delivery, nil
}
func (s *deliveryStore) AuthorizeDelivery(context.Context, domain.Run) error {
	if !s.authorized {
		return app.Problem("forbidden", 403)
	}
	return nil
}
func (s *deliveryStore) Complete(_ context.Context, _ domain.Delivery, status, code string, receipt *domain.Acceptance, _ time.Time, _ time.Time) error {
	s.status, s.code, s.receipt = status, code, receipt
	return nil
}

type deliveryPeer struct {
	receipt domain.Acceptance
	found   bool
	err     error
	posts   int
}

func (p *deliveryPeer) Lookup(context.Context, domain.Run) (domain.Acceptance, error) {
	if p.found {
		return p.receipt, nil
	}
	if p.err != nil {
		return domain.Acceptance{}, p.err
	}
	return domain.Acceptance{}, app.ErrNotFound
}
func (p *deliveryPeer) Accept(context.Context, domain.Run) (domain.Acceptance, error) {
	p.posts++
	p.found = true
	return domain.Acceptance{}, errors.New("response lost after durable acceptance")
}
func deliveryFixture() (*deliveryStore, *deliveryPeer) {
	id := uuid.NewString()
	run := domain.Run{Command: domain.Command{CommandID: id, RunID: id, FlowType: domain.FlowType, WorkflowID: "lanverse:creation:" + id}, PayloadHash: "fixed-hash"}
	receipt := domain.Acceptance{Schema: "creation-acceptance-production", CommandID: id, RunID: id, PayloadHash: run.PayloadHash, FlowType: domain.FlowType, WorkflowID: run.Command.WorkflowID, ReceiptID: uuid.NewString(), AcceptedAt: time.Now().UTC()}
	return &deliveryStore{delivery: domain.Delivery{Run: run, Fence: 1, Attempts: 1}, authorized: true}, &deliveryPeer{receipt: receipt}
}
func TestCreationDeliveryRecoversLostResponseWithoutAnotherSubmission(t *testing.T) {
	store, peer := deliveryFixture()
	worker := app.NewDispatcher(store, peer, time.Now)
	if err := worker.DispatchOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.status != domain.Unknown || peer.posts != 1 {
		t.Fatalf("unknown delivery status=%s posts=%d", store.status, peer.posts)
	}
	store.authorized = false // Reconciliation of already accepted work survives revocation.
	if err := worker.DispatchOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.status != domain.Accepted || peer.posts != 1 || store.receipt.ReceiptID != peer.receipt.ReceiptID {
		t.Fatal("lost response created a new run")
	}
}
func TestCreationDeliveryNeverSubmitsAfterRevocationOrUnknownLookup(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		store, peer := deliveryFixture()
		store.authorized = false
		if unknown {
			peer.err = errors.New("lookup timeout")
		}
		if err := app.NewDispatcher(store, peer, time.Now).DispatchOne(context.Background()); err != nil {
			t.Fatal(err)
		}
		if peer.posts != 0 {
			t.Fatal("submitted without known absence and current permission")
		}
		if !unknown && store.status != domain.Blocked {
			t.Fatal("revocation did not block submission")
		}
	}
}
func TestCreationDeliveryRejectsForeignReceipt(t *testing.T) {
	store, peer := deliveryFixture()
	peer.found = true
	peer.receipt.RunID = uuid.NewString()
	if err := app.NewDispatcher(store, peer, time.Now).DispatchOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.status == domain.Accepted || store.receipt != nil {
		t.Fatal("accepted receipt from another run")
	}
}

func TestCreationRejectsNonCanonicalIdentityBeforePersistence(t *testing.T) {
	service := app.NewService(nil, app.Config{})
	_, err := service.Create(context.Background(), app.Actor{UserID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", TokenVersion: 1}, app.CreateCommand{ProjectID: "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA", DocumentRevisionID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", SourceHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", IdempotencyKey: "test"})
	var problem *app.Error
	if !errors.As(err, &problem) || problem.Status != 422 {
		t.Fatalf("noncanonical identity reached persistence: %v", err)
	}
}

func TestCreationDeliveryStoresActionableSafeErrorCodes(t *testing.T) {
	for status, code := range map[int]string{401: "agent_authorization_rejected", 404: "agent_route_unavailable", 409: "agent_command_conflict", 422: "agent_command_invalid"} {
		store, peer := deliveryFixture()
		peer.err = app.Problem("upstream-private-detail", status)
		if err := app.NewDispatcher(store, peer, time.Now).DispatchOne(context.Background()); err != nil {
			t.Fatal(err)
		}
		if store.status != domain.Blocked || store.code != code || peer.posts != 0 {
			t.Fatalf("unsafe failure classification: %d %s", status, store.code)
		}
	}
}
