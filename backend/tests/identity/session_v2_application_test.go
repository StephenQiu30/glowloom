package identity_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	tokenadapter "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/sessiontoken"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// Serial rollback model tests application decisions, not PostgreSQL concurrency.
type v2Model struct {
	identity.EmailTx
	mu                  sync.Mutex
	user                domain.User
	sessions            map[uuid.UUID]domain.AuthSession
	refresh             map[string]domain.RefreshToken
	guards              map[string]domain.LoginGuard
	failAudit, mailFail bool
	onRefreshLock       func()
	onGenerationLock    func(int64)
}

func (m *v2Model) EmailTransaction(_ context.Context, fn func(identity.EmailTx) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u := m.user
	ss := map[uuid.UUID]domain.AuthSession{}
	rr := map[string]domain.RefreshToken{}
	for k, v := range m.sessions {
		ss[k] = v
	}
	for k, v := range m.refresh {
		rr[k] = v
	}
	err := fn(m)
	if err != nil {
		m.user = u
		m.sessions = ss
		m.refresh = rr
	}
	return err
}
func (*v2Model) RegistrationEnabled() (bool, error) { return true, nil }
func (m *v2Model) AccountByID(id uuid.UUID) (domain.User, error) {
	if id != m.user.ID {
		return domain.User{}, domain.ErrUserNotFound
	}
	return m.user, nil
}
func (m *v2Model) AccountByEmail(s string) (domain.User, error) {
	if s != m.user.Email {
		return domain.User{}, domain.ErrUserNotFound
	}
	return m.user, nil
}
func (m *v2Model) UpdateAccount(u domain.User) error             { m.user = u; return nil }
func (m *v2Model) Guard(k string) (domain.LoginGuard, error)     { return m.guards[k], nil }
func (m *v2Model) SaveGuard(k string, g domain.LoginGuard) error { m.guards[k] = g; return nil }
func (*v2Model) Budget(string) (domain.EmailBudget, error)       { return domain.EmailBudget{}, nil }
func (*v2Model) SaveBudget(string, domain.EmailBudget) error     { return nil }
func (m *v2Model) InsertSession(s domain.AuthSession) error      { m.sessions[s.ID] = s; return nil }
func (m *v2Model) SetSessionSecurity(id uuid.UUID, h []byte) error {
	s := m.sessions[id]
	s.CSRFHash = h
	m.sessions[id] = s
	return nil
}
func (m *v2Model) SessionByID(id uuid.UUID, _ bool) (domain.AuthSession, error) {
	s, ok := m.sessions[id]
	if !ok {
		return s, identity.ErrSessionInvalid
	}
	return s, nil
}
func (m *v2Model) FindRefresh(d []byte, lock bool) (domain.RefreshToken, error) {
	if lock && m.onRefreshLock != nil {
		m.onRefreshLock()
	}
	r, ok := m.refresh[string(d)]
	if !ok {
		return r, identity.ErrSessionInvalid
	}
	return r, nil
}
func (m *v2Model) RefreshByGeneration(id uuid.UUID, g int64, lock bool) (domain.RefreshToken, error) {
	if lock && m.onGenerationLock != nil {
		m.onGenerationLock(g)
	}
	for _, r := range m.refresh {
		if r.SessionID == id && r.Generation == g {
			return r, nil
		}
	}
	return domain.RefreshToken{}, identity.ErrSessionInvalid
}
func (m *v2Model) InsertRefresh(r domain.RefreshToken) error {
	m.refresh[string(r.Digest)] = r
	return nil
}
func (m *v2Model) RotateRefresh(old, next domain.RefreshToken, op uuid.UUID, until time.Time, b []byte) error {
	if old.ConsumedAt != nil {
		return identity.ErrSessionInvalid
	}
	old.ConsumedAt = &next.IssuedAt
	old.RotationRequestID = op
	old.ReplayUntil = &until
	old.ReceiptCiphertext = b
	m.refresh[string(old.Digest)] = old
	m.refresh[string(next.Digest)] = next
	s := m.sessions[old.SessionID]
	s.RefreshGeneration = next.Generation
	m.sessions[s.ID] = s
	return nil
}
func (m *v2Model) RevokeRefreshChain(id uuid.UUID, n time.Time) error {
	for k, r := range m.refresh {
		if r.SessionID == id {
			r.RevokedAt = &n
			r.ReceiptCiphertext = nil
			r.ReplayUntil = nil
			m.refresh[k] = r
		}
	}
	return nil
}
func (m *v2Model) RevokeSession(u, id uuid.UUID, n time.Time) error {
	s := m.sessions[id]
	if s.AccountID != u {
		return identity.ErrSessionInvalid
	}
	s.RevokedAt = &n
	m.sessions[id] = s
	return m.RevokeRefreshChain(id, n)
}
func (m *v2Model) RevokeAccountSessions(u uuid.UUID, n time.Time) error {
	for id, s := range m.sessions {
		if s.AccountID == u {
			if e := m.RevokeSession(u, id, n); e != nil {
				return e
			}
		}
	}
	return nil
}
func (m *v2Model) TouchSession(id uuid.UUID, n time.Time) error {
	s := m.sessions[id]
	s.LastActiveAt = n
	m.sessions[id] = s
	return nil
}
func (m *v2Model) ReauthenticateSession(id uuid.UUID, e int64, n time.Time) error {
	s := m.sessions[id]
	s.ReauthAt = &n
	s.ReauthEpoch = &e
	m.sessions[id] = s
	return nil
}
func (m *v2Model) AppendAuthEvent(uuid.UUID, string, time.Time) error {
	if m.failAudit {
		return errors.New("audit unavailable")
	}
	return nil
}
func (m *v2Model) AppendSessionSecurityEvent(uuid.UUID, uuid.UUID, uuid.UUID, string, string, time.Time) error {
	return m.AppendAuthEvent(uuid.Nil, "", time.Time{})
}
func (m *v2Model) EnqueueMail(identity.QueuedMail) error {
	if m.mailFail {
		return errors.New("mail unavailable")
	}
	return nil
}

