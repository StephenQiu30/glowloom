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

func TestStoryGraphOwnerSetRequiresExactPlanningFactsAndReplaysOnePublication(t *testing.T) {
	fixture := newOwnerSetFixture(t, false)
	service := storygraphapp.NewService(fixture.store, storygraphapp.Config{
		Now:   func() time.Time { return time.Date(2026, time.August, 28, 4, 0, 0, 0, time.UTC) },
		NewID: uuid.NewString,
	})
	command := fixture.command("compile-owner-set")

	published, err := service.CompileOwnerSet(context.Background(), storygraphapp.Actor{
		UserID: fixture.userID, TokenVersion: 1,
	}, command)
	if err != nil {
		t.Fatalf("compile exact Owner Set: %v", err)
	}
	if published.Version.VersionNo != 1 || published.Head.CurrentVersionID != published.Version.ID ||
		published.Receipt.ResourceID != published.Version.ID || fixture.store.versionWrites != 1 ||
		fixture.store.receiptWrites != 1 || fixture.store.outboxWrites != 1 {
		t.Fatalf("unexpected Owner Set publication: result=%#v store=%#v", published, fixture.store)
	}

	replayed, err := service.CompileOwnerSet(context.Background(), storygraphapp.Actor{
		UserID: fixture.userID, TokenVersion: 1,
	}, command)
	if err != nil || replayed.Version.ID != published.Version.ID || replayed.Receipt.ID != published.Receipt.ID ||
		fixture.store.versionWrites != 1 || fixture.store.receiptWrites != 1 || fixture.store.outboxWrites != 1 {
		t.Fatalf("Owner Set replay diverged: result=%#v error=%v store=%#v", replayed, err, fixture.store)
	}

	drifted := command
	drifted.OwnerSetHash = hashText("different-owner-set")
	_, err = service.CompileOwnerSet(context.Background(), storygraphapp.Actor{
		UserID: fixture.userID, TokenVersion: 1,
	}, drifted)
	assertOwnerSetConflict(t, err)
	if fixture.store.versionWrites != 1 || fixture.store.receiptWrites != 1 || fixture.store.outboxWrites != 1 {
		t.Fatalf("idempotency conflict wrote facts: %#v", fixture.store)
	}

	contaminated := newOwnerSetFixture(t, true)
	contaminatedService := storygraphapp.NewService(contaminated.store, storygraphapp.Config{
		Now:   func() time.Time { return time.Date(2026, time.August, 28, 4, 0, 0, 0, time.UTC) },
		NewID: uuid.NewString,
	})
	_, err = contaminatedService.CompileOwnerSet(context.Background(), storygraphapp.Actor{
		UserID: contaminated.userID, TokenVersion: 1,
	}, contaminated.command("reject-extra-planning-owner"))
	assertOwnerSetConflict(t, err)
	if contaminated.store.versionWrites != 0 || contaminated.store.receiptWrites != 0 || contaminated.store.outboxWrites != 0 {
		t.Fatalf("inexact Planning Owner Set wrote facts: %#v", contaminated.store)
	}
}

type ownerSetFixture struct {
	store         *ownerSetStore
	userID        string
	projectID     string
	bibleVersion  string
	bibleHash     string
	planningOwner storygraph.OwnerHeadRef
}

func newOwnerSetFixture(t *testing.T, withExtraPlanningOwner bool) ownerSetFixture {
	t.Helper()
	workspaceID := uuid.NewString()
	projectID := uuid.NewString()
	userID := uuid.NewString()
	sourceVersionID := uuid.NewString()
	bibleVersionID := uuid.NewString()
	planningVersionID := uuid.NewString()
	sourceHash := hashText("source")
	bibleHash := hashText("bible")
	planningHash := hashText("planning")
	evidence := storygraph.EvidenceRef{
		DocumentRevisionID: sourceVersionID, AbsoluteStart: 0, AbsoluteEnd: 1, TextHash: hashText("a"),
	}
	sourceOwner := storygraph.OwnerRef{
		OwnerKind: "production/script", OwnerLogicalID: uuid.NewString(), OwnerVersionID: sourceVersionID,
		OwnerRevision: 1, ContentHash: sourceHash,
	}
	bibleOwner := storygraph.OwnerRef{
		OwnerKind: "production/bible", OwnerLogicalID: "source-evidence", FragmentKey: "range/0/1",
		OwnerVersionID: bibleVersionID, OwnerRevision: 1, ContentHash: bibleHash,
	}
	planningOwner := storygraph.OwnerRef{
		OwnerKind: "production/planning", OwnerLogicalID: uuid.NewString(), FragmentKey: "scene/scene-1",
		OwnerVersionID: planningVersionID, OwnerRevision: 1, ContentHash: planningHash,
	}
	nodes := []storygraph.Node{
		ownerSetNode(t, storygraph.NodeTypeSourceRevision, sourceOwner, nil),
		ownerSetNode(t, storygraph.NodeTypeSourceEvidence, bibleOwner, []storygraph.EvidenceRef{evidence}),
		ownerSetNode(t, storygraph.NodeTypeScene, planningOwner, []storygraph.EvidenceRef{evidence}),
	}
	heads := []storygraph.OwnerHeadRef{
		storygraph.OwnerHeadRefFrom(sourceOwner), storygraph.OwnerHeadRefFrom(bibleOwner), storygraph.OwnerHeadRefFrom(planningOwner),
	}
	if withExtraPlanningOwner {
		extra := storygraph.OwnerRef{
			OwnerKind: "production/planning", OwnerLogicalID: uuid.NewString(), FragmentKey: "scene/scene-2",
			OwnerVersionID: uuid.NewString(), OwnerRevision: 1, ContentHash: hashText("extra-planning"),
		}
		nodes = append(nodes, ownerSetNode(t, storygraph.NodeTypeScene, extra, []storygraph.EvidenceRef{evidence}))
		heads = append(heads, storygraph.OwnerHeadRefFrom(extra))
	}
	store := &ownerSetStore{
		state: storygraph.PublicationState{WorkspaceID: workspaceID, ProjectID: projectID},
		snapshot: storygraph.OwnerSnapshot{
			Origin: storygraph.OwnerSnapshotOriginConfirmed, WorkspaceID: workspaceID, ProjectID: projectID,
			SourceRevisionID: sourceVersionID, SourceRevisionHash: sourceHash, OwnerHeads: heads,
			Graph: storygraph.Snapshot{SchemaVersion: storygraph.SchemaVersion, Nodes: nodes, Edges: []storygraph.Edge{}},
		},
		receipts: map[string]platformcommand.Receipt{}, versions: map[string]storygraph.Version{},
	}
	return ownerSetFixture{
		store: store, userID: userID, projectID: projectID, bibleVersion: bibleVersionID, bibleHash: bibleHash,
		planningOwner: storygraph.OwnerHeadRefFrom(planningOwner),
	}
}

