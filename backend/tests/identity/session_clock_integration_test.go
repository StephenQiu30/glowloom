package identity_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// The barrier holds only the request, never a database operation. Both requests
// use the existing outer rollback transaction, so no fixture escapes rollback.
type delayedProfileStore struct {
	identity.SelfProfileStore
	entered chan struct{}
	release chan struct{}
}

func (s *delayedProfileStore) ProfileTransaction(ctx context.Context, actor identity.Principal, clock func() time.Time, run func(identity.SelfProfileTx) error) error {
	close(s.entered)
	select {
	case <-s.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return s.SelfProfileStore.ProfileTransaction(ctx, actor, clock, run)
}

func TestSessionClockPreferencesHTTPInterleaving(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		delay time.Duration
		touch bool
		want  int
	}{
		{"concurrent_session_touch", time.Second, true, http.StatusOK},
		{"idle_expires_while_waiting", 12 * time.Hour, false, http.StatusUnauthorized},
		{"clock_rewinds_while_waiting", -time.Second, false, http.StatusUnauthorized},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			tx, _, protector, login, start := finalAuthFixture(t)
			var clock atomic.Int64
			clock.Store(start.UnixNano())
			now := func() time.Time { return time.Unix(0, clock.Load()).UTC() }
			auth := identity.NewEmailAuth(pgidentity.NewStore(tx), protector, identity.EmailPolicy{}, now)
			delayed := &delayedProfileStore{SelfProfileStore: pgidentity.NewSelfProfileStore(tx, nil), entered: make(chan struct{}), release: make(chan struct{})}
			gin.SetMode(gin.TestMode)
			router := gin.New()
			handler := identityhttp.NewEmailAuthHandler(auth, "http://localhost:3200", false)
			handler.Register(router.Group("/api"))
			identityhttp.NewSelfProfileHandler(identity.NewSelfProfile(delayed, now)).Register(router.Group("/api", handler.RequireSession()))
			request := func(path string) *httptest.ResponseRecorder {
				req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
				req.AddCookie(&http.Cookie{Name: "lanverse_session", Value: login.Token})
				response := httptest.NewRecorder()
				router.ServeHTTP(response, req)
				return response
			}
			result := make(chan *httptest.ResponseRecorder, 1)
			go func() { result <- request("/api/me/preferences") }()
			select {
			case <-delayed.entered:
			case r := <-result:
				t.Fatalf("preferences did not reach barrier: %d", r.Code)
			case <-time.After(5 * time.Second):
				t.Fatal("preferences did not reach barrier")
			}
			clock.Store(start.Add(scenario.delay).UnixNano())
			var touched int
			if scenario.touch {
				touched = request("/api/session").Code
			}
			close(delayed.release)
			var response *httptest.ResponseRecorder
			select {
			case response = <-result:
			case <-time.After(5 * time.Second):
				t.Fatal("preferences did not finish")
			}
			if scenario.touch && touched != http.StatusOK {
				t.Fatalf("concurrent session status=%d", touched)
			}
			if response.Code != scenario.want {
				t.Fatalf("preferences status=%d, want %d", response.Code, scenario.want)
			}
		})
	}
}

// Advance the controlled clock at the completed session SELECT. This catches
// sampling at transaction entry (which the HTTP request barrier alone cannot).
func TestSessionClockSamplesAfterSessionRowLock(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		delay    time.Duration
		absolute bool
		recent   bool
		want     error
	}{
		{name: "idle_boundary", delay: 12 * time.Hour, want: identity.ErrSessionInvalid},
		{name: "absolute_boundary", delay: time.Second, absolute: true, want: identity.ErrSessionInvalid},
		{name: "clock_rewind", delay: -time.Second, want: identity.ErrSessionInvalid},
		{name: "recent_proof_boundary", delay: 5 * time.Minute, recent: true, want: identity.ErrReauthenticationRequired},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			tx, _, _, login, start := finalAuthFixture(t)
			actor := identity.Principal{ID: login.User.ID, SessionID: login.Session.ID, CredentialEpoch: login.User.SessionEpoch, PlatformEpoch: login.User.PlatformEpoch}
			if scenario.absolute {
				if err := tx.Exec(`UPDATE identity.user_session SET absolute_expires_at=? WHERE id=?`, start.Add(time.Second), login.Session.ID).Error; err != nil {
					t.Fatal(err)
				}
			}
			if scenario.recent {
				if err := tx.Exec(`UPDATE identity.user_session SET reauth_at=?,reauth_epoch=? WHERE id=?`, *start, login.User.SessionEpoch, login.Session.ID).Error; err != nil {
					t.Fatal(err)
				}
			}
			now := *start
			reached := false
			callback := "test_session_clock_" + scenario.name
			if err := tx.Callback().Row().After("gorm:row").Register(callback, func(query *gorm.DB) {
				sql := query.Statement.SQL.String()
				if strings.Contains(sql, "FROM identity.user_session") && strings.Contains(sql, "FOR UPDATE") {
					reached = true
					now = start.Add(scenario.delay)
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := tx.Callback().Row().Remove(callback); err != nil {
					t.Error(err)
				}
			})
			clock := func() time.Time { return now }
			var err error
			if scenario.recent {
				err = pgidentity.NewStore(tx).CheckRecentSession(t.Context(), actor, clock)
			} else {
				_, err = identity.NewSelfProfile(pgidentity.NewSelfProfileStore(tx, nil), clock).ReadPreferences(t.Context(), actor)
			}
			if !reached {
				t.Fatal("session row lock hook not reached")
			}
			if !errors.Is(err, scenario.want) {
				t.Fatalf("lock-time authorization error=%v, want %v", err, scenario.want)
			}
		})
	}
}

