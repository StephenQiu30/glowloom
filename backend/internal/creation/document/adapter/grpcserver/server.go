// Package grpcserver exposes only bounded Creation document extraction.
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

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creation"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
)

const (
	// ChunkSize bounds one protocol frame, independently of aggregate receipts.
	ChunkSize = 256 << 10
	// MaxMessageBytes allows only the bounded frame and protobuf overhead.
	MaxMessageBytes = ChunkSize + 1024
)

// Server admits a bounded number of complete byte-stream lifecycles.
type Server struct {
	pb.UnimplementedExtractionServer
	extractor app.Extractor
	token     string
	slots     chan struct{}
}

// New requires independent service identity and a finite global in-flight bound.
func New(extractor app.Extractor, token string, concurrent int) (*Server, error) {
	if extractor == nil || !serviceauth.ValidToken(token) || concurrent < 1 || concurrent > 8 {
		return nil, errors.New("invalid Creation extraction configuration")
	}
	return &Server{extractor: extractor, token: token, slots: make(chan struct{}, concurrent)}, nil
}

// Extract never interprets source bytes as paths, URLs or publication authority.
func (s *Server) Extract(stream grpc.BidiStreamingServer[pb.ExtractRequest, pb.ExtractResponse]) error {
	ctx := stream.Context()
	md, _ := metadata.FromIncomingContext(ctx)
	auth := md.Get("authorization")
	if len(auth) != 1 || subtle.ConstantTimeCompare([]byte(auth[0]), []byte("Bearer "+s.token)) != 1 {
		return status.Error(codes.Unauthenticated, "Creation service identity required")
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 61*time.Second {
		return status.Error(codes.InvalidArgument, "bounded extraction deadline required")
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return status.Error(codes.ResourceExhausted, "Creation extraction busy")
	}
	first, err := stream.Recv()
	if err != nil {
		return receiveError(err)
	}
	header := first.GetSource()
	if header == nil || header.ByteSize < 1 || header.ByteSize > app.MaxBytes || !validHash(header.Sha256) || (header.MimeType != "text/plain" && header.MimeType != "application/vnd.openxmlformats-officedocument.wordprocessingml.document") {
		return status.Error(codes.InvalidArgument, "invalid original document receipt")
	}
	file, err := os.CreateTemp("", "glowloom-creation-source-*")
	if err != nil {
		return status.Error(codes.Unavailable, "Creation temporary storage unavailable")
	}
	defer func() { _ = file.Close(); _ = os.Remove(file.Name()) }()
	hash := sha256.New()
	var size int64
	for {
		frame, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return receiveError(err)
		}
		if err := ctx.Err(); err != nil {
			return status.FromContextError(err).Err()
		}
		part := frame.GetChunk()
		if len(part) < 1 || len(part) > ChunkSize || int64(len(part)) > header.ByteSize-size {
			return status.Error(codes.InvalidArgument, "invalid document byte stream")
		}
		if _, err := file.Write(part); err != nil {
			return status.Error(codes.Unavailable, "Creation temporary storage unavailable")
		}
		_, _ = hash.Write(part)
		size += int64(len(part))
	}
	if size != header.ByteSize || hex.EncodeToString(hash.Sum(nil)) != header.Sha256 {
		return status.Error(codes.InvalidArgument, "document receipt mismatch")
	}
	result, err := s.extractor.Extract(ctx, &app.File{File: file, Size: size, SHA256: header.Sha256, MIMEType: header.MimeType})
	if err != nil {
		if ctx.Err() != nil {
			return status.FromContextError(ctx.Err()).Err()
		}
		if errors.Is(err, app.ErrExtractionFailed) {
			return status.Error(codes.FailedPrecondition, "document extraction rejected")
		}
		return status.Error(codes.Unavailable, "Creation extraction unavailable")
	}
	data, err := app.EncodeResult(result)
	if err != nil {
		return status.Error(codes.DataLoss, "invalid extracted document receipt")
	}
	for len(data) > 0 {
		if err := ctx.Err(); err != nil {
			return status.FromContextError(err).Err()
		}
		n := min(len(data), ChunkSize)
		if err := stream.Send(&pb.ExtractResponse{JsonChunk: data[:n]}); err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

func validHash(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
func receiveError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.FromContextError(err).Err()
	}
	if errors.Is(err, io.EOF) {
		return status.Error(codes.InvalidArgument, "missing document header")
	}
	if status.Code(err) != codes.Unknown {
		return err
	}
	return status.Error(codes.Unavailable, "document transport unavailable")
}
