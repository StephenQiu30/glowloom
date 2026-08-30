package search_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	eventing "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
	searchapp "github.com/StephenQiu30/lanverse/backend/internal/search/application"
	search "github.com/StephenQiu30/lanverse/backend/internal/search/domain"
)

func TestKafkaProjectorLoadsCurrentPostgreSQLSnapshotInsteadOfTrustingPayload(t *testing.T) {
	now := time.Date(2026, 8, 27, 17, 0, 0, 0, time.UTC)
	reader := &searchSnapshotReader{storygraph: storyGraphSnapshot()}
	index := &recordingProjectionIndex{}
	projector := searchapp.NewProjector(reader, index, func() time.Time { return now })
	payload, err := json.Marshal(map[string]any{
		"version_id": searchVersionID, "version_no": 1,
		"owner_set_hash": searchHash, "topology_hash": searchHash, "content_hash": searchHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := eventing.NewEnvelope(eventing.OutboxEvent{
		ID: uuid.NewString(), EventType: eventing.StoryGraphVersionPublished, EventVersion: 1,
		WorkspaceID: searchWorkspaceID, ProjectID: searchProjectID, AggregateKind: "storygraph",
		AggregateID: searchProjectID, AggregateRevision: 1, SourceReceiptID: uuid.NewString(),
		Payload: payload, OccurredAt: now,
	}, eventing.TraceContext{RequestID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if err = projector.Process(context.Background(), envelope); err != nil {
		t.Fatal(err)
	}
	if reader.calls != 1 || index.snapshot.ContentHash != reader.storygraph.ContentHash || index.source.Kind != search.SourceEvent || index.source.ID != envelope.EventID {
		t.Fatalf("projector did not use current Owner snapshot: %#v source=%#v", index.snapshot, index.source)
	}
}

func TestReindexBuildsBothKindsFromPostgreSQLSnapshots(t *testing.T) {
	reader := &searchSnapshotReader{script: scriptSnapshot(), storygraph: storyGraphSnapshot()}
	index := &recordingProjectionIndex{}
	service := searchapp.NewReindexer(reader, index, func() time.Time { return time.Date(2026, 8, 27, 18, 0, 0, 0, time.UTC) }, func() string { return searchEventID })
	result, err := service.Reindex(context.Background(), search.KindScript)
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != search.KindScript || index.rebuildSource.Kind != search.SourceReindex || index.rebuildSource.ID != searchEventID {
		t.Fatalf("unexpected reindex contract: %#v source=%#v", result, index.rebuildSource)
	}
}

type recordingProjectionIndex struct {
	searchIndex
	snapshot      search.Snapshot
	source        search.ProjectionSource
	rebuildSource search.ProjectionSource
}

func (value *recordingProjectionIndex) Project(_ context.Context, snapshot search.Snapshot, source search.ProjectionSource, _ time.Time) error {
	value.snapshot, value.source = snapshot, source
	return nil
}

func (value *recordingProjectionIndex) Rebuild(_ context.Context, kind search.Kind, _ []search.Snapshot, source search.ProjectionSource, _ time.Time) (search.ReindexResult, error) {
	value.rebuildSource = source
	return search.ReindexResult{Kind: kind, IndexVersion: "backing-blue", Alias: "read-alias"}, nil
}
