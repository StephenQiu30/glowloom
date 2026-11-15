package identityregistration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	pgcreation "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/pgsource"
	creation "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func governanceSourceFixture(t *testing.T) (*registrationFixture, *identity.Governance, identity.PasswordChangeInput, uuid.UUID) {
	t.Helper()
	f, password := passwordFixture(t)
	current, err := f.auth.Current(t.Context(), password.Credential.LegacyToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.idb.Exec(`UPDATE identity."user" SET platform_role='platform_admin',platform_epoch=platform_epoch+1 WHERE id=?`, current.User.ID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.idb.Exec(`UPDATE identity."user" SET platform_role='creator' WHERE id=?`, current.User.ID).Error; err != nil {
			t.Error(err)
		}
	})
	if err := f.auth.Reauthenticate(t.Context(), password.Credential.LegacyToken, password.CurrentPassword); err != nil {
		t.Fatal(err)
	}
	store := pgidentity.NewSourceGovernanceStore(f.idb)
	service := identity.NewGovernance(store, f.protector, func() time.Time { return f.clock })
	if err := service.WithSource(f.auth, store, f.writers, f.protector); err != nil {
		t.Fatal(err)
	}
	return f, service, password, current.User.ID
}

func TestGovernanceSourceSessionRevocationWaitsForClosureAndOriginalProofRecovers(t *testing.T) {
	f, service, password, _ := governanceSourceFixture(t)
	target, targetPassword := passwordFixture(t)
	current, err := target.auth.Current(t.Context(), targetPassword.Credential.LegacyToken)
	if err != nil {
		t.Fatal(err)
	}
	id := current.User.ID
	f.clock = time.Now().UTC().Truncate(time.Microsecond)
	input := identity.GovernanceSourceCommand{RequestID: uuid.New(), TargetID: id, Revision: current.User.Revision, Action: "revoke_sessions", Reason: "revoke synthetic account sessions", Credential: password.Credential}
	f.controlLoss.kind = writefence.Account
	f.controlLoss.lose.Store(true)
	if err := service.ExecuteSourceCommand(t.Context(), input); err == nil {
		t.Fatal("qualification succeeded before four close ACKs")
	}
	if _, err := target.auth.Current(t.Context(), targetPassword.Credential.LegacyToken); err != nil {
		t.Fatal("target session revoked before source closure ACKs")
	}
	caller, err := f.auth.Current(t.Context(), password.Credential.LegacyToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.idb.Exec(`UPDATE identity.user_session SET revoked_at=statement_timestamp() WHERE id=?`, caller.Session.ID).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := service.ExecuteSourceCommand(t.Context(), input); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := target.auth.Current(t.Context(), targetPassword.Credential.LegacyToken); err == nil {
		t.Fatal("target session survived governance revocation")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action='account.revoke_sessions'`, id) != 1 {
		t.Fatal("governance replay repeated mutation/audit")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.account_qualification_change WHERE request_id=? AND outcome='accepted' AND mutation_applied_at IS NOT NULL AND completed_at IS NOT NULL`, input.RequestID) != 1 {
		t.Fatal("source responsibility not complete")
	}
}

func TestGovernanceSourceLastAvailableAdminIncludesPendingRemoval(t *testing.T) {
	f, first, firstPassword, firstID := governanceSourceFixture(t)
	_, second, secondPassword, secondID := governanceSourceFixture(t)
	f.controlLoss.kind = writefence.Account
	f.controlLoss.lose.Store(true)
	var oneRevision, twoRevision int64
	if err := f.idb.Raw(`SELECT revision FROM identity."user" WHERE id=?`, firstID).Scan(&oneRevision).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.idb.Raw(`SELECT revision FROM identity."user" WHERE id=?`, secondID).Scan(&twoRevision).Error; err != nil {
		t.Fatal(err)
	}
	one := identity.GovernanceSourceCommand{RequestID: uuid.New(), TargetID: firstID, Revision: oneRevision, Action: "revoke_admin", Reason: "remove synthetic platform privilege", Credential: firstPassword.Credential}
	if err := first.ExecuteSourceCommand(t.Context(), one); err == nil {
		t.Fatal("close ACK loss not exercised")
	}
	two := identity.GovernanceSourceCommand{RequestID: uuid.New(), TargetID: secondID, Revision: twoRevision, Action: "revoke_admin", Reason: "remove synthetic platform privilege", Credential: secondPassword.Credential}
	if err := second.ExecuteSourceCommand(t.Context(), two); !errors.Is(err, identity.ErrLastPlatformAdmin) {
		t.Fatalf("pending removal counted as available admin: %v", err)
	}
	if err := first.ExecuteSourceCommand(t.Context(), one); err != nil {
		t.Fatal(err)
	}
	if err := first.ExecuteSourceCommand(t.Context(), one); err != nil {
		t.Fatal("original administrator proof could not recover its own removed privilege")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE id=? AND platform_role='platform_admin'`, secondID) != 1 {
		t.Fatal("last platform administrator removed")
	}
}

type testCreationGovernanceParticipant struct{ service *creation.AccountGovernance }

func ownedGovernanceReceipt(r creation.AccountGovernanceReceipt) identity.GovernanceTeamReceipt {
	return identity.GovernanceTeamReceipt{RequestID: r.RequestID, AccountID: r.AccountID, Action: r.Action, InputSHA256: r.InputSHA256, IdentityClose: r.IdentityClose, RejectedDuties: r.RejectedDuties, BlockingTeams: r.BlockingTeams, Teams: r.Teams}
}
func (p testCreationGovernanceParticipant) PrepareAccountGovernance(ctx context.Context, intent identity.AccountQualificationIntent) (identity.GovernanceTeamReceipt, error) {
	r, err := p.service.Prepare(ctx, creationIntent(intent))
	return ownedGovernanceReceipt(r), err
}
func (p testCreationGovernanceParticipant) ResolveAccountGovernance(ctx context.Context, intent identity.AccountQualificationIntent) (identity.GovernanceTeamReceipt, error) {
	r, err := p.service.Resolve(ctx, intent.RequestID)
	return ownedGovernanceReceipt(r), err
}

func TestGovernanceSourceActualTeamDisableEnableRejectAndDelete(t *testing.T) {
	f, service, password, _ := governanceSourceFixture(t)
	target, targetPassword := passwordFixture(t)
	other, otherPassword := passwordFixture(t)
	targetUser, err := target.auth.Current(t.Context(), targetPassword.Credential.LegacyToken)
	if err != nil {
		t.Fatal(err)
	}
	otherUser, err := other.auth.Current(t.Context(), otherPassword.Credential.LegacyToken)
	if err != nil {
		t.Fatal(err)
	}
	team, scope, organization := uuid.New(), uuid.New(), uuid.New()
	if err := f.cdb.Exec(`INSERT INTO workspace.organization(id,name) VALUES(?,'account governance business fixture')`, organization).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.cdb.Exec(`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'account governance business fixture',?)`, team, targetUser.User.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.cdb.Exec(`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, scope, team, organization).Error; err != nil {
		t.Fatal(err)
	}
	for _, member := range []struct {
		id   uuid.UUID
		role string
	}{{targetUser.User.ID, "team_admin"}, {otherUser.User.ID, "member"}} {
		if err := f.cdb.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,?,'active',statement_timestamp(),'created')`, team, member.id, member.role).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.creationWriters.Set(t.Context(), writefence.Authority{Issuer: writefence.Creation, Kind: writefence.Scope, ID: scope, Epoch: 0}, uuid.New(), false); err != nil {
		t.Fatal(err)
	}
	creator, err := creation.NewAccountGovernance(pgcreation.NewAccountGovernanceStore(f.cdb), directGovernanceQualification{source: identity.NewTeamQualifications(pgidentity.NewTeamQualificationStore(f.idb))}, f.creationWriters)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.WithTeamGovernance(testCreationGovernanceParticipant{service: creator}); err != nil {
		t.Fatal(err)
	}
	input := identity.GovernanceSourceCommand{RequestID: uuid.New(), TargetID: targetUser.User.ID, Revision: targetUser.User.Revision, Action: "disable", Reason: "synthetic original account governance", Credential: password.Credential}
	if err := service.ExecuteSourceCommand(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE id=? AND status='disabled' AND session_epoch=? AND platform_epoch=? AND revision=?`, targetUser.User.ID, targetUser.User.SessionEpoch+1, targetUser.User.PlatformEpoch+1, targetUser.User.Revision+1) != 1 {
		t.Fatal("owned disable mutation/counters not committed")
	}
	if count(t, f.cdb, `SELECT count(*) FROM workspace.team WHERE id=? AND governance_blocked AND revision=2 AND auth_epoch=2`, team) != 1 {
		t.Fatal("actual loss of last admin did not freeze team")
	}
	if count(t, f.cdb, `SELECT count(*) FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='creation' AND authority_kind='scope' AND authority_id=? AND epoch=2 AND blocked AND complete`, scope) != 1 {
		t.Fatal("actual permanent freeze lacked source four ACKs")
	}
	input.RequestID = uuid.New()
	input.Revision = targetUser.User.Revision + 1
	input.Action = "enable"
	if err := service.ExecuteSourceCommand(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE id=? AND status='active' AND session_epoch=? AND platform_epoch=? AND revision=?`, targetUser.User.ID, targetUser.User.SessionEpoch+1, targetUser.User.PlatformEpoch+1, targetUser.User.Revision+2) != 1 {
		t.Fatal("enable restored old sessions or changed original counters")
	}
	if count(t, f.cdb, `SELECT count(*) FROM workspace.team WHERE id=? AND governance_blocked AND revision=3 AND auth_epoch=3`, team) != 1 {
		t.Fatal("enable automatically restored a frozen team")
	}
	input.RequestID = uuid.New()
	input.Revision = targetUser.User.Revision + 2
	input.Action = "delete"
	if err := service.ExecuteSourceCommand(t.Context(), input); !errors.Is(err, identity.ErrDeletionDuties) {
		t.Fatalf("last team administrator deletion was not rejected: %v", err)
	}
	if err := service.ExecuteSourceCommand(t.Context(), input); !errors.Is(err, identity.ErrDeletionDuties) {
		t.Fatal("original rejected decision did not replay")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE id=? AND NOT is_delete AND status='active' AND revision=?`, targetUser.User.ID, targetUser.User.Revision+2) != 1 {
		t.Fatal("duties rejection changed target account")
	}
	if count(t, f.idb, `SELECT count(*) FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='identity' AND authority_kind='account' AND authority_id=? AND epoch=6 AND NOT blocked AND complete`, targetUser.User.ID) != 1 {
		t.Fatal("duties rejection did not durably restore original account policy")
	}
	if count(t, f.cdb, `SELECT count(*) FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='creation' AND authority_kind='scope' AND authority_id=? AND epoch=2 AND blocked AND complete`, scope) != 1 {
		t.Fatal("duties rejection changed shared scope epoch")
	}
	if err := f.cdb.Exec(`UPDATE workspace.team_member SET role='team_admin',revision=revision+1,auth_epoch=auth_epoch+1 WHERE team_id=? AND user_id=?`, team, otherUser.User.ID).Error; err != nil {
		t.Fatal(err)
	}
	input.RequestID = uuid.New()
	if err := service.ExecuteSourceCommand(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE id=? AND is_delete AND status='disabled' AND session_epoch=? AND platform_epoch=? AND revision=?`, targetUser.User.ID, targetUser.User.SessionEpoch+2, targetUser.User.PlatformEpoch+2, targetUser.User.Revision+3) != 1 {
		t.Fatal("ready Creation receipt did not commit owned deletion")
	}
	if count(t, f.cdb, `SELECT count(*) FROM workspace.team_member WHERE team_id=? AND user_id=? AND status='removed'`, team, targetUser.User.ID) != 1 {
		t.Fatal("Creation did not remove retained target membership")
	}
	if err := service.ExecuteSourceCommand(t.Context(), input); err != nil {
		t.Fatal("completed delete could not replay original proof")
	}
}

func TestGovernanceSourceDisabledAdminRemovalNeverReopensAccount(t *testing.T) {
	f, service, password, _ := governanceSourceFixture(t)
	target, _, _, targetID := governanceSourceFixture(t)
	if err := target.idb.Exec(`UPDATE identity."user" SET status='disabled',session_epoch=session_epoch+1,platform_epoch=platform_epoch+1,revision=revision+1 WHERE id=?`, targetID).Error; err != nil {
		t.Fatal(err)
	}
	if err := target.idb.Exec(`UPDATE identity.user_session SET revoked_at=statement_timestamp() WHERE account_id=?`, targetID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := target.writers.Set(t.Context(), writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: targetID, Epoch: 1}, uuid.New(), true); err != nil {
		t.Fatal(err)
	}
	f.clock = time.Now().UTC().Truncate(time.Microsecond)
	var version int64
	if err := f.idb.Raw(`SELECT revision FROM identity."user" WHERE id=?`, targetID).Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	input := identity.GovernanceSourceCommand{RequestID: uuid.New(), TargetID: targetID, Revision: version, Action: "revoke_admin", Reason: "remove disabled synthetic privilege", Credential: password.Credential}
	if err := service.ExecuteSourceCommand(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	p, err := pgidentity.NewSourceGovernanceStore(f.idb).LoadAcceptedGovernance(t.Context(), input.RequestID)
	if err != nil || !p.Completed || p.ReopenControl.CoordinationID != uuid.Nil || !p.CloseControl.Blocked || p.CloseControl.Epoch != 3 {
		t.Fatalf("disabled admin removal reopened qualification: %v", err)
	}
	if count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE id=? AND status='disabled' AND platform_role='creator' AND session_epoch=2`, targetID) != 1 {
		t.Fatal("disabled administrator role removal changed unrelated qualification")
	}
}
