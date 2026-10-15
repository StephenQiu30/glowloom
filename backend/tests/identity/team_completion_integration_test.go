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

func completionProtector(t *testing.T) *credentialproof.Protector {
	t.Helper()
	p, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func completionPayload(t *testing.T, tx *gorm.DB, recipient, template string) map[string]string {
	t.Helper()
	var m struct {
		EventID          uuid.UUID
		KeyID            string
		EncryptedPayload []byte
	}
	if err := tx.Raw(`SELECT event_id,key_id,encrypted_payload FROM identity.mail_delivery WHERE recipient=? AND template=? ORDER BY created_at DESC,event_id DESC LIMIT 1`, recipient, template).Scan(&m).Error; err != nil {
		t.Fatal(err)
	}
	plain, err := completionProtector(t).Open(m.EventID, m.KeyID, m.EncryptedPayload)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	if err = json.Unmarshal(plain, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}
func TestInvitedRegistrationFinalRecheck(t *testing.T) {
	tx, gov, actor, now := governanceFixture(t)
	if err := tx.Exec(`UPDATE identity.platform_settings SET registration_enabled=false,team_creation_enabled=true`).Error; err != nil {
		t.Fatal(err)
	}
	teams := workspace.NewTeams(pgworkspace.NewTeamStore(tx, func(tx *gorm.DB) workspace.TeamActorAccess {
		return pgidentity.NewTeamActorStore(tx, completionProtector(t))
	}), func() time.Time { return now })
	team, err := teams.Create(t.Context(), actor, uuid.New(), "Invited signup")
	if err != nil {
		t.Fatal(err)
	}
	i, err := gov.CreateTeamInvitation(t.Context(), actor, uuid.New(), team.ID, "new-invited@example.invalid", "member")
	if err != nil {
		t.Fatal(err)
	}
	token := completionPayload(t, tx, i.EmailKey, "team_invitation")["token"]
	store := pgidentity.NewInvitationEmailStore(tx, func(tx *gorm.DB) identity.TeamGovernance { return pgworkspace.NewTeamGovernance(tx) })
	auth := identity.NewEmailAuth(store, completionProtector(t), identity.EmailPolicy{TermsVersion: "test", SourceHourlyLimit: 100, GlobalHourlyLimit: 1000}, func() time.Time { return now })
	if _, err := auth.StartRegistration(t.Context(), i.EmailKey, ""); !errors.Is(err, identity.ErrRegistrationUnavailable) {
		t.Fatalf("closed public registration: %v", err)
	}
	start, err := auth.StartRegistration(t.Context(), i.EmailKey, token)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := auth.VerifyRegistration(t.Context(), start.OperationID, start.FlowSecret, completionPayload(t, tx, i.EmailKey, "registration_code")["code"])
	if err != nil {
		t.Fatal(err)
	}
	if err := gov.RevokeInvitation(t.Context(), actor, i.ID, i.Revision, "Withdraw invitation proof", &team.ID); err != nil {
		t.Fatal(err)
	}
	_, err = auth.CompleteRegistration(t.Context(), identity.CompleteRegistrationInput{OperationID: start.OperationID, FlowSecret: start.FlowSecret, Grant: proof.Grant, DisplayName: "Invited", Password: "long invited password", ConfirmPassword: "long invited password", TermsAccepted: true, TermsVersion: "test"})
	if !errors.Is(err, identity.ErrProofInvalid) {
		t.Fatalf("revoked grant accepted: %v", err)
	}
	jobs, err := pgidentity.NewMailStore(tx).Claim(t.Context(), 100, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range jobs {
		if job.Recipient == i.EmailKey && (job.Template == "registration_code" || job.Template == "team_invitation") {
			t.Fatal("revoked invited proof mail remained deliverable")
		}
	}
}
func TestProposalLoginAndReceipt(t *testing.T) {
	tx, gov, actor, now := governanceFixture(t)
	auth, target := verifiedTestActor(t, tx, "proposal-login@example.invalid", now)
	var revision int64
	tx.Raw(`SELECT revision FROM identity."user" WHERE id=?`, target.ID).Scan(&revision)
	key := uuid.New()
	p, err := gov.ProposeAdminWithKey(t.Context(), actor, key, target.ID, revision, "Grant explicit platform role")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := gov.ProposeAdminWithKey(t.Context(), actor, key, target.ID, revision, "Grant explicit platform role")
	if err != nil || replay.ID != p.ID {
		t.Fatalf("receipt replay: %v", err)
	}
	login, err := auth.Login(t.Context(), "proposal-login@example.invalid", "long password for tests", false)
	if err != nil {
		t.Fatal(err)
	}
	if err = auth.Reauthenticate(t.Context(), login.Token, "long password for tests"); err != nil {
		t.Fatal(err)
	}
	target = identity.Principal{ID: login.User.ID, SessionID: login.Session.ID, CredentialEpoch: login.User.SessionEpoch, PlatformEpoch: login.User.PlatformEpoch}
	if err = gov.AcceptProposal(t.Context(), target, p.ID, p.Revision); err != nil {
		t.Fatalf("login invalidated acceptance: %v", err)
	}
}

func TestPlatformTeamRestoreAndDirectory(t *testing.T) {
	tx, _, actor, now := governanceFixture(t)
	_, member := verifiedTestActor(t, tx, "platform-team-owner@example.invalid", now)
	if err := tx.Exec(`UPDATE identity.platform_settings SET team_creation_enabled=true`).Error; err != nil {
		t.Fatal(err)
	}
	store := pgworkspace.NewTeamStore(tx, func(tx *gorm.DB) workspace.TeamActorAccess {
		return pgidentity.NewTeamActorStore(tx, completionProtector(t))
	})
	teams := workspace.NewTeams(store, func() time.Time { return now })
	team, err := teams.Create(t.Context(), member, uuid.New(), "Archived platform team")
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Exec(`UPDATE workspace.team SET status='archived' WHERE id=?`, team.ID).Error; err != nil {
		t.Fatal(err)
	}
	service := workspace.NewPlatformTeams(store, completionProtector(t), func() time.Time { return now })
	if _, err = service.Read(t.Context(), member, team.ID); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member entered platform team directory: %v", err)
	}
	if err = service.Suspend(t.Context(), actor, team.ID, 1, "Suspend after review"); err != nil {
		t.Fatal(err)
	}
	if err = service.Restore(t.Context(), actor, team.ID, 2, "Restore after review"); err != nil {
		t.Fatal(err)
	}
	got, err := service.Read(t.Context(), actor, team.ID)
	if err != nil || got.Status != "archived" {
		t.Fatalf("restore lost archive: %+v %v", got, err)
	}
	if _, err := teams.Create(t.Context(), member, uuid.New(), "Archived platform second"); err != nil {
		t.Fatal(err)
	}
	page, err := service.List(t.Context(), actor, workspace.PlatformTeamFilter{Search: "Archived platform", Limit: 1})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("directory: %+v %v", page, err)
	}
	if page.NextCursor == nil {
		t.Fatal("platform team continuation missing")
	}
	next, err := service.List(t.Context(), actor, workspace.PlatformTeamFilter{Search: "Archived platform", Limit: 1, Cursor: *page.NextCursor})
	if err != nil || len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID {
		t.Fatalf("platform keyset failed: %v", err)
	}
	if _, err = service.List(t.Context(), actor, workspace.PlatformTeamFilter{Search: "Archived platform", Limit: 1, Status: "active", Cursor: *page.NextCursor}); !errors.Is(err, workspace.ErrInvalidTeam) {
		t.Fatalf("platform changed filter accepted: %v", err)
	}
	if _, err = teams.Read(t.Context(), actor, team.ID); err == nil {
		t.Fatal("platform gained member content scope")
	}
}

func TestGovernanceDirectoryAndInvitationDelivery(t *testing.T) {
	tx, gov, actor, now := governanceFixture(t)
	first, err := gov.CreateAccountInvitation(t.Context(), actor, uuid.New(), "page-one@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	token := completionPayload(t, tx, first.EmailKey, "account_invitation")["token"]
	preview, err := gov.PublicInvitationPreview(t.Context(), token)
	if err != nil || preview.Kind != "account" {
		t.Fatalf("public preview: %v", err)
	}
	raw, _ := json.Marshal(preview)
	if strings.Contains(string(raw), "email") || strings.Contains(string(raw), actor.ID.String()) {
		t.Fatal("public invitation disclosure")
	}
	_, err = gov.CreateAccountInvitation(t.Context(), actor, uuid.New(), "page-two@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	page, err := gov.ListInvitations(t.Context(), actor, nil, identity.GovernanceFilter{Limit: 1, Status: "pending"})
	if err != nil || page.NextCursor == nil {
		t.Fatalf("missing continuation: %+v %v", page, err)
	}
	next, err := gov.ListInvitations(t.Context(), actor, nil, identity.GovernanceFilter{Limit: 1, Status: "pending", Cursor: *page.NextCursor})
	if err != nil || len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID {
		t.Fatalf("keyset pagination: %+v %v", next, err)
	}
	if _, err = gov.ListInvitations(t.Context(), actor, nil, identity.GovernanceFilter{Limit: 2, Status: "pending", Cursor: *page.NextCursor}); !errors.Is(err, identity.ErrInvalidGovernance) {
		t.Fatalf("changed filter accepted: %v", err)
	}
	if err = tx.Exec(`UPDATE identity.email_budget SET sent_at='{}'`).Error; err != nil {
		t.Fatal(err)
	}
	if err = gov.ResendInvitation(t.Context(), actor, first.ID, 1, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = gov.PublicInvitationPreview(t.Context(), token); !errors.Is(err, identity.ErrProofInvalid) {
		t.Fatalf("old proof survived resend: %v", err)
	}
	jobs, err := pgidentity.NewMailStore(tx).Claim(t.Context(), 100, now)
	if err != nil {
		t.Fatal(err)
	}
	var sentFirst int
	for _, j := range jobs {
		if j.Recipient == first.EmailKey {
			sentFirst++
		}
	}
	if sentFirst != 1 {
		t.Fatalf("superseded mail claimed: %d", sentFirst)
	}
	if err = gov.RevokeInvitation(t.Context(), actor, first.ID, 2, "Revoke before lease reclaim", nil); err != nil {
		t.Fatal(err)
	}
	jobs, err = pgidentity.NewMailStore(tx).Claim(t.Context(), 100, now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.Recipient == first.EmailKey {
			t.Fatal("revoked leased mail reclaimed")
		}
	}
}
func TestInvitedRegistrationCreatesOrdinaryAccount(t *testing.T) {
	tx, gov, actor, now := governanceFixture(t)
	if err := tx.Exec(`UPDATE identity.platform_settings SET registration_enabled=false,team_creation_enabled=true`).Error; err != nil {
		t.Fatal(err)
	}
	_, owner := verifiedTestActor(t, tx, "invited-team-owner@example.invalid", now)
	teams := workspace.NewTeams(pgworkspace.NewTeamStore(tx, func(tx *gorm.DB) workspace.TeamActorAccess {
		return pgidentity.NewTeamActorStore(tx, completionProtector(t))
	}), func() time.Time { return now })
	team, err := teams.Create(t.Context(), owner, uuid.New(), "Platform invited team")
	if err != nil {
		t.Fatal(err)
	}
	i, err := gov.CreatePlatformTeamInvitation(t.Context(), actor, uuid.New(), team.ID, "complete-invited@example.invalid", "team_admin", "Invite recovery administrator")
	if err != nil {
		t.Fatal(err)
	}
	token := completionPayload(t, tx, i.EmailKey, "team_invitation")["token"]
	store := pgidentity.NewInvitationEmailStore(tx, func(tx *gorm.DB) identity.TeamGovernance { return pgworkspace.NewTeamGovernance(tx) })
	auth := identity.NewEmailAuth(store, completionProtector(t), identity.EmailPolicy{TermsVersion: "test", SourceHourlyLimit: 100, GlobalHourlyLimit: 1000}, func() time.Time { return now })
	if _, err = auth.StartRegistration(t.Context(), "different@example.invalid", token); !errors.Is(err, identity.ErrProofInvalid) {
		t.Fatalf("email binding bypass: %v", err)
	}
	start, err := auth.StartRegistration(t.Context(), i.EmailKey, token)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := auth.VerifyRegistration(t.Context(), start.OperationID, start.FlowSecret, completionPayload(t, tx, i.EmailKey, "registration_code")["code"])
	if err != nil {
		t.Fatal(err)
	}
	id, err := auth.CompleteRegistration(t.Context(), identity.CompleteRegistrationInput{OperationID: start.OperationID, FlowSecret: start.FlowSecret, Grant: proof.Grant, DisplayName: "Invited", Password: "long invited password", ConfirmPassword: "long invited password", TermsAccepted: true, TermsVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	var role string
	tx.Raw(`SELECT platform_role FROM identity."user" WHERE id=?`, id).Scan(&role)
	if role != "creator" {
		t.Fatal("invitation granted platform admin")
	}
	var n int64
	tx.Raw(`SELECT count(*) FROM workspace.team_member WHERE user_id=?`, id).Scan(&n)
	if n != 0 {
		t.Fatal("signup implicitly accepted team")
	}
	login, err := auth.Login(t.Context(), i.EmailKey, "long invited password", false)
	if err != nil {
		t.Fatal(err)
	}
	a := identity.Principal{ID: id, SessionID: login.Session.ID, CredentialEpoch: login.User.SessionEpoch, PlatformEpoch: login.User.PlatformEpoch}
	if err = gov.AcceptInvitation(t.Context(), a, token); err != nil {
		t.Fatal(err)
	}
	tx.Raw(`SELECT count(*) FROM workspace.team_member WHERE team_id=? AND user_id=? AND role='team_admin'`, team.ID, id).Scan(&n)
	if n != 1 {
		t.Fatal("explicit acceptance did not join team")
	}
	if _, err = teams.Read(t.Context(), actor, team.ID); err == nil {
		t.Fatal("platform inviter gained membership")
	}
}
func TestPlatformResetAndAllowedActions(t *testing.T) {
	tx, _, actor, now := governanceFixture(t)
	auth, target := verifiedTestActor(t, tx, "reset-target@example.invalid", now)
	service := identity.NewPlatformPasswordReset(pgidentity.NewGovernanceStore(tx, func(tx *gorm.DB) identity.TeamGovernance { return pgworkspace.NewTeamGovernance(tx) }), auth, func() time.Time { return now })
	var revision int64
	tx.Raw(`SELECT revision FROM identity."user" WHERE id=?`, target.ID).Scan(&revision)
	if err := service.Send(t.Context(), actor, target.ID, revision, "Recover account by email"); err != nil {
		t.Fatal(err)
	}
	if err := pgidentity.NewStore(tx).CheckSession(t.Context(), target, func() time.Time { return now }); err != nil {
		t.Fatal("recovery request revoked existing session before proof", err)
	}
	if _, err := auth.Login(t.Context(), "reset-target@example.invalid", "long password for tests", false); err != nil {
		t.Fatal("recovery request changed current password before proof", err)
	}
	payload := completionPayload(t, tx, "reset-target@example.invalid", "password_reset")
	if payload["token"] == "" {
		t.Fatal("recovery workflow missing")
	}
	if err := auth.CompletePasswordReset(t.Context(), payload["token"], "new recovered password", "new recovered password"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='creator' WHERE id<>?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	directory := identity.NewPlatformDirectory(pgidentity.NewPlatformDirectoryStore(tx), completionProtector(t), func() time.Time { return now })
	detail, err := directory.Detail(t.Context(), actor, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.AllowedActions) == 0 {
		t.Fatal("governance actions remain empty")
	}
	for _, action := range detail.AllowedActions {
		if action == "disable" || action == "revoke_admin" {
			t.Fatal("last administrator destructive action advertised")
		}
	}
}

func TestProposalPaginationBindings(t *testing.T) {
	tx, gov, actor, now := governanceFixture(t)
	var key uuid.UUID
	var target identity.Principal
	var revision int64
	for _, email := range []string{"pagination-target1@example.invalid", "pagination-target2@example.invalid"} {
		_, target = verifiedTestActor(t, tx, email, now)
		if err := tx.Raw(`SELECT revision FROM identity."user" WHERE id=?`, target.ID).Scan(&revision).Error; err != nil {
			t.Fatal(err)
		}
		key = uuid.New()
		if _, err := gov.ProposeAdminWithKey(t.Context(), actor, key, target.ID, revision, "Explicit authority proposal"); err != nil {
			t.Fatal(err)
		}
	}
	page, err := gov.ListProposals(t.Context(), actor, nil, "platform", identity.GovernanceFilter{Limit: 1})
	if err != nil || page.NextCursor == nil {
		t.Fatalf("proposal cursor missing: %v", err)
	}
	second, err := gov.ListProposals(t.Context(), actor, nil, "platform", identity.GovernanceFilter{Limit: 1, Cursor: *page.NextCursor})
	if err != nil || len(second.Items) != 1 || second.Items[0].ID == page.Items[0].ID {
		t.Fatalf("proposal keyset: %v", err)
	}
	if _, err = gov.ListProposals(t.Context(), target, nil, "mine", identity.GovernanceFilter{Limit: 1, Cursor: *page.NextCursor}); !errors.Is(err, identity.ErrInvalidGovernance) {
		t.Fatalf("cross actor cursor accepted: %v", err)
	}
	if _, err = gov.ProposeAdminWithKey(t.Context(), actor, key, target.ID, revision, "Different proposal reason"); !errors.Is(err, identity.ErrGovernanceConflict) {
		t.Fatalf("key body mismatch accepted: %v", err)
	}
	directory := identity.NewPlatformDirectory(pgidentity.NewPlatformDirectoryStore(tx), completionProtector(t), func() time.Time { return now })
	detail, err := directory.Detail(t.Context(), actor, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range detail.AllowedActions {
		if action == "propose_admin" {
			t.Fatal("pending grant offered twice")
		}
	}
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='creator',platform_epoch=platform_epoch+1 WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = gov.ListProposals(t.Context(), actor, nil, "platform", identity.GovernanceFilter{Limit: 1, Cursor: *page.NextCursor}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("revoked source directory: %v", err)
	}
}
func TestTeamNotificationFailureRollsBack(t *testing.T) {
	tx, _, actor, now := governanceFixture(t)
	if err := tx.Exec(`UPDATE identity.platform_settings SET team_creation_enabled=true`).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`CREATE FUNCTION pg_temp.reject_team_mail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.template='team_governance' THEN RAISE EXCEPTION 'test notification failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_reject_team_mail BEFORE INSERT ON identity.mail_delivery FOR EACH ROW EXECUTE FUNCTION pg_temp.reject_team_mail()`).Error; err != nil {
		t.Fatal(err)
	}
	teams := workspace.NewTeams(pgworkspace.NewTeamStore(tx, func(db *gorm.DB) workspace.TeamActorAccess {
		return pgidentity.NewTeamActorStore(db, completionProtector(t))
	}), func() time.Time { return now })
	if _, err := teams.Create(t.Context(), actor, uuid.New(), "Rollback notification"); err == nil {
		t.Fatal("outbox failure did not abort team create")
	}
	var n int64
	if err := tx.Raw(`SELECT count(*) FROM workspace.team WHERE name='Rollback notification'`).Scan(&n).Error; err != nil || n != 0 {
		t.Fatalf("team persisted without notification: %d %v", n, err)
	}
}
