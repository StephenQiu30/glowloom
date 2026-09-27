package billing_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/lanverse/backend/internal/billing/application"
	"github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
)

func TestChangeBudgetCommitsSignedLedgerAndBothEvents(t *testing.T) {
	_, database := billingDB(t)
	actor, projectID := billingProject(t, database)
	budgetID := uuid.New()
	if err := database.Exec(`
		INSERT INTO billing.budget (id, project_id, limit_micros)
		VALUES (?::uuid, ?::uuid, 400)
	`, budgetID.String(), projectID.String()).Error; err != nil {
		t.Fatalf("create budget: %v", err)
	}
	command := billingapp.NewChangeBudgetCommand(pgbilling.NewStore(database), time.Now)
	first, err := command.Execute(t.Context(), actor, billingapp.ChangeBudgetInput{
		ProjectID: projectID, LimitMicros: 500, ExpectedRevision: 1, RequestID: uuid.NewString(),
	})
	if err != nil || first.LimitMicros != 500 || first.Revision != 2 || first.IsOverrun {
		t.Fatalf("first budget change = %+v: %v", first, err)
	}
	second, err := command.Execute(t.Context(), actor, billingapp.ChangeBudgetInput{
		ProjectID: projectID, LimitMicros: 350, ExpectedRevision: 2, RequestID: uuid.NewString(),
	})
	if err != nil || second.LimitMicros != 350 || second.Revision != 3 {
		t.Fatalf("second budget change = %+v: %v", second, err)
	}
	var entries []struct {
		AmountMicros int64
		EntryType    string
		CreateBy     uuid.UUID
	}
	if err := database.Raw(`
		SELECT amount_micros, entry_type, create_by
		FROM billing.ledger_entry
		WHERE project_id = ?::uuid ORDER BY create_time, id
	`, projectID.String()).Scan(&entries).Error; err != nil {
		t.Fatalf("read budget ledger: %v", err)
	}
	if len(entries) != 2 || entries[0].AmountMicros != 100 || entries[1].AmountMicros != -150 ||
		entries[0].EntryType != "budget_change" || entries[1].EntryType != "budget_change" ||
		entries[0].CreateBy != actor.ID || entries[1].CreateBy != actor.ID {
		t.Fatalf("budget ledger = %+v", entries)
	}
	var events []struct {
		Topic        string
		PartitionKey string
		Payload      []byte
	}
	if err := database.Raw(`
		SELECT topic, partition_key, payload FROM infra.outbox
		WHERE partition_key = ? ORDER BY create_time, id
	`, projectID.String()).Scan(&events).Error; err != nil {
		t.Fatalf("read budget events: %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("budget events = %d, want 4", len(events))
	}
	var changed, audited int
	for _, event := range events {
		if event.PartitionKey != projectID.String() {
			t.Fatalf("wrong event partition = %q", event.PartitionKey)
		}
		switch event.Topic {
		case "lanverse.billing.budget_changed.v1":
			changed++
		case "lanverse.audit.recorded.v1":
			audited++
		default:
			t.Fatalf("unexpected budget topic = %q", event.Topic)
		}
	}
	if changed != 2 || audited != 2 {
		t.Fatalf("budget event topics = changed %d, audit %d", changed, audited)
	}
	if _, err := command.Execute(t.Context(), actor, billingapp.ChangeBudgetInput{
		ProjectID: projectID, LimitMicros: 600, ExpectedRevision: 2, RequestID: uuid.NewString(),
	}); !errors.Is(err, domain.ErrBudgetRevision) {
		t.Fatalf("stale revision error = %v", err)
	}
	unchanged, err := command.Execute(t.Context(), actor, billingapp.ChangeBudgetInput{
		ProjectID: projectID, LimitMicros: 350, ExpectedRevision: 3, RequestID: uuid.NewString(),
	})
	if err != nil || unchanged.Revision != 3 || unchanged.LimitMicros != 350 {
		t.Fatalf("no-op budget change = %+v: %v", unchanged, err)
	}
	var entryCount, eventCount int64
	if err := database.Raw(`SELECT count(*) FROM billing.ledger_entry WHERE project_id = ?::uuid`, projectID.String()).Scan(&entryCount).Error; err != nil {
		t.Fatalf("count ledger entries: %v", err)
	}
	if err := database.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key = ?`, projectID.String()).Scan(&eventCount).Error; err != nil {
		t.Fatalf("count budget events: %v", err)
	}
	if entryCount != 2 || eventCount != 4 {
		t.Fatalf("no-op wrote ledger/outbox: %d/%d", entryCount, eventCount)
	}
}

func TestChangeBudgetRollsBackWhenOutboxWriteFails(t *testing.T) {
	_, database := billingDB(t)
	actor, projectID := billingProject(t, database)
	budgetID := uuid.New()
	if err := database.Exec(`
		INSERT INTO billing.budget (id, project_id, limit_micros)
		VALUES (?::uuid, ?::uuid, 400)
	`, budgetID.String(), projectID.String()).Error; err != nil {
		t.Fatalf("create budget: %v", err)
	}
	constraint := fmt.Sprintf(`
		ALTER TABLE infra.outbox ADD CONSTRAINT ck_billing_test_reject_change
		CHECK (topic <> 'lanverse.billing.budget_changed.v1' OR partition_key <> '%s')
	`, projectID.String())
	if err := database.Exec(constraint).Error; err != nil {
		t.Fatalf("install outbox failure constraint: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		if err := database.WithContext(ctx).Exec(`ALTER TABLE infra.outbox DROP CONSTRAINT ck_billing_test_reject_change`).Error; err != nil {
			t.Errorf("remove outbox failure constraint: %v", err)
		}
	})
	_, err := billingapp.NewChangeBudgetCommand(pgbilling.NewStore(database), time.Now).Execute(t.Context(), actor, billingapp.ChangeBudgetInput{
		ProjectID: projectID, LimitMicros: 500, ExpectedRevision: 1, RequestID: uuid.NewString(),
	})
	if err == nil {
		t.Fatal("outbox failure accepted budget change")
	}
	var row struct {
		LimitMicros int64
		Revision    int64
	}
	if err := database.Raw(`SELECT limit_micros, revision FROM billing.budget WHERE id = ?::uuid`, budgetID.String()).Scan(&row).Error; err != nil {
		t.Fatalf("read budget after rollback: %v", err)
	}
	var ledgerCount, eventCount int64
	if err := database.Raw(`SELECT count(*) FROM billing.ledger_entry WHERE project_id = ?::uuid`, projectID.String()).Scan(&ledgerCount).Error; err != nil {
		t.Fatalf("count ledger after rollback: %v", err)
	}
	if err := database.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key = ?`, projectID.String()).Scan(&eventCount).Error; err != nil {
		t.Fatalf("count events after rollback: %v", err)
	}
	if row.LimitMicros != 400 || row.Revision != 1 || ledgerCount != 0 || eventCount != 0 {
		t.Fatalf("partial budget transaction: row=%+v ledger=%d events=%d", row, ledgerCount, eventCount)
	}
}

