package operation_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	pgbilling "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	task "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/tests/taskfixture"
)

func TestProductionFlowTaskCancelsBatchAfterWorkerRestart(t *testing.T) {
	db := operationStoreDB(t)
	actor, project, batch, items := seedConfirmableBatch(t, db, 1)
	s := operationStore(db)
	_, err := s.ConfirmBatchQuote(t.Context(), actor, operationapp.ConfirmBatchQuoteInput{ProjectID: project, BatchID: batch, RequestID: uuid.NewString()})
	require.NoError(t, err)
	require.NoError(t, s.MarkWorkflowBatchRunning(t.Context(), batch))
	require.NoError(t, s.MarkWorkflowBatchCancelRequested(t.Context(), batch))
	h := map[string]taskruntime.Handler{}
	task.RegisterFlow(h, s, app.NewOperationFinalizer(db, s, pgbilling.NewStore(db)), nil)
	stop := taskfixture.Start(t, db, h)
	stop()
	taskfixture.Start(t, db, h)
	taskfixture.WaitWork(t, db, "operation.batch", batch.String(), 15*time.Second)
	assertCancelledBatchMember(t, db, items[0].Operation.ID)
	var status string
	require.NoError(t, db.Raw(`SELECT status FROM operation.batch WHERE id=?`, batch).Scan(&status).Error)
	require.Equal(t, "cancelled", status)
}
