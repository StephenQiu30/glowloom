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
	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
	storygraphdomain "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type referencePlanWorldSource struct {
	world storygraphdomain.ReferencePlanWorldReadSet
}

func (source referencePlanWorldSource) ReferencePlanWorld(
	_ context.Context,
	_ storygraphapp.Actor,
	_ string,
) (storygraphdomain.ReferencePlanWorldReadSet, error) {
	return source.world, nil
}

type referencePlanVisualSource struct {
	revision workflowapp.ReferencePlanVisualFoundationCandidateRevision
	calls    int
}

func (source *referencePlanVisualSource) ExactVisualFoundationCandidate(
	_ context.Context,
	_, _, candidateID, revisionHash string,
) (workflowapp.ReferencePlanVisualFoundationCandidateRevision, error) {
	source.calls++
	if candidateID != source.revision.ID || revisionHash != source.revision.RevisionHash {
		return workflowapp.ReferencePlanVisualFoundationCandidateRevision{}, agentapp.ErrNotFound
	}
	return source.revision, nil
}

type recordingReferencePlanOwner struct {
	command agentapp.ExecuteReferencePlanCommand
	calls   int
}

func (owner *recordingReferencePlanOwner) Execute(
	_ context.Context,
	command agentapp.ExecuteReferencePlanCommand,
) (agentapp.Candidate, error) {
	owner.calls++
	owner.command = command
	return agentapp.Candidate{
		ID: uuid.NewString(), WorkspaceID: command.Input.WorkspaceID, ProjectID: command.Input.ProjectID,
		StageKey: agentcontract.ReferencePlanStageKey, Revision: 1,
		CandidateType: "reference_plan_candidate", CandidateRevisionHash: sceneTextHash("reference-plan-candidate"),
	}, nil
}

func TestReferencePlanWorkflowNodeCompilesExactFrozenInput(t *testing.T) {
	release := curatedFaithfulRelease(t)
	inventory := referencePlanInventoryFixture()
	revision := referencePlanVisualFoundationRevision(t, inventory, release.ContentHash)
	workspaceID := inventory.CharacterSeeds[0].IdentityRef.WorkspaceID
	projectID := inventory.CharacterSeeds[0].IdentityRef.ProjectID
	world := storygraphdomain.ReferencePlanWorldReadSet{
		WorkspaceID: workspaceID, ProjectID: projectID,
		StoryGraphVersionID: uuid.NewString(), StoryGraphContentHash: sceneTextHash("reference-plan-storygraph"),
		Inventory: inventory,
	}
	selection := frozenProjectSelection(t, workspaceID, projectID, release)
	selections := &visualFoundationSelectionSource{exact: selection}
	visualSource := &referencePlanVisualSource{revision: revision}
	owner := &recordingReferencePlanOwner{}
	executor := workflowproduction.NewNodeExecutor(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		workflowproduction.SceneAnalysisDependencies{
			ReferencePlan: &workflowproduction.ReferencePlanDependencies{
				Selections: selections,
				FindRelease: func(key, version string) (presetdomain.Release, bool, error) {
					return release, key == release.Key && version == release.Release, nil
				},
				Worlds: referencePlanWorldSource{world: world}, Sources: visualSource, Candidates: owner,
			},
		},
	)
	frozen := []authoring.FrozenReference{{
		Kind: "script_revision", ID: uuid.NewString(), Version: "1", Hash: sceneTextHash("script"),
	}}
	nodeInput, _, inputHash, err := workflow.BuildNodeInput(workflow.NodeInputSnapshot{
		SchemaVersion: workflow.NodeInputSchemaVersion, Config: json.RawMessage(`{}`),
		Bindings: []workflow.NodeInputBinding{
			{
				Port: "storygraph", ValueType: "storygraph_version", SourceKind: workflow.NodeInputSourceNodeOutput,
				SourceNodeID: "production-storygraph", SourcePort: "storygraph", ReferenceID: world.StoryGraphVersionID,
				ReferenceVersion: "1", ContentHash: world.StoryGraphContentHash,
			},
			{
				Port: "selection", ValueType: "project_preset_selection", SourceKind: workflow.NodeInputSourceNodeOutput,
				SourceNodeID: "preset-selection", SourcePort: "selection", ReferenceID: selection.ID,
				ReferenceVersion: "1", ContentHash: selection.ContentHash,
			},
			{
				Port: "visual_foundation", ValueType: "visual_foundation_candidate", SourceKind: workflow.NodeInputSourceNodeOutput,
				SourceNodeID: "visual-foundation", SourcePort: "candidate", ReferenceID: revision.ID,
				ReferenceVersion: "1", ContentHash: revision.RevisionHash,
			},
		},
		FrozenInputs: frozen,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := executor.Execute(context.Background(), workflow.NodeExecutorCommand{
		NodeActivityCommand: workflow.NodeActivityCommand{
			WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), NodeID: "reference-plan",
			Executor: "activity.plan_reference_assets", Attempt: 1,
		},
		WorkspaceID: workspaceID, ProjectID: projectID,
		InitiatorUserID: selection.SelectedBy, InitiatorTokenVersion: 1,
		IdempotencyKey: "execute-reference-plan", Input: nodeInput, InputHash: inputHash,
		OutputPorts: []authoring.PortDefinition{{Key: "candidate", ValueType: "reference_plan_candidate", Required: true}},
	})
	if err != nil || result.Status != "SUCCEEDED" || len(result.Output.Bindings) != 1 ||
		result.Output.Bindings[0].ValueType != "reference_plan_candidate" ||
		selections.exactCalls != 1 || visualSource.calls != 1 || owner.calls != 1 {
		t.Fatalf("execute exact Reference Plan input: result=%#v calls=%d/%d/%d err=%v", result, selections.exactCalls, visualSource.calls, owner.calls, err)
	}
	want, _, err := workflowapp.CompileReferencePlanInput(workflowapp.ReferencePlanInputCommand{
		Inventory: inventory, VisualFoundationRevision: revision, PresetRelease: release,
	})
	if err != nil || owner.command.Input.ReferenceTargetSeedRoot != want.ReferenceTargetSeedRoot ||
		owner.command.Input.VisualFoundationCandidateRevisionHash != revision.RevisionHash {
		t.Fatalf("Reference Plan execution input=%#v want=%#v err=%v", owner.command.Input, want, err)
	}
}
