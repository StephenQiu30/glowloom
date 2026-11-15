package microservices_test

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"

	creationrpc "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/grpchandoff"
	creationapp "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	executionrpc "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/creationhandoff"
	executionpg "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"
	executionapp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationhandoff"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

const frozenExecutionToken = "test-execution-to-creation-input-token-20261115"
const frozenCreationToken = "test-creation-to-execution-input-token-20261115"

func serveFrozenRPC(t *testing.T, server *grpc.Server) string {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		if err := <-done; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Error(err)
		}
	})
	return listener.Addr().String()
}
func frozenWireKey(key executionapp.FrozenInputKey) *pb.FrozenInputKey {
	return &pb.FrozenInputKey{HandoffId: key.HandoffID[:], CommandId: key.CommandID[:], CommandSha256: key.CommandSHA256, SnapshotSha256: key.SnapshotSHA256}
}
func frozenRPCContext(t *testing.T, token string) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	return metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token)), cancel
}
func frozenRPCConnection(t *testing.T, address string) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(), grpc.WithDefaultCallOptions(grpc.MaxRetryRPCBufferSize(0)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	})
	return conn
}

type checkedFrozenSource struct {
	store creationrpc.FrozenInputs
	calls atomic.Int32
	bad   atomic.Bool
}

func (s *checkedFrozenSource) ReadFrozenInput(ctx context.Context, key creationapp.FrozenInputKey) (creationapp.FrozenInputEnvelope, error) {
	s.calls.Add(1)
	md, _ := metadata.FromIncomingContext(ctx)
	for key := range md {
		if key != "traceparent" && key != "tracestate" {
			s.bad.Store(true)
		}
	}
	return s.store.ReadFrozenInput(ctx, key)
}

type lostFrozenACK struct {
	store executionapp.FrozenInputAcceptance
	first atomic.Bool
	calls atomic.Int32
}

func (s *lostFrozenACK) AcceptFrozenInput(ctx context.Context, key executionapp.FrozenInputKey) (executionapp.FrozenInputReceipt, error) {
	s.calls.Add(1)
	receipt, err := s.store.AcceptFrozenInput(ctx, key)
	if err == nil && s.first.CompareAndSwap(false, true) {
		return executionapp.FrozenInputReceipt{}, status.Error(codes.Unavailable, "synthetic ACK loss after commit")
	}
	return receipt, err
}
func (s *lostFrozenACK) CancelFrozenInput(ctx context.Context, key executionapp.FrozenInputKey, command writefence.Command) (executionapp.FrozenInputCancelProof, error) {
	return s.store.CancelFrozenInput(ctx, key, command)
}

func TestFrozenInputRPCRealRestrictedPGLostACKAndCancel(t *testing.T) {
	f := newFrozenFixture(t)
	envelope := f.freeze(t)
	key := executionKey(envelope)
	// With one connection per owner, any RPC inside an owner Tx would deadlock.
	for _, db := range []*gorm.DB{f.creation, f.execution} {
		pool, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		pool.SetMaxOpenConns(1)
	}
	checked := &checkedFrozenSource{store: f.store}
	sourceServer, err := creationrpc.NewServer(checked, frozenExecutionToken, insecure.NewCredentials())
	if err != nil {
		t.Fatal(err)
	}
	sourceAddress := serveFrozenRPC(t, sourceServer)
	source, err := executionrpc.NewSourceClient(executionrpc.Target{Address: sourceAddress, Token: frozenExecutionToken, Transport: insecure.NewCredentials()})
	if err != nil {
		t.Fatal(err)
	}
	target := executionpg.NewFrozenInputStore(f.execution, source)
	ack := &lostFrozenACK{store: target}
	targetServer, err := executionrpc.NewConsumerServer(ack, frozenCreationToken, insecure.NewCredentials())
	if err != nil {
		t.Fatal(err)
	}
	targetAddress := serveFrozenRPC(t, targetServer)
	client := pb.NewFrozenInputConsumerClient(frozenRPCConnection(t, targetAddress))
	ctx, cancel := frozenRPCContext(t, frozenCreationToken)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "x-caller-principal", "forged", "baggage", "private=data", "traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	if _, err := client.AcceptFrozenInput(ctx, frozenWireKey(key)); status.Code(err) != codes.Unavailable {
		t.Fatalf("first accept must report unknown ACK: %v", err)
	}
	if ack.calls.Load() != 1 {
		t.Fatalf("side effect automatically replayed calls=%d", ack.calls.Load())
	}
	second, err := client.AcceptFrozenInput(ctx, frozenWireKey(key))
	if err != nil {
		t.Fatal(err)
	}
	if checked.bad.Load() || checked.calls.Load() != 1 {
		t.Fatalf("metadata leaked or source replayed: bad=%t calls=%d", checked.bad.Load(), checked.calls.Load())
	}
	receipt, err := target.AcceptFrozenInput(ctx, key)
	if err != nil || string(second.OperationId) != string(receipt.OperationID[:]) || string(second.ReceiptId) != string(receipt.ReceiptID[:]) {
		t.Fatalf("wrong original ACK %+v %v", receipt, err)
	}
	cancellation, err := creationrpc.NewCancellationClient(creationrpc.Target{Address: targetAddress, Token: frozenCreationToken, Transport: insecure.NewCredentials()})
	if err != nil {
		t.Fatal(err)
	}
	command := f.cancelCommand(t)
	closed, err := creationapp.NewFrozenInputCancellation(f.store, cancellation).Cancel(ctx, envelope.Key(), command)
	if err != nil || closed.CancelProof == nil || closed.CancelProof.Outcome != "cancelled_input" {
		t.Fatalf("closed=%+v err=%v", closed, err)
	}
	replay, err := creationapp.NewFrozenInputCancellation(f.store, cancellation).Cancel(ctx, envelope.Key(), command)
	if err != nil || !reflect.DeepEqual(replay.CancelProof, closed.CancelProof) {
		t.Fatalf("original cancel ACK failed %+v %v", replay.CancelProof, err)
	}
	if _, err := client.AcceptFrozenInput(ctx, frozenWireKey(key)); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("late RPC acceptance err=%v", err)
	}
}

