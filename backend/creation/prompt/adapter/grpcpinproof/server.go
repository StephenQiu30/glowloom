// Package grpcpinproof exposes original Creation pin consumption decisions to Media.
package grpcpinproof

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationreference"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
)

type server struct {
	pb.UnimplementedReferencePinConsumptionsServer
	reader application.ReferencePinConsumptionReader
}

// NewServer requires Media's dedicated bearer audience and verified certificate.
// The composition root owns serving, stopping and joining the injected listener.
func NewServer(reader application.ReferencePinConsumptionReader, token, mediaDNS string, transport credentials.TransportCredentials) (*grpc.Server, error) {
	if reader == nil || transport == nil || transport.Info().SecurityProtocol != "tls" {
		return nil, domain.ErrFrozenInputInvalid
	}
	authenticate, err := serviceauth.MTLSUnary(token, mediaDNS)
	if err != nil {
		return nil, err
	}
	listener := grpc.NewServer(grpc.Creds(transport.Clone()), grpc.UnaryInterceptor(authenticate),
		grpc.MaxRecvMsgSize(4096), grpc.MaxSendMsgSize(4096), grpc.MaxHeaderListSize(16384))
	pb.RegisterReferencePinConsumptionsServer(listener, &server{reader: reader})
	return listener, nil
}

func (s *server) ReadConsumption(ctx context.Context, wire *pb.ReferencePinConsumptionKey) (*pb.ReferencePinConsumption, error) {
	key, err := decodeKey(wire)
	if err != nil {
		return nil, proofStatus(err)
	}
	value, err := s.reader.ReadReferencePinConsumption(ctx, key)
	if err != nil {
		return nil, proofStatus(err)
	}
	if value.Key != key {
		return nil, proofStatus(domain.ErrFrozenInputConflict)
	}
	var outcome pb.ReferencePinConsumptionOutcome
	switch value.Outcome {
	case "adopted":
		if len(value.SourceSnapshotSHA256) != 64 {
			return nil, proofStatus(domain.ErrFrozenInputConflict)
		}
		outcome = pb.ReferencePinConsumptionOutcome_REFERENCE_PIN_CONSUMPTION_OUTCOME_ADOPTED
	case "not_adopted_frozen":
		if value.SourceSnapshotSHA256 != "" {
			return nil, proofStatus(domain.ErrFrozenInputConflict)
		}
		outcome = pb.ReferencePinConsumptionOutcome_REFERENCE_PIN_CONSUMPTION_OUTCOME_NOT_ADOPTED_FROZEN
	default:
		return nil, proofStatus(domain.ErrReferenceUnavailable)
	}
	return &pb.ReferencePinConsumption{Key: wire, Outcome: outcome, SourceSnapshotSha256: value.SourceSnapshotSHA256}, nil
}

func decodeKey(wire *pb.ReferencePinConsumptionKey) (application.ReferencePinConsumptionKey, error) {
	var key application.ReferencePinConsumptionKey
	if wire == nil || len(wire.ProtoReflect().GetUnknown()) != 0 {
		return key, domain.ErrFrozenInputInvalid
	}
	for _, field := range []struct {
		wire string
		id   *uuid.UUID
	}{
		{wire.PinId, &key.PinID}, {wire.AcquireCommandId, &key.AcquireCommandID}, {wire.SourceCommandId, &key.SourceCommandID},
		{wire.HandoffId, &key.HandoffID}, {wire.ScopeId, &key.ScopeID}, {wire.ProjectId, &key.ProjectID}, {wire.AssetId, &key.AssetID},
	} {
		id, err := uuid.Parse(field.wire)
		if err != nil || id == uuid.Nil || id.String() != field.wire {
			return key, domain.ErrFrozenInputInvalid
		}
		*field.id = id
	}
	key.SourceCommandSHA256, key.PinSnapshotSHA256 = wire.SourceCommandSha256, wire.PinSnapshotSha256
	key.Kind, key.AssetRevision = wire.MediaKind, wire.AssetRevision
	return key, key.Validate()
}

func proofStatus(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	case errors.Is(err, domain.ErrFrozenInputInvalid):
		return status.Error(codes.InvalidArgument, "complete original pin consumption identity required")
	case errors.Is(err, domain.ErrFrozenInputConflict):
		return status.Error(codes.FailedPrecondition, "original pin consumption evidence mismatch")
	default:
		return status.Error(codes.Unavailable, "original pin consumption decision unavailable")
	}
}
