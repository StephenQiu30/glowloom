package workflow_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestCompilerProducesEquivalentDefinitionForGuidedAndCanvas(t *testing.T) {
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	graph := compilerJourneyGraph()
	input := authoring.FrozenReference{
		Kind: "script_revision", ID: "00000000-0000-0000-0000-000000000101", Version: "1", Hash: strings.Repeat("a", 64),
	}
	guided := compilerRevision(t, catalog, "GUIDED", graph, json.RawMessage(`{"guided":{"step":1}}`), input)
	canvas := compilerRevision(t, catalog, "CANVAS", graph, json.RawMessage(`{"viewport":{"x":90,"y":80,"zoom":2}}`), input)
	canvas.ID = "00000000-0000-0000-0000-000000000902"

	contract, err := workflow.SystemCompilerContract(catalog.Key)
	if err != nil {
		t.Fatalf("resolve system compiler contract: %v", err)
	}
	first, err := workflow.Compile(workflow.CompilationSource{Revision: guided, Catalog: catalog}, contract)
	if err != nil {
		t.Fatalf("compile guided revision: %v", err)
	}
	second, err := workflow.Compile(workflow.CompilationSource{Revision: canvas, Catalog: catalog}, contract)
	if err != nil {
		t.Fatalf("compile canvas revision: %v", err)
	}
	if len(first.Definition.ContentHash) != 64 || len(first.RunInputSnapshot.ContentHash) != 64 {
		t.Fatalf("compiler omitted content hashes: %#v", first)
	}
	if first.Definition.ContentHash != second.Definition.ContentHash || first.RunInputSnapshot.ContentHash != second.RunInputSnapshot.ContentHash {
		t.Fatalf("authoring UI changed execution result: guided=%#v canvas=%#v", first, second)
	}
	if first.Definition.AuthoringRevisionID == second.Definition.AuthoringRevisionID {
		t.Fatal("definition metadata lost the immutable source revision identity")
	}
	wantOrder := []string{
		"script", "bible", "bible-review", "episodes", "episodes-review", "structure", "structure-review", "storyboard", "storyboard-review", "export",
	}
	if !slices.Equal(first.Definition.ExecutionOrder, wantOrder) {
		t.Fatalf("execution order = %v, want %v", first.Definition.ExecutionOrder, wantOrder)
	}
	if len(first.Definition.ExecutionGraph.Nodes) != 10 || len(first.Definition.NodeExecutions) != 10 {
		t.Fatalf("unexpected compiled graph: %#v", first.Definition)
	}
	humanGates := 0
	for _, node := range first.Definition.NodeExecutions {
		if node.Executor == "" || node.DefinitionContentHash == "" || node.CachePolicy == "" || len(node.OutputPorts) == 0 {
			t.Fatalf("incomplete node execution descriptor: %#v", node)
		}
		if node.RiskLevel == "human_gate" {
			humanGates++
		}
	}
	if humanGates != 4 {
		t.Fatalf("human gate count = %d, want 4", humanGates)
	}
	if first.Definition.WorkflowType != "lanverse.episode-production" || first.Definition.WorkflowTypeVersion != "1.0.0" {
		t.Fatalf("unexpected Temporal workflow binding: %#v", first.Definition)
	}
}

