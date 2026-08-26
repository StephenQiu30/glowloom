package storygraph_test

import (
	"encoding/json"
	"slices"
	"testing"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestCompilerFreezesSortedOwnerSetAndRejectsNonOwnerSources(t *testing.T) {
	sourceOwner := storygraph.OwnerRef{
		OwnerKind: "production/script", OwnerLogicalID: "document-1",
		OwnerVersionID: "10000000-0000-0000-0000-000000000001", OwnerRevision: 1,
		ContentHash: hashText("source"),
	}
	episodeOwner := storygraph.OwnerRef{
		OwnerKind: "production/project", OwnerLogicalID: "episode-1",
		OwnerVersionID: "10000000-0000-0000-0000-000000000002", OwnerRevision: 2,
		ContentHash: hashText("episode"),
	}
	sourceNode := nodeFor(t, storygraph.NodeTypeSourceRevision, sourceOwner, `{"version_no":1}`)
	episodeNode := nodeFor(t, storygraph.NodeTypeEpisode, episodeOwner, `{"position":1}`)
	edge := newEdge(t, storygraph.EdgeTypeDerivedFrom, sourceNode.StoryNodeKey, episodeNode.StoryNodeKey, storygraph.EdgeQualifier{})
	heads := []storygraph.OwnerHeadRef{
		storygraph.OwnerHeadRefFrom(episodeOwner),
		storygraph.OwnerHeadRefFrom(sourceOwner),
	}
	snapshot := storygraph.OwnerSnapshot{
		Origin:             storygraph.OwnerSnapshotOriginConfirmed,
		WorkspaceID:        "20000000-0000-0000-0000-000000000001",
		ProjectID:          "20000000-0000-0000-0000-000000000002",
		SourceRevisionID:   sourceOwner.OwnerVersionID,
		SourceRevisionHash: sourceOwner.ContentHash,
		OwnerHeads:         heads,
		Graph:              storygraph.Snapshot{SchemaVersion: storygraph.SchemaVersion, Nodes: []storygraph.Node{episodeNode, sourceNode}, Edges: []storygraph.Edge{edge}},
	}

	compiled, err := storygraph.CompileOwnerSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.OwnerHeads) != 2 || compiled.OwnerHeads[0].OwnerKind != "production/project" || len(compiled.OwnerSetHash) != 64 {
		t.Fatalf("owner set was not canonicalized: %#v", compiled)
	}

	reversed := snapshot
	reversed.OwnerHeads = slices.Clone(snapshot.OwnerHeads)
	slices.Reverse(reversed.OwnerHeads)
	reversed.Graph.Nodes = slices.Clone(snapshot.Graph.Nodes)
	slices.Reverse(reversed.Graph.Nodes)
	again, err := storygraph.CompileOwnerSnapshot(reversed)
	if err != nil || again.OwnerSetHash != compiled.OwnerSetHash || again.Graph.ContentHash != compiled.Graph.ContentHash {
		t.Fatalf("input traversal changed compilation: %#v %v", again, err)
	}

	contaminated := snapshot
	contaminated.Origin = "agent_candidate"
	if _, err = storygraph.CompileOwnerSnapshot(contaminated); err == nil {
		t.Fatal("compiler accepted an Agent candidate as an Owner snapshot")
	}

	drifted := snapshot
	drifted.Graph.Nodes = slices.Clone(snapshot.Graph.Nodes)
	drifted.Graph.Nodes[0].OwnerRef.OwnerRevision++
	drifted.Graph.Nodes[0].ContentHash = ""
	if _, err = storygraph.CompileOwnerSnapshot(drifted); err == nil {
		t.Fatal("compiler accepted a node outside the frozen Owner Set")
	}
}

func TestCanonicalGraphRejectsUnknownEdgeEndpointCombinations(t *testing.T) {
	assetOwner := storygraph.OwnerRef{
		OwnerKind: "asset", OwnerLogicalID: "asset-1", FragmentKey: "identity",
		OwnerVersionID: "30000000-0000-0000-0000-000000000001", OwnerRevision: 1,
		ContentHash: hashText("asset"),
	}
	episodeOwner := storygraph.OwnerRef{
		OwnerKind: "production/project", OwnerLogicalID: "episode-1",
		OwnerVersionID: "30000000-0000-0000-0000-000000000002", OwnerRevision: 1,
		ContentHash: hashText("episode"),
	}
	asset := nodeFor(t, storygraph.NodeTypeAssetIdentity, assetOwner, `{"asset_kind":"character"}`)
	episode := nodeFor(t, storygraph.NodeTypeEpisode, episodeOwner, `{"position":1}`)
	invalid := newEdge(t, storygraph.EdgeTypeContains, asset.StoryNodeKey, episode.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: "invalid"})

	if _, err := storygraph.Canonicalize(storygraph.Snapshot{
		SchemaVersion: storygraph.SchemaVersion,
		Nodes:         []storygraph.Node{asset, episode},
		Edges:         []storygraph.Edge{invalid},
	}); err == nil {
		t.Fatal("contains accepted asset_identity -> episode")
	}
}

func nodeFor(t *testing.T, nodeType storygraph.NodeType, owner storygraph.OwnerRef, payload string) storygraph.Node {
	t.Helper()
	key, err := storygraph.DeriveStoryNodeKey(nodeType, owner)
	if err != nil {
		t.Fatal(err)
	}
	return storygraph.Node{
		StoryNodeKey: key,
		NodeType:     nodeType,
		OwnerRef:     owner,
		EvidenceRefs: []storygraph.EvidenceRef{},
		Payload:      json.RawMessage(payload),
	}
}
