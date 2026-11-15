// Package creationpins consumes current Creation admission for new Media pins.
package creationpins

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationreference"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
	"github.com/StephenQiu30/glowloom-server/backend/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// Target is the explicit Creation admission endpoint and its dedicated Media audience.
type Target struct {
	Address, Token string
	Transport      credentials.TransportCredentials
}

// Client owns its connection and never forwards user credentials to Creation.
type Client struct {
	connection *grpc.ClientConn
	admitter   pb.ReferencePinAdmissionsClient
	token      string
}

var _ application.ReferencePinSourceAdmission = (*Client)(nil)

// New requires verified TLS. The composition root closes it after joining workers.
func New(target Target) (*Client, error) {
	host, port, err := net.SplitHostPort(target.Address)
	number, portErr := strconv.Atoi(port)
	if err != nil || host == "" || strings.ContainsAny(host, "/@?#% ") || strings.IndexFunc(host, unicode.IsSpace) >= 0 ||
		portErr != nil || number < 1 || number > 65535 || !serviceauth.ValidToken(target.Token) ||
		target.Transport == nil || target.Transport.Info().SecurityProtocol != "tls" {
		return nil, domain.ErrReferencePinProof
	}
	connection, err := grpc.NewClient(target.Address, grpc.WithTransportCredentials(target.Transport.Clone()),
		grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(8192), grpc.MaxCallSendMsgSize(8192), grpc.MaxRetryRPCBufferSize(0)))
	if err != nil {
		return nil, err
	}
	return &Client{connection: connection, admitter: pb.NewReferencePinAdmissionsClient(connection), token: target.Token}, nil
}

// Close releases the private channel after callers have stopped.
func (c *Client) Close() error { return c.connection.Close() }

// AdmitReferencePinSource makes a single original-command call. Unknown results
// remain unknown; a retry must use this same command and recheck current policy.
func (c *Client) AdmitReferencePinSource(ctx context.Context, request application.ReferencePinRequest) (application.ReferencePinSourceProof, error) {
	var proof application.ReferencePinSourceProof
	if c == nil || c.admitter == nil || request.Validate() != nil || request.Command.Action != "media.reference_pin.acquire" || len(request.Command.Authorities) != 4 {
		return proof, domain.ErrReferencePinProof
	}
	if _, err := request.Command.AuthorityDigest(); err != nil {
		return proof, err
	}
	wire := &pb.ReferencePinAdmissionRequest{Command: grpctransport.EncodeCommand(request.Command), ScopeId: request.Identity.ScopeID.String(), ProjectId: request.Identity.ProjectID.String()}
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(callCtx, carrier)
	outgoing := metadata.Pairs("authorization", "Bearer "+c.token)
	for _, key := range []string{"traceparent", "tracestate"} {
		if value := carrier[key]; value != "" {
			outgoing.Set(key, value)
		}
	}
	response, err := c.admitter.AdmitAcquisition(metadata.NewOutgoingContext(callCtx, outgoing), wire)
	if err != nil {
		if ctx.Err() != nil {
			return proof, ctx.Err()
		}
		return proof, application.ErrUnavailable
	}
	if response == nil || len(response.ProtoReflect().GetUnknown()) != 0 || !proto.Equal(response.Request, wire) {
		return proof, domain.ErrReferencePinProof
	}
	for _, field := range []struct {
		text string
		id   *uuid.UUID
	}{{response.AccountId, &proof.AccountID}, {response.SessionId, &proof.SessionID}, {response.PhysicalScopeId, &proof.PhysicalScopeID}} {
		id, err := uuid.Parse(field.text)
		if err != nil || id == uuid.Nil || id.String() != field.text {
			return application.ReferencePinSourceProof{}, domain.ErrReferencePinProof
		}
		*field.id = id
	}
	if err := proof.Validate(request); err != nil {
		return application.ReferencePinSourceProof{}, err
	}
	return proof, nil
}
