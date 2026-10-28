package identity_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

func TestPlatformDeleteRetainsIdentityAndReplays(t *testing.T) {
	tx, svc, actor, now := governanceFixture(t)
	_, target := verifiedTestActor(t, tx, "delete-target@example.invalid", now)
	if err := tx.Exec(`UPDATE identity."user" SET revision=1 WHERE id=?`, target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET reauth_at=NULL,reauth_epoch=NULL WHERE id=?`, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	for range 2 {
		if err := svc.ExecuteUserCommand(t.Context(), actor, target.ID, 1, "", "delete", key); err != nil {
			t.Fatal(err)
		}
	}
	var state struct {
		IsDelete bool
		Status   string
		Revision int64
		Email    string
	}
	if err := tx.Raw(`SELECT is_delete,status,revision,email_key AS email FROM identity."user" WHERE id=?`, target.ID).Scan(&state).Error; err != nil {
		t.Fatal(err)
	}
	if !state.IsDelete || state.Status != "disabled" || state.Revision != 2 || state.Email != "delete-target@example.invalid" {
		t.Fatalf("unexpected retained identity: %+v", state)
	}
	if err := pgidentity.NewStore(tx).CheckSession(t.Context(), target, func() time.Time { return now }); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("deleted session survives", err)
	}
	receipt, err := svc.UserCommandReceipt(t.Context(), actor, target.ID, key)
	if err != nil || receipt.Action != "delete" {
		t.Fatal("missing delete receipt", receipt, err)
	}
	if err := svc.ExecuteUserCommand(t.Context(), actor, target.ID, 2, "", "delete", key); !errors.Is(err, identity.ErrGovernanceConflict) {
		t.Fatal("changed payload replay", err)
	}
	var events int64
	if err := tx.Raw(`SELECT count(*) FROM identity.security_event WHERE actor_id=? AND target_user_id=? AND action='account.delete' AND reason<>''`, actor.ID, target.ID).Scan(&events).Error; err != nil || events != 1 {
		t.Fatal("missing single platform audit", events, err)
	}
}

func TestSimpleDisableKeepsSecurityBoundaries(t *testing.T) {
	tx, svc, actor, now := governanceFixture(t)
	_, target := verifiedTestActor(t, tx, "simple-disable@example.invalid", now)
	if err := tx.Exec(`UPDATE identity."user" SET revision=1 WHERE id=?`, target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET reauth_at=NULL,reauth_epoch=NULL WHERE id=?`, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.ExecuteUserCommand(t.Context(), actor, target.ID, 9, "", "disable", uuid.New()); !errors.Is(err, domain.ErrRevisionConflict) {
		t.Fatal("stale revision accepted", err)
	}
	if err := svc.ExecuteUserCommand(t.Context(), target, actor.ID, 1, "", "disable", uuid.New()); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("ordinary user can disable", err)
	}
	if err := svc.ExecuteUserCommand(t.Context(), actor, actor.ID, 1, "", "delete", uuid.New()); !errors.Is(err, identity.ErrInvalidGovernance) {
		t.Fatal("self deletion accepted", err)
	}
	if err := svc.ExecuteUserCommand(t.Context(), actor, target.ID, 1, "", "disable", uuid.New()); err != nil {
		t.Fatal(err)
	}
	if err := svc.ExecuteUserCommand(t.Context(), actor, target.ID, 2, "", "delete", uuid.New()); err != nil {
		t.Fatal("disabled account cannot be deleted", err)
	}
}