type v2Proof struct{}

func (v2Proof) KeyID() string { return "fixture" }
func (v2Proof) Digest(p string, id uuid.UUID, s string) []byte {
	h := sha256.Sum256([]byte(p + id.String() + s))
	return h[:]
}
func (v2Proof) Seal(_ uuid.UUID, b []byte) ([]byte, error) { return b, nil }

type v2Codec struct {
	claims map[string]identity.AccessClaims
	fail   bool
}

func (*v2Codec) CanSign() bool { return true }

func (c *v2Codec) Sign(a identity.AccessClaims) (string, error) {
	if c.fail {
		return "", errors.New("sign failed")
	}
	s := uuid.NewString()
	c.claims[s] = a
	return s, nil
}
func (c *v2Codec) Verify(s string, _ time.Time) (identity.AccessClaims, error) {
	a, ok := c.claims[s]
	if !ok {
		return a, identity.ErrAccessInvalid
	}
	return a, nil
}

type v2Receipt struct{ fail bool }

func (r *v2Receipt) Seal(_ uuid.UUID, _ int64, _ uuid.UUID, b []byte) ([]byte, error) {
	if r.fail {
		return nil, errors.New("seal failed")
	}
	return b, nil
}
func (*v2Receipt) Open(_ uuid.UUID, _ int64, _ uuid.UUID, b []byte) ([]byte, error) { return b, nil }

type v2Coordinator struct {
	down    bool
	receipt []byte
}

