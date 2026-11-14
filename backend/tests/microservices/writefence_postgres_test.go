package microservices_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func fenceDatabase(t *testing.T, owner string) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_WRITE_FENCE_DB_DSN")
	if dsn == "" {
		t.Skip("requires GLOWLOOM_TEST_WRITE_FENCE_DB_DSN for an isolated loopback *_test database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" ||
		(parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost" && parsed.Hostname() != "::1") ||
		!strings.HasSuffix(strings.TrimPrefix(parsed.Path, "/"), "_test") || parsed.Port() == "" || parsed.RawQuery == "" {
		t.Fatal("write fence tests require an explicit isolated loopback *_test PostgreSQL URL")
	}
	if owner != "" {
		switch owner {
		case "media", "execution", "creation", "identity":
		default:
			t.Fatal("unknown test owner")
		}
		query := parsed.Query()
		query.Set("options", "-c role=glowloom_"+owner)
		parsed.RawQuery = query.Encode()
	}
	database, err := gorm.Open(postgres.Open(parsed.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("open isolated write fence database:", err)
	}
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	})
	if owner != "" {
		var actual string
		if err := database.WithContext(t.Context()).Raw("SELECT infra.runtime_owner()").Scan(&actual).Error; err != nil || actual != owner {
			t.Fatalf("test connection owner = %q, error = %v", actual, err)
		}
	}
	return database
}

func admittedCommand(t *testing.T, database *gorm.DB) (*pgfence.Store, writefence.Command) {
	t.Helper()
	store := pgfence.New(database)
	command := fenceCommand()
	for _, authority := range command.Authorities {
		if err := store.Install(t.Context(), writefence.Control{Authority: authority, CoordinationID: uuid.New()}); err != nil {
			t.Fatal(err)
		}
	}
	if saved, err := store.Admit(t.Context(), command); err != nil || saved.State != writefence.Admitted {
		t.Fatalf("admit command: state=%s, err=%v", saved.State, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := database.WithContext(ctx).Exec("DELETE FROM infra.writer_command WHERE id = ?", command.ID).Error; err != nil {
			t.Error(err)
		}
		for _, authority := range command.Authorities {
			if err := database.WithContext(ctx).Exec("DELETE FROM infra.writer_fence WHERE issuer = ? AND authority_kind = ? AND authority_id = ?", authority.Issuer, authority.Kind, authority.ID).Error; err != nil {
				t.Error(err)
			}
		}
	})
	return store, command
}

func TestWriteFencePostgresBlocksRevocationUntilLocalEffectCommits(t *testing.T) {
	database := fenceDatabase(t, "media")
	admin := fenceDatabase(t, "")
	store, command := admittedCommand(t, database)
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancel()
	tx := database.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	if err := store.Guard(ctx, tx, command); err != nil {
		t.Fatal(err)
	}
	var holder int
	if err := tx.Raw("SELECT pg_backend_pid()").Scan(&holder).Error; err != nil {
		t.Fatal(err)
	}
	control := writefence.Control{Authority: command.Authorities[0], CoordinationID: uuid.New(), Blocked: true}
	control.Epoch++
	done := make(chan error, 1)
	go func() {
		defer close(done)
		done <- store.Install(ctx, control)
	}()
	defer func() {
		_ = tx.Rollback().Error
		cancel()
		for range done {
		}
	}()
	// Actual database blocking, not elapsed sleep or an in-memory lock, is the
	// evidence that the final writer transaction and revocation serialize.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	blocked := false
	for !blocked {
		if err := admin.WithContext(ctx).Raw(`SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity WHERE datname = current_database()
			AND ? = ANY(pg_blocking_pids(pid)) AND query LIKE '%writer_fence%'
		)`, holder).Scan(&blocked).Error; err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("revocation returned before the final transaction: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
	if err := store.Complete(ctx, tx, command, json.RawMessage(`{"outcome":"published","sequence":9007199254740993}`)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	restarted := pgfence.New(database)
	saved, err := restarted.Load(ctx, command.ID)
	if err != nil || saved.State != writefence.Committed || !strings.Contains(string(saved.Receipt), "9007199254740993") {
		t.Fatalf("committed receipt lost after revocation: %+v, %v", saved, err)
	}
	replayed, err := restarted.Admit(ctx, command)
	if err != nil || replayed.State != writefence.Committed {
		t.Fatalf("lost-ACK replay did not resolve original committed command: %+v, %v", replayed, err)
	}
	err = database.WithContext(ctx).Transaction(func(replayTX *gorm.DB) error {
		return restarted.Guard(ctx, replayTX, command)
	})
	if !errors.Is(err, writefence.ErrCommitted) {
		t.Fatalf("committed command could run effects twice: %v", err)
	}
	err = database.WithContext(ctx).Transaction(func(replayTX *gorm.DB) error {
		return restarted.Complete(ctx, replayTX, command, json.RawMessage(`{"sequence":9007199254740993,"outcome":"published"}`))
	})
	if err != nil {
		t.Fatalf("exact committed receipt could not be acknowledged again: %v", err)
	}
	err = database.WithContext(ctx).Transaction(func(replayTX *gorm.DB) error {
		return restarted.Complete(ctx, replayTX, command, json.RawMessage(`{"outcome":"different"}`))
	})
	if !errors.Is(err, writefence.ErrConflict) {
		t.Fatalf("replay replaced original committed receipt: %v", err)
	}
}

func TestWriteFencePostgresRejectsRevocationFirstAndRetainsUnknown(t *testing.T) {
	database := fenceDatabase(t, "media")
	store, command := admittedCommand(t, database)
	control := writefence.Control{Authority: command.Authorities[0], CoordinationID: uuid.New(), Blocked: true}
	control.Epoch++
	if err := store.Install(t.Context(), control); err != nil {
		t.Fatal(err)
	}
	err := database.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		return store.Guard(t.Context(), tx, command)
	})
	if !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("old authority crossed committed fence: %v", err)
	}
	saved, err := pgfence.New(database).Load(t.Context(), command.ID)
	if err != nil || saved.State != writefence.Admitted || len(saved.Receipt) != 0 {
		t.Fatalf("revocation incorrectly discarded unknown command: %+v, %v", saved, err)
	}
	frozen, err := store.Freeze(t.Context(), command.ID)
	if err != nil || frozen.State != writefence.Frozen {
		t.Fatalf("freeze original responsibility: %+v, %v", frozen, err)
	}
	control.Epoch++
	control.Blocked = false
	control.CoordinationID = uuid.New()
	if err := store.Install(t.Context(), control); err != nil {
		t.Fatal(err)
	}
	err = database.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error { return store.Guard(t.Context(), tx, command) })
	if !errors.Is(err, writefence.ErrFrozen) {
		t.Fatalf("reopened account resurrected frozen command: %v", err)
	}
	stale := control
	stale.Epoch--
	if err := store.Install(t.Context(), stale); !errors.Is(err, writefence.ErrStale) {
		t.Fatalf("late old control accepted: %v", err)
	}
}

