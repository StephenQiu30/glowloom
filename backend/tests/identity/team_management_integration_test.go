package identity_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func TestTeamCreationAndMemberProjection(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, admin := verifiedTestActor(t, tx, "team-admin@example.invalid", now)
	_, member := verifiedTestActor(t, tx, "team-member@example.invalid", now)
	_, outsider := verifiedTestActor(t, tx, "team-outsider@example.invalid", now)
	store := pgworkspace.NewTeamStore(tx, func(tx *gorm.DB) workspace.TeamActorAccess {
		return pgidentity.NewTeamActorStore(tx, completionProtector(t))
	})
	teams := workspace.NewTeams(store, func() time.Time { return now })
	key := uuid.New()
	if _, err := teams.Create(t.Context(), admin, key, "Creative team"); !errors.Is(err, workspace.ErrTeamCreationClosed) {
		t.Fatalf("closed team creation accepted: %v", err)
	}
	if err := tx.Exec(`UPDATE identity.platform_settings SET team_creation_enabled=true WHERE singleton`).Error; err != nil {
		t.Fatal(err)
	}
	team, err := teams.Create(t.Context(), admin, key, "Creative team")
	if err != nil {
		t.Fatal(err)
	}
	spaces, err := teams.Spaces(t.Context(), admin)
	if err != nil || len(spaces) != 2 {
		t.Fatal("personal and team spaces not available", err)
	}
	for _, space := range spaces {
		if !space.Writable {
			t.Fatal("active creator space unexpectedly read-only")
		}
	}
	replay, err := teams.Create(t.Context(), admin, key, "Creative team")
	if err != nil || replay.ID != team.ID {
		t.Fatal("team retry duplicated creation", err)
	}
	if _, err := teams.Create(t.Context(), admin, key, "Different team"); !errors.Is(err, workspace.ErrIdempotencyConflict) {
		t.Fatalf("key reused with new request: %v", err)
	}
	if err := tx.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'member','active',?,'invited')`, team.ID, member.ID, now).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := teams.Read(t.Context(), outsider, team.ID); !errors.Is(err, workspace.ErrScopeNotFound) {
		t.Fatalf("outsider can read team: %v", err)
	}
	public, err := teams.Members(t.Context(), member, team.ID)
	if err != nil || len(public) != 2 {
		t.Fatal("missing team directory", err)
	}
	for _, m := range public {
		if m.Email != nil || m.Status != nil || m.Revision != nil {
			t.Fatal("ordinary member received governance fields")
		}
	}
	private, err := teams.Members(t.Context(), admin, team.ID)
	if err != nil || len(private) != 2 || private[0].Email == nil {
		t.Fatal("administrator directory missing required fields", err)
	}
	for i := 1; i < 5; i++ {
		if _, err := teams.Create(t.Context(), admin, uuid.New(), "Another team"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := teams.Create(t.Context(), admin, uuid.New(), "Sixth team"); !errors.Is(err, workspace.ErrTeamCapacity) {
		t.Fatalf("creation limit not enforced: %v", err)
	}
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM workspace.team WHERE created_by=?`, admin.ID).Scan(&count).Error; err != nil || count != 5 {
		t.Fatal("partial or excess team creation", err)
	}
}

func TestTeamMembershipRevokesOnlyOneTeam(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, admin := verifiedTestActor(t, tx, "team-control@example.invalid", now)
	_, member := verifiedTestActor(t, tx, "team-controlled@example.invalid", now)
	if err := tx.Exec(`UPDATE identity.platform_settings SET team_creation_enabled=true WHERE singleton`).Error; err != nil {
		t.Fatal(err)
	}
	svc := workspace.NewTeams(pgworkspace.NewTeamStore(tx, func(tx *gorm.DB) workspace.TeamActorAccess {
		return pgidentity.NewTeamActorStore(tx, completionProtector(t))
	}), func() time.Time { return now })
	a, err := svc.Create(t.Context(), admin, uuid.New(), "Team Alpha")
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Create(t.Context(), admin, uuid.New(), "Team Bravo")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{a.ID, b.ID} {
		if err := tx.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'member','active',?,'invited')`, id, member.ID, now).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.ChangeRole(t.Context(), admin, b.ID, member.ID, 1, "team_admin", "Promote team administrator"); !errors.Is(err, identity.ErrReauthenticationRequired) {
		t.Fatal("role change skipped recent password proof", err)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET reauth_at=?,reauth_epoch=? WHERE id=?`, now, admin.CredentialEpoch, admin.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.ChangeRole(t.Context(), admin, b.ID, member.ID, 1, "team_admin", "Promote team administrator"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Leave(t.Context(), admin, b.ID, 1); err != nil {
		t.Fatal("handoff did not release former administrator", err)
	}
	if err := svc.ChangeMember(t.Context(), member, a.ID, admin.ID, 1, "remove", "Attempt by ordinary member"); !errors.Is(err, workspace.ErrTeamForbidden) {
		t.Fatalf("member escalated governance: %v", err)
	}
	if err := svc.ChangeMember(t.Context(), admin, a.ID, member.ID, 1, "suspend", "Access temporarily suspended"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Read(t.Context(), member, a.ID); !errors.Is(err, workspace.ErrScopeNotFound) {
		t.Fatalf("suspended member still has access: %v", err)
	}
	if _, err := svc.Read(t.Context(), member, b.ID); err != nil {
		t.Fatal("suspension affected another team", err)
	}
	if err := svc.Leave(t.Context(), admin, a.ID, 1); !errors.Is(err, workspace.ErrLastTeamAdministrator) {
		t.Fatalf("last administrator abandoned team: %v", err)
	}
	if err := svc.ChangeMember(t.Context(), admin, a.ID, member.ID, 1, "restore", "Restore team membership"); !errors.Is(err, workspace.ErrTeamRevisionConflict) {
		t.Fatal("stale member update accepted", err)
	}
	if err := svc.ChangeMember(t.Context(), admin, a.ID, member.ID, 2, "remove", "Remove team membership"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ChangeMember(t.Context(), admin, a.ID, member.ID, 3, "restore", "Restore removed membership"); !errors.Is(err, workspace.ErrTeamStateConflict) {
		t.Fatal("removed member resurrected without invitation", err)
	}
}
