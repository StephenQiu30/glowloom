package catalog_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/pricesync"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func TestPriceCandidateStorePersistsAndRefreshesOnlyPendingSnapshots(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("requires disposable catalog database initialized from backend/db/schema.sql")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	modelID := "integration/" + uuid.NewString()
	fetchedAt := time.Now().UTC().Truncate(time.Microsecond)
	candidate, err := pricesync.ParseOpenRouterPriceCandidate(modelID, json.RawMessage(`{"prompt":"0.0000025","completion":"0.00001"}`), fetchedAt)
	require.NoError(t, err)
	store := postgres.NewStore(conn.DB)
	changed, err := store.UpsertPriceCandidates(t.Context(), []domain.PriceCandidate{candidate})
	require.NoError(t, err)
	require.Equal(t, 1, changed)

	candidate.SourceFetchedAt = fetchedAt.Add(time.Minute)
	changed, err = store.UpsertPriceCandidates(t.Context(), []domain.PriceCandidate{candidate})
	require.NoError(t, err)
	require.Equal(t, 1, changed)
	var row struct {
		SourceFetchedAt time.Time
		Status          string
		Rule            string
	}
	require.NoError(t, conn.DB.Raw(`SELECT source_fetched_at,status,rule::text AS rule FROM catalog.model_price_candidate WHERE id=?::uuid`, candidate.ID.String()).Scan(&row).Error)
	require.True(t, candidate.SourceFetchedAt.Equal(row.SourceFetchedAt))
	require.Equal(t, "pending_review", row.Status)
	require.JSONEq(t, string(candidate.Rule), row.Rule)
}
