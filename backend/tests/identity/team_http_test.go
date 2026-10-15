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
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	workspacehttp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/http"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func TestTeamHTTPCurrentIdentityAndProjection(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	auth, _ := verifiedTestActor(t, tx, "team-http@example.invalid", now)
	if err := tx.Exec(`UPDATE identity.platform_settings SET team_creation_enabled=true WHERE singleton`).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := identityhttp.NewEmailAuthHandler(auth, "http://localhost:3200", false)
	h.Register(r.Group("/api"))
	store := pgworkspace.NewTeamStore(tx, func(tx *gorm.DB) workspace.TeamActorAccess {
		return pgidentity.NewTeamActorStore(tx, completionProtector(t))
	})
	teams := workspace.NewTeams(store, func() time.Time { return now })
	key, err := credentialproof.New("test", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	directory := workspace.NewTeamDirectory(store, key, func() time.Time { return now })
	workspacehttp.NewTeamHandler(teams, directory).Register(r.Group("/api", h.RequireSession()))
	var cookie *http.Cookie
	csrf := ""
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost:3200")
		req.Header.Set("Idempotency-Key", uuid.NewString())
		req.Header.Set("X-CSRF-Token", csrf)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	w := call("POST", "/api/sessions", `{"email":"team-http@example.invalid","password":"long password for tests"}`)
	if w.Code != 201 {
		t.Fatal(w.Code)
	}
	cookie = w.Result().Cookies()[0]
	var session identityhttp.EmailSessionView
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if w := call("POST", "/api/teams", `{"name":"Creative team"}`); w.Code != 403 {
		t.Fatal("team write accepted without CSRF")
	}
	csrf = session.CSRFToken
	w = call("POST", "/api/teams", `{"name":"Creative team","platform_role":"platform_admin"}`)
	if w.Code != 422 {
		t.Fatal("team creation accepted privileged fields", w.Code)
	}
	w = call("POST", "/api/teams", `{"name":"Creative team"}`)
	if w.Code != 201 {
		t.Fatalf("team creation: %d %s", w.Code, w.Body.String())
	}
	var team struct {
		ID       uuid.UUID `json:"id"`
		Revision string    `json:"revision"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &team); err != nil || team.ID == uuid.Nil || team.Revision != "1" {
		t.Fatal("invalid team contract", err)
	}
	if strings.Contains(w.Body.String(), "physical_scope_id") || strings.Contains(w.Body.String(), "org_id") {
		t.Fatal("physical storage escaped team DTO")
	}
	w = call("GET", "/api/me/spaces", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "physical_scope_id") {
		t.Fatal("invalid space projection", w.Code)
	}
	w = call("GET", "/api/teams/"+team.ID.String()+"/members?limit=1", "")
	if w.Code != 200 {
		t.Fatal("filtered member directory unavailable", w.Code)
	}
	for _, query := range []string{"?limit=%ZZ", "?limit=101", "?role=unknown", "?role=member&role=team_admin", "?from=invalid", "?scope_id=other"} {
		if w := call("GET", "/api/teams/"+team.ID.String()+"/members"+query, ""); w.Code != 422 {
			t.Fatal("invalid member query accepted", query, w.Code)
		}
	}
	w = call("PATCH", "/api/teams/"+team.ID.String(), `{"expected_revision":"1","name":"Renamed team"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Renamed team") {
		t.Fatal("rename failed", w.Code)
	}
	w = call("PATCH", "/api/teams/"+team.ID.String(), `{"expected_revision":"1","name":"Stale name"}`)
	if w.Code != 409 {
		t.Fatal("stale team edit accepted", w.Code)
	}
	w = call("POST", "/api/teams/"+team.ID.String()+"/leave", `{"expected_revision":"1"}`)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "last_team_administrator") {
		t.Fatal("last administrator left team", w.Code)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=? WHERE id=?`, now, session.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if w := call("GET", "/api/teams/"+team.ID.String(), ""); w.Code != 401 {
		t.Fatal("revoked session retained team access", w.Code)
	}
}
