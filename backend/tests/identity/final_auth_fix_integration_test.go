package identity_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// All writes below use the existing rollback harness, including nested auth
// transactions. No sender, external object store, or new database is used.
func finalAuthFixture(t *testing.T) (*gorm.DB, *identity.Auth, *credentialproof.Protector, identity.AuthResult, *time.Time) {
	t.Helper()
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, actor := verifiedTestActor(t, tx, "final-auth@example.invalid", now)
	protector, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	auth := identity.NewEmailAuth(pgidentity.NewStore(tx), protector, identity.EmailPolicy{}, func() time.Time { return now })
	login, err := auth.Login(t.Context(), "final-auth@example.invalid", "long password for tests", true)
	if err != nil || login.User.ID != actor.ID {
		t.Fatalf("fixture login: %v", err)
	}
	return tx, auth, protector, login, &now
}

func finalAuthRequest(t *testing.T, router *gin.Engine, path, peer, forwarded string, payload any, login identity.AuthResult) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, bytes.NewReader(raw))
	request.RemoteAddr = peer
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost:3200")
	request.Header.Set("Idempotency-Key", uuid.NewString())
	request.Header.Set("X-Forwarded-For", forwarded)
	request.Header.Set("X-Real-IP", forwarded)
	request.Header.Set("Forwarded", "for="+forwarded)
	if login.Token != "" {
		request.AddCookie(&http.Cookie{Name: "lanverse_session", Value: login.Token})
		request.Header.Set("X-CSRF-Token", login.CSRFToken)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func finalAuthRouter(auth *identity.Auth) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	identityhttp.NewEmailAuthHandler(auth, "http://localhost:3200", false).Register(router.Group("/api"))
	return router
}

