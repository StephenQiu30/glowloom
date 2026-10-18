package identity_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

func TestHistoricalPasswordFlagNeverRestrictsAuthenticatedAccount(t *testing.T) {
	for _, tc := range []struct {
		name, password string
		legacy         bool
	}{
		{"current-marked", "long password for tests", false},
		{"legacy-marked", "long password for tests", true},
		{"short-legacy-marked", "old", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := sessionV2RollbackTransaction(t)
			now := time.Now().UTC()
			email := tc.name + "@no-force.example.invalid"
			auth, actor := verifiedTestActor(t, tx, email, now)
			if tc.legacy {
				hash, err := bcrypt.GenerateFromPassword([]byte(tc.password), 12)
				if err != nil {
					t.Fatal(err)
				}
				if err := tx.Exec(`UPDATE identity."user" SET password_hash=? WHERE id=?`, string(hash), actor.ID).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.Exec(`UPDATE identity."user" SET must_change_password=true WHERE id=?`, actor.ID).Error; err != nil {
				t.Fatal(err)
			}
			login, err := auth.Login(t.Context(), email, tc.password, false)
			if err != nil {
				t.Fatalf("valid existing credential rejected: %v", err)
			}
			if !domain.VerifyPassword(login.User.PasswordHash, tc.password) {
				t.Fatal("login changed existing credential")
			}
			actor.SessionID, actor.CredentialEpoch = login.Session.ID, login.User.SessionEpoch
			if err := pgidentity.NewStore(tx).CheckSession(t.Context(), actor, func() time.Time { return now }); err != nil {
				t.Errorf("historical flag blocked session: %v", err)
			}
			if err := pgidentity.NewStore(tx).CheckActive(t.Context(), actor.ID); err != nil {
				t.Errorf("historical flag blocked worker authority: %v", err)
			}
			profile := identity.NewSelfProfile(pgidentity.NewSelfProfileStore(tx, nil), func() time.Time { return now })
			if _, err := profile.Read(t.Context(), actor); err != nil {
				t.Errorf("historical flag blocked profile: %v", err)
			}
			router := gin.New()
			handler := identityhttp.NewEmailAuthHandler(auth, "http://localhost:3200", false)
			handler.Register(router.Group("/api"))
			router.GET("/protected", handler.RequireSession(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
			for _, path := range []string{"/protected", "/api/session"} {
				req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
				req.AddCookie(&http.Cookie{Name: "lanverse_session", Value: login.Token})
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if rec.Code >= 300 {
					t.Errorf("%s blocked historical flag: %d", path, rec.Code)
				}
				if path == "/api/session" {
					var wire map[string]json.RawMessage
					if err := json.Unmarshal(rec.Body.Bytes(), &wire); err != nil {
						t.Fatal(err)
					}
					if _, ok := wire["must_change_password"]; ok {
						t.Error("session still exposes forced password contract")
					}
					var session identityhttp.EmailSessionView
					if err := json.Unmarshal(rec.Body.Bytes(), &session); err != nil {
						t.Fatal(err)
					}
					if len(session.Actor.Capabilities) == 0 {
						t.Error("historical flag removed ordinary capabilities")
					}
				}
			}
		})
	}
}

func TestHistoricalPasswordFlagDoesNotBypassRevocation(t *testing.T) {
	for _, tc := range []struct {
		name, sql string
		account   bool
	}{
		{"disabled", `UPDATE identity."user" SET status='disabled' WHERE id=?`, true},
		{"deleted", `UPDATE identity."user" SET is_delete=true,deleted_at=now() WHERE id=?`, true},
		{"expired", `UPDATE identity.user_session SET created_at=now()-interval '2 days',absolute_expires_at=now()-interval '1 day' WHERE account_id=?`, false},
		{"revoked", `UPDATE identity.user_session SET revoked_at=now() WHERE account_id=?`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := existingIdentityTransaction(t)
			now := time.Now().UTC()
			email := tc.name + "@no-force-revocation.invalid"
			auth, actor := verifiedTestActor(t, tx, email, now)
			if err := tx.Exec(`UPDATE identity."user" SET must_change_password=true WHERE id=?`, actor.ID).Error; err != nil {
				t.Fatal(err)
			}
			login, err := auth.Login(t.Context(), email, "long password for tests", false)
			if err != nil {
				t.Fatal(err)
			}
			actor.SessionID = login.Session.ID
			actor.CredentialEpoch = login.User.SessionEpoch
			if err := tx.Exec(tc.sql, actor.ID).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := auth.Current(t.Context(), login.Token); err == nil {
				t.Fatal("unavailable session accepted")
			}
			if err := pgidentity.NewStore(tx).CheckSession(t.Context(), actor, func() time.Time { return now }); err == nil {
				t.Fatal("unavailable actor reached content")
			}
			if tc.account {
				if err := pgidentity.NewStore(tx).CheckActive(t.Context(), actor.ID); err == nil {
					t.Fatal("unavailable account retained worker authority")
				}
			}
		})
	}
}
