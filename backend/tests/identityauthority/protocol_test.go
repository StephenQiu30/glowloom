package identityauthority_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	authority "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcauthority"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func rawClient(t *testing.T, address string) pb.AuthorityClient {
	t.Helper()
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return pb.NewAuthorityClient(conn)
}
func privateContext(t *testing.T, token string) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	return metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
}
func wireCommand(c writefence.Command) *coordination.Command {
	value := &coordination.Command{Id: c.ID[:], Sha256: c.SHA256, Action: c.Action}
	for _, a := range c.Authorities {
		issuer := coordination.Issuer_ISSUER_IDENTITY
		kind := coordination.AuthorityKind_AUTHORITY_KIND_ACCOUNT
		if a.Issuer == writefence.Creation {
			issuer = coordination.Issuer_ISSUER_CREATION
			kind = coordination.AuthorityKind_AUTHORITY_KIND_SCOPE
		} else if a.Kind == writefence.Session {
			kind = coordination.AuthorityKind_AUTHORITY_KIND_SESSION
		}
		value.Authorities = append(value.Authorities, &coordination.Authority{Issuer: issuer, Kind: kind, Id: a.ID[:], Epoch: a.Epoch})
	}
	return value
}
func requestFor(a *policyAuth, s *exactPolicy) *pb.AdmitSessionCommandRequest {
	return &pb.AdmitSessionCommandRequest{Caller: coordination.Writer_WRITER_MEDIA, Target: coordination.Writer_WRITER_MEDIA, Credential: &pb.Credential{Proof: &pb.Credential_LegacyToken{LegacyToken: strings.Repeat("c", 43)}, CsrfToken: a.result.CSRFToken}, Command: wireCommand(s.command)}
}
func unknown(message proto.Message) {
	b := protowire.AppendTag(nil, 99, protowire.BytesType)
	b = protowire.AppendBytes(b, []byte("forged-principal"))
	message.ProtoReflect().SetUnknown(b)
}
func TestServerRejectsWrongCallerCredentialAndCommand(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*pb.AdmitSessionCommandRequest)
		code   codes.Code
	}{
		{"caller", func(r *pb.AdmitSessionCommandRequest) { r.Caller = coordination.Writer_WRITER_EXECUTION }, codes.PermissionDenied},
		{"target", func(r *pb.AdmitSessionCommandRequest) { r.Target = coordination.Writer_WRITER_EXECUTION }, codes.PermissionDenied},
		{"missing csrf", func(r *pb.AdmitSessionCommandRequest) { r.Credential.CsrfToken = "" }, codes.InvalidArgument},
		{"bad csrf encoding", func(r *pb.AdmitSessionCommandRequest) { r.Credential.CsrfToken = "bad" }, codes.InvalidArgument},
		{"wrong csrf", func(r *pb.AdmitSessionCommandRequest) { r.Credential.CsrfToken = strings.Repeat("A", 43) }, codes.PermissionDenied},
		{"missing proof", func(r *pb.AdmitSessionCommandRequest) { r.Credential.Proof = nil }, codes.InvalidArgument},
		{"refresh extension", func(r *pb.AdmitSessionCommandRequest) { unknown(r.Credential) }, codes.InvalidArgument},
		{"forged principal extension", func(r *pb.AdmitSessionCommandRequest) { unknown(r) }, codes.InvalidArgument},
		{"nested unknown", func(r *pb.AdmitSessionCommandRequest) { unknown(r.Command.Authorities[0]) }, codes.InvalidArgument},
		{"uuid", func(r *pb.AdmitSessionCommandRequest) { r.Command.Id = []byte{1} }, codes.InvalidArgument},
		{"enum", func(r *pb.AdmitSessionCommandRequest) { r.Command.Authorities[0].Kind = coordination.AuthorityKind(99) }, codes.InvalidArgument},
		{"missing account", func(r *pb.AdmitSessionCommandRequest) { r.Command.Authorities = r.Command.Authorities[1:] }, codes.PermissionDenied},
		{"wrong account", func(r *pb.AdmitSessionCommandRequest) { id := uuid.New(); r.Command.Authorities[0].Id = id[:] }, codes.PermissionDenied},
		{"wrong session", func(r *pb.AdmitSessionCommandRequest) { id := uuid.New(); r.Command.Authorities[1].Id = id[:] }, codes.PermissionDenied},
		{"extra identity", func(r *pb.AdmitSessionCommandRequest) {
			a := proto.Clone(r.Command.Authorities[0]).(*coordination.Authority)
			id := uuid.New()
			a.Id = id[:]
			r.Command.Authorities = append(r.Command.Authorities, a)
		}, codes.PermissionDenied},
		{"epoch", func(r *pb.AdmitSessionCommandRequest) { r.Command.Authorities[1].Epoch++ }, codes.AlreadyExists},
		{"action", func(r *pb.AdmitSessionCommandRequest) { r.Command.Action = "media.delete" }, codes.AlreadyExists},
		{"digest", func(r *pb.AdmitSessionCommandRequest) { r.Command.Sha256 = strings.Repeat("b", 64) }, codes.AlreadyExists},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, p, tokens := fixture(t)
			s, err := authority.NewServer(a, p, tokens, time.Now, insecure.NewCredentials())
			require.NoError(t, err)
			client := rawClient(t, serve(t, s))
			request := requestFor(a, p)
			tc.change(request)
			_, err = client.AdmitSessionCommand(privateContext(t, tokens[writefence.WriterMedia]), request)
			require.Equal(t, tc.code, status.Code(err))
			if tc.code != codes.AlreadyExists {
				require.Zero(t, p.calls)
			}
		})
	}
}
func TestServiceTokenDeadlineAndLimits(t *testing.T) {
	a, p, tokens := fixture(t)
	s, err := authority.NewServer(a, p, tokens, time.Now, insecure.NewCredentials())
	require.NoError(t, err)
	client := rawClient(t, serve(t, s))
	r := requestFor(a, p)
	for _, token := range []string{"", strings.Repeat("x", 40), tokens[writefence.WriterExecution]} {
		_, err := client.AdmitSessionCommand(privateContext(t, token), r)
		require.Contains(t, []codes.Code{codes.Unauthenticated, codes.PermissionDenied}, status.Code(err))
	}
	_, err = client.AdmitSessionCommand(metadata.NewOutgoingContext(t.Context(), metadata.Pairs("authorization", "Bearer "+tokens[writefence.WriterMedia])), r)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	_, err = client.AdmitSessionCommand(metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+tokens[writefence.WriterMedia])), r)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	ctx = metadata.AppendToOutgoingContext(privateContext(t, tokens[writefence.WriterMedia]), "authorization", "Bearer "+tokens[writefence.WriterMedia])
	_, err = client.AdmitSessionCommand(ctx, r)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	r.Command.Action = strings.Repeat("a", authority.MaxMessageBytes+1)
	_, err = client.AdmitSessionCommand(privateContext(t, tokens[writefence.WriterMedia]), r)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.Zero(t, p.calls)
}
func TestConstructorRejectsInvalidDedicatedTokens(t *testing.T) {
	for _, token := range []string{"", strings.Repeat("x", 31), strings.Repeat("x", 4097), strings.Repeat("x", 32) + " ", strings.Repeat("x", 32) + "\n", strings.Repeat("x", 32) + "é"} {
		a, p, tokens := fixture(t)
		tokens[writefence.WriterMedia] = token
		_, err := authority.NewServer(a, p, tokens, time.Now, insecure.NewCredentials())
		require.Error(t, err)
		_, err = authority.NewClient(writefence.WriterMedia, authority.Target{Address: "127.0.0.1:1234", Token: token, Transport: insecure.NewCredentials()})
		require.Error(t, err)
	}
	a, p, tokens := fixture(t)
	tokens[writefence.WriterMedia] = tokens[writefence.WriterCreation]
	_, err := authority.NewServer(a, p, tokens, time.Now, insecure.NewCredentials())
	require.Error(t, err)
	_, err = authority.NewServer(a, p, tokens, time.Now, nil)
	require.Error(t, err)
}

