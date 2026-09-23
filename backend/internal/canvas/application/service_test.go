package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

const (
	projectID = "019fb2d0-a000-7000-8000-000000000001"
	userID    = "019fb2d0-a000-7000-8000-000000000002"
	nodeID    = "019fb2d0-a000-7000-8000-000000000003"
)

type testStore struct {
	document *domain.Document
	receipts map[string]platformcommand.Receipt
	mediaErr error
	canWrite bool
}

func (store *testStore) WithinTransaction(_ context.Context, run func(Repository) error) error {
	copyStore := *store
	copyStore.receipts = make(map[string]platformcommand.Receipt, len(store.receipts))
	for key, value := range store.receipts {
		copyStore.receipts[key] = value
	}
	if store.document != nil {
		copyDocument := *store.document
		copyStore.document = &copyDocument
	}
	if err := run(&copyStore); err != nil {
		return err
	}
	store.document, store.receipts = copyStore.document, copyStore.receipts
	return nil
}

func (store *testStore) ProjectScope(_ context.Context, _ Actor, requested string, write bool) (string, error) {
	if requested != projectID || (write && !store.canWrite) {
		return "", &Error{Code: "forbidden", Status: 403}
	}
	return "019fb2d0-a000-7000-8000-000000000004", nil
}
func (store *testStore) FindDocument(_ context.Context, _ string, _ bool) (domain.Document, bool, error) {
	if store.document == nil {
		return domain.Document{}, false, nil
	}
	return *store.document, true, nil
}
func (store *testStore) SaveDocument(_ context.Context, _ string, document domain.Document) error {
	store.document = &document
	return nil
}
func (store *testStore) ValidateMediaVersion(_ context.Context, _, _, _ string) error {
	return store.mediaErr
}
func (store *testStore) FindReceipt(_ context.Context, _, _, key string) (platformcommand.Receipt, error) {
	receipt, ok := store.receipts[key]
	if !ok {
		return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
	}
	return receipt, nil
}
func (store *testStore) CreateReceipt(_ context.Context, receipt platformcommand.Receipt) error {
	store.receipts[receipt.IdempotencyKey] = receipt
	return nil
}

func TestCanvasOperationPersistsAndReplaysWithoutDuplicateMutation(t *testing.T) {
	store := &testStore{canWrite: true, receipts: map[string]platformcommand.Receipt{}}
	service := NewService(store, func() time.Time { return time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC) }, func() string { return "019fb2d0-a000-7000-8000-000000000005" })
	actor := Actor{UserID: userID, TokenVersion: 1}
	empty, err := service.Get(context.Background(), actor, projectID)
	if err != nil || empty.Revision != 0 || len(empty.Nodes) != 0 {
		t.Fatalf("unexpected empty canvas: %+v, %v", empty, err)
	}
	command := ApplyCommand{ProjectID: projectID, IdempotencyKey: "create-one", Operations: []domain.Operation{{Kind: "create_node", Node: domain.Node{ID: nodeID, Kind: "text", Title: "提示词", Width: 280, Height: 180}}}}
	first, err := service.Apply(context.Background(), actor, command)
	if err != nil || first.Document.Revision != 1 || len(first.Document.Nodes) != 1 || first.Replayed {
		t.Fatalf("first operation failed: %+v, %v", first, err)
	}
	replay, err := service.Apply(context.Background(), actor, command)
	if err != nil || !replay.Replayed || replay.AppliedRevision != 1 || store.document.Revision != 1 {
		t.Fatalf("replay changed the document: %+v, %v", replay, err)
	}
	command.Operations[0].Node.Title = "different"
	if _, err = service.Apply(context.Background(), actor, command); err == nil {
		t.Fatal("same idempotency key accepted different input")
	}
}

func TestCanvasOperationRejectsMediaAndViewerWithoutSaving(t *testing.T) {
	store := &testStore{canWrite: true, receipts: map[string]platformcommand.Receipt{}, mediaErr: errors.New("media not ready")}
	service := NewService(store, time.Now, func() string { return "019fb2d0-a000-7000-8000-000000000005" })
	actor := Actor{UserID: userID, TokenVersion: 1}
	command := ApplyCommand{ProjectID: projectID, IdempotencyKey: "with-media", Operations: []domain.Operation{{Kind: "create_node", Node: domain.Node{ID: nodeID, Kind: "image", Title: "图", Width: 280, Height: 220, MediaVersionID: "019fb2d0-a000-7000-8000-000000000006"}}}}
	if _, err := service.Apply(context.Background(), actor, command); err == nil || store.document != nil {
		t.Fatalf("unready media created a canvas node: %v", err)
	}
	store.mediaErr = nil
	store.canWrite = false
	if _, err := service.Apply(context.Background(), actor, command); err == nil || store.document != nil {
		t.Fatalf("viewer created a canvas node: %v", err)
	}
}
