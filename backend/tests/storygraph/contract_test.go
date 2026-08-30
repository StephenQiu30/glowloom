package storygraph_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

type storyGraphFixture struct {
	SchemaVersion   string                     `json:"schema_version"`
	GraphBoundaries []storygraph.GraphBoundary `json:"graph_boundaries"`
	NodeKeyCase     struct {
		NodeType    storygraph.NodeType `json:"node_type"`
		OwnerRef    storygraph.OwnerRef `json:"owner_ref"`
		ExpectedKey string              `json:"expected_key"`
	} `json:"node_key_case"`
	EdgeKeyCase struct {
		EdgeType    storygraph.EdgeType      `json:"edge_type"`
		From        string                   `json:"from_node_key"`
		To          string                   `json:"to_node_key"`
		Qualifier   storygraph.EdgeQualifier `json:"qualifier"`
		ExpectedKey string                   `json:"expected_key"`
	} `json:"edge_key_case"`
	ExpectedTopologyHash string                  `json:"expected_topology_hash"`
	ExpectedContentHash  string                  `json:"expected_content_hash"`
	Claim                storygraph.ClaimPayload `json:"claim"`
}

func TestStoryGraphContractFixtureFreezesBoundariesAndStableKeys(t *testing.T) {
	fixture := loadStoryGraphFixture(t)
	if fixture.SchemaVersion != storygraph.SchemaVersion {
		t.Fatalf("schema version = %q", fixture.SchemaVersion)
	}
	if got := storygraph.GraphBoundaries(); !equalBoundaries(got, fixture.GraphBoundaries) {
		t.Fatalf("graph boundaries = %#v, want %#v", got, fixture.GraphBoundaries)
	}
	nodeKey, err := storygraph.DeriveStoryNodeKey(fixture.NodeKeyCase.NodeType, fixture.NodeKeyCase.OwnerRef)
	if err != nil || nodeKey != fixture.NodeKeyCase.ExpectedKey {
		t.Fatalf("node key = %q, want %q, error = %v", nodeKey, fixture.NodeKeyCase.ExpectedKey, err)
	}
	changedVersion := fixture.NodeKeyCase.OwnerRef
	changedVersion.OwnerVersionID = "10000000-0000-0000-0000-000000000099"
	changedVersion.OwnerRevision++
	changedVersion.ContentHash = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	stableKey, err := storygraph.DeriveStoryNodeKey(fixture.NodeKeyCase.NodeType, changedVersion)
	if err != nil || stableKey != nodeKey {
		t.Fatalf("exact owner version changed stable node key to %q: %v", stableKey, err)
	}
	edgeKey, err := storygraph.DeriveEdgeKey(fixture.EdgeKeyCase.EdgeType, fixture.EdgeKeyCase.From, fixture.EdgeKeyCase.To, fixture.EdgeKeyCase.Qualifier)
	if err != nil || edgeKey != fixture.EdgeKeyCase.ExpectedKey {
		t.Fatalf("edge key = %q, want %q, error = %v", edgeKey, fixture.EdgeKeyCase.ExpectedKey, err)
	}
}

func TestClaimPayloadRequiresTypedParticipantsAnchorsAndScope(t *testing.T) {
	fixture := loadStoryGraphFixture(t)
	if err := fixture.Claim.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := fixture.Claim
	invalid.Participants = invalid.Participants[:1]
	if err := invalid.Validate(); err == nil {
		t.Fatal("claim accepted without the object participant")
	}
	invalid = fixture.Claim
	invalid.Anchors = nil
	if err := invalid.Validate(); err == nil {
		t.Fatal("claim accepted without a semantic anchor")
	}
}

