package microservices_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	identitydomain "github.com/StephenQiu30/glowloom-server/backend/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	fencerpc "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func TestIdentitySessionRevocationResumesOriginalSourceDecisionAfterLostACK(t *testing.T) {
	for _, mode := range []string{"other-device", "current-device"} {
		t.Run(mode, func(t *testing.T) {
			testSessionRevocationRecovery(t, mode == "current-device")
		})
	}
}

func testSessionRevocationRecovery(t *testing.T, current bool) {
	t.Helper()
	admin := fenceDatabase(t, "")
	database := authorityDatabase(t, "identity")
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor, target := sessionRevocationActor(t, admin, now)
	if current {
		actor.SessionID = target
	}
	store := pgidentity.NewSessionRevocationStore(database)
	targets := make(map[writefence.Writer]fencerpc.Target)
	stores := make(map[writefence.Writer]*pgfence.Store)
	calls := make(map[writefence.Writer]*atomic.Int32)
	var loseACK, sourceLocksReleased atomic.Bool
	for _, writer := range writefence.Writers() {
		writerDB := authorityDatabase(t, string(writer))
		stores[writer] = pgfence.New(writerDB)
		server, err := fencerpc.NewServer(writer, stores[writer], identityFenceToken(writer), creationFenceToken(writer))
		if err != nil {
			t.Fatal(err)
		}
		counter := new(atomic.Int32)
		calls[writer] = counter
		address := startFenceRPC(t, server, func(ctx context.Context, request any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			counter.Add(1)
			if writer == writefence.WriterIdentity && loseACK.Load() {
				err := admin.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
					if err := tx.Exec(`SELECT id FROM identity."user" WHERE id=? FOR UPDATE NOWAIT`, actor.ID).Error; err != nil {
						return err
					}
					return tx.Exec(`SELECT id FROM identity.user_session WHERE id=? FOR UPDATE NOWAIT`, target).Error
				})
				if err != nil {
					return nil, status.Error(codes.Internal, "source retained policy locks across RPC")
				}
				sourceLocksReleased.Store(true)
			}
			response, err := handler(ctx, request)
			if err == nil && writer == writefence.WriterMedia && loseACK.CompareAndSwap(true, false) {
				return nil, status.Error(codes.Unavailable, "synthetic response loss after durable install")
			}
			return response, err
		})
		targets[writer] = fencerpc.Target{Address: address, Token: identityFenceToken(writer), Transport: insecure.NewCredentials()}
	}
	client, err := fencerpc.NewClient(writefence.Identity, targets)
	if err != nil {
		t.Fatal(err)
	}
	coordinator := writefence.NewCoordinator(pgfence.NewAuthorityStore(database), client)
	authorities := []writefence.Authority{
		{Issuer: writefence.Identity, Kind: writefence.Account, ID: actor.ID},
		{Issuer: writefence.Identity, Kind: writefence.Session, ID: target},
	}
	for i, authority := range authorities {
		state, err := coordinator.Set(t.Context(), authority, uuid.New(), false)
		if err != nil || !state.Complete {
			t.Fatalf("initialize live policy authorities: %+v %v", state, err)
		}
		authorities[i] = state.Control.Authority
	}
	command := authorityCommand(authorities[0])
	command.Action = "operation.confirm"
	command.Authorities = authorities
	targetActor := actor
	targetActor.SessionID = target
	admissions := pgidentity.NewSessionAdmissionStore(database)
	if err := admissions.RegisterSessionCommand(t.Context(), targetActor, command, writefence.WriterExecution, func() time.Time { return now }); err != nil {
		t.Fatal("register actual session policy:", err)
	}
	if _, err := stores[writefence.WriterExecution].Admit(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	for _, counter := range calls {
		counter.Store(0)
	}
	loseACK.Store(true)
	service := identity.NewSessionRevocations(store, coordinator, func() time.Time { return now })
	if err := service.Revoke(t.Context(), actor, target); !errors.Is(err, writefence.ErrUnavailable) {
		t.Fatalf("lost ACK reported successful revocation: %v", err)
	}
	pending, err := store.LoadSessionRevocation(t.Context(), target)
	if err != nil || pending.CompletedAt != nil || pending.Control.Epoch != 2 {
		t.Fatalf("source did not retain original pending decision: %+v %v", pending, err)
	}
	assertSessionLocallyRevoked(t, admin, target)
	if !sourceLocksReleased.Load() {
		t.Fatal("remote control was not observed after source transaction release")
	}
	var events int
	if err := admin.Raw(`SELECT count(*) FROM identity.security_event WHERE request_id=?`, pending.Control.CoordinationID.String()).Scan(&events).Error; err != nil || events != 0 {
		t.Fatalf("pending source recorded successful completion: %d %v", events, err)
	}
	if current {
		// A cleaned-up credential is not evidence that its original distributed
		// responsibility disappeared. Resume must use the retained source journal.
		if err := admin.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(`DELETE FROM identity.session_refresh_token WHERE session_id=?`, target).Error; err != nil {
				return err
			}
			return tx.Exec(`DELETE FROM identity.user_session WHERE id=?`, target).Error
		}); err != nil {
			t.Fatal(err)
		}
	}
	// A restart reconstructs all source/transport state from durable records.
	restartedClient, err := fencerpc.NewClient(writefence.Identity, targets)
	if err != nil {
		t.Fatal(err)
	}
	restarted := identity.NewSessionRevocations(pgidentity.NewSessionRevocationStore(database), writefence.NewCoordinator(pgfence.NewAuthorityStore(database), restartedClient), func() time.Time { return now.Add(time.Second) })
	if err := restarted.Resume(t.Context(), target); err != nil {
		t.Fatal(err)
	}
	completed, err := store.LoadSessionRevocation(t.Context(), target)
	if err != nil || completed.CompletedAt == nil || completed.Control != pending.Control || completed.RequestSessionID != actor.SessionID {
		t.Fatalf("restart changed original revocation: %+v %v", completed, err)
	}
	if calls[writefence.WriterIdentity].Load() != 1 || calls[writefence.WriterCreation].Load() != 1 || calls[writefence.WriterMedia].Load() != 2 || calls[writefence.WriterExecution].Load() != 1 {
		t.Fatal("recovery replayed already-acknowledged writers or lost original pending writer")
	}
	if err := restarted.Resume(t.Context(), target); err != nil {
		t.Fatal(err)
	}
	if err := service.Revoke(t.Context(), actor, target); (!current && err != nil) || (current && !errors.Is(err, identity.ErrSessionInvalid)) {
		t.Fatalf("repeated policy did not preserve current actor requirements: %v", err)
	}
	changed := command
	changed.ID = uuid.New()
	if err := admissions.RegisterSessionCommand(t.Context(), targetActor, changed, writefence.WriterExecution, func() time.Time { return now }); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("revoked session registered new work: %v", err)
	}
	effectDB := authorityDatabase(t, "execution")
	if err := effectDB.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		return stores[writefence.WriterExecution].Guard(t.Context(), tx, command)
	}); !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("old admitted command crossed acknowledged revocation: %v", err)
	}
	if err := admin.Raw(`SELECT count(*) FROM identity.security_event WHERE request_id=? AND action='session.revoked'`, pending.Control.CoordinationID.String()).Scan(&events).Error; err != nil || events != 1 {
		t.Fatalf("completion audit was not exactly once: %d %v", events, err)
	}
}

