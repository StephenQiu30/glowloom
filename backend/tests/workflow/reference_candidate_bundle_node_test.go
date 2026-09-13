package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	genapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	production "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	flow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
	"github.com/google/uuid"
)

type bundleNodeOwner struct {
	calls int
	mode  string
}

func (owner *bundleNodeOwner) Materialize(ctx context.Context, _ genapp.Actor, command genapp.ReferenceCandidateBundleCommand) (gen.ReferenceCandidateBundle, error) {
	owner.calls++
	if owner.mode == "owner_error" {
		return gen.ReferenceCandidateBundle{}, errors.New("Owner commit failed")
	}
	if err := ctx.Err(); err != nil {
		return gen.ReferenceCandidateBundle{}, err
	}
	v := gen.ReferenceCandidateBundle{ID: uuid.NewString(), WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, ExecutionRef: command.ExecutionRef, VisionReviewRef: command.VisionReviewRef, ContentHash: strings.Repeat("a", 64)}
	if owner.mode == "owner_scope" {
		v.ProjectID = uuid.NewString()
	}
	return v, nil
}

func TestReferenceCandidateBundleNodeRequiresExactUpstreamReview(t *testing.T) {
	for _, mode := range []string{"success", "config", "binding", "review_revision", "execution_revision", "port", "input_hash", "owner_error", "owner_scope", "cancelled", "missing_owner"} {
		t.Run(mode, func(t *testing.T) {
			owner := &bundleNodeOwner{mode: mode}
			dependencies := production.SceneAnalysisDependencies{ReferenceBundles: owner}
			if mode == "missing_owner" {
				dependencies.ReferenceBundles = nil
			}
			executor := production.NewNodeExecutor(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, dependencies)
			input := flow.NodeInputSnapshot{SchemaVersion: flow.NodeInputSchemaVersion, Config: json.RawMessage(`{}`), Bindings: []flow.NodeInputBinding{{Port: "review", ValueType: "vision_review_candidate", SourceKind: flow.NodeInputSourceNodeOutput, SourceNodeID: "review", SourcePort: "candidate", ReferenceID: uuid.NewString(), ReferenceVersion: "1", ContentHash: strings.Repeat("a", 64)}}, FrozenInputs: []authoring.FrozenReference{{Kind: "reference_execution", ID: uuid.NewString(), Version: "1", Hash: strings.Repeat("b", 64)}}}
			switch mode {
			case "config":
				input.Config = json.RawMessage(`{"approve":true}`)
			case "binding":
				input.Bindings[0].ValueType = "reference_brief_candidate"
			case "review_revision":
				input.Bindings[0].ReferenceVersion = "2"
			case "execution_revision":
				input.FrozenInputs[0].Version = "2"
			}
			input, _, hash, err := flow.BuildNodeInput(input)
			if err != nil {
				t.Fatal(err)
			}
			command := flow.NodeExecutorCommand{NodeActivityCommand: flow.NodeActivityCommand{WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), Executor: "activity.materialize_reference_candidate_bundle", Attempt: 1}, WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(), InitiatorUserID: uuid.NewString(), InitiatorTokenVersion: 1, Input: input, InputHash: hash, OutputPorts: []authoring.PortDefinition{{Key: "bundle", ValueType: "reference_candidate_bundle", Required: true}}}
			command.IdempotencyKey = "bundle-node"
			if mode == "port" {
				command.OutputPorts[0].Key = "asset"
			}
			if mode == "input_hash" {
				command.InputHash = strings.Repeat("c", 64)
			}
			ctx := context.Background()
			if mode == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			value, err := executor.Execute(ctx, command)
			if mode == "success" {
				if err != nil || value.Status != "SUCCEEDED" || len(value.Output.Bindings) != 1 || owner.calls != 1 {
					t.Fatalf("Bundle output: %+v %v", value, err)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid Bundle node succeeded")
			}
			if mode != "owner_error" && mode != "owner_scope" && mode != "cancelled" && owner.calls != 0 {
				t.Fatal("invalid input reached Owner")
			}
		})
	}
}
