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

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/grpcpins"
	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationreference"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
	"github.com/StephenQiu30/glowloom-server/backend/media/adapter/creationpins"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

type pinSourceAdmitCall func(context.Context, workspacesource.AdmissionRequest) (workspacesource.Permit, error)

func (f pinSourceAdmitCall) Admit(ctx context.Context, request workspacesource.AdmissionRequest) (workspacesource.Permit, error) {
	return f(ctx, request)
}

func TestMediaPinSourceTransportRequiresExactMediaPeerAndClosedCommand(t *testing.T) {
	f := newPinFixture(t)
	proof, _ := (pinSourceFixture{}).AdmitReferencePinSource(t.Context(), f.acquire)
	var calls atomic.Int32
	admitter := pinSourceAdmitCall(func(ctx context.Context, r workspacesource.AdmissionRequest) (workspacesource.Permit, error) {
		calls.Add(1)
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 10*time.Second || r.Target != writefence.WriterMedia || r.Command.Action != "media.reference_pin.acquire" {
			t.Error("unbounded or retargeted source policy call")
		}
		md, _ := metadata.FromIncomingContext(ctx)
		for _, key := range []string{"cookie", "x-actor-id", "x-internal-token"} {
			if len(md.Get(key)) != 0 {
				t.Errorf("client forwarded private metadata %s", key)
			}
		}
		return workspacesource.Permit{Command: r.Command, Target: r.Target, ScopeID: r.ScopeID, ProjectID: r.ProjectID, PhysicalScopeID: proof.PhysicalScopeID, AccountID: proof.AccountID, SessionID: proof.SessionID}, nil
	})
	serverTLS, clientTLS, wrongPeer := ownerPeerTLS(t, "media")
	token := strings.Repeat("pin-admission-transport-fixture", 2)
	server, err := grpcpins.NewServer(admitter, token, "media", serverTLS)
	if err != nil {
		t.Fatal(err)
	}
	address := serveSourcePipeline(t, server)
	for _, shape := range []string{"wrong-token", "wrong-peer", "valid"} {
		t.Run(shape, func(t *testing.T) {
			target := creationpins.Target{Address: address, Token: token, Transport: clientTLS}
			if shape == "wrong-token" {
				target.Token = strings.Repeat("different-pin-audience-fixture", 2)
			}
			if shape == "wrong-peer" {
				target.Transport = wrongPeer
			}
			client, err := creationpins.New(target)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = client.Close() }()
			ctx := metadata.NewOutgoingContext(t.Context(), metadata.Pairs("cookie", "synthetic-user-cookie", "x-actor-id", uuid.NewString(), "x-internal-token", "synthetic-incoming-token"))
			before := calls.Load()
			got, err := client.AdmitReferencePinSource(ctx, f.acquire)
			if shape == "valid" {
				if err != nil || got != proof || calls.Load() != before+1 {
					t.Fatal("valid exact source call", got, calls.Load(), err)
				}
			} else if !errors.Is(err, mediaapp.ErrUnavailable) || calls.Load() != before {
				t.Fatal("foreign capability reached source policy", calls.Load(), err)
			}
		})
	}
	connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(clientTLS.Clone()), grpc.WithDisableRetry())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	original := &pb.ReferencePinAdmissionRequest{Command: grpctransport.EncodeCommand(f.acquire.Command), ScopeId: f.acquire.Identity.ScopeID.String(), ProjectId: f.acquire.Identity.ProjectID.String()}
	for _, shape := range []string{"request-extension", "command-extension", "authority-extension", "other-action", "noncanonical-scope", "duplicate-auth"} {
		t.Run(shape, func(t *testing.T) {
			wire := proto.Clone(original).(*pb.ReferencePinAdmissionRequest)
			pairs := []string{"authorization", "Bearer " + token}
			switch shape {
			case "request-extension":
				wire.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
			case "command-extension":
				wire.Command.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
			case "authority-extension":
				wire.Command.Authorities[0].ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
			case "other-action":
				wire.Command.Action = "media.upload.publish"
			case "noncanonical-scope":
				wire.ScopeId = "{" + wire.ScopeId + "}"
			case "duplicate-auth":
				pairs = append(pairs, "authorization", "Bearer "+token)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			before := calls.Load()
			_, err := pb.NewReferencePinAdmissionsClient(connection).AdmitAcquisition(metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...)), wire)
			want := codes.InvalidArgument
			if shape == "duplicate-auth" {
				want = codes.Unauthenticated
			}
			if status.Code(err) != want || calls.Load() != before {
				t.Fatal("invalid wire reached policy", status.Code(err), calls.Load(), err)
			}
		})
	}
}
