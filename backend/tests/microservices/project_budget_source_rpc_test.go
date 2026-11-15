package microservices_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	sourcerpc "github.com/StephenQiu30/glowloom-server/backend/execution/billing/adapter/creationpreparation"
	billingapp "github.com/StephenQiu30/glowloom-server/backend/execution/billing/application"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationpreparation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

type projectSourceRPC struct {
	pb.UnimplementedProjectPreparationsServer
	mode  string
	token string
}

func (s *projectSourceRPC) ResolveProjectPreparation(ctx context.Context, in *pb.ProjectPreparationKey) (*pb.ProjectPreparedProof, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	auth := md.Get("authorization")
	deadline, ok := ctx.Deadline()
	if len(auth) != 1 || auth[0] != "Bearer "+s.token || len(md.Get("cookie")) != 0 || len(md.Get("baggage")) != 0 || len(md.Get("x-user-token")) != 0 || !ok || time.Until(deadline) > 10*time.Second {
		return nil, status.Error(codes.Unauthenticated, "unexpected forwarded metadata")
	}
	response := &pb.ProjectPreparedProof{Request: proto.Clone(in).(*pb.ProjectPreparationKey), Phase: "prepared"}
	switch s.mode {
	case "404":
		return nil, status.Error(codes.NotFound, "original source absent")
	case "unavailable":
		return nil, status.Error(codes.Unavailable, "source unavailable")
	case "preparing":
		response.Phase = "preparing"
	case "retargeted":
		id := uuid.New()
		response.Request.ProjectId = id[:]
	case "unknown":
		response.Request.ProtoReflect().SetUnknown([]byte{0x98, 0x06, 0x01})
	}
	return response, nil
}

func TestProjectBudgetSourceClientRequiresOriginalClosedPreparedFacts(t *testing.T) {
	for _, mode := range []string{"prepared", "404", "unavailable", "preparing", "retargeted", "unknown", "wrong_peer"} {
		t.Run(mode, func(t *testing.T) {
			transport, peer, wrong := ownerPeerTLS(t, "execution")
			token := strings.Repeat("p", 40)
			server := grpc.NewServer(grpc.Creds(transport))
			pb.RegisterProjectPreparationsServer(server, &projectSourceRPC{mode: mode, token: token})
			if mode == "wrong_peer" {
				peer = wrong
			}
			client, err := sourcerpc.New(sourcerpc.Target{Address: serveFrozenRPC(t, server), Token: token, Transport: peer})
			if err != nil {
				t.Fatal(err)
			}
			ctx := metadata.NewOutgoingContext(t.Context(), metadata.Pairs("authorization", "Bearer user-token", "cookie", "private cookie", "baggage", "sensitive", "x-user-token", "user credential"))
			p := projectBudgetRequest()
			proof, err := client.ResolveProjectPreparation(ctx, p)
			switch mode {
			case "prepared":
				if err != nil || proof.Request != p || proof.Phase != "prepared" {
					t.Fatalf("original source proof failed: %+v %v", proof, err)
				}
			case "404":
				if !errors.Is(err, billingapp.ErrProjectBudgetUnknown) {
					t.Fatalf("404 became ready: %v", err)
				}
			case "unavailable", "wrong_peer":
				if !errors.Is(err, writefence.ErrUnavailable) {
					t.Fatalf("failed peer became ready: %v", err)
				}
			default:
				if !errors.Is(err, billingapp.ErrProjectBudgetPreparation) {
					t.Fatalf("changed source accepted: %v", err)
				}
			}
		})
	}
}
