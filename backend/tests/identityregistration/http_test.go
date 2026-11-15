package identityregistration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/http"
)

func TestRegistrationPostgresHTTPPreservesOriginalFlowAcrossUnknown(t *testing.T) {
	f := newRegistrationFixture(t)
	input := f.input(t)
	engine := gin.New()
	identityhttp.NewEmailAuthHandler(f.auth, "http://registration.invalid", false).Register(engine.Group("/api"))
	body, err := json.Marshal(identityhttp.RegistrationCompleteRequest{Grant: input.Grant, DisplayName: input.DisplayName, Password: input.Password, Confirmation: input.ConfirmPassword, TermsVersion: input.TermsVersion, TermsAccepted: true})
	if err != nil {
		t.Fatal(err)
	}
	send := func(origin string, cookie bool) *httptest.ResponseRecorder {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://registration.invalid/api/registrations/"+input.OperationID.String()+"/complete", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", origin)
		request.Header.Set("Idempotency-Key", input.OperationID.String())
		if cookie {
			request.AddCookie(&http.Cookie{Name: "glowloom_registration_flow", Value: input.OperationID.String() + "." + input.FlowSecret})
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response
	}
	f.wallet.lose.Store(true)
	unknown := send("http://registration.invalid", true)
	if unknown.Code < 500 || len(unknown.Result().Cookies()) != 0 || strings.Contains(unknown.Body.String(), "preparing") {
		t.Fatalf("unknown changed public contract: status=%d cookies=%d", unknown.Code, len(unknown.Result().Cookies()))
	}
	if n := count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE id IN(SELECT account_id FROM identity.registration_preparation WHERE operation_id=?)`, input.OperationID); n != 0 {
		t.Fatal("HTTP unknown exposed account")
	}
	if response := send("http://attacker.invalid", true); response.Code != 403 {
		t.Fatalf("Origin=%d", response.Code)
	}
	if response := send("http://registration.invalid", false); response.Code < 400 {
		t.Fatal("missing original flow proof accepted")
	}
	success := send("http://registration.invalid", true)
	if success.Code != http.StatusCreated {
		t.Fatalf("recover HTTP=%d", success.Code)
	}
	var result identityhttp.RegistrationCompleteView
	if err := json.Unmarshal(success.Body.Bytes(), &result); err != nil || result.AccountID == uuid.Nil || result.Next != "login" {
		t.Fatalf("public view changed: %v", err)
	}
	cookies := success.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "glowloom_registration_flow" || cookies[0].MaxAge != -1 || !cookies[0].HttpOnly || cookies[0].Path != "/api/registrations" {
		t.Fatal("successful completion changed original cookie contract")
	}
	replay := send("http://registration.invalid", true)
	if replay.Code != http.StatusCreated {
		t.Fatalf("lost activation reply original proof=%d", replay.Code)
	}
	var repeated identityhttp.RegistrationCompleteView
	if err := json.Unmarshal(replay.Body.Bytes(), &repeated); err != nil || repeated.AccountID != result.AccountID {
		t.Fatal("original activation reply changed identity")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.user_session WHERE account_id=?`, result.AccountID) != 0 {
		t.Fatal("HTTP replay created a session")
	}
}
