package identityregistration_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func TestRegistrationPostgresAccountInitialACKUnknownKeepsAccountPrivate(t *testing.T) {
	f := newRegistrationFixture(t)
	input := f.input(t)
	f.controlLoss.kind = writefence.Account
	f.controlLoss.lose.Store(true)
	if _, err := f.auth.CompleteRegistration(t.Context(), input); err == nil {
		t.Fatal("incomplete account bootstrap reported success")
	}
	p, err := f.store.LoadAcceptedRegistration(t.Context(), input.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE id=?`, p.AccountID) != 0 {
		t.Fatal("initial account head ACK failure exposed user")
	}
	if _, err := f.store.ActivateRegistration(t.Context(), p); err == nil {
		t.Fatal("activation trusted readiness without actual four source ACKs")
	}
	id, err := f.auth.CompleteRegistration(t.Context(), input)
	if err != nil || id != p.AccountID {
		t.Fatalf("original account bootstrap recovery=%s %v", id, err)
	}
	state, err := pgfence.NewAuthorityStore(f.idb).LoadCoordination(t.Context(), p.AccountControl().CoordinationID)
	if err != nil || !state.Complete || len(state.Acknowledged) != 4 {
		t.Fatalf("account ACKs=%v %v", state, err)
	}
	closed, err := f.writers.Set(t.Context(), p.AccountControl().Authority, uuid.New(), true)
	if err != nil || !closed.Complete {
		t.Fatal(err)
	}
	repeated, err := f.auth.CompleteRegistration(t.Context(), input)
	if err != nil || repeated != id {
		t.Fatalf("original registered reply changed=%s %v", repeated, err)
	}
	var head struct {
		Epoch   int64
		Blocked bool
	}
	if err := f.idb.Raw(`SELECT epoch,blocked FROM infra.authority_head WHERE issuer='identity' AND authority_kind='account' AND authority_id=?`, id).Scan(&head).Error; err != nil || head.Epoch != 2 || !head.Blocked {
		t.Fatalf("registration replay reopened old account=%v %v", head, err)
	}
}

func TestSessionDirectoryHidesAcceptedButUnactivatedLogin(t *testing.T) {
	f, password := passwordFixture(t)
	current, err := f.auth.Current(t.Context(), password.Credential.LegacyToken)
	if err != nil {
		t.Fatal(err)
	}
	f.controlLoss.kind = writefence.Session
	f.controlLoss.lose.Store(true)
	request := uuid.New()
	if _, err := f.auth.LoginFromCommand(t.Context(), request, current.User.Email, password.CurrentPassword, false, "pending-directory-fixture"); err == nil {
		t.Fatal("pending login ACK loss not exercised")
	}
	actor := identity.Principal{ID: current.User.ID, SessionID: current.Session.ID, CredentialEpoch: current.User.SessionEpoch}
	store := pgidentity.NewSessionDirectoryStore(f.idb)
	rows, err := store.ListSelfSessions(t.Context(), actor, func() time.Time { return f.clock })
	if err != nil || len(rows) != 1 || rows[0].ID != current.Session.ID {
		t.Fatalf("pending source session exposed in directory: count=%d %v", len(rows), err)
	}
	if _, err := f.auth.LoginFromCommand(t.Context(), request, current.User.Email, password.CurrentPassword, false, "pending-directory-fixture"); err != nil {
		t.Fatal(err)
	}
	rows, err = store.ListSelfSessions(t.Context(), actor, func() time.Time { return f.clock })
	if err != nil || len(rows) != 2 {
		t.Fatalf("activated original session missing from directory: count=%d %v", len(rows), err)
	}
}

func TestRegistrationPostgresLoginInitialSessionACKAndExactReplay(t *testing.T) {
	f := newRegistrationFixture(t)
	input := f.input(t)
	id, err := f.auth.CompleteRegistration(t.Context(), input)
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
	if _, err := f.auth.Login(t.Context(), email, input.Password, false); !errors.Is(err, identity.ErrSessionActivation) {
		t.Fatalf("native login without original key bypassed source preparation: %v", err)
	}
	requestID := uuid.New()
	f.controlLoss.kind = writefence.Session
	f.controlLoss.lose.Store(true)
	result, err := f.auth.LoginFromCommand(t.Context(), requestID, email, input.Password, false, "fixture")
	if err == nil || result.Token != "" || result.AccessToken != "" || result.Session.ID != uuid.Nil {
		t.Fatal("initial session ACK failure leaked credentials")
	}
	var saved struct {
		SessionID          uuid.UUID
		ResponseCiphertext []byte
	}
	if err := f.idb.Raw(`SELECT session_id,response_ciphertext FROM identity.session_activation WHERE request_id=?`, requestID).Scan(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if saved.SessionID == uuid.Nil || len(saved.ResponseCiphertext) == 0 {
		t.Fatal("accepted login responsibility or encrypted reply absent")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action='session.created'`, id) != 0 {
		t.Fatal("login success audit preceded source ACKs")
	}
	if _, err := f.auth.LoginFromCommand(t.Context(), requestID, email, "different credential", false, "fixture"); !errors.Is(err, identity.ErrProofInvalid) {
		t.Fatalf("retargeted login recovery=%v", err)
	}
	result, err = f.auth.LoginFromCommand(t.Context(), requestID, email, input.Password, false, "fixture")
	if err != nil || result.Session.ID != saved.SessionID || result.Token == "" || result.CSRFToken == "" {
		t.Fatalf("same original login recovery session=%s %v", result.Session.ID, err)
	}
	again, err := f.auth.LoginFromCommand(t.Context(), requestID, email, input.Password, false, "different ingress after retry")
	if err != nil || again.Session.ID != result.Session.ID || again.Token != result.Token || again.CSRFToken != result.CSRFToken {
		t.Fatal("lost original reply minted another session credential")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.user_session WHERE account_id=?`, id) != 1 || count(t, f.idb, `SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action='session.created'`, id) != 1 {
		t.Fatal("original login not exact once")
	}
	command := writefence.Command{ID: uuid.New(), SHA256: strings.Repeat("a", 64), Action: "media.registration_smoke", Authorities: []writefence.Authority{{Issuer: writefence.Identity, Kind: writefence.Account, ID: id, Epoch: 1}, {Issuer: writefence.Identity, Kind: writefence.Session, ID: result.Session.ID, Epoch: 1}}}
	actor := identity.Principal{ID: id, OrgID: result.User.OrgID, SessionID: result.Session.ID, CredentialEpoch: result.User.SessionEpoch}
	if err := pgidentity.NewSessionAdmissionStore(f.idb).RegisterSessionCommand(t.Context(), actor, command, writefence.WriterMedia, func() time.Time { return f.clock }); err != nil {
		t.Fatalf("new login cannot admit original command: %v", err)
	}
	proof, err := pgidentity.NewSessionAdmissionStore(f.idb).ReadSessionAdmission(t.Context(), command, writefence.WriterMedia, func() time.Time { return f.clock })
	if err != nil || proof.AccountID != id || proof.SessionID != result.Session.ID || proof.CredentialEpoch != result.User.SessionEpoch {
		t.Fatalf("live source admission proof=%v %v", proof, err)
	}
	mediaDB := registrationDB(t, "media")
	if _, err := pgfence.New(mediaDB).Admit(t.Context(), command); err != nil {
		t.Fatalf("effect owner cannot accept exact source-proved command: %v", err)
	}
	if err := mediaDB.Transaction(func(tx *gorm.DB) error { return pgfence.New(mediaDB).Guard(t.Context(), tx, command) }); err != nil {
		t.Fatalf("new login initial heads unavailable on effect writer: %v", err)
	}
	state, err := f.writers.Set(t.Context(), command.Authorities[1], uuid.New(), true)
	if err != nil || !state.Complete {
		t.Fatal(err)
	}
	if _, err := f.auth.LoginFromCommand(t.Context(), requestID, email, input.Password, false, "fixture"); err == nil {
		t.Fatal("original login reply reopened a later closed session")
	}
}
