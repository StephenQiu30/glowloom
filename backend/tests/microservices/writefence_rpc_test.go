package microservices_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	fencerpc "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
)

func identityFenceToken(writer writefence.Writer) string {
	return "identity-to-" + string(writer) + "-fence-test-token-32-bytes"
}

func creationFenceToken(writer writefence.Writer) string {
	return "creation-to-" + string(writer) + "-fence-test-token-32-bytes"
}

func startFenceRPC(t *testing.T, service pb.WriterFenceServer, interceptor grpc.UnaryServerInterceptor) string {
	t.Helper()
	var config net.ListenConfig
	listener, err := config.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	options := []grpc.ServerOption{grpc.MaxRecvMsgSize(fencerpc.MaxMessageBytes), grpc.MaxSendMsgSize(fencerpc.MaxMessageBytes), grpc.WaitForHandlers(true)}
	if interceptor != nil {
		options = append(options, grpc.UnaryInterceptor(interceptor))
	}
	server := grpc.NewServer(options...)
	pb.RegisterWriterFenceServer(server, service)
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

func fenceTargets(address string) map[writefence.Writer]fencerpc.Target {
	result := make(map[writefence.Writer]fencerpc.Target)
	for _, writer := range writefence.Writers() {
		result[writer] = fencerpc.Target{Address: address, Token: identityFenceToken(writer), Transport: insecure.NewCredentials()}
	}
	return result
}

type fenceRPCStore struct {
	owner   writefence.Writer
	install func(context.Context, writefence.Control) error
	load    func(context.Context, uuid.UUID) (writefence.Record, error)
}

func (s fenceRPCStore) Owner(context.Context) (writefence.Writer, error) {
	if s.owner != "" {
		return s.owner, nil
	}
	return writefence.WriterMedia, nil
}

func (s fenceRPCStore) Install(ctx context.Context, control writefence.Control) error {
	if s.install != nil {
		return s.install(ctx, control)
	}
	return nil
}

func (s fenceRPCStore) Load(ctx context.Context, id uuid.UUID) (writefence.Record, error) {
	if s.load != nil {
		return s.load(ctx, id)
	}
	return writefence.Record{}, writefence.ErrNotFound
}

func wireControl() *pb.Control {
	id, coordinationID := uuid.New(), uuid.New()
	return &pb.Control{Authority: &pb.Authority{Issuer: pb.Issuer_ISSUER_IDENTITY, Kind: pb.AuthorityKind_AUTHORITY_KIND_SESSION, Id: append([]byte(nil), id[:]...), Epoch: 2}, Blocked: true, CoordinationId: append([]byte(nil), coordinationID[:]...)}
}

func TestWriteFenceRPCRejectsForgedIssuerAudienceAndMalformedControl(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*pb.InstallRequest)
		tokens []string
		noTime bool
		code   codes.Code
	}{
		{name: "missing identity", code: codes.Unauthenticated},
		{name: "duplicate identity", tokens: []string{identityFenceToken(writefence.WriterMedia), identityFenceToken(writefence.WriterMedia)}, code: codes.Unauthenticated},
		{name: "wrong audience token", tokens: []string{identityFenceToken(writefence.WriterExecution)}, code: codes.Unauthenticated},
		{name: "wrong issuer", tokens: []string{creationFenceToken(writefence.WriterMedia)}, code: codes.PermissionDenied},
		{name: "wrong target", mutate: func(r *pb.InstallRequest) { r.Writer = pb.Writer_WRITER_EXECUTION }, tokens: []string{identityFenceToken(writefence.WriterMedia)}, code: codes.PermissionDenied},
		{name: "missing deadline", noTime: true, tokens: []string{identityFenceToken(writefence.WriterMedia)}, code: codes.InvalidArgument},
		{name: "missing control", mutate: func(r *pb.InstallRequest) { r.Control = nil }, tokens: []string{identityFenceToken(writefence.WriterMedia)}, code: codes.InvalidArgument},
		{name: "zero epoch", mutate: func(r *pb.InstallRequest) { r.Control.Authority.Epoch = 0 }, tokens: []string{identityFenceToken(writefence.WriterMedia)}, code: codes.InvalidArgument},
		{name: "invalid uuid", mutate: func(r *pb.InstallRequest) { r.Control.CoordinationId = []byte{1} }, tokens: []string{identityFenceToken(writefence.WriterMedia)}, code: codes.InvalidArgument},
		{name: "unknown enum", mutate: func(r *pb.InstallRequest) { r.Control.Authority.Kind = pb.AuthorityKind(99) }, tokens: []string{identityFenceToken(writefence.WriterMedia)}, code: codes.InvalidArgument},
		{name: "issuer kind mismatch", mutate: func(r *pb.InstallRequest) { r.Control.Authority.Kind = pb.AuthorityKind_AUTHORITY_KIND_PROJECT }, tokens: []string{identityFenceToken(writefence.WriterMedia)}, code: codes.InvalidArgument},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			service, err := fencerpc.NewServer(writefence.WriterMedia, fenceRPCStore{install: func(context.Context, writefence.Control) error { calls.Add(1); return nil }}, identityFenceToken(writefence.WriterMedia), creationFenceToken(writefence.WriterMedia))
			if err != nil {
				t.Fatal(err)
			}
			address := startFenceRPC(t, service, nil)
			connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = connection.Close() }()
			ctx := t.Context()
			if !test.noTime {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, time.Second)
				defer cancel()
			}
			var values []string
			for _, token := range test.tokens {
				values = append(values, "authorization", "Bearer "+token)
			}
			ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(values...))
			request := &pb.InstallRequest{Writer: pb.Writer_WRITER_MEDIA, Control: wireControl()}
			if test.mutate != nil {
				test.mutate(request)
			}
			_, err = pb.NewWriterFenceClient(connection).Install(ctx, request)
			if status.Code(err) != test.code || calls.Load() != 0 {
				t.Fatalf("rejected control: status=%v, calls=%d", err, calls.Load())
			}
		})
	}
}