func TestIdentitySessionRevocationCompletesFirstTryWithNanosecondClock(t *testing.T) {
	admin := fenceDatabase(t, "")
	database := authorityDatabase(t, "identity")
	now := time.Now().UTC().Truncate(time.Microsecond).Add(321 * time.Nanosecond)
	actor, target := sessionRevocationActor(t, admin, now)
	writers := make(authorityLocalWriters)
	for _, writer := range writefence.Writers() {
		writers[writer] = pgfence.New(authorityDatabase(t, string(writer)))
	}
	store := pgidentity.NewSessionRevocationStore(database)
	service := identity.NewSessionRevocations(store, writefence.NewCoordinator(pgfence.NewAuthorityStore(database), writers), func() time.Time { return now })
	if err := service.Revoke(t.Context(), actor, target); err != nil {
		t.Fatal("first completion rejected a normal nanosecond clock:", err)
	}
	completed, err := store.LoadSessionRevocation(t.Context(), target)
	if err != nil || completed.CompletedAt == nil {
		t.Fatalf("source was not durably completed: %+v %v", completed, err)
	}
	if completed.RequestedAt.Nanosecond()%1000 != 0 || completed.CompletedAt.Nanosecond()%1000 != 0 {
		t.Fatal("source receipt does not preserve PostgreSQL timestamp precision")
	}
}

func TestIdentitySessionRevocationEnforcesCurrentPolicyBeforeClosure(t *testing.T) {
	admin := fenceDatabase(t, "")
	database := authorityDatabase(t, "identity")
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor, target := sessionRevocationActor(t, admin, now)
	other, _ := sessionRevocationActor(t, admin, now)
	store := pgidentity.NewSessionRevocationStore(database)
	for _, candidate := range []identity.Principal{other, {ID: actor.ID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch + 1}, {ID: actor.ID, SessionID: uuid.New(), CredentialEpoch: actor.CredentialEpoch}} {
		if _, err := store.PrepareSessionRevocation(t.Context(), candidate, target, func() time.Time { return now }); !errors.Is(err, identity.ErrSessionInvalid) {
			t.Fatalf("foreign or stale principal prepared closure: %v", err)
		}
	}
	var count int
	if err := admin.Raw(`SELECT count(*) FROM identity.session_revocation WHERE session_id=?`, target).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("rejected policy created source responsibility: %d %v", count, err)
	}
	var session struct{ RevokedAt *time.Time }
	if err := admin.Raw(`SELECT revoked_at FROM identity.user_session WHERE id=?`, target).Scan(&session).Error; err != nil || session.RevokedAt != nil {
		t.Fatalf("rejected policy revoked target: %v %v", session.RevokedAt, err)
	}
	if _, err := pgidentity.NewSessionRevocationStore(authorityDatabase(t, "execution")).PrepareSessionRevocation(t.Context(), actor, target, func() time.Time { return now }); err == nil {
		t.Fatal("another writer prepared Identity policy")
	}
}