func (fixture ownerSetFixture) command(key string) storygraphapp.CompileOwnerSetCommand {
	return storygraphapp.CompileOwnerSetCommand{
		ProjectID: fixture.projectID, OwnerSetID: uuid.NewString(), OwnerSetHash: hashText("owner-set"),
		RequiredBibleVersionID: fixture.bibleVersion, RequiredBibleHash: fixture.bibleHash,
		RequiredOwners: []storygraph.OwnerHeadRef{fixture.planningOwner}, IdempotencyKey: key,
	}
}

func ownerSetNode(
	t *testing.T,
	nodeType storygraph.NodeType,
	owner storygraph.OwnerRef,
	evidence []storygraph.EvidenceRef,
) storygraph.Node {
	t.Helper()
	key, err := storygraph.DeriveStoryNodeKey(nodeType, owner)
	if err != nil {
		t.Fatal(err)
	}
	return storygraph.Node{
		StoryNodeKey: key, NodeType: nodeType, OwnerRef: owner, EvidenceRefs: evidence, Payload: []byte(`{}`),
	}
}

func assertOwnerSetConflict(t *testing.T, err error) {
	t.Helper()
	var applicationError *storygraphapp.Error
	if !errors.As(err, &applicationError) || applicationError.Code != "resource_conflict" {
		t.Fatalf("error=%#v, want resource_conflict", err)
	}
}

type ownerSetStore struct {
	state         storygraph.PublicationState
	snapshot      storygraph.OwnerSnapshot
	receipts      map[string]platformcommand.Receipt
	versions      map[string]storygraph.Version
	versionWrites int
	receiptWrites int
	outboxWrites  int
}

func (store *ownerSetStore) WithinSerializableTransaction(
	ctx context.Context,
	operation func(storygraphapp.Repository) error,
) error {
	return operation(store)
}

func (store *ownerSetStore) LockPublication(context.Context, storygraphapp.Actor, string) (storygraph.PublicationState, error) {
	return store.state, nil
}

func (store *ownerSetStore) LoadOwnerSnapshot(context.Context, storygraph.PublicationState) (storygraph.OwnerSnapshot, error) {
	return store.snapshot, nil
}

func (store *ownerSetStore) FindReceipt(
	_ context.Context,
	_ string,
	operation string,
	key string,
) (platformcommand.Receipt, error) {
	receipt, exists := store.receipts[operation+"\x00"+key]
	if !exists {
		return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
	}
	return receipt, nil
}

func (store *ownerSetStore) GetVersion(_ context.Context, versionID string) (storygraph.Version, error) {
	version, exists := store.versions[versionID]
	if !exists {
		return storygraph.Version{}, storygraphapp.ErrNotFound
	}
	return version, nil
}

func (store *ownerSetStore) CreateVersion(_ context.Context, version storygraph.Version) error {
	store.versionWrites++
	store.versions[version.ID] = version
	return nil
}

func (store *ownerSetStore) SwitchHead(
	_ context.Context,
	_ storygraph.PublicationState,
	version storygraph.Version,
) (storygraph.Head, error) {
	store.state.CurrentVersionID = version.ID
	store.state.CurrentContentHash = version.ContentHash
	store.state.HeadRevision = version.VersionNo
	return storygraph.Head{
		WorkspaceID: version.WorkspaceID, ProjectID: version.ProjectID, CurrentVersionID: version.ID,
		CurrentContentHash: version.ContentHash, Revision: version.VersionNo, UpdatedAt: version.PublishedAt,
	}, nil
}

func (store *ownerSetStore) CreateReceipt(_ context.Context, receipt platformcommand.Receipt) error {
	store.receiptWrites++
	store.receipts[receipt.Operation+"\x00"+receipt.IdempotencyKey] = receipt
	return nil
}

func (store *ownerSetStore) CreateOutbox(context.Context, storygraph.OutboxEvent) error {
	store.outboxWrites++
	return nil
}
