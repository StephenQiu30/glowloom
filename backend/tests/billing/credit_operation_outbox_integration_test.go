package billing_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
)

func TestOperationCreditOutcomeOutboxReplaysAcrossMonthsAndConcurrentWriters(t *testing.T) {
	ctx, database, operationID := operationCreditOutcomeFixture(t)
	at := time.Now().UTC()
	const writers = 3
	start := make(chan struct{})
	errorsByWriter := make([]error, writers)
	var wait sync.WaitGroup
	for index := range writers {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			errorsByWriter[index] = queueOperationOutcomeAsApp(ctx, database, operationID, at.AddDate(0, index, 0))
		}(index)
	}
	close(start)
	wait.Wait()
	for _, err := range errorsByWriter {
		require.NoError(t, err)
	}
	var count int64
	require.NoError(t, database.Raw(`SELECT count(*) FROM infra.outbox WHERE topic=? AND payload->>'hold_id'=(SELECT result->>'hold_id' FROM billing.credit_command WHERE source_id=? AND caller_id=? AND action='freeze')`, billingapp.CreditSettlementOutcomeTopic, operationID, billingapp.CreditOperationCallerID).Scan(&count).Error)
	require.EqualValues(t, 1, count, "one stable business event survives concurrent and cross-month replays")
}

func TestOperationCreditOutcomeOutboxRejectsConflictingPersistedFacts(t *testing.T) {
	for _, scenario := range []string{"amount_micros", "scope_id", "hold_id", "outcome", "event_id", "event_type", "topic", "partition", "duplicate_id"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, database, operationID := operationCreditOutcomeFixture(t)
			at := time.Now().UTC()
			require.NoError(t, queueOperationOutcomeAsApp(ctx, database, operationID, at))
			var original struct {
				ID      uuid.UUID
				Payload []byte
			}
			require.NoError(t, database.Raw(`SELECT id,payload FROM infra.outbox WHERE topic=? AND payload->>'hold_id'=(SELECT result->>'hold_id' FROM billing.credit_command WHERE source_id=? AND caller_id=? AND action='freeze')`, billingapp.CreditSettlementOutcomeTopic, operationID, billingapp.CreditOperationCallerID).Scan(&original).Error)
			switch scenario {
			case "topic":
				require.NoError(t, database.Exec(`UPDATE infra.outbox SET topic='fixture.other' WHERE id=?`, original.ID).Error)
			case "partition":
				require.NoError(t, database.Exec(`UPDATE infra.outbox SET partition_key=? WHERE id=?`, uuid.NewString(), original.ID).Error)
			case "duplicate_id":
				require.NoError(t, database.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload,create_time) SELECT id,topic,partition_key,payload,create_time+interval '1 month' FROM infra.outbox WHERE id=?`, original.ID).Error)
			default:
				var payload map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(original.Payload, &payload))
				value := any(uuid.NewString())
				switch scenario {
				case "amount_micros":
					value = 30001
				case "outcome":
					value = "failed"
				case "event_type":
					value = "fixture.other"
				}
				raw, err := json.Marshal(value)
				require.NoError(t, err)
				payload[scenario] = raw
				changed, err := json.Marshal(payload)
				require.NoError(t, err)
				require.NoError(t, database.Exec(`UPDATE infra.outbox SET payload=?::jsonb WHERE id=?`, string(changed), original.ID).Error)
			}
			require.ErrorIs(t, queueOperationOutcomeAsApp(ctx, database, operationID, at), billingapp.ErrCreditOutcomeConflict)
		})
	}
}

func operationCreditOutcomeFixture(t *testing.T) (context.Context, *gorm.DB, uuid.UUID) {
	t.Helper()
	ctx, database := billingDB(t)
	actor, project := billingProject(t, database)
	seedReserveBudget(t, database, project, 100000)
	scope := bindWallet(t, database, project)
	issueWallet(t, database, scope, uuid.New(), 100000, time.Now().Add(time.Hour), nil)
	operationID := seedReserveOperation(t, database, project, 100000)
	freeze := testCreditCommand(billingapp.CreditOperationCallerID, actor.ScopeID, billingapp.CreditFreeze, operationID, uuid.New(), 100000)
	accepted, err := billingpg.NewStore(database).SubmitCreditCommand(ctx, freeze)
	require.NoError(t, err)
	processCreditCommandAsApp(ctx, t, database, accepted.ID)
	return ctx, database, operationID
}

func queueOperationOutcomeAsApp(ctx context.Context, database *gorm.DB, operationID uuid.UUID, at time.Time) error {
	return database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
			return err
		}
		return billingpg.NewStore(tx).QueueOperationCreditSettlementOutcomeInTransaction(ctx, tx, operationID, "succeeded", 30000, at)
	})
}
