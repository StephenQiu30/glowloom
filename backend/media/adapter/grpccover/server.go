// Package grpccover exposes closed-project cover facts to Creation.
package grpccover

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediacover"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	"github.com/StephenQiu30/glowloom-server/backend/media/application"
)

type server struct {
	pb.UnimplementedProjectCoverStatesServer
	reader application.ProjectCoverReader
	slots  chan struct{}
}

// NewServer requires Creation's dedicated audience and DNS-verified client TLS.
// The composition root owns the listener, serving and bounded shutdown.
func NewServer(reader application.ProjectCoverReader, token, creationDNS string, transport credentials.TransportCredentials) (*grpc.Server, error) {
	if reader == nil || transport == nil || transport.Info().SecurityProtocol != "tls" {
		return nil, writefence.ErrInvalid
	}
	authenticate, err := serviceauth.MTLSUnary(token, creationDNS)
	if err != nil {
		return nil, err
	}
	listener := grpc.NewServer(grpc.Creds(transport.Clone()), grpc.UnaryInterceptor(authenticate), grpc.MaxRecvMsgSize(4096), grpc.MaxSendMsgSize(8192), grpc.MaxHeaderListSize(16384))
	pb.RegisterProjectCoverStatesServer(listener, &server{reader: reader, slots: make(chan struct{}, 16)})
	return listener, nil
}

func (s *server) ReadProjectCover(ctx context.Context, wire *pb.ProjectCoverRequest) (*pb.ProjectCoverObservation, error) {
	request, err := decodeRequest(wire)
	if err != nil {
		return nil, coverStatus(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, coverStatus(err)
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return nil, status.Error(codes.ResourceExhausted, "cover observation capacity reached")
	}
	value, err := s.reader.ReadProjectCover(ctx, request)
	if err != nil {
		return nil, coverStatus(err)
	}
	if value.Request != request || value.AssetRevision < 0 || value.Available && value.AssetRevision < 1 {
		return nil, coverStatus(writefence.ErrConflict)
	}
	return &pb.ProjectCoverObservation{Request: wire, Available: value.Available, AssetRevision: value.AssetRevision}, nil
}

func decodeRequest(wire *pb.ProjectCoverRequest) (application.ProjectCoverRequest, error) {
	var request application.ProjectCoverRequest
	if wire == nil || len(wire.ProtoReflect().GetUnknown()) != 0 {
		return request, writefence.ErrInvalid
	}
	for _, field := range []struct {
		raw []byte
		id  *uuid.UUID
	}{
		{wire.CloseId, &request.CloseID}, {wire.ProjectId, &request.ProjectID}, {wire.OrganizationId, &request.OrganizationID},
		{wire.ScopeId, &request.ScopeID}, {wire.AssetId, &request.AssetID}, {wire.CoordinationId, &request.CoordinationID},
	} {
		value, err := uuid.FromBytes(field.raw)
		if err != nil || value == uuid.Nil {
			return request, writefence.ErrInvalid
		}
		*field.id = value
	}
	request.ProjectEpoch = wire.ProjectEpoch
	return request, request.Validate()
}

func coverStatus(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	case errors.Is(err, writefence.ErrInvalid):
		return status.Error(codes.InvalidArgument, "complete original closure and cover identity required")
	case errors.Is(err, writefence.ErrFenced), errors.Is(err, writefence.ErrConflict):
		return status.Error(codes.FailedPrecondition, "original project closure binding unavailable")
	default:
		return status.Error(codes.Unavailable, "cover observation unavailable")
	}
}
