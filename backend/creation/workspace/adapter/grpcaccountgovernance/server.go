// Package grpcaccountgovernance exposes Creation's retained account team effects.
package grpcaccountgovernance

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	workspace "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationgovernance"
	identitypb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// Service is the real source-qualified Creation use case, not a count port.
type Service interface {
	Prepare(context.Context, workspace.AccountGovernanceIntent) (workspace.AccountGovernanceReceipt, error)
	Resolve(context.Context, uuid.UUID) (workspace.AccountGovernanceReceipt, error)
}

// NewServer binds this responsibility to the dedicated verified Identity peer.
// It owns no SQL/environment setup and cannot accept browser credentials.
func NewServer(service Service, token, identityDNS string, transport credentials.TransportCredentials) (*grpc.Server, error) {
	if service == nil || transport == nil {
		return nil, writefence.ErrInvalid
	}
	interceptor, err := serviceauth.MTLSUnary(token, identityDNS)
	if err != nil {
		return nil, err
	}
	s := grpc.NewServer(grpc.Creds(transport.Clone()), grpc.MaxRecvMsgSize(32<<10), grpc.MaxSendMsgSize(32<<10), grpc.MaxHeaderListSize(8192), grpc.UnaryInterceptor(interceptor))
	pb.RegisterAccountGovernanceServer(s, &server{service: service})
	return s, nil
}

type server struct {
	pb.UnimplementedAccountGovernanceServer
	service Service
}