func TestFrozenInputRPCRejectsWrongIdentityDeadlineAndUnknownFields(t *testing.T) {
	f := newFrozenFixture(t)
	envelope := f.freeze(t)
	key := executionKey(envelope)
	checked := &checkedFrozenSource{store: f.store}
	server, err := creationrpc.NewServer(checked, frozenExecutionToken, insecure.NewCredentials())
	if err != nil {
		t.Fatal(err)
	}
	client := pb.NewFrozenInputsClient(frozenRPCConnection(t, serveFrozenRPC(t, server)))
	for _, test := range []struct {
		name, token        string
		deadline           time.Duration
		unknown, duplicate bool
		want               codes.Code
	}{
		{"wrong owner token", frozenCreationToken, time.Second, false, false, codes.Unauthenticated},
		{"missing token", "", time.Second, false, false, codes.Unauthenticated},
		{"unbounded", frozenExecutionToken, 0, false, false, codes.InvalidArgument},
		{"long deadline", frozenExecutionToken, time.Minute, false, false, codes.InvalidArgument},
		{"unknown field", frozenExecutionToken, time.Second, true, false, codes.InvalidArgument},
		{"duplicate identity", frozenExecutionToken, time.Second, false, true, codes.Unauthenticated},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			cancel := func() {}
			if test.deadline > 0 {
				ctx, cancel = context.WithTimeout(ctx, test.deadline)
			}
			defer cancel()
			md := metadata.MD{}
			if test.token != "" {
				md.Set("authorization", "Bearer "+test.token)
			}
			if test.duplicate {
				md.Append("authorization", "Bearer "+test.token)
			}
			ctx = metadata.NewOutgoingContext(ctx, md)
			request := frozenWireKey(key)
			if test.unknown {
				request.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01})
			}
			if _, err := client.ResolveFrozenInput(ctx, request); status.Code(err) != test.want {
				t.Fatalf("code=%s want=%s err=%v", status.Code(err), test.want, err)
			}
		})
	}
	if checked.calls.Load() != 0 {
		t.Fatal("invalid RPC reached source policy store")
	}
	ctx, cancel := frozenRPCContext(t, frozenExecutionToken)
	defer cancel()
	missing := frozenWireKey(key)
	missing.SnapshotSha256 = strings.Repeat("e", 64)
	if _, err := client.ResolveFrozenInput(ctx, missing); status.Code(err) != codes.AlreadyExists || strings.Contains(err.Error(), "SELECT") {
		t.Fatalf("closed failure err=%v", err)
	}
	if _, err := creationrpc.NewServer(checked, frozenExecutionToken, nil); err == nil {
		t.Fatal("missing transport accepted")
	}
	if _, err := executionrpc.NewSourceClient(executionrpc.Target{Address: "127.0.0.1:80", Token: frozenExecutionToken}); err == nil {
		t.Fatal("missing target transport accepted")
	}
}

