package identityfixture

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// Session upgrades an explicitly synthetic account fixture to a verified email
// session and its personal scope, without inferring any platform permission.
func Session(t *testing.T, database *gorm.DB, actor identity.Principal) identity.Principal {
	t.Helper()
	var login string
	if err := database.Raw(`SELECT login_name FROM identity."user" WHERE id=?`, actor.ID).Scan(&login).Error; err != nil || !strings.Contains(login, actor.ID.String()) {
		t.Fatal("session fixture requires an explicitly synthetic account")
	}
	const password = "fixture-only login password"
	hash, err := domain.HashPassword(password, "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := database.Exec(`UPDATE identity."user" SET email_key=?,email_verified_at=?,password_hash=?,password_changed_at=? WHERE id=?`, actor.ID.String()+"@fixture.invalid", now, hash, now, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO workspace.organization(id,name) VALUES(?,?) ON CONFLICT(id) DO NOTHING`, actor.OrgID, "Fixture "+actor.OrgID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO workspace.creation_scope(id,kind,owner_user_id,physical_scope_id) VALUES(?,'personal',?,?) ON CONFLICT DO NOTHING`, uuid.New(), actor.ID, actor.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	cfg := Configure(t, config.Config{})
	result := Login(t, database, cfg, actor.ID, password)
	return identity.Principal{ID: result.User.ID, OrgID: result.User.OrgID, LoginName: result.User.LoginName, DisplayName: result.User.DisplayName, Role: result.User.Role, PlatformRole: result.User.PlatformRole, PlatformEpoch: result.User.PlatformEpoch, CredentialEpoch: result.User.SessionEpoch, SessionID: result.Session.ID, ScopeID: Scope(t, database, actor.ID)}
}
