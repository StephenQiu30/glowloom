package storygraph_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

type storyGraphQueryReader struct {
	current             string
	currentOwnerSetHash string
	versions            map[string]storygraph.Version
}

func (reader *storyGraphQueryReader) GetCurrentOwnerSetHash(_ context.Context, _ storygraphapp.Actor, _ string) (string, error) {
	return reader.currentOwnerSetHash, nil
}

func TestStoryGraphLargeLensEnforcesBoundAndRejectsDriftedCursor(t *testing.T) {
	first := largeQueryVersion(250, "10000000-0000-0000-0000-000000000010")
	second := largeQueryVersion(1, "10000000-0000-0000-0000-000000000011")
	second.VersionNo = 2
	reader := &storyGraphQueryReader{
		current:  first.ID,
		versions: map[string]storygraph.Version{first.ID: first, second.ID: second},
	}
	service := storygraphapp.NewQueryService(reader)
	query := storygraphapp.LensQuery{
		ProjectID: first.ProjectID, VersionRef: storygraphapp.VersionRefCurrent, Lens: "outline",
		ScopeKind: storygraphapp.ScopeProject, ScopeID: first.ProjectID, Depth: 0, Limit: 200,
	}

	page, err := service.Lens(context.Background(), storygraphapp.Actor{}, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Nodes) != 200 || !page.Truncated || page.NextCursor == "" {
		t.Fatalf("large graph page is not bounded: nodes=%d truncated=%t", len(page.Nodes), page.Truncated)
	}
	repeated, err := service.Lens(context.Background(), storygraphapp.Actor{}, query)
	if err != nil || repeated.ResultHash != page.ResultHash || repeated.NextCursor != page.NextCursor {
		t.Fatalf("large graph result is not deterministic: repeated=%#v error=%v", repeated, err)
	}
	reader.current = second.ID
	query.Cursor = page.NextCursor
	continued, err := service.Lens(context.Background(), storygraphapp.Actor{}, query)
	if err != nil || continued.VersionID != first.ID || len(continued.Nodes) != 50 || len(continued.Edges) != 150 || !continued.Truncated || len(continued.Nodes)+len(continued.Edges) > 200 {
		t.Fatalf("large graph continuation diverged: %#v error=%v", continued, err)
	}
	query.Cursor = continued.NextCursor
	completed, err := service.Lens(context.Background(), storygraphapp.Actor{}, query)
	if err != nil || len(completed.Nodes) != 0 || len(completed.Edges) != 99 || completed.Truncated || len(completed.Nodes)+len(completed.Edges) > 200 {
		t.Fatalf("large graph edge continuation diverged: %#v error=%v", completed, err)
	}

	drifted := query
	drifted.Cursor = page.NextCursor
	drifted.Limit = 199
	_, err = service.Lens(context.Background(), storygraphapp.Actor{}, drifted)
	expectStoryGraphQueryCode(t, err, "stale_storygraph_cursor")
	drifted = query
	drifted.VersionRef = second.ID
	_, err = service.Lens(context.Background(), storygraphapp.Actor{}, drifted)
	expectStoryGraphQueryCode(t, err, "stale_storygraph_cursor")
	tooLarge := query
	tooLarge.Cursor = ""
	tooLarge.Limit = 201
	_, err = service.Lens(context.Background(), storygraphapp.Actor{}, tooLarge)
	expectStoryGraphQueryCode(t, err, "invalid_storygraph")
}

