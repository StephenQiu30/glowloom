package workflow_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	app "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	adapter "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/generation"
	flow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type referenceCallNodeOwner struct {
	state                   gen.ReferenceCallState
	err                     error
	executions, expirations int
	command                 app.ClaimReferenceCallCommand
	actor                   app.Actor
	expire                  func(gen.ReferenceCallState) gen.ReferenceCallState
	mediaFailure            string
}

func referenceNodePNG() ([]byte, error) {
	var contents bytes.Buffer
	err := png.Encode(&contents, image.NewRGBA(image.Rect(0, 0, 1024, 1024)))
	return contents.Bytes(), err
}

func (o *referenceCallNodeOwner) Materialize(_ context.Context, actor app.Actor, command app.MaterializeReferenceStagedMediaCommand) (gen.ReferenceStagedMedia, error) {
	if o.mediaFailure == "error" {
		return gen.ReferenceStagedMedia{}, errors.New("private staging credential")
	}
	if actor != o.actor || command.ReceiptRef.ContentHash != o.state.Receipt.ContentHash || command.ReceiptRef.ID != o.state.Receipt.SubmissionToken || command.CallKey != o.state.CallKey || command.ExecutionRef != o.command.ExecutionRef {
		return gen.ReferenceStagedMedia{}, errors.New("wrong media identity")
	}
	media, err := gen.NewReferenceStagedMedia(*o.state.Receipt, gen.ReferenceObjectStoreRef{Profile: "minio", Bucket: "lanverse", ObjectKey: o.state.Receipt.Output.StagingObjectKey})
	if err != nil {
		return media, err
	}
	contents, err := referenceNodePNG()
	if err != nil {
		return gen.ReferenceStagedMedia{}, err
	}
	if o.mediaFailure == "rejected" {
		contents = nil
	}
	return gen.CompleteReferenceStagedMedia(media, contents, media.CreatedAt.Add(time.Second))
}

func (o *referenceCallNodeOwner) Execute(_ context.Context, actor app.Actor, command app.ClaimReferenceCallCommand) (gen.ReferenceCallState, error) {
	o.executions++
	o.command, o.actor = command, actor
	return o.state, o.err
}
func (o *referenceCallNodeOwner) Expire(_ context.Context, _ app.Actor, command app.ExpireReferenceCallCommand) (gen.ReferenceCallState, error) {
	o.expirations++
	if command.SubmissionToken != o.state.Dispatch.SubmissionToken || command.CallKey != o.command.CallKey || command.ExecutionRef != o.command.ExecutionRef {
		return gen.ReferenceCallState{}, errors.New("foreign expiry")
	}
	if o.expire != nil {
		o.state = o.expire(o.state)
	}
	return o.state, nil
}

