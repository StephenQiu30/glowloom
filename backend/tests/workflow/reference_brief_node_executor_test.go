package workflow_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type fixedReferenceBriefBatchSource struct {
	inputs  []agentcontract.ReferenceBriefInput
	receipt string
	hash    string
	calls   int
}

func (source *fixedReferenceBriefBatchSource) CompileBaseReferenceBriefInputs(
	_ context.Context,
	_, _, receiptID, receiptHash string,
	_ agentcontract.ReferenceBriefStageRelease,
) ([]agentcontract.ReferenceBriefInput, error) {
	source.calls++
	source.receipt, source.hash = receiptID, receiptHash
	return append([]agentcontract.ReferenceBriefInput(nil), source.inputs...), nil
}

type recordingReferenceBriefBatchOwner struct {
	commands []agentapp.ExecuteReferenceBriefCommand
}

func (owner *recordingReferenceBriefBatchOwner) ExecuteBatch(
	_ context.Context,
	workflowRunID string,
	nodeRunID string,
	inputs []agentcontract.ReferenceBriefInput,
) ([]agentapp.Candidate, error) {
	candidates := make([]agentapp.Candidate, len(inputs))
	for index, input := range inputs {
		owner.commands = append(owner.commands, agentapp.ExecuteReferenceBriefCommand{
			WorkflowRunID: workflowRunID, NodeRunID: nodeRunID, Input: input,
		})
		candidates[index] = agentapp.Candidate{
			ID: uuid.NewString(), WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID,
			StageKey: agentcontract.ReferenceBriefStageKey, Revision: 1,
			CandidateType: "reference_brief_candidate", CandidateRevisionHash: sceneTextHash(input.TargetBusinessKey),
		}
	}
	return candidates, nil
}

func TestReferenceBriefWorkflowNodeExecutesCanonicalBaseWaveAndPassesOwnerSet(t *testing.T) {
	workspaceID, projectID := uuid.NewString(), uuid.NewString()
	stageRelease := agentcontract.ReferenceBriefStageRelease{
		StageKey: agentcontract.ReferenceBriefStageKey, StageReleaseHash: sceneTextHash("reference-brief-release"),
	}
	source := &fixedReferenceBriefBatchSource{inputs: []agentcontract.ReferenceBriefInput{
		{WorkspaceID: workspaceID, ProjectID: projectID, TargetBusinessKey: "character_identity_anchor:hero", StageRelease: stageRelease},
		{WorkspaceID: workspaceID, ProjectID: projectID, TargetBusinessKey: "location_board:home", StageRelease: stageRelease},
	}}
	owner := &recordingReferenceBriefBatchOwner{}
	executor := workflowproduction.NewNodeExecutor(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		workflowproduction.SceneAnalysisDependencies{
			ReferenceBrief: &workflowproduction.ReferenceBriefDependencies{
				Inputs: source, Candidates: owner, StageRelease: stageRelease,
			},
		},
	)
	receiptID, receiptHash := uuid.NewString(), sceneTextHash("gate-three-owner-receipt")
	nodeInput, _, inputHash, err := workflow.BuildNodeInput(workflow.NodeInputSnapshot{
		SchemaVersion: workflow.NodeInputSchemaVersion, Config: json.RawMessage(`{}`),
		Bindings: []workflow.NodeInputBinding{{
			Port: "owners", ValueType: "visual_reference_owner_set", SourceKind: workflow.NodeInputSourceNodeOutput,
			SourceNodeID: "visual-foundation-scope-gate", SourcePort: "owners",
			ReferenceID: receiptID, ReferenceVersion: "1", ContentHash: receiptHash,
		}},
		FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: uuid.NewString(), Version: "1", Hash: sceneTextHash("script"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	workflowRunID, nodeRunID := uuid.NewString(), uuid.NewString()
	result, err := executor.Execute(context.Background(), workflow.NodeExecutorCommand{
		NodeActivityCommand: workflow.NodeActivityCommand{
			WorkflowRunID: workflowRunID, NodeRunID: nodeRunID, NodeID: "reference-brief-base-wave",
			Executor: "activity.compile_reference_briefs", Attempt: 1,
		},
		WorkspaceID: workspaceID, ProjectID: projectID,
		InitiatorUserID: uuid.NewString(), InitiatorTokenVersion: 1,
		IdempotencyKey: "execute-reference-brief-base-wave", Input: nodeInput, InputHash: inputHash,
		OutputPorts: []authoring.PortDefinition{{Key: "owners", ValueType: "visual_reference_owner_set", Required: true}},
	})
	if err != nil || result.Status != "SUCCEEDED" || source.calls != 1 ||
		source.receipt != receiptID || source.hash != receiptHash || len(owner.commands) != 2 ||
		owner.commands[0].Input.TargetBusinessKey != "character_identity_anchor:hero" ||
		owner.commands[1].Input.TargetBusinessKey != "location_board:home" ||
		owner.commands[0].WorkflowRunID != workflowRunID || owner.commands[0].NodeRunID != nodeRunID ||
		len(result.Output.Bindings) != 1 || result.Output.Bindings[0].Port != "owners" ||
		result.Output.Bindings[0].ReferenceID != receiptID || result.Output.Bindings[0].ContentHash != receiptHash {
		t.Fatalf("execute Reference Brief base wave: result=%#v source=%#v commands=%#v err=%v", result, source, owner.commands, err)
	}
}
