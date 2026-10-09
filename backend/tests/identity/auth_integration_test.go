package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	pgidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

func TestAuthRegistrationSessionPasswordAndFailureTransactions(t *testing.T) {
	dsn := os.Getenv("LV_TEST_AUTH_DB_DSN")
	if dsn == "" {
		t.Skip("requires a disposable database initialized from schema.sql")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	owner, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close() }()
	runtimeURL, err := url.Parse(dsn)
	if err != nil || (runtimeURL.Scheme != "postgres" && runtimeURL.Scheme != "postgresql") {
		t.Fatal("auth fixture requires a PostgreSQL URI")
	}
	query := runtimeURL.Query()
	query.Set("options", "-c role=lanverse_app")
	runtimeURL.RawQuery = query.Encode()
	conn, err := db.Open(ctx, runtimeURL.String(), noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	now := time.Date(2026, 10, 9, 4, 0, 0, 0, time.UTC)
	var runtimeRole string
	if err := conn.DB.Raw("SELECT current_user").Scan(&runtimeRole).Error; err != nil || runtimeRole != "lanverse_app" {
		t.Fatal("actual runtime role unavailable", err)
	}
	auth := identityapp.NewAuth(pgidentity.NewStore(conn.DB), func() time.Time { return now })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3200"))
	handler := identityhttp.NewAuthHandler(auth, false)
	handler.Register(router.Group("/api"))
	protected := router.Group("/api", handler.RequireSession())
	protected.GET("/private", func(c *gin.Context) { c.Status(204) })
	protected.GET("/admin/test", func(c *gin.Context) { c.Status(204) })
	request := func(method, path, body string, cookie *http.Cookie, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://localhost:3200")
		r.Header.Set("Idempotency-Key", uuid.NewString())
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: %d want %d: %s", method, path, w.Code, status, w.Body.String())
		}
		return w
	}
	view := func(w *httptest.ResponseRecorder) identityhttp.SessionView {
		t.Helper()
		var s identityhttp.SessionView
		if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(w.Body.String(), "token") || strings.Contains(w.Body.String(), "password_hash") {
			t.Fatal("secret in response")
		}
		return s
	}
	registration := `{"login_name":" Alice._- ","display_name":"Alice","password":"FirstPassword123","confirm_password":"FirstPassword123"}`
	w := request("POST", "/api/accounts/register", registration, nil, 201)
	session := view(w)
	cookie := w.Result().Cookies()[0]
	if session.Actor.LoginName != "alice._-" || session.Actor.Role != "creator" || session.Actor.MustChangePassword || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || !cookie.Expires.IsZero() {
		t.Fatal("registration/cookie policy")
	}
	request("GET", "/api/session", "", cookie, 200)
	request("GET", "/api/private", "", nil, 401)
	request("GET", "/api/private", "", cookie, 204)
	request("GET", "/api/admin/test", "", cookie, 403)
	request("POST", "/api/accounts/register", registration, nil, 422)
	request("POST", "/api/accounts/register", strings.Replace(registration, `"display_name"`, `"role":"admin","display_name"`, 1), nil, 422)
	request("GET", "/api/accounts/availability?login_name=ALICE._-", "", nil, 200)
	w = request("POST", "/api/sessions", `{"login_name":"alice._-","password":"FirstPassword123","persistent":true}`, nil, 201)
	other := w.Result().Cookies()[0]
	if other.Expires.IsZero() {
		t.Fatal("persistent cookie missing expiry")
	}
	request("DELETE", "/api/sessions/"+uuid.NewString(), "", cookie, 403)
	request("POST", "/api/me/password", `{"expected_credential_revision":2,"current_password":"FirstPassword123","new_password":"SecondPassword123","confirm_password":"SecondPassword123"}`, cookie, 409)
	request("POST", "/api/me/password", `{"expected_credential_revision":1,"current_password":"incorrect","new_password":"SecondPassword123","confirm_password":"SecondPassword123"}`, cookie, 422)
	w = request("POST", "/api/me/password", `{"expected_credential_revision":1,"current_password":"FirstPassword123","new_password":"SecondPassword123","confirm_password":"SecondPassword123"}`, cookie, 200)
	changed := view(w)
	fresh := w.Result().Cookies()[0]
	if changed.Actor.CredentialRevision != 2 || fresh.Value == cookie.Value {
		t.Fatal("credential/session rotation")
	}
	request("GET", "/api/session", "", cookie, 401)
	request("GET", "/api/session", "", other, 401)
	request("GET", "/api/session", "", fresh, 200)
	for range 5 {
		request("POST", "/api/sessions", `{"login_name":"alice._-","password":"wrong"}`, nil, 401)
	}
	request("POST", "/api/sessions", `{"login_name":"alice._-","password":"SecondPassword123"}`, nil, 401)
	now = now.Add(15 * time.Minute)
	w = request("POST", "/api/sessions", `{"login_name":"ALICE._-","password":"SecondPassword123"}`, nil, 201)
	fresh = w.Result().Cookies()[0]
	current := view(w)
	if err := owner.DB.WithContext(ctx).Exec(`UPDATE identity."user" SET must_change_password=true WHERE id=?::uuid`, current.Actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	request("GET", "/api/session", "", fresh, 200)
	request("GET", "/api/private", "", fresh, 403)
	w = request("POST", "/api/me/password", `{"expected_credential_revision":2,"current_password":"SecondPassword123","new_password":"ThirdPassword123","confirm_password":"ThirdPassword123"}`, fresh, 200)
	fresh = w.Result().Cookies()[0]
	current = view(w)
	request("GET", "/api/private", "", fresh, 204)
	request("DELETE", "/api/sessions/"+current.SessionID, "", fresh, 204)
	request("DELETE", "/api/sessions/"+current.SessionID, "", fresh, 204)
	request("GET", "/api/session", "", fresh, 401)
	w = request("POST", "/api/sessions", `{"login_name":"alice._-","password":"ThirdPassword123"}`, nil, 201)
	fresh = w.Result().Cookies()[0]
	now = now.Add(12 * time.Hour)
	request("GET", "/api/session", "", fresh, 401)
	w = request("POST", "/api/sessions", `{"login_name":"alice._-","password":"ThirdPassword123"}`, nil, 201)
	fresh = w.Result().Cookies()[0]
	current = view(w)
	if err := owner.DB.WithContext(ctx).Exec(`UPDATE identity.user_session SET created_at=?,last_active_at=?,absolute_expires_at=? WHERE id=?::uuid`, now.Add(-7*24*time.Hour), now, now, current.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	request("GET", "/api/session", "", fresh, 401)
	w = request("POST", "/api/sessions", `{"login_name":"alice._-","password":"ThirdPassword123"}`, nil, 201)
	fresh = w.Result().Cookies()[0]
	if err := owner.DB.WithContext(ctx).Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?::uuid`, current.Actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	request("GET", "/api/session", "", fresh, 401)
	request("POST", "/api/sessions", `{"login_name":"alice._-","password":"ThirdPassword123"}`, nil, 401)
	request("POST", "/api/sessions", `{"login_name":"missing","password":"wrong"}`, nil, 401)
	// Concurrent registration must have one winner and one durable session.
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, err := auth.Register(ctx, "race", "Race", "RacePassword123", "RacePassword123")
			outcomes <- err
		})
	}
	wg.Wait()
	close(outcomes)
	successes := 0
	for err := range outcomes {
		if err == nil {
			successes++
		} else if !errors.Is(err, identityapp.ErrLoginTaken) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("registration winners: %d", successes)
	}
	second, err := auth.Register(ctx, "second", "Second", "SecondAccount123", "SecondAccount123")
	if err != nil {
		t.Fatal(err)
	}
	secondCookie := &http.Cookie{Name: "lanverse_session", Value: second.Token}
	raceSession, err := auth.Login(ctx, "race", "RacePassword123", false)
	if err != nil {
		t.Fatal(err)
	}
	raceCookie := &http.Cookie{Name: "lanverse_session", Value: raceSession.Token}
	request("DELETE", "/api/sessions/"+second.Session.ID.String(), "", raceCookie, 403)
	request("GET", "/api/session", "", secondCookie, 200)
	// Same-login attempts serialize without losing failure timestamps.
	for range 5 {
		wg.Go(func() {
			_, err := auth.Login(ctx, "second", "wrong", false)
			if !errors.Is(err, identityapp.ErrInvalidCredentials) {
				t.Errorf("concurrent failed login: %v", err)
			}
		})
	}
	wg.Wait()
	var failures int
	if err := owner.DB.WithContext(ctx).Raw(`SELECT cardinality(failure_times) FROM identity.login_guard WHERE login_key='second'`).Scan(&failures).Error; err != nil || failures != 5 {
		t.Fatalf("concurrent failures=%d: %v", failures, err)
	}
	if _, err := auth.Login(ctx, "second", "SecondAccount123", false); !errors.Is(err, identityapp.ErrInvalidCredentials) {
		t.Fatalf("concurrent lock not persisted: %v", err)
	}
	// Concurrent password changes from the same revision admit only one winner.
	changes := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, err := auth.ChangePassword(ctx, raceSession.Token, 1, "RacePassword123", "RaceChanged123", "RaceChanged123")
			changes <- err
		})
	}
	wg.Wait()
	close(changes)
	winners := 0
	for err := range changes {
		if err == nil {
			winners++
		} else if !errors.Is(err, identityapp.ErrSessionInvalid) && !errors.Is(err, identitydomain.ErrRevisionConflict) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("password change winners=%d", winners)
	}
	// An event failure must roll back the account and session.
	if err := owner.DB.WithContext(ctx).Exec(`CREATE FUNCTION identity.reject_auth_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test event failure'; END $$; CREATE TRIGGER reject_auth_event BEFORE INSERT ON identity.auth_event FOR EACH ROW EXECUTE FUNCTION identity.reject_auth_event()`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Register(ctx, "rollback", "Rollback", "RollbackPassword123", "RollbackPassword123"); err == nil {
		t.Fatal("event failure was ignored")
	}
	var count int64
	if err := owner.DB.WithContext(ctx).Raw(`SELECT count(*) FROM identity."user" WHERE login_name='rollback'`).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("account survived failed transaction", err)
	}
	if err := owner.DB.WithContext(ctx).Exec(`DROP TRIGGER reject_auth_event ON identity.auth_event; DROP FUNCTION identity.reject_auth_event()`).Error; err != nil {
		t.Fatal(err)
	}
	var secretCount int64
	if err := owner.DB.WithContext(ctx).Raw(`SELECT count(*) FROM identity.user_session WHERE octet_length(token_hash) <> 32`).Scan(&secretCount).Error; err != nil || secretCount != 0 {
		t.Fatal("session digest", err)
	}
	t.Log("real PostgreSQL registration, concurrent uniqueness, sessions, password rotation, revocation, lock/expiry boundaries and rollback passed")
}
