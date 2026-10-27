package identity_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func governanceFixture(t *testing.T) (*gorm.DB, *identity.Governance, identity.Principal, time.Time) {
	t.Helper()
	// CI supplies the isolated canvas schema; governance uses the same installed identity tables.
	if os.Getenv("GLOWLOOM_TEST_EXISTING_DB_DSN") == "" && os.Getenv("GLOWLOOM_TEST_CANVAS_DB_DSN") != "" {
		t.Setenv("GLOWLOOM_TEST_EXISTING_DB_DSN", os.Getenv("GLOWLOOM_TEST_CANVAS_DB_DSN"))
	}
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, actor := verifiedTestActor(t, tx, "governance-admin@example.invalid", now)
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	actor.PlatformRole = "platform_admin"
	if err := tx.Exec(`UPDATE identity."user" SET revision=1 WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET reauth_at=?,reauth_epoch=? WHERE id=?`, now, actor.CredentialEpoch, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	protector, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	svc := identity.NewGovernance(pgidentity.NewGovernanceStore(tx, func(tx *gorm.DB) identity.TeamGovernance { return pgworkspace.NewTeamGovernance(tx) }), protector, func() time.Time { return now })
	return tx, svc, actor, now
}
func TestPlatformGovernanceAtomicFreeze(t *testing.T) {
	tx, svc, actor, now := governanceFixture(t)
	_, target := verifiedTestActor(t, tx, "governance-target@example.invalid", now)
	if err := tx.Exec(`UPDATE identity."user" SET revision=1 WHERE id=?`, target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET reauth_at=?,reauth_epoch=? WHERE id=?`, now, target.CredentialEpoch, target.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity.platform_settings SET team_creation_enabled=true`).Error; err != nil {
		t.Fatal(err)
	}
	teams := workspace.NewTeams(pgworkspace.NewTeamStore(tx, func(tx *gorm.DB) workspace.TeamActorAccess {
		return pgidentity.NewTeamActorStore(tx, completionProtector(t))
	}), func() time.Time { return now })
	team, err := teams.Create(t.Context(), target, uuid.New(), "Governed team")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Disable(t.Context(), target, actor.ID, 1, "Unauthorized mutation"); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("ordinary user bypass: %v", err)
	}
	if err := svc.Disable(t.Context(), actor, target.ID, 1, "Account abuse confirmed"); err != nil {
		t.Fatal(err)
	}
	var state struct {
		Status            string
		GovernanceBlocked bool
		Revision          int64
	}
	if err := tx.Raw(`SELECT t.governance_blocked,u.status,u.revision FROM workspace.team t JOIN identity."user" u ON u.id=? WHERE t.id=?`, target.ID, team.ID).Scan(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.Status != "disabled" || !state.GovernanceBlocked || state.Revision != 2 {
		t.Fatalf("non-atomic freeze: %+v", state)
	}
	if err := svc.Enable(t.Context(), actor, target.ID, 1, "Restore after review"); !errors.Is(err, domain.ErrRevisionConflict) {
		t.Fatal("stale write accepted", err)
	}
	if err := svc.Enable(t.Context(), actor, target.ID, 2, "Restore after review"); err != nil {
		t.Fatal(err)
	}
	if err := pgidentity.NewStore(tx).CheckSession(t.Context(), target, func() time.Time { return now }); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("old session resurrected", err)
	}
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM identity.mail_delivery WHERE recipient='governance-target@example.invalid' AND template='account_governance' AND encrypted_payload IS NOT NULL`).Scan(&count).Error; err != nil || count != 2 {
		t.Fatal("missing encrypted notifications", count, err)
	}
}
func TestPlatformGrantExplicitAcceptanceAndAuthority(t *testing.T) {
	tx, svc, actor, now := governanceFixture(t)
	_, target := verifiedTestActor(t, tx, "grant-target@example.invalid", now)
	if err := tx.Exec(`UPDATE identity."user" SET revision=1 WHERE id=?`, target.ID).Error; err != nil {
		t.Fatal(err)
	}
	proposal, err := svc.ProposeAdmin(t.Context(), actor, target.ID, 1, "Grant platform administration")
	if err != nil {
		t.Fatal(err)
	}
	var role string
	if err := tx.Raw(`SELECT platform_role FROM identity."user" WHERE id=?`, target.ID).Scan(&role).Error; err != nil || role != "creator" {
		t.Fatal("grant before acceptance", err)
	}
	if err := svc.AcceptProposal(t.Context(), target, proposal.ID, 1); !errors.Is(err, identity.ErrReauthenticationRequired) {
		t.Fatal("accept lacks recent proof", err)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET reauth_at=?,reauth_epoch=? WHERE id=?`, now, target.CredentialEpoch, target.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='creator',platform_epoch=platform_epoch+1 WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.AcceptProposal(t.Context(), target, proposal.ID, 1); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("revoked source granted authority", err)
	}
}
func TestGovernanceSettingsVersionAndLastAdmin(t *testing.T) {
	tx, svc, actor, _ := governanceFixture(t)
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='creator' WHERE id<>?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.RevokeAdmin(t.Context(), actor, actor.ID, 1, "Remove own admin role"); !errors.Is(err, identity.ErrLastPlatformAdmin) {
		t.Fatal("last administrator lost", err)
	}
	settings, err := svc.Settings(t.Context(), actor)
	if err != nil {
		t.Fatal(err)
	}
	disabled := false
	changed, err := svc.UpdateSettings(t.Context(), actor, settings.Revision, &disabled, &disabled, "Close public entry points")
	if err != nil {
		t.Fatal(err)
	}
	if changed.RegistrationEnabled || changed.TeamCreationEnabled || changed.Revision != settings.Revision+1 {
		t.Fatal("settings mismatch")
	}
	if _, err := svc.UpdateSettings(t.Context(), actor, settings.Revision, &disabled, nil, "Stale settings update"); !errors.Is(err, domain.ErrRevisionConflict) {
		t.Fatal("stale settings accepted", err)
	}
}

