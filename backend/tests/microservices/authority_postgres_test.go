package microservices_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// The test DSN is a fixture administrator, but every operation below uses a real
// restricted LOGIN. These roles must be provisioned offline, not by application DDL.
func authorityDatabase(t *testing.T, owner string) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_WRITE_FENCE_DB_DSN")
	if dsn == "" {
		t.Skip("requires GLOWLOOM_TEST_WRITE_FENCE_DB_DSN and offline owner_b_*_test LOGIN fixtures")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") ||
		(parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost" && parsed.Hostname() != "::1") ||
		!strings.HasSuffix(strings.TrimPrefix(parsed.Path, "/"), "_test") || parsed.Port() == "" {
		t.Fatal("authority tests require an explicit isolated loopback *_test PostgreSQL URL")
	}
	if err := writefence.Writer(owner).Validate(); err != nil {
		t.Fatal(err)
	}
	parsed.User = url.User("owner_b_" + owner + "_test")
	query := parsed.Query()
	query.Del("options")
	parsed.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(parsed.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("open restricted authority fixture:", err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	})
	var safe bool
	if err := db.Raw(`SELECT infra.runtime_owner()=? AND NOT (rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls)
		AND NOT pg_has_role(session_user,'glowloom_app','MEMBER') FROM pg_roles WHERE rolname=session_user`, owner).Scan(&safe).Error; err != nil || !safe {
		t.Fatalf("fixture is not a restricted %s LOGIN: %t %v", owner, safe, err)
	}
	return db
}

func authorityCommand(authority writefence.Authority) writefence.Command {
	return writefence.Command{ID: uuid.New(), SHA256: strings.Repeat("a", 64), Action: "media.upload.publish", Authorities: []writefence.Authority{authority}}
}

func completedAuthority(t *testing.T, store *pgfence.AuthorityStore, authority writefence.Authority, blocked bool) writefence.Coordination {
	t.Helper()
	state, err := store.Begin(t.Context(), authority, uuid.New(), blocked)
	if err != nil {
		t.Fatal(err)
	}
	for _, writer := range writefence.Writers() {
		state, err = store.Acknowledge(t.Context(), state.Control, writer)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !state.Complete {
		t.Fatal("four exact ACKs did not complete coordination")
	}
	return state
}

func registerAuthority(t *testing.T, db *gorm.DB, store *pgfence.AuthorityStore, command writefence.Command, target writefence.Writer) error {
	t.Helper()
	return db.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error { return store.Register(t.Context(), tx, command, target) })
}

func TestAuthorityPostgresPersistsExactHistoryAndEpochCAS(t *testing.T) {
	db := authorityDatabase(t, "identity")
	store := pgfence.NewAuthorityStore(db)
	authority := writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Session, ID: uuid.New()}
	id := uuid.New()
	pending, err := store.Begin(t.Context(), authority, id, false)
	if err != nil || pending.Control.Epoch != 1 || pending.Complete {
		t.Fatalf("initialize: %+v %v", pending, err)
	}
	command := authorityCommand(pending.Control.Authority)
	if err := registerAuthority(t, db, store, command, writefence.WriterMedia); !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("pending open admitted: %v", err)
	}
	if _, err := store.Begin(t.Context(), pending.Control.Authority, uuid.New(), true); !errors.Is(err, writefence.ErrConflict) {
		t.Fatalf("preempted pending control: %v", err)
	}
	for _, mutate := range []func(*writefence.Authority){
		func(a *writefence.Authority) { a.Epoch++ }, func(a *writefence.Authority) { a.ID = uuid.New() }, func(a *writefence.Authority) { a.Kind = writefence.Account },
	} {
		changed := authority
		mutate(&changed)
		if _, err := store.Begin(t.Context(), changed, id, false); !errors.Is(err, writefence.ErrConflict) {
			t.Fatalf("coordination ID rebound: %v", err)
		}
	}
	if _, err := store.Begin(t.Context(), authority, id, true); !errors.Is(err, writefence.ErrConflict) {
		t.Fatalf("decision rebound: %v", err)
	}
	for i, writer := range writefence.Writers() {
		pending, err = store.Acknowledge(t.Context(), pending.Control, writer)
		if err != nil || pending.Complete != (i == 3) {
			t.Fatalf("ACK %s: %+v %v", writer, pending, err)
		}
	}
	if err := registerAuthority(t, db, store, command, writefence.WriterMedia); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Begin(t.Context(), authority, uuid.New(), true); !errors.Is(err, writefence.ErrStale) {
		t.Fatalf("stale CAS accepted: %v", err)
	}
	blocked := completedAuthority(t, store, pending.Control.Authority, true)
	if err := registerAuthority(t, db, store, command, writefence.WriterMedia); !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("blocked authority admitted: %v", err)
	}
	for _, mutate := range []func(*writefence.Control){
		func(c *writefence.Control) { c.Blocked = false }, func(c *writefence.Control) { c.Epoch++ },
	} {
		changed := blocked.Control
		mutate(&changed)
		if _, err := store.Acknowledge(t.Context(), changed, writefence.WriterMedia); !errors.Is(err, writefence.ErrConflict) {
			t.Fatalf("inexact ACK accepted: %v", err)
		}
	}
	if _, err := store.Acknowledge(t.Context(), pending.Control, writefence.WriterExecution); !errors.Is(err, writefence.ErrStale) {
		t.Fatalf("late ACK changed new head: %v", err)
	}
	replayed, err := pgfence.NewAuthorityStore(db).Begin(t.Context(), authority, id, false)
	if err != nil || !replayed.Complete || replayed.Control != pending.Control {
		t.Fatalf("old coordination history lost: %+v %v", replayed, err)
	}
	if err := registerAuthority(t, db, store, command, writefence.WriterMedia); !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("historical open reopened current authority: %v", err)
	}
	opened := completedAuthority(t, store, blocked.Control.Authority, false)
	if opened.Control.Epoch != 3 {
		t.Fatalf("nonmonotonic epoch: %+v", opened)
	}
	if err := registerAuthority(t, db, store, command, writefence.WriterMedia); !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("new open revived old admission: %v", err)
	}
}

