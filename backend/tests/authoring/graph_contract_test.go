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
	if catalog.Key != "lanverse.production" || catalog.Version != "15.0.0" || len(catalog.ContentHash) != 64 {
		t.Fatalf("unexpected catalog identity: %#v", catalog)
	}

	want := []string{
		"agent.episode_analysis@1.0.0",
		"agent.episode_segmentation@1.0.0",
		"agent.production_bible@1.0.0",
		"agent.source_evidence@1.0.0",
		"agent.story_analysis@1.0.0",
		"agent.story_review@1.0.0",
		"agent.storyboard_draft@2.0.0",
		"generation.reference_asset@1.0.0",
		"human.episode_plan_review@1.0.0",
		"human.episode_plan_review@2.0.0",
		"human.episode_structure_review@1.0.0",
		"human.episode_structure_review@2.0.0",
		"human.production_bible_review@1.0.0",
		"human.production_bible_review@2.0.0",
		"human.storyboard_review@2.0.0",
		"input.script_revision@1.0.0",
		"production.bible_materialization@1.0.0",
		"production.episode_plan@2.0.0",
		"production.episode_structure@1.0.0",
		"production.storygraph_compile@1.0.0",
	}
	got := make([]string, 0, len(catalog.Definitions))
	for _, definition := range catalog.Definitions {
		got = append(got, definition.Key+"@"+definition.Version)
		if definition.Executor == "" || len(definition.ContentHash) != 64 {
			t.Fatalf("incomplete node definition: %#v", definition)
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("system catalog keys = %v, want %v", got, want)
	}
}

func TestReferenceAssetGenerationConsumesApprovedIntentsBeforeReturningCandidates(t *testing.T) {
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range catalog.Definitions {
		if definition.Key != "generation.reference_asset" {
			continue
		}
		if definition.Version != "1.0.0" || definition.Executor != "activity.reference_asset_generation" ||
			definition.CachePolicy != "by_inputs" || definition.RiskLevel != "external_ai" ||
			len(definition.InputPorts) != 1 || len(definition.OutputPorts) != 1 ||
			definition.InputPorts[0].Key != "intents" ||
			definition.InputPorts[0].ValueType != "approved_storyboard_intents" ||
			definition.OutputPorts[0].Key != "candidates" ||
			definition.OutputPorts[0].ValueType != "generation_candidate_set" {
			t.Fatalf("Reference Asset generation contract = %#v", definition)
		}
		configSchema := string(definition.ConfigSchema)
		if !strings.Contains(configSchema, `"asset_id"`) || !strings.Contains(configSchema, `"asset_state_id"`) ||
			!strings.Contains(configSchema, `"additionalProperties":false`) {
			t.Fatalf("Reference Asset config schema = %s", definition.ConfigSchema)
		}
		return
	}
	t.Fatal("Reference Asset generation is absent from the system catalog")
}

func TestStoryboardIntentHumanGateOnlyFreezesApprovedIntents(t *testing.T) {
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range catalog.Definitions {
		if definition.Key != "human.storyboard_review" || definition.Version != "2.0.0" {
			continue
		}
		if definition.Executor != "gate.storyboard_review" || definition.RiskLevel != "human_gate" ||
			definition.CachePolicy != "never" || len(definition.InputPorts) != 1 || len(definition.OutputPorts) != 1 ||
			definition.InputPorts[0].Key != "candidate" ||
			definition.InputPorts[0].ValueType != "storyboard_intent_candidate_set" ||
			definition.OutputPorts[0].Key != "intents" ||
			definition.OutputPorts[0].ValueType != "approved_storyboard_intents" {
			t.Fatalf("Storyboard Intent Human Gate contract = %#v", definition)
		}
		return
	}
	t.Fatal("Storyboard Intent Human Gate v2 is absent from the system catalog")
}

func TestStoryboardDraftConsumesPublishedStoryGraphWithoutFormalizingShots(t *testing.T) {
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range catalog.Definitions {
		if definition.Key != "agent.storyboard_draft" {
			continue
		}
		if definition.Version != "2.0.0" || definition.Executor != "activity.storyboard_draft" ||
			definition.CachePolicy != "by_inputs" || definition.RiskLevel != "external_ai" ||
			len(definition.InputPorts) != 1 || len(definition.OutputPorts) != 1 ||
			definition.InputPorts[0].Key != "storygraph" ||
			definition.InputPorts[0].ValueType != "storygraph_version" ||
			definition.OutputPorts[0].Key != "candidate" ||
			definition.OutputPorts[0].ValueType != "storyboard_intent_candidate_set" {
			t.Fatalf("Storyboard Draft contract = %#v", definition)
		}
		return
	}
	t.Fatal("Storyboard Draft v2 is absent from the system catalog")
}

