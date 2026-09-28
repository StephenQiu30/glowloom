package billing_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/lanverse/backend/internal/billing/application"
	billingdomain "github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
)

func TestReserveInTransactionPreservesBudgetAndLedger(t *testing.T) {
	_, database := billingDB(t)
	actor, projectID := billingProject(t, database)
	seedReserveBudget(t, database, projectID, 100)
	store := pgbilling.NewStore(database)
	first := seedReserveOperation(t, database, projectID, 60)
	second := seedReserveOperation(t, database, projectID, 40)
	for _, tc := range []struct {
		operationID uuid.UUID
		amount      int64
		want        int64
	}{
		{first, 60, 40},
		{second, 40, 0},
	} {
		input := billingapp.ReserveInput{
			ProjectID: projectID, OperationID: tc.operationID, ActorID: actor.ID,
			AmountMicros: tc.amount, OccurredAt: time.Now().UTC(),
		}
		var result billingapp.ReserveResult
		if err := database.Transaction(func(tx *gorm.DB) error {
			var err error
			result, err = store.ReserveInTransaction(t.Context(), tx, input)
			if err != nil {
				return err
			}
			return attachReserveOperation(tx, tc.operationID, projectID, result.ReservationID, actor.ID)
		}); err != nil {
			t.Fatalf("reserve %d: %v", tc.amount, err)
		}
		if result.AvailableMicros != tc.want || result.ReservationID == uuid.Nil {
			t.Fatalf("reserve result = %+v, want available %d", result, tc.want)
		}
		assertReserveFacts(t, database, tc.operationID, tc.amount)
	}
	tooExpensive := seedReserveOperation(t, database, projectID, 1)
	if err := database.Transaction(func(tx *gorm.DB) error {
		_, err := store.ReserveInTransaction(t.Context(), tx, billingapp.ReserveInput{
			ProjectID: projectID, OperationID: tooExpensive,
			ActorID: actor.ID, AmountMicros: 1, OccurredAt: time.Now().UTC(),
		})
		return err
	}); !errors.Is(err, billingdomain.ErrBudgetInsufficient) {
		t.Fatalf("insufficient budget error = %v", err)
	}
	assertReserveBalance(t, database, projectID, 100, 3)
}

func TestReserveInTransactionRollbackAndZeroCost(t *testing.T) {
	_, database := billingDB(t)
	actor, projectID := billingProject(t, database)
	seedReserveBudget(t, database, projectID, 0)
	operationID := seedReserveOperation(t, database, projectID, 0)
	store := pgbilling.NewStore(database)
	input := billingapp.ReserveInput{
		ProjectID: projectID, OperationID: operationID, ActorID: actor.ID,
		AmountMicros: 0, OccurredAt: time.Now().UTC(),
	}
	rollback := errors.New("caller rejects confirmation")
	if err := database.Transaction(func(tx *gorm.DB) error {
		if _, err := store.ReserveInTransaction(t.Context(), tx, input); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("rollback transaction = %v", err)
	}
	assertReserveBalance(t, database, projectID, 0, 1)
	var count int64
	if err := database.Raw(`SELECT count(*) FROM billing.reservation WHERE operation_id = ?::uuid`, operationID.String()).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("rolled back reservation count = %d, %v", count, err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		result, err := store.ReserveInTransaction(t.Context(), tx, input)
		if err != nil {
			return err
		}
		return attachReserveOperation(tx, operationID, projectID, result.ReservationID, actor.ID)
	}); err != nil {
		t.Fatalf("zero-cost reserve as runtime role: %v", err)
	}
	assertReserveFacts(t, database, operationID, 0)
	assertReserveBalance(t, database, projectID, 0, 2)
}

func TestReserveInTransactionSerializesCompetingQuotes(t *testing.T) {
	_, database := billingDB(t)
	actor, projectID := billingProject(t, database)
	seedReserveBudget(t, database, projectID, 100)
	store := pgbilling.NewStore(database)
	operations := []uuid.UUID{
		seedReserveOperation(t, database, projectID, 60),
		seedReserveOperation(t, database, projectID, 60),
	}
	var wg sync.WaitGroup
	results := make([]error, len(operations))
	for index, operationID := range operations {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[index] = database.Transaction(func(tx *gorm.DB) error {
				result, err := store.ReserveInTransaction(t.Context(), tx, billingapp.ReserveInput{
					ProjectID: projectID, OperationID: operationID, ActorID: actor.ID,
					AmountMicros: 60, OccurredAt: time.Now().UTC(),
				})
				if err != nil {
					return err
				}
				return attachReserveOperation(tx, operationID, projectID, result.ReservationID, actor.ID)
			})
		}()
	}
	wg.Wait()
	var success, insufficient int
	for _, err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, billingdomain.ErrBudgetInsufficient):
			insufficient++
		default:
			t.Fatalf("competing confirmation error = %v", err)
		}
	}
	if success != 1 || insufficient != 1 {
		t.Fatalf("competing confirmations: success=%d, insufficient=%d", success, insufficient)
	}
	assertReserveBalance(t, database, projectID, 60, 2)
}

