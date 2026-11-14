package creationdocument_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/adapter/grpcserver"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/adapter/native"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/extractiongrpc"
)

const clientOriginalText = "第一幕\r\n\r\n正文😀"

type clientContractServer struct {
	pb.UnimplementedExtractionServer
	calls atomic.Int32
	serve func(grpc.BidiStreamingServer[pb.ExtractRequest, pb.ExtractResponse]) error
}

func (s *clientContractServer) Extract(stream grpc.BidiStreamingServer[pb.ExtractRequest, pb.ExtractResponse]) error {
	s.calls.Add(1)
	// Every response scenario first consumes and checks the complete request,
	// ensuring final-status tests exercise a processed, non-replayable upload.
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	header := first.GetSource()
	want := sourceFrame([]byte(clientOriginalText)).GetSource()
	if header == nil || header.ByteSize != want.ByteSize || header.Sha256 != want.Sha256 || header.MimeType != want.MimeType {
		return status.Error(codes.InvalidArgument, "client sent an invalid original header")
	}
	var body bytes.Buffer
	for {
		frame, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		part := frame.GetChunk()
		if len(part) == 0 || len(part) > grpcserver.ChunkSize || body.Len()+len(part) > len(clientOriginalText) {
			return status.Error(codes.InvalidArgument, "client sent invalid original frames")
		}
		_, _ = body.Write(part)
	}
	if body.String() != clientOriginalText {
		return status.Error(codes.InvalidArgument, "client changed original bytes")
	}
	return s.serve(stream)
}

func clientContractConnection(t *testing.T, serve func(grpc.BidiStreamingServer[pb.ExtractRequest, pb.ExtractResponse]) error) (*extractiongrpc.Client, *clientContractServer) {
	t.Helper()
	var config net.ListenConfig
	listener, err := config.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	handler := &clientContractServer{serve: serve}
	server := grpc.NewServer(grpc.MaxRecvMsgSize(grpcserver.MaxMessageBytes))
	pb.RegisterExtractionServer(server, handler)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, grpc.ErrServerStopped) && !errors.Is(err, net.ErrClosed) {
				t.Errorf("stop client contract server: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("client contract server did not stop")
		}
	})
	client, err := extractiongrpc.New(listener.Addr().String(), token, insecure.NewCredentials())
	if err != nil {
		t.Fatal(err)
	}
	return client, handler
}

func clientContractOriginal(t *testing.T) *app.File {
	t.Helper()
	file := nativeOriginal(t, []byte(clientOriginalText), "text/plain")
	if _, err := file.File.Seek(2, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	return file
}

func clientContractReceipt(t *testing.T) []byte {
	t.Helper()
	result, err := native.NewExtractor().Extract(t.Context(), clientContractOriginal(t))
	if err != nil {
		t.Fatalf("extract a genuine native receipt: %v", err)
	}
	data, err := app.EncodeResult(result)
	if err != nil {
		t.Fatalf("native result rejected by result validation: %v", err)
	}
	return data
}

func assertClientOriginalPreserved(t *testing.T, file *app.File, name string) {
	t.Helper()
	if file.File == nil {
		t.Fatal("client took ownership of original")
	}
	if _, err := file.File.Stat(); err != nil {
		t.Fatalf("client closed original: %v", err)
	}
	if _, err := os.Stat(name); err != nil {
		t.Fatalf("client removed original: %v", err)
	}
	if offset, err := file.File.Seek(0, io.SeekCurrent); err != nil || offset != 2 {
		t.Fatalf("client changed original cursor: offset=%d error=%v", offset, err)
	}
}

func assertNoClientResult(t *testing.T, result app.Result) {
	t.Helper()
	if result.Document.Type != "" || len(result.Document.Content) != 0 || result.Encoding != "" || len(result.Mapping) != 0 || len(result.Warnings) != 0 {
		t.Fatal("client exposed a partial or unverified extraction result")
	}
}

func TestCreationClientContractRequiresFinalOKAndDoesNotReplay(t *testing.T) {
	receipt := clientContractReceipt(t)
	for _, tc := range []struct {
		name   string
		status codes.Code
	}{
		{"complete JSON and final OK", codes.OK},
		{"complete JSON and final Unavailable", codes.Unavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, handler := clientContractConnection(t, func(stream grpc.BidiStreamingServer[pb.ExtractRequest, pb.ExtractResponse]) error {
				// Split inside JSON instead of relying on one frame per document.
				middle := len(receipt) / 2
				for _, data := range [][]byte{receipt[:middle], receipt[middle:]} {
					if err := stream.Send(&pb.ExtractResponse{JsonChunk: data}); err != nil {
						return err
					}
				}
				if tc.status != codes.OK {
					return status.Error(tc.status, "final extraction status unavailable")
				}
				return nil
			})
			file := clientContractOriginal(t)
			name := file.File.Name()
			result, err := client.Extract(t.Context(), file)
			if tc.status == codes.OK {
				if err != nil {
					t.Fatal(err)
				}
				text, err := result.Document.PlainText()
				if err != nil || text != "第一幕\n\n正文😀" || result.Encoding != "utf-8" || len(result.Mapping) != 1 {
					t.Fatalf("valid native receipt changed: text=%q error=%v", text, err)
				}
			} else {
				if !errors.Is(err, extractiongrpc.ErrUnavailable) || errors.Is(err, app.ErrExtractionFailed) {
					t.Fatalf("final Unavailable accepted or treated as bad source: %v", err)
				}
				assertNoClientResult(t, result)
			}
			if calls := handler.calls.Load(); calls != 1 {
				t.Fatalf("processed extraction was replayed: calls=%d", calls)
			}
			assertClientOriginalPreserved(t, file, name)
		})
	}
}