func TestStoryGraphCompilerConsumesPlanningOwnerSetAndPublishesExactVersion(t *testing.T) {
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range catalog.Definitions {
		if definition.Key != "production.storygraph_compile" {
			continue
		}
		if definition.Executor != "activity.storygraph_compile" || definition.CachePolicy != "never" ||
			definition.RiskLevel != "low" || len(definition.InputPorts) != 1 || len(definition.OutputPorts) != 1 ||
			definition.InputPorts[0].Key != "structures" || definition.InputPorts[0].ValueType != "planning_owner_set" ||
			definition.OutputPorts[0].Key != "storygraph" || definition.OutputPorts[0].ValueType != "storygraph_version" {
			t.Fatalf("StoryGraph compiler contract = %#v", definition)
		}
		return
	}
	t.Fatal("StoryGraph compiler is absent from the system catalog")
}

func TestEpisodePlanningHumanGatePublishesOwnerSet(t *testing.T) {
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range catalog.Definitions {
		if definition.Key != "human.episode_structure_review" || definition.Version != "2.0.0" {
			continue
		}
		if definition.Executor != "gate.episode_structure_review" || definition.RiskLevel != "human_gate" ||
			definition.CachePolicy != "never" || len(definition.InputPorts) != 1 || len(definition.OutputPorts) != 1 ||
			definition.InputPorts[0].Key != "candidate" || definition.InputPorts[0].ValueType != "episode_planning_candidate_set" ||
			definition.OutputPorts[0].Key != "structures" || definition.OutputPorts[0].ValueType != "planning_owner_set" {
			t.Fatalf("Episode Planning Human Gate contract = %#v", definition)
		}
		return
	}
	t.Fatal("Episode Planning Human Gate v2 is absent from the system catalog")
}

func TestEpisodeAnalysisConsumesPublishedSetAndBibleMaterialization(t *testing.T) {
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range catalog.Definitions {
		if definition.Key != "agent.episode_analysis" {
			continue
		}
		if definition.Executor != "activity.episode_analysis" || definition.CachePolicy != "by_inputs" ||
			definition.RiskLevel != "external_ai" || len(definition.InputPorts) != 2 || len(definition.OutputPorts) != 1 ||
			definition.InputPorts[0].Key != "episodes" || definition.InputPorts[0].ValueType != "episode_set" ||
			definition.InputPorts[1].Key != "materialization" ||
			definition.InputPorts[1].ValueType != "production_bible_materialization" ||
			definition.OutputPorts[0].Key != "candidate" ||
			definition.OutputPorts[0].ValueType != "episode_planning_candidate_set" {
			t.Fatalf("Episode analysis node contract = %#v", definition)
		}
		return
	}
	t.Fatal("Episode analysis is absent from the system catalog")
}

func TestEpisodePlanHumanGateConsumesSegmentationCandidateAndPublishesEpisodeSet(t *testing.T) {
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range catalog.Definitions {
		if definition.Key != "human.episode_plan_review" || definition.Version != "2.0.0" {
			continue
		}
		if definition.Executor != "gate.episode_plan_review" || definition.RiskLevel != "human_gate" ||
			definition.CachePolicy != "never" || len(definition.InputPorts) != 1 || len(definition.OutputPorts) != 1 ||
			definition.InputPorts[0].Key != "candidate" || definition.InputPorts[0].ValueType != "episode_segmentation_candidate" ||
			definition.OutputPorts[0].Key != "episodes" || definition.OutputPorts[0].ValueType != "episode_set" {
			t.Fatalf("Episode Plan Human Gate contract = %#v", definition)
		}
		return
	}
	t.Fatal("Episode Plan Human Gate v2 is absent from the system catalog")
}

