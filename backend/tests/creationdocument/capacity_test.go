package creationdocument_test

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/adapter/grpcserver"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/application"
)

type waitingExtraction struct {
	entered chan struct{}
	calls   atomic.Int32
}

func (e *waitingExtraction) Extract(ctx context.Context, _ *app.File) (app.Result, error) {
	e.calls.Add(1)
	close(e.entered)
	<-ctx.Done()
	return app.Result{}, ctx.Err()
}

func TestCreationRPCEnforcesGlobalCapacityBeforeSecondExtraction(t *testing.T) {
	extractor := &waitingExtraction{entered: make(chan struct{})}
	handler, err := grpcserver.New(extractor, token, 1)
	if err != nil {
		t.Fatal(err)
	}
	var lc net.ListenConfig
	listener, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(grpc.WaitForHandlers(true), grpc.MaxRecvMsgSize(grpcserver.MaxMessageBytes))
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
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
	original := nativeOriginal(t, []byte("capacity"), "text/plain")
	first, err := pb.NewExtractionClient(conn).Extract(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Send(&pb.ExtractRequest{Payload: &pb.ExtractRequest_Source{Source: &pb.Source{ByteSize: original.Size, Sha256: original.SHA256, MimeType: original.MIMEType}}}); err != nil {
		t.Fatal(err)
	}
	if err := first.Send(&pb.ExtractRequest{Payload: &pb.ExtractRequest_Chunk{Chunk: []byte("capacity")}}); err != nil {
		t.Fatal(err)
	}
	if err := first.CloseSend(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-extractor.entered:
	case <-ctx.Done():
		t.Fatal("first extraction did not enter")
	}
	second, err := pb.NewExtractionClient(conn).Extract(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = second.Recv()
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("capacity status=%v", err)
	}
	if extractor.calls.Load() != 1 {
		t.Fatalf("native calls=%d", extractor.calls.Load())
	}
	cancel()
	if _, err := first.Recv(); status.Code(err) != codes.Canceled {
		t.Fatalf("first cancellation=%v", err)
	}
}
