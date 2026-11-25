package identity_test

import (
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identitymail "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/mail"
	pg "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestEmailChangeHTTPContract(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	auth, a := verifiedTestActor(t, tx, "change-http@example.invalid", now)
	tx.Exec(`UPDATE identity.user_session SET reauth_at=?,reauth_epoch=? WHERE id=?`, now, a.CredentialEpoch, a.SessionID)
	var rev int64
	tx.Raw(`SELECT revision FROM identity."user" WHERE id=?`, a.ID).Scan(&rev)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	group := r.Group("/api", func(c *gin.Context) { c.Set("principal", a) })
	identityhttp.NewEmailChangeHandler(app.NewEmailChange(auth, pg.NewEmailChangeStore(tx), func() time.Time { return now }), false).Register(group)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	w := call("POST", "/api/me/email-change", `{"expected_revision":"`+strconv.FormatInt(rev, 10)+`","new_email":"change-http-new@example.invalid","role":"platform_admin"}`)
	if w.Code != 422 {
		t.Fatal("unknown field accepted", w.Code)
	}
	w = call("POST", "/api/me/email-change", `{"expected_revision":"`+strconv.FormatInt(rev, 10)+`","new_email":"change-http-new@example.invalid"}`)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var start identityhttp.AcceptedView
	if err := json.Unmarshal(w.Body.Bytes(), &start); err != nil {
		t.Fatal(err)
	}
	op := start.OperationID
	tx.Exec(`UPDATE identity.mail_delivery SET status='submitted' WHERE challenge_id IN (SELECT id FROM identity.auth_challenge WHERE operation_id=?)`, op)
	for _, side := range []string{"old", "new"} {
		w = call("POST", "/api/me/email-change/"+op.String()+"/verify", `{"side":"`+side+`","code":"`+changeCode(t, tx, op, side)+`"}`)
		if w.Code != 204 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	w = call("POST", "/api/me/email-change/"+op.String()+"/complete", `{"expected_revision":"`+strconv.FormatInt(rev, 10)+`"}`)
	if w.Code != 204 || len(w.Result().Cookies()) == 0 || w.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatal("completion/cookie", w.Code, w.Body.String())
	}
}
func TestEmailChangeMailTemplates(t *testing.T) {
	sender, err := identitymail.NewSender(identitymail.SMTPConfig{Host: "smtp.example.invalid", Port: 465, TLSMode: "tls", FromEmail: "noreply@example.invalid", PublicOrigin: "https://example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	for _, side := range []string{"email_old", "email_new"} {
		if _, err := sender.Message(uuid.New(), side, "creator@example.invalid", []byte(`{"code":"123456"}`)); err != nil {
			t.Fatal(err)
		}
		if _, err := sender.Message(uuid.New(), side, "creator@example.invalid", []byte(`{"code":"12345"}`)); err == nil {
			t.Fatal("bad code rendered")
		}
	}
	if _, err := sender.Message(uuid.New(), "email_changed", "creator@example.invalid", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
}