func TestStoryGraphRejectsWrongOwnerAndReturnsDeterministicCycle(t *testing.T) {
	fixture := loadStoryGraphFixture(t)
	wrongOwner := fixture.NodeKeyCase.OwnerRef
	wrongOwner.OwnerKind = "production/bible"
	if _, err := storygraph.DeriveStoryNodeKey(fixture.NodeKeyCase.NodeType, wrongOwner); err == nil {
		t.Fatal("asset identity accepted a non-asset owner")
	}
	cycle := []storygraph.Edge{
		{EdgeKey: "sge_a", FromNodeKey: "sgn_a", ToNodeKey: "sgn_b"},
		{EdgeKey: "sge_b", FromNodeKey: "sgn_b", ToNodeKey: "sgn_c"},
		{EdgeKey: "sge_c", FromNodeKey: "sgn_c", ToNodeKey: "sgn_a"},
		{EdgeKey: "sge_d", FromNodeKey: "sgn_a", ToNodeKey: "sgn_d"},
		{EdgeKey: "sge_e", FromNodeKey: "sgn_d", ToNodeKey: "sgn_a"},
	}
	path, err := storygraph.TopologicalOrder([]string{"sgn_d", "sgn_c", "sgn_a", "sgn_b"}, cycle)
	if err == nil || path != nil {
		t.Fatalf("cycle returned order %#v, error %v", path, err)
	}
	cycleError, ok := err.(*storygraph.CycleError)
	if !ok || string(mustJSON(cycleError.Path)) != `["sgn_a","sgn_d","sgn_a"]` {
		t.Fatalf("cycle path = %#v, error = %v", cycleError.Path, err)
	}
}

func TestCanonicalSnapshotIsDeterministicAndSeparatesTopologyFromOwnerContent(t *testing.T) {
	fixture := loadStoryGraphFixture(t)
	assetA := storygraph.OwnerRef{OwnerKind: "asset", OwnerLogicalID: "character-a", FragmentKey: "identity", OwnerVersionID: "70000000-0000-0000-0000-000000000001", OwnerRevision: 1, ContentHash: hashOf("a")}
	assetB := storygraph.OwnerRef{OwnerKind: "asset", OwnerLogicalID: "character-b", FragmentKey: "identity", OwnerVersionID: "70000000-0000-0000-0000-000000000002", OwnerRevision: 1, ContentHash: hashOf("b")}
	scene := storygraph.OwnerRef{OwnerKind: "production/planning", OwnerLogicalID: "episode-1", FragmentKey: "scene-1", OwnerVersionID: "70000000-0000-0000-0000-000000000003", OwnerRevision: 1, ContentHash: hashOf("c")}
	claim := storygraph.OwnerRef{OwnerKind: "production/bible", OwnerLogicalID: "bible-main", FragmentKey: "relationship-1", OwnerVersionID: "70000000-0000-0000-0000-000000000004", OwnerRevision: 1, ContentHash: hashOf("d")}

	assetAKey := mustNodeKey(t, storygraph.NodeTypeAssetIdentity, assetA)
	assetBKey := mustNodeKey(t, storygraph.NodeTypeAssetIdentity, assetB)
	sceneKey := mustNodeKey(t, storygraph.NodeTypeScene, scene)
	claimKey := mustNodeKey(t, storygraph.NodeTypeRelationshipClaim, claim)
	claimPayload := fixture.Claim
	claimPayload.Participants[0].StoryNodeKey = assetAKey
	claimPayload.Participants[1].StoryNodeKey = assetBKey
	claimPayload.Anchors = []string{sceneKey}

	nodes := []storygraph.Node{
		{StoryNodeKey: claimKey, NodeType: storygraph.NodeTypeRelationshipClaim, OwnerRef: claim, EvidenceRefs: []storygraph.EvidenceRef{{DocumentRevisionID: "80000000-0000-0000-0000-000000000001", AbsoluteStart: 4, AbsoluteEnd: 8, TextHash: hashOf("e")}}, Payload: mustJSON(claimPayload)},
		{StoryNodeKey: sceneKey, NodeType: storygraph.NodeTypeScene, OwnerRef: scene, Payload: json.RawMessage(`{}`)},
		{StoryNodeKey: assetBKey, NodeType: storygraph.NodeTypeAssetIdentity, OwnerRef: assetB, Payload: json.RawMessage(`{"asset_kind":"character"}`)},
		{StoryNodeKey: assetAKey, NodeType: storygraph.NodeTypeAssetIdentity, OwnerRef: assetA, Payload: json.RawMessage(`{"asset_kind":"character"}`)},
	}
	edges := []storygraph.Edge{
		newEdge(t, storygraph.EdgeTypeClaimAnchor, sceneKey, claimKey, storygraph.EdgeQualifier{AnchorRole: "scene"}),
		newEdge(t, storygraph.EdgeTypeClaimParticipant, assetBKey, claimKey, storygraph.EdgeQualifier{ParticipantRole: "object"}),
		newEdge(t, storygraph.EdgeTypeClaimParticipant, assetAKey, claimKey, storygraph.EdgeQualifier{ParticipantRole: "subject"}),
	}

	first, err := storygraph.Canonicalize(storygraph.Snapshot{SchemaVersion: storygraph.SchemaVersion, Nodes: nodes, Edges: edges})
	if err != nil {
		t.Fatal(err)
	}
	second, err := storygraph.Canonicalize(storygraph.Snapshot{SchemaVersion: storygraph.SchemaVersion, Nodes: reversed(nodes), Edges: reversed(edges)})
	if err != nil {
		t.Fatal(err)
	}
	if first.TopologyHash != second.TopologyHash || first.ContentHash != second.ContentHash || string(mustJSON(first)) != string(mustJSON(second)) {
		t.Fatal("canonical snapshot changed with input traversal order")
	}
	if first.TopologyHash != fixture.ExpectedTopologyHash || first.ContentHash != fixture.ExpectedContentHash {
		t.Fatalf("canonical hashes topology=%s content=%s", first.TopologyHash, first.ContentHash)
	}

	changedNodes := append([]storygraph.Node(nil), nodes...)
	for index := range changedNodes {
		if changedNodes[index].StoryNodeKey == assetAKey {
			changedNodes[index].OwnerRef.OwnerVersionID = "70000000-0000-0000-0000-000000000099"
			changedNodes[index].OwnerRef.OwnerRevision = 2
			changedNodes[index].OwnerRef.ContentHash = hashOf("f")
			changedNodes[index].ContentHash = ""
		}
	}
	changed, err := storygraph.Canonicalize(storygraph.Snapshot{SchemaVersion: storygraph.SchemaVersion, Nodes: changedNodes, Edges: edges})
	if err != nil {
		t.Fatal(err)
	}
	if changed.TopologyHash != first.TopologyHash || changed.ContentHash == first.ContentHash {
		t.Fatalf("exact Owner change topology=%s/%s content=%s/%s", changed.TopologyHash, first.TopologyHash, changed.ContentHash, first.ContentHash)
	}
}