func TestCompilerExcludesVisualNodesAndRejectsRevisionDrift(t *testing.T) {
	catalog, err := authoring.NewCatalog("compiler-test", "1.0.0", []authoring.NodeDefinition{
		{
			Key: "test.source", Version: "1.0.0", Name: "Source", Category: "input", Executor: "workflow.source",
			OutputPorts:  []authoring.PortDefinition{{Key: "value", ValueType: "artifact", Required: true}},
			ConfigSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), CachePolicy: "never", RiskLevel: "low", Executable: true,
		},
		{
			Key: "visual.comment", Version: "1.0.0", Name: "Comment", Category: "visual", Executor: "visual.none",
			ConfigSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`),
			CachePolicy:  "never", RiskLevel: "low", Executable: false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	input := authoring.FrozenReference{
		Kind: "artifact", ID: "00000000-0000-0000-0000-000000000201", Version: "1", Hash: strings.Repeat("b", 64),
	}
	revision := compilerRevision(t, catalog, "CANVAS", authoring.Graph{Nodes: []authoring.Node{
		{ID: "source", DefinitionKey: "test.source", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
		{ID: "note", DefinitionKey: "visual.comment", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{"text":"仅画布说明"}`)},
	}}, json.RawMessage(`{"nodes":{}}`), input)

	contract := workflow.CompilerContract{
		CompilerVersion: "1.0.0", WorkflowType: "lanverse.compiler-test",
		WorkflowTypeVersion: "1.0.0", RuntimeContractVersion: "1.0.0",
	}
	compiled, err := workflow.Compile(workflow.CompilationSource{Revision: revision, Catalog: catalog}, contract)
	if err != nil {
		t.Fatalf("compile revision with visual node: %v", err)
	}
	if len(compiled.Definition.ExecutionGraph.Nodes) != 1 || compiled.Definition.ExecutionGraph.Nodes[0].ID != "source" || len(compiled.Definition.NodeExecutions) != 1 {
		t.Fatalf("visual node entered executable definition: %#v", compiled.Definition)
	}

	drifted := revision
	drifted.ExecutionHash = strings.Repeat("f", 64)
	if _, err = workflow.Compile(workflow.CompilationSource{Revision: drifted, Catalog: catalog}, contract); err == nil {
		t.Fatal("compiler accepted an authoring revision whose execution hash drifted")
	}
	drifted = revision
	drifted.CatalogExecutionHash = strings.Repeat("e", 64)
	if _, err = workflow.Compile(workflow.CompilationSource{Revision: drifted, Catalog: catalog}, contract); err == nil {
		t.Fatal("compiler accepted a node catalog binding whose hash drifted")
	}
}

func compilerRevision(
	t *testing.T,
	catalog authoring.Catalog,
	mode string,
	graph authoring.Graph,
	layout json.RawMessage,
	input authoring.FrozenReference,
) authoring.Revision {
	t.Helper()
	snapshot, err := authoring.PublishSnapshot(authoring.DraftSnapshot{
		AuthoringMode: mode, Graph: graph, Layout: layout, FrozenInputs: []authoring.FrozenReference{input},
	}, catalog)
	if err != nil {
		t.Fatalf("publish compiler fixture: %v", err)
	}
	return authoring.Revision{
		ID: "00000000-0000-0000-0000-000000000901", WorkspaceID: "00000000-0000-0000-0000-000000000801",
		ProjectID: "00000000-0000-0000-0000-000000000701", DraftID: uuid.NewString(), CatalogID: uuid.NewString(),
		RevisionNo: 1, RevisionSnapshot: snapshot, CreatedBy: uuid.NewString(), CreatedAt: time.Date(2026, time.August, 25, 18, 0, 0, 0, time.UTC),
	}
}

func compilerJourneyGraph() authoring.Graph {
	node := func(id, key, config string) authoring.Node {
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
			node("export", "production.storyboard_export", `{}`),
			node("storyboard-review", "human.storyboard_review", `{}`),
			node("storyboard", "agent.storyboard_draft", `{}`),
			node("structure-review", "human.episode_structure_review", `{}`),
			node("structure", "production.episode_structure", `{}`),
			node("episodes-review", "human.episode_plan_review", `{}`),
			node("episodes", "production.episode_plan", `{"episode_count":5}`),
			node("bible-review", "human.production_bible_review", `{}`),
			node("bible", "agent.production_bible", `{}`),
			node("script", "input.script_revision", `{"document_revision_id":"00000000-0000-0000-0000-000000000101"}`),
		},
		Edges: []authoring.Edge{
			edge("review-export", "storyboard-review", "storyboards", "export", "storyboards"),
			edge("storyboard-review", "storyboard", "candidate", "storyboard-review", "candidate"),
			edge("review-storyboard", "structure-review", "structures", "storyboard", "structures"),
			edge("structure-review", "structure", "candidate", "structure-review", "candidate"),
			edge("review-structure", "episodes-review", "episodes", "structure", "episodes"),
			edge("episodes-review", "episodes", "candidate", "episodes-review", "candidate"),
			edge("review-episodes", "bible-review", "bible", "episodes", "bible"),
			edge("script-episodes", "script", "script", "episodes", "script"),
			edge("bible-review", "bible", "candidate", "bible-review", "candidate"),
			edge("script-bible", "script", "script", "bible", "script"),
		},
	}
}
