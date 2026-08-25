package authoring_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
)

func TestSystemCatalogCoversScriptToStoryboardJourney(t *testing.T) {
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatalf("build system catalog: %v", err)
	}
	if catalog.Key != "lanverse.production" || catalog.Version != "2.0.0" || len(catalog.ContentHash) != 64 {
		t.Fatalf("unexpected catalog identity: %#v", catalog)
	}

	want := []string{
		"agent.production_bible",
		"agent.storyboard_draft",
		"human.episode_plan_review",
		"human.episode_structure_review",
		"human.production_bible_review",
		"human.storyboard_review",
		"input.script_revision",
		"production.episode_plan",
		"production.episode_structure",
		"production.storyboard_export",
	}
	got := make([]string, 0, len(catalog.Definitions))
	for _, definition := range catalog.Definitions {
		got = append(got, definition.Key)
		wantVersion := "1.0.0"
		if definition.Key == "production.episode_plan" {
			wantVersion = "2.0.0"
		}
		if definition.Version != wantVersion || definition.Executor == "" || len(definition.ContentHash) != 64 {
			t.Fatalf("incomplete node definition: %#v", definition)
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("system catalog keys = %v, want %v", got, want)
	}
}

func TestPublishSnapshotNormalizesGraphAndExcludesLayoutFromExecutionHash(t *testing.T) {
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	draft := authoring.DraftSnapshot{
		AuthoringMode: "guided",
		Graph:         scriptToStoryboardGraph(),
		Layout:        json.RawMessage(`{"nodes":{"script":{"x":10,"y":20}}}`),
		FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: "00000000-0000-0000-0000-000000000101", Version: "1",
			Hash: strings.Repeat("a", 64),
		}},
	}

	first, err := authoring.PublishSnapshot(draft, catalog)
	if err != nil {
		t.Fatalf("publish valid graph: %v", err)
	}
	if len(first.ExecutionHash) != 64 || len(first.ContentHash) != 64 || first.ExecutionHash == first.ContentHash {
		t.Fatalf("unexpected snapshot hashes: %#v", first)
	}

	reordered := draft
	reordered.Graph.Nodes = append([]authoring.Node(nil), draft.Graph.Nodes...)
	reordered.Graph.Edges = append([]authoring.Edge(nil), draft.Graph.Edges...)
	slices.Reverse(reordered.Graph.Nodes)
	slices.Reverse(reordered.Graph.Edges)
	reordered.Layout = json.RawMessage(`{"nodes":{"script":{"x":900,"y":800}},"viewport":{"zoom":2}}`)
	second, err := authoring.PublishSnapshot(reordered, catalog)
	if err != nil {
		t.Fatalf("publish reordered graph: %v", err)
	}
	if first.ExecutionHash != second.ExecutionHash {
		t.Fatalf("layout or ordering changed execution hash: first=%s second=%s", first.ExecutionHash, second.ExecutionHash)
	}
	if first.ContentHash == second.ContentHash {
		t.Fatal("layout change must remain visible in the immutable revision content hash")
	}
}

func TestGraphValidationRejectsUnknownVersionInvalidConfigAndPortMismatch(t *testing.T) {
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*authoring.Graph)
	}{
		{
			name: "unknown node version",
			mutate: func(graph *authoring.Graph) {
				graph.Nodes[0].DefinitionVersion = "9.9.9"
			},
		},
		{
			name: "invalid config",
			mutate: func(graph *authoring.Graph) {
				for index := range graph.Nodes {
					if graph.Nodes[index].DefinitionKey == "production.episode_plan" {
						graph.Nodes[index].Config = json.RawMessage(`{"episode_count":0}`)
					}
				}
			},
		},
		{
			name: "port type mismatch",
			mutate: func(graph *authoring.Graph) {
				graph.Edges[0].ToPort = "bible"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph := scriptToStoryboardGraph()
			test.mutate(&graph)
			if _, err := authoring.ValidateGraph(graph, catalog); err == nil {
				t.Fatal("invalid graph was accepted")
			}
		})
	}
}