func TestChangeBudgetConcurrentRevisionAllowsOneWinner(t *testing.T) {
	_, database := billingDB(t)
	actor, projectID := billingProject(t, database)
	budgetID := uuid.New()
	if err := database.Exec(`
		INSERT INTO billing.budget (id, project_id, limit_micros)
		VALUES (?::uuid, ?::uuid, 400)
	`, budgetID.String(), projectID.String()).Error; err != nil {
		t.Fatalf("create budget: %v", err)
	}
	command := billingapp.NewChangeBudgetCommand(pgbilling.NewStore(database), time.Now)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, amount := range []int64{500, 600} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := command.Execute(t.Context(), actor, billingapp.ChangeBudgetInput{
				ProjectID: projectID, LimitMicros: amount, ExpectedRevision: 1, RequestID: uuid.NewString(),
			})
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var succeeded, conflicted int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, domain.ErrBudgetRevision):
			conflicted++
		default:
			t.Fatalf("concurrent change returned %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent results = success %d conflict %d", succeeded, conflicted)
	}
	var row struct {
		LimitMicros int64
		Revision    int64
	}
	if err := database.Raw(`SELECT limit_micros, revision FROM billing.budget WHERE id = ?::uuid`, budgetID.String()).Scan(&row).Error; err != nil {
		t.Fatalf("read concurrent budget: %v", err)
	}
	var ledgerCount, eventCount int64
	if err := database.Raw(`SELECT count(*) FROM billing.ledger_entry WHERE project_id = ?::uuid`, projectID.String()).Scan(&ledgerCount).Error; err != nil {
		t.Fatalf("count concurrent ledger: %v", err)
	}
	if err := database.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key = ?`, projectID.String()).Scan(&eventCount).Error; err != nil {
		t.Fatalf("count concurrent events: %v", err)
	}
	if (row.LimitMicros != 500 && row.LimitMicros != 600) || row.Revision != 2 || ledgerCount != 1 || eventCount != 2 {
		t.Fatalf("concurrent budget state = %+v ledger=%d events=%d", row, ledgerCount, eventCount)
	}
}

func TestChangeBudgetRejectsArchivedProject(t *testing.T) {
	_, database := billingDB(t)
	actor, projectID := billingProject(t, database)
	if err := database.Exec(`
		INSERT INTO billing.budget (id, project_id, limit_micros)
		VALUES (?::uuid, ?::uuid, 400)
	`, uuid.NewString(), projectID.String()).Error; err != nil {
		t.Fatalf("create budget: %v", err)
	}
	if err := database.Exec(`
		UPDATE workspace.project SET status = 'archived', archived_at = now()
		WHERE id = ?::uuid
	`, projectID.String()).Error; err != nil {
		t.Fatalf("archive project: %v", err)
	}
	_, err := billingapp.NewChangeBudgetCommand(pgbilling.NewStore(database), time.Now).Execute(t.Context(), actor, billingapp.ChangeBudgetInput{
		ProjectID: projectID, LimitMicros: 500, ExpectedRevision: 1, RequestID: uuid.NewString(),
	})
	if !errors.Is(err, pgbilling.ErrProjectNotWritable) {
		t.Fatalf("archived project change error = %v", err)
	}
	var ledgerCount, eventCount int64
	if err := database.Raw(`SELECT count(*) FROM billing.ledger_entry WHERE project_id = ?::uuid`, projectID.String()).Scan(&ledgerCount).Error; err != nil {
		t.Fatalf("count archived project ledger: %v", err)
	}
	if err := database.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key = ?`, projectID.String()).Scan(&eventCount).Error; err != nil {
		t.Fatalf("count archived project events: %v", err)
	}
	if ledgerCount != 0 || eventCount != 0 {
		t.Fatalf("archived project wrote ledger/outbox: %d/%d", ledgerCount, eventCount)
	}
}

