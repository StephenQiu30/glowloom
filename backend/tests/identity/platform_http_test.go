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

func TestPlatformDirectoryHTTPProjection(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC()
	auth, actor := verifiedTestActor(t, tx, "platform-http@example.invalid", now)
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='platform_admin',platform_epoch=platform_epoch+1,create_time=? WHERE id=?`, now.Add(-time.Second), actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	key, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := identityhttp.NewEmailAuthHandler(auth, "http://localhost:3200", false)
	h.Register(router.Group("/api"))
	identityhttp.NewPlatformDirectoryHandler(identity.NewPlatformDirectory(pgidentity.NewPlatformDirectoryStore(tx), key, func() time.Time { return now })).Register(router.Group("/api", h.RequireSession()))
	r := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(`{"email":"platform-http@example.invalid","password":"long password for tests"}`))
	r.Header.Set("Origin", "http://localhost:3200")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", uuid.NewString())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatalf("login %d", w.Code)
	}
	cookie := w.Result().Cookies()[0]
	get := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	for _, suffix := range []string{"?verified=1", "?in_team=yes", "?status=unknown", "?order=password_hash", "?limit=101", "?org_id=anything", "?limit=%ZZ"} {
		if w := get("/api/platform/users" + suffix); w.Code != 422 {
			t.Fatalf("unsafe filter accepted %s: %d", suffix, w.Code)
		}
	}
	w = get("/api/platform/users?search=platform-http%40example.invalid")
	var page struct {
		Items []map[string]any `json:"items"`
	}
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != 1 {
		t.Fatalf("invalid directory response %d", w.Code)
	}
	row := page.Items[0]
	if _, ok := row["revision"].(string); !ok {
		t.Fatal("revision is not a string")
	}
	for _, key := range []string{"password_hash", "session_epoch", "credential_epoch", "csrf_token", "org_id", "login_name", "prompt_suffix"} {
		if _, ok := row[key]; ok {
			t.Fatal("private field escaped DTO")
		}
	}
	if w := get("/api/platform/users/" + uuid.NewString()); w.Code != 404 {
		t.Fatal("missing user not mapped to 404")
	}
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='creator',platform_epoch=platform_epoch+1 WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if w := get("/api/platform/users/" + actor.ID.String()); w.Code != 403 {
		t.Fatal("revoked authority still allowed")
	}
}
