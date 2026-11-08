package operation_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/app"
	billingevent "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/event"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	inboxapp "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	operationpg "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

func TestCreditOperationSagaFreezesBeforeDispatchAndReleasesOnZeroCostTerminal(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, _, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	item := quotedSnapshotItem(projectID, 20)
	item.Operation.Capability = quoted.Capability
	item.Operation.ModelProfileVersionID = quoted.ModelProfileVersionID
	item.Operation.PriceRuleVersionID = quoted.PriceRuleVersionID
	store := operationStore(database).WithCreditOperationSaga(true)
	require.NoError(t, store.CreateQuoteSnapshot(t.Context(), actor, nil, []operationpg.QuoteItem{item}))

	requestID := uuid.NewString()
	confirmed, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: item.Operation.ID, RequestID: requestID,
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, confirmed.ReservationID)
	replayed, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: item.Operation.ID, RequestID: requestID,
	})
	require.NoError(t, err)
	require.Equal(t, confirmed, replayed)

	var freeze struct {
		ID     uuid.UUID
		Status string
	}
	require.NoError(t, database.Raw(`SELECT id,status FROM billing.credit_command
		WHERE caller_id=? AND action='freeze' AND source_id=?::uuid`, billingapp.CreditOperationCallerID,
		item.Operation.ID).Scan(&freeze).Error)
	require.NotEqual(t, uuid.Nil, freeze.ID)
	require.Equal(t, "accepted", freeze.Status)
	_, err = store.LoadWorkflowOperation(t.Context(), item.Operation.ID)
	require.ErrorIs(t, err, operationapp.ErrCreditFreezePending)

	processOperationCreditCommand(t, database, freeze.ID)
	consumeOperationCreditEvent(t, database, billingevent.CreditCommandResultTopic, "command_id", freeze.ID, store)
	loaded, err := store.LoadWorkflowOperation(t.Context(), item.Operation.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusConfirmed, loaded.Operation.Status)

	_, err = store.TransitionWorkflowOperation(t.Context(), operationapp.TransitionInput{
		OperationID: item.Operation.ID, From: []domain.Status{domain.StatusConfirmed},
		To: domain.StatusCancelling, Reason: "credit_operation_saga_integration",
	})
	require.NoError(t, err)
	finalizer := app.NewOperationFinalizer(database, store, billingpg.NewStore(database))
	require.NoError(t, finalizer.FinalizeOperation(t.Context(), operationSettlementInput(item.Operation.ID)))

	var hold struct {
		ID uuid.UUID `json:"hold_id"`
	}
	var result []byte
	require.NoError(t, database.Raw(`SELECT result FROM billing.credit_command WHERE id=?::uuid`, freeze.ID).Scan(&result).Error)
	require.NoError(t, json.Unmarshal(result, &hold))
	require.NotEqual(t, uuid.Nil, hold.ID)
	consumeOperationCreditEvent(t, database, billingevent.CreditSettlementOutcomeTopic, "hold_id", hold.ID, store)

	var release struct {
		ID     uuid.UUID
		Status string
	}
	require.NoError(t, database.Raw(`SELECT id,status FROM billing.credit_command
		WHERE caller_id=? AND action='release' AND reference_id=?::uuid`, billingapp.CreditSagaCallerID, hold.ID).Scan(&release).Error)
	require.NotEqual(t, uuid.Nil, release.ID)
	require.Equal(t, "accepted", release.Status)
	processOperationCreditCommand(t, database, release.ID)
	consumeOperationCreditEvent(t, database, billingevent.CreditCommandResultTopic, "command_id", release.ID, store)

	var holdStatus, sagaStatus string
	require.NoError(t, database.Raw(`SELECT status FROM billing.credit_hold WHERE id=?::uuid`, hold.ID).Scan(&holdStatus).Error)
	require.NoError(t, database.Raw(`SELECT status FROM billing.credit_saga WHERE hold_id=?::uuid`, hold.ID).Scan(&sagaStatus).Error)
	require.Equal(t, "released", holdStatus)
	require.Equal(t, "released", sagaStatus)
}

