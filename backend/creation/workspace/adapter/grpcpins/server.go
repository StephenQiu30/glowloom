// Package grpcpins admits new Media reference acquisitions under current Creation policy.
package grpcpins

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationreference"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
)

// Admitter resolves the original target-Media Identity proof before locking and
// registering current Creation policy. The private service token grants no user policy.
type Admitter interface {
	Admit(context.Context, workspacesource.AdmissionRequest) (workspacesource.Permit, error)
}

type server struct {
	pb.UnimplementedReferencePinAdmissionsServer
	admitter Admitter
}

// NewServer exposes only acquisition admission, with a dedicated Media audience
// and verified peer certificate. The root owns listening, stopping and joining.
func NewServer(admitter Admitter, token, mediaDNS string, transport credentials.TransportCredentials) (*grpc.Server, error) {
	if admitter == nil || transport == nil || transport.Info().SecurityProtocol != "tls" {
		return nil, writefence.ErrInvalid
	}
	authenticate, err := serviceauth.MTLSUnary(token, mediaDNS)
	if err != nil {
		return nil, err
	}
	listener := grpc.NewServer(grpc.Creds(transport.Clone()), grpc.UnaryInterceptor(authenticate),
		grpc.MaxRecvMsgSize(8192), grpc.MaxSendMsgSize(8192), grpc.MaxHeaderListSize(16384))
	pb.RegisterReferencePinAdmissionsServer(listener, &server{admitter: admitter})
	return listener, nil
}

func (s *server) AdmitAcquisition(ctx context.Context, wire *pb.ReferencePinAdmissionRequest) (*pb.ReferencePinAdmission, error) {
	request, err := decode(wire)
	if err != nil {
		return nil, admissionStatus(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	permit, err := s.admitter.Admit(ctx, request)
	if err != nil {
		return nil, admissionStatus(err)
	}
	actual, actualErr := permit.Command.AuthorityDigest()
	expected, expectedErr := request.Command.AuthorityDigest()
	if actualErr != nil || expectedErr != nil || actual != expected || permit.Command.ID != request.Command.ID ||
		permit.Command.Action != request.Command.Action || permit.Command.SHA256 != request.Command.SHA256 ||
		permit.Target != writefence.WriterMedia || permit.ScopeID != request.ScopeID || permit.ProjectID != request.ProjectID || permit.PhysicalScopeID == uuid.Nil ||
		!matchesActor(request.Command, permit.AccountID, permit.SessionID) {
		return nil, admissionStatus(writefence.ErrConflict)
	}
	return &pb.ReferencePinAdmission{Request: wire, AccountId: permit.AccountID.String(), SessionId: permit.SessionID.String(), PhysicalScopeId: permit.PhysicalScopeID.String()}, nil
}

func decode(wire *pb.ReferencePinAdmissionRequest) (workspacesource.AdmissionRequest, error) {
	request := workspacesource.AdmissionRequest{Target: writefence.WriterMedia}
	if wire == nil || len(wire.ProtoReflect().GetUnknown()) != 0 || wire.Command == nil || len(wire.Command.ProtoReflect().GetUnknown()) != 0 {
		return request, writefence.ErrInvalid
	}
	for _, authority := range wire.Command.Authorities {
		if authority == nil || len(authority.ProtoReflect().GetUnknown()) != 0 {
			return request, writefence.ErrInvalid
		}
	}
	command, err := grpctransport.DecodeCommand(wire.Command)
	if err != nil || command.Action != "media.reference_pin.acquire" {
		return request, writefence.ErrInvalid
	}
	request.Command = command
	for _, field := range []struct {
		text string
		id   *uuid.UUID
	}{{wire.ScopeId, &request.ScopeID}, {wire.ProjectId, &request.ProjectID}} {
		id, err := uuid.Parse(field.text)
		if err != nil || id == uuid.Nil || id.String() != field.text {
			return request, writefence.ErrInvalid
		}
		*field.id = id
	}
	return request, nil
}

func matchesActor(command writefence.Command, account, session uuid.UUID) bool {
	if account == uuid.Nil || session == uuid.Nil || len(command.Authorities) != 4 {
		return false
	}
	var accountFound, sessionFound bool
	for _, a := range command.Authorities {
		accountFound = accountFound || a.Issuer == writefence.Identity && a.Kind == writefence.Account && a.ID == account
		sessionFound = sessionFound || a.Issuer == writefence.Identity && a.Kind == writefence.Session && a.ID == session
	}
	return accountFound && sessionFound
}

func admissionStatus(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	case errors.Is(err, writefence.ErrInvalid):
		return status.Error(codes.InvalidArgument, "exact original acquisition command required")
	case errors.Is(err, writefence.ErrConflict):
		return status.Error(codes.AlreadyExists, "original acquisition identity differs")
	case errors.Is(err, writefence.ErrFenced), errors.Is(err, writefence.ErrFrozen),
		errors.Is(err, workspacesource.ErrProjectNotFound), errors.Is(err, workspacesource.ErrScopeNotFound):
		return status.Error(codes.FailedPrecondition, "current acquisition source policy denied")
	default:
		return status.Error(codes.Unavailable, "original acquisition admission unavailable")
	}
}
