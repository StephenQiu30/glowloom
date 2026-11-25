package identity_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestPlatformDirectoryProjection(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, admin := verifiedTestActor(t, tx, "directory-admin@example.invalid", now)
	_, creator := verifiedTestActor(t, tx, "directory-creator@example.invalid", now)
	key, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	directory := identity.NewPlatformDirectory(pgidentity.NewPlatformDirectoryStore(tx), key, func() time.Time { return now })
	if _, err := directory.List(t.Context(), creator, identity.DirectoryFilter{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("creator read global directory")
	}
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='platform_admin',platform_epoch=platform_epoch+1 WHERE id=?`, admin.ID).Error; err != nil {
		t.Fatal(err)
	}
	admin.PlatformEpoch++
	admin.PlatformRole = "platform_admin"
	label := "directory-" + uuid.NewString()
	ids := []uuid.UUID{admin.ID, creator.ID}
	if err := tx.Exec(`UPDATE identity."user" SET display_name=?,create_time=? WHERE id IN ?`, label, now.Add(-time.Second), ids).Error; err != nil {
		t.Fatal(err)
	}
	f := identity.DirectoryFilter{Search: label, Limit: 1}
	first, err := directory.List(t.Context(), admin, f)
	if err != nil || len(first.Items) != 1 || first.NextCursor == nil {
		t.Fatalf("directory first page: %v", err)
	}
	f.Cursor = *first.NextCursor
	second, err := directory.List(t.Context(), admin, f)
	if err != nil || len(second.Items) != 1 || second.NextCursor != nil || first.Items[0].ID == second.Items[0].ID {
		t.Fatal("unstable directory pagination")
	}
	if _, err := directory.List(t.Context(), creator, f); !errors.Is(err, identity.ErrInvalidDirectoryFilter) {
		t.Fatal("cross-actor directory cursor accepted")
	}
	f.Search = "changed"
	if _, err := directory.List(t.Context(), admin, f); !errors.Is(err, identity.ErrInvalidDirectoryFilter) {
		t.Fatal("directory filter changed with old cursor")
	}
	page, err := directory.List(t.Context(), admin, identity.DirectoryFilter{Search: " DIRECTORY-CREATOR@EXAMPLE.INVALID "})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != creator.ID {
		t.Fatal("normalized exact email search failed")
	}
	if page, err := directory.List(t.Context(), admin, identity.DirectoryFilter{Search: "%_"}); err != nil || len(page.Items) != 0 {
		t.Fatal("search wildcard expanded")
	}
	team := uuid.New()
	if err := tx.Exec(`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'Directory team',?)`, team, creator.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',?,'created')`, team, creator.ID, now).Error; err != nil {
		t.Fatal(err)
	}
	inTeam := true
	page, err = directory.List(t.Context(), admin, identity.DirectoryFilter{Search: label, InTeam: &inTeam})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != creator.ID || page.Items[0].ActiveTeamCount != 1 {
		t.Fatal("team membership filter/count incorrect")
	}
	if _, err := directory.List(t.Context(), creator, identity.DirectoryFilter{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("team admin acquired global authority")
	}
	detail, err := directory.Detail(t.Context(), admin, creator.ID)
	if err != nil || len(detail.Teams) != 1 || detail.Teams[0].TeamID != team {
		t.Fatal("team summary missing")
	}
	raw, _ := json.Marshal(detail)
	for _, secret := range []string{"PasswordHash", "SessionEpoch", "CredentialEpoch", "LoginName", "OrgID", "PromptSuffix"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("unsafe directory projection")
		}
	}
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='creator',platform_epoch=platform_epoch+1 WHERE id=?`, admin.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := directory.Detail(t.Context(), admin, creator.ID); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("revoked platform admin read detail")
	}
}
