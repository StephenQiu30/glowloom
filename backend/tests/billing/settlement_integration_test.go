package billing_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/lanverse/backend/internal/billing/application"
)

func settlementFixture(t *testing.T, database *gorm.DB, limit, reserved int64) billingapp.SettleInput {
	t.Helper()
	_, projectID := billingProject(t, database)
	operationID, reservationID := uuid.New(), uuid.New()
	if err := database.Exec(`
		INSERT INTO billing.budget (id, project_id, limit_micros, reserved_micros)
		VALUES (?::uuid, ?::uuid, ?, ?)
	`, uuid.NewString(), projectID.String(), limit, reserved).Error; err != nil {
		t.Fatalf("create budget: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO operation.operation
		  (id, project_id, capability, mode, input_hash, origin, status)
		VALUES (?::uuid, ?::uuid, 'image.generate', 'text_to_image', ?, 'upload', 'confirmed')
	`, operationID.String(), projectID.String(), "settlement-"+operationID.String()).Error; err != nil {
		t.Fatalf("create operation: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO billing.reservation
		  (id, project_id, operation_id, amount_micros, status)
		VALUES (?::uuid, ?::uuid, ?::uuid, ?, 'held')
	`, reservationID.String(), projectID.String(), operationID.String(), reserved).Error; err != nil {
		t.Fatalf("create reservation: %v", err)
	}
	if err := database.Exec(`UPDATE operation.operation SET reservation_id = ?::uuid WHERE id = ?::uuid`, reservationID.String(), operationID.String()).Error; err != nil {
		t.Fatalf("attach reservation: %v", err)
	}
	modelKey, region := "mock.image", "domestic"
	return billingapp.SettleInput{
		ProjectID: projectID, OperationID: operationID,
		Capability: "image.generate", ModelKey: &modelKey, Region: &region,
		OccurredAt: time.Now().UTC(),
	}
}

func settlementRows(t *testing.T, database *gorm.DB, input billingapp.SettleInput) (reserved, settled int64, overrun bool, revision int64, entries map[string]int64, events map[string]int64) {
	t.Helper()
	var budget struct {
		ReservedMicros int64
		SettledMicros  int64
		IsOverrun      bool
		Revision       int64
	}
	if err := database.Raw(`
		SELECT reserved_micros, settled_micros, is_overrun, revision
		FROM billing.budget WHERE project_id = ?::uuid
	`, input.ProjectID.String()).Scan(&budget).Error; err != nil {
		t.Fatalf("read budget: %v", err)
	}
	entries = make(map[string]int64)
	var ledger []struct {
		EntryType    string
		AmountMicros int64
	}
	if err := database.Raw(`SELECT entry_type, amount_micros FROM billing.ledger_entry WHERE project_id = ?::uuid AND operation_id = ?::uuid`, input.ProjectID.String(), input.OperationID.String()).Scan(&ledger).Error; err != nil {
		t.Fatalf("read settlement ledger: %v", err)
	}
	for _, entry := range ledger {
		entries[entry.EntryType] += entry.AmountMicros
	}
	events = make(map[string]int64)
	var outbox []struct{ Topic string }
	if err := database.Raw(`
		SELECT topic FROM infra.outbox
		WHERE partition_key = ? AND
		  (payload -> 'aggregate' ->> 'id' = ? OR payload -> 'data' ->> 'operation_id' = ?)
	`, input.ProjectID.String(), input.OperationID.String(), input.OperationID.String()).Scan(&outbox).Error; err != nil {
		t.Fatalf("read settlement outbox: %v", err)
	}
	for _, event := range outbox {
		events[event.Topic]++
	}
	return budget.ReservedMicros, budget.SettledMicros, budget.IsOverrun, budget.Revision, entries, events
}

func TestSettleInTransactionRecordsFactsAndReplaysOnce(t *testing.T) {
	for _, tc := range []struct {
		name, reservationStatus          string
		limit, reserved, actual          int64
		cap, overrun                     bool
		charge, release, providerOverage int64
	}{
		{"unused reservation", "settled", 100, 80, 50, false, false, 50, 30, 0},
		{"token cost capped", "settled", 100, 80, 120, true, false, 80, 0, 40},
		{"non token overrun", "settled", 100, 80, 130, false, true, 130, 0, 0},
		{"zero charge", "released", 100, 80, 0, false, false, 0, 80, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, database := billingDB(t)
			input := settlementFixture(t, database, tc.limit, tc.reserved)
			input.ActualCostMicros, input.CapAtReservation = tc.actual, tc.cap
			store := pgbilling.NewStore(database)
			for attempt := 0; attempt < 2; attempt++ {
				err := database.Transaction(func(tx *gorm.DB) error {
					result, err := store.SettleInTransaction(t.Context(), tx, input)
					if err != nil {
						return err
					}
					if result.ChargeMicros != tc.charge || result.ReleasedMicros != tc.release ||
						result.ProviderOverageMicros != tc.providerOverage || result.BudgetOverrun != tc.overrun ||
						result.AlreadySettled != (attempt == 1) {
						t.Fatalf("attempt %d result = %+v", attempt, result)
					}
					return nil
				})
				if err != nil {
					t.Fatalf("settle attempt %d: %v", attempt, err)
				}
			}
			var status string
			if err := database.Raw(`SELECT status FROM billing.reservation WHERE operation_id = ?::uuid`, input.OperationID.String()).Scan(&status).Error; err != nil || status != tc.reservationStatus {
				t.Fatalf("reservation status=%q err=%v", status, err)
			}
			reserved, settled, overrun, revision, entries, events := settlementRows(t, database, input)
			if reserved != 0 || settled != tc.charge || overrun != tc.overrun || revision != 2 ||
				entries["settle"] != tc.charge || entries["release"] != tc.release ||
				entries["provider_overage"] != tc.providerOverage ||
				events["lanverse.billing.settled.v1"] != 1 {
				t.Fatalf("budget=%d/%d overrun=%t revision=%d entries=%v events=%v", reserved, settled, overrun, revision, entries, events)
			}
			if tc.overrun && events["lanverse.billing.budget_overrun.v1"] != 1 {
				t.Fatalf("missing overrun event: %v", events)
			}
			input.ActualCostMicros++
			if err := database.Transaction(func(tx *gorm.DB) error {
				_, err := store.SettleInTransaction(t.Context(), tx, input)
				return err
			}); !errors.Is(err, pgbilling.ErrSettlementConflict) {
				t.Fatalf("conflicting replay accepted: %v", err)
			}
			input.ActualCostMicros--
			input.CapAtReservation = !input.CapAtReservation
			if err := database.Transaction(func(tx *gorm.DB) error {
				_, err := store.SettleInTransaction(t.Context(), tx, input)
				return err
			}); !errors.Is(err, pgbilling.ErrSettlementConflict) {
				t.Fatalf("changed charging policy accepted on replay: %v", err)
			}
			input.CapAtReservation = tc.cap
			otherModel := "other.model"
			input.ModelKey = &otherModel
			if err := database.Transaction(func(tx *gorm.DB) error {
				_, err := store.SettleInTransaction(t.Context(), tx, input)
				return err
			}); !errors.Is(err, pgbilling.ErrSettlementConflict) {
				t.Fatalf("changed ledger dimension accepted on replay: %v", err)
			}
		})
	}
}