func TestReserveInTransactionDoesNotDoubleHoldAnOperation(t *testing.T) {
	_, database := billingDB(t)
	actor, projectID := billingProject(t, database)
	seedReserveBudget(t, database, projectID, 120)
	operationID := seedReserveOperation(t, database, projectID, 60)
	store := pgbilling.NewStore(database)
	input := billingapp.ReserveInput{
		ProjectID: projectID, OperationID: operationID, ActorID: actor.ID,
		AmountMicros: 60, OccurredAt: time.Now().UTC(),
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		result, err := store.ReserveInTransaction(t.Context(), tx, input)
		if err != nil {
			return err
		}
		return attachReserveOperation(tx, operationID, projectID, result.ReservationID, actor.ID)
	}); err != nil {
		t.Fatalf("first reservation: %v", err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		_, err := store.ReserveInTransaction(t.Context(), tx, input)
		return err
	}); err == nil {
		t.Fatal("duplicate reservation unexpectedly succeeded")
	}
	assertReserveBalance(t, database, projectID, 60, 2)
	assertReserveFacts(t, database, operationID, 60)
}

func seedReserveBudget(t *testing.T, database *gorm.DB, projectID uuid.UUID, limit int64) {
	t.Helper()
	if err := database.Exec(`INSERT INTO billing.budget (id, project_id, limit_micros) VALUES (?::uuid, ?::uuid, ?)`, uuid.NewString(), projectID.String(), limit).Error; err != nil {
		t.Fatalf("create budget: %v", err)
	}
}

func seedReserveOperation(t *testing.T, database *gorm.DB, projectID uuid.UUID, amount int64) uuid.UUID {
	t.Helper()
	operationID := uuid.New()
	if err := database.Exec(`
		INSERT INTO operation.operation (id, project_id, capability, mode, input_hash, origin, status,
		  quote_micros, quote_expires_at, model_profile_version_id, price_rule_version_id, region)
		VALUES (?::uuid, ?::uuid, 'image.generate', 'text_to_image', ?, 'pipeline', 'quoted',
		  ?, now() + interval '15 minutes', ?::uuid, ?::uuid, 'domestic')
	`, operationID.String(), projectID.String(), "reserve-"+operationID.String(), amount, uuid.NewString(), uuid.NewString()).Error; err != nil {
		t.Fatalf("create quoted operation: %v", err)
	}
	return operationID
}

func attachReserveOperation(tx *gorm.DB, operationID, projectID, reservationID, actorID uuid.UUID) error {
	result := tx.Exec(`
		UPDATE operation.operation
		SET reservation_id = ?::uuid, status = 'confirmed', confirmed_at = now(), confirmed_by = ?::uuid
		WHERE id = ?::uuid AND project_id = ?::uuid AND status = 'quoted'
	`, reservationID.String(), actorID.String(), operationID.String(), projectID.String())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("operation was not quoted")
	}
	return nil
}

func assertReserveFacts(t *testing.T, database *gorm.DB, operationID uuid.UUID, amount int64) {
	t.Helper()
	var facts []struct {
		AmountMicros int64
		EntryType    string
	}
	if err := database.Raw(`
		SELECT r.amount_micros, l.entry_type
		FROM billing.reservation AS r
		JOIN billing.ledger_entry AS l ON l.operation_id = r.operation_id AND l.project_id = r.project_id
		WHERE r.operation_id = ?::uuid AND r.status = 'held'
	`, operationID.String()).Scan(&facts).Error; err != nil || len(facts) != 1 || facts[0].AmountMicros != amount || facts[0].EntryType != "reserve" {
		t.Fatalf("reservation and ledger = %+v, %v", facts, err)
	}
}

func assertReserveBalance(t *testing.T, database *gorm.DB, projectID uuid.UUID, amount, revision int64) {
	t.Helper()
	var budget struct{ ReservedMicros, Revision int64 }
	if err := database.Raw(`SELECT reserved_micros, revision FROM billing.budget WHERE project_id = ?::uuid`, projectID.String()).Scan(&budget).Error; err != nil || budget.ReservedMicros != amount || budget.Revision != revision {
		t.Fatalf("budget after reserve = %+v, %v; want reserved %d revision %d", budget, err, amount, revision)
	}
}