func TestIdentitySessionRevocationPreparationRollsBackAsOneTransaction(t *testing.T) {
	admin := fenceDatabase(t, "")
	database := authorityDatabase(t, "identity")
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor, target := sessionRevocationActor(t, admin, now)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, err := pgidentity.NewSessionRevocationStore(database).PrepareSessionRevocation(ctx, actor, target, func() time.Time {
		cancel() // The clock is sampled only after the real policy locks.
		return now
	})
	if err == nil {
		t.Fatal("canceled preparation committed")
	}
	var pending int
	if err := admin.Raw(`SELECT count(*) FROM identity.session_revocation WHERE session_id=?`, target).Scan(&pending).Error; err != nil || pending != 0 {
		t.Fatalf("rolled-back decision retained source journal: %d %v", pending, err)
	}
	var active int
	if err := admin.Raw(`SELECT count(*) FROM identity.session_refresh_token WHERE session_id=? AND revoked_at IS NULL`, target).Scan(&active).Error; err != nil || active != 1 {
		t.Fatalf("preparation partially revoked refresh chain: %d %v", active, err)
	}
	if err := database.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		_, err := pgidentity.NewSessionRevocationStore(tx).PrepareSessionRevocation(t.Context(), actor, target, func() time.Time { return now })
		if !errors.Is(err, writefence.ErrTransaction) {
			t.Fatalf("outer transaction could leak across RPC: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestIdentitySessionRevocationNeedsAllWriterEvidenceBeforeSourceCompletion(t *testing.T) {
	admin := fenceDatabase(t, "")
	database := authorityDatabase(t, "identity")
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor, target := sessionRevocationActor(t, admin, now)
	store := pgidentity.NewSessionRevocationStore(database)
	ticket, err := store.PrepareSessionRevocation(t.Context(), actor, target, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteSessionRevocation(t.Context(), ticket, now); err == nil {
		t.Fatal("source completion accepted missing writer ACKs")
	}
	loaded, err := store.LoadSessionRevocation(t.Context(), target)
	if err != nil || loaded.CompletedAt != nil || loaded.Control != ticket.Control {
		t.Fatalf("failed completion erased original responsibility: %+v %v", loaded, err)
	}
}

func sessionRevocationActor(t *testing.T, admin *gorm.DB, now time.Time) (identity.Principal, uuid.UUID) {
	t.Helper()
	uid, actorSession, target := uuid.New(), uuid.New(), uuid.New()
	store := pgidentity.NewStore(admin)
	if err := store.EmailTransaction(t.Context(), func(tx identity.EmailTx) error {
		if err := tx.CreatePersonalAccount(identitydomain.User{ID: uid, OrgID: uuid.New(), LoginName: uuid.NewString(), Email: uid.String() + "@example.invalid", EmailVersion: 1, EmailVerifiedAt: now, DisplayName: "session revocation fixture", PasswordHash: "synthetic-unused-password-hash", PasswordChangedAt: now, TermsVersion: "test", TermsAcceptedAt: now}); err != nil {
			return err
		}
		for _, id := range []uuid.UUID{actorSession, target} {
			digest := sha256.Sum256(id[:])
			if err := tx.InsertSession(identitydomain.AuthSession{ID: id, AccountID: uid, TokenHash: digest[:], CredentialRevision: 1, CreatedAt: now, LastActiveAt: now, AbsoluteExpiresAt: now.Add(time.Hour), ProtocolVersion: 2, IdleTimeoutSeconds: 43200}); err != nil {
				return err
			}
			refreshDigest := sha256.Sum256(append(id[:], 'r'))
			if err := tx.InsertRefresh(identitydomain.RefreshToken{ID: uuid.New(), SessionID: id, AccountID: uid, Digest: refreshDigest[:], IssuedAt: now}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return identity.Principal{ID: uid, SessionID: actorSession, CredentialEpoch: 1}, target
}

func assertSessionLocallyRevoked(t *testing.T, admin *gorm.DB, id uuid.UUID) {
	t.Helper()
	var closed bool
	if err := admin.Raw(`SELECT revoked_at IS NOT NULL FROM identity.user_session WHERE id=?`, id).Scan(&closed).Error; err != nil || !closed {
		t.Fatalf("source session is still active: %v", err)
	}
	var active int
	if err := admin.Raw(`SELECT count(*) FROM identity.session_refresh_token WHERE session_id=? AND (revoked_at IS NULL OR replay_until IS NOT NULL OR receipt_ciphertext IS NOT NULL)`, id).Scan(&active).Error; err != nil || active != 0 {
		t.Fatalf("source left active refresh material: %d %v", active, err)
	}
}
