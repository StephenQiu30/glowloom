package identityauthority_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	authority "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcauthority"
	"github.com/StephenQiu30/glowloom-server/backend/identity/adapter/sessiontoken"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// authModel is a read-only test transaction contract for the real Auth policy.
// Unexpected writes panic through the deliberately unimplemented EmailTx.
type authModel struct {
	identity.EmailTx
	mu       sync.Mutex
	user     domain.User
	sessions map[uuid.UUID]domain.AuthSession
}

func (m *authModel) EmailTransaction(ctx context.Context, run func(identity.EmailTx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return run(m)
}
func (*authModel) RegistrationEnabled() (bool, error) { return true, nil }
func (m *authModel) AccountByID(id uuid.UUID) (domain.User, error) {
	if id != m.user.ID {
		return domain.User{}, domain.ErrUserNotFound
	}
	return m.user, nil
}
func (m *authModel) SessionByID(id uuid.UUID, _ bool) (domain.AuthSession, error) {
	s, ok := m.sessions[id]
	if !ok {
		return s, identity.ErrSessionInvalid
	}
	return s, nil
}
func (m *authModel) SessionByHash(digest []byte, _ bool) (domain.AuthSession, error) {
	for _, s := range m.sessions {
		if bytes.Equal(s.TokenHash, digest) {
			return s, nil
		}
	}
	return domain.AuthSession{}, identity.ErrSessionInvalid
}

type proofProtector struct{ identity.ProofProtector }

func (proofProtector) Digest(purpose string, id uuid.UUID, value string) []byte {
	h := hmac.New(sha256.New, []byte("authority-test-only-key"))
	_, _ = h.Write([]byte(purpose))
	_, _ = h.Write(id[:])
	_, _ = h.Write([]byte(value))
	return h.Sum(nil)
}

type noRevocationHint struct{ identity.RefreshCoordinator }

func (noRevocationHint) IsRevoked(context.Context, uuid.UUID) (bool, error) { return false, nil }

type actualAuth struct {
	auth                                   *identity.Auth
	model                                  *authModel
	legacy, access, csrfLegacy, csrfAccess string
	now                                    time.Time
	legacySession, accessSession           uuid.UUID
	codec                                  identity.AccessTokenCodec
}

func authenticationFixture(t *testing.T) *actualAuth {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	user := domain.User{ID: uuid.New(), Status: domain.StatusActive, SessionEpoch: 3, EmailVerifiedAt: now.Add(-time.Hour)}
	m := &authModel{user: user, sessions: map[uuid.UUID]domain.AuthSession{}}
	legacy := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	raw, _ := base64.RawURLEncoding.DecodeString(legacy)
	digest := sha256.Sum256(raw)
	legacySession := domain.AuthSession{ID: uuid.New(), AccountID: user.ID, TokenHash: digest[:], CredentialRevision: 3, CreatedAt: now.Add(-time.Hour), LastActiveAt: now.Add(-time.Minute), AbsoluteExpiresAt: now.Add(time.Hour), ProtocolVersion: 1}
	csrfLegacy := base64.RawURLEncoding.EncodeToString(proofProtector{}.Digest("session_csrf", legacySession.ID, legacy))
	h := sha256.Sum256([]byte(csrfLegacy))
	legacySession.CSRFHash = append([]byte(nil), h[:]...)
	m.sessions[legacySession.ID] = legacySession
	csrfAccess := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	h = sha256.Sum256([]byte(csrfAccess))
	accessSession := legacySession
	accessSession.ID = uuid.New()
	accessSession.ProtocolVersion = 2
	accessSession.IdleTimeoutSeconds = 43200
	accessSession.CSRFHash = h[:]
	accessSession.TokenHash = bytes.Repeat([]byte{3}, 32)
	m.sessions[accessSession.ID] = accessSession
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	private := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	public := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey)})
	codec, err := sessiontoken.New("authority-test", private, map[string][]byte{"authority-test": public})
	require.NoError(t, err)
	access, err := codec.Sign(identity.AccessClaims{UserID: user.ID, SessionID: accessSession.ID, TokenID: uuid.New(), CredentialEpoch: 3, IssuedAt: now, NotBefore: now, ExpiresAt: now.Add(5 * time.Minute)})
	require.NoError(t, err)
	a := identity.NewEmailAuth(m, proofProtector{}, identity.EmailPolicy{}, func() time.Time { return now }).WithSessionSecurity(codec, nil, noRevocationHint{})
	return &actualAuth{auth: a, model: m, legacy: legacy, access: access, csrfLegacy: csrfLegacy, csrfAccess: csrfAccess, now: now, legacySession: legacySession.ID, accessSession: accessSession.ID, codec: codec}
}
func TestRealAuthRejectsForgedExpiredRevokedAndWrongEpochCredentials(t *testing.T) {
	for _, kind := range []string{"legacy", "access"} {
		for _, failure := range []string{"valid", "wrong token", "wrong csrf", "expired", "revoked", "wrong epoch", "disabled", "wrong session owner", "legacy window closed"} {
			if kind == "access" && failure == "legacy window closed" {
				continue
			}
			t.Run(kind+"/"+failure, func(t *testing.T) {
				f := authenticationFixture(t)
				_, p, tokens := fixture(t)
				sid := f.legacySession
				token, csrf := f.legacy, f.csrfLegacy
				if kind == "access" {
					sid = f.accessSession
					token, csrf = f.access, f.csrfAccess
				}
				session := f.model.sessions[sid]
				switch failure {
				case "wrong token":
					if kind == "access" {
						token = "forged.access.signature"
					} else {
						token = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32))
					}
				case "wrong csrf":
					csrf = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32))
				case "expired":
					session.AbsoluteExpiresAt = f.now
				case "revoked":
					session.RevokedAt = &f.now
				case "wrong epoch":
					f.model.user.SessionEpoch++
				case "disabled":
					f.model.user.Status = domain.StatusDisabled
				case "wrong session owner":
					session.AccountID = uuid.New()
				case "legacy window closed":
					f.auth.WithLegacyUpgradeUntil(f.now)
				}
				f.model.sessions[sid] = session
				s, err := authority.NewServer(f.auth, p, tokens, time.Now, insecure.NewCredentials())
				require.NoError(t, err)
				c, err := authority.NewClient(writefence.WriterMedia, authority.Target{Address: serve(t, s), Token: tokens[writefence.WriterMedia], Transport: insecure.NewCredentials()})
				require.NoError(t, err)
				credential := authority.LegacyCredential(token, csrf)
				if kind == "access" {
					credential = authority.AccessCredential(token, csrf)
				}
				got, err := c.Verify(t.Context(), credential)
				if failure == "valid" {
					require.NoError(t, err)
					require.Equal(t, f.model.user.ID, got.AccountID)
					require.Equal(t, sid, got.SessionID)
					require.EqualValues(t, 3, got.CredentialEpoch)
					require.Equal(t, session.LastActiveAt, f.model.sessions[sid].LastActiveAt)
				} else {
					require.Error(t, err)
				}
				require.Zero(t, p.calls)
			})
		}
	}
}
func TestRealAccessExpiryBoundaryAndNoRefreshFallback(t *testing.T) {
	f := authenticationFixture(t)
	_, p, tokens := fixture(t)
	f.access = mustSign(t, f.codec, identity.AccessClaims{UserID: f.model.user.ID, SessionID: f.accessSession, TokenID: uuid.New(), CredentialEpoch: 3, IssuedAt: f.now.Add(-time.Minute), NotBefore: f.now.Add(-time.Minute), ExpiresAt: f.now})
	s, err := authority.NewServer(f.auth, p, tokens, time.Now, insecure.NewCredentials())
	require.NoError(t, err)
	c, err := authority.NewClient(writefence.WriterMedia, authority.Target{Address: serve(t, s), Token: tokens[writefence.WriterMedia], Transport: insecure.NewCredentials()})
	require.NoError(t, err)
	_, err = c.Verify(t.Context(), authority.AccessCredential(f.access, f.csrfAccess))
	require.ErrorIs(t, err, identity.ErrAccessExpired)
	require.NotErrorIs(t, err, identity.ErrSessionInvalid)
	// A refresh-like opaque value is never tried as legacy after access failure.
	_, err = c.Verify(t.Context(), authority.AccessCredential(f.legacy, f.csrfLegacy))
	require.ErrorIs(t, err, identity.ErrSessionInvalid)
	require.Zero(t, p.calls)
}
func TestRealAuthVerifyAllowsOptionalCSRFButAdmissionRequiresProof(t *testing.T) {
	for _, kind := range []string{"legacy", "access"} {
		t.Run(kind, func(t *testing.T) {
			f := authenticationFixture(t)
			a, policy, tokens := fixture(t)
			s, err := authority.NewServer(f.auth, policy, tokens, time.Now, insecure.NewCredentials())
			require.NoError(t, err)
			address := serve(t, s)
			client, err := authority.NewClient(writefence.WriterMedia, authority.Target{Address: address, Token: tokens[writefence.WriterMedia], Transport: insecure.NewCredentials()})
			require.NoError(t, err)
			credential := authority.LegacyCredential(f.legacy, "")
			wire := &pb.Credential{Proof: &pb.Credential_LegacyToken{LegacyToken: f.legacy}}
			wrong := authority.LegacyCredential(f.legacy, base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32)))
			malformed := authority.LegacyCredential(f.legacy, "invalid")
			session := f.legacySession
			if kind == "access" {
				credential = authority.AccessCredential(f.access, "")
				wire.Proof = &pb.Credential_AccessToken{AccessToken: f.access}
				wrong = authority.AccessCredential(f.access, base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32)))
				malformed = authority.AccessCredential(f.access, "invalid")
				session = f.accessSession
			}
			before := f.model.sessions[session].LastActiveAt
			got, err := client.Verify(t.Context(), credential)
			require.NoError(t, err)
			require.Equal(t, f.model.user.ID, got.AccountID)
			require.Equal(t, session, got.SessionID)
			require.Equal(t, before, f.model.sessions[session].LastActiveAt)
			_, err = client.Verify(t.Context(), wrong)
			require.Error(t, err)
			_, err = client.Verify(t.Context(), malformed)
			require.ErrorIs(t, err, writefence.ErrInvalid)
			_, err = client.AdmitSessionCommand(t.Context(), credential, policy.command, writefence.WriterMedia)
			require.ErrorIs(t, err, writefence.ErrInvalid)
			request := requestFor(a, policy)
			request.Credential = wire
			_, err = rawClient(t, address).AdmitSessionCommand(privateContext(t, tokens[writefence.WriterMedia]), request)
			require.Equal(t, codes.InvalidArgument, status.Code(err))
			require.Zero(t, policy.calls, "read verification must never grant command admission")
		})
	}
}

