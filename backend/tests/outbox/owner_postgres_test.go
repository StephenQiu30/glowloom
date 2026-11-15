package outbox_test

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

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/outbox"
	pgoutbox "github.com/StephenQiu30/glowloom-server/backend/internal/platform/outbox/postgres"
)

func ownerOutboxDatabase(t *testing.T, owner string) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_WRITE_FENCE_DB_DSN")
	if dsn == "" {
		t.Skip("requires offline four-owner LOGIN fixtures and GLOWLOOM_TEST_WRITE_FENCE_DB_DSN")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") ||
		(parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost" && parsed.Hostname() != "::1") ||
		parsed.Port() == "" || !strings.HasSuffix(parsed.Path, "_test") {
		t.Fatal("owner outbox tests require an explicit isolated loopback *_test PostgreSQL URL")
	}
	parsed.User = url.User("owner_b_" + owner + "_test")
	query := parsed.Query()
	query.Del("options")
	parsed.RawQuery = query.Encode()
	database, err := gorm.Open(postgres.Open(parsed.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
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
	var safe bool
	if err := database.Raw(`SELECT infra.runtime_owner()=? AND NOT (rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls)
		AND NOT pg_has_role(session_user,'glowloom_app','MEMBER') FROM pg_roles WHERE rolname=session_user`, owner).Scan(&safe).Error; err != nil || !safe {
		t.Fatalf("owner connection is not a restricted %s LOGIN: safe=%t error=%v", owner, safe, err)
	}
	return database
}

func insertOwnerOutbox(t *testing.T, database *gorm.DB, id uuid.UUID, created time.Time, owner string) {
	t.Helper()
	if err := database.Exec(`INSERT INTO infra.outbox(id,create_time,next_attempt_at,topic,partition_key,payload,headers)
		VALUES (?,?,?,'fixture.owner_delivery',?,jsonb_build_object('event_id',?::text,'event_type','fixture.owner_delivery'),jsonb_build_object('writer',?::text))`,
		id, created, created, owner, id.String(), owner).Error; err != nil {
		t.Fatal(err)
	}
}

func TestOwnerOutboxNeverPublishesFromCallerTransaction(t *testing.T) {
	for _, owner := range []string{"identity", "creation", "media", "execution"} {
		t.Run(owner, func(t *testing.T) {
			database := ownerOutboxDatabase(t, owner)
			ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
			t.Cleanup(cancel)
			tx := database.WithContext(ctx).Begin()
			if tx.Error != nil {
				t.Fatal(tx.Error)
			}
			t.Cleanup(func() {
				if err := tx.Rollback().Error; err != nil {
					t.Error(err)
				}
			})
			insertOwnerOutbox(t, tx, uuid.New(), time.Now().UTC().Add(-time.Minute), owner)
			publisher := &recordingPublisher{}
			sent, err := outbox.NewRelay(pgoutbox.NewStore(tx), publisher).RunOnce(t.Context())
			if !errors.Is(err, pgoutbox.ErrIndependentTransaction) || sent || len(publisher.events) != 0 {
				t.Fatalf("uncommitted caller transaction reached publisher: sent=%t published=%d error=%v", sent, len(publisher.events), err)
			}
		})
	}
}

func TestOwnerOutboxSameEventIdentityKeepsDeliveryAndReplayIsolated(t *testing.T) {
	id := uuid.New()
	created := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	stores := make(map[string]*pgoutbox.Store)
	events := make(map[string]outbox.Event)
	for _, owner := range []string{"identity", "creation", "media", "execution"} {
		database := ownerOutboxDatabase(t, owner)
		insertOwnerOutbox(t, database, id, created, owner)
		store := pgoutbox.NewStore(database)
		event, found, err := store.ClaimNext(t.Context(), created.Add(time.Second), time.Minute)
		if err != nil || !found || event.ID != id.String() || event.PartitionKey != owner || event.Headers["writer"] != owner {
			t.Fatalf("%s claim escaped its owner: event=%+v found=%t error=%v", owner, event, found, err)
		}
		stores[owner], events[owner] = store, event
	}
	for _, owner := range []string{"identity", "creation", "media", "execution"} {
		t.Run(owner, func(t *testing.T) {
			store, original := stores[owner], events[owner]
			other := "identity"
			if owner == other {
				other = "creation"
			}
			if err := store.Acknowledge(t.Context(), events[other], created.Add(2*time.Second)); !errors.Is(err, pgoutbox.ErrLeaseLost) {
				t.Fatalf("foreign lease acknowledged another owner's same-ID event: %v", err)
			}
			dead, err := store.Retry(t.Context(), original, "broker_timeout", 1, created.Add(3*time.Second), created.Add(2*time.Second))
			if err != nil || !dead {
				t.Fatalf("%s durable dead letter: dead=%t error=%v", owner, dead, err)
			}
			if err := store.ReplayDeadLetter(t.Context(), id, created, uuid.New(), "isolated test operator", created.Add(3*time.Second)); err != nil {
				t.Fatal(err)
			}
			current, found, err := store.ClaimNext(t.Context(), created.Add(4*time.Second), time.Minute)
			if err != nil || !found || current.ClaimToken == original.ClaimToken || current.Headers["writer"] != owner {
				t.Fatalf("%s replay claim: event=%+v found=%t error=%v", owner, current, found, err)
			}
			if err := store.Acknowledge(t.Context(), original, created.Add(5*time.Second)); !errors.Is(err, pgoutbox.ErrLeaseLost) {
				t.Fatalf("old delivery lease survived replay: %v", err)
			}
			if err := store.Acknowledge(t.Context(), current, created.Add(5*time.Second)); err != nil {
				t.Fatal(err)
			}
			if _, found, err := store.ClaimNext(t.Context(), created.Add(6*time.Second), time.Minute); err != nil || found {
				t.Fatalf("%s replay left another visible event: found=%t error=%v", owner, found, err)
			}
		})
	}
}
