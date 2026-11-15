package microservices_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/identitysource"
	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

type sourceAuthorityServer struct {
	pb.UnimplementedAuthorityServer
	verify  func(context.Context, *pb.VerifyRequest) (*pb.VerifyResponse, error)
	admit   func(context.Context, *pb.AdmitSessionCommandRequest) (*pb.AdmitSessionCommandResponse, error)
	resolve func(context.Context, *pb.ResolveAdmissionRequest) (*pb.ResolveAdmissionResponse, error)
}

func (s *sourceAuthorityServer) Verify(ctx context.Context, request *pb.VerifyRequest) (*pb.VerifyResponse, error) {
	return s.verify(ctx, request)
}
func (s *sourceAuthorityServer) AdmitSessionCommand(ctx context.Context, request *pb.AdmitSessionCommandRequest) (*pb.AdmitSessionCommandResponse, error) {
	return s.admit(ctx, request)
}
func (s *sourceAuthorityServer) ResolveAdmission(ctx context.Context, request *pb.ResolveAdmissionRequest) (*pb.ResolveAdmissionResponse, error) {
	return s.resolve(ctx, request)
}
func startSourceAuthority(t *testing.T, server *sourceAuthorityServer) *identitysource.Client {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	transport := grpc.NewServer()
	pb.RegisterAuthorityServer(transport, server)
	done := make(chan error, 1)
	go func() { done <- transport.Serve(listener) }()
	t.Cleanup(func() {
		transport.Stop()
		if err := <-done; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Error(err)
		}
	})
	client, err := identitysource.New(listener.Addr().String(), strings.Repeat("c", 32), insecure.NewCredentials())
	if err != nil {
		t.Fatal(err)
	}
	return client
}
func sourceWireIdentity(id workspacesource.Identity) *pb.Identity {
	return &pb.Identity{AccountId: append([]byte(nil), id.AccountID[:]...), SessionId: append([]byte(nil), id.SessionID[:]...), CredentialEpoch: id.CredentialEpoch}
}
func sourceRPCProof() string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))
}

func TestCreationSourceIdentityRPCUsesFixedCallerOriginalProofAndExactCommand(t *testing.T) {
	id, request := sourceAdmissionFixture()
	var verifies, admits atomic.Int32
	check := func(ctx context.Context) {
		t.Helper()
		md, _ := metadata.FromIncomingContext(ctx)
		if got := md.Get("authorization"); len(got) != 1 || got[0] != "Bearer "+strings.Repeat("c", 32) {
			t.Error("dedicated Creation authentication missing")
		}
	}
	client := startSourceAuthority(t, &sourceAuthorityServer{
		verify: func(ctx context.Context, r *pb.VerifyRequest) (*pb.VerifyResponse, error) {
			check(ctx)
			verifies.Add(1)
			if r.Caller != coordination.Writer_WRITER_CREATION || r.Credential.GetAccessToken() != "original.access.token" || r.Credential.CsrfToken != sourceRPCProof() {
				t.Error("original credential or fixed caller changed")
			}
			return &pb.VerifyResponse{Identity: sourceWireIdentity(id)}, nil
		},
		admit: func(ctx context.Context, r *pb.AdmitSessionCommandRequest) (*pb.AdmitSessionCommandResponse, error) {
			check(ctx)
			admits.Add(1)
			if r.Caller != coordination.Writer_WRITER_CREATION || r.Target != coordination.Writer_WRITER_CREATION || r.Credential.GetAccessToken() != "original.access.token" || r.Credential.CsrfToken != sourceRPCProof() || string(r.Command.Id) != string(request.Command.ID[:]) || r.Command.Sha256 != request.Command.SHA256 || r.Command.Action != request.Command.Action || len(r.Command.Authorities) != 4 {
				t.Error("admission binding changed")
			}
			return &pb.AdmitSessionCommandResponse{Identity: sourceWireIdentity(id), Command: r.Command, Target: r.Target}, nil
		},
	})
	credential := workspacesource.AccessCredential("original.access.token", sourceRPCProof())
	verified, err := client.VerifySourceIdentity(t.Context(), credential)
	if err != nil || verified != id {
		t.Fatalf("verify: %+v %v", verified, err)
	}
	admitted, err := client.AdmitSourceSession(t.Context(), credential, request.Command, writefence.WriterCreation)
	if err != nil || admitted != id {
		t.Fatalf("admit: %+v %v", admitted, err)
	}
	if _, err := client.AdmitSourceSession(t.Context(), credential, request.Command, writefence.WriterExecution); !errors.Is(err, writefence.ErrInvalid) {
		t.Fatalf("Creation delegated to foreign writer: %v", err)
	}
	if verifies.Load() != 1 || admits.Load() != 1 {
		t.Fatalf("calls: verify %d admit %d", verifies.Load(), admits.Load())
	}
}