func TestWriteFencePostgresEffectAndReceiptRollbackTogether(t *testing.T) {
	database := fenceDatabase(t, "execution")
	store, command := admittedCommand(t, database)
	event := uuid.New()
	interrupted := errors.New("simulated process exit before commit")
	err := database.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		if err := store.Guard(t.Context(), tx, command); err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload)
			VALUES (?, 'microservices.writefence.test', ?, '{}')`, event, command.ID.String()).Error; err != nil {
			return err
		}
		if err := store.Complete(t.Context(), tx, command, json.RawMessage(`{"outcome":"ready"}`)); err != nil {
			return err
		}
		return interrupted
	})
	if !errors.Is(err, interrupted) {
		t.Fatal(err)
	}
	var count int64
	if err := database.Table("infra.outbox").WithContext(t.Context()).Where("id = ?", event).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rolled-back outbox effect exists: count=%d, err=%v", count, err)
	}
	loaded, err := store.Load(t.Context(), command.ID)
	if err != nil || loaded.State != writefence.Admitted || len(loaded.Receipt) != 0 {
		t.Fatalf("rolled-back receipt became committed: %+v, %v", loaded, err)
	}
}

func TestWriteFencePostgresRejectsNumericExpansionBeforeEffectCommit(t *testing.T) {
	database := fenceDatabase(t, "media")
	store, command := admittedCommand(t, database)
	event := uuid.New()
	err := database.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		if err := store.Guard(t.Context(), tx, command); err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload)
			VALUES (?, 'microservices.writefence.test', ?, '{}')`, event, command.ID.String()).Error; err != nil {
			return err
		}
		return store.Complete(t.Context(), tx, command, []byte(`{"values":[`+strings.Repeat(`1e100000,`, 11)+`1]}`))
	})
	if !errors.Is(err, writefence.ErrInvalid) {
		t.Fatalf("expanding numeric receipt committed: %v", err)
	}
	var count int64
	if err := database.WithContext(t.Context()).Table("infra.outbox").Where("id = ?", event).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("invalid receipt left an effect behind: %d, %v", count, err)
	}
	if loaded, err := store.Load(t.Context(), command.ID); err != nil || loaded.State != writefence.Admitted {
		t.Fatalf("invalid numeric result became committed: %+v, %v", loaded, err)
	}
	if err := database.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		return store.Complete(t.Context(), tx, command, []byte(`{"amount":1.25e3,"tiny":1e-4}`))
	}); err != nil {
		t.Fatal(err)
	}
	if loaded, err := pgfence.New(database).Load(t.Context(), command.ID); err != nil || string(loaded.Receipt) != `{"amount":1250,"tiny":0.0001}` {
		t.Fatalf("numeric result did not survive JSONB round-trip: %s, %v", loaded.Receipt, err)
	}
}