func referenceCallNodeFixture(t *testing.T) (flow.NodeExecutorCommand, gen.ReferenceCallState, gen.ReferenceCallReceipt) {
	t.Helper()
	ref := gen.GenerationRevisionRef{ID: uuid.NewString(), Revision: 1, ContentHash: strings.Repeat("a", 64)}
	_, calls, err := gen.BuildReferenceProviderJob(ref, []gen.ReferenceProviderCallInput{{SlotKey: "front", CompiledRequestHash: strings.Repeat("b", 64)}})
	if err != nil {
		t.Fatal(err)
	}
	command := flow.NodeExecutorCommand{NodeActivityCommand: flow.NodeActivityCommand{WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), NodeID: "reference-front", Executor: "activity.reference_image_call", Attempt: 1}, WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(), InitiatorUserID: uuid.NewString(), InitiatorTokenVersion: 1, IdempotencyKey: "reference-node", OutputPorts: []authoring.PortDefinition{{Key: "receipt", ValueType: "reference_call_receipt", Required: true}}}
	config, _ := json.Marshal(map[string]any{"execution_ref": ref, "call_key": calls[0].CallKey})
	command.Input, _, command.InputHash, err = flow.BuildNodeInput(flow.NodeInputSnapshot{SchemaVersion: flow.NodeInputSchemaVersion, Config: config, FrozenInputs: []authoring.FrozenReference{{Kind: "script_revision", ID: uuid.NewString(), Version: "1", Hash: ref.ContentHash}}})
	if err != nil {
		t.Fatal(err)
	}
	pending, _ := gen.NewReferenceCallState(calls[0].CallKey)
	now := time.Now().UTC().Truncate(time.Microsecond)
	state, _, err := gen.ClaimReferenceCall(pending, gen.ReferenceCallDispatch{SubmissionToken: uuid.NewString(), DispatchedBy: command.InitiatorUserID, MembershipTokenVersion: 1, DispatchedAt: now, DeadlineAt: now.Add(180 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	slot := gen.ReferenceOutputSlot{SlotKey: "front", ViewRole: "front", Required: true, AllowedMediaTypes: []string{"image/png"}, AspectRatio: "1:1", MinWidth: 1024, MinHeight: 1024, MaxBytes: 10 << 20, SemanticRequirements: []string{"preserve identity"}, QCRubricRefs: []gen.ReferenceOutputQCRubricRef{{ContractID: "reference-qc", ContentHash: ref.ContentHash}}}
	contents, err := referenceNodePNG()
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents)
	receipt, err := gen.BuildReferenceCallReceipt(gen.ReferenceCallReceiptInput{WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, Call: calls[0], SubmissionToken: state.Dispatch.SubmissionToken, Slot: slot, ObservedAt: now.Add(time.Second), Disposition: "staged", Usage: gen.ProviderUsageObservation{ImageCount: 1}, Output: &gen.ProviderOutput{OutputKey: "image", StagingObjectKey: "staging/reference/" + command.WorkspaceID + "/" + command.ProjectID + "/" + ref.ID + "/" + calls[0].CallKey + "/" + state.Dispatch.SubmissionToken + "/image.png", SHA256: hex.EncodeToString(digest[:]), MediaType: "image/png", Bytes: int64(len(contents)), Width: 1024, Height: 1024}})
	if err != nil {
		t.Fatal(err)
	}
	return command, state, receipt
}

func TestReferenceCallNodeExecutesRecoversAndFailsClosed(t *testing.T) {
	for _, mode := range []string{"success", "dispatching", "expired", "unknown", "failed", "pending", "corrupt", "foreign", "error", "media_error", "media_rejected"} {
		t.Run(mode, func(t *testing.T) {
			command, state, receipt := referenceCallNodeFixture(t)
			owner := &referenceCallNodeOwner{state: state}
			switch mode {
			case "success", "foreign", "media_error", "media_rejected":
				owner.mediaFailure = strings.TrimPrefix(mode, "media_")
				if mode == "foreign" {
					receipt.WorkspaceID = uuid.NewString()
					receipt.Output.StagingObjectKey = "staging/reference/" + receipt.WorkspaceID + "/" + receipt.ProjectID + "/" + receipt.Call.ExecutionRef.ID + "/" + receipt.Call.CallKey + "/" + receipt.SubmissionToken + "/image.png"
					receipt, _ = gen.BuildReferenceCallReceipt(receipt.ReferenceCallReceiptInput)
				}
				owner.state, _, _ = gen.RecordReferenceCallReceipt(state, receipt)
			case "expired":
				owner.expire = func(s gen.ReferenceCallState) gen.ReferenceCallState {
					result, _, err := gen.ExpireReferenceCall(s, s.Dispatch.DeadlineAt)
					if err != nil {
						t.Fatal(err)
					}
					return result
				}
			case "unknown":
				owner.state, _, _ = gen.ExpireReferenceCall(state, state.Dispatch.DeadlineAt)
			case "failed":
				receipt.Disposition, receipt.ReasonCode, receipt.Output, receipt.Usage = "not_sent", "submit_not_attempted", nil, gen.ProviderUsageObservation{}
				receipt, _ = gen.BuildReferenceCallReceipt(receipt.ReferenceCallReceiptInput)
				owner.state, _, _ = gen.RecordReferenceCallReceipt(state, receipt)
			case "pending":
				owner.state, _ = gen.NewReferenceCallState(state.CallKey)
			case "corrupt":
				owner.state.ContentHash = strings.Repeat("f", 64)
			case "error":
				owner.err = errors.New("synthetic provider credential must not enter history")
			}
			executor, err := adapter.NewReferenceCallNodeExecutor(owner, owner, owner)
			if err != nil {
				t.Fatal(err)
			}
			result, err := executor.Execute(context.Background(), command)
			if owner.executions != 1 || owner.command.ExpectedRevision != 1 || owner.actor.UserID != command.InitiatorUserID {
				t.Fatal("owner execution boundary not forwarded")
			}
			switch mode {
			case "success":
				if err != nil || result.Status != "SUCCEEDED" || len(result.Output.Bindings) != 1 || result.Output.Bindings[0].ReferenceID != receipt.SubmissionToken || result.Output.Bindings[0].ContentHash != receipt.ContentHash {
					t.Fatalf("receipt output: %+v %v", result, err)
				}
			case "dispatching":
				if err != nil || result.Status != "RETRYING" || owner.expirations != 1 {
					t.Fatalf("poll: %+v %v", result, err)
				}
			case "expired", "unknown":
				if err != nil || result.Status != flow.NodeActivityNeedsAttention || result.NextAction != flow.ManualProviderReconciliationNextAction {
					t.Fatalf("attention: %+v %v", result, err)
				}
			default:
				if err == nil || result.Status != "" || strings.Contains(err.Error(), "credential") {
					t.Fatalf("failure leaked: %+v %v", result, err)
				}
			}
		})
	}
}

func TestReferenceCallNodeRejectsInvalidInputBeforeOwnerIO(t *testing.T) {
	for _, fault := range []string{"scope", "token_version", "executor", "hash", "output", "extra_config", "duplicate_config", "execution", "call", "null_config"} {
		t.Run(fault, func(t *testing.T) {
			command, state, _ := referenceCallNodeFixture(t)
			switch fault {
			case "scope":
				command.WorkspaceID = uuid.Nil.String()
			case "token_version":
				command.InitiatorTokenVersion = 0
			case "executor":
				command.Executor = "activity.reference_asset_generation"
			case "hash":
				command.InputHash = strings.Repeat("f", 64)
			case "output":
				command.OutputPorts[0].ValueType = "asset_version"
			case "extra_config":
				command.Input.Config = append(command.Input.Config[:len(command.Input.Config)-1], []byte(`,"submission_token":"not-a-permit"}`)...)
			case "duplicate_config":
				command.Input.Config = append(command.Input.Config[:len(command.Input.Config)-1], []byte(`,"call_key":"duplicate"}`)...)
			case "execution":
				command.Input.Config = json.RawMessage(`{"execution_ref":{"id":"invalid","revision":1,"content_hash":"bad"},"call_key":"bad"}`)
			case "call":
				command.Input.Config = json.RawMessage(`{"call_key":"invalid"}`)
			case "null_config":
				command.Input.Config = json.RawMessage(`null`)
			}
			owner := &referenceCallNodeOwner{state: state}
			executor, err := adapter.NewReferenceCallNodeExecutor(owner, owner, owner)
			if err != nil {
				t.Fatal(err)
			}
			if result, err := executor.Execute(context.Background(), command); err == nil || result.Status != "" || owner.executions != 0 || owner.expirations != 0 {
				t.Fatalf("%s reached owner: %+v %v", fault, result, err)
			}
		})
	}
	if _, err := adapter.NewReferenceCallNodeExecutor(nil, &referenceCallNodeOwner{}, &referenceCallNodeOwner{}); err == nil {
		t.Fatal("missing execution accepted")
	}
	if _, err := adapter.NewReferenceCallNodeExecutor(&referenceCallNodeOwner{}, nil, &referenceCallNodeOwner{}); err == nil {
		t.Fatal("missing recovery accepted")
	}
	if _, err := adapter.NewReferenceCallNodeExecutor(&referenceCallNodeOwner{}, &referenceCallNodeOwner{}, nil); err == nil {
		t.Fatal("missing media owner accepted")
	}
}

func TestReferenceCallCatalogCompilesExactConfigWithoutCache(t *testing.T) {
	command, _, _ := referenceCallNodeFixture(t)
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, definition := range catalog.Definitions {
		if definition.Key == "generation.reference_image_call" {
			found = true
			if definition.CachePolicy != "never" || definition.RiskLevel != "external_ai" || len(definition.InputPorts) != 0 || len(definition.OutputPorts) != 1 {
				t.Fatal("unsafe reference node catalog contract")
			}
		}
	}
	if !found {
		t.Fatal("reference call node not registered")
	}
	graph := authoring.Graph{Nodes: []authoring.Node{{ID: "reference", DefinitionKey: "generation.reference_image_call", DefinitionVersion: "1.0.0", Config: command.Input.Config}}}
	if _, err = authoring.ValidateGraph(graph, catalog); err != nil {
		t.Fatal(err)
	}
	graph.Nodes[0].Config = append(graph.Nodes[0].Config[:len(graph.Nodes[0].Config)-1], []byte(`,"provider":"unsafe"}`)...)
	if _, err = authoring.ValidateGraph(graph, catalog); err == nil {
		t.Fatal("free provider config accepted")
	}
}
