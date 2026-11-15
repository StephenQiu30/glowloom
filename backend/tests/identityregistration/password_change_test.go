package identityregistration_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func passwordFixture(t *testing.T) (*registrationFixture, identity.PasswordChangeInput) {
	t.Helper()
	f := newRegistrationFixture(t)
	registration := f.input(t)
	id, err := f.auth.CompleteRegistration(t.Context(), registration)
	if err != nil {
		t.Fatal(err)
	}
	var email string
	if err := f.idb.Raw(`SELECT email_key FROM identity."user" WHERE id=?`, id).Scan(&email).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.auth.WithSessionActivation(f.store, f.writers, f.protector); err != nil {
		t.Fatal(err)
	}
	if err := f.auth.WithPasswordChanges(pgidentity.NewPasswordChangeStore(f.idb), f.writers, f.protector); err != nil {
		t.Fatal(err)
	}
	session, err := f.auth.LoginFromCommand(t.Context(), uuid.New(), email, registration.Password, false, "password-source-fixture")
	if err != nil {
		t.Fatal(err)
	}
	chosen := "replacement-source-fixture-password"
	return f, identity.PasswordChangeInput{RequestID: uuid.New(), Credential: identity.SessionCredential{LegacyToken: session.Token, CSRFToken: session.CSRFToken}, CredentialEpoch: session.User.SessionEpoch, CurrentPassword: registration.Password, NewPassword: chosen, Confirmation: chosen}
}
func TestPasswordChangeSourceClosesBeforeMutationAndOriginalProofRecovers(t *testing.T) {
	f, input := passwordFixture(t)
	f.controlLoss.kind = writefence.Account
	f.controlLoss.lose.Store(true)
	result, err := f.auth.ChangePasswordCommand(t.Context(), input)
	if err == nil || result.Token != "" || result.Session.ID != uuid.Nil {
		t.Fatal("unknown source closure published password success/credentials")
	}
	record, err := pgidentity.NewPasswordChangeStore(f.idb).LoadAcceptedPasswordChange(t.Context(), input.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	var hash string
	if err := f.idb.Raw(`SELECT password_hash FROM identity."user" WHERE id=?`, record.AccountID).Scan(&hash).Error; err != nil {
		t.Fatal(err)
	}
	if !domain.VerifyPassword(hash, input.CurrentPassword) || domain.VerifyPassword(hash, input.NewPassword) {
		t.Fatal("password changed before four original source closure ACKs")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.user_session WHERE account_id=?`, record.AccountID) != 1 {
		t.Fatal("replacement session prepared before closure was acknowledged")
	}
	wrong := input
	wrong.NewPassword = "different-source-fixture-password"
	wrong.Confirmation = wrong.NewPassword
	if _, err := f.auth.ChangePasswordCommand(t.Context(), wrong); !errors.Is(err, identity.ErrProofInvalid) {
		t.Fatalf("retargeted accepted password command=%v", err)
	}
	// Losing the browser's original live session must not strand an already
	// accepted source responsibility or allow a fresh target/session on recovery.
	if err := f.idb.Exec(`UPDATE identity.user_session SET revoked_at=statement_timestamp() WHERE id=?`, record.ActorSessionID).Error; err != nil {
		t.Fatal(err)
	}
	result, err = f.auth.ChangePasswordCommand(t.Context(), input)
	if err != nil || result.Token == "" || result.User.SessionEpoch != input.CredentialEpoch+1 {
		t.Fatalf("accepted password recovery=%v", err)
	}
	again, err := f.auth.ChangePasswordCommand(t.Context(), input)
	if err != nil || again.Token != result.Token || again.Session.ID != result.Session.ID || again.CSRFToken != result.CSRFToken {
		t.Fatal("original password retry minted another session")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.user_session WHERE account_id=?`, record.AccountID) != 2 {
		t.Fatal("replacement session was not exact once")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action='password.changed'`, record.AccountID) != 1 {
		t.Fatal("password event was not exact once")
	}
	if _, err := f.auth.Current(t.Context(), input.Credential.LegacyToken); err == nil {
		t.Fatal("old credential remained valid")
	}
	if _, err := f.auth.Current(t.Context(), result.Token); err != nil {
		t.Fatalf("new source-prepared credential unavailable: %v", err)
	}
}
func TestPasswordChangeUnknownReplacementACKKeepsEncryptedOriginalReply(t *testing.T) {
	f, input := passwordFixture(t)
	f.controlLoss.kind = writefence.Session
	f.controlLoss.lose.Store(true)
	if result, err := f.auth.ChangePasswordCommand(t.Context(), input); err == nil || result.Token != "" {
		t.Fatal("unknown replacement head reported success")
	}
	store := pgidentity.NewPasswordChangeStore(f.idb)
	record, err := store.LoadAcceptedPasswordChange(t.Context(), input.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if record.ReplacementSessionID == uuid.Nil || len(record.ResponseCiphertext) == 0 || record.Completed {
		t.Fatal("unknown replacement lost original responsibility")
	}
	result, err := f.auth.ChangePasswordCommand(t.Context(), input)
	if err != nil || result.Session.ID != record.ReplacementSessionID {
		t.Fatalf("original replacement recovery=%v", err)
	}
	if _, err := f.auth.ChangePasswordCommand(t.Context(), identity.PasswordChangeInput{RequestID: uuid.New(), Credential: input.Credential, CredentialEpoch: input.CredentialEpoch, CurrentPassword: input.CurrentPassword, NewPassword: input.NewPassword, Confirmation: input.Confirmation}); err == nil {
		t.Fatal("revoked original credential authorized a fresh change")
	}
}
