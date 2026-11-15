// Package grpcpreparation exposes original persisted project preparation facts to
// the authenticated Execution participant. It exposes no create or write permit.
package grpcpreparation

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

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationpreparation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
)

// Reader resolves only exact already-persisted Creation resources and hashes.
type Reader interface {
	ReadProjectPreparation(context.Context, workspacesource.ProjectBudgetPreparation) (workspacesource.ProjectPreparation, error)
}

// NewServer requires the dedicated Execution token, verified mTLS name and TLS
// policy. Being a private service caller never substitutes for source facts.
func NewServer(reader Reader, token, executionDNS string, transport credentials.TransportCredentials) (*grpc.Server, error) {
	if reader == nil || transport == nil {
		return nil, workspacesource.ErrProjectPreparationConflict
	}
	interceptor, err := serviceauth.MTLSUnary(token, executionDNS)
	if err != nil {
		return nil, err
	}
	server := grpc.NewServer(grpc.Creds(transport.Clone()), grpc.MaxRecvMsgSize(4096), grpc.MaxSendMsgSize(4096), grpc.MaxHeaderListSize(8192), grpc.UnaryInterceptor(interceptor))
	pb.RegisterProjectPreparationsServer(server, &service{reader: reader})
	return server, nil
}

type service struct {
	pb.UnimplementedProjectPreparationsServer
	reader Reader
}

func decode(request *pb.ProjectPreparationKey) (workspacesource.ProjectBudgetPreparation, error) {
	var key workspacesource.ProjectBudgetPreparation
	if request == nil || len(request.ProtoReflect().GetUnknown()) != 0 {
		return key, workspacesource.ErrProjectPreparationConflict
	}
	values := []struct {
		raw []byte
		to  *uuid.UUID
	}{{request.PreparationId, &key.PreparationID}, {request.SourceCommandId, &key.SourceCommandID}, {request.ExecutionCommandId, &key.ExecutionCommandID}, {request.ProjectId, &key.ProjectID}, {request.OrganizationId, &key.OrganizationID}, {request.ScopeId, &key.ScopeID}}
	for _, value := range values {
		id, err := uuid.FromBytes(value.raw)
		if err != nil || id == uuid.Nil {
			return workspacesource.ProjectBudgetPreparation{}, workspacesource.ErrProjectPreparationConflict
		}
		*value.to = id
	}
	key.SourceSHA256 = request.SourceSha256
	key.InputSHA256 = request.InputSha256
	return key, key.Validate()
}

func (s *service) ResolveProjectPreparation(ctx context.Context, request *pb.ProjectPreparationKey) (*pb.ProjectPreparedProof, error) {
	key, err := decode(request)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid project preparation key")
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	prepared, err := s.reader.ReadProjectPreparation(bounded, key)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return nil, status.FromContextError(err).Err()
		case errors.Is(err, workspacesource.ErrProjectPreparationUnknown):
			return nil, status.Error(codes.NotFound, "project preparation unknown")
		case errors.Is(err, workspacesource.ErrProjectPreparationConflict):
			return nil, status.Error(codes.AlreadyExists, "project preparation key conflict")
		default:
			return nil, status.Error(codes.Unavailable, "project preparation unavailable")
		}
	}
	if prepared.Spec.Validate() != nil || prepared.Spec.BudgetRequest() != key {
		return nil, status.Error(codes.AlreadyExists, "project preparation key conflict")
	}
	return &pb.ProjectPreparedProof{Request: &pb.ProjectPreparationKey{PreparationId: key.PreparationID[:], SourceCommandId: key.SourceCommandID[:], SourceSha256: key.SourceSHA256, InputSha256: key.InputSHA256, ExecutionCommandId: key.ExecutionCommandID[:], ProjectId: key.ProjectID[:], OrganizationId: key.OrganizationID[:], ScopeId: key.ScopeID[:]}, Phase: "prepared"}, nil
}