func TestGraphValidationRejectsGeneralCycles(t *testing.T) {
	emptySchema := json.RawMessage(`{"type":"object","additionalProperties":false}`)
	catalog, err := authoring.NewCatalog("cycle-test", "1.0.0", []authoring.NodeDefinition{
		{
			Key: "test.pass", Version: "1.0.0", Name: "Pass", Category: "control", Executor: "workflow.pass",
			InputPorts:   []authoring.PortDefinition{{Key: "value", ValueType: "artifact", Required: true}},
			OutputPorts:  []authoring.PortDefinition{{Key: "value", ValueType: "artifact", Required: true}},
			ConfigSchema: emptySchema, CachePolicy: "never", RiskLevel: "low", Executable: true,
		},
	})
	if err != nil {
		t.Fatalf("build cycle test catalog: %v", err)
	}
	graph := authoring.Graph{
		Nodes: []authoring.Node{
			{ID: "a", DefinitionKey: "test.pass", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			{ID: "b", DefinitionKey: "test.pass", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
		},
		Edges: []authoring.Edge{
			{ID: "a-to-b", FromNodeID: "a", FromPort: "value", ToNodeID: "b", ToPort: "value"},
			{ID: "b-to-a", FromNodeID: "b", FromPort: "value", ToNodeID: "a", ToPort: "value"},
		},
	}
	if _, err = authoring.ValidateGraph(graph, catalog); err == nil {
		t.Fatal("general cycle was accepted")
	}
}

func TestPublishSnapshotExcludesVisualNodesFromExecutionHash(t *testing.T) {
	emptySchema := json.RawMessage(`{"type":"object","additionalProperties":false}`)
	commentSchema := json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`)
	catalog, err := authoring.NewCatalog("visual-test", "1.0.0", []authoring.NodeDefinition{
		{
			Key: "test.source", Version: "1.0.0", Name: "Source", Category: "input", Executor: "workflow.source",
			OutputPorts:  []authoring.PortDefinition{{Key: "value", ValueType: "artifact", Required: true}},
			ConfigSchema: emptySchema, CachePolicy: "never", RiskLevel: "low", Executable: true,
		},
		{
			Key: "visual.comment", Version: "1.0.0", Name: "Comment", Category: "visual", Executor: "visual.none",
			ConfigSchema: commentSchema, CachePolicy: "never", RiskLevel: "low", Executable: false,
		},
	})
	if err != nil {
		t.Fatalf("build visual test catalog: %v", err)
	}
	draft := authoring.DraftSnapshot{
		AuthoringMode: "canvas",
		Graph: authoring.Graph{Nodes: []authoring.Node{
			{ID: "source", DefinitionKey: "test.source", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			{ID: "note", DefinitionKey: "visual.comment", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{"text":"第一版注释"}`)},
		}},
		Layout: json.RawMessage(`{}`),
		FrozenInputs: []authoring.FrozenReference{{
			Kind: "artifact", ID: "00000000-0000-0000-0000-000000000201", Version: "1", Hash: strings.Repeat("b", 64),
		}},
	}
	first, err := authoring.PublishSnapshot(draft, catalog)
	if err != nil {
		t.Fatalf("publish graph with visual node: %v", err)
	}
	draft.Graph.Nodes[1].Config = json.RawMessage(`{"text":"只改变画布说明"}`)
	second, err := authoring.PublishSnapshot(draft, catalog)
	if err != nil {
		t.Fatalf("republish graph with changed visual node: %v", err)
	}
	if first.ExecutionHash != second.ExecutionHash {
		t.Fatalf("visual node changed execution hash: first=%s second=%s", first.ExecutionHash, second.ExecutionHash)
	}
	if first.ContentHash == second.ContentHash {
		t.Fatal("visual node change must remain visible in revision content hash")
	}
}

func scriptToStoryboardGraph() authoring.Graph {
	node := func(id, key string, config string) authoring.Node {
		version := "1.0.0"
		if key == "production.episode_plan" {
			version = "2.0.0"
		}
		return authoring.Node{ID: id, DefinitionKey: key, DefinitionVersion: version, Config: json.RawMessage(config)}
	}
	edge := func(id, from, fromPort, to, toPort string) authoring.Edge {
		return authoring.Edge{ID: id, FromNodeID: from, FromPort: fromPort, ToNodeID: to, ToPort: toPort}
	}
	return authoring.Graph{
		Nodes: []authoring.Node{
			node("script", "input.script_revision", `{"document_revision_id":"00000000-0000-0000-0000-000000000101"}`),
			node("bible", "agent.production_bible", `{}`),
			node("bible-review", "human.production_bible_review", `{}`),
			node("episodes", "production.episode_plan", `{"episode_count":5}`),
			node("episodes-review", "human.episode_plan_review", `{}`),
			node("structure", "production.episode_structure", `{}`),
			node("structure-review", "human.episode_structure_review", `{}`),
			node("storyboard", "agent.storyboard_draft", `{}`),
			node("storyboard-review", "human.storyboard_review", `{}`),
			node("export", "production.storyboard_export", `{}`),
		},
		Edges: []authoring.Edge{
			edge("script-bible", "script", "script", "bible", "script"),
			edge("bible-review", "bible", "candidate", "bible-review", "candidate"),
			edge("script-episodes", "script", "script", "episodes", "script"),
			edge("review-episodes", "bible-review", "bible", "episodes", "bible"),
			edge("episodes-review", "episodes", "candidate", "episodes-review", "candidate"),
			edge("review-structure", "episodes-review", "episodes", "structure", "episodes"),
			edge("structure-review", "structure", "candidate", "structure-review", "candidate"),
			edge("review-storyboard", "structure-review", "structures", "storyboard", "structures"),
			edge("storyboard-review", "storyboard", "candidate", "storyboard-review", "candidate"),
			edge("review-export", "storyboard-review", "storyboards", "export", "storyboards"),
		},
	}
}
