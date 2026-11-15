package identityregistration_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcgovernance"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	coordinationpb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationgovernance"
	identitypb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// This peer deliberately sends malformed receipts to exercise the consumer's
// closed boundary. Business effects are tested through real owners separately.
type governanceWirePeer struct {
	pb.UnimplementedAccountGovernanceServer
	response   *pb.AccountGovernanceResponse
	requestID  uuid.UUID
	calls      atomic.Int64
	badContext atomic.Bool
}

func (p *governanceWirePeer) context(ctx context.Context) {
	p.calls.Add(1)
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 10*time.Second || len(metadata.ValueFromIncomingContext(ctx, "cookie")) != 0 || len(metadata.ValueFromIncomingContext(ctx, "x-browser-proof")) != 0 {
		p.badContext.Store(true)
	}
}
func (p *governanceWirePeer) Prepare(ctx context.Context, r *pb.PrepareAccountGovernanceRequest) (*pb.AccountGovernanceResponse, error) {
	p.context(ctx)
	if r.Intent == nil || string(r.Intent.RequestId) != string(p.requestID[:]) {
		return nil, status.Error(codes.InvalidArgument, "request mismatch")
	}
	return p.response, nil
}
func (p *governanceWirePeer) Resolve(ctx context.Context, r *pb.ResolveAccountGovernanceRequest) (*pb.AccountGovernanceResponse, error) {
	p.context(ctx)
	if string(r.RequestId) != string(p.requestID[:]) {
		return nil, status.Error(codes.InvalidArgument, "request mismatch")
	}
	return p.response, nil
}

