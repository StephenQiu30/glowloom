package identity_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestSecurityEventHTTPFilters(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC()
	auth, _ := verifiedTestActor(t, tx, "event-http@example.invalid", now)
	key, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := identityhttp.NewEmailAuthHandler(auth, "http://localhost:3200", false)
	h.Register(router.Group("/api"))
	identityhttp.NewEventHandler(identity.NewEventQuery(pgidentity.NewEventStore(tx), key, func() time.Time { return now })).Register(router.Group("/api", h.RequireSession()))
	r := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(`{"email":"event-http@example.invalid","password":"long password for tests"}`))
	r.Header.Set("Origin", "http://localhost:3200")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", uuid.NewString())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatalf("login: %d", w.Code)
	}
	cookie := w.Result().Cookies()[0]
	get := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	for _, suffix := range []string{"?limit=101", "?limit=0", "?limit=1&limit=2", "?target_user_id=anything", "?scope_kind=platform", "?from=bad", "?from=2030-01-01T00:00:00Z&to=2020-01-01T00:00:00Z", "?limit=%ZZ"} {
		if w := get("/api/me/security-events" + suffix); w.Code != 422 {
			t.Fatalf("invalid filter accepted %s: %d", suffix, w.Code)
		}
	}
	w = get("/api/me/security-events")
	var page identityhttp.SecurityEventPageView
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Items == nil || page.AsOf.IsZero() {
		t.Fatal("event page contract invalid")
	}
	if w := get("/api/platform/audit-events"); w.Code != 403 {
		t.Fatal("creator accessed platform audit route")
	}
}
