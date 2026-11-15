package identityregistration_test

import (
	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
	"gorm.io/gorm"

	workspacesourcepg "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/pgsource"

	coordinationpb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"

	fencerpc "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"

	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"

	creationrpc "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/grpcregistration"

	executionrpc "github.com/StephenQiu30/glowloom-server/backend/execution/billing/adapter/grpcregistration"

	pgexecution "github.com/StephenQiu30/glowloom-server/backend/execution/billing/adapter/postgres"

	execution "github.com/StephenQiu30/glowloom-server/backend/execution/billing/application"
	"github.com/StephenQiu30/glowloom-server/backend/identity/adapter/credentialproof"

	identityrpc "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcregistration"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

func registrationDB(t *testing.T, owner string) *gorm.DB {
	t.Helper()
	name := os.Getenv("GLOWLOOM_TEST_REGISTRATION_DB_NAME")
	if name == "" {
		t.Skip("requires newly initialized isolated registration database on loopback 52080")
	}
	if !regexp.MustCompile(`^glowloom_[a-z0-9_]+_test$`).MatchString(name) {
		t.Fatal("invalid dedicated registration database name")
	}
	switch owner {
	case "identity", "creation", "execution", "media":
	default:
		t.Fatal("invalid fixture owner")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	t.Cleanup(cancel)
	conn, err := db.Open(ctx, fmt.Sprintf("postgres://owner_b_%s_test@127.0.0.1:52080/%s?sslmode=disable", owner, name), noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	})
	var actual string
	if err := conn.DB.Raw(`SELECT infra.runtime_owner()`).Scan(&actual).Error; err != nil || actual != owner {
		t.Fatalf("actual owner=%q %v", actual, err)
	}
	return conn.DB
}

type ackLostParticipant struct {
	participant identity.RegistrationParticipant
	lose        atomic.Bool
}

func (p *ackLostParticipant) PrepareRegistration(ctx context.Context, v identity.RegistrationPreparation) (identity.RegistrationReady, error) {
	r, err := p.participant.PrepareRegistration(ctx, v)
	if err != nil {
		return r, err
	}
	if p.lose.CompareAndSwap(true, false) {
		return identity.RegistrationReady{}, errors.New("synthetic lost commit acknowledgement")
	}
	return r, nil
}

type registrationEndpoint struct{ Address, Token, Name string }

type registrationFixture struct {
	idb, cdb, edb            *gorm.DB
	auth                     *identity.Auth
	store                    *pgidentity.RegistrationStore
	protector                *credentialproof.Protector
	clock                    time.Time
	wallet                   *ackLostParticipant
	creation                 identity.RegistrationParticipant
	writers                  *writefence.Coordinator
	controlLoss              *lostControlWriter
	creationWriters          *writefence.Coordinator
	tls                      *tlsFixture
	writerPeers              map[writefence.Writer]registrationEndpoint
	creationPeer, walletPeer registrationEndpoint
}

type lostControlWriter struct {
	writefence.ControlWriter
	lose atomic.Bool
	kind writefence.Kind
}

func (w *lostControlWriter) Install(ctx context.Context, target writefence.Writer, control writefence.Control) error {
	if err := w.ControlWriter.Install(ctx, target, control); err != nil {
		return err
	}
	if target == writefence.WriterMedia && control.Kind == w.kind && w.lose.CompareAndSwap(true, false) {
		return errors.New("synthetic lost original writer ACK")
	}
	return nil
}
func newRegistrationWriterCoordinator(t *testing.T, f *registrationFixture, certs *tlsFixture) *writefence.Coordinator {
	t.Helper()
	databases := map[writefence.Writer]*gorm.DB{writefence.WriterIdentity: f.idb, writefence.WriterCreation: f.cdb, writefence.WriterExecution: f.edb, writefence.WriterMedia: registrationDB(t, "media")}
	targets := map[writefence.Writer]fencerpc.Target{}
	f.writerPeers = map[writefence.Writer]registrationEndpoint{}
	creationTargets := map[writefence.Writer]fencerpc.Target{}
	for _, writer := range writefence.Writers() {
		token := strings.Repeat("identity-writer-", 3) + string(writer)
		creationToken := strings.Repeat("creation-writer-", 3) + string(writer)
		handler, err := fencerpc.NewServer(writer, pgfence.New(databases[writer]), token, creationToken)
		if err != nil {
			t.Fatal(err)
		}
		s := grpc.NewServer(grpc.Creds(certs.server(t, string(writer)+".writer")))
		coordinationpb.RegisterWriterFenceServer(s, handler)
		address := serve(t, s)
		f.writerPeers[writer] = registrationEndpoint{Address: address, Token: token, Name: string(writer) + ".writer"}
		targets[writer] = fencerpc.Target{Address: address, Token: token, Transport: certs.client(t, string(writer)+".writer", "identity.controls")}
		creationTargets[writer] = fencerpc.Target{Address: address, Token: creationToken, Transport: certs.client(t, string(writer)+".writer", "creation.controls")}
	}
	creationClient, err := fencerpc.NewClient(writefence.Creation, creationTargets)
	if err != nil {
		t.Fatal(err)
	}
	f.creationWriters = writefence.NewCoordinator(pgfence.NewAuthorityStore(f.cdb), creationClient)
	client, err := fencerpc.NewClient(writefence.Identity, targets)
	if err != nil {
		t.Fatal(err)
	}
	f.controlLoss = &lostControlWriter{ControlWriter: client, kind: writefence.Session}
	return writefence.NewCoordinator(pgfence.NewAuthorityStore(f.idb), f.controlLoss)
}

func newRegistrationFixture(t *testing.T) *registrationFixture {
	t.Helper()
	f := &registrationFixture{idb: registrationDB(t, "identity"), cdb: registrationDB(t, "creation"), edb: registrationDB(t, "execution"), clock: time.Now().UTC().Truncate(time.Microsecond)}
	var err error
	f.protector, err = credentialproof.New("registration-fixture", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	f.store = pgidentity.NewRegistrationStore(f.idb)
	f.auth = identity.NewEmailAuth(f.store, f.protector, identity.EmailPolicy{TermsVersion: "registration-test-terms", SourceHourlyLimit: 100000, GlobalHourlyLimit: 100000}, func() time.Time { return f.clock })
	if err := f.idb.Exec(`UPDATE identity.platform_settings SET registration_enabled=true`).Error; err != nil {
		t.Fatal(err)
	}
	tls := newTLS(t)
	f.tls = tls
	f.writers = newRegistrationWriterCoordinator(t, f, tls)
	token := strings.Repeat("identity-registration-", 3)
	scope, err := workspacesource.NewPersonalRegistrations(workspacesourcepg.NewPersonalRegistrationStore(f.cdb))
	if err != nil {
		t.Fatal(err)
	}
	wallet, err := execution.NewRegistrationWallets(pgexecution.NewRegistrationWalletStore(f.edb))
	if err != nil {
		t.Fatal(err)
	}
	readyScope, err := workspacesource.NewReadyPersonalRegistrations(scope, workspacesourcepg.New(f.cdb), f.creationWriters)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := creationrpc.NewServer(readyScope, token, "identity.registration", tls.server(t, "creation.registration"))
	if err != nil {
		t.Fatal(err)
	}
	ws, err := executionrpc.NewServer(wallet, token+"wallet", "identity.registration", tls.server(t, "execution.registration"))
	if err != nil {
		t.Fatal(err)
	}
	creationAddress := serve(t, cs)
	f.creationPeer = registrationEndpoint{Address: creationAddress, Token: token, Name: "creation.registration"}
	cc, err := identityrpc.NewPersonalScopeClient(identityrpc.Target{Address: creationAddress, Token: token, Transport: tls.client(t, "creation.registration", "identity.registration")})
	if err != nil {
		t.Fatal(err)
	}
	walletAddress := serve(t, ws)
	f.walletPeer = registrationEndpoint{Address: walletAddress, Token: token + "wallet", Name: "execution.registration"}
	wc, err := identityrpc.NewWalletClient(identityrpc.Target{Address: walletAddress, Token: token + "wallet", Transport: tls.client(t, "execution.registration", "identity.registration")})
	if err != nil {
		t.Fatal(err)
	}
	f.creation = cc
	f.wallet = &ackLostParticipant{participant: wc}
	if err := f.auth.WithRegistrationPreparation(f.store, cc, f.wallet, f.writers); err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *registrationFixture) input(t *testing.T) identity.CompleteRegistrationInput {
	t.Helper()
	return f.inputForEmail(t, uuid.NewString()+"@registration.invalid")
}
func (f *registrationFixture) inputForEmail(t *testing.T, email string) identity.CompleteRegistrationInput {
	t.Helper()
	start, err := f.auth.StartRegistrationFrom(t.Context(), email, "", uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	var mail struct {
		EventID          uuid.UUID
		KeyID            string
		EncryptedPayload []byte
	}
	if err := f.idb.Raw(`SELECT m.event_id,m.key_id,m.encrypted_payload FROM identity.mail_delivery m JOIN identity.auth_challenge c ON c.id=m.challenge_id WHERE c.operation_id=? AND m.template='registration_code'`, start.OperationID).Scan(&mail).Error; err != nil {
		t.Fatal(err)
	}
	plain, err := f.protector.Open(mail.EventID, mail.KeyID, mail.EncryptedPayload)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(plain)
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(plain, &payload); err != nil {
		t.Fatal(err)
	}
	proof, err := f.auth.VerifyRegistration(t.Context(), start.OperationID, start.FlowSecret, payload.Code)
	if err != nil {
		t.Fatal(err)
	}
	return identity.CompleteRegistrationInput{OperationID: start.OperationID, FlowSecret: start.FlowSecret, Grant: proof.Grant, DisplayName: "准备接受测试", Password: "registration-test-password", ConfirmPassword: "registration-test-password", TermsVersion: "registration-test-terms", TermsAccepted: true}
}
func count(t *testing.T, db *gorm.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.Raw(query, args...).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRegistrationPostgresWalletNeedsNoCreationSQL(t *testing.T) {
	f := newRegistrationFixture(t)
	in := f.input(t)
	_, _ = f.auth.CompleteRegistration(t.Context(), in)
	p, err := f.store.LoadAcceptedRegistration(t.Context(), in.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pgexecution.NewRegistrationWalletStore(f.edb).PrepareRegistrationWallet(t.Context(), execution.RegistrationWallet{OperationID: p.OperationID, AccountID: p.AccountID, OrganizationID: p.OrganizationID, ScopeID: p.ScopeID, InputSHA256: p.InputSHA256, CreationReady: true})
	if err != nil {
		t.Fatalf("accepted wallet source must prepare with Execution role alone: %v", err)
	}
}

func TestRegistrationPostgresSourceScopesNeedNoIdentitySQL(t *testing.T) {
	f := newRegistrationFixture(t)
	in := f.input(t)
	_, _ = f.auth.CompleteRegistration(t.Context(), in)
	p, err := f.store.LoadAcceptedRegistration(t.Context(), in.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	r, err := workspacesourcepg.NewPersonalRegistrationStore(f.cdb).PreparePersonalRegistration(t.Context(), workspacesource.PersonalRegistration{OperationID: p.OperationID, AccountID: p.AccountID, OrganizationID: p.OrganizationID, ScopeID: p.ScopeID, InputSHA256: p.InputSHA256})
	if err != nil {
		t.Fatalf("accepted source must prepare with Creation role alone: %v", err)
	}
	if r.AccountID != p.AccountID {
		t.Fatal("source receipt changed account")
	}
}

func TestRegistrationPostgresUnknownIsInvisibleAndOriginalProofRecovers(t *testing.T) {
	f := newRegistrationFixture(t)
	in := f.input(t)
	f.wallet.lose.Store(true)
	if _, err := f.auth.CompleteRegistration(t.Context(), in); err == nil {
		t.Fatal("lost wallet acknowledgement reported registration success")
	}
	var pending struct {
		AccountID, ScopeID         uuid.UUID
		EmailKey                   string
		CreationReady, WalletReady bool
	}
	if err := f.idb.Raw(`SELECT account_id,scope_id,email_key,creation_ready,wallet_ready FROM identity.registration_preparation WHERE operation_id=?`, in.OperationID).Scan(&pending).Error; err != nil {
		t.Fatal(err)
	}
	if pending.AccountID == uuid.Nil || !pending.CreationReady || pending.WalletReady {
		t.Fatal("pending responsibility or exact readiness lost")
	}
	if n := count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE id=?`, pending.AccountID); n != 0 {
		t.Fatal("pending account visible")
	}
	if _, err := f.auth.Login(t.Context(), pending.EmailKey, in.Password, false); err == nil {
		t.Fatal("pending account logged in")
	}
	if n := count(t, f.idb, `SELECT count(*) FROM infra.task_work WHERE kind=? AND business_id=?`, identity.RegistrationRecoveryTask, in.OperationID.String()); n != 1 {
		t.Fatal("missing original recovery task")
	}
	changed := in
	changed.Password = "different-test-password"
	changed.ConfirmPassword = changed.Password
	if _, err := f.auth.CompleteRegistration(t.Context(), changed); !errors.Is(err, identity.ErrProofInvalid) {
		t.Fatalf("retargeted original input=%v", err)
	}
	if err := f.idb.Exec(`UPDATE identity.auth_challenge SET created_at=statement_timestamp()-interval '2 hours',expires_at=statement_timestamp()-interval '1 hour' WHERE operation_id=?`, in.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.idb.Exec(`UPDATE identity.platform_settings SET registration_enabled=false`).Error; err != nil {
		t.Fatal(err)
	}
	id, err := f.auth.CompleteRegistration(t.Context(), in)
	if err != nil || id != pending.AccountID {
		t.Fatalf("original recovery=%s %v", id, err)
	}
	again, err := f.auth.CompleteRegistration(t.Context(), in)
	if err != nil || again != id {
		t.Fatalf("activation acknowledgement recovery=%s %v", again, err)
	}
	if n := count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE id=?`, id); n != 1 {
		t.Fatal("duplicate activated identity")
	}
	if n := count(t, f.idb, `SELECT count(*) FROM identity.user_session WHERE account_id=?`, id); n != 0 {
		t.Fatal("registration recovery minted a session")
	}
	if n := count(t, f.cdb, `SELECT count(*) FROM workspace.creation_scope WHERE id=?`, pending.ScopeID); n != 1 {
		t.Fatal("duplicate or missing scope")
	}
	if n := count(t, f.edb, `SELECT count(*) FROM billing.wallet_account WHERE scope_id=?`, pending.ScopeID); n != 1 {
		t.Fatal("duplicate or missing wallet")
	}
	if n := count(t, f.idb, `SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action='account.registered'`, id); n != 1 {
		t.Fatal("registered audit not exact once")
	}
	if _, err := f.auth.Login(t.Context(), pending.EmailKey, in.Password, false); err != nil {
		t.Fatalf("activated login=%v", err)
	}
	if err := f.idb.Exec(`UPDATE identity.platform_settings SET registration_enabled=true`).Error; err != nil {
		t.Fatal(err)
	}
}

func TestRegistrationPostgresAcceptedProofSurvivesPolicyUpgrade(t *testing.T) {
	f := newRegistrationFixture(t)
	input := f.input(t)
	f.wallet.lose.Store(true)
	if _, err := f.auth.CompleteRegistration(t.Context(), input); err == nil {
		t.Fatal("lost ACK reported success")
	}
	accepted, err := f.store.LoadAcceptedRegistration(t.Context(), input.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	upgraded := identity.NewEmailAuth(f.store, f.protector, identity.EmailPolicy{TermsVersion: "registration-upgraded-terms", SourceHourlyLimit: 100000, GlobalHourlyLimit: 100000, BlockedPasswords: []string{input.Password}}, func() time.Time { return f.clock })
	if err := upgraded.WithRegistrationPreparation(f.store, f.creation, f.wallet, f.writers); err != nil {
		t.Fatal(err)
	}
	id, err := upgraded.CompleteRegistration(t.Context(), input)
	if err != nil || id != accepted.AccountID {
		t.Fatalf("original accepted proof after policy upgrade=%s %v", id, err)
	}
	var terms string
	if err := f.idb.Raw(`SELECT terms_version FROM identity."user" WHERE id=?`, id).Scan(&terms).Error; err != nil || terms != input.TermsVersion {
		t.Fatalf("accepted terms changed=%q %v", terms, err)
	}
	fresh := f.input(t)
	if _, err := upgraded.CompleteRegistration(t.Context(), fresh); err == nil {
		t.Fatal("new request bypassed current terms")
	}
	fresh.TermsVersion = "registration-upgraded-terms"
	if _, err := upgraded.CompleteRegistration(t.Context(), fresh); err == nil {
		t.Fatal("new request bypassed current password policy")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.registration_preparation WHERE operation_id=?`, fresh.OperationID) != 0 {
		t.Fatal("invalid new request reserved an identity")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.user_session WHERE account_id=?`, id) != 0 {
		t.Fatal("policy recovery minted a session")
	}
}

func TestRegistrationPostgresConcurrentReplayAndMailboxReservation(t *testing.T) {
	f := newRegistrationFixture(t)
	in := f.input(t)
	var group sync.WaitGroup
	results := make(chan uuid.UUID, 12)
	failures := make(chan error, 12)
	for range 12 {
		group.Go(func() {
			id, err := f.auth.CompleteRegistration(t.Context(), in)
			if err != nil {
				failures <- err
			} else {
				results <- id
			}
		})
	}
	group.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	var id uuid.UUID
	for result := range results {
		if id != uuid.Nil && id != result {
			t.Fatal("same command produced multiple identities")
		}
		id = result
	}
	if id == uuid.Nil {
		t.Fatal("no activated registration")
	}
	if n := count(t, f.idb, `SELECT count(*) FROM identity.registration_preparation WHERE operation_id=?`, in.OperationID); n != 1 {
		t.Fatal("duplicate accepted operation")
	}
	if n := count(t, f.idb, `SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action='account.registered'`, id); n != 1 {
		t.Fatal("duplicate registration audit")
	}
}

func TestRegistrationPostgresConcurrentDifferentProofsReserveOneMailbox(t *testing.T) {
	f := newRegistrationFixture(t)
	email := uuid.NewString() + "@reservation.invalid"
	first := f.inputForEmail(t, email)
	bucket := hex.EncodeToString(f.protector.Digest("email_budget", uuid.Nil, email))
	if err := f.idb.Exec(`UPDATE identity.email_budget SET sent_at=ARRAY[statement_timestamp()-interval '61 seconds'] WHERE bucket_key=?`, bucket).Error; err != nil {
		t.Fatal(err)
	}
	second := f.inputForEmail(t, email)
	results := make(chan error, 2)
	var group sync.WaitGroup
	for _, input := range []identity.CompleteRegistrationInput{first, second} {
		group.Go(func() { _, err := f.auth.CompleteRegistration(t.Context(), input); results <- err })
	}
	group.Wait()
	close(results)
	success, rejected := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, identity.ErrProofInvalid):
			rejected++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("mailbox race success=%d rejected=%d", success, rejected)
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.registration_preparation WHERE email_key=?`, email) != 1 || count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE email_key=?`, email) != 1 {
		t.Fatal("mailbox admitted more than one original registration")
	}
}

func TestRegistrationPostgresOwnerBoundaryAndNoOuterTransaction(t *testing.T) {
	f := newRegistrationFixture(t)
	if err := f.idb.Exec(`SELECT id FROM workspace.creation_scope LIMIT 1`).Error; err == nil {
		t.Fatal("Identity reads Creation tables")
	}
	if err := f.cdb.Exec(`SELECT id FROM identity."user" LIMIT 1`).Error; err == nil {
		t.Fatal("Creation reads Identity tables")
	}
	if err := f.edb.Exec(`SELECT id FROM workspace.creation_scope LIMIT 1`).Error; err == nil {
		t.Fatal("Execution reads Creation tables")
	}
	p := identity.RegistrationPreparation{OperationID: uuid.New(), AccountID: uuid.New(), OrganizationID: uuid.New(), ScopeID: uuid.New(), InputSHA256: strings.Repeat("a", 64)}
	if err := f.idb.Transaction(func(tx *gorm.DB) error {
		_, err := pgidentity.NewRegistrationStore(tx).ActivateRegistration(t.Context(), p)
		if err == nil {
			t.Fatal("registration accepted outer transaction")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, database := range []*gorm.DB{f.idb, f.cdb, f.edb} {
		var ddl bool
		if err := database.Raw(`SELECT has_database_privilege(current_user,current_database(),'CREATE') OR has_schema_privilege(current_user,'public','CREATE')`).Scan(&ddl).Error; err != nil || ddl {
			t.Fatalf("runtime role has DDL capability=%v %v", ddl, err)
		}
	}
}

type blockedRegistrationParticipant struct {
	participant identity.RegistrationParticipant
	entered     chan struct{}
	release     chan struct{}
}

func (p *blockedRegistrationParticipant) PrepareRegistration(ctx context.Context, v identity.RegistrationPreparation) (identity.RegistrationReady, error) {
	r, err := p.participant.PrepareRegistration(ctx, v)
	if err != nil {
		return r, err
	}
	close(p.entered)
	select {
	case <-p.release:
		return r, nil
	case <-ctx.Done():
		return identity.RegistrationReady{}, ctx.Err()
	}
}

func TestRegistrationPostgresRecoveryLeaseFencesActivation(t *testing.T) {
	f := newRegistrationFixture(t)
	in := f.input(t)
	f.wallet.lose.Store(true)
	if _, err := f.auth.CompleteRegistration(t.Context(), in); err == nil {
		t.Fatal("lost ACK succeeded")
	}
	tasks := taskruntime.NewStore(f.idb).ForBusiness(in.OperationID.String())
	blocked := &blockedRegistrationParticipant{participant: f.wallet, entered: make(chan struct{}), release: make(chan struct{})}
	coordinator, err := identity.NewRegistrationPreparation(f.store, f.creation, blocked, f.writers)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	attempt := make(chan error, 1)
	claims := make(chan taskruntime.Job, 1)
	worker := &taskruntime.Worker{Store: tasks, Concurrency: 1, Lease: 30 * time.Second, Poll: time.Millisecond, Handlers: map[string]taskruntime.Handler{
		identity.RegistrationRecoveryTask: func(ctx context.Context, j taskruntime.Job) (time.Time, error) {
			claims <- j
			_, err := coordinator.ResumeAccepted(ctx, in.OperationID, f.store)
			attempt <- err
			cancel()
			return time.Time{}, err
		},
	}}
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	select {
	case <-blocked.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("recovery worker did not reach participant")
	}
	old := <-claims
	if err := f.idb.Exec(`UPDATE infra.task_work SET owner='replacement-registration-worker',epoch=epoch+1,lease_until=statement_timestamp()-interval '1 second' WHERE kind=? AND business_id=?`, identity.RegistrationRecoveryTask, in.OperationID.String()).Error; err != nil {
		t.Fatal(err)
	}
	close(blocked.release)
	select {
	case err := <-attempt:
		if !errors.Is(err, taskruntime.ErrLeaseLost) {
			t.Fatalf("old source transaction=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("old worker did not finish")
	}
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if n := count(t, f.idb, `SELECT count(*) FROM identity.registration_preparation WHERE operation_id=? AND (wallet_ready OR activated_at IS NOT NULL)`, in.OperationID); n != 0 {
		t.Fatal("stale worker committed readiness or activation")
	}
	if err := tasks.Complete(t.Context(), old); !errors.Is(err, taskruntime.ErrLeaseLost) {
		t.Fatalf("old task completion=%v", err)
	}
	coordinator, err = identity.NewRegistrationPreparation(f.store, f.creation, f.wallet, f.writers)
	if err != nil {
		t.Fatal(err)
	}
	recoveryCtx, stop := context.WithCancel(t.Context())
	defer stop()
	worker.Handlers[identity.RegistrationRecoveryTask] = func(ctx context.Context, _ taskruntime.Job) (time.Time, error) {
		_, err := coordinator.ResumeAccepted(ctx, in.OperationID, f.store)
		return time.Time{}, err
	}
	go func() { done <- worker.Run(recoveryCtx) }()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case <-timeout.C:
			stop()
			if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
				t.Error(err)
			}
			t.Fatal("reconstructed recovery worker did not complete original task")
		case <-tick.C:
			if count(t, f.idb, `SELECT count(*) FROM infra.task_work WHERE kind=? AND business_id=? AND completed_at IS NOT NULL`, identity.RegistrationRecoveryTask, in.OperationID.String()) == 1 {
				stop()
				if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				return
			}
		}
	}
}
