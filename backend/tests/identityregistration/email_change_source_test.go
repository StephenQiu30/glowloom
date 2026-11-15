package identityregistration_test

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func emailChangeFixture(t *testing.T, target ...string) (*registrationFixture, *identity.EmailChange, identity.EmailChangeCommand, identity.Principal, string) {
	t.Helper()
	f, password := passwordFixture(t)
	if err := f.auth.Reauthenticate(t.Context(), password.Credential.LegacyToken, password.CurrentPassword); err != nil {
		t.Fatal(err)
	}
	current, err := f.auth.Current(t.Context(), password.Credential.LegacyToken)
	if err != nil {
		t.Fatal(err)
	}
	actor := identity.Principal{ID: current.User.ID, SessionID: current.Session.ID, CredentialEpoch: current.User.SessionEpoch}
	bucket := hex.EncodeToString(f.protector.Digest("email_budget", uuid.Nil, current.User.Email))
	if err := f.idb.Exec(`UPDATE identity.email_budget SET sent_at=ARRAY[?::timestamptz] WHERE bucket_key=?`, f.clock.Add(-61*time.Second), bucket).Error; err != nil {
		t.Fatal(err)
	}
	service := identity.NewEmailChange(f.auth, pgidentity.NewSourceEmailChangeStore(f.idb), func() time.Time { return f.clock })
	if err := service.WithSource(pgidentity.NewSourceEmailChangeStore(f.idb), f.writers, f.protector); err != nil {
		t.Fatal(err)
	}
	next := uuid.NewString() + "@email-source.invalid"
	if len(target) == 1 {
		next = target[0]
		bucket := hex.EncodeToString(f.protector.Digest("email_budget", uuid.Nil, next))
		if err := f.idb.Exec(`UPDATE identity.email_budget SET sent_at=ARRAY[?::timestamptz] WHERE bucket_key=?`, f.clock.Add(-61*time.Second), bucket).Error; err != nil {
			t.Fatal(err)
		}
	}
	op, err := service.Start(t.Context(), actor, next, current.User.Revision)
	if err != nil {
		t.Fatal(err)
	}
	for _, side := range []string{"old", "new"} {
		var mail struct {
			EventID          uuid.UUID
			KeyID            string
			EncryptedPayload []byte
		}
		if err := f.idb.Raw(`SELECT m.event_id,m.key_id,m.encrypted_payload FROM identity.mail_delivery m JOIN identity.auth_challenge c ON c.id=m.challenge_id WHERE c.operation_id=? AND c.purpose=?`, op, "email_"+side).Scan(&mail).Error; err != nil {
			t.Fatal(err)
		}
		plain, err := f.protector.Open(mail.EventID, mail.KeyID, mail.EncryptedPayload)
		if err != nil {
			t.Fatal(err)
		}
		var payload struct{ Code string }
		if err := json.Unmarshal(plain, &payload); err != nil {
			t.Fatal(err)
		}
		clear(plain)
		if err := f.idb.Exec(`UPDATE identity.mail_delivery SET status='submitted' WHERE event_id=?`, mail.EventID).Error; err != nil {
			t.Fatal(err)
		}
		if err := service.Verify(t.Context(), actor, op, side, payload.Code); err != nil {
			t.Fatal(err)
		}
	}
	return f, service, identity.EmailChangeCommand{RequestID: uuid.New(), OperationID: op, Revision: current.User.Revision, Credential: password.Credential}, actor, next
}

