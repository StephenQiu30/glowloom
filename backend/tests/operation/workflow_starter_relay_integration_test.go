package operation_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestConfirmationCommitsRunnableTaskWithoutEventRedelivery(t *testing.T) {
	db := operationStoreDB(t)
	actor, project := operationStoreProject(t, db)
	quoted, _, _ := seedQuoteCatalog(t, db, project, time.Now().UTC())
	item := quotedSnapshotItem(project, 1)
	item.Operation = quoted
	item.Inputs[0].OperationID = quoted.ID
	s := operationStore(db)
	require.NoError(t, s.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{item}))
	in := operationapp.ConfirmSingleQuoteInput{ProjectID: project, OperationID: quoted.ID, RequestID: uuid.NewString()}
	for n := 0; n < 2; n++ {
		_, err := s.ConfirmSingleQuote(t.Context(), actor, in)
		require.NoError(t, err)
	}
	var count int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM infra.task_work WHERE kind='operation.run' AND business_id=?`, quoted.ID.String()).Scan(&count).Error)
	require.EqualValues(t, 1, count)
}
func TestBatchConfirmationCommitsOnlyParentTask(t *testing.T) {
	db := operationStoreDB(t)
	actor, project, batch, items := seedConfirmableBatch(t, db, 2)
	s := operationStore(db)
	_, err := s.ConfirmBatchQuote(t.Context(), actor, operationapp.ConfirmBatchQuoteInput{ProjectID: project, BatchID: batch, RequestID: uuid.NewString()})
	require.NoError(t, err)
	var count int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM infra.task_work WHERE kind='operation.batch' AND business_id=?`, batch.String()).Scan(&count).Error)
	require.EqualValues(t, 1, count)
	for _, item := range items {
		require.NoError(t, db.Raw(`SELECT count(*) FROM infra.task_work WHERE kind='operation.run' AND business_id=?`, item.Operation.ID.String()).Scan(&count).Error)
		require.Zero(t, count)
	}
}
