// Package mediaoutput adapts Media's private protocol without importing Media business code.
package mediaoutput

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediapublication"
	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/outputpublication"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

const chunkSize = 256 << 10
const messageBytes = chunkSize + 16384

// Target requires verified private peer credentials and a dedicated publication token.
type Target struct {
	Address, Token string
	Transport      credentials.TransportCredentials
}

// Client never retries an unknown publication or falls back to shared database access.
type Client struct{ target Target }

// New requires an explicit private Media endpoint, dedicated token and peer transport.
func New(target Target) (*Client, error) {
	host, port, err := net.SplitHostPort(target.Address)
	number, e := strconv.Atoi(port)
	if err != nil || e != nil || host == "" || strings.ContainsAny(host, "/@?#% \t\r\n") || number < 1 || number > 65535 || !serviceauth.ValidToken(target.Token) || target.Transport == nil {
		return nil, contract.ErrInvalidOutputPublication
	}
	target.Transport = target.Transport.Clone()
	return &Client{target: target}, nil
}
func (c *Client) connect(ctx context.Context) (context.Context, context.CancelFunc, *grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	md := metadata.Pairs("authorization", "Bearer "+c.target.Token)
	for _, key := range []string{"traceparent", "tracestate"} {
		if value := carrier[key]; value != "" {
			md.Set(key, value)
		}
	}
	ctx = metadata.NewOutgoingContext(ctx, md)
	conn, err := grpc.NewClient(c.target.Address, grpc.WithTransportCredentials(c.target.Transport.Clone()), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(messageBytes), grpc.MaxCallSendMsgSize(messageBytes), grpc.MaxRetryRPCBufferSize(0)))
	if err != nil {
		cancel()
		return nil, nil, nil, writefence.ErrUnavailable
	}
	return ctx, cancel, conn, nil
}

// Publish transmits the exact original header/body once, then requires final RPC status.
func (c *Client) Publish(ctx context.Context, req contract.Request, source io.Reader) (contract.OperationOutputReceipt, error) {
	var out contract.OperationOutputReceipt
	if c == nil || source == nil || req.Validate() != nil {
		return out, contract.ErrInvalidOutputPublication
	}
	ctx, cancel, conn, err := c.connect(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer func() { _ = conn.Close() }()
	stream, err := pb.NewOperationOutputsClient(conn).PublishOperationOutput(ctx)
	if err != nil {
		return out, localError(err)
	}
	err = stream.Send(&pb.PublishOperationOutputFrame{Payload: &pb.PublishOperationOutputFrame_Publication{Publication: contract.EncodeRequest(req)}})
	if err == nil {
		reader := io.LimitReader(source, req.Output.ByteSize+1)
		for {
			buf := make([]byte, chunkSize)
			n, readErr := reader.Read(buf)
			if n > 0 {
				err = stream.Send(&pb.PublishOperationOutputFrame{Payload: &pb.PublishOperationOutputFrame_Chunk{Chunk: buf[:n]}})
				if err != nil {
					break
				}
			}
			if readErr != nil {
				if !errors.Is(readErr, io.EOF) {
					cancel()
					return out, readErr
				}
				break
			}
		}
	}
	// A replay may return its original ACK while the client is still sending.
	// CloseAndRecv is mandatory even when Send observed EOF.
	response, recvErr := stream.CloseAndRecv()
	if recvErr != nil {
		return out, localError(recvErr)
	}
	out, err = contract.DecodeReceipt(response)
	if err != nil {
		return out, err
	}
	return out, req.Matches(out)
}

// Resolve queries only the original command, without resubmitting bytes or supplier work.
func (c *Client) Resolve(ctx context.Context, req contract.Request) (contract.OperationOutputReceipt, error) {
	var out contract.OperationOutputReceipt
	if c == nil || req.Validate() != nil {
		return out, contract.ErrInvalidOutputPublication
	}
	ctx, cancel, conn, err := c.connect(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer func() { _ = conn.Close() }()
	response, err := pb.NewOperationOutputsClient(conn).ResolveOperationOutput(ctx, contract.EncodeRequest(req))
	if err != nil {
		return out, localError(err)
	}
	out, err = contract.DecodeReceipt(response)
	if err != nil {
		return out, err
	}
	return out, req.Matches(out)
}
func localError(err error) error {
	switch status.Code(err) {
	case codes.NotFound:
		return contract.ErrOutputPublicationUnknown
	case codes.AlreadyExists:
		return contract.ErrOutputPublicationConflict
	case codes.InvalidArgument:
		return contract.ErrInvalidOutputPublication
	case codes.FailedPrecondition, codes.PermissionDenied:
		return writefence.ErrFenced
	case codes.Canceled:
		return context.Canceled
	case codes.DeadlineExceeded:
		return context.DeadlineExceeded
	default:
		return writefence.ErrUnavailable
	}
}
