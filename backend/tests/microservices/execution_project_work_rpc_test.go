package microservices_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	workrpc "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/grpcprojectwork"
	executionpg "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"
	executionapp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/executionwork"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
)

type projectWorkReaderFunc func(context.Context, executionapp.ProjectWorkRequest) (executionapp.ProjectWorkSnapshot, error)

func (f projectWorkReaderFunc) ReadProjectWork(ctx context.Context, request executionapp.ProjectWorkRequest) (executionapp.ProjectWorkSnapshot, error) {
	return f(ctx, request)
}

func executionWorkWire(request executionapp.ProjectWorkRequest) *pb.ProjectWorkRequest {
	out := &pb.ProjectWorkRequest{CloseId: request.CloseID[:], ProjectId: request.ProjectID[:], OrganizationId: request.OrganizationID[:], ScopeId: request.ScopeID[:], ProjectEpoch: request.ProjectEpoch, ManifestSha256: request.ManifestSHA256}
	for _, command := range request.Commands {
		out.Commands = append(out.Commands, grpctransport.EncodeCommand(command))
	}
	return out
}
func projectWorkRPCClient(t *testing.T, address string, transport credentials.TransportCredentials) pb.ProjectWorkClient {
	t.Helper()
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(transport), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(), grpc.WithDefaultCallOptions(grpc.MaxRetryRPCBufferSize(0)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return pb.NewProjectWorkClient(conn)
}
func projectWorkCall(t *testing.T, client pb.ProjectWorkClient, request *pb.ProjectWorkRequest, token string) (*pb.ProjectWorkSnapshot, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	return client.ReadProjectWork(metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token)), request)
}

func TestExecutionProjectWorkRPCRequiresExactClosedAuthenticatedEnvelope(t *testing.T) {
	token := strings.Repeat("w", 40)
	request := executionapp.ProjectWorkRequest{CloseID: uuid.New(), ProjectID: uuid.New(), OrganizationID: uuid.New(), ScopeID: uuid.New(), ProjectEpoch: 3, ManifestSHA256: strings.Repeat("a", 64)}
	request.Commands = []writefence.Command{{ID: uuid.New(), Action: "execution.frozen_input.accept", SHA256: strings.Repeat("b", 64), Authorities: []writefence.Authority{{Issuer: writefence.Creation, Kind: writefence.Project, ID: request.ProjectID, Epoch: 2}, {Issuer: writefence.Creation, Kind: writefence.Scope, ID: request.ScopeID, Epoch: 1}}}}
	var calls atomic.Int32
	reader := projectWorkReaderFunc(func(ctx context.Context, request executionapp.ProjectWorkRequest) (executionapp.ProjectWorkSnapshot, error) {
		calls.Add(1)
		for _, key := range []string{"cookie", "baggage", "authorization", "x-actor-id"} {
			if len(metadata.ValueFromIncomingContext(ctx, key)) != 0 {
				t.Errorf("private metadata leaked to owner: %s", key)
			}
		}
		return executionapp.ProjectWorkSnapshot{Request: request, BusinessState: executionapp.ProjectWorkUnknown, Commands: []executionapp.ProjectCommandObservation{{Command: request.Commands[0], State: writefence.Committed, ReceiptSHA256: strings.Repeat("c", 64)}}}, nil
	})
	serverTLS, clientTLS, wrongPeer := ownerPeerTLS(t, "creation")
	server, err := workrpc.NewServer(reader, token, serverTLS)
	if err != nil {
		t.Fatal(err)
	}
	address := serveFrozenRPC(t, server)
	client := projectWorkRPCClient(t, address, clientTLS)
	wire := executionWorkWire(request)
	response, err := projectWorkCall(t, client, wire, token)
	if err != nil || response.BusinessState != pb.BusinessState_UNKNOWN || !proto.Equal(response.Request, wire) || len(response.Commands) != 1 || !proto.Equal(response.Commands[0].Command, wire.Commands[0]) || response.Commands[0].ReceiptSha256 != strings.Repeat("c", 64) {
		t.Fatalf("responsibility echo=%+v err=%v", response, err)
	}
	for _, bad := range []string{"wrong token", "duplicate authorization", "unbounded deadline", "unknown command field", "unknown authority field", "retargeted project", "oversized page"} {
		t.Run(bad, func(t *testing.T) {
			value := proto.Clone(wire).(*pb.ProjectWorkRequest)
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			md := metadata.Pairs("authorization", "Bearer "+token)
			expected := codes.InvalidArgument
			switch bad {
			case "wrong token":
				md.Set("authorization", "Bearer "+strings.Repeat("x", 40))
				expected = codes.Unauthenticated
			case "duplicate authorization":
				md.Append("authorization", "Bearer "+token)
				expected = codes.Unauthenticated
			case "unbounded deadline":
				ctx = t.Context()
			case "unknown command field":
				value.Commands[0].ProtoReflect().SetUnknown([]byte{0x98, 0x06, 0x01})
			case "unknown authority field":
				value.Commands[0].Authorities[0].ProtoReflect().SetUnknown([]byte{0x98, 0x06, 0x01})
			case "retargeted project":
				id := uuid.New()
				value.ProjectId = id[:]
			case "oversized page":
				for len(value.Commands) < 129 {
					value.Commands = append(value.Commands, value.Commands[0])
				}
			}
			_, err := client.ReadProjectWork(metadata.NewOutgoingContext(ctx, md), value)
			if status.Code(err) != expected {
				t.Fatalf("status=%v want=%v", err, expected)
			}
		})
	}
	if calls.Load() != 1 {
		t.Fatalf("invalid requests reached owner: %d", calls.Load())
	}
	_, err = projectWorkCall(t, projectWorkRPCClient(t, address, wrongPeer), wire, token)
	if err == nil || calls.Load() != 1 {
		t.Fatalf("wrong mTLS peer reached owner: calls=%d err=%v", calls.Load(), err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	_, err = client.ReadProjectWork(metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token, "cookie", "session=private", "baggage", "private=1", "x-actor-id", uuid.NewString())), wire)
	if err != nil || calls.Load() != 2 {
		t.Fatal("sanitized authenticated request", err, calls.Load())
	}
}