func TestFinalPasswordChangeQueuesAtomicNotice(t *testing.T) {
	tx, auth, protector, login, now := finalAuthFixture(t)
	response := finalAuthRequest(t, finalAuthRouter(auth), "/api/me/password", "127.0.0.1:1234", "", map[string]any{
		"expected_credential_epoch": fmt.Sprint(login.User.SessionEpoch), "current_password": "long password for tests", "new_password": "new long password", "confirm_password": "new long password",
	}, login)
	if response.Code != 200 {
		t.Fatalf("change status = %d: %s", response.Code, response.Body)
	}
	var mails []struct {
		EventID          uuid.UUID
		KeyID            string
		EncryptedPayload []byte
		ExpiresAt        time.Time
	}
	if err := tx.Raw(`SELECT event_id,key_id,encrypted_payload,expires_at FROM identity.mail_delivery WHERE template='password_changed' AND recipient=?`, login.User.Email).Scan(&mails).Error; err != nil {
		t.Fatal(err)
	}
	if len(mails) != 1 {
		t.Fatalf("password change queued %d notices, want exactly one", len(mails))
	}
	plain, err := protector.Open(mails[0].EventID, mails[0].KeyID, mails[0].EncryptedPayload)
	if err != nil || string(plain) != "{}" || !mails[0].ExpiresAt.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("invalid encrypted notification: %v", err)
	}
	if _, err := auth.Current(t.Context(), login.Token); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("old session survived rotation")
	}
	if _, err := auth.ChangePassword(t.Context(), login.Token, login.User.SessionEpoch, "long password for tests", "new long password", "new long password"); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("stale replay = %v", err)
	}
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM identity.mail_delivery WHERE template='password_changed' AND recipient=?`, login.User.Email).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("replay duplicated notice: %d %v", count, err)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("rotated cookie absent")
	}
	current, err := auth.Current(t.Context(), cookies[0].Value)
	if err != nil || current.User.SessionEpoch != login.User.SessionEpoch+1 || !current.Session.Persistent {
		t.Fatalf("rotation invalid: %v", err)
	}
}

type finalMailFailureStore struct {
	identity.EmailStore
	err   error
	calls int
}
type finalMailFailureTx struct {
	identity.EmailTx
	store *finalMailFailureStore
}

func (s *finalMailFailureStore) EmailTransaction(ctx context.Context, run func(identity.EmailTx) error) error {
	return s.EmailStore.EmailTransaction(ctx, func(tx identity.EmailTx) error { return run(&finalMailFailureTx{EmailTx: tx, store: s}) })
}
func (tx *finalMailFailureTx) EnqueueMail(identity.QueuedMail) error {
	tx.store.calls++
	return tx.store.err
}

func TestFinalPasswordChangeNoticeFailureRollsBack(t *testing.T) {
	tx, auth, protector, login, now := finalAuthFixture(t)
	failed := &finalMailFailureStore{EmailStore: pgidentity.NewStore(tx), err: errors.New("injected queue failure")}
	failingAuth := identity.NewEmailAuth(failed, protector, identity.EmailPolicy{}, func() time.Time { return *now })
	result, err := failingAuth.ChangePassword(t.Context(), login.Token, login.User.SessionEpoch, "long password for tests", "new long password", "new long password")
	if !errors.Is(err, failed.err) || failed.calls != 1 {
		t.Fatalf("queue failure not reached/returned: calls=%d err=%v", failed.calls, err)
	}
	if result.Token != "" {
		t.Fatal("rolled back rotation exposed a token")
	}
	current, err := auth.Current(t.Context(), login.Token)
	if err != nil || current.User.PasswordHash != login.User.PasswordHash || current.User.SessionEpoch != login.User.SessionEpoch || current.User.Revision != login.User.Revision {
		t.Fatalf("failed notice partially changed credentials: %v", err)
	}
	var counts struct{ Sessions, Events, Mails int64 }
	if err := tx.Raw(`SELECT (SELECT count(*) FROM identity.user_session WHERE account_id=?) AS sessions,(SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action='password.changed') AS events,(SELECT count(*) FROM identity.mail_delivery WHERE recipient=?) AS mails`, login.User.ID, login.User.ID, login.User.Email).Scan(&counts).Error; err != nil {
		t.Fatal(err)
	}
	if counts.Sessions != 2 || counts.Events != 0 || counts.Mails != 0 {
		t.Fatalf("partial rotation state: %+v", counts)
	}
}

func TestFinalPasswordProofGuardSharedAcrossEndpoints(t *testing.T) {
	for _, direction := range []string{"reauth-to-change", "change-to-reauth"} {
		t.Run(direction, func(t *testing.T) {
			tx, auth, protector, login, now := finalAuthFixture(t)
			store := pgidentity.NewStore(tx)
			key := hex.EncodeToString(protector.Digest("reauth_guard", login.User.ID, ""))
			failChange := func(password string) error {
				_, err := auth.ChangePassword(t.Context(), login.Token, login.User.SessionEpoch, password, "new long password", "new long password")
				return err
			}
			for i := 0; i < 5; i++ {
				var err error
				if direction == "reauth-to-change" {
					err = auth.Reauthenticate(t.Context(), login.Token, "wrong")
				} else {
					err = failChange("wrong")
				}
				if err == nil {
					t.Fatal("wrong password accepted")
				}
			}
			var guard domain.LoginGuard
			if err := store.EmailTransaction(t.Context(), func(tx identity.EmailTx) error { var err error; guard, err = tx.Guard(key); return err }); err != nil {
				t.Fatal(err)
			}
			if !guard.Locked(*now) || len(guard.Failures) != 5 {
				t.Fatalf("failure guard rolled back: %+v", guard)
			}
			lockedUntil := guard.LockedUntil
			*now = now.Add(14 * time.Minute)
			if err := auth.Reauthenticate(t.Context(), login.Token, "long password for tests"); !errors.Is(err, identity.ErrInvalidCredentials) {
				t.Fatalf("reauth bypassed lock: %v", err)
			}
			if err := failChange("long password for tests"); !errors.Is(err, identity.ErrCurrentPassword) {
				t.Fatalf("change bypassed lock: %v", err)
			}
			if err := store.EmailTransaction(t.Context(), func(tx identity.EmailTx) error { var err error; guard, err = tx.Guard(key); return err }); err != nil {
				t.Fatal(err)
			}
			if !guard.LockedUntil.Equal(lockedUntil) || len(guard.Failures) != 5 {
				t.Fatal("locked attempt renewed lock")
			}
			var events int64
			if err := tx.Raw(`SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action='authentication.failed'`, login.User.ID).Scan(&events).Error; err != nil || events != 7 {
				t.Fatalf("failed proof audit not committed: %d %v", events, err)
			}
			*now = lockedUntil
			if err := auth.Reauthenticate(t.Context(), login.Token, "long password for tests"); err != nil {
				t.Fatal(err)
			}
			// A recent proof never substitutes for the current password in a change.
			if err := failChange("wrong"); !errors.Is(err, identity.ErrCurrentPassword) {
				t.Fatalf("recent proof bypassed current password: %v", err)
			}
			rotated, err := auth.ChangePassword(t.Context(), login.Token, login.User.SessionEpoch, "long password for tests", "new long password", "new long password")
			if err != nil || rotated.Session.ID == login.Session.ID {
				t.Fatalf("expiry rotation failed: %v", err)
			}
			if err := store.EmailTransaction(t.Context(), func(tx identity.EmailTx) error { var err error; guard, err = tx.Guard(key); return err }); err != nil {
				t.Fatal(err)
			}
			if len(guard.Failures) != 0 || !guard.LockedUntil.IsZero() {
				t.Fatal("successful proof did not clear shared failures")
			}
		})
	}
}

func TestFinalLoginSourceBudgetHTTP(t *testing.T) {
	tx, auth, protector, login, now := finalAuthFixture(t)
	store := pgidentity.NewStore(tx)
	key := "login_source/" + hex.EncodeToString(protector.Digest("login_source", uuid.Nil, "127.0.0.1"))
	seed := make([]time.Time, 96)
	for i := range seed {
		seed[i] = *now
	}
	if err := store.EmailTransaction(t.Context(), func(tx identity.EmailTx) error {
		if _, err := tx.Budget(key); err != nil {
			return err
		}
		return tx.SaveBudget(key, domain.EmailBudget{SentAt: []time.Time{}, FailedAt: seed})
	}); err != nil {
		t.Fatal(err)
	}
	router := finalAuthRouter(auth)
	request := func(email, password, peer, forwarded string) *httptest.ResponseRecorder {
		return finalAuthRequest(t, router, "/api/sessions", peer, forwarded, map[string]any{"email": email, "password": password, "persistent": false}, identity.AuthResult{})
	}
	// Known failure, unknown mailbox, malformed email, and success all consume
	// the source window. The last admitted attempt is still allowed to succeed.
	for i, email := range []string{login.User.Email, "rotated-unknown@example.invalid", "malformed-email"} {
		response := request(email, "wrong", "127.0.0.1:1234", fmt.Sprintf("203.0.113.%d", i))
		if response.Code != 401 {
			t.Fatalf("attempt %d status=%d body=%s", i, response.Code, response.Body)
		}
	}
	if response := request(login.User.Email, "long password for tests", "127.0.0.1:4321", "198.51.100.1"); response.Code != 201 {
		t.Fatalf("100th admitted attempt=%d", response.Code)
	}
	for _, email := range []string{login.User.Email, "another@example.invalid", "malformed-email"} {
		response := request(email, "long password for tests", "127.0.0.1:9999", "198.51.100.2")
		if response.Code != 429 {
			t.Fatalf("same peer rotated email/forwarded header bypassed budget: %d", response.Code)
		}
		if response.Header().Get("Retry-After") != "900" {
			t.Fatalf("retry receipt=%s", response.Header().Get("Retry-After"))
		}
	}
	if response := request(login.User.Email, "long password for tests", "127.0.0.2:1234", "198.51.100.2"); response.Code != 201 {
		t.Fatalf("independent peer blocked: %d", response.Code)
	}
	*now = now.Add(15*time.Minute - time.Microsecond)
	if response := request(login.User.Email, "long password for tests", "127.0.0.1:1234", ""); response.Code != 429 {
		t.Fatalf("window expired early: %d", response.Code)
	}
	*now = now.Add(time.Microsecond)
	if response := request(login.User.Email, "long password for tests", "127.0.0.1:1234", ""); response.Code != 201 {
		t.Fatalf("window did not expire independently: %d", response.Code)
	}
	var budget domain.EmailBudget
	if err := store.EmailTransaction(t.Context(), func(tx identity.EmailTx) error { var err error; budget, err = tx.Budget(key); return err }); err != nil {
		t.Fatal(err)
	}
	if len(budget.FailedAt) != 1 || len(budget.SentAt) != 0 {
		t.Fatalf("source expiry/mail isolation: %+v", budget)
	}
}

func TestFinalInternalLoginCannotBypassSourceBudget(t *testing.T) {
	tx, auth, protector, login, now := finalAuthFixture(t)
	key := "login_source/" + hex.EncodeToString(protector.Digest("login_source", uuid.Nil, "unknown"))
	attempts := make([]time.Time, 100)
	for i := range attempts {
		attempts[i] = *now
	}
	if err := pgidentity.NewStore(tx).EmailTransaction(t.Context(), func(tx identity.EmailTx) error {
		if _, err := tx.Budget(key); err != nil {
			return err
		}
		return tx.SaveBudget(key, domain.EmailBudget{SentAt: []time.Time{}, FailedAt: attempts})
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Login(t.Context(), login.User.Email, "long password for tests", false); !errors.Is(err, identity.ErrRateLimited) {
		t.Fatalf("internal Login bypassed budget: %v", err)
	}
}
