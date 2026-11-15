// Package mediapins verifies Media's original pin receipts before local adoption.
package mediapins

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediareference"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
)

// Target identifies Media's private receipt listener and its dedicated audience.
type Target struct {
	Address, Token string
	Transport      credentials.TransportCredentials
}

// Client owns its connection; the Creation root must close it after joining work.
type Client struct {
	connection *grpc.ClientConn
	reader     pb.CreationReferencePinsClient
	token      string
}

var _ application.FrozenInputPins = (*Client)(nil)

// New requires explicit mTLS and never enables a local database fallback.
func New(target Target) (*Client, error) {
	host, port, err := net.SplitHostPort(target.Address)
	number, portErr := strconv.Atoi(port)
	if err != nil || host == "" || portErr != nil || number < 1 || number > 65535 ||
		!serviceauth.ValidToken(target.Token) || target.Transport == nil || target.Transport.Info().SecurityProtocol != "tls" {
		return nil, domain.ErrReferenceUnavailable
	}
	connection, err := grpc.NewClient(target.Address, grpc.WithTransportCredentials(target.Transport.Clone()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(4096), grpc.MaxCallSendMsgSize(4096)))
	if err != nil {
		return nil, err
	}
	return &Client{connection: connection, reader: pb.NewCreationReferencePinsClient(connection), token: target.Token}, nil
}

// Close releases the caller-owned gRPC transport after all work has stopped.
func (c *Client) Close() error { return c.connection.Close() }

// VerifyFrozenInputPins binds every read to Creation's separate source command,
// not Execution's acceptance command or a caller-supplied new authority.
func (c *Client) VerifyFrozenInputPins(ctx context.Context, request application.FreezeInputRequest) ([]domain.FrozenPin, error) {
	if c == nil || c.reader == nil || request.HandoffID == uuid.Nil || request.ScopeID == uuid.Nil || request.ProjectID == uuid.Nil ||
		request.SourceCommand.ID != uuid.NewSHA1(request.HandoffID, []byte("creation.frozen_input.freeze")) ||
		request.SourceCommand.Action != "creation.frozen_input.freeze" || request.Pins == nil || len(request.Pins) > 255 {
		return nil, domain.ErrReferenceUnavailable
	}
	digest, err := request.SourceCommandSHA256()
	if err != nil || digest != request.SourceCommand.SHA256 {
		return nil, domain.ErrFrozenInputConflict
	}
	verified := make([]domain.FrozenPin, 0, len(request.Pins))
	seen := make(map[uuid.UUID]bool, len(request.Pins))
	for _, pin := range request.Pins {
		if pin.PinID == uuid.Nil || pin.AcquireCommandID == uuid.Nil || pin.AssetID == uuid.Nil || seen[pin.PinID] {
			return nil, domain.ErrFrozenInputConflict
		}
		seen[pin.PinID] = true
		key := &pb.ReferencePinReceiptKey{
			PinId: pin.PinID.String(), AcquireCommandId: pin.AcquireCommandID.String(),
			ConsumerCommandId: request.SourceCommand.ID.String(), ConsumerCommandSha256: request.SourceCommand.SHA256,
			ConsumerResourceId: request.HandoffID.String(), ScopeId: request.ScopeID.String(), ProjectId: request.ProjectID.String(),
		}
		callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		receipt, err := c.reader.ReadAcquisition(c.outgoing(callCtx), key)
		cancel()
		if err != nil {
			return nil, domain.ErrReferenceUnavailable
		}
		if receipt == nil || len(receipt.ProtoReflect().GetUnknown()) != 0 || receipt.Key == nil ||
			len(receipt.Key.ProtoReflect().GetUnknown()) != 0 || !proto.Equal(receipt.Key, key) ||
			receipt.AssetId != pin.AssetID.String() || receipt.AssetRevision != pin.AssetRevision ||
			receipt.MediaKind != pin.Kind || receipt.SnapshotSha256 != pin.SnapshotSHA256 {
			return nil, domain.ErrFrozenInputConflict
		}
		if receipt.Retention != pb.ReferencePinRetention_REFERENCE_PIN_RETENTION_HELD && receipt.Retention != pb.ReferencePinRetention_REFERENCE_PIN_RETENTION_BOUND {
			return nil, domain.ErrReferenceUnavailable
		}
		verified = append(verified, pin)
	}
	return verified, nil
}

func (c *Client) outgoing(ctx context.Context) context.Context {
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	outgoing := metadata.Pairs("authorization", "Bearer "+c.token)
	for _, key := range []string{"traceparent", "tracestate"} {
		if value := carrier[key]; value != "" {
			outgoing.Set(key, value)
		}
	}
	return metadata.NewOutgoingContext(ctx, outgoing)
}
