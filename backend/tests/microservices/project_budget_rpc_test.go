package microservices_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	budgetrpc "github.com/StephenQiu30/glowloom-server/backend/execution/billing/adapter/grpcprojectbudget"
	billingapp "github.com/StephenQiu30/glowloom-server/backend/execution/billing/application"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/execution"
)

func budgetWire(p billingapp.ProjectBudgetPreparation) *pb.ProjectBudgetPreparation {
	return &pb.ProjectBudgetPreparation{ExecutionCommandId: p.ExecutionCommandID[:], PreparationId: p.PreparationID[:], SourceCommandId: p.SourceCommandID[:], SourceSha256: p.SourceSHA256, InputSha256: p.InputSHA256, ProjectId: p.ProjectID[:], OrganizationId: p.OrganizationID[:], ScopeId: p.ScopeID[:]}
}
func projectBudgetRPCConnection(t *testing.T, address string, transport credentials.TransportCredentials) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(transport), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(), grpc.WithDefaultCallOptions(grpc.MaxRetryRPCBufferSize(0)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	})
	return conn
}
func projectBudgetRPCContext(t *testing.T, token string) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	return metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token)), cancel
}
func TestProjectBudgetRPCRequiresExactCreationPeerTokenDeadlineAndClosedFields(t *testing.T) {
	repo := &projectBudgetRepository{}
	source := &projectBudgetProof{}
	service, err := billingapp.NewProjectBudgets(repo, source)
	if err != nil {
		t.Fatal(err)
	}
	serverTransport, creationPeer, foreignPeer := ownerPeerTLS(t, "creation")
	token := strings.Repeat("c", 40)
	server, err := budgetrpc.NewServer(service, token, serverTransport)
	if err != nil {
		t.Fatal(err)
	}
	address := serveFrozenRPC(t, server)
	good := pb.NewProjectBudgetsClient(projectBudgetRPCConnection(t, address, creationPeer))
	wrong := pb.NewProjectBudgetsClient(projectBudgetRPCConnection(t, address, foreignPeer))
	p := projectBudgetRequest()
	request := budgetWire(p)
	ctx, cancel := projectBudgetRPCContext(t, token)
	defer cancel()
	if _, err := wrong.PrepareProjectBudget(ctx, request); err == nil {
		t.Fatal("foreign mTLS peer prepared project budget")
	}
	wrongToken, stop := projectBudgetRPCContext(t, strings.Repeat("i", 40))
	defer stop()
	if _, err := good.PrepareProjectBudget(wrongToken, request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("foreign service token err=%v", err)
	}
	duplicate := metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token, "authorization", "Bearer "+token))
	if _, err := good.PrepareProjectBudget(duplicate, request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("duplicate identity accepted: %v", err)
	}
	unknown := proto.Clone(request).(*pb.ProjectBudgetPreparation)
	unknown.ProtoReflect().SetUnknown([]byte{0x98, 0x06, 0x01})
	if _, err := good.PrepareProjectBudget(ctx, unknown); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unknown fields accepted: %v", err)
	}
	malformed := proto.Clone(request).(*pb.ProjectBudgetPreparation)
	malformed.ProjectId = []byte{1}
	if _, err := good.PrepareProjectBudget(ctx, malformed); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("malformed project accepted: %v", err)
	}
	if source.calls != 0 || repo.commits != 0 {
		t.Fatalf("invalid private command caused effect: source=%d commits=%d", source.calls, repo.commits)
	}
	receipt, err := good.PrepareProjectBudget(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(receipt.Original, request) || receipt.LimitMicros != 9_007_199_254_740_991 || receipt.SpentMicros != 0 || receipt.ReservedMicros != 0 || !receipt.WalletPrepared || receipt.BudgetRevision != 1 {
		t.Fatalf("changed original receipt: %v", receipt)
	}
	replay, err := good.ResolveProjectBudget(ctx, request)
	if err != nil || !proto.Equal(receipt, replay) || source.calls != 1 || repo.commits != 1 {
		t.Fatalf("private original resolve failed: err=%v calls=%d commits=%d", err, source.calls, repo.commits)
	}
}
