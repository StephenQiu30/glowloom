// Package creationpinproof verifies durable Creation consumption before Media custody changes.
package creationpinproof

import (
	"context"
	"net"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationreference"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// Target identifies Creation's private consumer-proof listener and Media audience.
type Target struct {
	Address, Token string
	Transport      credentials.TransportCredentials
}

// Client owns the authenticated connection used outside Media SQL transactions.
type Client struct {
	connection *grpc.ClientConn
	reader     pb.ReferencePinConsumptionsClient
	token      string
}

var _ application.ReferencePinConsumerVerifier = (*Client)(nil)

// New requires explicit verified TLS, an address and a dedicated service token.
func New(target Target) (*Client, error) {
	host, port, err := net.SplitHostPort(target.Address)
	number, portErr := strconv.Atoi(port)
	if err != nil || host == "" || portErr != nil || number < 1 || number > 65535 || !serviceauth.ValidToken(target.Token) ||
		target.Transport == nil || target.Transport.Info().SecurityProtocol != "tls" {
		return nil, domain.ErrReferencePinProof
	}
	connection, err := grpc.NewClient(target.Address, grpc.WithTransportCredentials(target.Transport.Clone()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(4096), grpc.MaxCallSendMsgSize(4096)))
	if err != nil {
		return nil, err
	}
	return &Client{connection: connection, reader: pb.NewReferencePinConsumptionsClient(connection), token: target.Token}, nil
}

// Close releases the connection after the owner has joined every active worker.
func (c *Client) Close() error { return c.connection.Close() }

// VerifyAdoption requires committed source adoption of this exact pin snapshot.
func (c *Client) VerifyAdoption(ctx context.Context, pin domain.ReferencePin) (domain.ReferencePinProof, error) {
	return c.verify(ctx, pin, "adopted")
}

// VerifyRelease supports only permanent non-adoption. Historical adopted pins
// cannot be released without a separately implemented exact removal responsibility.
func (c *Client) VerifyRelease(ctx context.Context, pin domain.ReferencePin) (domain.ReferencePinProof, error) {
	return c.verify(ctx, pin, "not_adopted_frozen")
}

func (c *Client) verify(ctx context.Context, pin domain.ReferencePin, expected string) (domain.ReferencePinProof, error) {
	if c == nil || c.reader == nil || pin.Identity.Validate() != nil || pin.Identity.Consumer != "creation" || pin.Kind == "" {
		return domain.ReferencePinProof{}, domain.ErrReferencePinProof
	}
	i := pin.Identity
	key := &pb.ReferencePinConsumptionKey{
		PinId: i.PinID.String(), AcquireCommandId: pin.AcquireCommandID.String(), SourceCommandId: i.ConsumerCommandID.String(),
		SourceCommandSha256: i.ConsumerCommandSHA256, HandoffId: i.ConsumerResourceID.String(), ScopeId: i.ScopeID.String(), ProjectId: i.ProjectID.String(),
		AssetId: i.AssetID.String(), AssetRevision: i.AssetRevision, MediaKind: pin.Kind, PinSnapshotSha256: pin.SnapshotSHA256(),
	}
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
	receipt, err := c.reader.ReadConsumption(metadata.NewOutgoingContext(callCtx, outgoing), key)
	if err != nil {
		if ctx.Err() != nil {
			return domain.ReferencePinProof{}, ctx.Err()
		}
		return domain.ReferencePinProof{}, application.ErrUnavailable
	}
	if receipt == nil || len(receipt.ProtoReflect().GetUnknown()) != 0 || receipt.Key == nil ||
		len(receipt.Key.ProtoReflect().GetUnknown()) != 0 || !proto.Equal(receipt.Key, key) {
		return domain.ReferencePinProof{}, domain.ErrReferencePinProof
	}
	var outcome string
	switch receipt.Outcome {
	case pb.ReferencePinConsumptionOutcome_REFERENCE_PIN_CONSUMPTION_OUTCOME_ADOPTED:
		if !domain.ReferencePinDigestValid(receipt.SourceSnapshotSha256) {
			return domain.ReferencePinProof{}, domain.ErrReferencePinProof
		}
		outcome = "adopted"
	case pb.ReferencePinConsumptionOutcome_REFERENCE_PIN_CONSUMPTION_OUTCOME_NOT_ADOPTED_FROZEN:
		if receipt.SourceSnapshotSha256 != "" {
			return domain.ReferencePinProof{}, domain.ErrReferencePinProof
		}
		outcome = "not_adopted_frozen"
	default:
		return domain.ReferencePinProof{}, domain.ErrReferencePinProof
	}
	if outcome != expected {
		return domain.ReferencePinProof{}, domain.ErrReferencePinProof
	}
	raw, err := protojson.Marshal(receipt)
	if err != nil {
		return domain.ReferencePinProof{}, domain.ErrReferencePinProof
	}
	return domain.ReferencePinProof{Identity: i, AcquireCommandID: pin.AcquireCommandID, SnapshotSHA256: pin.SnapshotSHA256(), Outcome: outcome, Receipt: raw}, nil
}