type responseServer struct {
	pb.UnimplementedAuthorityServer
	mutate  func(*pb.AdmitSessionCommandResponse)
	calls   atomic.Int32
	wait    bool
	entered chan struct{}
}

func (s *responseServer) AdmitSessionCommand(ctx context.Context, r *pb.AdmitSessionCommandRequest) (*pb.AdmitSessionCommandResponse, error) {
	s.calls.Add(1)
	if s.entered != nil {
		close(s.entered)
	}
	if s.wait {
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	a := r.Command.Authorities
	out := &pb.AdmitSessionCommandResponse{Identity: &pb.Identity{AccountId: a[0].Id, SessionId: a[1].Id, CredentialEpoch: 3}, Command: r.Command, Target: r.Target}
	if s.mutate != nil {
		s.mutate(out)
	}
	return out, nil
}
func maliciousClient(t *testing.T, f *responseServer, tokens map[writefence.Writer]string) *authority.Client {
	t.Helper()
	s := grpc.NewServer()
	pb.RegisterAuthorityServer(s, f)
	c, err := authority.NewClient(writefence.WriterMedia, authority.Target{Address: serve(t, s), Token: tokens[writefence.WriterMedia], Transport: insecure.NewCredentials()})
	require.NoError(t, err)
	return c
}
func TestClientRejectsIncompleteOrMismatchedAcknowledgement(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*pb.AdmitSessionCommandResponse)
	}{
		{"target", func(r *pb.AdmitSessionCommandResponse) { r.Target = coordination.Writer_WRITER_EXECUTION }},
		{"action", func(r *pb.AdmitSessionCommandResponse) { r.Command.Action = "other.action" }},
		{"digest", func(r *pb.AdmitSessionCommandResponse) { r.Command.Sha256 = strings.Repeat("b", 64) }},
		{"epoch", func(r *pb.AdmitSessionCommandResponse) { r.Command.Authorities[0].Epoch++ }},
		{"identity", func(r *pb.AdmitSessionCommandResponse) { id := uuid.New(); r.Identity.SessionId = id[:] }},
		{"unknown", func(r *pb.AdmitSessionCommandResponse) { unknown(r) }},
		{"nested unknown", func(r *pb.AdmitSessionCommandResponse) { unknown(r.Identity) }},
		{"missing", func(r *pb.AdmitSessionCommandResponse) { r.Identity = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, p, tokens := fixture(t)
			f := &responseServer{mutate: tc.mutate}
			c := maliciousClient(t, f, tokens)
			_, err := c.AdmitSessionCommand(t.Context(), authority.LegacyCredential(strings.Repeat("c", 43), a.result.CSRFToken), p.command, writefence.WriterMedia)
			require.ErrorIs(t, err, writefence.ErrUnavailable)
			require.EqualValues(t, 1, f.calls.Load())
		})
	}
}
func TestCancellationAndLostAcknowledgementAreSingleCalls(t *testing.T) {
	for _, cancelAfterSend := range []bool{false, true} {
		t.Run(map[bool]string{false: "lost acknowledgement", true: "explicit cancellation"}[cancelAfterSend], func(t *testing.T) {
			a, p, tokens := fixture(t)
			f := &responseServer{wait: true, entered: make(chan struct{})}
			c := maliciousClient(t, f, tokens)
			ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := c.AdmitSessionCommand(ctx, authority.LegacyCredential(strings.Repeat("c", 43), a.result.CSRFToken), p.command, writefence.WriterMedia)
				done <- err
			}()
			<-f.entered
			if cancelAfterSend {
				cancel()
			}
			err := <-done
			if cancelAfterSend {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			}
			require.EqualValues(t, 1, f.calls.Load())
		})
	}
}

