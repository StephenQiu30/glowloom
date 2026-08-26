package workflow_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.temporal.io/sdk/testsuite"
	temporalworkflow "go.temporal.io/sdk/workflow"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	workflowtemporal "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestSystemShotCatalogCompilesOnlyToDedicatedTemporalWorkflow(t *testing.T) {
	catalog, err := authoring.SystemShotCatalog()
	if err != nil {
		t.Fatalf("build system Shot catalog: %v", err)
	}
	if catalog.Key != "lanverse.shot" || catalog.Version != "1.0.0" || len(catalog.Definitions) != 4 {
		t.Fatalf("unexpected system Shot catalog identity: %#v", catalog)
	}
	wantDefinitions := []string{
		"human.generation_image_review@1.0.0",
		"input.generation_candidate_set@1.0.0",
		"input.production_shot@1.0.0",
		"production.shot_image_binding@1.0.0",
	}
	observedDefinitions := make([]string, len(catalog.Definitions))
	for index, definition := range catalog.Definitions {
		observedDefinitions[index] = definition.Key + "@" + definition.Version
		if !definition.Executable || definition.ContentHash == "" {
			t.Fatalf("Shot catalog contains an incomplete definition: %#v", definition)
		}
	}
	if !slices.Equal(observedDefinitions, wantDefinitions) {
		t.Fatalf("Shot catalog definitions = %v, want %v", observedDefinitions, wantDefinitions)
	}

	shotID, providerJobID := uuid.NewString(), uuid.NewString()
	graph := authoring.Graph{
		Nodes: []authoring.Node{
			{ID: "shot", DefinitionKey: "input.production_shot", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{"shot_id":"` + shotID + `"}`)},
			{ID: "candidates", DefinitionKey: "input.generation_candidate_set", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{"provider_job_id":"` + providerJobID + `"}`)},
			{ID: "image-review", DefinitionKey: "human.generation_image_review", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			{ID: "bind-image", DefinitionKey: "production.shot_image_binding", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{"expected_current_revision":0}`)},
		},
		Edges: []authoring.Edge{
			{ID: "shot-candidates", FromNodeID: "shot", FromPort: "shot", ToNodeID: "candidates", ToPort: "shot"},
			{ID: "candidate-review", FromNodeID: "candidates", FromPort: "candidates", ToNodeID: "image-review", ToPort: "candidates"},
			{ID: "review-binding", FromNodeID: "image-review", FromPort: "selection", ToNodeID: "bind-image", ToPort: "selection"},
			{ID: "shot-binding", FromNodeID: "shot", FromPort: "shot", ToNodeID: "bind-image", ToPort: "shot"},
		},
	}
	revision := compilerRevision(t, catalog, "GUIDED", graph, json.RawMessage(`{"guided":{"step":"shot-image"}}`), authoring.FrozenReference{
		Kind: "script_revision", ID: uuid.NewString(), Version: "1", Hash: strings.Repeat("a", 64),
	})
	contract, err := workflow.SystemCompilerContract(catalog.Key)
	if err != nil {
		t.Fatalf("resolve Shot compiler contract: %v", err)
	}
	compiled, err := workflow.Compile(workflow.CompilationSource{Revision: revision, Catalog: catalog}, contract)
	if err != nil {
		t.Fatalf("compile formal Shot workflow: %v", err)
	}
	if compiled.Definition.WorkflowType != workflowtemporal.ShotProductionWorkflowName ||
		compiled.Definition.WorkflowTypeVersion != "1.0.0" ||
		!slices.Equal(compiled.Definition.ExecutionOrder, []string{"shot", "candidates", "image-review", "bind-image"}) {
		t.Fatalf("formal Shot workflow compilation drifted: %#v", compiled.Definition)
	}

	episodeCatalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	episodeContract, err := workflow.SystemCompilerContract(episodeCatalog.Key)
	if err != nil {
		t.Fatalf("resolve Episode compiler contract: %v", err)
	}
	if episodeContract.WorkflowType != workflowtemporal.EpisodeProductionWorkflowName ||
		episodeContract.WorkflowType == contract.WorkflowType {
		t.Fatalf("Episode and Shot compiler contracts are not isolated: episode=%#v shot=%#v", episodeContract, contract)
	}
	if _, err = workflow.SystemCompilerContract("caller.supplied.workflow"); err == nil {
		t.Fatal("compiler accepted a caller-supplied Workflow type/catalog identity")
	}
}

func TestShotWorkflowRejectsEpisodeStartIdentity(t *testing.T) {
	request := episodeWorkflowStartRequest()
	request.WorkflowType = workflowtemporal.ShotProductionWorkflowName
	request.WorkflowID = "lanverse:shot-contract:" + uuid.NewString()
	request.WorkflowRunID = uuid.NewString()

	environment := new(testsuite.WorkflowTestSuite).NewTestWorkflowEnvironment()
	environment.RegisterWorkflowWithOptions(
		workflowtemporal.ShotProductionWorkflow,
		temporalworkflow.RegisterOptions{Name: workflowtemporal.ShotProductionWorkflowName},
	)
	environment.ExecuteWorkflow(workflowtemporal.ShotProductionWorkflowName, request)
	if !environment.IsWorkflowCompleted() || environment.GetWorkflowError() == nil {
		t.Fatalf("Shot Workflow did not fail without its own execution plan activities: %v", environment.GetWorkflowError())
	}

	request.WorkflowType = workflowtemporal.EpisodeProductionWorkflowName
	environment = new(testsuite.WorkflowTestSuite).NewTestWorkflowEnvironment()
	environment.RegisterWorkflowWithOptions(
		workflowtemporal.ShotProductionWorkflow,
		temporalworkflow.RegisterOptions{Name: workflowtemporal.ShotProductionWorkflowName},
	)
	environment.ExecuteWorkflow(workflowtemporal.ShotProductionWorkflowName, request)
	if !environment.IsWorkflowCompleted() || environment.GetWorkflowError() == nil ||
		!strings.Contains(environment.GetWorkflowError().Error(), "invalid Shot workflow start input") {
		t.Fatalf("Shot Workflow accepted Episode start identity: %v", environment.GetWorkflowError())
	}
}
