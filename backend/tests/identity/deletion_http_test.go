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

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identitymail "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/mail"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestDeletionHTTPRequiresConfirmation(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	auth, _ := verifiedTestActor(t, tx, "deletion-http@example.invalid", now)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := identityhttp.NewEmailAuthHandler(auth, "http://localhost:3200", false)
	h.Register(r.Group("/api"))
	identityhttp.NewAccountDeletionHandler(identity.NewAccountDeletion(auth, pgidentity.NewAccountDeletionStore(tx), func() time.Time { return now }), false).Register(r.Group("/api", h.RequireSession()))
	var cookie *http.Cookie
	csrf := ""
	call := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), "POST", path, strings.NewReader(body))
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
	w := call("/api/sessions", `{"email":"deletion-http@example.invalid","password":"long password for tests"}`)
	if w.Code != 201 {
		t.Fatal(w.Code)
	}
	cookie = w.Result().Cookies()[0]
	var session identityhttp.EmailSessionView
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	csrf = session.CSRFToken
	if w := call("/api/me/deletion/challenge", `{}`); w.Code != 403 || !strings.Contains(w.Body.String(), "reauthentication_required") {
		t.Fatalf("recent proof missing: %d %s", w.Code, w.Body.String())
	}
	if w := call("/api/me/reauthentication", `{"password":"long password for tests"}`); w.Code != 204 {
		t.Fatal(w.Code)
	}
	w = call("/api/me/deletion/preview", `{}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "soft-delete-v1") {
		t.Fatal(w.Code)
	}
	var impact struct {
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &impact); err != nil {
		t.Fatal(err)
	}
	w = call("/api/me/deletion/challenge", `{}`)
	if w.Code != 202 {
		t.Fatal(w.Code)
	}
	var start identityhttp.AcceptedView
	if err := json.Unmarshal(w.Body.Bytes(), &start); err != nil {
		t.Fatal(err)
	}
	code := deletionCode(t, tx, start.OperationID)
	body := `{"expected_revision":"` + impact.Revision + `","operation_id":"` + start.OperationID.String() + `","code":"` + code + `","policy_version":"soft-delete-v1"}`
	oldCSRF := csrf
	csrf = ""
	if w := call("/api/me/deletion", body); w.Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
	csrf = oldCSRF
	w = call("/api/me/deletion", body)
	if w.Code != 204 {
		t.Fatalf("deletion failed %d %s", w.Code, w.Body.String())
	}
	if len(w.Result().Cookies()) == 0 || w.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatal("session cookie not cleared")
	}
}

func TestDeletionMailDescribesSoftDeletion(t *testing.T) {
	sender, err := identitymail.NewSender(identitymail.SMTPConfig{Host: "smtp.example.invalid", Port: 465, TLSMode: "tls", FromEmail: "noreply@example.invalid", PublicOrigin: "https://example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sender.Message(uuid.New(), "deletion_confirm", "creator@example.invalid", []byte(`{"code":"123456"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := sender.Message(uuid.New(), "deletion_confirm", "creator@example.invalid", []byte(`{"code":"12345"}`)); err == nil {
		t.Fatal("invalid code accepted")
	}
}
