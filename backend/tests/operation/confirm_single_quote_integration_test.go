package operation_test

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	billingdomain "github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func TestConfirmSingleQuoteCommitsReservationAndEvents(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	now := time.Now().UTC()
	quoted, _, _ := seedQuoteCatalog(t, database, projectID, now)
	quoted.QuoteMicros = int64Pointer(60)
	item := quotedSnapshotItem(projectID, 60)
	item.Operation = quoted
	item.Inputs[0].OperationID = quoted.ID
	store := pgoperation.NewStore(database)
	if err := store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{item}); err != nil {
		t.Fatalf("seed quote: %v", err)
	}
	input := operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: quoted.ID,
		RequestID: uuid.NewString(),
	}
	result, err := store.ConfirmSingleQuote(t.Context(), actor, input)
	if err != nil || result.ReservationID == uuid.Nil || result.AvailableMicros != 40 {
		t.Fatalf("confirm quote = %+v, %v", result, err)
	}
	got, err := store.FindOperation(t.Context(), actor, projectID, quoted.ID)
	if err != nil || got.Status != domain.StatusConfirmed ||
		got.ReservationID == nil || *got.ReservationID != result.ReservationID {
		t.Fatalf("confirmed operation = %+v, %v", got, err)
	}
	var reservations, ledger, events int64
	if err := database.Raw(`SELECT count(*) FROM billing.reservation WHERE operation_id = ?::uuid AND status = 'held'`, quoted.ID.String()).Scan(&reservations).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Raw(`SELECT count(*) FROM billing.ledger_entry WHERE operation_id = ?::uuid AND entry_type = 'reserve'`, quoted.ID.String()).Scan(&ledger).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key = ? AND topic IN ('lanverse.operation.confirmed.v1', 'lanverse.audit.recorded.v1')`, projectID.String()).Scan(&events).Error; err != nil {
		t.Fatal(err)
	}
	if reservations != 1 || ledger != 1 || events != 2 {
		t.Fatalf("confirmed facts: reservations=%d ledger=%d events=%d", reservations, ledger, events)
	}
	if replay, err := store.ConfirmSingleQuote(t.Context(), actor, input); err != nil || replay != result {
		t.Fatalf("replayed confirmation = %+v, %v", replay, err)
	}
	input.RequestID = uuid.NewString()
	if _, err := store.ConfirmSingleQuote(t.Context(), actor, input); !errors.Is(err, domain.ErrQuoteNotConfirmable) {
		t.Fatalf("new request against confirmed operation = %v", err)
	}
}

func TestConfirmSingleQuoteRollsBackOnInsufficientBudget(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	now := time.Now().UTC()
	quoted, _, _ := seedQuoteCatalog(t, database, projectID, now)
	quoted.QuoteMicros = int64Pointer(101)
	item := quotedSnapshotItem(projectID, 101)
	item.Operation = quoted
	item.Inputs[0].OperationID = quoted.ID
	store := pgoperation.NewStore(database)
	if err := store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{item}); err != nil {
		t.Fatalf("seed quote: %v", err)
	}
	_, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: quoted.ID,
		RequestID: uuid.NewString(),
	})
	if !errors.Is(err, billingdomain.ErrBudgetInsufficient) {
		t.Fatalf("insufficient budget = %v", err)
	}
	got, err := store.FindOperation(t.Context(), actor, projectID, quoted.ID)
	if err != nil || got.Status != domain.StatusQuoted || got.ReservationID != nil {
		t.Fatalf("quote after failed confirmation = %+v, %v", got, err)
	}
}

func TestConfirmSingleQuoteRejectsForeignAndRevokedActors(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	foreign, _ := operationStoreProject(t, database)
	now := time.Now().UTC()
	quoted, _, _ := seedQuoteCatalog(t, database, projectID, now)
	item := quotedSnapshotItem(projectID, 1)
	item.Operation = quoted
	item.Inputs[0].OperationID = quoted.ID
	store := pgoperation.NewStore(database)
	if err := store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{item}); err != nil {
		t.Fatalf("seed quote: %v", err)
	}
	input := operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: quoted.ID,
		RequestID: uuid.NewString(),
	}
	if _, err := store.ConfirmSingleQuote(t.Context(), foreign, input); !errors.Is(err, pgoperation.ErrNotFound) {
		t.Fatalf("foreign actor = %v", err)
	}
	if err := database.Exec(`UPDATE identity."user" SET status = 'disabled' WHERE id = ?::uuid`, actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfirmSingleQuote(t.Context(), actor, input); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked actor = %v", err)
	}
}

func TestConfirmSingleQuoteExpiresChangedModelBeforeReservation(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	now := time.Now().UTC()
	quoted, modelID, _ := seedQuoteCatalog(t, database, projectID, now)
	item := quotedSnapshotItem(projectID, 1)
	item.Operation = quoted
	item.Inputs[0].OperationID = quoted.ID
	store := pgoperation.NewStore(database)
	if err := store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{item}); err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE catalog.model_profile SET status = 'disabled' WHERE id = ?::uuid`, modelID.String()).Error; err != nil {
		t.Fatal(err)
	}
	requestID := uuid.NewString()
	_, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: quoted.ID, RequestID: requestID,
	})
	var stale *pgoperation.QuoteStaleError
	if !errors.As(err, &stale) || len(stale.Reasons) != 2 ||
		stale.Reasons[0] != "model_unavailable" || stale.Reasons[1] != "price_unavailable" {
		t.Fatalf("changed model rejection = %v", err)
	}
	_, replayErr := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: quoted.ID, RequestID: requestID,
	})
	var replayStale *pgoperation.QuoteStaleError
	if !errors.As(replayErr, &replayStale) || !reflect.DeepEqual(replayStale.Reasons, stale.Reasons) {
		t.Fatalf("stale quote replay = %v", replayErr)
	}
	got, err := store.FindOperation(t.Context(), actor, projectID, quoted.ID)
	if err != nil || got.Status != domain.StatusExpired {
		t.Fatalf("stale quote status = %+v, %v", got, err)
	}
	var reservations int64
	if err := database.Raw(`SELECT count(*) FROM billing.reservation WHERE operation_id = ?::uuid`, quoted.ID.String()).Scan(&reservations).Error; err != nil || reservations != 0 {
		t.Fatalf("stale quote reservations=%d err=%v", reservations, err)
	}
}