func TestStoryGraphFiveLensesAreBoundedAndImpactTraversesBothDirections(t *testing.T) {
	keys := []string{
		"sgn_0000000000000000000000000000000000000000000000000000000000000001",
		"sgn_0000000000000000000000000000000000000000000000000000000000000002",
		"sgn_0000000000000000000000000000000000000000000000000000000000000003",
		"sgn_0000000000000000000000000000000000000000000000000000000000000004",
	}
	version := storygraph.Version{
		ID: "10000000-0000-0000-0000-000000000020", ProjectID: "40000000-0000-0000-0000-000000000002",
		VersionNo: 1, ContentHash: hashText("five-lens"),
		Nodes: []storygraph.Node{
			{StoryNodeKey: keys[0], NodeType: storygraph.NodeTypeSourceRevision, ContentHash: hashText("source")},
			{StoryNodeKey: keys[1], NodeType: storygraph.NodeTypeScene, ContentHash: hashText("scene")},
			{StoryNodeKey: keys[2], NodeType: storygraph.NodeTypeAssetIdentity, ContentHash: hashText("asset")},
			{StoryNodeKey: keys[3], NodeType: storygraph.NodeTypeShot, ContentHash: hashText("shot")},
		},
		Edges: []storygraph.Edge{
			{EdgeKey: "sge_0000000000000000000000000000000000000000000000000000000000000001", FromNodeKey: keys[0], ToNodeKey: keys[1], ContentHash: hashText("source-scene")},
			{EdgeKey: "sge_0000000000000000000000000000000000000000000000000000000000000002", FromNodeKey: keys[1], ToNodeKey: keys[3], ContentHash: hashText("scene-shot")},
		},
	}
	reader := &storyGraphQueryReader{current: version.ID, versions: map[string]storygraph.Version{version.ID: version}}
	service := storygraphapp.NewQueryService(reader)
	for _, test := range []struct {
		lens, scopeKind, scopeID string
		depth, nodes             int
	}{
		{"outline", storygraphapp.ScopeProject, version.ProjectID, 0, 2},
		{"narrative", storygraphapp.ScopeProject, version.ProjectID, 0, 2},
		{"entity", storygraphapp.ScopeProject, version.ProjectID, 0, 1},
		{"production", storygraphapp.ScopeProject, version.ProjectID, 0, 1},
		{"impact", storygraphapp.ScopeStoryNode, keys[1], 1, 3},
	} {
		result, err := service.Lens(context.Background(), storygraphapp.Actor{}, storygraphapp.LensQuery{
			ProjectID: version.ProjectID, VersionRef: version.ID, Lens: test.lens,
			ScopeKind: test.scopeKind, ScopeID: test.scopeID, Depth: test.depth, Limit: 20,
		})
		if err != nil || len(result.Nodes) != test.nodes || len(result.ResultHash) != 64 {
			t.Fatalf("lens %s result=%#v error=%v", test.lens, result, err)
		}
	}
}

