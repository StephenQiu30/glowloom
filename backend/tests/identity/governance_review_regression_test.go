package identity_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func invitationProof(t *testing.T, tx *gorm.DB, email string) string {
	t.Helper()
	var mail struct {
		EventID          uuid.UUID
		KeyID            string
		EncryptedPayload []byte
	}
	if err := tx.Raw(`SELECT event_id,key_id,encrypted_payload FROM identity.mail_delivery WHERE recipient=? AND template='team_invitation' ORDER BY created_at DESC,event_id LIMIT 1`, email).Scan(&mail).Error; err != nil {
		t.Fatal(err)
	}
	p, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := p.Open(mail.EventID, mail.KeyID, mail.EncryptedPayload)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(plain)
	var data struct{ Token string }
	if err := json.Unmarshal(plain, &data); err != nil {
		t.Fatal(err)
	}
	return data.Token
}

func TestTeamInvitationClosureRevokesPending(t *testing.T) {
	for _, acceptFirst := range []bool{false, true} {
		name := "close-before-accept"
		if acceptFirst {
			name = "accept-before-close"
		}
		t.Run(name, func(t *testing.T) {
			tx, svc, actor, now := governanceFixture(t)
			_, target := verifiedTestActor(t, tx, "close-invitation@example.invalid", now)
			if err := tx.Exec(`UPDATE identity.platform_settings SET team_creation_enabled=true`).Error; err != nil {
				t.Fatal(err)
			}
			teams := workspace.NewTeams(pgworkspace.NewTeamStore(tx, func(tx *gorm.DB) workspace.TeamActorAccess {
				return pgidentity.NewTeamActorStore(tx, completionProtector(t))
			}), func() time.Time { return now })
			team, err := teams.Create(t.Context(), actor, uuid.New(), "Invitation lifecycle")
			if err != nil {
				t.Fatal(err)
			}
			invite, err := svc.CreateTeamInvitation(t.Context(), actor, uuid.New(), team.ID, "close-invitation@example.invalid", "member")
			if err != nil {
				t.Fatal(err)
			}
			token := invitationProof(t, tx, "close-invitation@example.invalid")
			if acceptFirst {
				if err := svc.AcceptInvitation(t.Context(), target, token); err != nil {
					t.Fatal(err)
				}
			}
			current, err := teams.Read(t.Context(), actor, team.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := teams.SetInvitations(t.Context(), actor, team.ID, current.Revision, false); err != nil {
				t.Fatal(err)
			}
			var saved struct {
				Status   string
				Revision int64
			}
			if err := tx.Raw(`SELECT status,revision FROM identity.invitation WHERE id=?`, invite.ID).Scan(&saved).Error; err != nil {
				t.Fatal(err)
			}
			expected := "revoked"
			if acceptFirst {
				expected = "accepted"
			}
			if saved.Status != expected || saved.Revision != 2 {
				t.Fatalf("incorrect terminal invitation: %+v", saved)
			}
			if err := teams.SetInvitations(t.Context(), actor, team.ID, current.Revision+1, true); err != nil {
				t.Fatal(err)
			}
			if err := svc.AcceptInvitation(t.Context(), target, token); !errors.Is(err, identity.ErrProofInvalid) {
				t.Fatal("reopening resurrected old invitation", err)
			}
			var members int64
			if err := tx.Raw(`SELECT count(*) FROM workspace.team_member WHERE team_id=? AND user_id=? AND status='active'`, team.ID, target.ID).Scan(&members).Error; err != nil {
				t.Fatal(err)
			}
			want := int64(0)
			if acceptFirst {
				want = 1
			}
			if members != want {
				t.Fatal("invitation close altered accepted membership or admitted stale proof")
			}
		})
	}
}

func TestTeamInvitationClosureAuditRollback(t *testing.T) {
	tx, svc, actor, now := governanceFixture(t)
	if err := tx.Exec(`UPDATE identity.platform_settings SET team_creation_enabled=true`).Error; err != nil {
		t.Fatal(err)
	}
	teams := workspace.NewTeams(pgworkspace.NewTeamStore(tx, func(tx *gorm.DB) workspace.TeamActorAccess {
		return pgidentity.NewTeamActorStore(tx, completionProtector(t))
	}), func() time.Time { return now })
	team, err := teams.Create(t.Context(), actor, uuid.New(), "Atomic invitation switch")
	if err != nil {
		t.Fatal(err)
	}
	invite, err := svc.CreateTeamInvitation(t.Context(), actor, uuid.New(), team.ID, "pending-close@example.invalid", "member")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`ALTER TABLE identity.security_event ADD CONSTRAINT test_invitation_close_audit CHECK(action<>'team.settings_changed') NOT VALID`).Error; err != nil {
		t.Fatal(err)
	}
	if err := teams.SetInvitations(t.Context(), actor, team.ID, 1, false); err == nil {
		t.Fatal("injected audit error ignored")
	}
	var unchanged bool
	if err := tx.Raw(`SELECT t.invitation_enabled AND t.revision=1 AND i.status='pending' AND i.revision=1 FROM workspace.team t JOIN identity.invitation i ON i.team_id=t.id WHERE t.id=? AND i.id=?`, team.ID, invite.ID).Scan(&unchanged).Error; err != nil || !unchanged {
		t.Fatal("switch or invitation escaped rollback", err)
	}
}

