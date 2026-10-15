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

func TestGovernanceHTTPBoundary(t *testing.T) {
	tx, svc, actor, now := governanceFixture(t)
	protector, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	auth := identity.NewEmailAuth(pgidentity.NewStore(tx), protector, identity.EmailPolicy{TermsVersion: "test", SourceHourlyLimit: 100, GlobalHourlyLimit: 1000}, func() time.Time { return now })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := identityhttp.NewEmailAuthHandler(auth, "http://localhost:3200", false)
	h.Register(router.Group("/api"))
	protected := router.Group("/api", h.RequireSession())
	identityhttp.NewGovernanceHandler(svc).Register(protected)
	identityhttp.NewInvitationHandler(svc).Register(protected)
	login := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/sessions", strings.NewReader(`{"email":"governance-admin@example.invalid","password":"long password for tests"}`))
	login.Header.Set("Origin", "http://localhost:3200")
	login.Header.Set("Idempotency-Key", uuid.NewString())
	login.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, login)
	if w.Code != 201 {
		t.Fatal("login failed", w.Code)
	}
	cookie := w.Result().Cookies()[0]
	var session struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET reauth_at=?,reauth_epoch=? WHERE account_id=?`, now, actor.CredentialEpoch, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body, csrf string) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
		r.AddCookie(cookie)
		r.Header.Set("Origin", "http://localhost:3200")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	if w := request(http.MethodPatch, "/api/platform/settings", `{"expected_revision":"1","registration_enabled":false,"reason":"Close registration"}`, ""); w.Code != 403 {
		t.Fatal("write without CSRF accepted", w.Code)
	}
	if w := request(http.MethodPatch, "/api/platform/settings", `{"expected_revision":1,"registration_enabled":false,"reason":"Close registration"}`, session.CSRFToken); w.Code != 422 {
		t.Fatal("numeric revision accepted", w.Code)
	}
	if w := request(http.MethodPatch, "/api/platform/settings", `{"expected_revision":"1","registration_enabled":false,"reason":"Close registration","platform_role":"platform_admin"}`, session.CSRFToken); w.Code != 422 {
		t.Fatal("unknown field accepted", w.Code)
	}
	w = request(http.MethodPatch, "/api/platform/settings", `{"expected_revision":"1","registration_enabled":false,"reason":"Close registration"}`, session.CSRFToken)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("settings failed", w.Code)
	}
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["revision"] != "2" {
		t.Fatal("revision is not canonical string")
	}
	_, target := verifiedTestActor(t, tx, "cancel-proposal-target@example.invalid", now)
	var targetRevision int64
	if err := tx.Raw(`SELECT revision FROM identity."user" WHERE id=?`, target.ID).Scan(&targetRevision).Error; err != nil {
		t.Fatal(err)
	}
	proposal, err := svc.ProposeAdmin(t.Context(), actor, target.ID, targetRevision, "Original grant rationale")
	if err != nil {
		t.Fatal(err)
	}
	cancelPath := "/api/platform/admin-grants/" + proposal.ID.String() + "/cancel"
	for _, body := range []string{`{"expected_revision":"1"}`, `{"expected_revision":"1","reason":"    "}`, `{"expected_revision":"1","reason":"tiny"}`, `{"expected_revision":"1","reason":"contains\ncontrol"}`, `{"expected_revision":"1","reason":"` + strings.Repeat("界", 501) + `"}`} {
		if w := request(http.MethodPost, cancelPath, body, session.CSRFToken); w.Code != 422 {
			t.Fatal("invalid cancellation reason accepted", w.Code)
		}
	}
	if w := request(http.MethodPost, cancelPath, `{"expected_revision":"1","reason":"Withdraw after access review"}`, session.CSRFToken); w.Code != 204 {
		t.Fatal("valid cancellation Mutation rejected", w.Code)
	}
	var auditReason string
	if err := tx.Raw(`SELECT reason FROM identity.security_event WHERE action='authority.cancelled' AND target_user_id=?`, target.ID).Scan(&auditReason).Error; err != nil || auditReason != "Withdraw after access review" {
		t.Fatal("cancellation did not audit current reason", err)
	}
	var originalReason string
	if err := tx.Raw(`SELECT reason FROM identity.authority_proposal WHERE id=?`, proposal.ID).Scan(&originalReason).Error; err != nil || originalReason != "Original grant rationale" {
		t.Fatal("cancellation overwrote proposal rationale", err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='creator',platform_epoch=platform_epoch+1 WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if w := request(http.MethodGet, "/api/platform/settings", "", ""); w.Code != 403 {
		t.Fatal("revoked platform authority survived", w.Code)
	}
}