func TestConfirmSingleQuoteRunsAsRuntimeRole(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, _, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	item := quotedSnapshotItem(projectID, 1)
	item.Operation = quoted
	item.Inputs[0].OperationID = quoted.ID
	if err := pgoperation.NewStore(database).CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{item}); err != nil {
		t.Fatal(err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		_, err := pgoperation.NewStore(tx).ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
			ProjectID: projectID, OperationID: quoted.ID, RequestID: uuid.NewString(),
		})
		return err
	}); err != nil {
		t.Fatalf("confirm as runtime role: %v", err)
	}
}

func TestConfirmSingleQuoteRollsBackWhenOutboxInsertFails(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, _, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	item := quotedSnapshotItem(projectID, 1)
	item.Operation = quoted
	item.Inputs[0].OperationID = quoted.ID
	store := pgoperation.NewStore(database)
	if err := store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{item}); err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`REVOKE INSERT ON infra.outbox FROM lanverse_app`).Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := database.Exec(`GRANT INSERT ON infra.outbox TO lanverse_app`).Error; err != nil {
			t.Errorf("restore outbox grant: %v", err)
		}
	}()
	err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		_, err := pgoperation.NewStore(tx).ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
			ProjectID: projectID, OperationID: quoted.ID, RequestID: uuid.NewString(),
		})
		return err
	})
	if err == nil {
		t.Fatal("confirmation succeeded without Outbox INSERT privilege")
	}
	got, err := store.FindOperation(t.Context(), actor, projectID, quoted.ID)
	if err != nil || got.Status != domain.StatusQuoted || got.ReservationID != nil {
		t.Fatalf("quote after Outbox failure = %+v, %v", got, err)
	}
	var reservations, ledger int64
	if err := database.Raw(`SELECT count(*) FROM billing.reservation WHERE operation_id = ?::uuid`, quoted.ID.String()).Scan(&reservations).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Raw(`SELECT count(*) FROM billing.ledger_entry WHERE operation_id = ?::uuid`, quoted.ID.String()).Scan(&ledger).Error; err != nil {
		t.Fatal(err)
	}
	if reservations != 0 || ledger != 0 {
		t.Fatalf("Outbox failure left reservation=%d ledger=%d", reservations, ledger)
	}
}

func TestConfirmSingleQuoteSerializesCompetingBudgetReservations(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, _, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	store := pgoperation.NewStore(database)
	operationIDs := make([]uuid.UUID, 2)
	for index := range operationIDs {
		item := quotedSnapshotItem(projectID, 60)
		item.Operation.Capability = quoted.Capability
		item.Operation.ModelProfileVersionID = quoted.ModelProfileVersionID
		item.Operation.PriceRuleVersionID = quoted.PriceRuleVersionID
		if err := store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{item}); err != nil {
			t.Fatal(err)
		}
		operationIDs[index] = item.Operation.ID
	}
	var wait sync.WaitGroup
	results := make([]error, len(operationIDs))
	for index, operationID := range operationIDs {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, results[index] = store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
				ProjectID: projectID, OperationID: operationID, RequestID: uuid.NewString(),
			})
		}()
	}
	wait.Wait()
	var confirmed, insufficient int
	for _, err := range results {
		switch {
		case err == nil:
			confirmed++
		case errors.Is(err, billingdomain.ErrBudgetInsufficient):
			insufficient++
		default:
			t.Fatalf("unexpected concurrent result: %v", err)
		}
	}
	if confirmed != 1 || insufficient != 1 {
		t.Fatalf("concurrent outcomes confirmed=%d insufficient=%d", confirmed, insufficient)
	}
	var held int64
	if err := database.Raw(`SELECT count(*) FROM billing.reservation WHERE project_id = ?::uuid AND status = 'held'`, projectID.String()).Scan(&held).Error; err != nil || held != 1 {
		t.Fatalf("held reservations=%d err=%v", held, err)
	}
}

func int64Pointer(value int64) *int64 { return &value }