func TestProductionBibleMaterializationConsumesConfirmedVersionAndPublishesBindingSnapshot(t *testing.T) {
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range catalog.Definitions {
		if definition.Key != "production.bible_materialization" {
			continue
		}
		if definition.Executor != "activity.production_bible_materialization" || definition.CachePolicy != "never" ||
			definition.RiskLevel != "low" || len(definition.InputPorts) != 1 || len(definition.OutputPorts) != 1 ||
			definition.InputPorts[0].Key != "bible" || definition.InputPorts[0].ValueType != "production_bible_version" ||
			definition.OutputPorts[0].Key != "materialization" ||
			definition.OutputPorts[0].ValueType != "production_bible_materialization" {
			t.Fatalf("Production Bible materialization contract = %#v", definition)
		}
		return
	}
	t.Fatal("Production Bible materialization is absent from the system catalog")
}

func TestProductionBibleHumanGateConsumesReviewedStoryCandidateAndPublishesVersion(t *testing.T) {
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range catalog.Definitions {
		if definition.Key != "human.production_bible_review" || definition.Version != "2.0.0" {
			continue
		}
		if definition.Version != "2.0.0" || len(definition.InputPorts) != 1 || len(definition.OutputPorts) != 1 ||
			definition.InputPorts[0].Key != "candidate" || definition.InputPorts[0].ValueType != "story_reconciliation_candidate" ||
			definition.OutputPorts[0].Key != "bible" || definition.OutputPorts[0].ValueType != "production_bible_version" {
			t.Fatalf("Production Bible Human Gate contract = %#v", definition)
		}
		return
	}
	t.Fatal("Production Bible Human Gate is absent from the system catalog")
}

func TestStoryReviewNodeFreezesBoundedRepairRounds(t *testing.T) {
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	graph := authoring.Graph{
		Nodes: []authoring.Node{
			{ID: "script", DefinitionKey: "input.script_revision", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{"document_revision_id":"00000000-0000-0000-0000-000000000001"}`)},
			{ID: "evidence", DefinitionKey: "agent.source_evidence", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			{ID: "story", DefinitionKey: "agent.story_analysis", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			{ID: "review", DefinitionKey: "agent.story_review", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{"max_repair_rounds":2}`)},
		},
		Edges: []authoring.Edge{
			{ID: "script-evidence", FromNodeID: "script", FromPort: "script", ToNodeID: "evidence", ToPort: "script"},
			{ID: "evidence-story", FromNodeID: "evidence", FromPort: "evidence", ToNodeID: "story", ToPort: "evidence"},
			{ID: "story-review", FromNodeID: "story", FromPort: "candidate", ToNodeID: "review", ToPort: "candidate"},
		},
	}
	if _, err = authoring.ValidateGraph(graph, catalog); err != nil {
		t.Fatalf("bounded Story Review node was rejected: %v", err)
	}
	graph.Nodes[3].Config = json.RawMessage(`{"max_repair_rounds":4}`)
	if _, err = authoring.ValidateGraph(graph, catalog); err == nil {
		t.Fatal("Story Review accepted an unbounded repair budget")
	}
}

func TestPublishSnapshotNormalizesGraphAndExcludesLayoutFromExecutionHash(t *testing.T) {
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	draft := authoring.DraftSnapshot{
		AuthoringMode: "guided",
		Graph:         storyToBibleGraph(),
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
					if graph.Nodes[index].DefinitionKey == "human.production_bible_review" {
						graph.Nodes[index].Config = json.RawMessage(`{"expected_bible_version":0}`)
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
			graph := storyToBibleGraph()
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

func storyToBibleGraph() authoring.Graph {
	node := func(id, key string, config string) authoring.Node {
		version := "1.0.0"
		if key == "human.production_bible_review" {
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
			node("evidence", "agent.source_evidence", `{}`),
			node("story", "agent.story_analysis", `{}`),
			node("story-review", "agent.story_review", `{"max_repair_rounds":2}`),
			node("bible-review", "human.production_bible_review", `{"expected_bible_version":1}`),
			node("bible-materialization", "production.bible_materialization", `{}`),
		},
		Edges: []authoring.Edge{
			edge("script-evidence", "script", "script", "evidence", "script"),
			edge("evidence-story", "evidence", "evidence", "story", "evidence"),
			edge("story-review", "story", "candidate", "story-review", "candidate"),
			edge("review-bible", "story-review", "candidate", "bible-review", "candidate"),
			edge("bible-materialization", "bible-review", "bible", "bible-materialization", "bible"),
		},
	}
}
