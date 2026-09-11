package workflow_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	presetdomain "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
	storygraphdomain "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type visualFoundationSelectionSource struct {
	current, exact presetdomain.ProjectSelection
	exactCalls     int
}

func (source *visualFoundationSelectionSource) Current(
	_ context.Context,
	_, _ string,
) (presetdomain.ProjectSelection, error) {
	return source.current, nil
}

func (source *visualFoundationSelectionSource) Exact(
	_ context.Context,
	_, _, _ string,
) (presetdomain.ProjectSelection, error) {
	source.exactCalls++
	return source.exact, nil
}

type visualFoundationWorldSource struct {
	world storygraphdomain.VisualFoundationWorldReadSet
}

func (source visualFoundationWorldSource) VisualFoundationWorld(
	_ context.Context,
	_ storygraphapp.Actor,
	_ string,
) (storygraphdomain.VisualFoundationWorldReadSet, error) {
	return source.world, nil
}

type visualFoundationConfirmedSource struct {
	source worlddomain.ConfirmedVisualFoundationSource
}

func (source visualFoundationConfirmedSource) Current(
	_ context.Context,
	_, _ string,
) (worlddomain.ConfirmedVisualFoundationSource, error) {
	return source.source, nil
}

type recordingVisualFoundationOwner struct {
	command agentapp.ExecuteVisualFoundationCommand
	calls   int
}

func (owner *recordingVisualFoundationOwner) Execute(
	_ context.Context,
	command agentapp.ExecuteVisualFoundationCommand,
) (agentapp.Candidate, error) {
	owner.calls++
	owner.command = command
	return agentapp.Candidate{
		ID: uuid.NewString(), WorkspaceID: command.Input.WorkspaceID, ProjectID: command.Input.ProjectID,
		StageKey: agentcontract.VisualFoundationStageKey, Revision: 1,
		CandidateType: "visual_foundation_candidate", CandidateRevisionHash: sceneTextHash("visual-foundation-candidate"),
	}, nil
}

func TestVisualFoundationWorkflowNodesFreezeSelectionAndExecuteExactInput(t *testing.T) {
	_, confirmedSource, world := faithfulVisualFoundationSources(t, false)
	release := curatedFaithfulRelease(t)
	selection := frozenProjectSelection(t, world.WorkspaceID, world.ProjectID, release)
	selections := &visualFoundationSelectionSource{current: selection, exact: selection}
	visualOwner := &recordingVisualFoundationOwner{}
	executor := workflowproduction.NewNodeExecutor(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		workflowproduction.SceneAnalysisDependencies{
			VisualFoundation: &workflowproduction.VisualFoundationDependencies{
				Selections: selections, FindRelease: func(key, version string) (presetdomain.Release, bool, error) {
					return release, key == release.Key && version == release.Release, nil
				},
				Worlds:  visualFoundationWorldSource{world: world},
				Sources: visualFoundationConfirmedSource{source: confirmedSource}, Candidates: visualOwner,
			},
		},
	)
	frozen := []authoring.FrozenReference{{
		Kind: "script_revision", ID: uuid.NewString(), Version: "1", Hash: sceneTextHash("script"),
	}}
	storyGraphBinding := workflow.NodeInputBinding{
		Port: "storygraph", ValueType: "storygraph_version", SourceKind: workflow.NodeInputSourceNodeOutput,
		SourceNodeID: "production-storygraph", SourcePort: "storygraph", ReferenceID: world.StoryGraphVersionID,
		ReferenceVersion: "1", ContentHash: world.StoryGraphContentHash,
	}
	selectionInput, _, selectionInputHash, err := workflow.BuildNodeInput(workflow.NodeInputSnapshot{
		SchemaVersion: workflow.NodeInputSchemaVersion, Config: json.RawMessage(`{}`),
		Bindings: []workflow.NodeInputBinding{storyGraphBinding}, FrozenInputs: frozen,
	})
	if err != nil {
		t.Fatal(err)
	}
	base := workflow.NodeExecutorCommand{
		NodeActivityCommand: workflow.NodeActivityCommand{
			WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), NodeID: "preset-selection",
			Executor: "activity.project_preset_selection", Attempt: 1,
		},
		WorkspaceID: world.WorkspaceID, ProjectID: world.ProjectID,
		InitiatorUserID: selection.SelectedBy, InitiatorTokenVersion: 1,
		IdempotencyKey: "freeze-project-preset", Input: selectionInput, InputHash: selectionInputHash,
		OutputPorts: []authoring.PortDefinition{{Key: "selection", ValueType: "project_preset_selection", Required: true}},
	}
	selectionResult, err := executor.Execute(context.Background(), base)
	if err != nil || selectionResult.Status != "SUCCEEDED" || len(selectionResult.Output.Bindings) != 1 ||
		selectionResult.Output.Bindings[0].ReferenceID != selection.ID ||
		selectionResult.Output.Bindings[0].ContentHash != selection.ContentHash {
		t.Fatalf("freeze exact Project Preset selection: result=%#v err=%v", selectionResult, err)
	}
	visualInput, _, visualInputHash, err := workflow.BuildNodeInput(workflow.NodeInputSnapshot{
		SchemaVersion: workflow.NodeInputSchemaVersion, Config: json.RawMessage(`{}`),
		Bindings: []workflow.NodeInputBinding{
			storyGraphBinding,
			{
				Port: "selection", ValueType: "project_preset_selection", SourceKind: workflow.NodeInputSourceNodeOutput,
				SourceNodeID: "preset-selection", SourcePort: "selection", ReferenceID: selection.ID,
				ReferenceVersion: "1", ContentHash: selection.ContentHash,
			},
		},
		FrozenInputs: frozen,
	})
	if err != nil {
		t.Fatal(err)
	}
	base.NodeActivityCommand = workflow.NodeActivityCommand{
		WorkflowRunID: base.WorkflowRunID, NodeRunID: uuid.NewString(), NodeID: "visual-foundation",
		Executor: "activity.resolve_visual_foundation", Attempt: 1,
	}
	base.IdempotencyKey = "execute-visual-foundation"
	base.Input, base.InputHash = visualInput, visualInputHash
	base.OutputPorts = []authoring.PortDefinition{{Key: "candidate", ValueType: "visual_foundation_candidate", Required: true}}
	result, err := executor.Execute(context.Background(), base)
	if err != nil || result.Status != "SUCCEEDED" || len(result.Output.Bindings) != 1 ||
		result.Output.Bindings[0].ValueType != "visual_foundation_candidate" ||
		selections.exactCalls != 1 || visualOwner.calls != 1 {
		t.Fatalf("execute exact Visual Foundation input: result=%#v exact=%d calls=%d err=%v", result, selections.exactCalls, visualOwner.calls, err)
	}
	wantInput, _, err := workflowapp.CompileFaithfulVisualFoundationInput(workflowapp.FaithfulVisualFoundationInputCommand{
		World: world, Source: confirmedSource, Selection: selection, PresetRelease: release,
	})
	if err != nil || visualOwner.command.Input.ProductionWorldOwnerSetHash != wantInput.ProductionWorldOwnerSetHash ||
		visualOwner.command.Input.PresetRelease != wantInput.PresetRelease || len(visualOwner.command.MediaAttachments) != 0 {
		t.Fatalf("Visual Foundation execution input=%#v want=%#v err=%v", visualOwner.command.Input, wantInput, err)
	}
}
