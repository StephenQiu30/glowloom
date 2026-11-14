package medianormalization_test

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

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/media"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/normalizationgrpc"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

type streamServer struct {
	pb.UnimplementedNormalizationServer
	serve func(grpc.BidiStreamingServer[pb.NormalizeRequest, pb.NormalizeResponse]) error
}

func (s *streamServer) Normalize(stream grpc.BidiStreamingServer[pb.NormalizeRequest, pb.NormalizeResponse]) error {
	return s.serve(stream)
}

func rawProcessor(t *testing.T, serve func(grpc.BidiStreamingServer[pb.NormalizeRequest, pb.NormalizeResponse]) error) *normalizationgrpc.Client {
	t.Helper()
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	pb.RegisterNormalizationServer(server, &streamServer{serve: serve})
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
		if err := <-done; err != nil && !errors.Is(err, grpc.ErrServerStopped) && !errors.Is(err, net.ErrClosed) {
			t.Errorf("serve processor: %v", err)
		}
	})
	client, err := normalizationgrpc.New(listener.Addr().String(), serviceToken, insecure.NewCredentials())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func clientSource(t *testing.T) *mediaapp.Downloaded {
	t.Helper()
	data := []byte("a recording received by a separate service")
	file, err := os.CreateTemp(t.TempDir(), "source-*.webm")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	source := &mediaapp.Downloaded{File: file, Size: int64(len(data)), SHA256: hex.EncodeToString(hash[:]), MIMEType: "video/webm"}
	t.Cleanup(func() { _ = source.Close() })
	return source
}

func drainSource(stream grpc.BidiStreamingServer[pb.NormalizeRequest, pb.NormalizeResponse]) error {
	for {
		_, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func canonicalFrame(data []byte) *pb.NormalizeResponse {
	hash := sha256.Sum256(data)
	return &pb.NormalizeResponse{Payload: &pb.NormalizeResponse_Canonical{Canonical: &pb.Canonical{ByteSize: int64(len(data)), Sha256: hex.EncodeToString(hash[:]), MimeType: "video/mp4", SourceCodec: "vp9"}}}
}

// Only a sniffable container signature: actual codec probing remains the upload
// use case's responsibility and is exercised with real FFmpeg in tests/media.
func mp4Signature() []byte {
	return []byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'm', 'p', '4', '2', 0, 0, 0, 0, 'm', 'p', '4', '2', 'i', 's', 'o', 'm'}
}

func TestNormalizationClientAcceptsOnlyCompleteVerifiedResult(t *testing.T) {
	data := mp4Signature()
	client := rawProcessor(t, func(stream grpc.BidiStreamingServer[pb.NormalizeRequest, pb.NormalizeResponse]) error {
		if err := drainSource(stream); err != nil {
			return err
		}
		if err := stream.Send(canonicalFrame(data)); err != nil {
			return err
		}
		return stream.Send(&pb.NormalizeResponse{Payload: &pb.NormalizeResponse_Chunk{Chunk: data}})
	})
	source := clientSource(t)
	position, err := source.File.Seek(3, io.SeekStart)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Normalize(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = result.File.Close() }()
	actual, err := io.ReadAll(result.File.File)
	if err != nil || !bytes.Equal(actual, data) || result.SourceCodec != "vp9" {
		t.Fatalf("result=%+v bytes=%v error=%v", result, actual, err)
	}
	if at, err := source.File.Seek(0, io.SeekCurrent); err != nil || at != position {
		t.Fatalf("caller file position changed: %d %v", at, err)
	}
}

func TestNormalizationClientRejectsUntrustedOrUncommittedResults(t *testing.T) {
	for _, test := range []struct {
		name string
		mode string
	}{
		{"truncated", "short"}, {"wrong checksum", "hash"}, {"wrong magic", "magic"}, {"duplicate header", "duplicate"}, {"oversize chunk", "large"}, {"failed final status", "lost"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := clientSource(t)
			directory := t.TempDir()
			t.Setenv("TMPDIR", directory)
			var calls atomic.Int32
			client := rawProcessor(t, func(stream grpc.BidiStreamingServer[pb.NormalizeRequest, pb.NormalizeResponse]) error {
				calls.Add(1)
				if err := drainSource(stream); err != nil {
					return err
				}
				data := mp4Signature()
				if test.mode == "magic" {
					data = bytes.Repeat([]byte("x"), len(data))
				}
				header := canonicalFrame(data)
				if err := stream.Send(header); err != nil {
					return err
				}
				switch test.mode {
				case "short":
					data = data[:len(data)-1]
				case "hash":
					data[len(data)-1] = 'z'
				case "duplicate":
					return stream.Send(header)
				case "large":
					data = make([]byte, (256<<10)+1)
				}
				if err := stream.Send(&pb.NormalizeResponse{Payload: &pb.NormalizeResponse_Chunk{Chunk: data}}); err != nil {
					return err
				}
				if test.mode == "lost" {
					return status.Error(codes.Unavailable, "lost final acknowledgement")
				}
				return nil
			})
			result, err := client.Normalize(t.Context(), source)
			if !errors.Is(err, mediaapp.ErrUnavailable) || result.File != nil {
				t.Fatalf("untrusted output accepted: %+v %v", result, err)
			}
			if calls.Load() != 1 {
				t.Fatalf("unexpected command replay: calls=%d", calls.Load())
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 0 {
				t.Fatalf("temporary files leaked: %v %v", entries, err)
			}
			if _, err := source.File.Stat(); err != nil {
				t.Fatalf("caller file closed: %v", err)
			}
		})
	}
}

func TestNormalizationClientDisabledAndCancelledCallsFailClosed(t *testing.T) {
	client, err := normalizationgrpc.New("", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Normalize(t.Context(), clientSource(t)); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatalf("missing target fell back: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.Normalize(ctx, clientSource(t)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
	if _, err := normalizationgrpc.New("127.0.0.1:9091", "", insecure.NewCredentials()); err == nil {
		t.Fatal("partial identity accepted")
	}
}
