package grpcpublication

import (
	"context"
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
	"google.golang.org/protobuf/proto"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediapublication"
	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/outputpublication"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// ProofTarget carries only the dedicated private peer address/token/mTLS configuration.
type ProofTarget struct {
	Address, Token string
	Transport      credentials.TransportCredentials
}

// ProofClient cannot ask Execution to submit, charge, adopt or modify a command.
type ProofClient struct{ target ProofTarget }

// NewProofClient requires explicit authentication and never opens a shared database.
func NewProofClient(target ProofTarget) (*ProofClient, error) {
	host, port, err := net.SplitHostPort(target.Address)
	number, e := strconv.Atoi(port)
	if err != nil || e != nil || host == "" || strings.ContainsAny(host, "/@?#% \t\r\n") || number < 1 || number > 65535 || !serviceauth.ValidToken(target.Token) || target.Transport == nil {
		return nil, contract.ErrInvalidOutputPublication
	}
	target.Transport = target.Transport.Clone()
	return &ProofClient{target: target}, nil
}

// VerifyOutputPublication checks the exact original immutable header, outside Media SQL.
func (c *ProofClient) VerifyOutputPublication(ctx context.Context, req contract.Request) error {
	if c == nil || req.Validate() != nil {
		return contract.ErrInvalidOutputPublication
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	md := metadata.Pairs("authorization", "Bearer "+c.target.Token)
	for _, key := range []string{"traceparent", "tracestate"} {
		if value := carrier[key]; value != "" {
			md.Set(key, value)
		}
	}
	ctx = metadata.NewOutgoingContext(ctx, md)
	conn, err := grpc.NewClient(c.target.Address, grpc.WithTransportCredentials(c.target.Transport.Clone()), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(MaxMessageBytes), grpc.MaxCallSendMsgSize(MaxMessageBytes), grpc.MaxRetryRPCBufferSize(0)))
	if err != nil {
		return writefence.ErrUnavailable
	}
	defer func() { _ = conn.Close() }()
	original := contract.EncodeRequest(req)
	response, err := pb.NewOutputDeliveryProofClient(conn).VerifyOutputPublication(ctx, original)
	if err != nil {
		switch status.Code(err) {
		case codes.NotFound:
			return contract.ErrOutputPublicationUnknown
		case codes.AlreadyExists:
			return contract.ErrOutputPublicationConflict
		case codes.InvalidArgument:
			return contract.ErrInvalidOutputPublication
		default:
			return writefence.ErrUnavailable
		}
	}
	if !contract.Closed(response) || !proto.Equal(original, response) {
		return contract.ErrOutputPublicationConflict
	}
	return nil
}
