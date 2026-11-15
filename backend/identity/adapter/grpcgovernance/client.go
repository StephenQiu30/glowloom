// Package grpcgovernance consumes Creation's exact retained-resource receipts.
package grpcgovernance

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/StephenQiu30/glowloom-server/backend/identity/application"
	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationgovernance"
	identitypb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// Client binds the only permitted Creation endpoint and Identity service audience.
// A timeout remains unknown and never authorizes a local SQL/count fallback.
type Client struct {
	address, token string
	transport      credentials.TransportCredentials
}

// New requires an explicit service identity and verified transport policy.
func New(address, token string, transport credentials.TransportCredentials) (*Client, error) {
	host, port, err := net.SplitHostPort(address)
	number, portErr := strconv.Atoi(port)
	if err != nil || host == "" || strings.ContainsAny(host, "/@?#% \t\n") || portErr != nil || number < 1 || number > 65535 || !serviceauth.ValidToken(token) || transport == nil {
		return nil, writefence.ErrInvalid
	}
	return &Client{address: address, token: token, transport: transport.Clone()}, nil
}
func (c *Client) connect(ctx context.Context) (context.Context, context.CancelFunc, *grpc.ClientConn, error) {
	if c == nil {
		return nil, nil, nil, writefence.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(bounded, carrier)
	values := []string{"authorization", "Bearer " + c.token}
	for _, key := range []string{"traceparent", "tracestate"} {
		if value := carrier[key]; value != "" {
			values = append(values, key, value)
		}
	}
	bounded = metadata.NewOutgoingContext(bounded, metadata.Pairs(values...))
	connection, err := grpc.NewClient(c.address, grpc.WithTransportCredentials(c.transport.Clone()), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(32<<10), grpc.MaxCallSendMsgSize(32<<10), grpc.MaxRetryRPCBufferSize(0)))
	if err != nil {
		cancel()
		return nil, nil, nil, writefence.ErrUnavailable
	}
	return bounded, cancel, connection, nil
}

// PrepareAccountGovernance invokes only the original fixed three-action command.
func (c *Client) PrepareAccountGovernance(ctx context.Context, intent application.AccountQualificationIntent) (application.GovernanceTeamReceipt, error) {
	if intent.Validate() != nil {
		return application.GovernanceTeamReceipt{}, writefence.ErrInvalid
	}
	ctx, cancel, connection, err := c.connect(ctx)
	if err != nil {
		return application.GovernanceTeamReceipt{}, err
	}
	defer cancel()
	defer func() { _ = connection.Close() }()
	response, err := pb.NewAccountGovernanceClient(connection).Prepare(ctx, &pb.PrepareAccountGovernanceRequest{Intent: encodeIntent(intent)})
	if err != nil {
		return application.GovernanceTeamReceipt{}, localError(err)
	}
	return decodeReceipt(response, intent)
}

