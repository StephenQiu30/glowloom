package creationdocument_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/adapter/grpcserver"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/adapter/native"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/extractiongrpc"
)

const token = "creation-test-service-identity-long-enough"

type countedExtractor struct{ calls atomic.Int32 }

func (e *countedExtractor) Extract(ctx context.Context, file *app.File) (app.Result, error) {
	e.calls.Add(1)
	return native.NewExtractor().Extract(ctx, file)
}

func rpcConnection(t *testing.T, extractor app.Extractor) *grpc.ClientConn {
	t.Helper()
	handler, err := grpcserver.New(extractor, token, 1)
	if err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer(grpc.MaxRecvMsgSize(grpcserver.MaxMessageBytes))
	pb.RegisterExtractionServer(server, handler)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
		if err := <-done; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Error(err)
		}
	})
	conn, err := grpc.NewClient("passthrough:///creation", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func sourceFrame(data []byte) *pb.ExtractRequest {
	hash := sha256.Sum256(data)
	return &pb.ExtractRequest{Payload: &pb.ExtractRequest_Source{Source: &pb.Source{ByteSize: int64(len(data)), Sha256: hex.EncodeToString(hash[:]), MimeType: "text/plain"}}}
}
func chunk(data []byte) *pb.ExtractRequest {
	return &pb.ExtractRequest{Payload: &pb.ExtractRequest_Chunk{Chunk: data}}
}

func TestCreationRPCRejectsMalformedOrUnauthenticatedBeforeExtraction(t *testing.T) {
	for _, tc := range []struct {
		name, identity string
		deadline       bool
		frames         []*pb.ExtractRequest
		code           codes.Code
	}{
		{"no_identity", "", true, []*pb.ExtractRequest{sourceFrame([]byte("hi")), chunk([]byte("hi"))}, codes.Unauthenticated},
		{"no_deadline", token, false, nil, codes.InvalidArgument},
		{"no_header", token, true, []*pb.ExtractRequest{chunk([]byte("hi"))}, codes.InvalidArgument},
		{"duplicate_header", token, true, []*pb.ExtractRequest{sourceFrame([]byte("hi")), sourceFrame([]byte("hi"))}, codes.InvalidArgument},
		{"truncated", token, true, []*pb.ExtractRequest{sourceFrame([]byte("hi")), chunk([]byte("h"))}, codes.InvalidArgument},
		{"wrong_hash", token, true, []*pb.ExtractRequest{sourceFrame([]byte("hi")), chunk([]byte("no"))}, codes.InvalidArgument},
		{"too_much", token, true, []*pb.ExtractRequest{sourceFrame([]byte("hi")), chunk([]byte("hello"))}, codes.InvalidArgument},
	} {
		t.Run(tc.name, func(t *testing.T) {
			extractor := &countedExtractor{}
			ctx := metadata.NewOutgoingContext(t.Context(), metadata.Pairs("authorization", "Bearer "+tc.identity))
			if tc.deadline {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
			}
			stream, err := pb.NewExtractionClient(rpcConnection(t, extractor)).Extract(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, frame := range tc.frames {
				if err := stream.Send(frame); err != nil {
					break
				}
			}
			_ = stream.CloseSend()
			_, err = stream.Recv()
			if status.Code(err) != tc.code || extractor.calls.Load() != 0 {
				t.Fatalf("code=%v calls=%d err=%v", status.Code(err), extractor.calls.Load(), err)
			}
		})
	}
}

func TestCreationClientExtractsRealTextAndPreservesCallerOwnership(t *testing.T) {
	var lc net.ListenConfig
	listener, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := grpcserver.New(native.NewExtractor(), token, 1)
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(grpc.MaxRecvMsgSize(grpcserver.MaxMessageBytes))
	pb.RegisterExtractionServer(server, handler)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
		if err := <-done; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Error(err)
		}
	})
	client, err := extractiongrpc.New(listener.Addr().String(), token, insecure.NewCredentials())
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.CreateTemp(t.TempDir(), "original-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	data := []byte("第一幕\r\n\r\n这是完整原文。")
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if _, err := file.Seek(2, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	result, err := client.Extract(t.Context(), &app.File{File: file, Size: int64(len(data)), SHA256: hex.EncodeToString(digest[:]), MIMEType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	text, err := result.Document.PlainText()
	if err != nil || text != "第一幕\n\n这是完整原文。" || result.Encoding != "utf-8" || len(result.Mapping) != 1 {
		t.Fatalf("result=%+v text=%q err=%v", result, text, err)
	}
	if offset, err := file.Seek(0, io.SeekCurrent); err != nil || offset != 2 {
		t.Fatalf("source cursor=%d err=%v", offset, err)
	}
	if _, err := file.Stat(); err != nil {
		t.Fatalf("source closed: %v", err)
	}
}

func TestCreationRPCDoesNotAcceptUnsupportedMimeOrOversizedSource(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*pb.Source)
	}{
		{"mime", func(s *pb.Source) { s.MimeType = "text/html" }},
		{"size", func(s *pb.Source) { s.ByteSize = app.MaxBytes + 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			extractor := &countedExtractor{}
			ctx, cancel := context.WithTimeout(metadata.NewOutgoingContext(t.Context(), metadata.Pairs("authorization", "Bearer "+token)), time.Second)
			defer cancel()
			stream, err := pb.NewExtractionClient(rpcConnection(t, extractor)).Extract(ctx)
			if err != nil {
				t.Fatal(err)
			}
			frame := sourceFrame(bytes.Repeat([]byte("x"), 2))
			tc.mutate(frame.GetSource())
			_ = stream.Send(frame)
			_ = stream.CloseSend()
			_, err = stream.Recv()
			if status.Code(err) != codes.InvalidArgument || extractor.calls.Load() != 0 {
				t.Fatalf("calls=%d err=%v", extractor.calls.Load(), err)
			}
		})
	}
}

func TestCreationClientDisabledAndCancelledFailClosed(t *testing.T) {
	client, err := extractiongrpc.New("", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Extract(t.Context(), nil); !errors.Is(err, extractiongrpc.ErrUnavailable) {
		t.Fatalf("disabled: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.Extract(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}
