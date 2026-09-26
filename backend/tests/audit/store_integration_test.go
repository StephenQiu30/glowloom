package audit_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	pgaudit "github.com/StephenQiu30/lanverse/backend/internal/audit/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/audit/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestAuditStoreScopesAndPagesRecords(t *testing.T) {
	dsn := os.Getenv("LV_TEST_AUDIT_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_AUDIT_DB_DSN to a disposable database with audit migrations applied")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	tx := conn.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatalf("begin fixture transaction: %v", tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	store := pgaudit.NewStore(tx)

	org := uuid.New()
	otherOrg := uuid.New()
	project := uuid.New()
	otherProject := uuid.New()
	actor := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	rows := []struct {
		id        uuid.UUID
		org       uuid.UUID
		project   uuid.UUID
		actor     uuid.UUID
		action    string
		createdAt time.Time
	}{
		{uuid.New(), org, project, actor, "budget.changed", now.Add(-time.Minute)},
		{uuid.New(), org, otherProject, actor, "project.created", now.Add(-2 * time.Minute)},
		{uuid.New(), org, project, actor, "budget.changed", now.Add(-3 * time.Minute)},
		{uuid.New(), otherOrg, project, actor, "budget.changed", now},
	}
	for _, row := range rows {
		if err := tx.Exec(`
			INSERT INTO audit.audit_log(id, org_id, project_id, actor_id, actor_kind, action, object_type, object_id, after, create_time)
			VALUES (?::uuid, ?::uuid, ?::uuid, ?::uuid, 'user', ?, 'budget', ?, '{"amount":100}', ?)
		`, row.id.String(), row.org.String(), row.project.String(), row.actor.String(), row.action, row.id.String(), row.createdAt).Error; err != nil {
			t.Fatalf("insert audit fixture: %v", err)
		}
	}

	page, err := store.List(ctx, org, domain.Filter{Limit: 2})
	if err != nil || len(page.Items) != 2 || page.Next == nil {
		t.Fatalf("first audit page = %+v, error = %v", page, err)
	}
	if page.Items[0].ID != rows[0].id || page.Items[1].ID != rows[1].id {
		t.Fatalf("first audit page IDs = %s, %s", page.Items[0].ID, page.Items[1].ID)
	}
	var after struct {
		Amount int `json:"amount"`
	}
	if err := json.Unmarshal(page.Items[0].After, &after); err != nil || page.Items[0].OrgID != org || after.Amount != 100 {
		t.Fatalf("first audit record fields = %+v", page.Items[0])
	}
	page2, err := store.List(ctx, org, domain.Filter{Limit: 2, Before: page.Next})
	if err != nil || len(page2.Items) != 1 || page2.Next != nil || page2.Items[0].ID != rows[2].id {
		t.Fatalf("second audit page = %+v, error = %v", page2, err)
	}
	filtered, err := store.List(ctx, org, domain.Filter{
		Limit: 10, ProjectID: &project, ActorID: &actor, Action: "budget.changed",
		From: now.Add(-4 * time.Minute), To: now,
	})
	if err != nil || len(filtered.Items) != 2 || filtered.Items[0].ID != rows[0].id {
		t.Fatalf("filtered audit page = %+v, error = %v", filtered, err)
	}
	if record, err := store.Get(ctx, org, rows[0].id); err != nil || record.OrgID != org || record.ID != rows[0].id {
		t.Fatalf("scoped audit lookup = %+v, error = %v", record, err)
	}
	if _, err := store.Get(ctx, org, rows[3].id); !errors.Is(err, pgaudit.ErrNotFound) {
		t.Fatalf("cross-org audit lookup = %v", err)
	}
	if _, err := store.List(ctx, uuid.Nil, domain.Filter{Limit: 10}); !errors.Is(err, pgaudit.ErrOrgIDRequired) {
		t.Fatalf("missing org audit list = %v", err)
	}
	if _, err := store.List(ctx, org, domain.Filter{Limit: 0}); !errors.Is(err, pgaudit.ErrInvalidLimit) {
		t.Fatalf("invalid audit limit = %v", err)
	}
	if _, err := store.List(ctx, org, domain.Filter{Limit: 10, From: now, To: now}); !errors.Is(err, pgaudit.ErrInvalidRange) {
		t.Fatalf("invalid audit range = %v", err)
	}
	if _, err := store.List(ctx, org, domain.Filter{Limit: 10, Before: &domain.Cursor{ID: uuid.New()}}); !errors.Is(err, pgaudit.ErrInvalidCursor) {
		t.Fatalf("invalid audit cursor = %v", err)
	}

	tieOrg := uuid.New()
	tieTime := now.Add(-time.Hour)
	tieIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	for _, id := range tieIDs {
		if err := tx.Exec(`
			INSERT INTO audit.audit_log(id, org_id, actor_kind, action, object_type, object_id, create_time)
			VALUES (?::uuid, ?::uuid, 'system', 'test.tie', 'test', ?, ?)
		`, id.String(), tieOrg.String(), id.String(), tieTime).Error; err != nil {
			t.Fatalf("insert tied-time audit fixture: %v", err)
		}
	}
	seen := make(map[uuid.UUID]bool, len(tieIDs))
	var cursor *domain.Cursor
	for range tieIDs {
		page, err := store.List(ctx, tieOrg, domain.Filter{Limit: 1, Before: cursor})
		if err != nil || len(page.Items) != 1 || seen[page.Items[0].ID] {
			t.Fatalf("tied-time audit page = %+v, error = %v", page, err)
		}
		seen[page.Items[0].ID] = true
		cursor = page.Next
	}
	if len(seen) != len(tieIDs) || cursor != nil {
		t.Fatalf("tied-time pagination saw %d records, final cursor = %+v", len(seen), cursor)
	}
}