// ResolveAccountGovernance asks only about the same stored original operation.
// It cannot replace a pending request or issue a new candidate/command.
func (c *Client) ResolveAccountGovernance(ctx context.Context, intent application.AccountQualificationIntent) (application.GovernanceTeamReceipt, error) {
	if intent.Validate() != nil {
		return application.GovernanceTeamReceipt{}, writefence.ErrInvalid
	}
	ctx, cancel, connection, err := c.connect(ctx)
	if err != nil {
		return application.GovernanceTeamReceipt{}, err
	}
	defer cancel()
	defer func() { _ = connection.Close() }()
	response, err := pb.NewAccountGovernanceClient(connection).Resolve(ctx, &pb.ResolveAccountGovernanceRequest{RequestId: intent.RequestID[:]})
	if err != nil {
		return application.GovernanceTeamReceipt{}, localError(err)
	}
	return decodeReceipt(response, intent)
}
func encodeIntent(p application.AccountQualificationIntent) *identitypb.AccountQualificationIntent {
	action := identitypb.AccountQualificationAction_ACCOUNT_QUALIFICATION_ACTION_DISABLE
	if p.Action == "enable" {
		action = identitypb.AccountQualificationAction_ACCOUNT_QUALIFICATION_ACTION_ENABLE
	}
	if p.Action == "delete" {
		action = identitypb.AccountQualificationAction_ACCOUNT_QUALIFICATION_ACTION_DELETE
	}
	return &identitypb.AccountQualificationIntent{RequestId: p.RequestID[:], AccountId: p.AccountID[:], Action: action, InputSha256: p.InputSHA256, EligibleAfter: p.EligibleAfter, CloseControl: &coordination.Control{Authority: &coordination.Authority{Issuer: coordination.Issuer_ISSUER_IDENTITY, Kind: coordination.AuthorityKind_AUTHORITY_KIND_ACCOUNT, Id: p.AccountID[:], Epoch: p.CloseControl.Epoch}, CoordinationId: p.CloseControl.CoordinationID[:], Blocked: true}}
}
func decodeReceipt(response *pb.AccountGovernanceResponse, intent application.AccountQualificationIntent) (application.GovernanceTeamReceipt, error) {
	if !closed(response) || response.Receipt == nil {
		return application.GovernanceTeamReceipt{}, writefence.ErrUnavailable
	}
	value := response.Receipt
	request, err := uuid.FromBytes(value.RequestId)
	if err != nil {
		return application.GovernanceTeamReceipt{}, writefence.ErrUnavailable
	}
	account, err := uuid.FromBytes(value.AccountId)
	if err != nil {
		return application.GovernanceTeamReceipt{}, writefence.ErrUnavailable
	}
	if value.IdentityClose == nil || value.IdentityClose.Authority == nil {
		return application.GovernanceTeamReceipt{}, writefence.ErrUnavailable
	}
	a := value.IdentityClose.Authority
	bound, err := uuid.FromBytes(a.Id)
	if err != nil || bound != account || a.Issuer != coordination.Issuer_ISSUER_IDENTITY || a.Kind != coordination.AuthorityKind_AUTHORITY_KIND_ACCOUNT {
		return application.GovernanceTeamReceipt{}, writefence.ErrUnavailable
	}
	coord, err := uuid.FromBytes(value.IdentityClose.CoordinationId)
	if err != nil {
		return application.GovernanceTeamReceipt{}, writefence.ErrUnavailable
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
		return application.GovernanceTeamReceipt{}, writefence.ErrUnavailable
	}
	rejected := false
	switch value.Verdict {
	case pb.AccountGovernanceVerdict_ACCOUNT_GOVERNANCE_VERDICT_READY:
	case pb.AccountGovernanceVerdict_ACCOUNT_GOVERNANCE_VERDICT_REJECTED_DUTIES:
		rejected = true
	default:
		return application.GovernanceTeamReceipt{}, writefence.ErrUnavailable
	}
	out := application.GovernanceTeamReceipt{RequestID: request, AccountID: account, Action: action, InputSHA256: value.InputSha256, IdentityClose: writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: account, Epoch: a.Epoch}, CoordinationID: coord, Blocked: value.IdentityClose.Blocked}, RejectedDuties: rejected, Teams: []uuid.UUID{}, BlockingTeams: []uuid.UUID{}}
	for _, raw := range value.TeamIds {
		id, err := uuid.FromBytes(raw)
		if err != nil {
			return out, writefence.ErrUnavailable
		}
		out.Teams = append(out.Teams, id)
	}
	for _, raw := range value.BlockingTeamIds {
		id, err := uuid.FromBytes(raw)
		if err != nil {
			return out, writefence.ErrUnavailable
		}
		out.BlockingTeams = append(out.BlockingTeams, id)
	}
	if !out.Matches(intent) {
		return application.GovernanceTeamReceipt{}, writefence.ErrUnavailable
	}
	return out, nil
}
func localError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	s := status.Convert(err)
	switch s.Code() {
	case codes.Canceled:
		return context.Canceled
	case codes.DeadlineExceeded:
		return context.DeadlineExceeded
	case codes.NotFound:
		return writefence.ErrNotFound
	case codes.InvalidArgument:
		return writefence.ErrInvalid
	case codes.FailedPrecondition:
		return writefence.ErrConflict
	default:
		return writefence.ErrUnavailable
	}
}
func closed(message proto.Message) bool {
	if message == nil {
		return false
	}
	return closedMessage(message.ProtoReflect())
}
func closedMessage(message protoreflect.Message) bool {
	if !message.IsValid() || len(message.GetUnknown()) > 0 {
		return false
	}
	valid := true
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		check := func(value protoreflect.Value) bool {
			if field.Kind() == protoreflect.MessageKind {
				return closedMessage(value.Message())
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

var _ application.GovernanceTeamParticipant = (*Client)(nil)
