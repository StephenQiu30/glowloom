package identityregistration_test

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func resetFixture(t *testing.T, disabled bool) (*registrationFixture, uuid.UUID, string, string) {
	t.Helper()
	f, password := passwordFixture(t)
	user, err := f.auth.Current(t.Context(), password.Credential.LegacyToken)
	if err != nil {
		t.Fatal(err)
	}
	if disabled {
		if err := f.idb.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, user.User.ID).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.writers.Set(t.Context(), writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: user.User.ID, Epoch: 1}, uuid.New(), true); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.auth.WithPasswordResets(pgidentity.NewPasswordResetStore(f.idb), f.writers, f.protector); err != nil {
		t.Fatal(err)
	}
	bucket := hex.EncodeToString(f.protector.Digest("email_budget", uuid.Nil, user.User.Email))
	if err := f.idb.Exec(`UPDATE identity.email_budget SET sent_at=ARRAY[statement_timestamp()-interval '61 seconds'] WHERE bucket_key=?`, bucket).Error; err != nil {
		t.Fatal(err)
	}
	started, err := f.auth.StartPasswordReset(t.Context(), user.User.Email, "reset-source-fixture")
	if err != nil {
		t.Fatal(err)
	}
	var mail struct {
		EventID          uuid.UUID
		KeyID            string
		EncryptedPayload []byte
	}
	if err := f.idb.Raw(`SELECT m.event_id,m.key_id,m.encrypted_payload FROM identity.mail_delivery m JOIN identity.auth_challenge c ON c.id=m.challenge_id WHERE c.operation_id=? AND m.template='password_reset'`, started.OperationID).Scan(&mail).Error; err != nil {
		t.Fatal(err)
	}
	plain, err := f.protector.Open(mail.EventID, mail.KeyID, mail.EncryptedPayload)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(plain)
	var payload struct{ Token string }
	if err := json.Unmarshal(plain, &payload); err != nil {
		t.Fatal(err)
	}
	return f, user.User.ID, payload.Token, password.CurrentPassword
}
func TestPasswordResetSourceOriginalConsumedProofRecoversWithoutSession(t *testing.T) {
	f, id, token, old := resetFixture(t, false)
	request := uuid.New()
	next := "reset-source-new-credential"
	f.controlLoss.kind = writefence.Account
	f.controlLoss.lose.Store(true)
	if err := f.auth.CompletePasswordResetCommand(t.Context(), request, token, next, next); err == nil {
		t.Fatal("reset reported success before source close ACK")
	}
	var hash string
	if err := f.idb.Raw(`SELECT password_hash FROM identity."user" WHERE id=?`, id).Scan(&hash).Error; err != nil {
		t.Fatal(err)
	}
	// Even after its own accepted proof is consumed and expires, the original
	// request must finish without inventing a browser session or new proof.
	if err := f.idb.Exec(`UPDATE identity.auth_challenge SET created_at=statement_timestamp()-interval '2 hours',expires_at=statement_timestamp()-interval '1 hour' WHERE user_id=? AND purpose='password_reset'`, id).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.auth.CompletePasswordResetCommand(t.Context(), request, token, next, next); err != nil {
		t.Fatal(err)
	}
	var email string
	if err := f.idb.Raw(`SELECT email_key FROM identity."user" WHERE id=?`, id).Scan(&email).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.LoginFromCommand(t.Context(), uuid.New(), email, old, false, "reset-fixture"); err == nil {
		t.Fatal("old password survived reset")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.user_session WHERE account_id=? AND revoked_at IS NULL`, id) != 0 {
		t.Fatal("reset created a session")
	}
	if _, err := f.auth.LoginFromCommand(t.Context(), uuid.New(), email, next, false, "reset-fixture"); err != nil {
		t.Fatalf("reopened active reset account cannot login: %v", err)
	}
	if err := f.auth.CompletePasswordResetCommand(t.Context(), request, token, next, next); err != nil {
		t.Fatal(err)
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action='password.changed'`, id) != 1 {
		t.Fatal("reset recovery repeated mutation")
	}
}
func TestPasswordResetSourceDisabledAccountNeverReopens(t *testing.T) {
	f, id, token, _ := resetFixture(t, true)
	request := uuid.New()
	next := "disabled-reset-next-credential"
	if err := f.auth.CompletePasswordResetCommand(t.Context(), request, token, next, next); err != nil {
		t.Fatal(err)
	}
	var head struct {
		Epoch             int64
		Blocked, Complete bool
		CoordinationID    uuid.UUID
	}
	if err := f.idb.Raw(`SELECT epoch,blocked,complete,coordination_id FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='identity' AND authority_kind='account' AND authority_id=?`, id).Scan(&head).Error; err != nil || !head.Blocked || !head.Complete || head.Epoch != 3 {
		t.Fatalf("disabled reset reopened qualification: %#v %v", head, err)
	}
	state, err := pgfence.NewAuthorityStore(f.idb).LoadCoordination(t.Context(), head.CoordinationID)
	if err != nil || !state.Complete || len(state.Acknowledged) != 4 {
		t.Fatal("disabled reset lacked four source ACKs")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE id=? AND status='disabled'`, id) != 1 {
		t.Fatal("reset enabled a disabled account")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.account_qualification_change WHERE request_id=? AND actor_session_id IS NULL AND proof_operation_id IS NOT NULL AND reopen_coordination_id IS NULL AND response_ciphertext IS NULL`, request) != 1 {
		t.Fatal("anonymous reset invented a session/reply or reopen")
	}
}
