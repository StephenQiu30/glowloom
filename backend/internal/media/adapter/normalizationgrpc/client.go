// Package normalizationgrpc connects the upload use case to private Media RPC.
package normalizationgrpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/media"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
)

const chunkSize = 256 << 10
const maxMessageBytes = chunkSize + 1024
const callBudget = 150 * time.Second

// Client has no durable commands, local fallback, application retry or shared file.
// A connection belongs to one conversion and is always closed with that call.
type Client struct {
	target    string
	token     string
	transport credentials.TransportCredentials
}

// New accepts an explicitly configured target and transport. An entirely absent
// configuration disables WebM conversion without disabling other upload kinds.
func New(target, token string, transport credentials.TransportCredentials) (*Client, error) {
	if target == "" && token == "" {
		return &Client{}, nil
	}
	if target == "" || strings.ContainsAny(target, "/?#@") || !serviceauth.ValidToken(token) || transport == nil {
		return nil, errors.New("invalid media normalization client configuration")
	}
	return &Client{target: target, token: token, transport: transport}, nil
}

// Normalize returns a separately owned local file only after checksum, content
// sniffing, size and the final successful RPC status have all been verified.
func (c *Client) Normalize(ctx context.Context, source *mediaapp.Downloaded) (mediaapp.NormalizedUpload, error) {
	if err := ctx.Err(); err != nil {
		return mediaapp.NormalizedUpload{}, err
	}
	if c == nil || c.target == "" {
		return mediaapp.NormalizedUpload{}, mediaapp.ErrUnavailable
	}
	if source == nil || source.File == nil || source.Size < 1 || source.MIMEType != "video/webm" || !validHash(source.SHA256) {
		return mediaapp.NormalizedUpload{}, mediaapp.ErrInvalidUpload
	}
	if source.Size > mediaapp.MaxUploadVideoBytes {
		return mediaapp.NormalizedUpload{}, mediaapp.ErrUploadTooLarge
	}
	info, err := source.File.Stat()
	if err != nil || info.Size() != source.Size {
		return mediaapp.NormalizedUpload{}, mediaapp.ErrInvalidUpload
	}
	ctx, cancel := context.WithTimeout(ctx, callBudget)
	defer cancel()
	// Forward only W3C trace correlation and this service's own identity, never
	// arbitrary caller metadata, end-user credentials or baggage.
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	outgoing := metadata.Pairs("authorization", "Bearer "+c.token)
	for key, value := range carrier {
		outgoing.Set(key, value)
	}
	ctx = metadata.NewOutgoingContext(ctx, outgoing)
	conn, err := grpc.NewClient(c.target, grpc.WithTransportCredentials(c.transport.Clone()), grpc.WithDisableRetry(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMessageBytes), grpc.MaxCallSendMsgSize(maxMessageBytes)))
	if err != nil {
		return mediaapp.NormalizedUpload{}, fmt.Errorf("connect media normalizer: %w", mediaapp.ErrUnavailable)
	}
	defer func() { _ = conn.Close() }()
	stream, err := pb.NewNormalizationClient(conn).Normalize(ctx)
	if err != nil {
		return mediaapp.NormalizedUpload{}, rpcError(err)
	}
	// No goroutine or unbounded send queue: the protocol receives the entire
	// source before emitting a header, and HTTP/2 flow control bounds each send.
	header := &pb.Source{ByteSize: source.Size, Sha256: source.SHA256, MimeType: source.MIMEType}
	if err := stream.Send(&pb.NormalizeRequest{Payload: &pb.NormalizeRequest_Source{Source: header}}); err != nil {
		return mediaapp.NormalizedUpload{}, sendError(stream, err)
	}
	reader := io.NewSectionReader(source.File, 0, source.Size)
	for {
		if err := ctx.Err(); err != nil {
			return mediaapp.NormalizedUpload{}, err
		}
		buf := make([]byte, chunkSize)
		n, err := reader.Read(buf)
		if n > 0 {
			if err := stream.Send(&pb.NormalizeRequest{Payload: &pb.NormalizeRequest_Chunk{Chunk: buf[:n]}}); err != nil {
				return mediaapp.NormalizedUpload{}, sendError(stream, err)
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return mediaapp.NormalizedUpload{}, fmt.Errorf("read normalization source: %w", err)
		}
	}
	if err := stream.CloseSend(); err != nil {
		return mediaapp.NormalizedUpload{}, sendError(stream, err)
	}
	return receiveCanonical(ctx, stream)
}