func TestCreditOperationSagaFreezeFailureReleasesProjectReservation(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, _, _ := seedUnfundedQuoteCatalog(t, database, projectID, time.Now().UTC())
	seedFixtureProcurement(t, database, *quoted.PriceRuleVersionID, *quoted.ModelProfileVersionID)
	var scopeID uuid.UUID
	require.NoError(t, database.Raw(`SELECT billing.bind_project_wallet(?::uuid)`, projectID).Scan(&scopeID).Error)
	require.NotEqual(t, uuid.Nil, scopeID)

	item := quotedSnapshotItem(projectID, 20)
	item.Operation.Capability = quoted.Capability
	item.Operation.ModelProfileVersionID = quoted.ModelProfileVersionID
	item.Operation.PriceRuleVersionID = quoted.PriceRuleVersionID
	store := operationStore(database).WithCreditOperationSaga(true)
	require.NoError(t, store.CreateQuoteSnapshot(t.Context(), actor, nil, []operationpg.QuoteItem{item}))
	confirmed, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: item.Operation.ID, RequestID: uuid.NewString(),
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, confirmed.ReservationID)

	var freeze struct {
		ID     uuid.UUID
		Status string
	}
	require.NoError(t, database.Raw(`SELECT id,status FROM billing.credit_command
		WHERE caller_id=? AND action='freeze' AND source_id=?::uuid`, billingapp.CreditOperationCallerID,
		item.Operation.ID).Scan(&freeze).Error)
	require.NotEqual(t, uuid.Nil, freeze.ID)
	processOperationCreditCommand(t, database, freeze.ID)
	consumeOperationCreditEvent(t, database, billingevent.CreditCommandResultTopic, "command_id", freeze.ID, store)

	var operation struct {
		Status        string
		FailureCode   *string
		SettledMicros *int64
	}
	require.NoError(t, database.Raw(`SELECT status,failure_code,settled_micros FROM operation.operation WHERE id=?::uuid`,
		item.Operation.ID).Scan(&operation).Error)
	require.Equal(t, string(domain.StatusFailed), operation.Status)
	require.NotNil(t, operation.FailureCode)
	require.Equal(t, "insufficient_credits", *operation.FailureCode)
	require.NotNil(t, operation.SettledMicros)
	require.Zero(t, *operation.SettledMicros)

	var reservedMicros int64
	require.NoError(t, database.Raw(`SELECT reserved_micros FROM billing.budget WHERE project_id=?::uuid`, projectID).Scan(&reservedMicros).Error)
	require.Zero(t, reservedMicros)
}

func operationSettlementInput(operationID uuid.UUID) operationflow.SettlementInput {
	return operationflow.SettlementInput{
		OperationID: operationID.String(), From: domain.StatusCancelling,
		To: domain.StatusCancelled, Reason: "credit_operation_saga_integration",
	}
}

func processOperationCreditCommand(t *testing.T, database *gorm.DB, commandID uuid.UUID) {
	t.Helper()
	err := withOperationCreditAppRole(database, func(tx *gorm.DB) error {
		return billingpg.NewStore(tx).ProcessCreditCommand(t.Context(), commandID)
	})
	require.NoError(t, err)
}

func consumeOperationCreditEvent(t *testing.T, database *gorm.DB, topic, identityField string, identity uuid.UUID, operations *operationpg.Store) {
	t.Helper()
	var record struct {
		ScopeID uuid.UUID
		Payload []byte
	}
	require.NoError(t, database.Raw(`SELECT partition_key AS scope_id,payload FROM infra.outbox
		WHERE topic=? AND payload->>?=? ORDER BY id DESC LIMIT 1`, topic, identityField, identity.String()).Scan(&record).Error)
	require.NotEqual(t, uuid.Nil, record.ScopeID)
	require.NotEmpty(t, record.Payload)
	err := withOperationCreditAppRole(database, func(tx *gorm.DB) error {
		store := billingpg.NewStore(tx)
		handler := billingevent.NewCreditEventHandler(store, nil, store, operations)
		return handler.Handle(t.Context(), inboxapp.Record{Topic: topic, Key: []byte(record.ScopeID.String()), Value: record.Payload})
	})
	require.NoError(t, err)
}

func withOperationCreditAppRole(database *gorm.DB, callback func(*gorm.DB) error) error {
	return database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
			return err
		}
		return callback(tx)
	})
}
