package medianormalization_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/media"
	grpcserver "github.com/StephenQiu30/glowloom-server/backend/internal/media/processing/adapter/grpcserver"
	processing "github.com/StephenQiu30/glowloom-server/backend/internal/media/processing/application"
)

const serviceToken = "normalization-test-token-at-least-32-bytes"

type normalizationFunc func(context.Context, *processing.File) (processing.Result, error)

func (f normalizationFunc) Normalize(ctx context.Context, file *processing.File) (processing.Result, error) {
	return f(ctx, file)
}

func normalizationRPC(t *testing.T, normalize normalizationFunc) pb.NormalizationClient {
	t.Helper()
	handler, err := grpcserver.New(normalize, serviceToken, 1)
	if err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer(grpc.MaxRecvMsgSize(grpcserver.MaxMessageBytes))
	pb.RegisterNormalizationServer(server, handler)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	conn, err := grpc.NewClient("passthrough:///normalization", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithDisableRetry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		server.Stop()
		_ = listener.Close()
		if err := <-done; err != nil && !errors.Is(err, grpc.ErrServerStopped) && !errors.Is(err, net.ErrClosed) {
			t.Errorf("serve normalization: %v", err)
		}
	})
	return pb.NewNormalizationClient(conn)
}

func normalizationContext(t *testing.T, token string) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}

func sourceFrame(data []byte) *pb.NormalizeRequest {
	hash := sha256.Sum256(data)
	return &pb.NormalizeRequest{Payload: &pb.NormalizeRequest_Source{Source: &pb.Source{ByteSize: int64(len(data)), Sha256: hex.EncodeToString(hash[:]), MimeType: "video/webm"}}}
}

func TestNormalizationRPCRejectsUnauthenticatedAndMalformedStreams(t *testing.T) {
	for _, test := range []struct {
		name   string
		token  string
		frames []*pb.NormalizeRequest
		code   codes.Code
	}{
		{"missing token", "", nil, codes.Unauthenticated},
		{"wrong token", strings.Repeat("x", 40), nil, codes.Unauthenticated},
		{"chunk before header", serviceToken, []*pb.NormalizeRequest{{Payload: &pb.NormalizeRequest_Chunk{Chunk: []byte("webm")}}}, codes.InvalidArgument},
		{"truncated source", serviceToken, []*pb.NormalizeRequest{sourceFrame([]byte("webm"))}, codes.InvalidArgument},
		{"hash mismatch", serviceToken, []*pb.NormalizeRequest{sourceFrame([]byte("webm")), {Payload: &pb.NormalizeRequest_Chunk{Chunk: []byte("nope")}}}, codes.InvalidArgument},
		{"second header", serviceToken, []*pb.NormalizeRequest{sourceFrame([]byte("webm")), sourceFrame([]byte("webm"))}, codes.InvalidArgument},
		{"oversize chunk", serviceToken, []*pb.NormalizeRequest{sourceFrame([]byte("webm")), {Payload: &pb.NormalizeRequest_Chunk{Chunk: make([]byte, grpcserver.ChunkSize+1)}}}, codes.ResourceExhausted},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := normalizationRPC(t, func(context.Context, *processing.File) (processing.Result, error) {
				t.Error("rejected stream reached native processor")
				return processing.Result{}, nil
			})
			stream, err := client.Normalize(normalizationContext(t, test.token))
			if err != nil {
				t.Fatal(err)
			}
			for _, frame := range test.frames {
				if err := stream.Send(frame); err != nil {
					break
				}
			}
			_ = stream.CloseSend()
			_, err = stream.Recv()
			if status.Code(err) != test.code {
				t.Fatalf("code=%v error=%v; want %v", status.Code(err), err, test.code)
			}
		})
	}
}

func TestNormalizationRPCRequiresBoundedDeadline(t *testing.T) {
	client := normalizationRPC(t, func(context.Context, *processing.File) (processing.Result, error) {
		t.Error("unbounded RPC reached processor")
		return processing.Result{}, nil
	})
	ctx := metadata.AppendToOutgoingContext(t.Context(), "authorization", "Bearer "+serviceToken)
	stream, err := client.Normalize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = stream.CloseSend()
	_, err = stream.Recv()
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unbounded call: %v", err)
	}
}

func TestNormalizationRPCStreamsVerifiedBytesAndDeletesItsFiles(t *testing.T) {
	data := []byte("a complete recording")
	canonical := []byte("canonical media bytes")
	var inputPath, outputPath string
	client := normalizationRPC(t, func(_ context.Context, source *processing.File) (processing.Result, error) {
		inputPath = source.File.Name()
		actual, err := io.ReadAll(source.File)
		if err != nil || string(actual) != string(data) {
			t.Errorf("source=%q error=%v", actual, err)
		}
		file, err := os.CreateTemp(t.TempDir(), "canonical-*.mp4")
		if err != nil {
			return processing.Result{}, err
		}
		outputPath = file.Name()
		if _, err := file.Write(canonical); err != nil {
			return processing.Result{}, err
		}
		hash := sha256.Sum256(canonical)
		return processing.Result{File: &processing.File{File: file, Size: int64(len(canonical)), SHA256: hex.EncodeToString(hash[:]), MIMEType: "video/mp4"}, SourceCodec: "vp9"}, nil
	})
	stream, err := client.Normalize(normalizationContext(t, serviceToken))
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(sourceFrame(data)); err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&pb.NormalizeRequest{Payload: &pb.NormalizeRequest_Chunk{Chunk: data}}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	first, err := stream.Recv()
	if err != nil || first.GetCanonical().GetSourceCodec() != "vp9" {
		t.Fatalf("header=%v error=%v", first, err)
	}
	var actual []byte
	for {
		frame, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		actual = append(actual, frame.GetChunk()...)
	}
	if string(actual) != string(canonical) {
		t.Fatalf("result=%q", actual)
	}
	// EOF is emitted after handler defers have closed and removed both files.
	for _, path := range []string{inputPath, outputPath} {
		if path == "" {
			t.Fatal("processor was not called")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("temporary file survives: %q: %v", path, err)
		}
	}
}
