package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

func sessionHTTPFixture(t *testing.T, secure bool) (*gin.Engine, *v2Model, *time.Time) {
	t.Helper()
	r, m, now, _ := sessionHTTPAuthFixture(t, secure)
	return r, m, now
}
func sessionHTTPAuthFixture(t *testing.T, secure bool) (*gin.Engine, *v2Model, *time.Time, *identity.Auth) {
	t.Helper()
	now := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	hash, err := bcrypt.GenerateFromPassword([]byte("short"), 12)
	if err != nil {
		t.Fatal(err)
	}
	m := &v2Model{user: domain.User{ID: uuid.New(), Email: "fixture@example.invalid", EmailVerifiedAt: now, Status: domain.StatusActive, PasswordHash: string(hash), SessionEpoch: 1, Revision: 1, MustChangePassword: true}, sessions: map[uuid.UUID]domain.AuthSession{}, refresh: map[string]domain.RefreshToken{}, guards: map[string]domain.LoginGuard{}}
	auth := identity.NewEmailAuth(m, v2Proof{}, identity.EmailPolicy{}, func() time.Time { return now }).WithSessionSecurity(&v2Codec{claims: map[string]identity.AccessClaims{}}, &v2Receipt{}, &v2Coordinator{})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := identityhttp.NewEmailAuthHandler(auth, "https://fixture.invalid", secure)
	h.Register(r.Group("/api"))
	protected := r.Group("/api", h.RequireSession())
	protected.GET("/me", func(c *gin.Context) { c.Status(200) })
	protected.GET("/operations/:id", func(c *gin.Context) { c.Status(200) })
	protected.PATCH("/projects/:pid", func(c *gin.Context) { c.Status(403) })
	return r, m, &now, auth
}
func sessionHTTPRequest(t *testing.T, r *gin.Engine, method, path string, cookies []*http.Cookie, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewBufferString(`{"email":"fixture@example.invalid","password":"short","persistent":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://fixture.invalid")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	req.Header.Set("X-CSRF-Token", csrf)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}
func TestSessionV2HTTP(t *testing.T) {
	for _, secure := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "secure"}[secure], func(t *testing.T) {
			r, m, now := sessionHTTPFixture(t, secure)
			login := sessionHTTPRequest(t, r, "POST", "/api/sessions", nil, "")
			if login.Code != 201 {
				t.Fatalf("login %d %s", login.Code, login.Body)
			}
			cookies := login.Result().Cookies()
			if len(cookies) != 3 {
				t.Fatalf("want three v2 cookies, got %d", len(cookies))
			}
			var csrf string
			for _, c := range cookies {
				if c.Path != "/" || c.Domain != "" || c.Secure != secure || c.SameSite != http.SameSiteLaxMode || c.Expires.IsZero() {
					t.Fatalf("attributes %+v", c)
				}
				if !c.HttpOnly {
					csrf = c.Value
				}
			}

			prefix := "glowloom_"
			if secure {
				prefix = "__Host-glowloom_"
			}
			for _, cookie := range cookies {
				expected := now.Add(30 * 24 * time.Hour)
				if cookie.Name == prefix+"access" {
					expected = now.Add(15 * time.Minute)
				}
				if !cookie.Expires.Equal(expected) || cookie.MaxAge != 0 {
					t.Fatalf("cookie deadline %+v", cookie)
				}
			}
			var p map[string]any
			if e := json.Unmarshal(login.Body.Bytes(), &p); e != nil {
				t.Fatal(e)
			}
			for _, k := range []string{"token", "access_token", "refresh_token"} {
				if _, ok := p[k]; ok {
					t.Fatalf("secret %s", k)
				}
			}
			if p["session_protocol"] != "v2" || p["server_time"] == nil || p["idle_expires_at"] == nil {
				t.Fatalf("metadata %v", p)
			}
			if !m.user.MustChangePassword {
				t.Fatal("historical flag mutated")
			}
			current := sessionHTTPRequest(t, r, "GET", "/api/session", cookies, "")
			if len(current.Result().Cookies()) != 0 {
				t.Fatal("GET issued credentials")
			}
			if current.Code != 200 {
				t.Fatalf("current %d %s", current.Code, current.Body)
			}
			*now = now.Add(15 * time.Minute)
			expired := sessionHTTPRequest(t, r, "GET", "/api/session", cookies, "")
			if expired.Code != 401 || !bytes.Contains(expired.Body.Bytes(), []byte(`"access_expired"`)) {
				t.Fatalf("expired %d %s", expired.Code, expired.Body)
			}
			refresh := sessionHTTPRequest(t, r, "POST", "/api/session/refresh", cookies, csrf)
			if refresh.Code != 200 {
				t.Fatalf("refresh %d %s", refresh.Code, refresh.Body)
			}
			if refresh.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cacheable")
			}
			empty := sessionHTTPRequest(t, r, "GET", "/api/session", nil, "")
			if empty.Code != 401 || !bytes.Contains(empty.Body.Bytes(), []byte(`"session_invalid"`)) {
				t.Fatalf("empty %d %s", empty.Code, empty.Body)
			}
		})
	}
}
func TestSessionV2MissingCSRF(t *testing.T) {
	r, m, _ := sessionHTTPFixture(t, false)
	login := sessionHTTPRequest(t, r, "POST", "/api/sessions", nil, "")
	for _, csrf := range []string{"", "wrong"} {
		rec := sessionHTTPRequest(t, r, "POST", "/api/session/refresh", login.Result().Cookies(), csrf)
		if rec.Code != 403 {
			t.Fatalf("csrf %d %s", rec.Code, rec.Body)
		}
		if len(m.refresh) != 1 {
			t.Fatal("rotated without CSRF")
		}
	}
	cookies := login.Result().Cookies()
	for _, cookie := range cookies {
		if cookie.Name == "glowloom_csrf" {
			cookie.Value = "forged"
		}
	}
	rec := sessionHTTPRequest(t, r, "POST", "/api/session/refresh", cookies, "forged")
	if rec.Code != 403 || len(m.refresh) != 1 {
		t.Fatalf("forged bound CSRF %d %s", rec.Code, rec.Body)
	}
}

func TestSessionV2HTTPActivity(t *testing.T) {
	r, m, now := sessionHTTPFixture(t, false)
	login := sessionHTTPRequest(t, r, "POST", "/api/sessions", nil, "")
	cookies := login.Result().Cookies()
	var csrf string
	var sid uuid.UUID
	for _, c := range cookies {
		if !c.HttpOnly {
			csrf = c.Value
		}
	}
	for id := range m.sessions {
		sid = id
	}
	initial := m.sessions[sid].LastActiveAt
	*now = now.Add(time.Minute)
	for _, tc := range []struct{ method, path, csrf string }{{"GET", "/api/operations/" + uuid.NewString(), ""}, {"PATCH", "/api/projects/" + uuid.NewString(), ""}, {"PATCH", "/api/projects/" + uuid.NewString(), csrf}} {
		sessionHTTPRequest(t, r, tc.method, tc.path, cookies, tc.csrf)
		if !m.sessions[sid].LastActiveAt.Equal(initial) {
			t.Fatalf("refused/background request touched: %+v", tc)
		}
	}
	rec := sessionHTTPRequest(t, r, "GET", "/api/me", cookies, "")
	if rec.Code != 200 || !m.sessions[sid].LastActiveAt.Equal(*now) {
		t.Fatalf("foreground GET did not touch: %d", rec.Code)
	}
}
func TestSessionV2HTTPLogoutAndProtocol(t *testing.T) {
	r, m, now := sessionHTTPFixture(t, false)
	login := sessionHTTPRequest(t, r, "POST", "/api/sessions", nil, "")
	cookies := login.Result().Cookies()
	var csrf string
	var sid uuid.UUID
	for _, c := range cookies {
		if !c.HttpOnly {
			csrf = c.Value
		}
	}
	for id := range m.sessions {
		sid = id
	}
	// A still-valid access also must revoke when no directory adapter is installed.
	rec := sessionHTTPRequest(t, r, "DELETE", "/api/sessions/"+sid.String(), cookies, csrf)
	if rec.Code != 204 || m.sessions[sid].RevokedAt == nil {
		t.Fatalf("valid access logout did not revoke: %d %s", rec.Code, rec.Body)
	}
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge != -1 || c.Value != "" || c.Domain != "" {
			t.Fatalf("cookie not removed locally: %+v", c)
		}
	}
	if len(rec.Result().Cookies()) != 7 {
		t.Fatal("must remove v2 and legacy cookies")
	}
	// Fresh device can log out after access expiry without refresh rotation.
	login = sessionHTTPRequest(t, r, "POST", "/api/sessions", nil, "")
	cookies = login.Result().Cookies()
	for _, c := range cookies {
		if !c.HttpOnly {
			csrf = c.Value
		}
	}
	for id, s := range m.sessions {
		if s.RevokedAt == nil {
			sid = id
		}
	}
	*now = now.Add(15 * time.Minute)
	wrong := sessionHTTPRequest(t, r, "DELETE", "/api/sessions/"+uuid.NewString(), cookies, csrf)
	if wrong.Code != 403 || m.sessions[sid].RevokedAt != nil {
		t.Fatalf("refresh revoked other target %d", wrong.Code)
	}
	rec = sessionHTTPRequest(t, r, "DELETE", "/api/sessions/"+sid.String(), cookies, csrf)
	if rec.Code != 204 || m.sessions[sid].RevokedAt == nil {
		t.Fatalf("expired access logout %d %s", rec.Code, rec.Body)
	}
	rec = sessionHTTPRequest(t, r, "DELETE", "/api/sessions/"+sid.String(), cookies, csrf)
	if rec.Code != 204 {
		t.Fatalf("logout not idempotent %d", rec.Code)
	}
}

func (m *v2Model) SessionByHash(hash []byte, _ bool) (domain.AuthSession, error) {
	for _, s := range m.sessions {
		if bytes.Equal(s.TokenHash, hash) {
			return s, nil
		}
	}
	return domain.AuthSession{}, identity.ErrSessionInvalid
}
func TestSessionV2HTTPProtocolSelection(t *testing.T) {
	r, m, _, auth := sessionHTTPAuthFixture(t, false)
	auth.WithSessionIssuance(false)
	login := sessionHTTPRequest(t, r, "POST", "/api/sessions", nil, "")
	if login.Code != 201 || len(login.Result().Cookies()) != 1 {
		t.Fatalf("v1 login %d %s", login.Code, login.Body)
	}
	legacy := login.Result().Cookies()
	current := sessionHTTPRequest(t, r, "GET", "/api/session", legacy, "")
	var p map[string]any
	if err := json.Unmarshal(current.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p["session_protocol"] != "v1" || p["access_expires_at"] != nil || p["refresh_after_at"] != nil || p["upgrade_available"] != false {
		t.Fatalf("legacy metadata %v", p)
	}
	var csrf string
	csrf, _ = p["csrf_token"].(string)
	bad := append(append([]*http.Cookie{}, legacy...), &http.Cookie{Name: "glowloom_access", Value: "invalid"})
	rec := sessionHTTPRequest(t, r, "GET", "/api/session", bad, "")
	if rec.Code != 401 {
		t.Fatalf("v2 failure fell back: %d", rec.Code)
	}
	rec = sessionHTTPRequest(t, r, "POST", "/api/session/upgrade", legacy, csrf)
	if rec.Code != 503 {
		t.Fatalf("upgrade prematurely ready %d", rec.Code)
	}
	for _, s := range m.sessions {
		if s.ProtocolVersion != 1 {
			t.Fatal("closed upgrade changed protocol")
		}
	}
}
func TestSessionV2HTTPSecurityErrors(t *testing.T) {
	r, m, _ := sessionHTTPFixture(t, false)
	login := sessionHTTPRequest(t, r, "POST", "/api/sessions", nil, "")
	cookies := login.Result().Cookies()
	var csrf string
	for _, c := range cookies {
		if !c.HttpOnly {
			csrf = c.Value
		}
	}
	for _, tc := range []struct {
		name, origin, key string
		want              int
	}{{"origin", "https://other.invalid", uuid.NewString(), 403}, {"key", "https://fixture.invalid", "", 422}} {
		req := httptest.NewRequestWithContext(t.Context(), "POST", "/api/session/refresh", nil)
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Idempotency-Key", tc.key)
		req.Header.Set("X-CSRF-Token", csrf)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != tc.want || rec.Header().Get("Cache-Control") != "no-store" || len(m.refresh) != 1 {
			t.Fatalf("%s: %d %s", tc.name, rec.Code, rec.Body)
		}
	}
	var refreshOnly []*http.Cookie
	for _, c := range cookies {
		if c.Name != "glowloom_access" {
			refreshOnly = append(refreshOnly, c)
		}
	}
	rec := sessionHTTPRequest(t, r, "GET", "/api/session", refreshOnly, "")
	if rec.Code != 401 || !bytes.Contains(rec.Body.Bytes(), []byte(`"access_expired"`)) {
		t.Fatalf("missing access %d %s", rec.Code, rec.Body)
	}
	// Random refresh is terminal and GET does not rotate it or clear credentials.
	random := append([]*http.Cookie{}, refreshOnly...)
	for i, c := range random {
		if c.Name == "glowloom_refresh" {
			copyCookie := *c
			copyCookie.Value = uuid.NewString()
			random[i] = &copyCookie
		}
	}
	rec = sessionHTTPRequest(t, r, "POST", "/api/session/refresh", random, csrf)
	if rec.Code != 401 || len(rec.Result().Cookies()) != 0 || len(m.refresh) != 1 {
		t.Fatalf("random refresh %d %s", rec.Code, rec.Body)
	}
}
func TestSessionV2HTTPSensitivePrincipal(t *testing.T) {
	r, m, _, _ := sessionHTTPAuthFixture(t, false)
	login := sessionHTTPRequest(t, r, "POST", "/api/sessions", nil, "")
	cookies := login.Result().Cookies()
	var csrf string
	for _, c := range cookies {
		if !c.HttpOnly {
			csrf = c.Value
		}
	}
	request := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), "POST", path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://fixture.invalid")
		req.Header.Set("Idempotency-Key", uuid.NewString())
		req.Header.Set("X-CSRF-Token", csrf)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	rec := request("/api/me/reauthentication", `{"password":"short"}`)
	if rec.Code != 204 {
		t.Fatalf("reauth %d %s", rec.Code, rec.Body)
	}
	for _, s := range m.sessions {
		if s.ReauthAt == nil {
			t.Fatal("principal reauthentication not recorded")
		}
	}
	rec = request("/api/me/password", `{"expected_credential_epoch":"1","current_password":"short","new_password":"NewStrongPassword789!","confirm_password":"NewStrongPassword789!"}`)
	if rec.Code != 200 || len(rec.Result().Cookies()) != 3 || m.user.SessionEpoch != 2 {
		t.Fatalf("password %d %s", rec.Code, rec.Body)
	}
}

func TestSessionV2HTTPOrdinaryCookies(t *testing.T) {
	r, _, now := sessionHTTPFixture(t, false)
	req := httptest.NewRequestWithContext(t.Context(), "POST", "/api/sessions", bytes.NewBufferString(`{"email":"fixture@example.invalid","password":"short","persistent":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://fixture.invalid")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 201 || len(rec.Result().Cookies()) != 3 {
		t.Fatalf("ordinary login %d %s", rec.Code, rec.Body)
	}
	for _, cookie := range rec.Result().Cookies() {
		if !cookie.Expires.IsZero() || cookie.MaxAge != 0 {
			t.Fatalf("ordinary cookie persists %+v", cookie)
		}
	}
	var p identityhttp.EmailSessionView
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if !p.AbsoluteExpiresAt.Equal(now.Add(7*24*time.Hour)) || !p.IdleExpiresAt.Equal(now.Add(12*time.Hour)) {
		t.Fatalf("ordinary deadlines %+v", p)
	}
}

type httpAdmissionFailure struct {
	identity.RefreshCoordinator
	err error
}

func (q httpAdmissionFailure) Admit(context.Context, uuid.UUID, string) (func(context.Context) error, error) {
	return nil, q.err
}
func TestSessionV2HTTPRefreshFailures(t *testing.T) {
	r, m, _, auth := sessionHTTPAuthFixture(t, false)
	login := sessionHTTPRequest(t, r, "POST", "/api/sessions", nil, "")
	cookies := login.Result().Cookies()
	var csrf string
	for _, c := range cookies {
		if !c.HttpOnly {
			csrf = c.Value
		}
	}
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{{identity.ErrRefreshBusy, 409, "refresh_busy"}, {identity.ErrRefreshRateLimited, 429, "rate_limited"}, {identity.ErrRefreshDependency, 503, "temporarily_unavailable"}} {
		auth.WithSessionSecurity(&v2Codec{claims: map[string]identity.AccessClaims{}}, &v2Receipt{}, httpAdmissionFailure{err: tc.err})
		rec := sessionHTTPRequest(t, r, "POST", "/api/session/refresh", cookies, csrf)
		if rec.Code != tc.status || !bytes.Contains(rec.Body.Bytes(), []byte(tc.code)) || len(rec.Result().Cookies()) != 0 || len(m.refresh) != 1 || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("refresh failure %d %s", rec.Code, rec.Body)
		}
	}
}