func TestCreationSourceIdentityRPCRejectsInexactOrUnknownAcknowledgements(t *testing.T) {
	for _, shape := range []string{"target", "command", "identity", "unknown-envelope", "unknown-identity", "unknown-command", "unknown-authority", "lost-ack"} {
		t.Run(shape, func(t *testing.T) {
			id, request := sourceAdmissionFixture()
			var calls atomic.Int32
			client := startSourceAuthority(t, &sourceAuthorityServer{admit: func(_ context.Context, r *pb.AdmitSessionCommandRequest) (*pb.AdmitSessionCommandResponse, error) {
				calls.Add(1)
				response := &pb.AdmitSessionCommandResponse{Identity: sourceWireIdentity(id), Command: proto.Clone(r.Command).(*coordination.Command), Target: r.Target}
				switch shape {
				case "target":
					response.Target = coordination.Writer_WRITER_EXECUTION
				case "command":
					response.Command.Sha256 = strings.Repeat("b", 64)
				case "identity":
					response.Identity.SessionId = response.Identity.AccountId
				case "unknown-envelope":
					response.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
				case "unknown-identity":
					response.Identity.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
				case "unknown-command":
					response.Command.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
				case "unknown-authority":
					response.Command.Authorities[0].ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
				case "lost-ack":
					return nil, status.Error(codes.Unavailable, "ack unavailable")
				}
				return response, nil
			}})
			_, err := client.AdmitSourceSession(t.Context(), workspacesource.LegacyCredential(sourceRPCProof(), sourceRPCProof()), request.Command, writefence.WriterCreation)
			if err == nil {
				t.Fatal("inexact or unknown response granted admission")
			}
			if calls.Load() != 1 {
				t.Fatalf("effect RPC retried: %d", calls.Load())
			}
		})
	}
}

func TestCreationSourceIdentityRPCRequiresCSRFForAdmissionAndBoundsConfiguration(t *testing.T) {
	id, request := sourceAdmissionFixture()
	var calls atomic.Int32
	client := startSourceAuthority(t, &sourceAuthorityServer{verify: func(_ context.Context, _ *pb.VerifyRequest) (*pb.VerifyResponse, error) {
		calls.Add(1)
		return &pb.VerifyResponse{Identity: sourceWireIdentity(id)}, nil
	}})
	if _, err := client.VerifySourceIdentity(t.Context(), workspacesource.LegacyCredential(sourceRPCProof(), "")); err != nil {
		t.Fatal("read-only verification requires no CSRF", err)
	}
	if _, err := client.AdmitSourceSession(t.Context(), workspacesource.LegacyCredential(sourceRPCProof(), ""), request.Command, writefence.WriterCreation); !errors.Is(err, writefence.ErrInvalid) {
		t.Fatalf("write omitted CSRF: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("invalid write reached RPC: %d", calls.Load())
	}
	for _, address := range []string{"", "localhost", "localhost:0", "localhost:65536", "http://localhost:10", "user@localhost:10"} {
		if _, err := identitysource.New(address, strings.Repeat("c", 32), insecure.NewCredentials()); !errors.Is(err, writefence.ErrInvalid) {
			t.Fatalf("bad address %q accepted: %v", address, err)
		}
	}
	if _, err := identitysource.New("localhost:10", strings.Repeat("c", 32), nil); !errors.Is(err, writefence.ErrInvalid) {
		t.Fatal("implicit transport accepted")
	}
	if _, err := identitysource.New("localhost:10", "short", insecure.NewCredentials()); !errors.Is(err, writefence.ErrInvalid) {
		t.Fatal("weak token accepted")
	}
}