func TestWriteFencePostgresStoresBoundedLargeReceipt(t *testing.T) {
	database := fenceDatabase(t, "media")
	store, command := admittedCommand(t, database)
	object := make(map[string]int, 90000)
	for i := range 90000 {
		object[fmt.Sprintf("k%05d", i)] = 0
	}
	receipt, err := json.Marshal(object)
	if err != nil || len(receipt) > writefence.MaxReceiptBytes {
		t.Fatalf("invalid large receipt fixture: %d bytes, %v", len(receipt), err)
	}
	err = database.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		return store.Complete(t.Context(), tx, command, receipt)
	})
	if err != nil {
		t.Fatalf("JSONB whitespace expansion rejected a bounded canonical receipt: %v", err)
	}
	loaded, err := pgfence.New(database).Load(t.Context(), command.ID)
	if err != nil || loaded.State != writefence.Committed || len(loaded.Receipt) > writefence.MaxReceiptBytes {
		t.Fatalf("bounded receipt could not be recovered after JSONB expansion: %d bytes, %v", len(loaded.Receipt), err)
	}
}

func TestWriteFencePostgresSeparatesOwnersAndConflictingIdentities(t *testing.T) {
	media := fenceDatabase(t, "media")
	store, command := admittedCommand(t, media)
	execution := pgfence.New(fenceDatabase(t, "execution"))
	if _, err := execution.Load(t.Context(), command.ID); !errors.Is(err, writefence.ErrNotFound) {
		t.Fatalf("execution could read media command: %v", err)
	}
	if _, err := execution.Admit(t.Context(), command); !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("grant bootstrapped unknown owner authority: %v", err)
	}
	changed := command
	changed.SHA256 = strings.Repeat("b", 64)
	if _, err := store.Admit(t.Context(), changed); !errors.Is(err, writefence.ErrConflict) {
		t.Fatalf("same identity accepted different input: %v", err)
	}
	if err := store.Guard(t.Context(), media, command); !errors.Is(err, writefence.ErrTransaction) {
		t.Fatalf("non-transactional final write accepted: %v", err)
	}
}