func TestExecutionProjectWorkRPCUsesConfiguredCreationDNS(t *testing.T) {
	serverTLS, creation, _ := ownerPeerTLS(t, "creation.runtime")
	var calls atomic.Int32
	reader := projectWorkReaderFunc(func(_ context.Context, request executionapp.ProjectWorkRequest) (executionapp.ProjectWorkSnapshot, error) {
		calls.Add(1)
		return executionapp.ProjectWorkSnapshot{Request: request, BusinessState: executionapp.ProjectWorkIdle}, nil
	})
	token := strings.Repeat("d", 40)
	server, err := workrpc.NewPeerServer(reader, token, "creation.runtime", serverTLS)
	if err != nil {
		t.Fatal(err)
	}
	request := executionapp.ProjectWorkRequest{CloseID: uuid.New(), ProjectID: uuid.New(), OrganizationID: uuid.New(), ScopeID: uuid.New(), ProjectEpoch: 1, ManifestSHA256: strings.Repeat("a", 64)}
	response, err := projectWorkCall(t, projectWorkRPCClient(t, serveFrozenRPC(t, server), creation), executionWorkWire(request), token)
	if err != nil || calls.Load() != 1 || !proto.Equal(response.Request, executionWorkWire(request)) {
		t.Fatalf("configured Creation peer rejected: calls=%d err=%v", calls.Load(), err)
	}
}

func TestExecutionProjectWorkRPCRejectsWrongRoleWithVerifiedSameCA(t *testing.T) {
	// TLS deliberately accepts this same-CA Execution certificate. The private
	// Creation-only RPC must independently enforce the intended peer identity.
	serverTLS, wrongRole, _ := ownerPeerTLS(t, "execution")
	var calls atomic.Int32
	reader := projectWorkReaderFunc(func(_ context.Context, request executionapp.ProjectWorkRequest) (executionapp.ProjectWorkSnapshot, error) {
		calls.Add(1)
		return executionapp.ProjectWorkSnapshot{Request: request, BusinessState: executionapp.ProjectWorkIdle}, nil
	})
	token := strings.Repeat("d", 40)
	server, err := workrpc.NewServer(reader, token, serverTLS)
	if err != nil {
		t.Fatal(err)
	}
	request := executionapp.ProjectWorkRequest{CloseID: uuid.New(), ProjectID: uuid.New(), OrganizationID: uuid.New(), ScopeID: uuid.New(), ProjectEpoch: 1, ManifestSHA256: strings.Repeat("a", 64)}
	client := projectWorkRPCClient(t, serveFrozenRPC(t, server), wrongRole)
	_, err = projectWorkCall(t, client, executionWorkWire(request), token)
	if status.Code(err) != codes.Unauthenticated || calls.Load() != 0 {
		t.Fatalf("verified wrong role reached Creation RPC: calls=%d err=%v", calls.Load(), err)
	}
}

func TestExecutionProjectWorkRPCUsesActualRestrictedPostgres(t *testing.T) {
	f := newFrozenFixture(t)
	request := executionProjectWorkRequest(t, f)
	request.Commands = []writefence.Command{f.request.Command}
	serverTLS, clientTLS, _ := ownerPeerTLS(t, "creation")
	token := strings.Repeat("r", 40)
	server, err := workrpc.NewServer(executionpg.NewProjectWorkStore(f.execution), token, serverTLS)
	if err != nil {
		t.Fatal(err)
	}
	client := projectWorkRPCClient(t, serveFrozenRPC(t, server), clientTLS)
	wire := executionWorkWire(request)
	response, err := projectWorkCall(t, client, wire, token)
	if err != nil || response.BusinessState != pb.BusinessState_BUSY || len(response.Commands) != 1 || response.Commands[0].State != pb.CommandState_ADMITTED || !proto.Equal(response.Request, wire) {
		t.Fatalf("actual project responsibility=%+v err=%v", response, err)
	}
}