func TestAuthorityPostgresAdmissionBindsWholeCommandAndRollsBack(t *testing.T) {
	for _, owner := range []string{"identity", "creation"} {
		t.Run(owner, func(t *testing.T) {
			db := authorityDatabase(t, owner)
			store := pgfence.NewAuthorityStore(db)
			kind := writefence.Account
			if owner == "creation" {
				kind = writefence.Project
			}
			state := completedAuthority(t, store, writefence.Authority{Issuer: writefence.Issuer(owner), Kind: kind, ID: uuid.New()}, false)
			command := authorityCommand(state.Control.Authority)
			foreign := writefence.Authority{Issuer: writefence.Creation, Kind: writefence.Scope, ID: uuid.New(), Epoch: 5}
			if owner == "creation" {
				foreign.Issuer = writefence.Identity
				foreign.Kind = writefence.Session
			}
			command.Authorities = append(command.Authorities, foreign)
			if err := store.Register(t.Context(), db, command, writefence.WriterMedia); !errors.Is(err, writefence.ErrTransaction) {
				t.Fatalf("autocommit admission accepted: %v", err)
			}
			interrupted := errors.New("business transaction rollback")
			err := db.Transaction(func(tx *gorm.DB) error {
				if err := store.Register(t.Context(), tx, command, writefence.WriterMedia); err != nil {
					return err
				}
				if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES (?,'authority.rollback',?,'{}')`, command.ID, command.ID.String()).Error; err != nil {
					return err
				}
				return interrupted
			})
			if !errors.Is(err, interrupted) {
				t.Fatal(err)
			}
			var count int64
			if err := db.Table("infra.authority_admission").Where("id=?", command.ID).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("rollback retained ledger: %d %v", count, err)
			}
			if err := db.Table("infra.outbox").Where("id=?", command.ID).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("rollback retained effect: %d %v", count, err)
			}
			if err := registerAuthority(t, db, store, command, writefence.WriterMedia); err != nil {
				t.Fatal(err)
			}
			if err := registerAuthority(t, db, pgfence.NewAuthorityStore(db), command, writefence.WriterMedia); err != nil {
				t.Fatalf("lost admission ACK could not replay: %v", err)
			}
			for _, mutate := range []func(*writefence.Command){
				func(c *writefence.Command) { c.SHA256 = strings.Repeat("b", 64) },
				func(c *writefence.Command) { c.Action = "media.upload.replace" },
				func(c *writefence.Command) { c.Authorities[1].Epoch++ },
				func(c *writefence.Command) { c.Authorities[1].ID = uuid.New() },
			} {
				changed := command
				changed.Authorities = append([]writefence.Authority(nil), command.Authorities...)
				mutate(&changed)
				if err := registerAuthority(t, db, store, changed, writefence.WriterMedia); !errors.Is(err, writefence.ErrConflict) {
					t.Fatalf("admission ID changed content: %v", err)
				}
			}
			if err := registerAuthority(t, db, store, command, writefence.WriterExecution); !errors.Is(err, writefence.ErrConflict) {
				t.Fatalf("target changed: %v", err)
			}
			if err := registerAuthority(t, db, store, authorityCommand(foreign), writefence.WriterMedia); !errors.Is(err, writefence.ErrInvalid) {
				t.Fatalf("foreign issuer admitted: %v", err)
			}
		})
	}
}

type authorityLocalWriters map[writefence.Writer]*pgfence.Store

func (w authorityLocalWriters) Install(ctx context.Context, writer writefence.Writer, control writefence.Control) error {
	return w[writer].Install(ctx, control)
}

type authorityLostACK struct {
	writefence.AuthorityRepository
	fail bool
}

func (r *authorityLostACK) Acknowledge(ctx context.Context, c writefence.Control, w writefence.Writer) (writefence.Coordination, error) {
	if r.fail && w == writefence.WriterMedia {
		r.fail = false
		return writefence.Coordination{}, writefence.ErrUnavailable
	}
	return r.AuthorityRepository.Acknowledge(ctx, c, w)
}

func TestAuthorityPostgresLostACKRestartAndOldCommandRemainFenced(t *testing.T) {
	db := authorityDatabase(t, "identity")
	store := pgfence.NewAuthorityStore(db)
	writers := make(authorityLocalWriters)
	for _, writer := range writefence.Writers() {
		writers[writer] = pgfence.New(authorityDatabase(t, string(writer)))
	}
	authority := writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: uuid.New()}
	lost := &authorityLostACK{AuthorityRepository: store, fail: true}
	id := uuid.New()
	pending, err := writefence.NewCoordinator(lost, writers).Set(t.Context(), authority, id, false)
	if !errors.Is(err, writefence.ErrUnavailable) || pending.Complete || len(pending.Acknowledged) != 2 {
		t.Fatalf("lost ACK completed: %+v %v", pending, err)
	}
	command := authorityCommand(pending.Control.Authority)
	if err := registerAuthority(t, db, store, command, writefence.WriterMedia); !errors.Is(err, writefence.ErrFenced) {
		t.Fatal(err)
	}
	coordinator := writefence.NewCoordinator(pgfence.NewAuthorityStore(db), writers)
	opened, err := coordinator.Set(t.Context(), authority, id, false)
	if err != nil || !opened.Complete {
		t.Fatalf("restart lost ACK: %+v %v", opened, err)
	}
	if err := registerAuthority(t, db, store, command, writefence.WriterMedia); err != nil {
		t.Fatal(err)
	}
	if _, err := writers[writefence.WriterMedia].Admit(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	blocked, err := coordinator.Set(t.Context(), opened.Control.Authority, uuid.New(), true)
	if err != nil || !blocked.Complete {
		t.Fatal(err)
	}
	if _, err := coordinator.Set(t.Context(), blocked.Control.Authority, uuid.New(), false); err != nil {
		t.Fatal(err)
	}
	mediaDB := authorityDatabase(t, "media")
	err = mediaDB.Transaction(func(tx *gorm.DB) error { return writers[writefence.WriterMedia].Guard(t.Context(), tx, command) })
	if !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("new open resurrected old admitted command: %v", err)
	}
}

func TestAuthorityPostgresAdmissionLockSerializesClosure(t *testing.T) {
	db := authorityDatabase(t, "creation")
	store := pgfence.NewAuthorityStore(db)
	state := completedAuthority(t, store, writefence.Authority{Issuer: writefence.Creation, Kind: writefence.Project, ID: uuid.New()}, false)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tx := db.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	command := authorityCommand(state.Control.Authority)
	if err := store.Register(ctx, tx, command, writefence.WriterExecution); err != nil {
		t.Fatal(err)
	}
	var holder int
	if err := tx.Raw("SELECT pg_backend_pid()").Scan(&holder).Error; err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := store.Begin(ctx, state.Control.Authority, uuid.New(), true)
		done <- err
		close(done)
	}()
	defer func() {
		_ = tx.Rollback().Error
		cancel()
		for range done {
		}
	}()
	admin := fenceDatabase(t, "")
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := admin.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND ?=ANY(pg_blocking_pids(pid)) AND query LIKE '%authority_head%')`, holder).Scan(&waiting).Error; err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("closure passed admission lock: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := registerAuthority(t, db, store, authorityCommand(state.Control.Authority), writefence.WriterExecution); !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("closure-first admitted new command: %v", err)
	}
}

func TestAuthorityPostgresRestrictedOwnersAndAppendOnlyEvidence(t *testing.T) {
	identity := authorityDatabase(t, "identity")
	store := pgfence.NewAuthorityStore(identity)
	state := completedAuthority(t, store, writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: uuid.New()}, false)
	command := authorityCommand(state.Control.Authority)
	if err := registerAuthority(t, identity, store, command, writefence.WriterMedia); err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{"identity", "creation", "media", "execution"} {
		t.Run(owner, func(t *testing.T) {
			db := authorityDatabase(t, owner)
			for _, table := range []string{"authority_coordination", "authority_acknowledgement", "authority_admission"} {
				for _, query := range []string{"DELETE FROM infra." + table, "UPDATE infra." + table + " SET service_owner=service_owner", "TRUNCATE infra." + table} {
					if err := db.Exec(query).Error; err == nil {
						t.Fatalf("evidence mutation allowed: %s", query)
					}
				}
			}
			for _, query := range []string{"UPDATE infra.authority_head SET service_owner='creation'", "DELETE FROM infra.authority_head", "SET ROLE glowloom_app"} {
				if err := db.Exec(query).Error; err == nil {
					t.Fatalf("owner escalation allowed: %s", query)
				}
			}
			if err := db.Exec("SET glowloom.service_owner='identity'").Error; err != nil {
				t.Fatal(err)
			}
			var count int64
			if err := db.Table("infra.authority_admission").Where("id=?", command.ID).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			want := int64(0)
			if owner == "identity" {
				want = 1
			}
			if count != want {
				t.Fatalf("forged GUC exposed another owner's evidence: %d", count)
			}
			foreign := writefence.Authority{Issuer: writefence.Creation, Kind: writefence.Project, ID: uuid.New()}
			if owner == "creation" {
				foreign = writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: uuid.New()}
			}
			if _, err := pgfence.NewAuthorityStore(db).Begin(t.Context(), foreign, uuid.New(), false); !errors.Is(err, writefence.ErrInvalid) {
				t.Fatalf("foreign issuer accepted: %v", err)
			}
			if owner == "media" || owner == "execution" {
				if err := registerAuthority(t, db, pgfence.NewAuthorityStore(db), command, writefence.WriterMedia); !errors.Is(err, writefence.ErrInvalid) {
					t.Fatalf("nonissuer owner admitted command: %v", err)
				}
			}
			if err := db.Exec(`INSERT INTO infra.authority_head(service_owner,issuer,authority_kind,authority_id,epoch,coordination_id) VALUES ('identity','identity','account',?,1,?)`, uuid.New(), uuid.New()).Error; owner != "identity" && err == nil {
				t.Fatal("forged SQL owner inserted authority")
			}
			var force bool
			if err := db.Raw(`SELECT bool_and(relrowsecurity AND relforcerowsecurity) FROM pg_class WHERE oid IN ('infra.authority_head'::regclass,'infra.authority_coordination'::regclass,'infra.authority_acknowledgement'::regclass,'infra.authority_admission'::regclass)`).Scan(&force).Error; err != nil || !force {
				t.Fatalf("FORCE RLS absent: %t %v", force, err)
			}
		})
	}
}