func TestReadAccessIdentityRemainsMinimalStrictAndCurrent(t *testing.T) {
	for _, failure := range []string{"account revoked", "session revoked", "credential epoch", "expired"} {
		t.Run(failure, func(t *testing.T) {
			f := authenticationFixture(t)
			f.model.user.PlatformRole = "platform_admin"
			f.model.user.PlatformEpoch = 19
			f.model.user.OrgID = uuid.New()
			f.model.user.LoginName = "private-login"
			f.model.user.DisplayName = "private-profile"
			before := f.model.sessions[f.accessSession].LastActiveAt
			_, err := f.auth.CurrentAccess(t.Context(), f.access, "", identity.NoActivity)
			require.ErrorIs(t, err, identity.ErrSessionInvalid, "original CurrentAccess must remain strict")
			got, err := f.auth.ReadAccessIdentity(t.Context(), f.access)
			require.NoError(t, err)
			require.Equal(t, identity.Principal{ID: f.model.user.ID, SessionID: f.accessSession, CredentialEpoch: 3}, got, "every field outside the three identity references must remain zero")
			require.Equal(t, before, f.model.sessions[f.accessSession].LastActiveAt)
			session := f.model.sessions[f.accessSession]
			switch failure {
			case "account revoked":
				f.model.user.Status = domain.StatusDisabled
			case "session revoked":
				session.RevokedAt = &f.now
			case "credential epoch":
				f.model.user.SessionEpoch++
			case "expired":
				session.AbsoluteExpiresAt = f.now
			}
			f.model.sessions[f.accessSession] = session
			got, err = f.auth.ReadAccessIdentity(t.Context(), f.access)
			require.Error(t, err)
			require.Equal(t, identity.Principal{}, got)
			require.Equal(t, before, f.model.sessions[f.accessSession].LastActiveAt)
		})
	}
}

func mustSign(t *testing.T, codec identity.AccessTokenCodec, claims identity.AccessClaims) string {
	t.Helper()
	raw, err := codec.Sign(claims)
	require.NoError(t, err)
	return raw
}