func (c *v2Coordinator) Admit(context.Context, uuid.UUID, string) (func(context.Context) error, error) {
	if c.down {
		return nil, identity.ErrRefreshDependency
	}
	return func(context.Context) error { return nil }, nil
}
func (c *v2Coordinator) ReadReceipt(context.Context, uuid.UUID, int64) ([]byte, error) {
	return c.receipt, nil
}
func (*v2Coordinator) WriteReceipt(context.Context, uuid.UUID, int64, []byte, time.Time) error {
	return errors.New("cache unavailable")
}
func (*v2Coordinator) MarkRevoked(context.Context, uuid.UUID, time.Time) error {
	return errors.New("hint unavailable")
}
func (c *v2Coordinator) IsRevoked(context.Context, uuid.UUID) (bool, error) {
	if c.down {
		return false, identity.ErrRefreshDependency
	}
	return false, nil
}
func TestSessionV2Application(t *testing.T) {
	now := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	h, err := bcrypt.GenerateFromPassword([]byte("short"), 12)
	if err != nil {
		t.Fatal(err)
	}
	setup := func() (*identity.Auth, *v2Model, *v2Codec, *v2Receipt, *v2Coordinator) {
		m := &v2Model{user: domain.User{ID: uuid.New(), Email: "fixture@example.invalid", EmailVerifiedAt: now, Status: domain.StatusActive, PasswordHash: string(h), SessionEpoch: 1, Revision: 1, MustChangePassword: true}, sessions: map[uuid.UUID]domain.AuthSession{}, refresh: map[string]domain.RefreshToken{}, guards: map[string]domain.LoginGuard{}}
		c := &v2Codec{claims: map[string]identity.AccessClaims{}}
		r := &v2Receipt{}
		q := &v2Coordinator{}
		a := identity.NewEmailAuth(m, v2Proof{}, identity.EmailPolicy{TermsVersion: "test"}, func() time.Time { return now }).WithSessionSecurity(c, r, q)
		return a, m, c, r, q
	}
	t.Run("short password and recovery", func(t *testing.T) {
		a, m, _, _, q := setup()
		res, e := a.Login(t.Context(), m.user.Email, "short", true)
		if e != nil {
			t.Fatal(e)
		}
		if res.Session.ProtocolVersion != 2 || res.AccessToken == "" || res.RefreshToken == "" || res.Session.AbsoluteExpiresAt.Sub(now) != 720*time.Hour {
			t.Fatal("missing issuance")
		}
		q.down = true
		if _, e = a.CurrentAccess(t.Context(), res.AccessToken, res.CSRFToken, identity.NoActivity); e != nil {
			t.Fatal(e)
		}
		if _, e = a.CurrentAccess(t.Context(), res.AccessToken, "forged", identity.NoActivity); !errors.Is(e, identity.ErrSessionInvalid) {
			t.Fatal(e)
		}
		q.down = false
		child, e := a.Refresh(t.Context(), res.RefreshToken, res.CSRFToken, "peer", uuid.New())
		if e != nil {
			t.Fatal(e)
		}
		for _, token := range []string{res.RefreshToken, child.RefreshToken} {
			again, e := a.Refresh(t.Context(), token, res.CSRFToken, "peer", uuid.New())
			if e != nil || again.RefreshToken != child.RefreshToken || again.AccessToken != child.AccessToken {
				t.Fatal("recovery differs", e)
			}
		}
		if child.CSRFToken != res.CSRFToken || child.Session.LastActiveAt != res.Session.LastActiveAt || child.Session.ReauthAt != nil || child.Session.AbsoluteExpiresAt != res.Session.AbsoluteExpiresAt {
			t.Fatal("refresh extended activity")
		}
		now = now.Add(11 * time.Second)
		if _, e = a.Refresh(t.Context(), res.RefreshToken, res.CSRFToken, "peer", uuid.New()); !errors.Is(e, identity.ErrSessionInvalid) {
			t.Fatal(e)
		}
		if m.sessions[res.Session.ID].RevokedAt == nil {
			t.Fatal("revocation rolled back")
		}
	})
	for _, failure := range []string{"sign", "audit"} {
		t.Run(failure+" rollback", func(t *testing.T) {
			a, m, c, r, _ := setup()
			c.fail = failure == "sign"
			r.fail = failure == "seal"
			m.failAudit = failure == "audit"
			_, e := a.Login(t.Context(), m.user.Email, "short", false)
			if e == nil || len(m.sessions) != 0 || len(m.refresh) != 0 {
				t.Fatal("issuance committed", e)
			}
		})
	}
	t.Run("rotation failure rolls back", func(t *testing.T) {
		for _, failure := range []string{"sign", "seal", "audit"} {
			t.Run(failure, func(t *testing.T) {
				a, m, c, r, _ := setup()
				res, e := a.Login(t.Context(), m.user.Email, "short", false)
				if e != nil {
					t.Fatal(e)
				}
				c.fail = failure == "sign"
				r.fail = failure == "seal"
				m.failAudit = failure == "audit"
				_, e = a.Refresh(t.Context(), res.RefreshToken, res.CSRFToken, "peer", uuid.New())
				if e == nil || m.sessions[res.Session.ID].RefreshGeneration != 0 || len(m.refresh) != 1 {
					t.Fatal("failed rotation committed", e)
				}
			})
		}
	})
	t.Run("epoch expiry signature and random digest", func(t *testing.T) {
		a, m, c, _, _ := setup()
		res, e := a.Login(t.Context(), m.user.Email, "short", false)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = a.CurrentAccess(t.Context(), "tampered", res.CSRFToken, identity.NoActivity); !errors.Is(e, identity.ErrAccessInvalid) {
			t.Fatal(e)
		}
		m.user.SessionEpoch++
		if _, e = a.CurrentAccess(t.Context(), res.AccessToken, res.CSRFToken, identity.NoActivity); !errors.Is(e, identity.ErrSessionInvalid) {
			t.Fatal("epoch accepted", e)
		}
		m.user.SessionEpoch--
		claims := c.claims[res.AccessToken]
		claims.ExpiresAt = now
		c.claims[res.AccessToken] = claims
		if _, e = a.CurrentAccess(t.Context(), res.AccessToken, res.CSRFToken, identity.NoActivity); !errors.Is(e, identity.ErrAccessExpired) {
			t.Fatal("exact expiry must classify access expiry", e)
		}
		random, _, _ := randomV2Token()
		if _, e = a.Refresh(t.Context(), random, res.CSRFToken, "peer", uuid.New()); !errors.Is(e, identity.ErrSessionInvalid) {
			t.Fatal(e)
		}
		if m.sessions[res.Session.ID].RevokedAt != nil {
			t.Fatal("random digest revoked session")
		}
	})
	t.Run("higher generation recovery cannot emit old cookies", func(t *testing.T) {
		a, m, _, _, _ := setup()
		res, e := a.Login(t.Context(), m.user.Email, "short", false)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = a.Refresh(t.Context(), res.RefreshToken, res.CSRFToken, "peer", uuid.New()); e != nil {
			t.Fatal(e)
		}
		s := m.sessions[res.Session.ID]
		s.RefreshGeneration = 2
		m.sessions[s.ID] = s
		got, e := a.Refresh(t.Context(), res.RefreshToken, res.CSRFToken, "peer", uuid.New())
		if !errors.Is(e, identity.ErrRefreshRaced) || got.AccessToken != "" || got.RefreshToken != "" {
			t.Fatal("old credentials emitted", e)
		}
	})
	t.Run("password proof guard and notification rollback", func(t *testing.T) {
		a, m, _, _, _ := setup()
		res, e := a.Login(t.Context(), m.user.Email, "short", false)
		if e != nil {
			t.Fatal(e)
		}
		actor := identity.Principal{ID: m.user.ID, SessionID: res.Session.ID, CredentialEpoch: m.user.SessionEpoch}
		if e = a.ReauthenticateCurrent(t.Context(), actor, "wrong"); !errors.Is(e, identity.ErrInvalidCredentials) {
			t.Fatal(e)
		}
		failures := 0
		for _, g := range m.guards {
			failures += len(g.Failures)
		}
		if failures != 1 {
			t.Fatal("password rejection rolled back guard")
		}
		if e = a.ReauthenticateCurrent(t.Context(), actor, "short"); e != nil {
			t.Fatal(e)
		}
		proof := m.sessions[res.Session.ID].ReauthAt
		if _, e = a.Refresh(t.Context(), res.RefreshToken, res.CSRFToken, "peer", uuid.New()); e != nil {
			t.Fatal(e)
		}
		if m.sessions[res.Session.ID].ReauthAt != proof {
			t.Fatal("refresh extended password proof")
		}
		m.mailFail = true
		_, e = a.ChangePasswordCurrent(t.Context(), actor, 1, "short", "NewStrongPassword789!", "NewStrongPassword789!")
		if e == nil || m.user.SessionEpoch != 1 || len(m.sessions) != 1 || m.sessions[res.Session.ID].RevokedAt != nil {
			t.Fatal("notification failure committed password rotation", e)
		}
		m.mailFail = false
		changed, e := a.ChangePasswordCurrent(t.Context(), actor, 1, "short", "NewStrongPassword789!", "NewStrongPassword789!")
		if e != nil || changed.Session.ProtocolVersion != 2 || m.user.SessionEpoch != 2 || m.sessions[res.Session.ID].RevokedAt == nil {
			t.Fatal("password rotation failed", e)
		}
	})
	t.Run("rollback retains verification and refresh", func(t *testing.T) {
		a, m, _, _, _ := setup()
		res, e := a.Login(t.Context(), m.user.Email, "short", false)
		if e != nil {
			t.Fatal(e)
		}
		a.WithSessionIssuance(false)
		if _, e = a.CurrentAccess(t.Context(), res.AccessToken, res.CSRFToken, identity.NoActivity); e != nil {
			t.Fatal(e)
		}
		if _, e = a.Refresh(t.Context(), res.RefreshToken, res.CSRFToken, "peer", uuid.New()); e != nil {
			t.Fatal(e)
		}
		if e = a.LogoutCurrentWithRefresh(t.Context(), res.RefreshToken, res.CSRFToken, res.Session.ID); e != nil || m.sessions[res.Session.ID].RevokedAt == nil {
			t.Fatal(e)
		}
	})

	t.Run("concurrent same old returns one committed child", func(t *testing.T) {
		a, m, _, _, _ := setup()
		res, e := a.Login(t.Context(), m.user.Email, "short", false)
		if e != nil {
			t.Fatal(e)
		}
		type outcome struct {
			r identity.AuthResult
			e error
		}
		results := make(chan outcome, 4)
		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() {
				r, e := a.Refresh(t.Context(), res.RefreshToken, res.CSRFToken, "peer", uuid.New())
				results <- outcome{r, e}
			})
		}
		wg.Wait()
		close(results)
		token := ""
		for got := range results {
			if got.e != nil {
				t.Fatal(got.e)
			}
			if token == "" {
				token = got.r.RefreshToken
			}
			if got.r.RefreshToken != token {
				t.Fatal("multiple successors")
			}
		}
		if len(m.refresh) != 2 || m.sessions[res.Session.ID].RefreshGeneration != 1 {
			t.Fatal("concurrent attempts advanced twice")
		}
	})
	t.Run("chosen original password issues ordinary policy", func(t *testing.T) {
		a, m, _, _, _ := setup()
		h, e := domain.HashPassword("ExistingPassword123!", "")
		if e != nil {
			t.Fatal(e)
		}
		m.user.PasswordHash = h
		res, e := a.Login(t.Context(), m.user.Email, "ExistingPassword123!", false)
		if e != nil || res.Session.ProtocolVersion != 2 || res.Session.AbsoluteExpiresAt.Sub(now) != 168*time.Hour || res.Session.IdleTimeoutSeconds != 43200 {
			t.Fatal("ordinary issuance failed", e)
		}
	})

	t.Run("real crypto round trip with fractional creation", func(t *testing.T) {
		a, m, _, _, q := setup()
		key, e := rsa.GenerateKey(rand.Reader, 2048)
		if e != nil {
			t.Fatal(e)
		}
		private := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
		public := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey)})
		codec, e := tokenadapter.New("fixture", private, map[string][]byte{"fixture": public})
		if e != nil {
			t.Fatal(e)
		}
		protector, e := tokenadapter.NewReceiptProtector(make([]byte, 32))
		if e != nil {
			t.Fatal(e)
		}
		a.WithSessionSecurity(codec, protector, q)
		now = now.Truncate(time.Second).Add(345678 * time.Microsecond)
		res, e := a.Login(t.Context(), m.user.Email, "short", false)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = a.CurrentAccess(t.Context(), res.AccessToken, res.CSRFToken, identity.NoActivity); e != nil {
			t.Fatal(e)
		}
		child, e := a.Refresh(t.Context(), res.RefreshToken, res.CSRFToken, "peer", uuid.New())
		if e != nil {
			t.Fatal(e)
		}
		same, e := a.Refresh(t.Context(), res.RefreshToken, res.CSRFToken, "peer", uuid.New())
		if e != nil || same.RefreshToken != child.RefreshToken || same.AccessToken != child.AccessToken {
			t.Fatal("encrypted receipt recovery failed", e)
		}
	})

	t.Run("numeric date rounds issue time", func(t *testing.T) {
		a, m, c, _, _ := setup()
		now = now.Add(123456789 * time.Nanosecond)
		res, e := a.Login(t.Context(), m.user.Email, "short", false)
		if e != nil {
			t.Fatal(e)
		}
		claims := c.claims[res.AccessToken]
		claims.IssuedAt = claims.IssuedAt.Truncate(time.Second)
		claims.NotBefore = claims.NotBefore.Truncate(time.Second)
		claims.ExpiresAt = claims.ExpiresAt.Truncate(time.Second)
		c.claims[res.AccessToken] = claims
		if _, e = a.CurrentAccess(t.Context(), res.AccessToken, res.CSRFToken, identity.NoActivity); e != nil {
			t.Fatal("issued JWT rejected after NumericDate rounding", e)
		}
	})
}

