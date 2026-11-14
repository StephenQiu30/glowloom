// Package grpcserver exposes bounded, transient Media processing RPCs.
package grpcserver

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/media"
	processing "github.com/StephenQiu30/glowloom-server/backend/internal/media/processing/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
)

// ChunkSize bounds each binary frame independently from the whole file limit.
const ChunkSize = 256 << 10

// MaxMessageBytes includes the protobuf envelope, not arbitrary whole files.
const MaxMessageBytes = ChunkSize + 1024

// MaxDuration includes receiving, processing and sending one transient result.
const MaxDuration = 3 * time.Minute

// Normalizer is the processor consumed by this protocol boundary.
type Normalizer interface {
	Normalize(context.Context, *processing.File) (processing.Result, error)
}

// Server owns only temporary files and concurrency slots, never published assets.
type Server struct {
	pb.UnimplementedNormalizationServer
	normalizer Normalizer
	token      string
	slots      chan struct{}
}

// New validates the private service identity and global processing bound.
func New(normalizer Normalizer, token string, concurrent int) (*Server, error) {
	if normalizer == nil || !serviceauth.ValidToken(token) || concurrent < 1 || concurrent > 8 {
		return nil, errors.New("invalid media normalization configuration")
	}
	return &Server{normalizer: normalizer, token: token, slots: make(chan struct{}, concurrent)}, nil
}

// Normalize receives one verified source and returns one fully computed file.
func (s *Server) Normalize(stream grpc.BidiStreamingServer[pb.NormalizeRequest, pb.NormalizeResponse]) error {
	ctx := stream.Context()
	md, _ := metadata.FromIncomingContext(ctx)
	auth := md.Get("authorization")
	if len(auth) != 1 || subtle.ConstantTimeCompare([]byte(auth[0]), []byte("Bearer "+s.token)) != 1 {
		return status.Error(codes.Unauthenticated, "media service authentication required")
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > MaxDuration {
		return status.Error(codes.InvalidArgument, "bounded media deadline required")
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return status.Error(codes.ResourceExhausted, "media processing capacity exhausted")
	}
	source, err := receiveSource(stream)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	result, err := s.normalizer.Normalize(ctx, source)
	if err != nil {
		return processStatus(err)
	}
	if result.File == nil || result.File.File == nil || result.File.File == source.File {
		return status.Error(codes.Internal, "invalid media processor result")
	}
	defer func() { _ = result.File.Close() }()
	file := result.File
	info, err := file.File.Stat()
	if err != nil || info.Size() != file.Size || file.Size < 1 || file.Size > processing.MaxBytes || !validHash(file.SHA256) || file.MIMEType != "video/mp4" || (result.SourceCodec != "vp8" && result.SourceCodec != "vp9") {
		return status.Error(codes.Internal, "invalid media processor result")
	}
	header := &pb.Canonical{ByteSize: file.Size, Sha256: file.SHA256, MimeType: file.MIMEType, SourceCodec: result.SourceCodec}
	if err := stream.Send(&pb.NormalizeResponse{Payload: &pb.NormalizeResponse_Canonical{Canonical: header}}); err != nil {
		return err
	}
	reader := io.NewSectionReader(file.File, 0, file.Size)
	hash := sha256.New()
	for {
		if err := ctx.Err(); err != nil {
			return status.FromContextError(err).Err()
		}
		buf := make([]byte, ChunkSize)
		n, err := reader.Read(buf)
		if n > 0 {
			_, _ = hash.Write(buf[:n])
			if sendErr := stream.Send(&pb.NormalizeResponse{Payload: &pb.NormalizeResponse_Chunk{Chunk: buf[:n]}}); sendErr != nil {
				return sendErr
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return status.Error(codes.Internal, "read canonical media failed")
		}
	}
	if hex.EncodeToString(hash.Sum(nil)) != file.SHA256 {
		return status.Error(codes.DataLoss, "canonical media checksum mismatch")
	}
	return nil
}

func receiveSource(stream grpc.BidiStreamingServer[pb.NormalizeRequest, pb.NormalizeResponse]) (*processing.File, error) {
	first, err := stream.Recv()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, status.Error(codes.InvalidArgument, "media source header required")
		}
		return nil, err
	}
	header := first.GetSource()
	if header == nil || header.ByteSize < 1 || header.ByteSize > processing.MaxBytes || header.MimeType != "video/webm" || !validHash(header.Sha256) {
		return nil, status.Error(codes.InvalidArgument, "invalid media source header")
	}
	file, err := os.CreateTemp("", "glowloom-media-rpc-*.webm")
	if err != nil {
		return nil, status.Error(codes.ResourceExhausted, "media temporary storage unavailable")
	}
	source := &processing.File{File: file, Size: header.ByteSize, SHA256: header.Sha256, MIMEType: header.MimeType}
	keep := false
	defer func() {
		if !keep {
			_ = source.Close()
		}
	}()
	hash := sha256.New()
	var size int64
	for {
		frame, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		chunk := frame.GetChunk()
		if len(chunk) > ChunkSize {
			return nil, status.Error(codes.ResourceExhausted, "media frame too large")
		}
		if len(chunk) == 0 || int64(len(chunk)) > header.ByteSize-size {
			return nil, status.Error(codes.InvalidArgument, "invalid media source stream")
		}
		if err := stream.Context().Err(); err != nil {
			return nil, status.FromContextError(err).Err()
		}
		if _, err := file.Write(chunk); err != nil {
			return nil, status.Error(codes.ResourceExhausted, "write media temporary file failed")
		}
		_, _ = hash.Write(chunk)
		size += int64(len(chunk))
	}
	if size != header.ByteSize || hex.EncodeToString(hash.Sum(nil)) != header.Sha256 {
		return nil, status.Error(codes.InvalidArgument, "media source size or checksum mismatch")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, status.Error(codes.Internal, "rewind media source failed")
	}
	keep = true
	return source, nil
}

func validHash(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func processStatus(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	case errors.Is(err, processing.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, "invalid media source")
	case errors.Is(err, processing.ErrUnsupported):
		return status.Error(codes.FailedPrecondition, "unsupported recording")
	case errors.Is(err, processing.ErrTooLarge):
		return status.Error(codes.OutOfRange, "recording exceeds media size limit")
	default:
		// Decoder diagnostics and temporary paths are not a protocol response.
		return status.Error(codes.Unavailable, "media processing unavailable")
	}
}
