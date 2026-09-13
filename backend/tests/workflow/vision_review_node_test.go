package workflow_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	genapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	flow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
	"github.com/google/uuid"
)

func TestVisionReviewNodeFreezesExecutionAndOnlyReturnsCandidateReference(t *testing.T) {
	raw, err := os.ReadFile("../agent/testdata/vision_review_input.json")
	if err != nil {
		t.Fatal(err)
	}
	input, err := contract.DecodeVisionReviewInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"accepted", "running", "rejected", "outcome_unknown", "missing_index", "wrong_freeze", "wrong_scope", "wrong_port", "wrong_candidate", "missing_dependency"} {
		t.Run(mode, func(t *testing.T) {
			source := &visionNodeOwner{input: input, status: "accepted"}
			dependencies := &workflowproduction.VisionReviewDependencies{Inputs: source, Execution: source, StageReleaseHash: input.Subject.StageReleaseHash}
			if mode == "missing_dependency" {
				dependencies.Execution = nil
			}
			executor := workflowproduction.NewNodeExecutor(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, workflowproduction.SceneAnalysisDependencies{VisionReview: dependencies})
			config := map[string]any{"execution_ref": input.Subject.ExecutionRef, "bundle_index": input.Subject.CandidateBundleIndex}
			if mode == "missing_index" {
				delete(config, "bundle_index")
			}
			encoded, _ := json.Marshal(config)
			nodeInput := flow.NodeInputSnapshot{SchemaVersion: flow.NodeInputSchemaVersion, Config: encoded, FrozenInputs: []authoring.FrozenReference{{Kind: "reference_execution", ID: input.Subject.ExecutionRef.ID, Version: "1", Hash: input.Subject.ExecutionRef.ContentHash}}}
			if mode == "wrong_freeze" {
				nodeInput.FrozenInputs[0].Hash = strings.Repeat("f", 64)
			}
			nodeInput, _, hash, err := flow.BuildNodeInput(nodeInput)
			if err != nil {
				t.Fatal(err)
			}
			command := flow.NodeExecutorCommand{NodeActivityCommand: flow.NodeActivityCommand{WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), Executor: "activity.review_reference_artifact", Attempt: 1}, WorkspaceID: input.Subject.WorkspaceID, ProjectID: input.Subject.ProjectID, InitiatorUserID: uuid.NewString(), InitiatorTokenVersion: 1, Input: nodeInput, InputHash: hash, OutputPorts: []authoring.PortDefinition{{Key: "candidate", ValueType: "vision_review_candidate", Required: true}}}
			command.IdempotencyKey = "vision-node"
			switch mode {
			case "running", "rejected", "outcome_unknown":
				source.status = mode
			case "wrong_scope":
				command.ProjectID = uuid.NewString()
			case "wrong_port":
				command.OutputPorts[0].ValueType = "reference_brief_candidate"
			case "wrong_candidate":
				source.badCandidate = true
			}
			got, err := executor.Execute(context.Background(), command)
			switch mode {
			case "accepted":
				if err != nil || got.Status != "SUCCEEDED" || len(got.Output.Bindings) != 1 || got.Output.Bindings[0].ReferenceID != source.candidate.ID {
					t.Fatalf("Candidate output: %+v %v", got, err)
				}
			case "running":
				if err != nil || got.Status != "RETRYING" {
					t.Fatalf("inflight result: %+v %v", got, err)
				}
			case "rejected", "outcome_unknown":
				if err != nil || got.Status != flow.NodeActivityNeedsAttention || !flow.ValidNodeAttentionReason(got.ErrorCode, got.NextAction) {
					t.Fatalf("unconfirmed result: %+v %v", got, err)
				}
			default:
				if err == nil {
					t.Fatal("invalid node accepted")
				}
				if mode != "wrong_candidate" && source.calls != 0 {
					t.Fatal("invalid node invoked Owner")
				}
			}
		})
	}
	if flow.ValidNodeAttentionReason(flow.VisionReviewUnconfirmedErrorCode, flow.ManualProviderReconciliationNextAction) || flow.ValidNodeAttentionReason("unexpected", flow.ManualVisionReviewNextAction) {
		t.Fatal("attention reason accepts unrelated recovery")
	}
}

type visionNodeOwner struct {
	input        contract.VisionReviewInput
	status       string
	calls        int
	badCandidate bool
	candidate    agentapp.Candidate
}

func (s *visionNodeOwner) CompileBaseVisionReviewInput(context.Context, genapp.Actor, string, string, int, string) (contract.VisionReviewInput, error) {
	return s.input, nil
}
func (s *visionNodeOwner) Execute(_ context.Context, command agentapp.ExecuteVisionReviewCommand) (agentapp.VisionReviewExecutionState, error) {
	s.calls++
	s.candidate = agentapp.Candidate{ID: uuid.NewString(), WorkspaceID: command.Input.Subject.WorkspaceID, ProjectID: command.Input.Subject.ProjectID, StageKey: contract.VisionReviewStageKey, CandidateType: "vision_review_candidate", Revision: 1, CandidateRevisionHash: strings.Repeat("b", 64)}
	if s.badCandidate {
		s.candidate.ProjectID = uuid.NewString()
	}
	return agentapp.VisionReviewExecutionState{Status: s.status, Candidate: s.candidate}, nil
}