func receiveCanonical(ctx context.Context, stream grpc.BidiStreamingClient[pb.NormalizeRequest, pb.NormalizeResponse]) (mediaapp.NormalizedUpload, error) {
	first, err := stream.Recv()
	if err != nil {
		return mediaapp.NormalizedUpload{}, rpcError(err)
	}
	header := first.GetCanonical()
	if header == nil || header.ByteSize < 1 || header.ByteSize > mediaapp.MaxUploadVideoBytes || !validHash(header.Sha256) || header.MimeType != "video/mp4" || (header.SourceCodec != "vp8" && header.SourceCodec != "vp9") {
		return mediaapp.NormalizedUpload{}, mediaapp.ErrUnavailable
	}
	file, err := os.CreateTemp("", "glowloom-media-received-*.mp4")
	if err != nil {
		return mediaapp.NormalizedUpload{}, fmt.Errorf("create canonical media: %w", err)
	}
	result := &mediaapp.Downloaded{File: file, Size: header.ByteSize, SHA256: header.Sha256, MIMEType: header.MimeType}
	keep := false
	defer func() {
		if !keep {
			_ = result.Close()
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
			return mediaapp.NormalizedUpload{}, rpcError(err)
		}
		if err := ctx.Err(); err != nil {
			return mediaapp.NormalizedUpload{}, err
		}
		chunk := frame.GetChunk()
		if len(chunk) < 1 || len(chunk) > chunkSize || int64(len(chunk)) > header.ByteSize-size {
			return mediaapp.NormalizedUpload{}, mediaapp.ErrUnavailable
		}
		if _, err := file.Write(chunk); err != nil {
			return mediaapp.NormalizedUpload{}, fmt.Errorf("write canonical media: %w", err)
		}
		_, _ = hash.Write(chunk)
		size += int64(len(chunk))
	}
	if size != header.ByteSize || hex.EncodeToString(hash.Sum(nil)) != header.Sha256 {
		return mediaapp.NormalizedUpload{}, mediaapp.ErrUnavailable
	}
	var magic [512]byte
	n, err := file.ReadAt(magic[:], 0)
	if (err != nil && !errors.Is(err, io.EOF)) || http.DetectContentType(magic[:n]) != "video/mp4" {
		return mediaapp.NormalizedUpload{}, mediaapp.ErrUnavailable
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return mediaapp.NormalizedUpload{}, fmt.Errorf("rewind canonical media: %w", err)
	}
	keep = true
	return mediaapp.NormalizedUpload{File: result, SourceCodec: header.SourceCodec}, nil
}

func sendError(stream grpc.BidiStreamingClient[pb.NormalizeRequest, pb.NormalizeResponse], sendErr error) error {
	// Send can return EOF when the server rejected a frame. Recv is required to
	// observe its final status rather than classifying that EOF as success.
	if errors.Is(sendErr, io.EOF) {
		_, err := stream.Recv()
		if err != nil {
			return rpcError(err)
		}
	}
	return rpcError(sendErr)
}

func validHash(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func rpcError(err error) error {
	switch status.Code(err) {
	case codes.Canceled:
		return context.Canceled
	case codes.DeadlineExceeded:
		return context.DeadlineExceeded
	case codes.InvalidArgument:
		return mediaapp.ErrInvalidUpload
	case codes.FailedPrecondition:
		return mediaapp.ErrUnsupportedUpload
	case codes.OutOfRange:
		return mediaapp.ErrUploadTooLarge
	default:
		return mediaapp.ErrUnavailable
	}
}