type traceAuth struct {
	policyAuth
	seen chan metadata.MD
	span chan trace.SpanContext
}

func (a *traceAuth) CurrentWithActivity(ctx context.Context, token string, activity identity.SessionActivity) (identity.AuthResult, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	a.seen <- md.Copy()
	a.span <- trace.SpanContextFromContext(ctx)
	return a.policyAuth.CurrentWithActivity(ctx, token, activity)
}
func TestW3CTraceIsolation(t *testing.T) {
	a, p, tokens := fixture(t)
	ta := &traceAuth{policyAuth: *a, seen: make(chan metadata.MD, 1), span: make(chan trace.SpanContext, 1)}
	s, err := authority.NewServer(ta, p, tokens, time.Now, insecure.NewCredentials())
	require.NoError(t, err)
	c, err := authority.NewClient(writefence.WriterMedia, authority.Target{Address: serve(t, s), Token: tokens[writefence.WriterMedia], Transport: insecure.NewCredentials()})
	require.NoError(t, err)
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled})
	ctx := trace.ContextWithSpanContext(t.Context(), sc)
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer user-secret", "cookie", "session=secret", "x-user-id", "forged", "baggage", "private=value", "traceparent", "forged"))
	_, err = c.Verify(ctx, authority.LegacyCredential(strings.Repeat("c", 43), a.result.CSRFToken))
	require.NoError(t, err)
	md := <-ta.seen
	require.Len(t, md, 1)
	require.Len(t, md.Get("traceparent"), 1)
	got := <-ta.span
	require.Equal(t, sc.TraceID(), got.TraceID())
	require.True(t, got.IsRemote())
}
