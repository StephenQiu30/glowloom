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

func TestBuildRerunScopeDirtiesOnlyDownstreamClosureAndRequiresSideInputs(t *testing.T) {
	definition := rerunDefinitionFixture()
	source := rerunProjectionFixtures(t, definition)

	scope, err := workflow.BuildRerunScope(definition, source, "b")
	if err != nil {
		t.Fatalf("build rerun scope: %v", err)
	}
	if !slices.Equal(scope.DirtyNodeIDs, []string{"b", "c"}) {
		t.Fatalf("dirty nodes = %v, want [b c]", scope.DirtyNodeIDs)
	}
	if !slices.Equal(scope.ReusedNodeIDs, []string{"a", "d"}) {
		t.Fatalf("reused nodes = %v, want [a d]", scope.ReusedNodeIDs)
	}

	for index := range source {
		if source[index].NodeID == "d" {
			source[index].Status = "QUEUED"
			source[index].Output = nil
			source[index].OutputHash = ""
		}
	}
	if _, err = workflow.BuildRerunScope(definition, source, "b"); err == nil || !strings.Contains(err.Error(), "required upstream") {
		t.Fatalf("rerun accepted missing side input: %v", err)
	}
}

func rerunDefinitionFixture() workflow.WorkflowDefinitionVersion {
	port := func(key string) authoring.PortDefinition {
		return authoring.PortDefinition{Key: key, ValueType: "fact", Required: true}
	}
	node := func(id string, inputs, outputs []authoring.PortDefinition) workflow.NodeExecution {
		return workflow.NodeExecution{
			NodeID: id, DefinitionKey: "test." + id, DefinitionVersion: "1.0.0",
			DefinitionContentHash: strings.Repeat(id, 64), Executor: "activity." + id,
			InputPorts: inputs, OutputPorts: outputs, CachePolicy: "never", RiskLevel: "low",
		}
	}
	edge := func(id, from, fromPort, to, toPort string) authoring.Edge {
		return authoring.Edge{ID: id, FromNodeID: from, FromPort: fromPort, ToNodeID: to, ToPort: toPort}
	}
	return workflow.WorkflowDefinitionVersion{
		ExecutionOrder: []string{"a", "b", "d", "c"},
		ExecutionGraph: authoring.Graph{Edges: []authoring.Edge{
			edge("a-b", "a", "a", "b", "a"), edge("b-c", "b", "b", "c", "b"),
			edge("d-c", "d", "d", "c", "d"),
		}},
		NodeExecutions: []workflow.NodeExecution{
			node("a", nil, []authoring.PortDefinition{port("a")}),
			node("b", []authoring.PortDefinition{port("a")}, []authoring.PortDefinition{port("b")}),
			node("d", nil, []authoring.PortDefinition{port("d")}),
			node("c", []authoring.PortDefinition{port("b"), port("d")}, []authoring.PortDefinition{port("c")}),
		},
	}
}

func rerunProjectionFixtures(t *testing.T, definition workflow.WorkflowDefinitionVersion) []workflow.NodeRunProjection {
	t.Helper()
	frozen := []authoring.FrozenReference{{
		Kind: "script_revision", ID: uuid.NewString(), Version: "1", Hash: strings.Repeat("f", 64),
	}}
	now := time.Date(2026, time.August, 26, 9, 0, 0, 0, time.UTC)
	workspaceID, runID := uuid.NewString(), uuid.NewString()
	result := make([]workflow.NodeRunProjection, 0, len(definition.NodeExecutions))
	for _, execution := range definition.NodeExecutions {
		_, input, inputHash, err := workflow.BuildNodeInput(workflow.NodeInputSnapshot{
			SchemaVersion: workflow.NodeInputSchemaVersion, Config: json.RawMessage(`{}`), FrozenInputs: frozen,
		})
		if err != nil {
			t.Fatalf("build %s input: %v", execution.NodeID, err)
		}
		_, output, outputHash, err := workflow.BuildNodeOutput(workflow.NodeOutputSnapshot{
			SchemaVersion: workflow.NodeOutputSchemaVersion,
			Bindings: []workflow.NodeOutputBinding{{
				Port: execution.OutputPorts[0].Key, ValueType: "fact", ReferenceID: uuid.NewString(),
				ReferenceVersion: "1", ContentHash: strings.Repeat(execution.NodeID, 64),
			}},
		})
		if err != nil {
			t.Fatalf("build %s output: %v", execution.NodeID, err)
		}
		result = append(result, workflow.NodeRunProjection{
			ID: uuid.NewString(), WorkspaceID: workspaceID, WorkflowRunID: runID,
			NodeID: execution.NodeID, DefinitionKey: execution.DefinitionKey,
			DefinitionVersion: execution.DefinitionVersion, Executor: execution.Executor,
			RiskLevel: execution.RiskLevel, Status: "SUCCEEDED", Attempt: 1,
			Input: input, InputHash: inputHash, Output: output, OutputHash: outputHash,
			Revision: 2, CreatedAt: now, UpdatedAt: now,
		})
	}
	return result
}
