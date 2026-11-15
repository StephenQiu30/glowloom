// Package grpcprojectwork exposes Media's read-only project responsibilities to Creation.
package grpcprojectwork

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediawork"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
	"github.com/StephenQiu30/glowloom-server/backend/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

type server struct {
	pb.UnimplementedProjectWorkServer
	reader application.ProjectWorkReader
}

// NewServer requires Creation's dedicated audience and verified DNS certificate.
// The composition root owns listening, cancellation, graceful stop and join.
func NewServer(reader application.ProjectWorkReader, token, creationDNS string, transport credentials.TransportCredentials) (*grpc.Server, error) {
	if reader == nil || transport == nil || transport.Info().SecurityProtocol != "tls" {
		return nil, writefence.ErrInvalid
	}
	authenticate, err := serviceauth.MTLSUnary(token, creationDNS)
	if err != nil {
		return nil, err
	}
	slots := make(chan struct{}, 16)
	intercept := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return authenticate(ctx, req, info, func(ctx context.Context, req any) (any, error) {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			default:
				return nil, status.Error(codes.ResourceExhausted, "project work reader capacity reached")
			}
			bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			return handler(bounded, req)
		})
	}
	listener := grpc.NewServer(grpc.Creds(transport.Clone()), grpc.UnaryInterceptor(intercept), grpc.MaxRecvMsgSize(512<<10), grpc.MaxSendMsgSize(2<<20), grpc.MaxHeaderListSize(16<<10))
	pb.RegisterProjectWorkServer(listener, &server{reader: reader})
	return listener, nil
}

func (s *server) ReadProjectWork(ctx context.Context, wire *pb.ProjectWorkRequest) (*pb.ProjectWorkSnapshot, error) {
	request, err := decodeRequest(wire)
	if err != nil {
		return nil, workStatus(err)
	}
	value, err := s.reader.ReadProjectWork(ctx, request)
	if err != nil {
		return nil, workStatus(err)
	}
	if len(value.Commands) != len(request.Commands) || !sameRequest(value.Request, request) {
		return nil, workStatus(writefence.ErrConflict)
	}
	out := &pb.ProjectWorkSnapshot{Request: wire, Commands: make([]*pb.CommandObservation, 0, len(value.Commands))}
	switch value.BusinessState {
	case application.ProjectWorkIdle:
		out.BusinessState = pb.BusinessState_IDLE
	case application.ProjectWorkBusy:
		out.BusinessState = pb.BusinessState_BUSY
	case application.ProjectWorkUnknown:
		out.BusinessState = pb.BusinessState_UNKNOWN
	default:
		return nil, workStatus(writefence.ErrConflict)
	}
	for i, observation := range value.Commands {
		if !sameCommand(observation.Command, request.Commands[i]) {
			return nil, workStatus(writefence.ErrConflict)
		}
		if observation.State == "committed" && !domain.ReferencePinDigestValid(observation.ReceiptSHA256) || observation.State != "committed" && observation.ReceiptSHA256 != "" {
			return nil, workStatus(writefence.ErrConflict)
		}
		row := &pb.CommandObservation{Command: wire.Commands[i], ReceiptSha256: observation.ReceiptSHA256}
		switch observation.State {
		case "committed":
			row.State = pb.CommandState_COMMITTED
		case "frozen":
			row.State = pb.CommandState_FROZEN
		case "admitted":
			row.State = pb.CommandState_ADMITTED
		case "unknown":
			row.State = pb.CommandState_COMMAND_UNKNOWN
		default:
			return nil, workStatus(writefence.ErrConflict)
		}
		out.Commands = append(out.Commands, row)
	}
	return out, nil
}

func decodeRequest(wire *pb.ProjectWorkRequest) (application.ProjectWorkRequest, error) {
	var request application.ProjectWorkRequest
	if wire == nil || len(wire.ProtoReflect().GetUnknown()) != 0 || len(wire.Commands) > 128 {
		return request, writefence.ErrInvalid
	}
	for _, field := range []struct {
		raw []byte
		id  *uuid.UUID
	}{{wire.CloseId, &request.CloseID}, {wire.ProjectId, &request.ProjectID}, {wire.OrganizationId, &request.OrganizationID}, {wire.ScopeId, &request.ScopeID}} {
		id, err := uuid.FromBytes(field.raw)
		if err != nil || id == uuid.Nil {
			return request, writefence.ErrInvalid
		}
		*field.id = id
	}
	request.ProjectEpoch, request.ManifestSHA256 = wire.ProjectEpoch, wire.ManifestSha256
	request.Commands = make([]writefence.Command, 0, len(wire.Commands))
	for _, command := range wire.Commands {
		if command == nil || len(command.ProtoReflect().GetUnknown()) != 0 {
			return application.ProjectWorkRequest{}, writefence.ErrInvalid
		}
		for _, authority := range command.Authorities {
			if authority == nil || len(authority.ProtoReflect().GetUnknown()) != 0 {
				return application.ProjectWorkRequest{}, writefence.ErrInvalid
			}
		}
		value, err := grpctransport.DecodeCommand(command)
		if err != nil {
			return application.ProjectWorkRequest{}, err
		}
		request.Commands = append(request.Commands, value)
	}
	return request, request.Validate()
}

func sameRequest(a, b application.ProjectWorkRequest) bool {
	if a.CloseID != b.CloseID || a.ProjectID != b.ProjectID || a.OrganizationID != b.OrganizationID || a.ScopeID != b.ScopeID || a.ProjectEpoch != b.ProjectEpoch || a.ManifestSHA256 != b.ManifestSHA256 || len(a.Commands) != len(b.Commands) {
		return false
	}
	for i, command := range a.Commands {
		if !sameCommand(command, b.Commands[i]) {
			return false
		}
	}
	return true
}

func sameCommand(a, b writefence.Command) bool {
	x, err := a.AuthorityDigest()
	if err != nil {
		return false
	}
	y, err := b.AuthorityDigest()
	return err == nil && x == y && a.ID == b.ID && a.SHA256 == b.SHA256 && a.Action == b.Action
}

func workStatus(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	case errors.Is(err, writefence.ErrInvalid):
		return status.Error(codes.InvalidArgument, "complete original project closure identity required")
	case errors.Is(err, writefence.ErrConflict), errors.Is(err, writefence.ErrFenced):
		return status.Error(codes.FailedPrecondition, "project responsibility identity or authority changed")
	default:
		return status.Error(codes.Unavailable, "project responsibility evidence unavailable")
	}
}
