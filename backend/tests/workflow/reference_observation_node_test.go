package workflow_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	app "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	adapter "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/generation"
	flow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type referenceObservationOwner struct {
	*referenceCallNodeOwner
	call        gen.ReferenceProviderCall
	reads       int
	observation func(gen.ReferenceCallState) gen.ReferenceCallState
}

func (owner *referenceObservationOwner) Observe(_ context.Context, _ app.Actor, command app.ClaimReferenceCallCommand) (gen.ReferenceCallState, error) {
	owner.reads++
	if owner.observation != nil {
		return owner.observation(owner.state), nil
	}
	return owner.state, owner.err
}
func (owner *referenceObservationOwner) Get(_ context.Context, _ app.Actor, _, _ string) (gen.ReferenceJobProgress, error) {
	job, calls, err := gen.BuildReferenceProviderJob(owner.call.ExecutionRef, []gen.ReferenceProviderCallInput{owner.call.ReferenceProviderCallInput})
	if err != nil {
		return gen.ReferenceJobProgress{}, err
	}
	return gen.BuildReferenceJobProgress(job, calls, []gen.ReferenceCallState{owner.state})
}

func TestReferenceObservationNodesRetainExplicitFailureAndStopUnknown(t *testing.T) {
	for _, mode := range []string{"success", "explicit_failure", "media_rejected", "media_error", "dispatching", "unknown", "expired", "wrong_frozen", "wrong_output", "extra_config"} {
		t.Run(mode, func(t *testing.T) {
			command, state, receipt := referenceCallNodeFixture(t)
			owner := &referenceObservationOwner{referenceCallNodeOwner: &referenceCallNodeOwner{state: state}, call: receipt.Call}
			if mode == "explicit_failure" {
				input := receipt.ReferenceCallReceiptInput
				input.Output = nil
				input.Usage = gen.ProviderUsageObservation{}
				input.Disposition, input.ReasonCode = "output_rejected", "invalid_png_contents"
				var err error
				receipt, err = gen.BuildReferenceCallReceipt(input)
				if err != nil {
					t.Fatal(err)
				}
			}
			if mode == "unknown" {
				owner.state, _, _ = gen.ExpireReferenceCall(state, state.Dispatch.DeadlineAt)
			} else if mode != "dispatching" && mode != "expired" {
				owner.state, _, _ = gen.RecordReferenceCallReceipt(state, receipt)
			}
			if mode == "expired" {
				owner.expire = func(state gen.ReferenceCallState) gen.ReferenceCallState {
					expired, _, _ := gen.ExpireReferenceCall(state, state.Dispatch.DeadlineAt)
					return expired
				}
			}
			if mode == "media_rejected" {
				owner.mediaFailure = "rejected"
			}
			if mode == "media_error" {
				owner.mediaFailure = "error"
			}
			progress, err := owner.Get(context.Background(), app.Actor{}, "", "")
			if err != nil {
				t.Fatal(err)
			}
			config := map[string]any{"execution_ref": receipt.Call.ExecutionRef, "job_hash": progress.JobHash, "call_key": receipt.Call.CallKey, "previous_call_key": ""}
			if mode == "extra_config" {
				config["should_dispatch"] = true
			}
			raw, _ := json.Marshal(config)
			command.Executor = "activity.reference_call_observation"
			command.Input.Config = raw
			command.Input.FrozenInputs = []authoring.FrozenReference{{Kind: "reference_execution", ID: receipt.Call.ExecutionRef.ID, Version: "1", Hash: receipt.Call.ExecutionRef.ContentHash}}
			if mode == "wrong_frozen" {
				command.Input.FrozenInputs[0].Hash = strings.Repeat("f", 64)
			}
			if mode == "wrong_output" {
				command.OutputPorts[0].ValueType = "asset_version"
			}
			command.Input, _, command.InputHash, err = flow.BuildNodeInput(command.Input)
			if err != nil {
				t.Fatal(err)
			}
			executor, err := adapter.NewReferenceObservationNodeExecutor(owner, owner, owner, owner)
			if err != nil {
				t.Fatal(err)
			}
			result, err := executor.Execute(context.Background(), command)
			switch mode {
			case "media_error", "wrong_frozen", "wrong_output", "extra_config":
				if err == nil || result.Status != "" {
					t.Fatal("unsafe observation accepted")
				}
				if strings.Contains(err.Error(), "credential") {
					t.Fatal("private error leaked")
				}
			case "dispatching":
				if err != nil || result.Status != "RETRYING" {
					t.Fatalf("dispatch wait: %+v %v", result, err)
				}
			case "unknown", "expired":
				if err != nil || result.Status != flow.NodeActivityNeedsAttention || len(result.Output.Bindings) != 0 {
					t.Fatalf("unknown lost: %+v %v", result, err)
				}
			default:
				if err != nil || result.Status != "SUCCEEDED" || result.Output.Bindings[0].ContentHash != receipt.ContentHash {
					t.Fatalf("explicit observation: %+v %v", result, err)
				}
				// The same frozen receipt, including a rejection, advances the summary.
				command.Executor = "activity.reference_execution_observation"
				command.OutputPorts = []authoring.PortDefinition{{Key: "execution", ValueType: "reference_execution_observation", Required: true}}
				config = map[string]any{"execution_ref": receipt.Call.ExecutionRef, "job_hash": progress.JobHash, "previous_call_key": receipt.Call.CallKey}
				command.Input.Config, _ = json.Marshal(config)
				command.Input.Bindings = []flow.NodeInputBinding{{Port: "previous", ValueType: "reference_call_receipt", SourceKind: flow.NodeInputSourceNodeOutput, SourceNodeID: "reference-call-00", SourcePort: "receipt", ReferenceID: receipt.SubmissionToken, ReferenceVersion: "1", ContentHash: receipt.ContentHash}}
				command.Input, _, command.InputHash, err = flow.BuildNodeInput(command.Input)
				if err != nil {
					t.Fatal(err)
				}
				summary, err := executor.Execute(context.Background(), command)
				if err != nil || summary.Status != "SUCCEEDED" || summary.Output.Bindings[0].ContentHash != progress.ContentHash || owner.executions != 1 {
					t.Fatalf("summary resent or changed observation: %+v %v sends=%d", summary, err, owner.executions)
				}
				if mode == "success" {
					owner.observation = func(current gen.ReferenceCallState) gen.ReferenceCallState {
						if owner.reads%2 == 0 {
							return state
						}
						return current
					}
					owner.reads = 0
					if _, err := executor.Execute(context.Background(), command); err == nil {
						t.Fatal("summary accepted a state outside the observed terminal snapshot")
					}
					owner.observation = nil
				}
				command.Input.Bindings[0].ContentHash = strings.Repeat("f", 64)
				command.Input, _, command.InputHash, err = flow.BuildNodeInput(command.Input)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := executor.Execute(context.Background(), command); err == nil {
					t.Fatal("foreign predecessor receipt accepted")
				}
			}
		})
	}
}
