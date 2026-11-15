package microservices_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/credentials"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/identitysource"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/pgsource"
	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	identityrpc "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcauthority"

	identitypg "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"

	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// This supplies actual Identity credentials and source heads with deliberately
// different generations. Registration/scope initialization remains an explicit
// fixture, not a claim that the full public registration/main has been accepted.
func nativeProjectIdentity(t *testing.T) (*sourcePolicyFixture, *identitysource.Client, workspacesource.Credential, time.Time) {
	t.Helper()
	serverTransport, clientTransport, _ := ownerPeerTLS(t, "creation")
	f, client, credential, now, _, _ := nativeProjectIdentityTLS(t, serverTransport, clientTransport)
	return f, client, credential, now
}

func nativeProjectIdentityTLS(t *testing.T, serverTransport, clientTransport credentials.TransportCredentials) (*sourcePolicyFixture, *identitysource.Client, workspacesource.Credential, time.Time, string, string) {
	t.Helper()
	f := newSourcePolicyFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	account, session := f.request.ActorID, f.request.SessionID
	raw := sha256.Sum256(session[:])
	legacy := base64.RawURLEncoding.EncodeToString(raw[:])
	tokenHash := sha256.Sum256(raw[:])
	csrf := base64.RawURLEncoding.EncodeToString(sourceProofProtector{}.Digest("session_csrf", session, legacy))
	csrfHash := sha256.Sum256([]byte(csrf))
	if err := f.admin.Exec(`UPDATE identity."user" SET email_key=?,email_verified_at=? WHERE id=?`, account.String()+"@native.invalid", now.Add(-time.Hour), account).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.admin.Exec(`INSERT INTO identity.user_session(id,account_id,token_hash,credential_revision,created_at,last_active_at,absolute_expires_at,persistent,protocol_version,csrf_hash) VALUES(?,?,?,1,?,?,?,false,1,?)`, session, account, tokenHash[:], now.Add(-time.Minute), now, now.Add(time.Hour), csrfHash[:]).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.creation.Exec(`UPDATE workspace.creation_scope SET revision=17 WHERE id=?`, f.request.ScopeID).Error; err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		authority writefence.Authority
		store     *pgfence.AuthorityStore
		epoch     int64
	}{{writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: account}, pgfence.NewAuthorityStore(f.identity), 2}, {writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Session, ID: session}, pgfence.NewAuthorityStore(f.identity), 3}, {writefence.Authority{Issuer: writefence.Creation, Kind: writefence.Scope, ID: f.request.ScopeID, Epoch: 1}, pgfence.NewAuthorityStore(f.creation), 4}} {
		authority := item.authority
		var control writefence.Control
		for authority.Epoch < item.epoch {
			control = completedAuthority(t, item.store, authority, false).Control
			authority = control.Authority
		}
		for _, db := range []*gorm.DB{f.identity, f.creation, f.execution, f.media} {
			if err := pgfence.New(db).Install(t.Context(), control); err != nil {
				t.Fatal(err)
			}
		}
	}
	tokens := map[writefence.Writer]string{}
	for _, writer := range writefence.Writers() {
		tokens[writer] = strings.Repeat(string(writer)+"-", 10)
	}
	auth := identity.NewEmailAuth(identitypg.NewStore(f.identity), sourceProofProtector{}, identity.EmailPolicy{}, func() time.Time { return now })
	server, err := identityrpc.NewServer(auth, identitypg.NewSessionAdmissionStore(f.identity), tokens, func() time.Time { return now }, serverTransport)
	if err != nil {
		t.Fatal(err)
	}
	address := serveProjectPreparation(t, server)
	client, err := identitysource.New(address, tokens[writefence.WriterCreation], clientTransport)
	if err != nil {
		t.Fatal(err)
	}
	return f, client, workspacesource.LegacyCredential(legacy, csrf), now, address, tokens[writefence.WriterCreation]
}

func waitNativeProjectTask(t *testing.T, f *sourcePolicyFixture, id uuid.UUID) {
	t.Helper()
	bounded, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var complete bool
		if err := f.admin.WithContext(bounded).Raw(`SELECT completed_at IS NOT NULL FROM infra.task_work WHERE service_owner='creation' AND kind=? AND business_id=?`, workspacesource.ProjectPreparationTask, id.String()).Scan(&complete).Error; err != nil {
			t.Fatal(err)
		}
		if complete {
			return
		}
		select {
		case <-ticker.C:
		case <-bounded.Done():
			t.Fatal("native project recovery worker did not complete original task")
		}
	}
}

