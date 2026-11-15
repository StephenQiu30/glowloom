// Package grpcregistration exposes Creation's private personal-scope participant.
package grpcregistration

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
)

// PersonalRegistrationService returns ready only after the exact Creation source
// controls are installed. The consumer accepts the actual source-ready use case.
type PersonalRegistrationService interface {
	Prepare(context.Context, workspacesource.PersonalRegistration) (workspacesource.PersonalRegistration, error)
	Resolve(context.Context, workspacesource.PersonalRegistration) (workspacesource.PersonalRegistration, error)
}

// NewServer requires the dedicated Identity mTLS name, token and verified TLS listener.
func NewServer(service PersonalRegistrationService, token, identityDNS string, transport credentials.TransportCredentials) (*grpc.Server, error) {
	if service == nil || transport == nil {
		return nil, workspacesource.ErrPersonalRegistration
	}
	interceptor, err := serviceauth.MTLSUnary(token, identityDNS)
	if err != nil {
		return nil, err
	}
	s := grpc.NewServer(grpc.Creds(transport.Clone()), grpc.MaxRecvMsgSize(4096), grpc.MaxSendMsgSize(4096), grpc.MaxHeaderListSize(8192), grpc.UnaryInterceptor(interceptor))
	pb.RegisterPersonalRegistrationServer(s, &server{service: service})
	return s, nil
}

type server struct {
	pb.UnimplementedPersonalRegistrationServer
	service PersonalRegistrationService
}

func decode(p *pb.PersonalRegistrationRequest) (workspacesource.PersonalRegistration, error) {
	var r workspacesource.PersonalRegistration
	if p == nil || len(p.ProtoReflect().GetUnknown()) != 0 {
		return r, workspacesource.ErrPersonalRegistration
	}
	var err error
	r.OperationID, err = uuid.FromBytes(p.OperationId)
	if err != nil {
		return r, workspacesource.ErrPersonalRegistration
	}
	r.AccountID, err = uuid.FromBytes(p.AccountId)
	if err != nil {
		return r, workspacesource.ErrPersonalRegistration
	}
	r.OrganizationID, err = uuid.FromBytes(p.OrganizationId)
	if err != nil {
		return r, workspacesource.ErrPersonalRegistration
	}
	r.ScopeID, err = uuid.FromBytes(p.ScopeId)
	if err != nil {
		return r, workspacesource.ErrPersonalRegistration
	}
	r.InputSHA256 = p.InputSha256
	return r, r.Validate()
}
func response(p workspacesource.PersonalRegistration) *pb.PersonalRegistrationReceipt {
	return &pb.PersonalRegistrationReceipt{Command: &pb.PersonalRegistrationRequest{OperationId: p.OperationID[:], AccountId: p.AccountID[:], OrganizationId: p.OrganizationID[:], ScopeId: p.ScopeID[:], InputSha256: p.InputSHA256}}
}
func rpcError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.FromContextError(err).Err()
	}
	if errors.Is(err, workspacesource.ErrPersonalRegistration) {
		return status.Error(codes.FailedPrecondition, "personal registration preparation conflict")
	}
	return status.Error(codes.Unavailable, "personal registration preparation unavailable")
}
func (s *server) Prepare(ctx context.Context, request *pb.PersonalRegistrationRequest) (*pb.PersonalRegistrationReceipt, error) {
	p, err := decode(request)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid personal registration command")
	}
	r, err := s.service.Prepare(ctx, p)
	if err != nil {
		return nil, rpcError(err)
	}
	return response(r), nil
}
func (s *server) Resolve(ctx context.Context, request *pb.PersonalRegistrationRequest) (*pb.PersonalRegistrationReceipt, error) {
	p, err := decode(request)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid personal registration command")
	}
	r, err := s.service.Resolve(ctx, p)
	if err != nil {
		return nil, rpcError(err)
	}
	return response(r), nil
}
