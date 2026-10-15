package identity_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func TestTeamDirectoryFiltersAndCursor(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, admin := verifiedTestActor(t, tx, "directory-admin@example.invalid", now)
	_, member := verifiedTestActor(t, tx, "directory-member@example.invalid", now)
	if err := tx.Exec(`UPDATE identity.platform_settings SET team_creation_enabled=true WHERE singleton`).Error; err != nil {
		t.Fatal(err)
	}
	store := pgworkspace.NewTeamStore(tx, func(tx *gorm.DB) workspace.TeamActorAccess {
		return pgidentity.NewTeamActorStore(tx, completionProtector(t))
	})
	team, err := workspace.NewTeams(store, func() time.Time { return now }).Create(t.Context(), admin, uuid.New(), "Directory team")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'member','active',?,'invited')`, team.ID, member.ID, now).Error; err != nil {
		t.Fatal(err)
	}
	key, err := credentialproof.New("team-test", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	directory := workspace.NewTeamDirectory(store, key, func() time.Time { return now })
	page, err := directory.List(t.Context(), admin, team.ID, workspace.MemberFilter{Limit: 1})
	if err != nil || len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatal("missing bounded page", err)
	}
	second, err := directory.List(t.Context(), admin, team.ID, workspace.MemberFilter{Limit: 1, Cursor: *page.NextCursor})
	if err != nil || len(second.Items) != 1 || second.Items[0].UserID == page.Items[0].UserID || second.NextCursor != nil {
		t.Fatal("invalid continuation", err)
	}
	if _, err := directory.List(t.Context(), member, team.ID, workspace.MemberFilter{Limit: 1, Cursor: *page.NextCursor}); !errors.Is(err, workspace.ErrInvalidMemberFilter) {
		t.Fatalf("cursor crossed actor: %v", err)
	}
	if _, err := directory.List(t.Context(), admin, team.ID, workspace.MemberFilter{Limit: 1, Role: "member", Cursor: *page.NextCursor}); !errors.Is(err, workspace.ErrInvalidMemberFilter) {
		t.Fatalf("cursor crossed filter: %v", err)
	}
	public, err := directory.List(t.Context(), member, team.ID, workspace.MemberFilter{Limit: 20})
	if err != nil || len(public.Items) != 2 {
		t.Fatal(err)
	}
	for _, row := range public.Items {
		if row.Email != nil || row.Status != nil || row.Revision != nil || row.AccessAvailable != nil {
			t.Fatal("public directory leaks governance data")
		}
	}
	if err := workspace.NewTeams(store, func() time.Time { return now }).ChangeMember(t.Context(), admin, team.ID, member.ID, 1, "remove", "Membership no longer needed"); err != nil {
		t.Fatal(err)
	}
	removed, err := directory.List(t.Context(), admin, team.ID, workspace.MemberFilter{Status: "removed", Search: "directory-member"})
	if err != nil || len(removed.Items) != 1 || removed.Items[0].Email == nil || *removed.Items[0].Status != "removed" {
		t.Fatal("removed member history lost", err)
	}
	if _, err := directory.List(t.Context(), admin, team.ID, workspace.MemberFilter{Limit: 1, Cursor: *page.NextCursor}); !errors.Is(err, workspace.ErrInvalidMemberFilter) {
		t.Fatal("cursor survived membership epoch change", err)
	}
}
