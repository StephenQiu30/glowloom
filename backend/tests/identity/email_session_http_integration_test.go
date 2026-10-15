package identity_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestEmailSessionHTTPCompleteReceiptAndLegacyProofRejection(t *testing.T) {
	tx := existingIdentityTransaction(t)
	auth, _ := verifiedTestActor(t, tx, "session-contract@example.invalid", time.Now().UTC())
	const origin = "http://127.0.0.1:3200"
	router := gin.New()
	identityhttp.NewEmailAuthHandler(auth, origin, false).Register(router.Group("/api"))
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/sessions", strings.NewReader(`{"email":"session-contract@example.invalid","password":"long password for tests","persistent":false}`))
	request.Header.Set("Origin", origin)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", uuid.NewString())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("email login status = %d", response.Code)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("login did not issue the existing protected session cookie")
	}
	var login identityhttp.EmailSessionView
	if err := json.Unmarshal(response.Body.Bytes(), &login); err != nil {
		t.Fatal("decode email session receipt")
	}
	read := func() *httptest.ResponseRecorder {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/session", nil)
		request.AddCookie(cookies[0])
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	current := read()
	if current.Code != http.StatusOK || current.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("current session did not return an uncached success receipt")
	}
	var receipt map[string]json.RawMessage
	if err := json.Unmarshal(current.Body.Bytes(), &receipt); err != nil {
		t.Fatal("decode current session receipt")
	}
	for _, key := range []string{"actor", "session_id", "last_active_at", "absolute_expires_at", "persistent", "credential_epoch", "csrf_token"} {
		if len(receipt[key]) == 0 || string(receipt[key]) == "null" {
			t.Errorf("session receipt omitted %s", key)
		}
	}
	var session identityhttp.EmailSessionView
	if err := json.Unmarshal(current.Body.Bytes(), &session); err != nil {
		t.Fatal("decode typed current session receipt")
	}
	if session.SessionID != login.SessionID || session.Persistent || session.CSRFToken == "" || session.CSRFToken != login.CSRFToken || session.CredentialEpoch == "" || session.Actor.Email != "session-contract@example.invalid" || session.Actor.Status != "active" || session.Actor.Revision == "" || session.Actor.Capabilities == nil {
		t.Fatal("current session does not satisfy the email frontend contract")
	}
	// A pre-cutover cookie must fail closed instead of producing partial success.
	if err := tx.Exec("UPDATE identity.user_session SET csrf_hash=NULL WHERE id=?", login.SessionID).Error; err != nil {
		t.Fatal("prepare rollback-only legacy session")
	}
	if legacy := read(); legacy.Code != http.StatusUnauthorized {
		t.Fatalf("legacy session status = %d, want 401", legacy.Code)
	}
}

// This changes only a synthetic account inside an existing-database rollback transaction.
func TestVoluntaryEmailPasswordEightCharactersRotatesSession(t *testing.T) {
	tx := existingIdentityTransaction(t)
	auth, actor := verifiedTestActor(t, tx, "password-eight@example.invalid", time.Now().UTC())
	if err := tx.Exec(`UPDATE identity."user" SET must_change_password=true WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal("prepare rollback-only restricted account")
	}
	login, err := auth.Login(t.Context(), "password-eight@example.invalid", "long password for tests", false)
	if err != nil {
		t.Fatal("login restricted fixture")
	}
	const origin = "http://127.0.0.1:3200"
	router := gin.New()
	identityhttp.NewEmailAuthHandler(auth, origin, false).Register(router.Group("/api"))
	input := identityhttp.EmailPasswordChangeRequest{Epoch: strconv.FormatInt(login.User.SessionEpoch, 10), Current: "long password for tests", New: "eightPwd", Confirmation: "eightPwd"}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/me/password", bytes.NewReader(body))
	request.AddCookie(&http.Cookie{Name: "lanverse_session", Value: login.Token})
	request.Header.Set("Origin", origin)
	request.Header.Set("X-CSRF-Token", login.CSRFToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", uuid.NewString())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("eight-character password update status = %d", response.Code)
	}
	var receipt identityhttp.EmailSessionView
	if err := json.Unmarshal(response.Body.Bytes(), &receipt); err != nil {
		t.Fatal("decode password update receipt")
	}
	if receipt.CredentialEpoch == input.Epoch || receipt.CSRFToken == "" || receipt.SessionID == login.Session.ID {
		t.Fatal("password update did not release restriction and rotate the session")
	}
	var restricted bool
	if err := tx.Raw(`SELECT must_change_password FROM identity."user" WHERE id=?`, actor.ID).Scan(&restricted).Error; err != nil || restricted {
		t.Fatal("password update did not persist unrestricted state")
	}
	if _, err := auth.Current(t.Context(), login.Token); err == nil {
		t.Fatal("old session survived password change")
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly {
		t.Fatal("password change did not issue protected replacement cookie")
	}
	current, err := auth.Current(t.Context(), cookies[0].Value)
	if err != nil || current.User.MustChangePassword {
		t.Fatal("replacement session remains restricted")
	}
	if _, err := auth.Login(t.Context(), "password-eight@example.invalid", "eightPwd", false); err != nil {
		t.Fatal("eight-character replacement password cannot log in")
	}
}

func TestHistoricalPasswordFlagDoesNotRestrictSessionDirectory(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC()
	auth, actor := verifiedTestActor(t, tx, "restricted-logout@example.invalid", now)
	if err := tx.Exec(`UPDATE identity."user" SET must_change_password=true WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	login, err := auth.Login(t.Context(), "restricted-logout@example.invalid", "long password for tests", false)
	if err != nil {
		t.Fatal(err)
	}
	other, err := auth.Login(t.Context(), "restricted-logout@example.invalid", "long password for tests", false)
	if err != nil {
		t.Fatal(err)
	}
	directory := identityhttp.NewSessionDirectoryHandler(identityapp.NewSessionDirectory(pgidentity.NewSessionDirectoryStore(tx), func() time.Time { return now }), false)
	router := gin.New()
	const origin = "http://127.0.0.1:3200"
	identityhttp.NewEmailAuthHandler(auth, origin, false).WithSessionRevocation(directory.Revoke).Register(router.Group("/api"))
	revoke := func(id uuid.UUID, proof string) *httptest.ResponseRecorder {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/api/sessions/"+id.String(), nil)
		request.AddCookie(&http.Cookie{Name: "lanverse_session", Value: login.Token})
		request.Header.Set("Origin", origin)
		request.Header.Set("X-CSRF-Token", proof)
		request.Header.Set("Idempotency-Key", uuid.NewString())
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	if response := revoke(login.Session.ID, ""); response.Code != http.StatusForbidden {
		t.Fatalf("logout without CSRF=%d", response.Code)
	}
	if response := revoke(other.Session.ID, login.CSRFToken); response.Code != http.StatusNoContent {
		t.Fatalf("restricted actor revoked another session=%d", response.Code)
	}
	response := revoke(login.Session.ID, login.CSRFToken)
	if response.Code != http.StatusNoContent {
		t.Fatalf("restricted self logout=%d, want 204", response.Code)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge >= 0 {
		t.Fatal("logout did not expire this browser cookie")
	}
	if _, err := auth.Current(t.Context(), login.Token); err == nil {
		t.Fatal("logout kept current session alive")
	}
	if _, err := auth.Current(t.Context(), other.Token); err == nil {
		t.Fatal("explicit revocation left other device active")
	}
}
