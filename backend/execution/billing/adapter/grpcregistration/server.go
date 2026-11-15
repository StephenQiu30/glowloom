// Package grpcregistration exposes Execution's private registration wallet participant.
package grpcregistration

import (
	"bytes"
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/StephenQiu30/glowloom-server/backend/execution/billing/application"
	creationpb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creation"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/execution"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
)

// NewServer requires the dedicated Identity certificate identity and token.
func NewServer(service *application.RegistrationWallets, token, identityDNS string, transport credentials.TransportCredentials) (*grpc.Server, error) {
	if service == nil || transport == nil {
		return nil, application.ErrRegistrationWallet
	}
	interceptor, err := serviceauth.MTLSUnary(token, identityDNS)
	if err != nil {
		return nil, err
	}
	s := grpc.NewServer(grpc.Creds(transport.Clone()), grpc.MaxRecvMsgSize(4096), grpc.MaxSendMsgSize(4096), grpc.MaxHeaderListSize(8192), grpc.UnaryInterceptor(interceptor))
	pb.RegisterRegistrationWalletServer(s, &server{service: service})
	return s, nil
}

type server struct {
	pb.UnimplementedRegistrationWalletServer
	service *application.RegistrationWallets
}

func decode(p *pb.RegistrationWalletRequest) (application.RegistrationWallet, error) {
	var r application.RegistrationWallet
	if p == nil || len(p.ProtoReflect().GetUnknown()) != 0 {
		return r, application.ErrRegistrationWallet
	}
	ready := p.CreationReady
	if ready == nil || ready.Command == nil || len(ready.ProtoReflect().GetUnknown()) != 0 || len(ready.Command.ProtoReflect().GetUnknown()) != 0 {
		return r, application.ErrRegistrationWallet
	}
	q := ready.Command
	if !bytes.Equal(q.OperationId, p.OperationId) || !bytes.Equal(q.AccountId, p.AccountId) || !bytes.Equal(q.OrganizationId, p.OrganizationId) || !bytes.Equal(q.ScopeId, p.ScopeId) || q.InputSha256 != p.InputSha256 {
		return r, application.ErrRegistrationWallet
	}
	r.CreationReady = true
	var err error
	r.OperationID, err = uuid.FromBytes(p.OperationId)
	if err != nil {
		return r, application.ErrRegistrationWallet
	}
	r.AccountID, err = uuid.FromBytes(p.AccountId)
	if err != nil {
		return r, application.ErrRegistrationWallet
	}
	r.OrganizationID, err = uuid.FromBytes(p.OrganizationId)
	if err != nil {
		return r, application.ErrRegistrationWallet
	}
	r.ScopeID, err = uuid.FromBytes(p.ScopeId)
	if err != nil {
		return r, application.ErrRegistrationWallet
	}
	r.InputSHA256 = p.InputSha256
	return r, r.Validate()
}
func response(p application.RegistrationWallet) *pb.RegistrationWalletReceipt {
	return &pb.RegistrationWalletReceipt{Command: &pb.RegistrationWalletRequest{OperationId: p.OperationID[:], AccountId: p.AccountID[:], OrganizationId: p.OrganizationID[:], ScopeId: p.ScopeID[:], InputSha256: p.InputSHA256, CreationReady: &creationpb.PersonalRegistrationReceipt{Command: &creationpb.PersonalRegistrationRequest{OperationId: p.OperationID[:], AccountId: p.AccountID[:], OrganizationId: p.OrganizationID[:], ScopeId: p.ScopeID[:], InputSha256: p.InputSHA256}}}}
}
func rpcError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.FromContextError(err).Err()
	}
	if errors.Is(err, application.ErrRegistrationWallet) {
		return status.Error(codes.FailedPrecondition, "registration wallet preparation conflict")
	}
	return status.Error(codes.Unavailable, "registration wallet preparation unavailable")
}
func (s *server) Prepare(ctx context.Context, request *pb.RegistrationWalletRequest) (*pb.RegistrationWalletReceipt, error) {
	p, err := decode(request)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid registration wallet command")
	}
	r, err := s.service.Prepare(ctx, p)
	if err != nil {
		return nil, rpcError(err)
	}
	return response(r), nil
}
func (s *server) Resolve(ctx context.Context, request *pb.RegistrationWalletRequest) (*pb.RegistrationWalletReceipt, error) {
	p, err := decode(request)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid registration wallet command")
	}
	r, err := s.service.Resolve(ctx, p)
	if err != nil {
		return nil, rpcError(err)
	}
	return response(r), nil
}