func TestRevokingSessionsPreservesCredentialAndTeamQualification(t *testing.T) {
	tx, svc, actor, now := governanceFixture(t)
	ownerAuth, owner := verifiedTestActor(t, tx, "password-required-admin@example.invalid", now)
	_, other := verifiedTestActor(t, tx, "other-team-admin@example.invalid", now)
	if err := tx.Exec(`UPDATE identity.platform_settings SET team_creation_enabled=true`).Error; err != nil {
		t.Fatal(err)
	}
	teams := workspace.NewTeams(pgworkspace.NewTeamStore(tx, func(tx *gorm.DB) workspace.TeamActorAccess {
		return pgidentity.NewTeamActorStore(tx, completionProtector(t))
	}), func() time.Time { return now })
	team, err := teams.Create(t.Context(), owner, uuid.New(), "Password recovery team")
	if err != nil {
		t.Fatal(err)
	}
	var revision int64
	if err := tx.Raw(`SELECT revision FROM identity."user" WHERE id=?`, owner.ID).Scan(&revision).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.RevokeSessions(t.Context(), actor, owner.ID, revision, "Revoke owner sessions"); err != nil {
		t.Fatal(err)
	}
	var blocked bool
	if err := tx.Raw(`SELECT governance_blocked FROM workspace.team WHERE id=?`, team.ID).Scan(&blocked).Error; err != nil || blocked {
		t.Fatal("password recovery froze sole administrator team", err)
	}
	if err := pgidentity.NewStore(tx).CheckSession(t.Context(), owner, now); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("revoked session retained access", err)
	}
	login, err := ownerAuth.Login(t.Context(), "password-required-admin@example.invalid", "long password for tests", false)
	if err != nil {
		t.Fatal(err)
	}
	restrictedActor := identity.Principal{ID: owner.ID, SessionID: login.Session.ID, CredentialEpoch: login.User.SessionEpoch, PlatformEpoch: login.User.PlatformEpoch}
	if err := pgidentity.NewStore(tx).CheckSession(t.Context(), restrictedActor, now); err != nil {
		t.Fatal("fresh session lost normal access", err)
	}
	if err := tx.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',?,'invited')`, team.ID, other.ID, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Raw(`SELECT revision FROM identity."user" WHERE id=?`, other.ID).Scan(&revision).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.Disable(t.Context(), actor, other.ID, revision, "Disable second administrator"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Raw(`SELECT governance_blocked FROM workspace.team WHERE id=?`, team.ID).Scan(&blocked).Error; err != nil || blocked {
		t.Fatal("password-required administrator lost governance qualification", err)
	}
	var restricted bool
	if err := tx.Raw(`SELECT must_change_password FROM identity."user" WHERE id=?`, owner.ID).Scan(&restricted).Error; err != nil || restricted {
		t.Fatal("session revocation introduced password restriction", err)
	}
}

func TestTeamTransferCancellationAuditsCurrentReason(t *testing.T) {
	tx, svc, actor, now := governanceFixture(t)
	_, target := verifiedTestActor(t, tx, "cancel-transfer-target@example.invalid", now)
	if err := tx.Exec(`UPDATE identity.platform_settings SET team_creation_enabled=true`).Error; err != nil {
		t.Fatal(err)
	}
	teams := workspace.NewTeams(pgworkspace.NewTeamStore(tx, func(tx *gorm.DB) workspace.TeamActorAccess {
		return pgidentity.NewTeamActorStore(tx, completionProtector(t))
	}), func() time.Time { return now })
	team, err := teams.Create(t.Context(), actor, uuid.New(), "Cancelled team transfer")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'member','active',?,'invited')`, team.ID, target.ID, now).Error; err != nil {
		t.Fatal(err)
	}
	proposal, err := svc.ProposeTransfer(t.Context(), actor, team.ID, target.ID, team.Revision, "Original handover rationale")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.CancelTransfer(t.Context(), actor, team.ID, proposal.ID, 1, "   "); !errors.Is(err, identity.ErrInvalidGovernance) {
		t.Fatal("missing transfer cancellation reason accepted", err)
	}
	if err := svc.CancelTransfer(t.Context(), actor, team.ID, proposal.ID, 1, "  Withdraw after team review  "); err != nil {
		t.Fatal(err)
	}
	var reason string
	if err := tx.Raw(`SELECT reason FROM identity.security_event WHERE team_id=? AND target_user_id=? AND action='authority.cancelled'`, team.ID, target.ID).Scan(&reason).Error; err != nil || reason != "Withdraw after team review" {
		t.Fatal("transfer cancellation audited original reason", err)
	}
}
