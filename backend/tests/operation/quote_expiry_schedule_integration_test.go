package operation_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	task "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

func TestScheduledQuoteExpiryRunsAgainstPostgresAfterRestart(t *testing.T) {
	db := operationStoreDB(t)
	actor, project := operationStoreProject(t, db)
	item := quotedSnapshotItem(project, 1)
	created := time.Now().Add(-20 * time.Minute)
	expires := created.Add(15 * time.Minute)
	item.Operation.CreateTime = created
	item.Operation.QuoteExpiresAt = &expires
	s := operationStore(db)
	require.NoError(t, s.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{item}))
	id := uuid.NewString()
	require.NoError(t, taskruntime.Enqueue(t.Context(), db, "operation.quote_expiry", id, time.Now().Add(-time.Hour)))
	store := taskruntime.NewStore(db).ForBusiness(id)
	job, err := store.Claim(t.Context(), []string{"operation.quote_expiry"}, "restarted-owner", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job)
	next, err := task.QuoteExpiryHandler(operationapp.NewQuoteExpiryService(s, 10))(t.Context(), *job)
	require.NoError(t, err)
	require.True(t, next.After(time.Now()))
	got, err := s.FindOperation(t.Context(), actor, project, item.Operation.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusExpired, got.Status)
	require.NoError(t, store.Reschedule(t.Context(), *job, next))
}
