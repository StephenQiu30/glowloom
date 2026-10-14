package identity_test

import (
	"bytes"
	"encoding/json"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuthOriginAndEnumeration(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC()
	protector, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	store := pgidentity.NewStore(tx)
	auth := identity.NewEmailAuth(store, protector, identity.EmailPolicy{TermsVersion: "test", SourceHourlyLimit: 100, GlobalHourlyLimit: 1000}, func() time.Time { return now })
	hash, err := domain.HashPassword("long password for tests", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EmailTransaction(t.Context(), func(t identity.EmailTx) error {
		return t.CreatePersonalAccount(domain.User{ID: uuid.New(), OrgID: uuid.New(), LoginName: uuid.NewString(), Email: "known@http.invalid", EmailVersion: 1, EmailVerifiedAt: now, DisplayName: "known", PasswordHash: hash, PasswordChangedAt: now, TermsVersion: "test", TermsAcceptedAt: now})
	}); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := identityhttp.NewEmailAuthHandler(auth, "http://localhost:3200", false)
	handler.Register(router.Group("/api"))
	request := func(path, origin string, payload any, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(payload)
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		r.Header.Set("Idempotency-Key", uuid.NewString())
		r.Header.Set("X-CSRF-Token", csrf)
		r.RemoteAddr = "127.0.0.1:1234"
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	if request("/api/registrations", "https://attacker.invalid", map[string]any{"email": "outsider@http.invalid"}, nil, "").Code != 403 {
		t.Fatal("cross-origin authentication accepted")
	}
	for _, email := range []string{"known@http.invalid", "unknown@http.invalid"} {
		response := request("/api/password-resets", "http://localhost:3200", map[string]any{"email": email}, nil, "")
		var accepted map[string]any
		if response.Code != 202 || json.Unmarshal(response.Body.Bytes(), &accepted) != nil || len(accepted) != 2 || accepted["operation_id"] == nil || accepted["retry_after_seconds"] == nil {
			t.Fatalf("enumerating recovery response: %d %s", response.Code, response.Body)
		}
	}
	login := request("/api/sessions", "http://localhost:3200", map[string]any{"email": "known@http.invalid", "password": "long password for tests", "persistent": false}, nil, "")
	if login.Code != 201 {
		t.Fatalf("email login: %d %s", login.Code, login.Body)
	}
	var session struct {
		CSRFToken       string `json:"csrf_token"`
		CredentialEpoch string `json:"credential_epoch"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil || session.CSRFToken == "" || session.CredentialEpoch == "" {
		t.Fatal("session contract missing")
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly {
		t.Fatal("session cookie unavailable")
	}
	for _, csrf := range []string{"", "wrong"} {
		if request("/api/me/reauthentication", "http://localhost:3200", map[string]any{"password": "long password for tests"}, cookies[0], csrf).Code != 403 {
			t.Fatal("unbound authenticated write accepted")
		}
	}
	if w := request("/api/me/reauthentication", "http://localhost:3200", map[string]any{"password": "long password for tests"}, cookies[0], session.CSRFToken); w.Code != 204 {
		t.Fatalf("CSRF-bound reauthentication rejected: %d", w.Code)
	}
	for _, path := range []string{"/api/accounts/register", "/api/accounts/availability"} {
		if request(path, "http://localhost:3200", map[string]any{}, nil, "").Code != 404 {
			t.Fatal("legacy public account route survives")
		}
	}
}

func TestSelfProfileHTTPRejectsAuthorityFields(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC()
	auth, actor := verifiedTestActor(t, tx, "self-http@example.invalid", now)
	login, err := auth.Login(t.Context(), "self-http@example.invalid", "long password for tests", false)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	authHandler := identityhttp.NewEmailAuthHandler(auth, "http://localhost:3200", false)
	api := router.Group("/api")
	authHandler.Register(api)
	protected := api.Group("", authHandler.RequireSession())
	identityhttp.NewSelfProfileHandler(identity.NewSelfProfile(pgidentity.NewSelfProfileStore(tx, nil), func() time.Time { return now })).Register(protected)
	request := func(method, path string, body any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://localhost:3200")
		r.Header.Set("X-CSRF-Token", login.CSRFToken)
		r.AddCookie(&http.Cookie{Name: "lanverse_session", Value: login.Token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	read := request("GET", "/api/me", nil)
	var me map[string]any
	if read.Code != 200 || json.Unmarshal(read.Body.Bytes(), &me) != nil || me["id"] != actor.ID.String() {
		t.Fatal("self read failed")
	}
	for _, field := range []string{"platform_role", "email", "role", "credential_epoch"} {
		body := map[string]any{"expected_revision": me["revision"], "display_name": "Updated", field: "forged"}
		if request("PATCH", "/api/me", body).Code != 422 {
			t.Fatalf("writable authority field %s", field)
		}
	}
	updated := request("PATCH", "/api/me", map[string]any{"expected_revision": me["revision"], "display_name": "Updated"})
	if updated.Code != 200 {
		t.Fatalf("profile update failed: %d", updated.Code)
	}
	if request("PATCH", "/api/me", map[string]any{"expected_revision": me["revision"], "display_name": "Stale"}).Code != 409 {
		t.Fatal("HTTP stale profile accepted")
	}
}