func TestSnapshotStrictDecodeRejectsCanvasStateAndInvalidEvidence(t *testing.T) {
	if _, err := storygraph.DecodeSnapshot([]byte(`{"schema_version":"storygraph-scene-production","nodes":[],"edges":[],"viewport":{"x":0}}`)); err == nil {
		t.Fatal("StoryGraph snapshot accepted Canvas viewport state")
	}
	invalid := storygraph.EvidenceRef{DocumentRevisionID: "80000000-0000-0000-0000-000000000001", AbsoluteStart: 8, AbsoluteEnd: 4, TextHash: hashOf("a")}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Evidence accepted a reversed Unicode half-open range")
	}
}

func loadStoryGraphFixture(t *testing.T) storyGraphFixture {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve fixture path")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "..", "fixtures", "storygraph", "storygraph-contract.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture storyGraphFixture
	if err = json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func equalBoundaries(left, right []storygraph.GraphBoundary) bool {
	return string(mustJSON(left)) == string(mustJSON(right))
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func hashOf(value string) string {
	return value + "000000000000000000000000000000000000000000000000000000000000000"[:63]
}

func mustNodeKey(t *testing.T, nodeType storygraph.NodeType, owner storygraph.OwnerRef) string {
	t.Helper()
	key, err := storygraph.DeriveStoryNodeKey(nodeType, owner)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func newEdge(t *testing.T, edgeType storygraph.EdgeType, from, to string, qualifier storygraph.EdgeQualifier) storygraph.Edge {
	t.Helper()
	key, err := storygraph.DeriveEdgeKey(edgeType, from, to, qualifier)
	if err != nil {
		t.Fatal(err)
	}
	return storygraph.Edge{EdgeKey: key, EdgeType: edgeType, FromNodeKey: from, ToNodeKey: to, Qualifier: qualifier}
}

func reversed[T any](values []T) []T {
	result := append([]T(nil), values...)
	slices.Reverse(result)
	return result
}