type wrongFenceACK struct {
	pb.UnimplementedWriterFenceServer
	mode  string
	calls atomic.Int32
}

func (s *wrongFenceACK) Install(_ context.Context, request *pb.InstallRequest) (*pb.InstallResponse, error) {
	s.calls.Add(1)
	if s.mode == "lost final status" {
		return nil, status.Error(codes.Unavailable, "simulated lost ACK")
	}
	response := &pb.InstallResponse{Writer: request.GetWriter(), Control: request.GetControl()}
	switch s.mode {
	case "wrong epoch":
		response.Control.Authority.Epoch++
	case "wrong coordination":
		id := uuid.New()
		response.Control.CoordinationId = append([]byte(nil), id[:]...)
	case "wrong target":
		response.Writer = pb.Writer_WRITER_EXECUTION
	case "wrong outcome":
		response.Control.Blocked = !response.Control.Blocked
	case "missing control":
		response.Control = nil
	}
	return response, nil
}

func TestWriteFenceRPCDoesNotAcknowledgeMalformedOrUnknownOutcome(t *testing.T) {
	for _, mode := range []string{"wrong epoch", "wrong coordination", "wrong target", "wrong outcome", "missing control", "lost final status"} {
		t.Run(mode, func(t *testing.T) {
			service := &wrongFenceACK{mode: mode}
			address := startFenceRPC(t, service, nil)
			client, err := fencerpc.NewClient(writefence.Identity, fenceTargets(address))
			if err != nil {
				t.Fatal(err)
			}
			control := writefence.Control{Authority: fenceCommand().Authorities[0], CoordinationID: uuid.New(), Blocked: true}
			if err := client.Install(t.Context(), writefence.WriterMedia, control); !errors.Is(err, writefence.ErrUnavailable) {
				t.Fatalf("invalid ACK became success: %v", err)
			}
			if service.calls.Load() != 1 {
				t.Fatalf("effect was transparently replayed %d times", service.calls.Load())
			}
		})
	}
}

func TestWriteFenceRPCPreservesOriginalReceiptAndMetadataIsolation(t *testing.T) {
	command := fenceCommand()
	var calls atomic.Int32
	service, err := fencerpc.NewServer(writefence.WriterMedia, fenceRPCStore{load: func(context.Context, uuid.UUID) (writefence.Record, error) {
		calls.Add(1)
		return writefence.Record{Command: command, State: writefence.Committed, Receipt: json.RawMessage(`{"sequence":9007199254740993}`)}, nil
	}}, identityFenceToken(writefence.WriterMedia), creationFenceToken(writefence.WriterMedia))
	if err != nil {
		t.Fatal(err)
	}
	address := startFenceRPC(t, service, func(ctx context.Context, request any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		if len(md.Get("authorization")) != 1 || len(md.Get("x-user-id")) != 0 || len(md.Get("baggage")) != 0 {
			return nil, status.Error(codes.PermissionDenied, "caller identity leaked")
		}
		if values := md.Get("traceparent"); len(values) != 1 || values[0] != "00-11223344556677889900112233445566-1122334455667788-01" {
			return nil, status.Error(codes.InvalidArgument, "missing original trace")
		}
		return handler(ctx, request)
	})
	client, err := fencerpc.NewClient(writefence.Identity, fenceTargets(address))
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := trace.TraceIDFromHex("11223344556677889900112233445566")
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex("1122334455667788")
	if err != nil {
		t.Fatal(err)
	}
	ctx := trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled}))
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer caller-session", "x-user-id", uuid.NewString(), "baggage", "secret=yes", "traceparent", "forged-caller-trace"))
	record, err := client.Resolve(ctx, writefence.WriterMedia, command)
	if err != nil || record.State != writefence.Committed || string(record.Receipt) != `{"sequence":9007199254740993}` {
		t.Fatalf("original receipt not recovered: %+v, %v", record, err)
	}
	different := command
	different.SHA256 = strings.Repeat("b", 64)
	if _, err := client.Resolve(ctx, writefence.WriterMedia, different); !errors.Is(err, writefence.ErrConflict) {
		t.Fatalf("another input could recover private outcome: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("unexpected replay: %d", calls.Load())
	}
}

func TestWriteFenceRPCCancelWaitsForActualControlAttempt(t *testing.T) {
	entered := make(chan struct{})
	service, err := fencerpc.NewServer(writefence.WriterMedia, fenceRPCStore{install: func(ctx context.Context, _ writefence.Control) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}}, identityFenceToken(writefence.WriterMedia), creationFenceToken(writefence.WriterMedia))
	if err != nil {
		t.Fatal(err)
	}
	address := startFenceRPC(t, service, nil)
	client, err := fencerpc.NewClient(writefence.Identity, fenceTargets(address))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- client.Install(ctx, writefence.WriterMedia, writefence.Control{Authority: fenceCommand().Authorities[0], CoordinationID: uuid.New(), Blocked: true})
	}()
	select {
	case <-entered:
		cancel()
	case <-ctx.Done():
		t.Fatal("control did not reach server")
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("caller cancellation changed classification: %v", err)
	}
}