func TestChangeBudgetPersistsLowEventOnThresholdCrossing(t *testing.T) {
	_, database := billingDB(t)
	actor, projectID := billingProject(t, database)
	budgetID := uuid.New()
	if err := database.Exec(`
		INSERT INTO billing.budget (id, project_id, limit_micros, settled_micros)
		VALUES (?::uuid, ?::uuid, 500, 400)
	`, budgetID.String(), projectID.String()).Error; err != nil {
		t.Fatalf("create budget at 20 percent: %v", err)
	}
	command := billingapp.NewChangeBudgetCommand(pgbilling.NewStore(database), time.Now)
	changed, err := command.Execute(t.Context(), actor, billingapp.ChangeBudgetInput{
		ProjectID: projectID, LimitMicros: 499, ExpectedRevision: 1, RequestID: uuid.NewString(),
	})
	if err != nil || changed.LimitMicros != 499 || changed.Revision != 2 {
		t.Fatalf("cross threshold: %+v, %v", changed, err)
	}
	var events []struct {
		ID      uuid.UUID
		Topic   string
		Payload []byte
	}
	if err := database.Raw(`
		SELECT id, topic, payload FROM infra.outbox WHERE partition_key = ?
	`, projectID.String()).Scan(&events).Error; err != nil {
		t.Fatalf("read budget events: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("threshold crossing events = %+v, want three", events)
	}
	var lowCount int
	for _, event := range events {
		if event.Topic == "lanverse.billing.budget_low.v1" {
			lowCount++
			if event.ID == uuid.Nil || !containsJSONEventID(event.Payload, event.ID) {
				t.Fatalf("invalid low event = %+v", event)
			}
		}
	}
	if lowCount != 1 {
		t.Fatalf("low event count = %d", lowCount)
	}
	var ledgerCount int64
	if err := database.Raw(`
		SELECT count(*) FROM billing.ledger_entry
		WHERE project_id = ?::uuid AND entry_type = 'budget_change' AND amount_micros = -1
	`, projectID.String()).Scan(&ledgerCount).Error; err != nil || ledgerCount != 1 {
		t.Fatalf("signed low-threshold ledger = %d, %v", ledgerCount, err)
	}
}

func TestChangeBudgetRollsBackWhenLowEventFails(t *testing.T) {
	_, database := billingDB(t)
	actor, projectID := billingProject(t, database)
	budgetID := uuid.New()
	if err := database.Exec(`
		INSERT INTO billing.budget (id, project_id, limit_micros, settled_micros)
		VALUES (?::uuid, ?::uuid, 500, 400)
	`, budgetID.String(), projectID.String()).Error; err != nil {
		t.Fatalf("create budget: %v", err)
	}
	constraint := fmt.Sprintf(`
		ALTER TABLE infra.outbox ADD CONSTRAINT ck_billing_test_reject_low
		CHECK (topic <> 'lanverse.billing.budget_low.v1' OR partition_key <> '%s')
	`, projectID.String())
	if err := database.Exec(constraint).Error; err != nil {
		t.Fatalf("install low event failure constraint: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		if err := database.WithContext(ctx).Exec(`ALTER TABLE infra.outbox DROP CONSTRAINT ck_billing_test_reject_low`).Error; err != nil {
			t.Errorf("remove low event failure constraint: %v", err)
		}
	})
	_, err := billingapp.NewChangeBudgetCommand(pgbilling.NewStore(database), time.Now).Execute(t.Context(), actor, billingapp.ChangeBudgetInput{
		ProjectID: projectID, LimitMicros: 499, ExpectedRevision: 1, RequestID: uuid.NewString(),
	})
	if err == nil {
		t.Fatal("low event failure accepted budget change")
	}
	var row struct {
		LimitMicros int64
		Revision    int64
	}
	if err := database.Raw(`SELECT limit_micros, revision FROM billing.budget WHERE id = ?::uuid`, budgetID.String()).Scan(&row).Error; err != nil {
		t.Fatalf("read rolled back budget: %v", err)
	}
	var ledgerCount, eventCount int64
	if err := database.Raw(`SELECT count(*) FROM billing.ledger_entry WHERE project_id = ?::uuid`, projectID.String()).Scan(&ledgerCount).Error; err != nil {
		t.Fatalf("count rolled back ledger: %v", err)
	}
	if err := database.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key = ?`, projectID.String()).Scan(&eventCount).Error; err != nil {
		t.Fatalf("count rolled back events: %v", err)
	}
	if row.LimitMicros != 500 || row.Revision != 1 || ledgerCount != 0 || eventCount != 0 {
		t.Fatalf("partial low-event transaction: row=%+v ledger=%d outbox=%d", row, ledgerCount, eventCount)
	}
}

func containsJSONEventID(payload []byte, id uuid.UUID) bool {
	var envelope struct {
		EventID uuid.UUID `json:"event_id"`
	}
	return json.Unmarshal(payload, &envelope) == nil && envelope.EventID == id
}