func TestSessionV2HTTPCookieAmbiguity(t *testing.T) {
	for _, secure := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "secure"}[secure], func(t *testing.T) {
			r, m, now := sessionHTTPFixture(t, secure)
			login := sessionHTTPRequest(t, r, "POST", "/api/sessions", nil, "")
			cookies := login.Result().Cookies()
			var csrf string
			var sid uuid.UUID
			for _, c := range cookies {
				if !c.HttpOnly {
					csrf = c.Value
				}
			}
			for id := range m.sessions {
				sid = id
			}
			initial := m.sessions[sid].LastActiveAt
			*now = now.Add(time.Minute)
			prefix := "glowloom_"
			foreign := "__Host-glowloom_"
			if secure {
				prefix, foreign = foreign, prefix
			}
			for _, name := range []string{prefix + "access", prefix + "refresh", prefix + "csrf"} {
				duplicate := append(append([]*http.Cookie{}, cookies...), &http.Cookie{Name: name, Value: "bad-second"})
				for _, tc := range []struct{ method, path string }{{"GET", "/api/session"}, {"GET", "/api/me"}, {"POST", "/api/session/refresh"}, {"DELETE", "/api/sessions/" + sid.String()}} {
					rec := sessionHTTPRequest(t, r, tc.method, tc.path, duplicate, csrf)
					if rec.Code != 401 || len(m.refresh) != 1 || m.sessions[sid].RevokedAt != nil || !m.sessions[sid].LastActiveAt.Equal(initial) {
						t.Errorf("duplicate %s %s: %d %s", name, tc.path, rec.Code, rec.Body)
					}
				}
			}
			for _, name := range []string{foreign + "access", foreign + "refresh", foreign + "csrf", "glowloom_session"} {
				mixed := append(append([]*http.Cookie{}, cookies...), &http.Cookie{Name: name, Value: "stale"})
				for _, tc := range []struct{ method, path string }{{"GET", "/api/session"}, {"POST", "/api/session/refresh"}, {"DELETE", "/api/sessions/" + sid.String()}} {
					rec := sessionHTTPRequest(t, r, tc.method, tc.path, mixed, csrf)
					if rec.Code != 401 || len(m.refresh) != 1 || m.sessions[sid].RevokedAt != nil || !m.sessions[sid].LastActiveAt.Equal(initial) {
						t.Errorf("namespace collision %s %s: %d", name, tc.path, rec.Code)
					}
				}
			}
			unrelated := append(append([]*http.Cookie{}, cookies...), &http.Cookie{Name: "theme", Value: "dark"}, &http.Cookie{Name: "theme", Value: "light"})
			rec := sessionHTTPRequest(t, r, "GET", "/api/me", unrelated, "")
			if rec.Code != 200 {
				t.Fatalf("unrelated cookies blocked %d", rec.Code)
			}
		})
	}
}
func TestSessionV2HTTPLegacyCookieAmbiguity(t *testing.T) {
	r, m, now, auth := sessionHTTPAuthFixture(t, false)
	auth.WithSessionIssuance(false)
	login := sessionHTTPRequest(t, r, "POST", "/api/sessions", nil, "")
	legacy := login.Result().Cookies()
	var session identityhttp.EmailSessionView
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	var sid uuid.UUID
	for id := range m.sessions {
		sid = id
	}
	initial := m.sessions[sid].LastActiveAt
	*now = now.Add(time.Minute)
	for _, name := range []string{"glowloom_session", "__Host-glowloom_access", "__Host-glowloom_refresh", "__Host-glowloom_csrf", "glowloom_access", "glowloom_refresh", "glowloom_csrf"} {
		cookies := append(append([]*http.Cookie{}, legacy...), &http.Cookie{Name: name, Value: "bad-second"})
		for _, tc := range []struct{ method, path string }{{"GET", "/api/session"}, {"POST", "/api/me/reauthentication"}, {"DELETE", "/api/sessions/" + sid.String()}} {
			rec := sessionHTTPRequest(t, r, tc.method, tc.path, cookies, session.CSRFToken)
			if rec.Code != 401 || m.sessions[sid].ReauthAt != nil || m.sessions[sid].RevokedAt != nil || !m.sessions[sid].LastActiveAt.Equal(initial) {
				t.Errorf("legacy fallback/duplicate %s %s: %d", name, tc.path, rec.Code)
			}
		}
	}
}
func TestSessionV2HTTPLoginClearsStaleNamespaces(t *testing.T) {
	for _, secure := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "secure"}[secure], func(t *testing.T) {
			r, _, _ := sessionHTTPFixture(t, secure)
			foreign := "glowloom_"
			if !secure {
				foreign = "__Host-glowloom_"
			}
			stale := []*http.Cookie{{Name: foreign + "access", Value: "stale"}, {Name: foreign + "refresh", Value: "stale"}, {Name: foreign + "csrf", Value: "stale"}, {Name: "glowloom_session", Value: "stale"}}
			rec := sessionHTTPRequest(t, r, "POST", "/api/sessions", stale, "")
			if rec.Code != 201 {
				t.Fatalf("stale cookies blocked original password %d", rec.Code)
			}
			jar := map[string]*http.Cookie{}
			for _, c := range stale {
				jar[c.Name] = c
			}
			removed := 0
			for _, c := range rec.Result().Cookies() {
				if c.MaxAge < 0 {
					delete(jar, c.Name)
					removed++
				} else {
					jar[c.Name] = c
				}
			}
			if removed != 4 || len(jar) != 3 {
				t.Fatalf("stale namespaces remain: removed=%d jar=%v", removed, jar)
			}
			var cookies []*http.Cookie
			for _, c := range jar {
				cookies = append(cookies, c)
			}
			current := sessionHTTPRequest(t, r, "GET", "/api/session", cookies, "")
			if current.Code != 200 {
				t.Fatalf("fresh canonical GET %d %s", current.Code, current.Body)
			}
		})
	}
}
func TestSessionV2HTTPFolderActivityRoute(t *testing.T) {
	r, m, now, auth := sessionHTTPAuthFixture(t, false)
	h := identityhttp.NewEmailAuthHandler(auth, "https://fixture.invalid", false)
	protected := r.Group("/api", h.RequireSession())
	for _, method := range []string{"PATCH", "DELETE"} {
		protected.Handle(method, "/project-folders/:folder_id", func(c *gin.Context) {
			if c.Param("folder_id") == "" {
				t.Error("missing real route parameter")
			}
			c.Status(204)
		})
	}
	login := sessionHTTPRequest(t, r, "POST", "/api/sessions", nil, "")
	cookies := login.Result().Cookies()
	var csrf string
	var sid uuid.UUID
	for _, c := range cookies {
		if !c.HttpOnly {
			csrf = c.Value
		}
	}
	for id := range m.sessions {
		sid = id
	}
	for _, method := range []string{"PATCH", "DELETE"} {
		*now = now.Add(time.Minute)
		rec := sessionHTTPRequest(t, r, method, "/api/project-folders/"+uuid.NewString(), cookies, csrf)
		if rec.Code != 204 || !m.sessions[sid].LastActiveAt.Equal(*now) {
			t.Errorf("%s :folder_id did not touch", method)
		}
	}
}

