package microservices_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	creationapp "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	executionrpc "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/creationhandoff"
	executionapp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationhandoff"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

type compilationWireServer struct {
	pb.UnimplementedFrozenInputsServer
	request  *pb.CompileFrozenInputRequest
	response *pb.FrozenInputCompilation
	failure  error
	calls    int
}

func (s *compilationWireServer) CompileFrozenInput(ctx context.Context, request *pb.CompileFrozenInputRequest) (*pb.FrozenInputCompilation, error) {
	s.calls++
	if !proto.Equal(request, s.request) {
		return nil, status.Error(codes.FailedPrecondition, "changed compilation request")
	}
	md, _ := metadata.FromIncomingContext(ctx)
	if values := md.Get("authorization"); len(values) != 1 || values[0] != "Bearer "+frozenExecutionToken {
		return nil, status.Error(codes.Unauthenticated, "source token required")
	}
	for _, key := range []string{"cookie", "baggage", "x-actor-id"} {
		if len(md.Get(key)) != 0 {
			return nil, status.Error(codes.InvalidArgument, "private metadata forwarded")
		}
	}
	return s.response, s.failure
}

func compilationConsumerFixture(t *testing.T) (*executionrpc.SourceClient, executionapp.CompileFrozenInputRequest, *compilationWireServer) {
	t.Helper()
	request := executionapp.CompileFrozenInputRequest{Key: executionapp.FrozenInputKey{HandoffID: uuid.New(), CommandID: uuid.New(), CommandSHA256: strings.Repeat("a", 64), SnapshotSHA256: strings.Repeat("b", 64)}, CatalogContext: executionapp.FrozenCatalogContext{ModelVersionID: uuid.New(), ModelKey: "selected-model", Capability: "image.generate", Mode: "text_to_image"}}
	contextHash, err := request.CatalogContext.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	producer := creationapp.FrozenInputCompilation{Key: creationapp.FrozenInputKey{HandoffID: request.Key.HandoffID, CommandID: request.Key.CommandID, CommandSHA256: request.Key.CommandSHA256, SnapshotSHA256: request.Key.SnapshotSHA256}, CatalogContext: creationapp.FrozenCatalogContext{ModelVersionID: request.CatalogContext.ModelVersionID, ModelKey: request.CatalogContext.ModelKey, Capability: request.CatalogContext.Capability, Mode: request.CatalogContext.Mode}, ContextSHA256: contextHash, ModelVersionID: request.CatalogContext.ModelVersionID, Mode: request.CatalogContext.Mode, OriginalPrompt: "original prompt", FinalPrompt: "compiled prompt", ParamsJSON: json.RawMessage(`{"seed":9007199254740993}`), OutputCount: 1, Media: []creationapp.FrozenCompiledMedia{}, CompilerVersion: "prompt-reference-v1"}
	raw, err := producer.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &producer); err != nil {
		t.Fatal(err)
	}
	wireRequest := &pb.CompileFrozenInputRequest{Key: &pb.FrozenInputKey{HandoffId: request.Key.HandoffID[:], CommandId: request.Key.CommandID[:], CommandSha256: request.Key.CommandSHA256, SnapshotSha256: request.Key.SnapshotSHA256}, CatalogContext: &pb.FrozenCatalogContext{ModelVersionId: request.CatalogContext.ModelVersionID[:], ModelKey: request.CatalogContext.ModelKey, Capability: request.CatalogContext.Capability, Mode: request.CatalogContext.Mode}}
	wire := &pb.FrozenInputCompilation{Key: proto.Clone(wireRequest.Key).(*pb.FrozenInputKey), CatalogContext: proto.Clone(wireRequest.CatalogContext).(*pb.FrozenCatalogContext), ContextSha256: contextHash, CompilationSha256: producer.CompilationSHA256, ModelVersionId: producer.ModelVersionID[:], Mode: producer.Mode, OriginalPrompt: producer.OriginalPrompt, FinalPrompt: producer.FinalPrompt, ParamsJson: producer.ParamsJSON, OutputCount: 1, CompilerVersion: producer.CompilerVersion, CanonicalCompilationJson: raw}
	checked := &compilationWireServer{request: wireRequest, response: wire}
	serverTLS, clientTLS, _ := ownerPeerTLS(t, "execution")
	server := grpc.NewServer(grpc.Creds(serverTLS))
	pb.RegisterFrozenInputsServer(server, checked)
	client, err := executionrpc.NewSourceClient(executionrpc.Target{Address: serveFrozenRPC(t, server), Token: frozenExecutionToken, Transport: clientTLS})
	if err != nil {
		t.Fatal(err)
	}
	return client, request, checked
}

