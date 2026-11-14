package microservices_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func TestIdentitySessionAdmissionAndRevocationSerializeAtOriginalPolicyLock(t *testing.T) {
	for _, admissionFirst := range []bool{true, false} {
		name := "revocation-first"
		if admissionFirst {
			name = "admission-first"
		}
		t.Run(name, func(t *testing.T) {
			testSessionPolicyRace(t, admissionFirst)
		})
	}
}

func testSessionPolicyRace(t *testing.T, admissionFirst bool) {
	t.Helper()
	admin := fenceDatabase(t, "")
	database := authorityDatabase(t, "identity")
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor, target := sessionRevocationActor(t, admin, now)
	targetActor := actor
	targetActor.SessionID = target
	authorities := []writefence.Authority{
		{Issuer: writefence.Identity, Kind: writefence.Account, ID: actor.ID},
		{Issuer: writefence.Identity, Kind: writefence.Session, ID: target},
	}
	for i, authority := range authorities {
		authorities[i] = completedAuthority(t, pgfence.NewAuthorityStore(database), authority, false).Control.Authority
	}
	command := authorityCommand(authorities[0])
	command.Authorities = authorities
	admissions := pgidentity.NewSessionAdmissionStore(database)
	revocations := pgidentity.NewSessionRevocationStore(database)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	var workers sync.WaitGroup
	locked, release := make(chan struct{}), make(chan struct{})
	var signal, unblock sync.Once
	defer func() {
		cancel()
		unblock.Do(func() { close(release) })
		workers.Wait()
	}()
	heldClock := func() time.Time {
		signal.Do(func() { close(locked) })
		select {
		case <-release:
		case <-ctx.Done():
		}
		return now
	}
	run := func(admit bool, clock func() time.Time) error {
		if admit {
			return admissions.RegisterSessionCommand(ctx, targetActor, command, writefence.WriterExecution, clock)
		}
		_, err := revocations.PrepareSessionRevocation(ctx, actor, target, clock)
		return err
	}
	first, second := make(chan error, 1), make(chan error, 1)
	workers.Go(func() { first <- run(admissionFirst, heldClock) })
	select {
	case <-locked:
	case <-ctx.Done():
		t.Fatal("first operation did not acquire real policy locks")
	}
	workers.Go(func() { second <- run(!admissionFirst, func() time.Time { return now }) })
	waitIdentityPolicyBlock(ctx, t, admin)
	unblock.Do(func() { close(release) })
	if err := <-first; err != nil {
		t.Fatal("first policy operation failed:", err)
	}
	secondErr := <-second
	if (admissionFirst && secondErr != nil) || (!admissionFirst && !errors.Is(secondErr, identity.ErrSessionInvalid)) {
		t.Fatalf("second operation did not observe serialized policy: %v", secondErr)
	}
	assertSessionLocallyRevoked(t, admin, target)
	var count int
	if err := admin.Raw(`SELECT count(*) FROM infra.authority_admission WHERE id=?`, command.ID).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	want := 0
	if admissionFirst {
		want = 1
	}
	if count != want {
		t.Fatalf("source admission crossed policy closure: got %d, want %d", count, want)
	}
}

func waitIdentityPolicyBlock(ctx context.Context, t *testing.T, admin *gorm.DB) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		err := admin.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity
			WHERE datname=current_database() AND usename='owner_b_identity_test'
			AND query LIKE '%identity.%user%FOR UPDATE%' AND cardinality(pg_blocking_pids(pid))>0)`).Scan(&blocked).Error
		if err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("no real PostgreSQL policy-lock contention was observed")
		}
	}
}
