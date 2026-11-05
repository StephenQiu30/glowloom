package operation_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	application "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// The former event starter eligibility is now enforced at confirmation and
// batch slot acquisition. Recovery scans task_work rather than Kafka history.
func TestBatchTaskStoreValidatesSelectedReservationsAndRuntimeRole(t *testing.T) {
	database := operationStoreDB(t)
	actor, project, batch, items := seedConfirmableBatch(t, database, 1)
	store := operationStore(database)
	_, err := store.ConfirmBatchQuote(t.Context(), actor, application.ConfirmBatchQuoteInput{ProjectID: project, BatchID: batch, RequestID: uuid.NewString()})
	require.NoError(t, err)
	snapshot, err := store.LoadWorkflowBatch(t.Context(), batch)
	require.NoError(t, err)
	require.Len(t, snapshot.Items, 1)
	require.Equal(t, items[0].Operation.ID, snapshot.Items[0].OperationID)
	require.NoError(t, database.Exec(`INSERT INTO operation.operation(id,project_id,batch_id,target_type,capability,mode,model_profile_version_id,price_rule_version_id,params,output_count,input_hash,origin,status,quote_micros,quote_expires_at,region) SELECT ?,project_id,batch_id,target_type,capability,mode,model_profile_version_id,price_rule_version_id,params,output_count,?,origin,'expired',quote_micros,quote_expires_at,region FROM operation.operation WHERE id=?`, uuid.New(), uuid.NewString(), items[0].Operation.ID).Error)
	snapshot, err = store.LoadWorkflowBatch(t.Context(), batch)
	require.NoError(t, err)
	require.Len(t, snapshot.Items, 1, "unselected expired quote may not become a task")
	require.NoError(t, database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
			return err
		}
		_, err := operationStore(tx).LoadWorkflowBatch(t.Context(), batch)
		return err
	}))
	require.NoError(t, store.MarkWorkflowBatchRunning(t.Context(), batch))
	require.NoError(t, store.MarkWorkflowBatchRunning(t.Context(), batch))
	require.NoError(t, database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
			return err
		}
		ok, err := operationStore(tx).AcquireBatchLaunch(t.Context(), batch, items[0].Operation.ID)
		if err == nil && !ok {
			return errors.New("runtime role could not claim batch child")
		}
		return err
	}))
	ok, err := store.AcquireBatchLaunch(t.Context(), batch, items[0].Operation.ID)
	require.NoError(t, err)
	require.True(t, ok)
	var slots int64
	require.NoError(t, database.Raw(`SELECT count(*) FROM operation.batch_launch WHERE operation_id=? AND state='active'`, items[0].Operation.ID).Scan(&slots).Error)
	require.EqualValues(t, 1, slots)
	require.NoError(t, database.Exec(`UPDATE operation.operation SET reservation_id=NULL WHERE id=?`, items[0].Operation.ID).Error)
	_, err = store.LoadWorkflowBatch(t.Context(), batch)
	require.ErrorIs(t, err, application.ErrInvalidWorkflowBatch)
}