func TestFrozenCompilationConsumerValidatesWholeCanonicalProducerReceipt(t *testing.T) {
	client, request, checked := compilationConsumerFixture(t)
	// PostgreSQL JSONB may add whitespace without changing the exact numeric value.
	checked.response.ParamsJson = []byte(`{ "seed" : 9007199254740993 }`)
	ctx := metadata.NewOutgoingContext(t.Context(), metadata.Pairs("authorization", "Bearer browser", "cookie", "private=1", "baggage", "private=1", "x-actor-id", uuid.NewString()))
	got, err := client.CompileFrozenInput(ctx, request)
	if err != nil || got.Key != request.Key || got.CatalogContext != request.CatalogContext || got.FinalPrompt != "compiled prompt" || !strings.Contains(string(got.ParamsJSON), "9007199254740993") || checked.calls != 1 {
		t.Fatalf("compiled facts=%+v calls=%d err=%v", got, checked.calls, err)
	}
}
func TestFrozenCompilationConsumerRejectsRetargetingAndWireCanonicalDrift(t *testing.T) {
	for _, bad := range []string{"key", "catalog", "prompt", "params", "digest", "compiler", "unknown nested", "canonical extra"} {
		t.Run(bad, func(t *testing.T) {
			client, request, checked := compilationConsumerFixture(t)
			switch bad {
			case "key":
				checked.response.Key.SnapshotSha256 = strings.Repeat("f", 64)
			case "catalog":
				checked.response.CatalogContext.ModelKey = "reselected-model"
			case "prompt":
				checked.response.FinalPrompt = "changed prompt"
			case "params":
				checked.response.ParamsJson = []byte(`{"seed":9007199254740992}`)
			case "digest":
				checked.response.CompilationSha256 = strings.Repeat("f", 64)
			case "compiler":
				checked.response.CompilerVersion = "unsupported"
			case "unknown nested":
				checked.response.CatalogContext.ProtoReflect().SetUnknown([]byte{0x98, 0x06, 0x01})
			case "canonical extra":
				var content map[string]json.RawMessage
				if err := json.Unmarshal(checked.response.CanonicalCompilationJson, &content); err != nil {
					t.Fatal(err)
				}
				content["extra_authority"] = json.RawMessage(`true`)
				checked.response.CanonicalCompilationJson, _ = json.Marshal(content)
			}
			if _, err := client.CompileFrozenInput(t.Context(), request); err == nil {
				t.Fatal("changed compilation accepted", bad)
			}
		})
	}
}
func TestFrozenCompilationConsumerPreservesSourceFailureMeanings(t *testing.T) {
	for _, test := range []struct {
		reason   pb.FrozenInputErrorReason
		expected error
	}{
		{pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_TEMPLATE_INVALID, executionapp.ErrFrozenTemplateInvalid},
		{pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_TEMPLATE_CHANGED, executionapp.ErrFrozenTemplateChanged},
		{pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_TEMPLATE_CONTEXT_UNAVAILABLE, executionapp.ErrFrozenTemplateContextUnavailable},
		{pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_UNSUPPORTED_COMPILER, executionapp.ErrFrozenCompilerUnsupported},
		{pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_SOURCE_STALE, executionapp.ErrQuoteSourceStale},
		{pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_REFERENCE_UNAVAILABLE, executionapp.ErrFrozenReferenceUnavailable},
	} {
		t.Run(test.reason.String(), func(t *testing.T) {
			client, request, checked := compilationConsumerFixture(t)
			failure, err := status.New(codes.FailedPrecondition, "closed source failure").WithDetails(&pb.FrozenInputFailure{Reason: test.reason})
			if err != nil {
				t.Fatal(err)
			}
			checked.failure = failure.Err()
			if _, err := client.CompileFrozenInput(t.Context(), request); !errors.Is(err, test.expected) {
				t.Fatalf("meaning=%v want=%v", err, test.expected)
			}
		})
	}
	client, request, checked := compilationConsumerFixture(t)
	checked.failure = status.Error(codes.Unavailable, "unavailable")
	if _, err := client.CompileFrozenInput(t.Context(), request); !errors.Is(err, writefence.ErrUnavailable) {
		t.Fatalf("unavailable source became ready: %v", err)
	}
}