func TestCreationClientContractRejectsInvalidJSONReceipts(t *testing.T) {
	receipt := clientContractReceipt(t)
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"malformed JSON", []byte("{bad JSON")},
		{"truncated JSON", receipt[:len(receipt)-1]},
		{"multiple JSON values", append(append([]byte{}, receipt...), receipt...)},
		{"unknown receipt field", append(append([]byte{}, receipt[:len(receipt)-1]...), []byte(`,"Unexpected":true}`)...)},
		{"unknown document field", bytes.Replace(receipt, []byte(`"type":"doc"`), []byte(`"type":"doc","unexpected":true`), 1)},
		{"null receipt", []byte("null")},
		{"missing receipt facts", []byte("{}")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, handler := clientContractConnection(t, func(stream grpc.BidiStreamingServer[pb.ExtractRequest, pb.ExtractResponse]) error {
				return stream.Send(&pb.ExtractResponse{JsonChunk: tc.data})
			})
			file := clientContractOriginal(t)
			name := file.File.Name()
			result, err := client.Extract(t.Context(), file)
			if !errors.Is(err, extractiongrpc.ErrUnavailable) || errors.Is(err, app.ErrExtractionFailed) {
				t.Fatalf("malformed result accepted or misclassified as bad original: %v", err)
			}
			assertNoClientResult(t, result)
			assertClientOriginalPreserved(t, file, name)
			if handler.calls.Load() != 1 {
				t.Fatal("invalid result caused original extraction to replay")
			}
		})
	}
}

func TestCreationClientContractBoundsResultFramesAndAggregate(t *testing.T) {
	for _, tc := range []struct {
		name  string
		sizes []int
	}{
		{"no response frames", nil},
		{"empty response frame", []int{0}},
		{"oversized response frame", []int{grpcserver.ChunkSize + 1}},
		{"oversized wire message", []int{grpcserver.MaxMessageBytes + 1}},
		{"oversized aggregate", func() []int {
			sizes := make([]int, app.MaxResultBytes/grpcserver.ChunkSize+1)
			for i := range sizes {
				sizes[i] = grpcserver.ChunkSize
			}
			sizes[len(sizes)-1] = 1
			return sizes
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := clientContractConnection(t, func(stream grpc.BidiStreamingServer[pb.ExtractRequest, pb.ExtractResponse]) error {
				for _, size := range tc.sizes {
					if err := stream.Send(&pb.ExtractResponse{JsonChunk: bytes.Repeat([]byte{'x'}, size)}); err != nil {
						return err
					}
				}
				return nil
			})
			file := clientContractOriginal(t)
			name := file.File.Name()
			result, err := client.Extract(t.Context(), file)
			if !errors.Is(err, extractiongrpc.ErrUnavailable) {
				t.Fatalf("unbounded/incomplete response accepted: %v", err)
			}
			assertNoClientResult(t, result)
			assertClientOriginalPreserved(t, file, name)
		})
	}
}