func TestTeamTransferAndInvitationAcceptance(t *testing.T) {
	tx, svc, actor, now := governanceFixture(t)
	_, target := verifiedTestActor(t, tx, "invited-recipient@example.invalid", now)
	if err := tx.Exec(`UPDATE identity.platform_settings SET team_creation_enabled=true`).Error; err != nil {
		t.Fatal(err)
	}
	teams := workspace.NewTeams(pgworkspace.NewTeamStore(tx, func(tx *gorm.DB) workspace.TeamActorAccess {
		return pgidentity.NewTeamActorStore(tx, completionProtector(t))
	}), func() time.Time { return now })
	team, err := teams.Create(t.Context(), actor, uuid.New(), "Invitation team")
	if err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	invitation, err := svc.CreateTeamInvitation(t.Context(), actor, key, team.ID, "invited-recipient@example.invalid", "member")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := svc.CreateTeamInvitation(t.Context(), actor, key, team.ID, "invited-recipient@example.invalid", "member")
	if err != nil || replay.ID != invitation.ID {
		t.Fatal("invitation create was not idempotent", err)
	}
	var mail struct {
		EventID          uuid.UUID
		KeyID            string
		EncryptedPayload []byte
	}
	if err := tx.Raw(`SELECT event_id,key_id,encrypted_payload FROM identity.mail_delivery WHERE recipient=? AND template='team_invitation'`, "invited-recipient@example.invalid").Scan(&mail).Error; err != nil {
		t.Fatal(err)
	}
	protector, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := protector.Open(mail.EventID, mail.KeyID, mail.EncryptedPayload)
	if err != nil {
		t.Fatal(err)
	}
	var data struct{ Token string }
	if err := json.Unmarshal(payload, &data); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(mail.EncryptedPayload, []byte(data.Token)) {
		t.Fatal("plaintext invitation stored")
	}
	if err := svc.AcceptInvitation(t.Context(), actor, data.Token); !errors.Is(err, identity.ErrProofInvalid) {
		t.Fatal("wrong email accepted invitation", err)
	}
	if err := svc.AcceptInvitation(t.Context(), target, data.Token); err != nil {
		t.Fatal(err)
	}
	if err := svc.AcceptInvitation(t.Context(), target, data.Token); !errors.Is(err, identity.ErrProofInvalid) {
		t.Fatal("double acceptance", err)
	}
	current, err := teams.Read(t.Context(), actor, team.ID)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := svc.ProposeTransfer(t.Context(), actor, team.ID, target.ID, current.Revision, "Transfer creative leadership")
	if err != nil {
		t.Fatal(err)
	}
	if err := teams.Archive(t.Context(), actor, team.ID, current.Revision, "Archive pending transfer"); !errors.Is(err, workspace.ErrTeamStateConflict) {
		t.Fatal("pending transfer archived", err)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET reauth_at=?,reauth_epoch=? WHERE id=?`, now, target.CredentialEpoch, target.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.AcceptProposal(t.Context(), target, proposal.ID, 1); err != nil {
		t.Fatal(err)
	}
	current, err = teams.Read(t.Context(), target, team.ID)
	if err != nil || current.ActorRole != "team_admin" {
		t.Fatal("transfer did not grant target", err)
	}
	old, err := teams.Read(t.Context(), actor, team.ID)
	if err != nil || old.ActorRole != "member" {
		t.Fatal("transfer left source administrator", err)
	}
	if err := teams.Archive(t.Context(), target, team.ID, current.Revision, "Archive after handover"); err != nil {
		t.Fatal(err)
	}
	archived, err := teams.Read(t.Context(), target, team.ID)
	if err != nil || archived.Status != "archived" {
		t.Fatal("archive missing", err)
	}
	if err := teams.Restore(t.Context(), target, team.ID, archived.Revision, "Restore creative team"); err != nil {
		t.Fatal(err)
	}
}
func TestPlatformGovernanceAuditFailureRollback(t *testing.T) {
	tx, svc, actor, now := governanceFixture(t)
	_, target := verifiedTestActor(t, tx, "rollback-target@example.invalid", now)
	if err := tx.Exec(`UPDATE identity."user" SET revision=1 WHERE id=?`, target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`ALTER TABLE identity.security_event ADD CONSTRAINT test_governance_audit_fail CHECK(action<>'account.disable') NOT VALID`).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.Disable(t.Context(), actor, target.ID, 1, "Audit must remain atomic"); err == nil {
		t.Fatal("injected audit failure succeeded")
	}
	var status string
	if err := tx.Raw(`SELECT status FROM identity."user" WHERE id=?`, target.ID).Scan(&status).Error; err != nil || status != "active" {
		t.Fatal("account mutation survived audit rollback", err)
	}
}

func TestInvitationScopeAndMembershipRevision(t *testing.T) {
	tx, svc, actor, now := governanceFixture(t)
	_, target := verifiedTestActor(t, tx, "old-invitation@example.invalid", now)
	if err := tx.Exec(`UPDATE identity.platform_settings SET team_creation_enabled=true`).Error; err != nil {
		t.Fatal(err)
	}
	teams := workspace.NewTeams(pgworkspace.NewTeamStore(tx, func(tx *gorm.DB) workspace.TeamActorAccess {
		return pgidentity.NewTeamActorStore(tx, completionProtector(t))
	}), func() time.Time { return now })
	team, err := teams.Create(t.Context(), actor, uuid.New(), "Invitation safety")
	if err != nil {
		t.Fatal(err)
	}
	invite, err := svc.CreateTeamInvitation(t.Context(), actor, uuid.New(), team.ID, "old-invitation@example.invalid", "member")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RevokeInvitation(t.Context(), actor, invite.ID, 1, "Wrong account endpoint", nil); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("team invitation mutated through platform account route", err)
	}
	other := uuid.New()
	if err := svc.RevokeInvitation(t.Context(), actor, invite.ID, 1, "Wrong team endpoint", &other); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("invitation accepted mismatched team", err)
	}
	if err := tx.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source,revision) VALUES(?,?,'member','removed',?,'invited',3)`, team.ID, target.ID, now).Error; err != nil {
		t.Fatal(err)
	}
	// An invitation issued before a member's remove cycle cannot resurrect that row.
	gateway := pgworkspace.NewTeamGovernance(tx)
	if err := gateway.AcceptInvitation(invite, target.ID, now); !errors.Is(err, workspace.ErrTeamRevisionConflict) {
		t.Fatal("old invitation revived removed membership", err)
	}
	if err := svc.RevokeInvitation(t.Context(), actor, invite.ID, 1, "Withdraw obsolete invitation", &team.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.RevokeInvitation(t.Context(), actor, invite.ID, 1, "Withdraw obsolete invitation", &team.ID); !errors.Is(err, domain.ErrRevisionConflict) {
		t.Fatal("repeat mutation duplicated side effects", err)
	}
}

func TestGovernanceUnlockUsesHashedLoginGuard(t *testing.T) {
	tx, svc, actor, now := governanceFixture(t)
	_, target := verifiedTestActor(t, tx, "unlock-target@example.invalid", now)
	if err := tx.Exec(`UPDATE identity."user" SET revision=1 WHERE id=?`, target.ID).Error; err != nil {
		t.Fatal(err)
	}
	protector, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	key := hex.EncodeToString(protector.Digest("login_guard", uuid.Nil, "unlock-target@example.invalid"))
	if err := tx.Exec(`UPDATE identity.login_guard SET locked_until=? WHERE login_key=?`, now.Add(time.Hour), key).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.Unlock(t.Context(), actor, target.ID, 1, "Unlock after owner verification"); err != nil {
		t.Fatal(err)
	}
	var locked bool
	if err := tx.Raw(`SELECT locked_until IS NOT NULL FROM identity.login_guard WHERE login_key=?`, key).Scan(&locked).Error; err != nil || locked {
		t.Fatal("email login guard still locked", err)
	}
}

func TestGovernanceTakeoverRequiresAcceptance(t *testing.T) {
	tx, svc, actor, now := governanceFixture(t)
	_, owner := verifiedTestActor(t, tx, "frozen-owner@example.invalid", now)
	_, successor := verifiedTestActor(t, tx, "takeover-target@example.invalid", now)
	if err := tx.Exec(`UPDATE identity."user" SET revision=1 WHERE id=?`, owner.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity.platform_settings SET team_creation_enabled=true`).Error; err != nil {
		t.Fatal(err)
	}
	teams := workspace.NewTeams(pgworkspace.NewTeamStore(tx, func(tx *gorm.DB) workspace.TeamActorAccess {
		return pgidentity.NewTeamActorStore(tx, completionProtector(t))
	}), func() time.Time { return now })
	team, err := teams.Create(t.Context(), owner, uuid.New(), "Frozen team")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Disable(t.Context(), actor, owner.ID, 1, "Emergency owner suspension"); err != nil {
		t.Fatal(err)
	}
	proposal, err := svc.ProposeTakeover(t.Context(), actor, team.ID, successor.ID, 2, "Recover unmanaged team")
	if err != nil {
		t.Fatal(err)
	}
	var blocked bool
	if err := tx.Raw(`SELECT governance_blocked FROM workspace.team WHERE id=?`, team.ID).Scan(&blocked).Error; err != nil || !blocked {
		t.Fatal("proposal unblocked team before acceptance", err)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET reauth_at=?,reauth_epoch=? WHERE id=?`, now, successor.CredentialEpoch, successor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.AcceptProposal(t.Context(), successor, proposal.ID, 1); err != nil {
		t.Fatal(err)
	}
	restored, err := teams.Read(t.Context(), successor, team.ID)
	if err != nil || restored.GovernanceBlocked || restored.ActorRole != "team_admin" {
		t.Fatal("accepted takeover did not repair governance", err)
	}
	if err := svc.AcceptProposal(t.Context(), successor, proposal.ID, 1); !errors.Is(err, domain.ErrRevisionConflict) {
		t.Fatal("takeover replay applied twice", err)
	}
}