func randomV2Token() (string, []byte, error) {
	h := sha256.Sum256([]byte(uuid.NewString()))
	return base64.RawURLEncoding.EncodeToString(h[:]), h[:], nil
}

// These cases advance the server clock inside persistence lock callbacks. They
// model a cleanup lock wait, without sleeping or claiming real PG concurrency.
func TestSessionV2ApplicationClockLocks(t *testing.T) {
	base := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	setup := func() (*identity.Auth, *v2Model, *time.Time, string, string, *v2Coordinator) {
		now := base
		csrf, _, _ := randomV2Token()
		token, _, _ := randomV2Token()
		raw, _ := base64.RawURLEncoding.DecodeString(token)
		digest := sha256.Sum256(raw)
		proof := sha256.Sum256([]byte(csrf))
		uid, sid := uuid.New(), uuid.New()
		m := &v2Model{user: domain.User{ID: uid, EmailVerifiedAt: base, Status: domain.StatusActive, SessionEpoch: 1}, sessions: map[uuid.UUID]domain.AuthSession{}, refresh: map[string]domain.RefreshToken{}, guards: map[string]domain.LoginGuard{}}
		m.sessions[sid] = domain.AuthSession{ID: sid, AccountID: uid, CredentialRevision: 1, ProtocolVersion: 2, IdleTimeoutSeconds: 43200, CreatedAt: base.Add(-24 * time.Hour), LastActiveAt: base, AbsoluteExpiresAt: base.Add(7 * 24 * time.Hour), CSRFHash: proof[:]}
		m.refresh[string(digest[:])] = domain.RefreshToken{ID: uuid.New(), SessionID: sid, AccountID: uid, Digest: digest[:], IssuedAt: base}
		q := &v2Coordinator{}
		a := identity.NewEmailAuth(m, v2Proof{}, identity.EmailPolicy{}, func() time.Time { return now }).WithSessionSecurity(&v2Codec{claims: map[string]identity.AccessClaims{}}, &v2Receipt{}, q)
		return a, m, &now, token, csrf, q
	}
	getSession := func(m *v2Model) domain.AuthSession {
		for _, s := range m.sessions {
			return s
		}
		panic("fixture absent")
	}
	for _, kind := range []string{"idle", "absolute"} {
		t.Run("current token lock crosses "+kind, func(t *testing.T) {
			a, m, now, token, csrf, _ := setup()
			s := getSession(m)
			if kind == "idle" {
				s.LastActiveAt = base.Add(-12*time.Hour + time.Second)
			} else {
				s.AbsoluteExpiresAt = base.Add(time.Second)
			}
			m.sessions[s.ID] = s
			m.onRefreshLock = func() { *now = base.Add(time.Second) }
			got, err := a.Refresh(t.Context(), token, csrf, "peer", uuid.New())
			if !errors.Is(err, identity.ErrSessionInvalid) || got.AccessToken != "" || got.RefreshToken != "" || m.sessions[s.ID].RefreshGeneration != 0 {
				t.Fatal("lock wait authorized expired rotation", err)
			}
		})
	}
	for _, lock := range []string{"current token", "child receipt"} {
		t.Run("known old crosses replay window at "+lock, func(t *testing.T) {
			a, m, now, token, csrf, _ := setup()
			child, err := a.Refresh(t.Context(), token, csrf, "peer", uuid.New())
			if err != nil {
				t.Fatal(err)
			}
			*now = base.Add(9 * time.Second)
			if lock == "current token" {
				m.onRefreshLock = func() { *now = base.Add(10 * time.Second) }
			} else {
				m.onGenerationLock = func(g int64) {
					if g == 1 {
						*now = base.Add(10 * time.Second)
					}
				}
			}
			got, err := a.Refresh(t.Context(), token, csrf, "peer", uuid.New())
			s := m.sessions[child.Session.ID]
			if !errors.Is(err, identity.ErrSessionInvalid) || got.AccessToken != "" || got.RefreshToken != "" || s.RevokedAt == nil {
				t.Fatal("known old crossing window failed to commit revocation", err)
			}
			for _, r := range m.refresh {
				if r.RevokedAt == nil {
					t.Fatal("refresh chain remained live")
				}
			}
		})
	}
	for _, lock := range []string{"current token", "child receipt", "parent receipt"} {
		t.Run("restore lock crosses session expiry at "+lock, func(t *testing.T) {
			a, m, now, token, csrf, _ := setup()
			child, err := a.Refresh(t.Context(), token, csrf, "peer", uuid.New())
			if err != nil {
				t.Fatal(err)
			}
			s := m.sessions[child.Session.ID]
			s.AbsoluteExpiresAt = base.Add(2 * time.Second)
			m.sessions[s.ID] = s
			*now = base.Add(time.Second)
			incoming := token
			switch lock {
			case "current token":
				m.onRefreshLock = func() { *now = base.Add(2 * time.Second) }
			case "child receipt":
				m.onGenerationLock = func(g int64) {
					if g == 1 {
						*now = base.Add(2 * time.Second)
					}
				}
			default:
				incoming = child.RefreshToken
				m.onGenerationLock = func(g int64) {
					if g == 0 {
						*now = base.Add(2 * time.Second)
					}
				}
			}
			got, err := a.Refresh(t.Context(), incoming, csrf, "peer", uuid.New())
			if !errors.Is(err, identity.ErrSessionInvalid) || got.AccessToken != "" || got.RefreshToken != "" || m.sessions[s.ID].RefreshGeneration != 1 {
				t.Fatal("lock wait restored expired device", err)
			}
		})
	}
	t.Run("immediate child crossing parent window rotates fresh", func(t *testing.T) {
		a, m, now, token, csrf, _ := setup()
		child, err := a.Refresh(t.Context(), token, csrf, "peer", uuid.New())
		if err != nil {
			t.Fatal(err)
		}
		*now = base.Add(9 * time.Second)
		m.onGenerationLock = func(g int64) {
			if g == 0 {
				*now = base.Add(10 * time.Second)
			}
		}
		got, err := a.Refresh(t.Context(), child.RefreshToken, csrf, "peer", uuid.New())
		if err != nil || got.RefreshToken == child.RefreshToken || got.Session.RefreshGeneration != 2 || !got.ServerTime.Equal(base.Add(10*time.Second)) {
			t.Fatal("parent lock returned stale recovery", err)
		}
	})
	t.Run("logout current lock crosses absolute expiry", func(t *testing.T) {
		a, m, now, token, csrf, _ := setup()
		s := getSession(m)
		s.AbsoluteExpiresAt = base.Add(time.Second)
		m.sessions[s.ID] = s
		m.onRefreshLock = func() { *now = base.Add(time.Second) }
		err := a.LogoutCurrentWithRefresh(t.Context(), token, csrf, s.ID)
		if !errors.Is(err, identity.ErrSessionInvalid) || m.sessions[s.ID].RevokedAt != nil {
			t.Fatal("logout classified expired credentials as current", err)
		}
	})
	t.Run("poisoned cache cannot replace durable recovery", func(t *testing.T) {
		a, m, _, token, csrf, q := setup()
		child, err := a.Refresh(t.Context(), token, csrf, "peer", uuid.New())
		if err != nil {
			t.Fatal(err)
		}
		q.receipt = []byte(`{"Access":"poisoned","Refresh":"poisoned"}`)
		got, err := a.Refresh(t.Context(), token, csrf, "peer", uuid.New())
		if err != nil || got.AccessToken != child.AccessToken || got.RefreshToken != child.RefreshToken {
			t.Fatal("cache replaced durable receipt", err)
		}
		s := m.sessions[child.Session.ID]
		s.RefreshGeneration = 2
		m.sessions[s.ID] = s
		got, err = a.Refresh(t.Context(), token, csrf, "peer", uuid.New())
		if !errors.Is(err, identity.ErrRefreshRaced) || got.AccessToken != "" || got.RefreshToken != "" {
			t.Fatal("cache bypassed current generation", err)
		}
	})
}
