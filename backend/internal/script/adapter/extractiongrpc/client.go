// Package extractiongrpc connects Creation consumers to the private extractor.
package extractiongrpc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"strings"
	"time"

	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creation"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
)

const chunkSize = 256 << 10
const maxMessageBytes = chunkSize + 1024

// ErrUnavailable is infrastructure failure, not a permanently invalid source.
var ErrUnavailable = errors.New("creation extraction unavailable")

// Client owns only immutable configuration; each RPC owns and closes its channel.
type Client struct {
	target, token string
	transport     credentials.TransportCredentials
}

// New rejects partial identity and never installs a local extraction fallback.
func New(target, token string, transport credentials.TransportCredentials) (*Client, error) {
	if target == "" && token == "" && transport == nil {
		return &Client{}, nil
	}
	host, port, err := net.SplitHostPort(target)
	if err != nil || host == "" || port == "" || strings.ContainsAny(target, "/?#@") || !serviceauth.ValidToken(token) || transport == nil {
		return nil, errors.New("invalid Creation extraction client configuration")
	}
	return &Client{target: target, token: token, transport: transport}, nil
}

// Extract verifies a complete final-OK receipt before exposing editable content.
// Its SectionReader never moves, closes or deletes the caller's original file.
func (c *Client) Extract(ctx context.Context, source *app.File) (app.Result, error) {
	if err := ctx.Err(); err != nil {
		return app.Result{}, err
	}
	if c == nil || c.target == "" {
		return app.Result{}, ErrUnavailable
	}
	if source == nil || source.File == nil || source.Size < 1 || source.Size > app.MaxBytes || (source.MIMEType != "text/plain" && source.MIMEType != "application/vnd.openxmlformats-officedocument.wordprocessingml.document") {
		return app.Result{}, app.ErrExtractionFailed
	}
	digest, err := hex.DecodeString(source.SHA256)
	if err != nil || len(digest) != sha256.Size || source.SHA256 != strings.ToLower(source.SHA256) {
		return app.Result{}, app.ErrExtractionFailed
	}
	info, err := source.File.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != source.Size {
		return app.Result{}, app.ErrExtractionFailed
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	md := metadata.Pairs("authorization", "Bearer "+c.token)
	for key, value := range carrier {
		md.Set(key, value)
	}
	ctx = metadata.NewOutgoingContext(ctx, md)
	conn, err := grpc.NewClient(c.target, grpc.WithTransportCredentials(c.transport.Clone()), grpc.WithDisableRetry(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMessageBytes), grpc.MaxCallSendMsgSize(maxMessageBytes)))
	if err != nil {
		return app.Result{}, ErrUnavailable
	}
	defer func() { _ = conn.Close() }()
	stream, err := pb.NewExtractionClient(conn).Extract(ctx)
	if err != nil {
		return app.Result{}, rpcError(err)
	}
	header := &pb.ExtractRequest{Payload: &pb.ExtractRequest_Source{Source: &pb.Source{ByteSize: source.Size, Sha256: source.SHA256, MimeType: source.MIMEType}}}
	if err := stream.Send(header); err != nil {
		return app.Result{}, sendError(stream, err)
	}
	reader := io.NewSectionReader(source.File, 0, source.Size)
	buf := make([]byte, chunkSize)
	for {
		if err := ctx.Err(); err != nil {
			return app.Result{}, err
		}
		n, err := reader.Read(buf)
		if n > 0 {
			if err := stream.Send(&pb.ExtractRequest{Payload: &pb.ExtractRequest_Chunk{Chunk: buf[:n]}}); err != nil {
				return app.Result{}, sendError(stream, err)
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return app.Result{}, err
		}
	}
	if err := stream.CloseSend(); err != nil {
		return app.Result{}, sendError(stream, err)
	}
	var result bytes.Buffer
	for {
		frame, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return app.Result{}, rpcError(err)
		}
		part := frame.GetJsonChunk()
		if len(part) < 1 || len(part) > chunkSize || len(part) > app.MaxResultBytes-result.Len() {
			return app.Result{}, ErrUnavailable
		}
		_, _ = result.Write(part)
	}
	decoded, err := app.DecodeResult(result.Bytes())
	if err != nil {
		return app.Result{}, ErrUnavailable
	}
	return decoded, ctx.Err()
}
func sendError(stream grpc.BidiStreamingClient[pb.ExtractRequest, pb.ExtractResponse], err error) error {
	if errors.Is(err, io.EOF) {
		if _, received := stream.Recv(); received != nil {
			return rpcError(received)
		}
	}
	return rpcError(err)
}
func rpcError(err error) error {
	switch status.Code(err) {
	case codes.Canceled:
		return context.Canceled
	case codes.DeadlineExceeded:
		return context.DeadlineExceeded
	case codes.InvalidArgument, codes.FailedPrecondition, codes.OutOfRange:
		return app.ErrExtractionFailed
	default:
		return ErrUnavailable
	}
}