type malformedFrozenSource struct {
	pb.UnimplementedFrozenInputsServer
	response *pb.FrozenInput
	calls    atomic.Int32
	err      error
}

func (s *malformedFrozenSource) ResolveFrozenInput(context.Context, *pb.FrozenInputKey) (*pb.FrozenInput, error) {
	s.calls.Add(1)
	return s.response, s.err
}
func TestFrozenInputRPCBindsAllMetadataToCompleteContent(t *testing.T) {
	f := newFrozenFixture(t)
	envelope := f.freeze(t)
	key := executionKey(envelope)
	sourceServer, err := creationrpc.NewServer(f.store, frozenExecutionToken, insecure.NewCredentials())
	if err != nil {
		t.Fatal(err)
	}
	originalClient := pb.NewFrozenInputsClient(frozenRPCConnection(t, serveFrozenRPC(t, sourceServer)))
	ctx, cancel := frozenRPCContext(t, frozenExecutionToken)
	defer cancel()
	original, err := originalClient.ResolveFrozenInput(ctx, frozenWireKey(key))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*pb.FrozenInput)
	}{
		{"actor", func(in *pb.FrozenInput) { in.ActorId[0] ^= 1 }},
		{"session", func(in *pb.FrozenInput) { in.SessionId[0] ^= 1 }},
		{"scope", func(in *pb.FrozenInput) { in.ScopeId[0] ^= 1 }},
		{"project", func(in *pb.FrozenInput) { in.ProjectId[0] ^= 1 }},
		{"source generation", func(in *pb.FrozenInput) { in.SourceGeneration++ }},
		{"draft revision", func(in *pb.FrozenInput) { in.DraftRevision++ }},
		{"scope revision", func(in *pb.FrozenInput) { in.ScopeRevision++ }},
		{"authority epoch", func(in *pb.FrozenInput) { in.Command.Authorities[0].Epoch++ }},
		{"source command identity", func(in *pb.FrozenInput) { in.SourceCommand.Id[0] ^= 1 }},
		{"source command hash", func(in *pb.FrozenInput) { in.SourceCommand.Sha256 = strings.Repeat("d", 64) }},
		{"physical organization", func(in *pb.FrozenInput) { in.OrgId[0] ^= 1 }},
		{"project aspect ratio", func(in *pb.FrozenInput) { in.AspectRatio = "9:16" }},
		{"command hash", func(in *pb.FrozenInput) { in.Command.Sha256 = strings.Repeat("d", 64) }},
		{"snapshot content", func(in *pb.FrozenInput) { in.CanonicalSnapshotJson = []byte(`{}`) }},
		{"snapshot digest", func(in *pb.FrozenInput) { in.Key.SnapshotSha256 = strings.Repeat("e", 64) }},
		{"unbound cancellation", func(in *pb.FrozenInput) { in.AdoptionBlocked = true }},
		{"unknown nested", func(in *pb.FrozenInput) { in.Source.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01}) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := proto.Clone(original).(*pb.FrozenInput)
			test.mutate(response)
			fake := &malformedFrozenSource{response: response}
			server := grpc.NewServer()
			pb.RegisterFrozenInputsServer(server, fake)
			source, err := executionrpc.NewSourceClient(executionrpc.Target{Address: serveFrozenRPC(t, server), Token: frozenExecutionToken, Transport: insecure.NewCredentials()})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := source.ReadFrozenInput(ctx, key); err == nil {
				t.Fatal("mismatched metadata/content accepted")
			}
			if fake.calls.Load() != 1 {
				t.Fatal("malformed response was replayed")
			}
		})
	}
	fake := &malformedFrozenSource{err: status.Error(codes.Unavailable, "sensitive SQL must not escape")}
	server := grpc.NewServer()
	pb.RegisterFrozenInputsServer(server, fake)
	source, err := executionrpc.NewSourceClient(executionrpc.Target{Address: serveFrozenRPC(t, server), Token: frozenExecutionToken, Transport: insecure.NewCredentials()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.ReadFrozenInput(ctx, key); !errors.Is(err, writefence.ErrUnavailable) {
		t.Fatalf("raw remote error escaped: %v", err)
	}
	if fake.calls.Load() != 1 {
		t.Fatal("unknown outcome automatically retried")
	}
}