func (s *server) Prepare(ctx context.Context, r *pb.PrepareAccountGovernanceRequest) (*pb.AccountGovernanceResponse, error) {
	if !closed(r) {
		return nil, rpcError(writefence.ErrInvalid)
	}
	intent, err := decodeIntent(r.Intent)
	if err != nil {
		return nil, rpcError(err)
	}
	receipt, err := s.service.Prepare(ctx, intent)
	if err != nil {
		return nil, rpcError(err)
	}
	if receipt.RequestID != intent.RequestID || receipt.AccountID != intent.AccountID || receipt.Action != intent.Action || receipt.InputSHA256 != intent.InputSHA256 || receipt.IdentityClose != intent.CloseControl {
		return nil, rpcError(writefence.ErrUnavailable)
	}
	return &pb.AccountGovernanceResponse{Receipt: encodeReceipt(receipt)}, nil
}
func (s *server) Resolve(ctx context.Context, r *pb.ResolveAccountGovernanceRequest) (*pb.AccountGovernanceResponse, error) {
	if !closed(r) {
		return nil, rpcError(writefence.ErrInvalid)
	}
	id, err := uuid.FromBytes(r.RequestId)
	if err != nil || id == uuid.Nil {
		return nil, rpcError(writefence.ErrInvalid)
	}
	receipt, err := s.service.Resolve(ctx, id)
	if err != nil {
		return nil, rpcError(err)
	}
	if receipt.RequestID != id {
		return nil, rpcError(writefence.ErrUnavailable)
	}
	return &pb.AccountGovernanceResponse{Receipt: encodeReceipt(receipt)}, nil
}
func decodeIntent(value *identitypb.AccountQualificationIntent) (workspace.AccountGovernanceIntent, error) {
	if value == nil || !closed(value) || value.CloseControl == nil || value.CloseControl.Authority == nil {
		return workspace.AccountGovernanceIntent{}, writefence.ErrInvalid
	}
	request, err := uuid.FromBytes(value.RequestId)
	if err != nil {
		return workspace.AccountGovernanceIntent{}, writefence.ErrInvalid
	}
	account, err := uuid.FromBytes(value.AccountId)
	if err != nil {
		return workspace.AccountGovernanceIntent{}, writefence.ErrInvalid
	}
	a := value.CloseControl.Authority
	bound, err := uuid.FromBytes(a.Id)
	if err != nil || bound != account || a.Issuer != coordination.Issuer_ISSUER_IDENTITY || a.Kind != coordination.AuthorityKind_AUTHORITY_KIND_ACCOUNT {
		return workspace.AccountGovernanceIntent{}, writefence.ErrInvalid
	}
	coord, err := uuid.FromBytes(value.CloseControl.CoordinationId)
	if err != nil {
		return workspace.AccountGovernanceIntent{}, writefence.ErrInvalid
	}
	action := ""
	switch value.Action {
	case identitypb.AccountQualificationAction_ACCOUNT_QUALIFICATION_ACTION_DISABLE:
		action = "disable"
	case identitypb.AccountQualificationAction_ACCOUNT_QUALIFICATION_ACTION_ENABLE:
		action = "enable"
	case identitypb.AccountQualificationAction_ACCOUNT_QUALIFICATION_ACTION_DELETE:
		action = "delete"
	default:
		return workspace.AccountGovernanceIntent{}, writefence.ErrInvalid
	}
	out := workspace.AccountGovernanceIntent{RequestID: request, AccountID: account, Action: action, InputSHA256: value.InputSha256, CloseControl: writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: account, Epoch: a.Epoch}, CoordinationID: coord, Blocked: value.CloseControl.Blocked}, EligibleAfter: value.EligibleAfter}
	return out, out.Validate()
}
func encodeReceipt(r workspace.AccountGovernanceReceipt) *pb.AccountGovernanceReceipt {
	action := identitypb.AccountQualificationAction_ACCOUNT_QUALIFICATION_ACTION_DISABLE
	if r.Action == "enable" {
		action = identitypb.AccountQualificationAction_ACCOUNT_QUALIFICATION_ACTION_ENABLE
	}
	if r.Action == "delete" {
		action = identitypb.AccountQualificationAction_ACCOUNT_QUALIFICATION_ACTION_DELETE
	}
	verdict := pb.AccountGovernanceVerdict_ACCOUNT_GOVERNANCE_VERDICT_READY
	if r.RejectedDuties {
		verdict = pb.AccountGovernanceVerdict_ACCOUNT_GOVERNANCE_VERDICT_REJECTED_DUTIES
	}
	out := &pb.AccountGovernanceReceipt{RequestId: r.RequestID[:], AccountId: r.AccountID[:], Action: action, InputSha256: r.InputSHA256, IdentityClose: &coordination.Control{Authority: &coordination.Authority{Issuer: coordination.Issuer_ISSUER_IDENTITY, Kind: coordination.AuthorityKind_AUTHORITY_KIND_ACCOUNT, Id: r.IdentityClose.ID[:], Epoch: r.IdentityClose.Epoch}, Blocked: r.IdentityClose.Blocked, CoordinationId: r.IdentityClose.CoordinationID[:]}, Verdict: verdict}
	for _, id := range r.Teams {
		out.TeamIds = append(out.TeamIds, append([]byte(nil), id[:]...))
	}
	for _, id := range r.BlockingTeams {
		out.BlockingTeamIds = append(out.BlockingTeamIds, append([]byte(nil), id[:]...))
	}
	return out
}
func rpcError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.FromContextError(err).Err()
	}
	if errors.Is(err, writefence.ErrInvalid) {
		return status.Error(codes.InvalidArgument, "invalid account governance responsibility")
	}
	if errors.Is(err, writefence.ErrNotFound) {
		return status.Error(codes.NotFound, "original account governance responsibility not found")
	}
	if errors.Is(err, writefence.ErrConflict) {
		return status.Error(codes.FailedPrecondition, "original account governance responsibility conflicts")
	}
	return status.Error(codes.Unavailable, "account governance responsibility unavailable")
}
func closed(message proto.Message) bool {
	if message == nil {
		return false
	}
	return closedReflect(message.ProtoReflect())
}
func closedReflect(message protoreflect.Message) bool {
	if !message.IsValid() || len(message.GetUnknown()) > 0 {
		return false
	}
	valid := true
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		check := func(value protoreflect.Value) bool {
			if field.Kind() == protoreflect.MessageKind {
				return closedReflect(value.Message())
			}
			if field.Kind() == protoreflect.EnumKind {
				return field.Enum().Values().ByNumber(value.Enum()) != nil
			}
			return true
		}
		switch {
		case field.IsList():
			for i := 0; i < value.List().Len(); i++ {
				if !check(value.List().Get(i)) {
					valid = false
					break
				}
			}
		case field.IsMap():
			valid = false
		default:
			valid = check(value)
		}
		return valid
	})
	return valid
}
