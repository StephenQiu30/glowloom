// Package creationpreparation consumes only Creation's immutable preparation wire facts.
package creationpreparation

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

	"github.com/StephenQiu30/glowloom-server/backend/execution/billing/application"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationpreparation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// Target fixes one private Creation peer and a separate Execution source-read token.
type Target struct {
	Address, Token string
	Transport      credentials.TransportCredentials
}

// Client reads already committed source facts without actor or credential forwarding.
type Client struct{ target Target }

// New validates the complete peer configuration before any source access.
func New(target Target) (*Client, error) {
	host, port, err := net.SplitHostPort(target.Address)
	number, e := strconv.Atoi(port)
	if err != nil || e != nil || host == "" || strings.ContainsAny(host, "/@?#% \t\r\n") || number < 1 || number > 65535 || !serviceauth.ValidToken(target.Token) || target.Transport == nil {
		return nil, application.ErrProjectBudgetPreparation
	}
	target.Transport = target.Transport.Clone()
	return &Client{target: target}, nil
}

// ResolveProjectPreparation requires an exact closed source echo and prepared phase.
// A 404, timeout or unavailable peer is never evidence that the source is ready.
func (c *Client) ResolveProjectPreparation(ctx context.Context, p application.ProjectBudgetPreparation) (application.ProjectPreparedProof, error) {
	var proof application.ProjectPreparedProof
	if c == nil || p.Validate() != nil {
		return proof, application.ErrProjectBudgetPreparation
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
	conn, err := grpc.NewClient(c.target.Address, grpc.WithTransportCredentials(c.target.Transport.Clone()), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(16384), grpc.MaxCallSendMsgSize(16384), grpc.MaxRetryRPCBufferSize(0)))
	if err != nil {
		return proof, writefence.ErrUnavailable
	}
	defer func() { _ = conn.Close() }()
	request := &pb.ProjectPreparationKey{PreparationId: p.PreparationID[:], SourceCommandId: p.SourceCommandID[:], SourceSha256: p.SourceSHA256, InputSha256: p.InputSHA256, ExecutionCommandId: p.ExecutionCommandID[:], ProjectId: p.ProjectID[:], OrganizationId: p.OrganizationID[:], ScopeId: p.ScopeID[:]}
	response, err := pb.NewProjectPreparationsClient(conn).ResolveProjectPreparation(ctx, request)
	if err != nil {
		switch status.Code(err) {
		case codes.NotFound:
			return proof, application.ErrProjectBudgetUnknown
		case codes.InvalidArgument, codes.AlreadyExists, codes.FailedPrecondition:
			return proof, application.ErrProjectBudgetPreparation
		default:
			return proof, writefence.ErrUnavailable
		}
	}
	if response == nil || len(response.ProtoReflect().GetUnknown()) != 0 || response.GetRequest() == nil || len(response.GetRequest().ProtoReflect().GetUnknown()) != 0 || !proto.Equal(response.GetRequest(), request) || response.GetPhase() != "prepared" {
		return proof, application.ErrProjectBudgetPreparation
	}
	return application.ProjectPreparedProof{Request: p, Phase: "prepared"}, nil
}

var _ application.ProjectPreparationSource = (*Client)(nil)
