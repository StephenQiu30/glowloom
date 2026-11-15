// Package grpcreference serves original Media acquisition evidence to Creation.
package grpcreference

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediareference"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

const maxMessageBytes = 4096

type server struct {
	pb.UnimplementedCreationReferencePinsServer
	reader application.ReferencePinReceiptReader
}

// NewServer requires a dedicated Creation audience and a verified mTLS listener.
// The caller owns Serve, graceful shutdown and the injected Media database pool.
func NewServer(reader application.ReferencePinReceiptReader, token, creationDNS string, transport credentials.TransportCredentials) (*grpc.Server, error) {
	if reader == nil || transport == nil || transport.Info().SecurityProtocol != "tls" {
		return nil, domain.ErrInvalidReferencePin
	}
	authenticate, err := serviceauth.MTLSUnary(token, creationDNS)
	if err != nil {
		return nil, err
	}
	listener := grpc.NewServer(grpc.Creds(transport.Clone()), grpc.UnaryInterceptor(authenticate),
		grpc.MaxRecvMsgSize(maxMessageBytes), grpc.MaxSendMsgSize(maxMessageBytes), grpc.MaxHeaderListSize(16384))
	pb.RegisterCreationReferencePinsServer(listener, &server{reader: reader})
	return listener, nil
}

func (s *server) ReadAcquisition(ctx context.Context, wire *pb.ReferencePinReceiptKey) (*pb.ReferencePinReceipt, error) {
	key, err := decodeKey(wire)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "complete Media acquisition identity required")
	}
	pin, err := s.reader.ReadReferencePinReceipt(ctx, key)
	if err != nil {
		return nil, receiptStatus(err)
	}
	i := pin.Identity
	if i.PinID != key.PinID || pin.AcquireCommandID != key.AcquireCommandID || i.Consumer != key.Consumer ||
		i.ConsumerCommandID != key.ConsumerCommandID || i.ConsumerCommandSHA256 != key.ConsumerCommandSHA256 ||
		i.ConsumerResourceID != key.ConsumerResourceID || i.ScopeID != key.ScopeID || i.ProjectID != key.ProjectID ||
		i.AssetID == uuid.Nil || i.AssetRevision < 1 || pin.Kind == "" {
		return nil, receiptStatus(domain.ErrReferencePinProof)
	}
	retention := map[string]pb.ReferencePinRetention{
		"held":     pb.ReferencePinRetention_REFERENCE_PIN_RETENTION_HELD,
		"bound":    pb.ReferencePinRetention_REFERENCE_PIN_RETENTION_BOUND,
		"released": pb.ReferencePinRetention_REFERENCE_PIN_RETENTION_RELEASED,
	}[pin.State]
	if retention == pb.ReferencePinRetention_REFERENCE_PIN_RETENTION_UNSPECIFIED {
		return nil, receiptStatus(domain.ErrReferencePinProof)
	}
	return &pb.ReferencePinReceipt{
		Key: wire, AssetId: i.AssetID.String(), AssetRevision: i.AssetRevision,
		MediaKind: pin.Kind, SnapshotSha256: pin.SnapshotSHA256(), Retention: retention,
	}, nil
}

func decodeKey(wire *pb.ReferencePinReceiptKey) (application.ReferencePinReceiptKey, error) {
	var key application.ReferencePinReceiptKey
	if wire == nil || len(wire.ProtoReflect().GetUnknown()) != 0 {
		return key, domain.ErrInvalidReferencePin
	}
	for _, field := range []struct {
		wire string
		id   *uuid.UUID
	}{
		{wire.PinId, &key.PinID}, {wire.AcquireCommandId, &key.AcquireCommandID},
		{wire.ConsumerCommandId, &key.ConsumerCommandID}, {wire.ConsumerResourceId, &key.ConsumerResourceID},
		{wire.ScopeId, &key.ScopeID}, {wire.ProjectId, &key.ProjectID},
	} {
		id, err := uuid.Parse(field.wire)
		if err != nil || id == uuid.Nil || id.String() != field.wire {
			return key, domain.ErrInvalidReferencePin
		}
		*field.id = id
	}
	key.Consumer = "creation"
	key.ConsumerCommandSHA256 = wire.ConsumerCommandSha256
	return key, key.Validate()
}

func receiptStatus(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	case errors.Is(err, domain.ErrReferencePinNotFound):
		return status.Error(codes.NotFound, "Media acquisition not found")
	case errors.Is(err, domain.ErrInvalidReferencePin):
		return status.Error(codes.InvalidArgument, "invalid Media acquisition identity")
	case errors.Is(err, domain.ErrReferencePinConflict), errors.Is(err, domain.ErrReferencePinProof):
		return status.Error(codes.FailedPrecondition, "Media acquisition evidence mismatch")
	default:
		return status.Error(codes.Unavailable, "Media acquisition evidence unavailable")
	}
}
