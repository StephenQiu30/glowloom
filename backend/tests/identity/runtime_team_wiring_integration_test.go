package identity_test

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func TestRuntimeTeamInvitationAndNotificationWiring(t *testing.T) {
	tx, gov, actor, now := governanceFixture(t)
	if err := tx.Exec(`UPDATE identity.platform_settings SET team_creation_enabled=true,registration_enabled=false`).Error; err != nil {
		t.Fatal(err)
	}
	teams := workspace.NewTeams(pgworkspace.NewTeamStore(tx, func(tx *gorm.DB) workspace.TeamActorAccess {
		return pgidentity.NewTeamActorStore(tx, completionProtector(t))
	}), func() time.Time { return now })
	team, err := teams.Create(t.Context(), actor, uuid.New(), "Runtime invitation team")
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := gov.CreateTeamInvitation(t.Context(), actor, uuid.New(), team.ID, "runtime-invited@example.invalid", "member")
	if err != nil {
		t.Fatal(err)
	}
	token := completionPayload(t, tx, invitation.EmailKey, "team_invitation")["token"]
	cfg := config.Config{IdentityKey: filepath.Join(t.TempDir(), "identity.key"), PublicOrigin: "http://localhost:3200", IdentityTermsVersion: identityhttp.LegalPackageVersion(), IdentityMailSourceHourlyLimit: 100, IdentityMailGlobalHourlyLimit: 1000}
	if err := credentialproof.CreateFile(cfg.IdentityKey); err != nil {
		t.Fatal(err)
	}
	material, err := os.ReadFile(cfg.IdentityKey)
	if err != nil {
		t.Fatal(err)
	}
	cfg.IdentityKey = string(material)
	auth, err := identityruntime.NewAuth(tx, cfg, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Run("invited_registration_policy", func(t *testing.T) {
		_, err := auth.StartRegistration(t.Context(), invitation.EmailKey, token)
		if identityhttp.AcceptedLegalVersion(cfg.IdentityTermsVersion) == "" {
			if !errors.Is(err, identity.ErrRegistrationUnavailable) {
				t.Fatal("prepared agreement did not keep invited registration closed", err)
			}
		} else if err != nil {
			t.Fatalf("runtime omitted team registration gateway: %v", err)
		}
	})
	t.Run("encrypted_team_notification", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		router := gin.New()
		router.Use(httpapi.Middleware(cfg.PublicOrigin))
		api := router.Group("/api")
		handler, register, err := identityruntime.RegisterSelf(tx, auth, cfg, nil, api, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		handler.Register(api)
		register(api.Group("", handler.RequireSession()))
		req := httptest.NewRequestWithContext(t.Context(), "POST", "/api/sessions", strings.NewReader(`{"email":"governance-admin@example.invalid","password":"long password for tests"}`))
		req.Header.Set("Origin", cfg.PublicOrigin)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", uuid.NewString())
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != 201 {
			t.Fatal("login failed", response.Code)
		}
		var session struct {
			CSRFToken string `json:"csrf_token"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil {
			t.Fatal(err)
		}
		create := httptest.NewRequestWithContext(t.Context(), "POST", "/api/teams", strings.NewReader(`{"name":"Runtime notification team"}`))
		create.Header.Set("Origin", cfg.PublicOrigin)
		create.Header.Set("Content-Type", "application/json")
		create.Header.Set("Idempotency-Key", uuid.NewString())
		create.Header.Set("X-CSRF-Token", session.CSRFToken)
		create.AddCookie(response.Result().Cookies()[0])
		result := httptest.NewRecorder()
		router.ServeHTTP(result, create)
		if result.Code != 201 {
			t.Fatalf("runtime team creation rejected without notice protector: %d", result.Code)
		}
		var mail struct {
			EventID          uuid.UUID
			KeyID            string
			EncryptedPayload []byte
		}
		protector, err := credentialproof.LoadValue(cfg.IdentityKey)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Raw(`SELECT event_id,key_id,encrypted_payload FROM identity.mail_delivery WHERE template='team_governance' AND recipient='governance-admin@example.invalid' AND key_id=?`, protector.KeyID()).Scan(&mail).Error; err != nil {
			t.Fatal(err)
		}
		if mail.EventID == uuid.Nil {
			t.Fatal("runtime team notification not enqueued")
		}
		if _, err := protector.Open(mail.EventID, mail.KeyID, mail.EncryptedPayload); err != nil {
			t.Fatal("runtime notification not encrypted with configured key")
		}
	})
}
