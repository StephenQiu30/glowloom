package identityauthority_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	authority "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcauthority"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func (p *exactPolicy) ReadSessionAdmission(ctx context.Context, command writefence.Command, target writefence.Writer, clock func() time.Time) (identity.SessionAdmissionProof, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reads++
	if err := ctx.Err(); err != nil {
		return identity.SessionAdmissionProof{}, err
	}
	if !p.registered {
		return identity.SessionAdmissionProof{}, writefence.ErrNotFound
	}
	if !p.open {
		return identity.SessionAdmissionProof{}, writefence.ErrFenced
	}
	a, err := command.AuthorityDigest()
	if err != nil {
		return identity.SessionAdmissionProof{}, err
	}
	b, _ := p.command.AuthorityDigest()
	if command.ID != p.command.ID || command.SHA256 != p.command.SHA256 || command.Action != p.command.Action || a != b || target != writefence.WriterMedia || clock == nil {
		return identity.SessionAdmissionProof{}, writefence.ErrConflict
	}
	return identity.SessionAdmissionProof{AccountID: p.actor.ID, SessionID: p.actor.SessionID, CredentialEpoch: p.actor.CredentialEpoch, Command: p.command, Target: target}, nil
}
func TestResolveAdmissionRequiresCreationAndOriginalDurableAdmission(t *testing.T) {
	auth, policy, tokens := fixture(t)
	s, err := authority.NewServer(auth, policy, tokens, time.Now, insecure.NewCredentials())
	require.NoError(t, err)
	address := serve(t, s)
	creation, err := authority.NewClient(writefence.WriterCreation, authority.Target{Address: address, Token: tokens[writefence.WriterCreation], Transport: insecure.NewCredentials()})
	require.NoError(t, err)
	_, err = creation.ResolveAdmission(t.Context(), policy.command, writefence.WriterMedia)
	require.ErrorIs(t, err, writefence.ErrNotFound)
	require.Zero(t, policy.calls)
	media, err := authority.NewClient(writefence.WriterMedia, authority.Target{Address: address, Token: tokens[writefence.WriterMedia], Transport: insecure.NewCredentials()})
	require.NoError(t, err)
	_, err = media.AdmitSessionCommand(t.Context(), authority.LegacyCredential(auth.result.CSRFToken, auth.result.CSRFToken), policy.command, writefence.WriterMedia)
	require.NoError(t, err)
	proof, err := creation.ResolveAdmission(t.Context(), policy.command, writefence.WriterMedia)
	require.NoError(t, err)
	require.Equal(t, policy.actor.ID, proof.Identity.AccountID)
	require.Equal(t, policy.actor.SessionID, proof.Identity.SessionID)
	require.Equal(t, policy.actor.CredentialEpoch, proof.Identity.CredentialEpoch)
	require.Equal(t, policy.command, proof.Command)
	require.Equal(t, writefence.WriterMedia, proof.Target)
	require.Equal(t, 1, policy.calls)
	for _, caller := range []writefence.Writer{writefence.WriterIdentity, writefence.WriterMedia, writefence.WriterExecution} {
		_, err = rawClient(t, address).ResolveAdmission(privateContext(t, tokens[caller]), &pb.ResolveAdmissionRequest{Caller: coordination.Writer_WRITER_CREATION, Command: wireCommand(policy.command), Target: coordination.Writer_WRITER_MEDIA})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	}
	policy.open = false
	_, err = creation.ResolveAdmission(t.Context(), policy.command, writefence.WriterMedia)
	require.ErrorIs(t, err, writefence.ErrFenced)
	require.Equal(t, 1, policy.calls)
}

type proofResponseServer struct {
	pb.UnimplementedAuthorityServer
	mutate  func(*pb.ResolveAdmissionResponse)
	calls   atomic.Int32
	wait    bool
	entered chan struct{}
}

func (s *proofResponseServer) ResolveAdmission(ctx context.Context, r *pb.ResolveAdmissionRequest) (*pb.ResolveAdmissionResponse, error) {
	s.calls.Add(1)
	if s.entered != nil {
		close(s.entered)
	}
	if s.wait {
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	response := &pb.ResolveAdmissionResponse{Proof: &pb.AdmissionProof{Identity: &pb.Identity{AccountId: r.Command.Authorities[0].Id, SessionId: r.Command.Authorities[1].Id, CredentialEpoch: 3}, Command: r.Command, Target: r.Target}}
	if s.mutate != nil {
		s.mutate(response)
	}
	return response, nil
}
func proofClient(t *testing.T, server *proofResponseServer) *authority.Client {
	t.Helper()
	_, _, tokens := fixture(t)
	s := grpc.NewServer()
	pb.RegisterAuthorityServer(s, server)
	client, err := authority.NewClient(writefence.WriterCreation, authority.Target{Address: serve(t, s), Token: tokens[writefence.WriterCreation], Transport: insecure.NewCredentials()})
	require.NoError(t, err)
	return client
}
func TestResolveAdmissionRejectsMismatchedAndUnknownProof(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*pb.ResolveAdmissionResponse)
	}{
		{"target", func(r *pb.ResolveAdmissionResponse) { r.Proof.Target = coordination.Writer_WRITER_EXECUTION }},
		{"command", func(r *pb.ResolveAdmissionResponse) { r.Proof.Command.Action = "other.action" }},
		{"epoch", func(r *pb.ResolveAdmissionResponse) { r.Proof.Command.Authorities[0].Epoch++ }},
		{"identity", func(r *pb.ResolveAdmissionResponse) { id := uuid.New(); r.Proof.Identity.SessionId = id[:] }},
		{"unknown", func(r *pb.ResolveAdmissionResponse) { unknown(r.Proof) }},
		{"nested unknown", func(r *pb.ResolveAdmissionResponse) { unknown(r.Proof.Command.Authorities[0]) }},
		{"missing", func(r *pb.ResolveAdmissionResponse) { r.Proof = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, policy, _ := fixture(t)
			server := &proofResponseServer{mutate: tc.change}
			client := proofClient(t, server)
			_, err := client.ResolveAdmission(t.Context(), policy.command, writefence.WriterMedia)
			require.ErrorIs(t, err, writefence.ErrUnavailable)
			require.EqualValues(t, 1, server.calls.Load())
		})
	}
}
func TestResolveAdmissionUnknownOutcomeHasNoReplay(t *testing.T) {
	_, policy, _ := fixture(t)
	server := &proofResponseServer{wait: true, entered: make(chan struct{})}
	client := proofClient(t, server)
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	_, err := client.ResolveAdmission(ctx, policy.command, writefence.WriterMedia)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.EqualValues(t, 1, server.calls.Load())
}
