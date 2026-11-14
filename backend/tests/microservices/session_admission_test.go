package microservices_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func TestIdentitySessionAdmissionBindsExactAccountSessionAndCurrentPolicy(t *testing.T) {
	admin := fenceDatabase(t, "")
	database := authorityDatabase(t, "identity")
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor, otherSession := sessionRevocationActor(t, admin, now)
	authorities := []writefence.Authority{
		{Issuer: writefence.Identity, Kind: writefence.Account, ID: actor.ID},
		{Issuer: writefence.Identity, Kind: writefence.Session, ID: actor.SessionID},
	}
	for i, authority := range authorities {
		authorities[i] = completedAuthority(t, pgfence.NewAuthorityStore(database), authority, false).Control.Authority
	}
	command := authorityCommand(authorities[0])
	command.Authorities = authorities
	store := pgidentity.NewSessionAdmissionStore(database)
	if err := store.RegisterSessionCommand(t.Context(), actor, command, writefence.WriterExecution, func() time.Time { return now }); err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterSessionCommand(t.Context(), actor, command, writefence.WriterExecution, func() time.Time { return now }); err != nil {
		t.Fatal("exact replay was not idempotent:", err)
	}
	for _, shape := range []string{"missing_account", "missing_session", "foreign_session", "foreign_account", "extra_identity"} {
		t.Run(shape, func(t *testing.T) {
			changed := command
			changed.ID = uuid.New()
			changed.Authorities = append([]writefence.Authority(nil), command.Authorities...)
			switch shape {
			case "missing_account":
				changed.Authorities = changed.Authorities[1:]
			case "missing_session":
				changed.Authorities = changed.Authorities[:1]
			case "foreign_session":
				changed.Authorities[1].ID = otherSession
			case "foreign_account":
				changed.Authorities[0].ID = uuid.New()
			case "extra_identity":
				changed.Authorities = append(changed.Authorities, writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Session, ID: otherSession, Epoch: 1})
			}
			if err := store.RegisterSessionCommand(t.Context(), actor, changed, writefence.WriterExecution, func() time.Time { return now }); !errors.Is(err, identity.ErrForbidden) {
				t.Fatalf("unbound command reached authority ledger: %v", err)
			}
		})
	}
	// Actor validity and ledger registration use the same original policy locks.
	if err := admin.Exec(`UPDATE identity.user_session SET revoked_at=? WHERE id=?`, now, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	changed := command
	changed.ID = uuid.New()
	if err := store.RegisterSessionCommand(t.Context(), actor, changed, writefence.WriterExecution, func() time.Time { return now }); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("revoked actor registered fresh work: %v", err)
	}
	var count int
	if err := admin.Raw(`SELECT count(*) FROM infra.authority_admission WHERE id=?`, changed.ID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("invalid actor left an admission: %d %v", count, err)
	}
}