func TestPlatformDeleteRequiresTeamHandoffAndRetainsWork(t *testing.T) {
	tx, svc, actor, now := governanceFixture(t)
	_, target := verifiedTestActor(t, tx, "team-delete@example.invalid", now)
	team, project := uuid.New(), uuid.New()
	for _, step := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE identity."user" SET revision=1 WHERE id=?`, []any{target.ID}},
		{`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'Retained team',?)`, []any{team, target.ID}},
		{`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',?,'created')`, []any{team, target.ID, now}},
		{`INSERT INTO workspace.project(id,org_id,owner_id,name,aspect_ratio,style_type) SELECT ?,org_id,id,'Retained work','16:9','realistic' FROM identity."user" WHERE id=?`, []any{project, target.ID}},
	} {
		if err := tx.Exec(step.sql, step.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	directory := identity.NewPlatformDirectory(pgidentity.NewPlatformDirectoryStore(tx), completionProtector(t), func() time.Time { return now })
	detail, err := directory.Detail(t.Context(), actor, target.ID)
	if err != nil || detail.BlockedActions["delete"] != "deletion_duties" {
		t.Fatal("missing handoff reason", detail.BlockedActions, err)
	}
	key := uuid.New()
	if err := svc.ExecuteUserCommand(t.Context(), actor, target.ID, 1, "", "delete", key); !errors.Is(err, identity.ErrDeletionDuties) {
		t.Fatal("last team admin can be deleted", err)
	}
	var revision int64
	if err := tx.Raw(`SELECT revision FROM identity."user" WHERE id=?`, target.ID).Scan(&revision).Error; err != nil || revision != 1 {
		t.Fatal("blocked delete mutated identity", revision, err)
	}
	if _, err := svc.UserCommandReceipt(t.Context(), actor, target.ID, key); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatal("blocked deletion stored successful receipt", err)
	}
	if err := tx.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',?,'transferred')`, team, actor.ID, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.ExecuteUserCommand(t.Context(), actor, target.ID, 1, "", "delete", key); err != nil {
		t.Fatal(err)
	}
	var retained bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.project p JOIN workspace.team t ON t.created_by=p.owner_id JOIN workspace.team_member m ON m.team_id=t.id AND m.user_id=p.owner_id WHERE p.id=? AND t.id=? AND NOT p.is_delete AND m.status='removed')`, project, team).Scan(&retained).Error; err != nil || !retained {
		t.Fatal("resources lost or team access retained", err)
	}
}

func TestPlatformDeleteDisabledAdministrator(t *testing.T) {
	tx, svc, actor, now := governanceFixture(t)
	_, target := verifiedTestActor(t, tx, "disabled-admin@example.invalid", now)
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='platform_admin',status='disabled',revision=1 WHERE id=?`, target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.ExecuteUserCommand(t.Context(), actor, target.ID, 1, "", "delete", uuid.New()); err != nil {
		t.Fatal("disabled administrator is not the last available administrator", err)
	}
}

func TestPlatformDeleteRollsBackWithoutNotification(t *testing.T) {
	tx, _, actor, now := governanceFixture(t)
	_, target := verifiedTestActor(t, tx, "rollback-delete@example.invalid", now)
	if err := tx.Exec(`UPDATE identity."user" SET revision=1 WHERE id=?`, target.ID).Error; err != nil {
		t.Fatal(err)
	}
	svc := identity.NewGovernance(pgidentity.NewGovernanceStore(tx, nil), nil, func() time.Time { return now })
	key := uuid.New()
	if err := svc.ExecuteUserCommand(t.Context(), actor, target.ID, 1, "", "delete", key); !errors.Is(err, identity.ErrRegistrationUnavailable) {
		t.Fatal("missing notice protector did not fail", err)
	}
	var unchanged bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM identity."user" WHERE id=? AND NOT is_delete AND status='active' AND revision=1)`, target.ID).Scan(&unchanged).Error; err != nil || !unchanged {
		t.Fatal("failed transaction left a deleted account", err)
	}
	if err := pgidentity.NewStore(tx).CheckSession(t.Context(), target, func() time.Time { return now }); err != nil {
		t.Fatal("failed transaction revoked session", err)
	}
	if _, err := svc.UserCommandReceipt(t.Context(), actor, target.ID, key); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatal("failed transaction left receipt", err)
	}
	var events int64
	if err := tx.Raw(`SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action='account.delete'`, target.ID).Scan(&events).Error; err != nil || events != 0 {
		t.Fatal("failed transaction left audit", events, err)
	}
}
