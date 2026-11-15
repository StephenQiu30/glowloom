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
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediacover"
	"github.com/StephenQiu30/glowloom-server/backend/media/adapter/grpccover"
	mediapg "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

type mediaCoverRead func(context.Context, mediaapp.ProjectCoverRequest) (mediaapp.ProjectCoverSnapshot, error)

func (f mediaCoverRead) ReadProjectCover(ctx context.Context, r mediaapp.ProjectCoverRequest) (mediaapp.ProjectCoverSnapshot, error) {
	return f(ctx, r)
}

func mediaCoverWire(r mediaapp.ProjectCoverRequest) *pb.ProjectCoverRequest {
	return &pb.ProjectCoverRequest{CloseId: r.CloseID[:], ProjectId: r.ProjectID[:], OrganizationId: r.OrganizationID[:], ScopeId: r.ScopeID[:], AssetId: r.AssetID[:], ProjectEpoch: r.ProjectEpoch, CoordinationId: r.CoordinationID[:]}
}

func TestMediaProjectCoverActualClosedPGOverAuthenticatedTCP(t *testing.T) {
	f := newMediaCoverFixture(t)
	request := mediaCoverRequest(t, f)
	installMediaCoverBarrier(t, f, request)
	serverTLS, clientTLS, _ := ownerPeerTLS(t, "creation")
	token := strings.Repeat("creation-cover-fixture", 2)
	server, err := grpccover.NewServer(mediapg.NewProjectCoverReader(f.media), token, "creation", serverTLS)
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
	wire := mediaCoverWire(request)
	client := pb.NewProjectCoverStatesClient(connection)
	value, err := client.ReadProjectCover(ctx, wire)
	if err != nil || !proto.Equal(value.GetRequest(), wire) || !value.Available || value.AssetRevision != 1 {
		t.Fatalf("original closed cover: %+v %v", value, err)
	}
	if err := f.admin.Exec(`UPDATE media.object_location SET state='unknown' WHERE id=?`, f.acquire.Identity.LocationID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReadProjectCover(ctx, wire); status.Code(err) != codes.Unavailable {
		t.Fatalf("unknown became an unavailable display fact: %v", err)
	}
}

func TestMediaProjectCoverTransportClosesRequestsAndReaderFacts(t *testing.T) {
	request := mediaapp.ProjectCoverRequest{CloseID: uuid.New(), ProjectID: uuid.New(), OrganizationID: uuid.New(), ScopeID: uuid.New(), AssetID: uuid.New(), ProjectEpoch: 2}
	request.CoordinationID = uuid.NewSHA1(request.CloseID, []byte("creation.project.close.authority"))
	var calls atomic.Int32
	var mode atomic.Value
	mode.Store("")
	reader := mediaCoverRead(func(ctx context.Context, r mediaapp.ProjectCoverRequest) (mediaapp.ProjectCoverSnapshot, error) {
		calls.Add(1)
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 10*time.Second {
			t.Error("unbounded cover read")
		}
		value := mediaapp.ProjectCoverSnapshot{Request: r, Available: true, AssetRevision: 4}
		switch mode.Load().(string) {
		case "reader-mismatch":
			value.Request.AssetID = uuid.New()
		case "reader-invalid-revision":
			value.AssetRevision = 0
		case "reader-unavailable":
			return mediaapp.ProjectCoverSnapshot{}, mediaapp.ErrUnavailable
		}
		return value, nil
	})
	serverTLS, clientTLS, wrongPeer := ownerPeerTLS(t, "creation")
	token := strings.Repeat("closed-cover-audience-fixture", 2)
	server, err := grpccover.NewServer(reader, token, "creation", serverTLS)
	if err != nil {
		t.Fatal(err)
	}
	address := serveSourcePipeline(t, server)
	for _, shape := range []string{"wrong-peer", "wrong-token", "duplicate-auth", "extension", "bad-uuid", "wrong-control", "invalid-epoch", "reader-mismatch", "reader-invalid-revision", "reader-unavailable", "valid"} {
		t.Run(shape, func(t *testing.T) {
			mode.Store(shape)
			transport := clientTLS.Clone()
			if shape == "wrong-peer" {
				transport = wrongPeer.Clone()
			}
			connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(transport), grpc.WithDisableRetry())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = connection.Close() }()
			wire := mediaCoverWire(request)
			pairs := []string{"authorization", "Bearer " + token}
			want := codes.InvalidArgument
			wantCalls := int32(0)
			switch shape {
			case "wrong-peer":
				want = codes.Unavailable
			case "wrong-token":
				pairs[1] = "Bearer " + strings.Repeat("different-cover-audience", 2)
				want = codes.Unauthenticated
			case "duplicate-auth":
				pairs = append(pairs, "authorization", "Bearer "+token)
				want = codes.Unauthenticated
			case "extension":
				wire.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
			case "bad-uuid":
				wire.AssetId = []byte{1}
			case "wrong-control":
				id := uuid.New()
				wire.CoordinationId = id[:]
			case "invalid-epoch":
				wire.ProjectEpoch = 1
			case "reader-mismatch", "reader-invalid-revision":
				want = codes.FailedPrecondition
				wantCalls = 1
			case "reader-unavailable":
				want = codes.Unavailable
				wantCalls = 1
			case "valid":
				want = codes.OK
				wantCalls = 1
			}
			before := calls.Load()
			ctx, cancel := context.WithTimeout(metadata.NewOutgoingContext(t.Context(), metadata.Pairs(pairs...)), 5*time.Second)
			defer cancel()
			value, err := pb.NewProjectCoverStatesClient(connection).ReadProjectCover(ctx, wire)
			if status.Code(err) != want || calls.Load()-before != wantCalls {
				t.Fatalf("shape=%s code=%s calls=%d want=%s/%d: %v", shape, status.Code(err), calls.Load()-before, want, wantCalls, err)
			}
			if want == codes.OK && (!proto.Equal(value.GetRequest(), wire) || !value.Available || value.AssetRevision != 4) {
				t.Fatalf("unbound observation: %+v", value)
			}
		})
	}
}
