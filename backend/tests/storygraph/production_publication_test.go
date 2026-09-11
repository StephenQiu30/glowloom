package storygraph_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestCompileProductionPublishesExactInputAndReplaysAtomically(t *testing.T) {
	snapshot := productionOwnerSnapshotFixture(t)
	store := &ownerSetStore{
		state:              storygraph.PublicationState{WorkspaceID: snapshot.WorkspaceID, ProjectID: snapshot.ProjectID},
		productionSnapshot: snapshot, receipts: map[string]platformcommand.Receipt{}, versions: map[string]storygraph.Version{},
	}
	service := storygraphapp.NewService(store, storygraphapp.Config{
		Now:   func() time.Time { return time.Date(2026, time.September, 10, 9, 0, 0, 0, time.UTC) },
		NewID: uuid.NewString,
	})
	command := storygraphapp.CompileProductionCommand{
		ProjectID:                  snapshot.ProjectID,
		ProductionWorldReceiptID:   snapshot.ProductionWorldConfirmationID,
		ProductionWorldReceiptHash: snapshot.ProductionWorldConfirmationHash,
		IdempotencyKey:             "publish-production-storygraph",
	}
	actor := storygraphapp.Actor{UserID: uuid.NewString(), TokenVersion: 1}

	published, err := service.CompileProduction(context.Background(), actor, command)
	if err != nil {
		t.Fatal(err)
	}
	if published.Version.SchemaVersion != storygraph.ProductionSchemaID || published.Version.ProductionInput == nil ||
		len(published.Version.ProductionInput.OwnerCollections) != 7 || published.Head.CurrentVersionID != published.Version.ID ||
		published.Receipt.ResourceID != published.Version.ID || store.versionWrites != 1 || store.receiptWrites != 1 || store.outboxWrites != 1 {
		t.Fatalf("Production StoryGraph publication = %#v store=%#v", published, store)
	}

	replayed, err := service.CompileProduction(context.Background(), actor, command)
	if err != nil || replayed.Version.ID != published.Version.ID || replayed.Receipt.ID != published.Receipt.ID ||
		store.versionWrites != 1 || store.receiptWrites != 1 || store.outboxWrites != 1 {
		t.Fatalf("Production StoryGraph replay = %#v err=%v store=%#v", replayed, err, store)
	}

	drifted := command
	drifted.ProductionWorldReceiptHash = productionHash("another-gate-two")
	if _, err = service.CompileProduction(context.Background(), actor, drifted); err == nil {
		t.Fatal("idempotency key accepted a different Production World receipt")
	}

	_, err = service.Compile(context.Background(), actor, storygraphapp.CompileCommand{
		ProjectID: snapshot.ProjectID, ExpectedHeadRevision: published.Head.Revision,
		ExpectedCurrentContentHash: published.Head.CurrentContentHash, IdempotencyKey: "reject-legacy-downgrade",
	})
	var applicationError *storygraphapp.Error
	if !errors.As(err, &applicationError) || applicationError.Code != "storygraph_schema_downgrade" ||
		store.versionWrites != 1 || store.receiptWrites != 1 || store.outboxWrites != 1 {
		t.Fatalf("legacy downgrade error=%#v store=%#v", err, store)
	}

	next := command
	next.ProductionWorldReceiptID = uuid.NewString()
	next.ProductionWorldReceiptHash = productionHash("next-production-world")
	next.IdempotencyKey = "append-production-storygraph"
	store.productionSnapshot.ProductionWorldConfirmationID = next.ProductionWorldReceiptID
	store.productionSnapshot.ProductionWorldConfirmationHash = next.ProductionWorldReceiptHash
	appended, err := service.CompileProduction(context.Background(), actor, next)
	if err != nil || appended.Version.VersionNo != 2 || appended.Version.ParentVersionID == nil ||
		*appended.Version.ParentVersionID != published.Version.ID || appended.Head.Revision != 2 ||
		store.versionWrites != 2 || store.receiptWrites != 2 || store.outboxWrites != 2 {
		t.Fatalf("append Production StoryGraph=%#v err=%v store=%#v", appended, err, store)
	}
}