func TestSessionClockProofConsumptionAfterLocks(t *testing.T) {
	for _, operation := range []string{"verify_email", "complete_email", "confirm_deletion"} {
		for _, boundary := range []string{"proof", "recent"} {
			t.Run(operation+"/"+boundary, func(t *testing.T) {
				tx, _, protector, login, now := finalAuthFixture(t)
				actor := identity.Principal{ID: login.User.ID, SessionID: login.Session.ID, CredentialEpoch: login.User.SessionEpoch, PlatformEpoch: login.User.PlatformEpoch}
				if err := tx.Exec(`UPDATE identity.user_session SET reauth_at=?,reauth_epoch=? WHERE id=?`, *now, actor.CredentialEpoch, actor.SessionID).Error; err != nil {
					t.Fatal(err)
				}
				clock := func() time.Time { return *now }
				auth := identity.NewEmailAuth(pgidentity.NewStore(tx), protector, identity.EmailPolicy{TermsVersion: "test", SourceHourlyLimit: 100, GlobalHourlyLimit: 1000}, clock)
				change := identity.NewEmailChange(auth, pgidentity.NewEmailChangeStore(tx), clock)
				deletion := identity.NewAccountDeletion(auth, pgidentity.NewAccountDeletionStore(tx), clock)
				var op uuid.UUID
				var code string
				if operation == "confirm_deletion" {
					start, err := deletion.Start(t.Context(), actor, "test")
					if err != nil {
						t.Fatal(err)
					}
					op = start.OperationID
					code = deletionCode(t, tx, op)
				} else {
					var err error
					op, err = change.Start(t.Context(), actor, "clock-new@example.invalid", login.User.Revision)
					if err != nil {
						t.Fatal(err)
					}
					code = changeCode(t, tx, op, "old")
					if err := tx.Exec(`UPDATE identity.mail_delivery SET status='submitted' WHERE challenge_id IN (SELECT id FROM identity.auth_challenge WHERE operation_id=?)`, op).Error; err != nil {
						t.Fatal(err)
					}
					if operation == "complete_email" {
						if err := change.Verify(t.Context(), actor, op, "old", code); err != nil {
							t.Fatal(err)
						}
						if err := change.Verify(t.Context(), actor, op, "new", changeCode(t, tx, op, "new")); err != nil {
							t.Fatal(err)
						}
					}
				}
				want := identity.ErrProofInvalid
				if boundary == "proof" {
					if err := tx.Exec(`UPDATE identity.auth_challenge SET expires_at=? WHERE operation_id=?`, now.Add(time.Second), op).Error; err != nil {
						t.Fatal(err)
					}
				} else {
					want = identity.ErrReauthenticationRequired
					if err := tx.Exec(`UPDATE identity.user_session SET reauth_at=? WHERE id=?`, now.Add(-5*time.Minute+time.Second), actor.SessionID).Error; err != nil {
						t.Fatal(err)
					}
				}
				match := "FROM identity.email_budget"
				if operation == "complete_email" {
					match = "FROM identity.auth_challenge"
				}
				reached := false
				callback := "test_proof_clock"
				if err := tx.Callback().Row().After("gorm:row").Register(callback, func(query *gorm.DB) {
					sql := query.Statement.SQL.String()
					if !reached && strings.Contains(sql, match) && strings.Contains(sql, "FOR UPDATE") {
						reached = true
						*now = now.Add(time.Second)
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := tx.Callback().Row().Remove(callback); err != nil {
						t.Error(err)
					}
				})
				var err error
				switch operation {
				case "verify_email":
					err = change.Verify(t.Context(), actor, op, "old", code)
				case "complete_email":
					err = change.Complete(t.Context(), actor, op, login.User.Revision)
				case "confirm_deletion":
					err = deletion.Confirm(t.Context(), actor, login.User.Revision, op, code)
				}
				if !reached {
					t.Fatal("proof authorization lock hook not reached")
				}
				if !errors.Is(err, want) {
					t.Fatalf("expired %s accepted by %s: %v", boundary, operation, err)
				}
				var unchanged bool
				if err := tx.Raw(`SELECT email_key=? AND session_epoch=? AND NOT is_delete FROM identity."user" WHERE id=?`, login.User.Email, login.User.SessionEpoch, login.User.ID).Scan(&unchanged).Error; err != nil || !unchanged {
					t.Fatal("expired proof changed account", err)
				}
				if operation != "complete_email" {
					var consumed int64
					if err := tx.Raw(`SELECT count(*) FROM identity.auth_challenge WHERE operation_id=? AND consumed_at IS NOT NULL`, op).Scan(&consumed).Error; err != nil || consumed != 0 {
						t.Fatal("expired proof consumed", err)
					}
				}
			})
		}
	}
}