func TestCreationNativeProjectUsesRealIdentityHeadSnapshotAndRecoversThroughActualWorker(t *testing.T) {
	f, authority, credential, now := nativeProjectIdentity(t)
	newUseCase, participant, _, _ := projectPreparationParticipants(t, f, true, false)
	store := pgsource.New(f.creation)
	entry, err := workspacesource.NewNativeProjectCreation(authority, store, workspacesource.NewAdmission(authority, store), pgfence.New(f.creation), newUseCase())
	if err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	input := workspacesource.CreateProjectInput{Name: "native actual project", Description: "native original", AspectRatio: "1:1", RequestID: uuid.NewString()}
	if _, err := entry.Execute(t.Context(), credential, f.request.ScopeID, key, input); !errors.Is(err, writefence.ErrUnavailable) {
		t.Fatalf("expected lost actual wallet ACK, got %v", err)
	}
	preparationID := uuid.NewSHA1(f.request.ActorID, []byte("creation.project.preparation/"+key.String()))
	worker := &taskruntime.Worker{Store: taskruntime.NewStore(f.creation).ForBusiness(preparationID.String()), Handlers: map[string]taskruntime.Handler{workspacesource.ProjectPreparationTask: newUseCase().Handle}, Concurrency: 1, Lease: time.Second, Poll: 5 * time.Millisecond}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			if err := <-done; err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(stop)
	waitNativeProjectTask(t, f, preparationID)
	stop()
	if participant.prepares != 1 || participant.resolves != 2 {
		t.Fatalf("worker changed original responsibility instead of Resolve: %d/%d", participant.prepares, participant.resolves)
	}
	scope, err := store.ReadProjectCreationScope(t.Context(), f.request.ActorID, f.request.ScopeID)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := workspacesource.ProjectCreationSHA256(scope.OrganizationID, scope.ScopeID, input)
	if err != nil {
		t.Fatal(err)
	}
	lookup, err := store.LookupProjectCreation(t.Context(), workspacesource.Identity{AccountID: f.request.ActorID, SessionID: f.request.SessionID, CredentialEpoch: 1}, scope, key, hash)
	if err != nil || lookup.Spec == nil || lookup.Activation == nil {
		t.Fatalf("native task did not commit original public result: %v", err)
	}
	spec := lookup.Spec
	if spec.ScopeRevision != 17 || spec.Command.Authorities[0].Epoch != 2 || spec.Command.Authorities[1].Epoch != 3 || spec.Command.Authorities[2].Epoch != 4 || spec.RequestID.String() != input.RequestID {
		t.Fatalf("native builder guessed generations or replaced original request: %+v", spec)
	}
	retry := input
	retry.RequestID = uuid.NewString()
	replayed, err := entry.Execute(t.Context(), credential, f.request.ScopeID, key, retry)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(lookup.Activation)
	actual, _ := json.Marshal(replayed)
	if string(original) != string(actual) || replayed.Status != "active" {
		t.Fatal("native retry replaced original response")
	}
	changed := retry
	changed.Name = "different logical request"
	if _, err := entry.Execute(t.Context(), credential, f.request.ScopeID, key, changed); !errors.Is(err, workspacesource.ErrIdempotencyConflict) {
		t.Fatalf("native duplicate key accepted changed input: %v", err)
	}
	_, token, _ := credential.IdentityProof()
	bad := sha256.Sum256([]byte("wrong-csrf" + f.request.SessionID.String()))
	if _, err := entry.Execute(t.Context(), workspacesource.LegacyCredential(token, base64.RawURLEncoding.EncodeToString(bad[:])), f.request.ScopeID, key, retry); !errors.Is(err, workspacesource.ErrSourceCSRFInvalid) {
		t.Fatalf("native replay lost exact Identity CSRF rejection: %v", err)
	}
	var activity struct{ LastActiveAt time.Time }
	if err := f.identity.Raw(`SELECT last_active_at FROM identity.user_session WHERE id=?`, f.request.SessionID).Scan(&activity).Error; err != nil || !activity.LastActiveAt.Equal(now) {
		t.Fatalf("authority snapshot/admission incorrectly refreshed activity: %+v %v", activity, err)
	}
}
