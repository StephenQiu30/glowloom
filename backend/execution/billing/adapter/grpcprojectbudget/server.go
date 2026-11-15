// Package grpcprojectbudget exposes Execution's private project bootstrap, not public spending.
package grpcprojectbudget

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/StephenQiu30/glowloom-server/backend/execution/billing/application"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/execution"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
)

// Budgets is the actual source-proof application coordinator consumed by this endpoint.
type Budgets interface {
	Prepare(context.Context, application.ProjectBudgetPreparation) (application.ProjectBudgetReceipt, error)
	Resolve(context.Context, application.ProjectBudgetPreparation) (application.ProjectBudgetReceipt, error)
}

type server struct {
	pb.UnimplementedProjectBudgetsServer
	budgets Budgets
	token   string
}

// NewServer requires dedicated Creation authentication and a verified mTLS listener.
// An absent Creation source proof is rejected by NewProjectBudgets, not bypassed here.
func NewServer(budgets Budgets, creationToken string, transport credentials.TransportCredentials) (*grpc.Server, error) {
	if budgets == nil || !serviceauth.ValidToken(creationToken) || transport == nil || transport.Info().SecurityProtocol != "tls" {
		return nil, application.ErrProjectBudgetPreparation
	}
	handler := &server{budgets: budgets, token: creationToken}
	listener := grpc.NewServer(grpc.Creds(transport.Clone()), grpc.MaxRecvMsgSize(32768), grpc.MaxSendMsgSize(32768), grpc.MaxHeaderListSize(16384), grpc.UnaryInterceptor(handler.authorize))
	pb.RegisterProjectBudgetsServer(listener, handler)
	return listener, nil
}
func (s *server) authorize(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	auth := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(auth) != 1 || subtle.ConstantTimeCompare([]byte(auth[0]), []byte("Bearer "+s.token)) != 1 {
		return nil, status.Error(codes.Unauthenticated, "Creation project preparation identity required")
	}
	remote, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "verified project preparation peer required")
	}
	tlsInfo, ok := remote.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tlsInfo.State.VerifiedChains) == 0 {
		return nil, status.Error(codes.Unauthenticated, "mutual TLS project preparation peer required")
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 10*time.Second {
		return nil, status.Error(codes.InvalidArgument, "bounded project preparation deadline required")
	}
	if info.FullMethod != pb.ProjectBudgets_PrepareProjectBudget_FullMethodName && info.FullMethod != pb.ProjectBudgets_ResolveProjectBudget_FullMethodName {
		return nil, status.Error(codes.Unimplemented, "unknown project preparation operation")
	}
	carrier := propagation.MapCarrier{}
	safe := metadata.MD{}
	for _, key := range []string{"traceparent", "tracestate"} {
		values := metadata.ValueFromIncomingContext(ctx, key)
		if len(values) == 1 {
			carrier[key] = values[0]
			safe.Set(key, values[0])
		}
	}
	ctx = propagation.TraceContext{}.Extract(ctx, carrier)
	return next(metadata.NewIncomingContext(ctx, safe), request)
}
func decodePreparation(value *pb.ProjectBudgetPreparation) (application.ProjectBudgetPreparation, error) {
	var out application.ProjectBudgetPreparation
	if value == nil || len(value.ProtoReflect().GetUnknown()) != 0 {
		return out, application.ErrProjectBudgetPreparation
	}
	ids := []*uuid.UUID{&out.ExecutionCommandID, &out.PreparationID, &out.SourceCommandID, &out.ProjectID, &out.OrganizationID, &out.ScopeID}
	raw := [][]byte{value.ExecutionCommandId, value.PreparationId, value.SourceCommandId, value.ProjectId, value.OrganizationId, value.ScopeId}
	for i, dst := range ids {
		id, err := uuid.FromBytes(raw[i])
		if err != nil {
			return out, application.ErrProjectBudgetPreparation
		}
		*dst = id
	}
	out.SourceSHA256 = value.SourceSha256
	out.InputSHA256 = value.InputSha256
	return out, out.Validate()
}
func encodePreparation(p application.ProjectBudgetPreparation) *pb.ProjectBudgetPreparation {
	return &pb.ProjectBudgetPreparation{ExecutionCommandId: p.ExecutionCommandID[:], PreparationId: p.PreparationID[:], SourceCommandId: p.SourceCommandID[:], SourceSha256: p.SourceSHA256, InputSha256: p.InputSHA256, ProjectId: p.ProjectID[:], OrganizationId: p.OrganizationID[:], ScopeId: p.ScopeID[:]}
}
func encodeReceipt(r application.ProjectBudgetReceipt) *pb.ProjectBudgetReceipt {
	return &pb.ProjectBudgetReceipt{Original: encodePreparation(r.Request), BudgetId: r.BudgetID[:], BudgetRevision: r.BudgetRevision, LimitMicros: r.LimitMicros, SpentMicros: r.SpentMicros, ReservedMicros: r.ReservedMicros, WalletPrepared: r.WalletPrepared}
}
func (s *server) PrepareProjectBudget(ctx context.Context, value *pb.ProjectBudgetPreparation) (*pb.ProjectBudgetReceipt, error) {
	p, err := decodePreparation(value)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid project preparation")
	}
	receipt, err := s.budgets.Prepare(ctx, p)
	if err != nil {
		return nil, closedError(err)
	}
	if receipt.Request != p || receipt.Validate() != nil {
		return nil, closedError(application.ErrProjectBudgetPreparation)
	}
	return encodeReceipt(receipt), nil
}
func (s *server) ResolveProjectBudget(ctx context.Context, value *pb.ProjectBudgetPreparation) (*pb.ProjectBudgetReceipt, error) {
	p, err := decodePreparation(value)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid project preparation")
	}
	receipt, err := s.budgets.Resolve(ctx, p)
	if err != nil {
		return nil, closedError(err)
	}
	if receipt.Request != p || receipt.Validate() != nil {
		return nil, closedError(application.ErrProjectBudgetPreparation)
	}
	return encodeReceipt(receipt), nil
}
func closedError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	case errors.Is(err, application.ErrProjectBudgetPreparation):
		return status.Error(codes.AlreadyExists, "project preparation identity conflict")
	case errors.Is(err, application.ErrProjectBudgetUnknown):
		return status.Error(codes.NotFound, "project preparation unresolved")
	case errors.Is(err, application.ErrProjectBudgetOwner):
		return status.Error(codes.PermissionDenied, "project preparation owner required")
	default:
		return status.Error(codes.Unavailable, "project preparation unavailable")
	}
}