func TestStoryGraphDiffClassifiesStableNodeAndEdgeKeys(t *testing.T) {
	nodeA := "sgn_1000000000000000000000000000000000000000000000000000000000000000"
	nodeB := "sgn_2000000000000000000000000000000000000000000000000000000000000000"
	nodeC := "sgn_3000000000000000000000000000000000000000000000000000000000000000"
	nodeD := "sgn_4000000000000000000000000000000000000000000000000000000000000000"
	edgeRemoved := "sge_1000000000000000000000000000000000000000000000000000000000000000"
	edgeAdded := "sge_2000000000000000000000000000000000000000000000000000000000000000"
	edgeChanged := "sge_3000000000000000000000000000000000000000000000000000000000000000"
	base := storygraph.Version{
		ID: "10000000-0000-0000-0000-000000000030", ProjectID: "40000000-0000-0000-0000-000000000002", ContentHash: hashText("diff-base"),
		Nodes: []storygraph.Node{{StoryNodeKey: nodeA, ContentHash: hashText("before")}, {StoryNodeKey: nodeB, ContentHash: hashText("removed")}, {StoryNodeKey: nodeD, ContentHash: hashText("stable")}},
		Edges: []storygraph.Edge{{EdgeKey: edgeRemoved, FromNodeKey: nodeB, ToNodeKey: nodeD, ContentHash: hashText("removed-edge")}, {EdgeKey: edgeChanged, FromNodeKey: nodeA, ToNodeKey: nodeD, ContentHash: hashText("before-edge")}},
	}
	target := storygraph.Version{
		ID: "10000000-0000-0000-0000-000000000031", ProjectID: base.ProjectID, ContentHash: hashText("diff-target"),
		Nodes: []storygraph.Node{{StoryNodeKey: nodeA, ContentHash: hashText("after")}, {StoryNodeKey: nodeC, ContentHash: hashText("added")}, {StoryNodeKey: nodeD, ContentHash: hashText("stable")}},
		Edges: []storygraph.Edge{{EdgeKey: edgeAdded, FromNodeKey: nodeC, ToNodeKey: nodeD, ContentHash: hashText("added-edge")}, {EdgeKey: edgeChanged, FromNodeKey: nodeA, ToNodeKey: nodeD, ContentHash: hashText("after-edge")}},
	}
	service := storygraphapp.NewQueryService(&storyGraphQueryReader{versions: map[string]storygraph.Version{base.ID: base, target.ID: target}})
	result, err := service.Diff(context.Background(), storygraphapp.Actor{}, storygraphapp.DiffQuery{
		ProjectID: base.ProjectID, BaseVersionID: base.ID, TargetVersionID: target.ID, Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	nodeChanges := map[string]string{}
	for _, change := range result.NodeChanges {
		nodeChanges[change.StoryNodeKey] = change.ChangeType
	}
	edgeChanges := map[string]string{}
	for _, change := range result.EdgeChanges {
		edgeChanges[change.EdgeKey] = change.ChangeType
	}
	if len(nodeChanges) != 3 || nodeChanges[nodeA] != "changed" || nodeChanges[nodeB] != "removed" || nodeChanges[nodeC] != "added" ||
		len(edgeChanges) != 3 || edgeChanges[edgeRemoved] != "removed" || edgeChanges[edgeAdded] != "added" || edgeChanges[edgeChanged] != "changed" {
		t.Fatalf("unexpected stable-key diff: nodes=%v edges=%v", nodeChanges, edgeChanges)
	}
}

func largeQueryVersion(size int, versionID string) storygraph.Version {
	nodes := make([]storygraph.Node, size)
	edges := make([]storygraph.Edge, 0, max(size-1, 0))
	for index := range nodes {
		nodes[index] = storygraph.Node{
			StoryNodeKey: fmt.Sprintf("sgn_%064x", index+1), NodeType: storygraph.NodeTypeSourceRevision,
			ContentHash: fmt.Sprintf("%064x", index+1),
		}
		if index > 0 {
			edges = append(edges, storygraph.Edge{
				EdgeKey: fmt.Sprintf("sge_%064x", index), FromNodeKey: nodes[index-1].StoryNodeKey,
				ToNodeKey: nodes[index].StoryNodeKey, ContentHash: fmt.Sprintf("%064x", index),
			})
		}
	}
	return storygraph.Version{
		ID: versionID, WorkspaceID: "40000000-0000-0000-0000-000000000001", ProjectID: "40000000-0000-0000-0000-000000000002",
		VersionNo: 1, SchemaVersion: storygraph.SchemaVersion, Nodes: nodes, Edges: edges,
		TopologyHash: hashText("large-topology"), ContentHash: hashText(versionID), Status: "published",
	}
}

func expectStoryGraphQueryCode(t *testing.T, err error, expected string) {
	t.Helper()
	var queryError *storygraphapp.Error
	if !errors.As(err, &queryError) || queryError.Code != expected {
		t.Fatalf("error=%v, want code %s", err, expected)
	}
}

func (reader *storyGraphQueryReader) GetCurrentVersion(_ context.Context, _ storygraphapp.Actor, _ string) (storygraph.Version, error) {
	return reader.versions[reader.current], nil
}

func (reader *storyGraphQueryReader) GetExactVersion(_ context.Context, _ storygraphapp.Actor, _, versionID string) (storygraph.Version, error) {
	value, ok := reader.versions[versionID]
	if !ok {
		return storygraph.Version{}, storygraphapp.ErrNotFound
	}
	return value, nil
}

func TestStoryGraphQueriesAreBoundedDeterministicAndPinCurrentCursor(t *testing.T) {
	first, keys := queryVersion(t, "10000000-0000-0000-0000-000000000001", "第一集")
	second, _ := queryVersion(t, "10000000-0000-0000-0000-000000000002", "第一集（修订）")
	second.VersionNo = 2
	parentID, parentHash := first.ID, first.ContentHash
	second.ParentVersionID, second.ParentContentHash = &parentID, &parentHash
	reader := &storyGraphQueryReader{
		current: first.ID, currentOwnerSetHash: first.OwnerSetHash,
		versions: map[string]storygraph.Version{
			first.ID: first, second.ID: second,
		},
	}
	service := storygraphapp.NewQueryService(reader)
	actor := storygraphapp.Actor{UserID: "20000000-0000-0000-0000-000000000001", TokenVersion: 1}

	firstPage, err := service.Lens(context.Background(), actor, storygraphapp.LensQuery{
		ProjectID: first.ProjectID, VersionRef: "current", Lens: "outline",
		ScopeKind: "project", ScopeID: first.ProjectID, Depth: 2, Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if firstPage.VersionID != first.ID || len(firstPage.Nodes) != 2 || !firstPage.Truncated || firstPage.NextCursor == "" || len(firstPage.ResultHash) != 64 {
		t.Fatalf("unexpected first Lens page: %#v", firstPage)
	}
	reader.current = second.ID
	continued, err := service.Lens(context.Background(), actor, storygraphapp.LensQuery{
		ProjectID: first.ProjectID, VersionRef: "current", Lens: "outline",
		ScopeKind: "project", ScopeID: first.ProjectID, Depth: 2, Limit: 2,
		Cursor: firstPage.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if continued.VersionID != first.ID || len(continued.Nodes) != 1 || len(continued.Edges) != 1 || !continued.Truncated {
		t.Fatalf("current cursor did not pin the original exact version: %#v", continued)
	}
	completed, err := service.Lens(context.Background(), actor, storygraphapp.LensQuery{
		ProjectID: first.ProjectID, VersionRef: "current", Lens: "outline",
		ScopeKind: "project", ScopeID: first.ProjectID, Depth: 2, Limit: 2,
		Cursor: continued.NextCursor,
	})
	if err != nil || completed.VersionID != first.ID || len(completed.Nodes) != 0 || len(completed.Edges) != 1 || completed.Truncated {
		t.Fatalf("current cursor did not complete the original graph: %#v error=%v", completed, err)
	}

	narrative, err := service.Lens(context.Background(), actor, storygraphapp.LensQuery{
		ProjectID: first.ProjectID, VersionRef: first.ID, Lens: "narrative",
		ScopeKind: "story_node", ScopeID: keys.scene, Depth: 1, Limit: 20,
	})
	if err != nil || len(narrative.Nodes) != 3 || len(narrative.Edges) != 2 {
		t.Fatalf("unexpected narrative Lens: %#v error=%v", narrative, err)
	}

	upstream, err := service.Trace(context.Background(), actor, storygraphapp.TraceQuery{
		ProjectID: first.ProjectID, VersionRef: first.ID, StoryNodeKey: keys.scene,
		Direction: "upstream", Depth: 2, Limit: 20,
	})
	if err != nil || len(upstream.Nodes) != 3 || len(upstream.Edges) != 2 {
		t.Fatalf("unexpected upstream trace: %#v error=%v", upstream, err)
	}
	downstream, err := service.Trace(context.Background(), actor, storygraphapp.TraceQuery{
		ProjectID: first.ProjectID, VersionRef: first.ID, StoryNodeKey: keys.scene,
		Direction: "downstream", Depth: 1, Limit: 20,
	})
	if err != nil || len(downstream.Nodes) != 3 || len(downstream.Edges) != 2 {
		t.Fatalf("unexpected downstream trace: %#v error=%v", downstream, err)
	}

	diff, err := service.Diff(context.Background(), actor, storygraphapp.DiffQuery{
		ProjectID: first.ProjectID, BaseVersionID: first.ID, TargetVersionID: second.ID, Limit: 20,
	})
	if err != nil || len(diff.NodeChanges) != 1 || diff.NodeChanges[0].ChangeType != "changed" ||
		diff.NodeChanges[0].StoryNodeKey != keys.episode || len(diff.EdgeChanges) != 0 || len(diff.ResultHash) != 64 {
		t.Fatalf("stable-key diff diverged: %#v error=%v", diff, err)
	}
}

type queryKeys struct{ episode, scene string }

func queryVersion(t *testing.T, versionID, episodeLabel string) (storygraph.Version, queryKeys) {
	t.Helper()
	sourceOwner := storygraph.OwnerRef{OwnerKind: "production/script", OwnerLogicalID: "document-1", OwnerVersionID: "30000000-0000-0000-0000-000000000001", OwnerRevision: 1, ContentHash: hashText("source")}
	episodeOwner := storygraph.OwnerRef{OwnerKind: "production/project", OwnerLogicalID: "episode-1", OwnerVersionID: "30000000-0000-0000-0000-000000000002", OwnerRevision: 1, ContentHash: hashText(episodeLabel)}
	planningOwner := storygraph.OwnerRef{OwnerKind: "production/planning", OwnerLogicalID: "episode-1", OwnerVersionID: "30000000-0000-0000-0000-000000000003", OwnerRevision: 1, ContentHash: hashText("planning")}
	source := nodeFor(t, storygraph.NodeTypeSourceRevision, sourceOwner, `{"version_no":1}`)
	episode := nodeFor(t, storygraph.NodeTypeEpisode, episodeOwner, `{"position":1}`)
	episode.Label = episodeLabel
	sceneOwner := planningOwner
	sceneOwner.FragmentKey = "scene/scene-1"
	scene := nodeFor(t, storygraph.NodeTypeScene, sceneOwner, `{"heading":"书房"}`)
	dialogueOwner := planningOwner
	dialogueOwner.FragmentKey = "dialogue/dialogue-1"
	dialogue := nodeFor(t, storygraph.NodeTypeDialogue, dialogueOwner, `{"text":"开始"}`)
	beatOwner := planningOwner
	beatOwner.FragmentKey = "narrative-beat/beat-1"
	beat := nodeFor(t, storygraph.NodeTypeNarrativeBeat, beatOwner, `{"text":"窗外"}`)
	edges := []storygraph.Edge{
		newEdge(t, storygraph.EdgeTypeDerivedFrom, source.StoryNodeKey, episode.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeContains, episode.StoryNodeKey, scene.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: "episode/1/scenes/1"}),
		newEdge(t, storygraph.EdgeTypeContains, scene.StoryNodeKey, dialogue.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: "scene/1/dialogues/1"}),
		newEdge(t, storygraph.EdgeTypeContains, scene.StoryNodeKey, beat.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: "scene/1/beats/1"}),
	}
	graph, err := storygraph.Canonicalize(storygraph.Snapshot{SchemaVersion: storygraph.SchemaVersion, Nodes: []storygraph.Node{source, episode, scene, dialogue, beat}, Edges: edges})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 27, 10, 0, 0, 0, time.UTC)
	return storygraph.Version{
		ID: versionID, WorkspaceID: "40000000-0000-0000-0000-000000000001", ProjectID: "40000000-0000-0000-0000-000000000002",
		VersionNo: 1, SchemaVersion: graph.SchemaVersion, Nodes: graph.Nodes, Edges: graph.Edges,
		TopologyHash: graph.TopologyHash, ContentHash: graph.ContentHash, Status: "published", PublishedAt: now, CreatedAt: now,
	}, queryKeys{episode: episode.StoryNodeKey, scene: scene.StoryNodeKey}
}