func TestEmailChangeSourceUnknownClosureReservesAddressAndOriginalProofRecovers(t *testing.T) {
	f, service, input, actor, next := emailChangeFixture(t)
	f.controlLoss.kind = writefence.Account
	f.controlLoss.lose.Store(true)
	if err := service.CompleteCommand(t.Context(), input); err == nil {
		t.Fatal("email change succeeded before source closure ACKs")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE id=? AND email_key=?`, actor.ID, next) != 0 {
		t.Fatal("email changed before four close ACKs")
	}
	// Another operation cannot claim the accepted candidate address while its
	// original request is waiting for a lost writer acknowledgement.
	var reserved bool
	if err := f.store.EmailTransaction(t.Context(), func(tx identity.EmailTx) error {
		var err error
		reserved, err = tx.EmailReserved(next)
		return err
	}); err != nil || !reserved {
		t.Fatalf("pending candidate address not reserved: %v", err)
	}
	if err := f.idb.Exec(`UPDATE identity.user_session SET revoked_at=statement_timestamp() WHERE id=?`, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.idb.Exec(`UPDATE identity.auth_challenge SET created_at=statement_timestamp()-interval '2 hours',expires_at=statement_timestamp()-interval '1 hour' WHERE operation_id=?`, input.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	wrong := input
	wrong.Revision++
	if err := service.CompleteCommand(t.Context(), wrong); !errors.Is(err, identity.ErrProofInvalid) {
		t.Fatalf("accepted email input could be changed: %v", err)
	}
	for range 2 {
		if err := service.CompleteCommand(t.Context(), input); err != nil {
			t.Fatal(err)
		}
	}
	if count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE id=? AND email_key=?`, actor.ID, next) != 1 {
		t.Fatal("original email request not applied")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.user_session WHERE account_id=? AND revoked_at IS NULL`, actor.ID) != 0 {
		t.Fatal("email change created a replacement session")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action='email.changed'`, actor.ID) != 1 {
		t.Fatal("email original replay repeated its mutation")
	}
	p, err := pgidentity.NewSourceEmailChangeStore(f.idb).LoadAcceptedEmailChange(t.Context(), input.RequestID)
	if err != nil || !p.Completed {
		t.Fatalf("email responsibility not complete: %v", err)
	}
	for _, control := range []writefence.Control{p.CloseControl, p.ReopenControl} {
		state, err := pgfence.NewAuthorityStore(f.idb).LoadCoordination(t.Context(), control.CoordinationID)
		if err != nil || !state.Complete || state.Control != control || len(state.Acknowledged) != 4 {
			t.Fatal("email change lacks exact four writer ACKs")
		}
	}
}

func TestEmailChangeSourceCompetingAccountsReserveOneAddress(t *testing.T) {
	first, one, a, _, next := emailChangeFixture(t)
	_, two, b, _, _ := emailChangeFixture(t, next)
	results := make(chan error, 2)
	start := make(chan struct{})
	go func() { <-start; results <- one.CompleteCommand(t.Context(), a) }()
	go func() { <-start; results <- two.CompleteCommand(t.Context(), b) }()
	close(start)
	success, unavailable := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			success++
		case errors.Is(err, identity.ErrEmailUnavailable):
			unavailable++
		default:
			t.Fatalf("competing email command: %v", err)
		}
	}
	if success != 1 || unavailable != 1 || count(t, first.idb, `SELECT count(*) FROM identity."user" WHERE email_key=?`, next) != 1 {
		t.Fatal("different accounts claimed the same destination")
	}
}

func TestEmailChangeSourcePendingExcludesAnotherCredentialAction(t *testing.T) {
	f, service, input, actor, _ := emailChangeFixture(t)
	f.controlLoss.kind = writefence.Account
	f.controlLoss.lose.Store(true)
	if err := service.CompleteCommand(t.Context(), input); err == nil {
		t.Fatal("source close loss not exercised")
	}
	password := identity.PasswordChangeInput{RequestID: uuid.New(), Credential: input.Credential, CredentialEpoch: actor.CredentialEpoch, CurrentPassword: "registration-test-password", NewPassword: "another-password-action", Confirmation: "another-password-action"}
	if _, err := f.auth.ChangePasswordCommand(t.Context(), password); err == nil {
		t.Fatal("second credential action crossed pending mailbox responsibility")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.account_qualification_change WHERE target_id=? AND outcome='pending'`, actor.ID) != 1 {
		t.Fatal("multiple credential actions accepted for one account")
	}
}

func TestEmailChangeSourceOldReopenCannotReplaceHigherClosure(t *testing.T) {
	f, service, input, actor, next := emailChangeFixture(t)
	f.controlLoss.kind = writefence.Account
	f.controlLoss.lose.Store(true)
	if err := service.CompleteCommand(t.Context(), input); err == nil {
		t.Fatal("source close loss not exercised")
	}
	p, err := pgidentity.NewSourceEmailChangeStore(f.idb).LoadAcceptedEmailChange(t.Context(), input.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	// Finish the original close ACKs, then persist a later owner decision without
	// applying the mailbox mutation. The older recovery cannot replace that head.
	expected := p.CloseControl.Authority
	expected.Epoch--
	if _, err := f.writers.Set(t.Context(), expected, p.CloseControl.CoordinationID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.writers.Set(t.Context(), p.CloseControl.Authority, uuid.New(), true); err != nil {
		t.Fatal(err)
	}
	if err := service.CompleteCommand(t.Context(), input); err == nil {
		t.Fatal("old email responsibility reopened a later account closure")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE id=? AND email_key=?`, actor.ID, next) != 0 {
		t.Fatal("stale mailbox responsibility applied mutation")
	}
	if count(t, f.idb, `SELECT count(*) FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='identity' AND authority_kind='account' AND authority_id=? AND epoch=? AND blocked AND complete`, actor.ID, p.CloseControl.Epoch+1) != 1 {
		t.Fatal("later source closure lost dominance")
	}
}
