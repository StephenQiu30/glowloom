package microservices_test

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

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediawork"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/media/adapter/grpcprojectwork"
	mediapg "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

type mediaWorkRead func(context.Context, mediaapp.ProjectWorkRequest) (mediaapp.ProjectWorkSnapshot, error)

func (f mediaWorkRead) ReadProjectWork(ctx context.Context, r mediaapp.ProjectWorkRequest) (mediaapp.ProjectWorkSnapshot, error) {
	return f(ctx, r)
}

func mediaWorkWire(r mediaapp.ProjectWorkRequest) *pb.ProjectWorkRequest {
	wire := &pb.ProjectWorkRequest{CloseId: r.CloseID[:], ProjectId: r.ProjectID[:], OrganizationId: r.OrganizationID[:], ScopeId: r.ScopeID[:], ProjectEpoch: r.ProjectEpoch, ManifestSha256: r.ManifestSHA256}
	for _, command := range r.Commands {
		wire.Commands = append(wire.Commands, grpctransport.EncodeCommand(command))
	}
	return wire
}

func TestMediaProjectWorkActualPGOverAuthenticatedTCP(t *testing.T) {
	f := newPinFixture(t)
	request := mediaWorkRequest(t, f)
	request.Commands = []writefence.Command{f.acquire.Command}
	serverTLS, clientTLS, _ := ownerPeerTLS(t, "creation")
	token := strings.Repeat("media-work-creation-fixture", 2)
	server, err := grpcprojectwork.NewServer(mediapg.NewProjectWorkReader(f.media), token, "creation", serverTLS)
	if err != nil {
		t.Fatal(err)
	}
	address := serveSourcePipeline(t, server)
	connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(clientTLS.Clone()), grpc.WithDisableRetry())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	ctx, cancel := context.WithTimeout(metadata.NewOutgoingContext(t.Context(), metadata.Pairs("authorization", "Bearer "+token)), 5*time.Second)
	defer cancel()
	wire := mediaWorkWire(request)
	client := pb.NewProjectWorkClient(connection)
	value, err := client.ReadProjectWork(ctx, wire)
	if err != nil || !proto.Equal(value.GetRequest(), wire) || value.BusinessState != pb.BusinessState_IDLE || len(value.Commands) != 1 || value.Commands[0].State != pb.CommandState_COMMAND_UNKNOWN {
		t.Fatalf("original responsibility page: %+v %v", value, err)
	}
	if _, err := pgfence.New(f.media).Load(t.Context(), f.acquire.Command.ID); !errors.Is(err, writefence.ErrNotFound) {
		t.Fatalf("read registered a missing command: %v", err)
	}
	if err := f.media.Exec(`UPDATE media.media_asset SET status='processing' WHERE id=?`, f.acquire.Identity.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	value, err = client.ReadProjectWork(ctx, wire)
	if err != nil || value.BusinessState != pb.BusinessState_BUSY {
		t.Fatalf("physical processing not retained: %+v %v", value, err)
	}
}

func TestMediaProjectWorkTransportRejectsForeignAndOpenEndedEvidence(t *testing.T) {
	request := mediaapp.ProjectWorkRequest{CloseID: uuid.New(), ProjectID: uuid.New(), OrganizationID: uuid.New(), ScopeID: uuid.New(), ProjectEpoch: 7, ManifestSHA256: strings.Repeat("e", 64)}
	request.Commands = []writefence.Command{{ID: uuid.New(), SHA256: strings.Repeat("a", 64), Action: "media.reference_pin.acquire", Authorities: []writefence.Authority{{Issuer: writefence.Creation, Kind: writefence.Project, ID: request.ProjectID, Epoch: 7}, {Issuer: writefence.Creation, Kind: writefence.Scope, ID: request.ScopeID, Epoch: 3}}}}
	var calls atomic.Int32
	var unavailable atomic.Bool
	reader := mediaWorkRead(func(ctx context.Context, r mediaapp.ProjectWorkRequest) (mediaapp.ProjectWorkSnapshot, error) {
		calls.Add(1)
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 10*time.Second {
			t.Error("unbounded work read")
		}
		if unavailable.Load() {
			return mediaapp.ProjectWorkSnapshot{}, mediaapp.ErrUnavailable
		}
		return mediaapp.ProjectWorkSnapshot{Request: r, BusinessState: mediaapp.ProjectWorkIdle, Commands: []mediaapp.ProjectCommandObservation{{Command: r.Commands[0], State: "unknown"}}}, nil
	})
	serverTLS, clientTLS, wrongPeer := ownerPeerTLS(t, "creation")
	token := strings.Repeat("media-work-closed-fixture", 2)
	server, err := grpcprojectwork.NewServer(reader, token, "creation", serverTLS)
	if err != nil {
		t.Fatal(err)
	}
	address := serveSourcePipeline(t, server)
	original := mediaWorkWire(request)
	for _, shape := range []string{"wrong-peer", "wrong-token", "duplicate-auth", "request-extension", "command-extension", "authority-extension", "bad-uuid", "bad-manifest", "too-many", "valid", "unavailable"} {
		t.Run(shape, func(t *testing.T) {
			transport := clientTLS.Clone()
			if shape == "wrong-peer" {
				transport = wrongPeer.Clone()
			}
			connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(transport), grpc.WithDisableRetry())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = connection.Close() }()
			wire := proto.Clone(original).(*pb.ProjectWorkRequest)
			audience := token
			if shape == "wrong-token" {
				audience = strings.Repeat("other-audience-fixture", 2)
			}
			pairs := []string{"authorization", "Bearer " + audience}
			want := codes.InvalidArgument
			switch shape {
			case "wrong-peer":
				want = codes.Unavailable // The fixture certificate is rejected during TLS, before RPC authentication.
			case "wrong-token":
				want = codes.Unauthenticated
			case "duplicate-auth":
				pairs = append(pairs, "authorization", "Bearer "+token)
				want = codes.Unauthenticated
			case "request-extension":
				wire.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
			case "command-extension":
				wire.Commands[0].ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
			case "authority-extension":
				wire.Commands[0].Authorities[0].ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
			case "bad-uuid":
				wire.ProjectId = []byte{1}
			case "bad-manifest":
				wire.ManifestSha256 = ""
			case "too-many":
				for len(wire.Commands) < 129 {
					wire.Commands = append(wire.Commands, wire.Commands[0])
				}
			case "valid":
				want = codes.OK
			case "unavailable":
				unavailable.Store(true)
				want = codes.Unavailable
			}
			ctx, cancel := context.WithTimeout(metadata.NewOutgoingContext(t.Context(), metadata.Pairs(pairs...)), 5*time.Second)
			defer cancel()
			before := calls.Load()
			value, err := pb.NewProjectWorkClient(connection).ReadProjectWork(ctx, wire)
			if status.Code(err) != want {
				t.Fatalf("evidence %s: %s %v", shape, status.Code(err), err)
			}
			switch shape {
			case "valid":
				if !proto.Equal(value.Request, original) || calls.Load() != before+1 {
					t.Fatal("valid original request was not echoed")
				}
			case "unavailable":
				if value != nil || calls.Load() != before+1 {
					t.Fatal("unavailable reader became an idle proof")
				}
			default:
				if calls.Load() != before {
					t.Fatal("foreign or extended request reached the reader")
				}
			}
		})
	}
}
