package operation_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

func TestOperationTaskAdmissionKeepsOneOriginalTaskAcrossCompletion(t *testing.T) {
	db := operationStoreDB(t)
	id := uuid.NewString()
	for n := 0; n < 2; n++ {
		require.NoError(t, taskruntime.Enqueue(t.Context(), db, "operation.run", id, time.Now()))
	}
	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM infra.task_work WHERE kind='operation.run' AND business_id=?`, id).Scan(&n).Error)
	require.EqualValues(t, 1, n)
	require.NoError(t, db.Exec(`UPDATE infra.task_work SET completed_at=clock_timestamp() WHERE kind='operation.run' AND business_id=?`, id).Error)
	require.NoError(t, taskruntime.Enqueue(t.Context(), db, "operation.run", id, time.Now()))
	var done bool
	require.NoError(t, db.Raw(`SELECT completed_at IS NOT NULL FROM infra.task_work WHERE kind='operation.run' AND business_id=?`, id).Scan(&done).Error)
	require.True(t, done)
}
