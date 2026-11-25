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
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

func deletionCode(t *testing.T, tx *gorm.DB, op uuid.UUID) string {
	t.Helper()
	var mail struct {
		EventID          uuid.UUID
		KeyID            string
		EncryptedPayload []byte
	}
	if err := tx.Raw(`SELECT m.event_id,m.key_id,m.encrypted_payload FROM identity.mail_delivery m JOIN identity.auth_challenge c ON c.id=m.challenge_id WHERE c.operation_id=? AND m.template='deletion_confirm'`, op).Scan(&mail).Error; err != nil {
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
	var data struct{ Code string }
	if err := json.Unmarshal(plain, &data); err != nil {
		t.Fatal(err)
	}
	return data.Code
}

func TestDeletionDutiesAndRevoke(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	auth, actor := verifiedTestActor(t, tx, "deletion@example.invalid", now)
	service := identity.NewAccountDeletion(auth, pgidentity.NewAccountDeletionStore(tx), func() time.Time { return now })
	if _, err := service.Start(t.Context(), actor, "test"); !errors.Is(err, identity.ErrReauthenticationRequired) {
		t.Fatalf("missing recent proof: %v", err)
	}
	login, err := auth.Login(t.Context(), "deletion@example.invalid", "long password for tests", false)
	if err != nil {
		t.Fatal(err)
	}
	actor.SessionID = login.Session.ID
	if err := auth.Reauthenticate(t.Context(), login.Token, "long password for tests"); err != nil {
		t.Fatal(err)
	}
	team := uuid.New()
	if err := tx.Exec(`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'Deletion team',?)`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',?,'created')`, team, actor.ID, now).Error; err != nil {
		t.Fatal(err)
	}
	impact, err := service.Preview(t.Context(), actor)
	if err != nil || len(impact.BlockingDuties) != 1 || len(impact.TeamIDs) != 1 {
		t.Fatalf("last team administrator not protected: %+v %v", impact, err)
	}
	if _, err := service.Start(t.Context(), actor, "test"); !errors.Is(err, identity.ErrDeletionDuties) {
		t.Fatalf("last team admin can delete: %v", err)
	}
	_, successor := verifiedTestActor(t, tx, "successor@example.invalid", now)
	if err := tx.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',?,'transferred')`, team, successor.ID, now).Error; err != nil {
		t.Fatal(err)
	}
	impact, err = service.Preview(t.Context(), actor)
	if err != nil {
		t.Fatal(err)
	}
	start, err := service.Start(t.Context(), actor, "test")
	if err != nil {
		t.Fatal(err)
	}
	code := deletionCode(t, tx, start.OperationID)
	projectID := uuid.New()
	if err := tx.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,name,aspect_ratio,style_type) SELECT ?,org_id,id,'Retained personal work','16:9','realistic' FROM identity."user" WHERE id=?`, projectID, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.Confirm(t.Context(), actor, impact.Revision-1, start.OperationID, code); !errors.Is(err, domain.ErrRevisionConflict) {
		t.Fatalf("stale confirmation accepted: %v", err)
	}
	if err := service.Confirm(t.Context(), actor, impact.Revision, start.OperationID, code); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Current(t.Context(), login.Token); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("deleted account session survives: %v", err)
	}
	if _, err := auth.Login(t.Context(), "deletion@example.invalid", "long password for tests", false); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("deleted account logs in: %v", err)
	}
	var retained bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM identity."user" u JOIN workspace.team t ON t.created_by=u.id JOIN workspace.team_member m ON m.team_id=t.id AND m.user_id=u.id WHERE u.id=? AND u.is_delete AND u.deleted_at IS NOT NULL AND u.status='disabled' AND u.email_key='deletion@example.invalid' AND m.status='removed')`, actor.ID).Scan(&retained).Error; err != nil || !retained {
		t.Fatal("soft deletion removed data or retained rights", err)
	}
	var teamVersions struct{ Revision, AuthEpoch int64 }
	if err := tx.Raw(`SELECT revision,auth_epoch FROM workspace.team WHERE id=?`, team).Scan(&teamVersions).Error; err != nil || teamVersions.Revision != 2 || teamVersions.AuthEpoch != 2 {
		t.Fatal("deletion left team authorization or cursor epoch unchanged", err)
	}
	var live int64
	tx.Raw(`SELECT count(*) FROM identity.user_session WHERE account_id=? AND revoked_at IS NULL`, actor.ID).Scan(&live)
	if live != 0 {
		t.Fatal("other sessions survived")
	}
	var resourceRetained bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.project p JOIN workspace.creation_scope s ON s.owner_user_id=p.owner_id WHERE p.id=? AND p.owner_id=? AND NOT p.is_delete)`, projectID, actor.ID).Scan(&resourceRetained).Error; err != nil || !resourceRetained {
		t.Fatal("soft deletion removed personal resources or their ownership", err)
	}
	// The deleted mailbox stays reserved, including proofs created before deletion.
	if err := pgidentity.NewStore(tx).EmailTransaction(t.Context(), func(et identity.EmailTx) error {
		reserved, err := et.EmailReserved("deletion@example.invalid")
		if err != nil {
			return err
		}
		if !reserved {
			t.Fatal("soft-deleted mailbox became available")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.Confirm(t.Context(), actor, impact.Revision, start.OperationID, code); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("deleted actor replay accepted: %v", err)
	}
}

func TestDeletionProofBinding(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	auth, a := verifiedTestActor(t, tx, "proof-delete@example.invalid", now)
	_, b := verifiedTestActor(t, tx, "other-delete@example.invalid", now)
	for _, actor := range []identity.Principal{a, b} {
		if err := tx.Exec(`UPDATE identity.user_session SET reauth_at=?,reauth_epoch=? WHERE id=?`, now, actor.CredentialEpoch, actor.SessionID).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := identity.NewAccountDeletion(auth, pgidentity.NewAccountDeletionStore(tx), func() time.Time { return now })
	impact, err := svc.Preview(t.Context(), a)
	if err != nil {
		t.Fatal(err)
	}
	start, err := svc.Start(t.Context(), a, "test")
	if err != nil {
		t.Fatal(err)
	}
	code := deletionCode(t, tx, start.OperationID)
	if err := svc.Confirm(t.Context(), b, impact.Revision, start.OperationID, code); !errors.Is(err, identity.ErrProofInvalid) {
		t.Fatalf("proof crossed actor: %v", err)
	}
	bad := "000000"
	if bad == code {
		bad = "111111"
	}
	for i := 0; i < 5; i++ {
		if err := svc.Confirm(t.Context(), a, impact.Revision, start.OperationID, bad); !errors.Is(err, identity.ErrProofInvalid) {
			t.Fatal(err)
		}
	}
	if err := svc.Confirm(t.Context(), a, impact.Revision, start.OperationID, code); !errors.Is(err, identity.ErrProofInvalid) {
		t.Fatalf("exhausted proof accepted: %v", err)
	}
}