func TestGovernanceClientRejectsUnboundAndUnknownReceipts(t *testing.T) {
	certs := newTLS(t)
	request, account, coord, team := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	intent := identity.AccountQualificationIntent{RequestID: request, AccountID: account, Action: "delete", InputSHA256: strings.Repeat("b", 64), CloseControl: writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: account, Epoch: 4}, CoordinationID: coord, Blocked: true}}
	base := &pb.AccountGovernanceResponse{Receipt: &pb.AccountGovernanceReceipt{RequestId: request[:], AccountId: account[:], Action: identitypb.AccountQualificationAction_ACCOUNT_QUALIFICATION_ACTION_DELETE, InputSha256: intent.InputSHA256, IdentityClose: &coordinationpb.Control{Authority: &coordinationpb.Authority{Issuer: coordinationpb.Issuer_ISSUER_IDENTITY, Kind: coordinationpb.AuthorityKind_AUTHORITY_KIND_ACCOUNT, Id: account[:], Epoch: 4}, CoordinationId: coord[:], Blocked: true}, Verdict: pb.AccountGovernanceVerdict_ACCOUNT_GOVERNANCE_VERDICT_READY, TeamIds: [][]byte{team[:]}}}
	cases := []struct {
		name     string
		mutate   func(*pb.AccountGovernanceResponse)
		accepted bool
	}{
		{"ready", func(*pb.AccountGovernanceResponse) {}, true},
		{"duties", func(r *pb.AccountGovernanceResponse) {
			r.Receipt.Verdict = pb.AccountGovernanceVerdict_ACCOUNT_GOVERNANCE_VERDICT_REJECTED_DUTIES
			r.Receipt.BlockingTeamIds = [][]byte{team[:]}
		}, true},
		{"no_receipt", func(r *pb.AccountGovernanceResponse) { r.Receipt = nil }, false},
		{"different_request", func(r *pb.AccountGovernanceResponse) { id := uuid.New(); r.Receipt.RequestId = id[:] }, false},
		{"different_account", func(r *pb.AccountGovernanceResponse) { id := uuid.New(); r.Receipt.AccountId = id[:] }, false},
		{"different_action", func(r *pb.AccountGovernanceResponse) {
			r.Receipt.Action = identitypb.AccountQualificationAction_ACCOUNT_QUALIFICATION_ACTION_ENABLE
		}, false},
		{"different_hash", func(r *pb.AccountGovernanceResponse) { r.Receipt.InputSha256 = strings.Repeat("c", 64) }, false},
		{"different_epoch", func(r *pb.AccountGovernanceResponse) { r.Receipt.IdentityClose.Authority.Epoch++ }, false},
		{"different_coordination", func(r *pb.AccountGovernanceResponse) {
			id := uuid.New()
			r.Receipt.IdentityClose.CoordinationId = id[:]
		}, false},
		{"open_control", func(r *pb.AccountGovernanceResponse) { r.Receipt.IdentityClose.Blocked = false }, false},
		{"different_kind", func(r *pb.AccountGovernanceResponse) {
			r.Receipt.IdentityClose.Authority.Kind = coordinationpb.AuthorityKind_AUTHORITY_KIND_SESSION
		}, false},
		{"no_control", func(r *pb.AccountGovernanceResponse) { r.Receipt.IdentityClose = nil }, false},
		{"unknown_verdict", func(r *pb.AccountGovernanceResponse) { r.Receipt.Verdict = pb.AccountGovernanceVerdict(99) }, false},
		{"unspecified_verdict", func(r *pb.AccountGovernanceResponse) {
			r.Receipt.Verdict = pb.AccountGovernanceVerdict_ACCOUNT_GOVERNANCE_VERDICT_UNSPECIFIED
		}, false},
		{"unknown_action", func(r *pb.AccountGovernanceResponse) { r.Receipt.Action = identitypb.AccountQualificationAction(99) }, false},
		{"unknown_top_fields", func(r *pb.AccountGovernanceResponse) { r.ProtoReflect().SetUnknown([]byte{0x98, 0x06, 0x01}) }, false},
		{"unknown_nested_fields", func(r *pb.AccountGovernanceResponse) {
			r.Receipt.IdentityClose.Authority.ProtoReflect().SetUnknown([]byte{0x98, 0x06, 0x01})
		}, false},
		{"duplicate_team", func(r *pb.AccountGovernanceResponse) { r.Receipt.TeamIds = append(r.Receipt.TeamIds, team[:]) }, false},
		{"nil_team", func(r *pb.AccountGovernanceResponse) { id := uuid.Nil; r.Receipt.TeamIds = [][]byte{id[:]} }, false},
		{"malformed_team", func(r *pb.AccountGovernanceResponse) { r.Receipt.TeamIds = [][]byte{[]byte("short")} }, false},
		{"ready_with_blockers", func(r *pb.AccountGovernanceResponse) { r.Receipt.BlockingTeamIds = [][]byte{team[:]} }, false},
		{"duties_without_blockers", func(r *pb.AccountGovernanceResponse) {
			r.Receipt.Verdict = pb.AccountGovernanceVerdict_ACCOUNT_GOVERNANCE_VERDICT_REJECTED_DUTIES
		}, false},
		{"blocker_not_in_snapshot", func(r *pb.AccountGovernanceResponse) {
			id := uuid.New()
			r.Receipt.Verdict = pb.AccountGovernanceVerdict_ACCOUNT_GOVERNANCE_VERDICT_REJECTED_DUTIES
			r.Receipt.BlockingTeamIds = [][]byte{id[:]}
		}, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			response := proto.Clone(base).(*pb.AccountGovernanceResponse)
			test.mutate(response)
			peer := &governanceWirePeer{response: response, requestID: request}
			token := strings.Repeat("governance-client-audience-", 3)
			interceptor, err := serviceauth.MTLSUnary(token, "identity.governance.client")
			if err != nil {
				t.Fatal(err)
			}
			server := grpc.NewServer(grpc.Creds(certs.server(t, "creation.governance.server")), grpc.UnaryInterceptor(interceptor))
			pb.RegisterAccountGovernanceServer(server, peer)
			client, err := grpcgovernance.New(serve(t, server), token, certs.client(t, "creation.governance.server", "identity.governance.client"))
			if err != nil {
				t.Fatal(err)
			}
			ctx := metadata.NewOutgoingContext(t.Context(), metadata.Pairs("cookie", "synthetic browser cookie", "x-browser-proof", "must not cross owner boundary", "authorization", "Bearer unrelated"))
			for _, call := range []func(context.Context, identity.AccountQualificationIntent) (identity.GovernanceTeamReceipt, error){client.PrepareAccountGovernance, client.ResolveAccountGovernance} {
				receipt, err := call(ctx, intent)
				if test.accepted {
					if err != nil || !receipt.Matches(intent) {
						t.Fatalf("bound receipt rejected: %v", err)
					}
				} else if !errors.Is(err, writefence.ErrUnavailable) {
					t.Fatalf("unknown/unbound receipt became a business outcome: %v", err)
				}
			}
			if peer.calls.Load() != 2 || peer.badContext.Load() {
				t.Fatal("client retried a mutation, omitted deadline, or forwarded browser metadata")
			}
		})
	}
}
