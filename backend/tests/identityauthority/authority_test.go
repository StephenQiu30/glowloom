package identityauthority_test

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	authority "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcauthority"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// exactPolicy is a test-only policy transaction contract. It rechecks the exact
// authenticated actor and never opens a missing authority. PostgreSQL acceptance
// is exercised separately with the real SessionAdmissionStore.
type exactPolicy struct {
	mu         sync.Mutex
	actor      identity.Principal
	command    writefence.Command
	calls      int
	reads      int
	registered bool
	open       bool
}

func (p *exactPolicy) RegisterSessionCommand(ctx context.Context, actor identity.Principal, command writefence.Command, target writefence.Writer, clock func() time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if err := ctx.Err(); err != nil {
		return err
	}
	if actor != p.actor || target != writefence.WriterMedia || clock == nil {
		return identity.ErrForbidden
	}
	if !p.open {
		return writefence.ErrFenced
	}
	a, err := command.AuthorityDigest()
	if err != nil {
		return err
	}
	b, _ := p.command.AuthorityDigest()
	if command.ID != p.command.ID || command.SHA256 != p.command.SHA256 || command.Action != p.command.Action || a != b {
		return writefence.ErrConflict
	}
	p.registered = true
	return nil
}

type policyAuth struct {
	result identity.AuthResult
	err    error
}

func (a *policyAuth) CurrentWithActivity(_ context.Context, _ string, activity identity.SessionActivity) (identity.AuthResult, error) {
	if activity != identity.NoActivity {
		panic("RPC must not mutate idle activity")
	}
	return a.result, a.err
}
func (a *policyAuth) CurrentAccess(ctx context.Context, _ string, csrf string, activity identity.SessionActivity) (identity.AuthResult, error) {
	if csrf != a.result.CSRFToken {
		return identity.AuthResult{}, identity.ErrSessionInvalid
	}
	return a.CurrentWithActivity(ctx, "", activity)
}
func (a *policyAuth) ReadAccessIdentity(context.Context, string) (identity.Principal, error) {
	if a.err != nil {
		return identity.Principal{}, a.err
	}
	return identity.Principal{ID: a.result.User.ID, SessionID: a.result.Session.ID, CredentialEpoch: a.result.User.SessionEpoch}, nil
}
func fixture(t *testing.T) (*policyAuth, *exactPolicy, map[writefence.Writer]string) {
	t.Helper()
	actor := identity.Principal{ID: uuid.New(), SessionID: uuid.New(), CredentialEpoch: 3}
	auth := &policyAuth{result: identity.AuthResult{User: domain.User{ID: actor.ID, SessionEpoch: 3}, Session: domain.AuthSession{ID: actor.SessionID, AccountID: actor.ID, CredentialRevision: 3}, CSRFToken: strings.Repeat("c", 43)}}
	command := writefence.Command{ID: uuid.New(), SHA256: strings.Repeat("a", 64), Action: "media.create", Authorities: []writefence.Authority{{Issuer: writefence.Identity, Kind: writefence.Account, ID: actor.ID, Epoch: 2}, {Issuer: writefence.Identity, Kind: writefence.Session, ID: actor.SessionID, Epoch: 4}, {Issuer: writefence.Creation, Kind: writefence.Scope, ID: uuid.New(), Epoch: 7}}}
	tokens := map[writefence.Writer]string{}
	for _, w := range writefence.Writers() {
		tokens[w] = strings.Repeat(string(w)+"-", 10)
	}
	return auth, &exactPolicy{actor: actor, command: command, open: true}, tokens
}
func serve(t *testing.T, s *grpc.Server) string {
	t.Helper()
	l, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan struct{})
	go func() { defer close(done); _ = s.Serve(l) }()
	t.Cleanup(func() { s.Stop(); _ = l.Close(); <-done })
	return l.Addr().String()
}
func TestVerifyIsNotAdmissionAndExactActorIsRechecked(t *testing.T) {
	auth, store, tokens := fixture(t)
	s, err := authority.NewServer(auth, store, tokens, time.Now, insecure.NewCredentials())
	require.NoError(t, err)
	c, err := authority.NewClient(writefence.WriterMedia, authority.Target{Address: serve(t, s), Token: tokens[writefence.WriterMedia], Transport: insecure.NewCredentials()})
	require.NoError(t, err)
	cred := authority.LegacyCredential(strings.Repeat("c", 43), auth.result.CSRFToken)
	got, err := c.Verify(t.Context(), cred)
	require.NoError(t, err)
	require.Equal(t, store.actor.ID, got.AccountID)
	require.Equal(t, 0, store.calls)
	got, err = c.AdmitSessionCommand(t.Context(), cred, store.command, writefence.WriterMedia)
	require.NoError(t, err)
	require.Equal(t, store.actor.SessionID, got.SessionID)
	require.Equal(t, 1, store.calls)
	store.open = false
	_, err = c.Verify(t.Context(), cred)
	require.NoError(t, err)
	_, err = c.AdmitSessionCommand(t.Context(), cred, store.command, writefence.WriterMedia)
	require.ErrorIs(t, err, writefence.ErrFenced)
}