func TestCreationClientContractRejectsPartialServiceIdentity(t *testing.T) {
	client, err := extractiongrpc.New("", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	file := clientContractOriginal(t)
	name := file.File.Name()
	result, err := client.Extract(t.Context(), file)
	if !errors.Is(err, extractiongrpc.ErrUnavailable) {
		t.Fatalf("unconfigured client attempted local extraction: %v", err)
	}
	assertNoClientResult(t, result)
	assertClientOriginalPreserved(t, file, name)
	for _, tc := range []struct {
		name, target, identity string
		transport              credentials.TransportCredentials
	}{
		{"missing address", "", token, insecure.NewCredentials()},
		{"missing token", "127.0.0.1:1", "", insecure.NewCredentials()},
		{"missing transport", "127.0.0.1:1", token, nil},
		{"only transport", "", "", insecure.NewCredentials()},
		{"short token", "127.0.0.1:1", strings.Repeat("x", 31), insecure.NewCredentials()},
		{"token whitespace", "127.0.0.1:1", token + " x", insecure.NewCredentials()},
		{"token control byte", "127.0.0.1:1", token + "\x00", insecure.NewCredentials()},
		{"URL instead of target", "http://127.0.0.1:1", token, insecure.NewCredentials()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if client, err := extractiongrpc.New(tc.target, tc.identity, tc.transport); err == nil || client != nil {
				t.Fatal("partial or unsafe service identity was accepted")
			}
		})
	}
}

func TestCreationClientContractPreservesCancellationWithoutReplay(t *testing.T) {
	entered := make(chan struct{}, 1)
	client, handler := clientContractConnection(t, func(stream grpc.BidiStreamingServer[pb.ExtractRequest, pb.ExtractResponse]) error {
		entered <- struct{}{}
		<-stream.Context().Done()
		return status.FromContextError(stream.Context().Err()).Err()
	})
	file := clientContractOriginal(t)
	name := file.File.Name()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := client.Extract(ctx, file)
	if !errors.Is(err, context.Canceled) || handler.calls.Load() != 0 {
		t.Fatalf("pre-cancelled client started extraction: calls=%d error=%v", handler.calls.Load(), err)
	}
	assertNoClientResult(t, result)

	type outcome struct {
		result app.Result
		err    error
	}
	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	done := make(chan outcome, 1)
	go func() {
		result, err := client.Extract(ctx, file)
		done <- outcome{result: result, err: err}
	}()
	select {
	case <-entered:
	case out := <-done:
		t.Fatalf("client returned before server received original: %v", out.err)
	case <-time.After(3 * time.Second):
		t.Fatal("client did not send original")
	}
	cancel()
	select {
	case out := <-done:
		if !errors.Is(out.err, context.Canceled) {
			t.Fatalf("in-flight cancellation lost: %v", out.err)
		}
		assertNoClientResult(t, out.result)
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled client did not exit")
	}
	if handler.calls.Load() != 1 {
		t.Fatalf("cancelled extraction was replayed: calls=%d", handler.calls.Load())
	}
	assertClientOriginalPreserved(t, file, name)
}

func TestCreationClientContractOnlyPropagatesW3CTraceAndServiceIdentity(t *testing.T) {
	receipt := clientContractReceipt(t)
	observed := make(chan metadata.MD, 1)
	client, _ := clientContractConnection(t, func(stream grpc.BidiStreamingServer[pb.ExtractRequest, pb.ExtractResponse]) error {
		incoming, _ := metadata.FromIncomingContext(stream.Context())
		observed <- incoming.Copy()
		return stream.Send(&pb.ExtractResponse{JsonChunk: receipt})
	})
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatal(err)
	}
	state, err := trace.ParseTraceState("test=creation")
	if err != nil {
		t.Fatal(err)
	}
	span := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled, TraceState: state, Remote: true})
	ctx := trace.ContextWithRemoteSpanContext(t.Context(), span)
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("x-incoming-private", "incoming-user-secret"))
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer caller-user-secret", "cookie", "session=private", "x-project-id", "private-project", "x-user-id", "private-user", "x-request-id", "private-request", "traceparent", "caller-controlled-trace", "tracestate", "caller=untrusted"))
	file := clientContractOriginal(t)
	name := file.File.Name()
	if _, err := client.Extract(ctx, file); err != nil {
		t.Fatal(err)
	}
	var incoming metadata.MD
	select {
	case incoming = <-observed:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not observe client metadata")
	}
	for key, want := range map[string]string{
		"authorization": "Bearer " + token,
		"traceparent":   "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"tracestate":    "test=creation",
	} {
		values := incoming.Get(key)
		if len(values) != 1 || values[0] != want {
			t.Fatalf("service identity/trace metadata %s missing or not isolated", key)
		}
	}
	for key := range incoming {
		switch key {
		case ":authority", "content-type", "user-agent", "grpc-accept-encoding", "grpc-encoding", "authorization", "traceparent", "tracestate":
		default:
			t.Errorf("client leaked caller metadata key %q", key)
		}
	}
	for _, forbidden := range []string{"caller-user-secret", "session=private", "private-project", "private-user", "private-request", "incoming-user-secret", "caller-controlled-trace", "caller=untrusted"} {
		if strings.Contains(fmt.Sprint(incoming), forbidden) {
			t.Fatal("client forwarded private caller metadata")
		}
	}
	assertClientOriginalPreserved(t, file, name)
}
