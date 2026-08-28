package workflow_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type storyReviewOwnerStub struct {
	state   bibleapp.StoryReviewState
	command bibleapp.StoryReviewCommand
}

func (owner *storyReviewOwnerStub) EnsureStoryReview(
	_ context.Context,
	command bibleapp.StoryReviewCommand,
) (bibleapp.StoryReviewState, error) {
	owner.command = command
	return owner.state, nil
}

func TestStoryReviewNodeWaitsForBoundedClosureAndNeverReturnsBudgetExhaustionAsSuccess(t *testing.T) {
	rootID := uuid.NewString()
	rootHash := strings.Repeat("a", 64)
	repairedID := uuid.NewString()
	repairedHash := strings.Repeat("b", 64)
	owner := &storyReviewOwnerStub{state: bibleapp.StoryReviewState{Status: "pending"}}
	executor := workflowproduction.NewNodeExecutor(nil, nil, nil, owner, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	input, _, inputHash, err := workflow.BuildNodeInput(workflow.NodeInputSnapshot{
		SchemaVersion: workflow.NodeInputSchemaVersion,
		Config:        json.RawMessage(`{"max_repair_rounds":2}`),
		Bindings: []workflow.NodeInputBinding{{
			Port: "candidate", ValueType: "story_reconciliation_candidate",
			SourceKind: workflow.NodeInputSourceNodeOutput, SourceNodeID: "story", SourcePort: "candidate",
			ReferenceID: rootID, ReferenceVersion: "1", ContentHash: rootHash,
		}},
		FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: uuid.NewString(), Version: "1", Hash: strings.Repeat("c", 64),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := workflow.NodeExecutorCommand{
		NodeActivityCommand: workflow.NodeActivityCommand{
			WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), NodeID: "review",
			Executor: "activity.story_review", Attempt: 1,
		},
		WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(),
		InitiatorUserID: uuid.NewString(), InitiatorTokenVersion: 1,
		IdempotencyKey: "story-review-node", Input: input, InputHash: inputHash,
		OutputPorts: []authoring.PortDefinition{{
			Key: "candidate", ValueType: "story_reconciliation_candidate", Required: true,
		}},
	}
	pending, err := executor.Execute(context.Background(), command)
	if err != nil || pending.Status != "RETRYING" || pending.Output.SchemaVersion != "" {
		t.Fatalf("pending Story Review result=%#v err=%v", pending, err)
	}
	if owner.command.CandidateRevisionID != rootID || owner.command.CandidateRevisionHash != rootHash ||
		owner.command.MaxRepairRounds != 2 || owner.command.Actor.UserID != command.InitiatorUserID {
		t.Fatalf("Story Review command lost its frozen input: %#v", owner.command)
	}

	owner.state = bibleapp.StoryReviewState{
		Status: "ready", CandidateRevisionID: repairedID,
		CandidateRevisionHash: repairedHash, CandidateRevisionNo: 2,
	}
	ready, err := executor.Execute(context.Background(), command)
	if err != nil || ready.Status != "SUCCEEDED" || len(ready.Output.Bindings) != 1 {
		t.Fatalf("ready Story Review result=%#v err=%v", ready, err)
	}
	if binding := ready.Output.Bindings[0]; binding.ReferenceID != repairedID ||
		binding.ReferenceVersion != "2" || binding.ContentHash != repairedHash {
		t.Fatalf("Story Review output did not publish the clean repaired revision: %#v", binding)
	}

	owner.state = bibleapp.StoryReviewState{Status: "needs_review", FailureCode: "repair_budget_exhausted"}
	failed, err := executor.Execute(context.Background(), command)
	if err == nil || failed.Status != "" || failed.Output.SchemaVersion != "" {
		t.Fatalf("budget exhaustion became a successful node output: result=%#v err=%v", failed, err)
	}
}