func TestSettleInTransactionRollsBackWithCaller(t *testing.T) {
	_, database := billingDB(t)
	input := settlementFixture(t, database, 100, 80)
	input.ActualCostMicros = 50
	stop := errors.New("caller rolled back")
	err := database.Transaction(func(tx *gorm.DB) error {
		if _, err := pgbilling.NewStore(database).SettleInTransaction(t.Context(), tx, input); err != nil {
			return err
		}
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatalf("outer rollback error = %v", err)
	}
	reserved, settled, overrun, revision, entries, events := settlementRows(t, database, input)
	if reserved != 80 || settled != 0 || overrun || revision != 1 || len(entries) != 0 || len(events) != 0 {
		t.Fatalf("partial settlement: budget=%d/%d overrun=%t revision=%d entries=%v events=%v", reserved, settled, overrun, revision, entries, events)
	}
}

func TestSettleInTransactionReplaysAfterOutboxPruning(t *testing.T) {
	_, database := billingDB(t)
	input := settlementFixture(t, database, 100, 80)
	input.ActualCostMicros = 50
	store := pgbilling.NewStore(database)
	if err := database.Transaction(func(tx *gorm.DB) error {
		_, err := store.SettleInTransaction(t.Context(), tx, input)
		return err
	}); err != nil {
		t.Fatalf("initial settlement: %v", err)
	}
	var envelope struct {
		OrgID     uuid.UUID `json:"org_id"`
		ProjectID uuid.UUID `json:"project_id"`
		Data      struct {
			ChargeMicros  int64 `json:"charge_micros"`
			OverageMicros int64 `json:"overage_micros"`
		} `json:"data"`
	}
	var row struct{ Payload []byte }
	if err := database.Raw(`
		SELECT payload FROM infra.outbox WHERE topic = 'lanverse.billing.settled.v1'
		  AND partition_key = ? AND payload -> 'aggregate' ->> 'id' = ?
	`, input.ProjectID.String(), input.OperationID.String()).Scan(&row).Error; err != nil {
		t.Fatalf("read settlement event: %v", err)
	}
	if err := json.Unmarshal(row.Payload, &envelope); err != nil || envelope.OrgID == uuid.Nil ||
		envelope.ProjectID != input.ProjectID || envelope.Data.ChargeMicros != 50 || envelope.Data.OverageMicros != 0 {
		t.Fatalf("settlement event=%+v err=%v", envelope, err)
	}
	if err := database.Exec(`
		DELETE FROM infra.outbox WHERE topic = 'lanverse.billing.settled.v1'
		  AND partition_key = ? AND payload -> 'aggregate' ->> 'id' = ?
	`, input.ProjectID.String(), input.OperationID.String()).Error; err != nil {
		t.Fatalf("prune settlement event: %v", err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		result, err := store.SettleInTransaction(t.Context(), tx, input)
		if err == nil && (!result.AlreadySettled || result.ChargeMicros != 50) {
			t.Fatalf("pruned replay result = %+v", result)
		}
		return err
	}); err != nil {
		t.Fatalf("replay after pruning: %v", err)
	}
	_, _, _, revision, entries, events := settlementRows(t, database, input)
	if revision != 2 || entries["settle"] != 50 || entries["release"] != 30 || len(events) != 0 {
		t.Fatalf("pruned replay wrote facts: revision=%d entries=%v events=%v", revision, entries, events)
	}
}

func TestSettleInTransactionUsesApplicationRole(t *testing.T) {
	_, database := billingDB(t)
	input := settlementFixture(t, database, 100, 80)
	input.ActualCostMicros = 50
	err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		_, err := pgbilling.NewStore(database).SettleInTransaction(t.Context(), tx, input)
		return err
	})
	if err != nil {
		t.Fatalf("settle with application role: %v", err)
	}
}

func TestSettleInTransactionConcurrentReplayWritesOnce(t *testing.T) {
	_, database := billingDB(t)
	input := settlementFixture(t, database, 100, 80)
	input.ActualCostMicros = 50
	store := pgbilling.NewStore(database)
	start := make(chan struct{})
	results := make(chan applicationResult, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			var replay bool
			err := database.Transaction(func(tx *gorm.DB) error {
				result, err := store.SettleInTransaction(t.Context(), tx, input)
				replay = result.AlreadySettled
				return err
			})
			results <- applicationResult{replay: replay, err: err}
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	var fresh, replay int
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent settlement: %v", result.err)
		}
		if result.replay {
			replay++
		} else {
			fresh++
		}
	}
	_, settled, _, revision, entries, events := settlementRows(t, database, input)
	if fresh != 1 || replay != 1 || settled != 50 || revision != 2 ||
		entries["settle"] != 50 || entries["release"] != 30 ||
		events["lanverse.billing.settled.v1"] != 1 {
		t.Fatalf("concurrent results fresh=%d replay=%d settled=%d revision=%d entries=%v events=%v", fresh, replay, settled, revision, entries, events)
	}
}

type applicationResult struct {
	replay bool
	err    error
}