func TestSessionV2HTTPPasswordReplacesLegacyCookies(t *testing.T) {
	r, _, _, auth := sessionHTTPAuthFixture(t, false)
	auth.WithSessionIssuance(false)
	login := sessionHTTPRequest(t, r, "POST", "/api/sessions", nil, "")
	cookies := login.Result().Cookies()
	var session identityhttp.EmailSessionView
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	auth.WithSessionIssuance(true)
	req := httptest.NewRequestWithContext(t.Context(), "POST", "/api/me/password", bytes.NewBufferString(`{"expected_credential_epoch":"1","current_password":"short","new_password":"NewStrongPassword789!","confirm_password":"NewStrongPassword789!"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://fixture.invalid")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	req.Header.Set("X-CSRF-Token", session.CSRFToken)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("legacy password transition %d %s", rec.Code, rec.Body)
	}
	jar := map[string]*http.Cookie{}
	for _, c := range cookies {
		jar[c.Name] = c
	}
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(jar, c.Name)
		} else {
			jar[c.Name] = c
		}
	}
	if _, ok := jar["glowloom_session"]; ok {
		t.Fatal("v2 password issuance retained conflicting legacy cookie")
	}
	cookies = nil
	for _, c := range jar {
		cookies = append(cookies, c)
	}
	current := sessionHTTPRequest(t, r, "GET", "/api/session", cookies, "")
	if current.Code != 200 {
		t.Fatalf("post-password current %d", current.Code)
	}
}

func TestSessionV2UpgradeHTTP(t *testing.T) {
	a, m, now, old := legacyUpgradeFixture(t)
	a.WithLegacyUpgradeUntil(now.Add(24 * time.Hour))
	r := gin.New()
	identityhttp.NewEmailAuthHandler(a, "https://fixture.invalid", false).WithSessionPolicy(identityhttp.SessionHTTPPolicy{V2Enabled: true, LegacyUpgradeUntil: now.Add(24 * time.Hour)}).Register(r.Group("/api"))
	cookies := []*http.Cookie{{Name: "glowloom_session", Value: old.Token}}
	// Use the actual legacy cookie name used by the existing transport.
	cookies[0].Name = "glowloom_session"
	current := sessionHTTPRequest(t, r, "GET", "/api/session", cookies, "")
	var metadata map[string]any
	if err := json.Unmarshal(current.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if current.Code != 200 || metadata["upgrade_available"] != true {
		t.Fatalf("upgrade hint missing: %d %s", current.Code, current.Body)
	}
	upgraded := sessionHTTPRequest(t, r, "POST", "/api/session/upgrade", cookies, old.CSRFToken)
	if upgraded.Code != 200 {
		t.Fatalf("upgrade: %d %s", upgraded.Code, upgraded.Body)
	}
	if err := json.Unmarshal(upgraded.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["session_id"] != old.Session.ID.String() || metadata["refresh_generation"] != "1" || metadata["session_protocol"] != "v2" {
		t.Fatal("upgrade contract missing")
	}
	if len(m.sessions) != 1 {
		t.Fatal("new device created")
	}
	again := sessionHTTPRequest(t, r, "POST", "/api/session/upgrade", cookies, old.CSRFToken)
	if again.Code != 200 {
		t.Fatalf("lost response %d %s", again.Code, again.Body)
	}
}

func TestSessionV2UpgradeTrustedSourceHTTP(t *testing.T) {
	a, _, now, old := legacyUpgradeFixture(t)
	q := &upgradeSourceCoordinator{}
	a.WithSessionSecurity(&v2Codec{claims: map[string]identity.AccessClaims{}}, &v2Receipt{}, q).WithLegacyUpgradeUntil(now.Add(time.Hour))
	r := gin.New()
	identityhttp.NewEmailAuthHandler(a, "https://fixture.invalid", false).Register(r.Group("/api"))
	// Same SID moves between two legitimate transport peers. A forwarded header
	// cannot select the peer bucket, and the SID cannot replace the peer source.
	for _, remote := range []string{"192.0.2.10:40100", "198.51.100.20:40200"} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/session/upgrade", nil)
		req.RemoteAddr = remote
		req.Header.Set("Origin", "https://fixture.invalid")
		req.Header.Set("X-CSRF-Token", old.CSRFToken)
		req.Header.Set("Idempotency-Key", uuid.NewString())
		req.Header.Set("X-Forwarded-For", "203.0.113.99")
		req.Header.Set("X-Real-IP", "203.0.113.98")
		req.AddCookie(&http.Cookie{Name: "glowloom_session", Value: old.Token})
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("upgrade failed: %d", rec.Code)
		}
	}
	if len(q.sources) != 2 || q.sources[0] != "192.0.2.10" || q.sources[1] != "198.51.100.20" {
		t.Fatalf("transport peers lost: %v", q.sources)
	}
	if q.sessions[0] != old.Session.ID || q.sessions[1] != old.Session.ID {
		t.Fatal("source isolation changed device")
	}
}
