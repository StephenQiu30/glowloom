package aigateway_test

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	pg "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

func TestRebuiltUnknownReconciliationIsAuditedAndIdempotent(t *testing.T) {
	for _, free := range []bool{false, true} {
		t.Run(map[bool]string{false: "charged", true: "not-charged"}[free], func(t *testing.T) {
			db, s, m, a := fixture(t)
			d, err := s.Admit(t.Context(), call(t, m, a))
			require.NoError(t, err)
			require.NoError(t, s.Unknown(t.Context(), d.Call.ID, d.Attempt))
			in := pg.Reconciliation{Request: d.Call.ID, Attempt: d.Attempt, Reference: "supplier-statement/fixture", Reason: "synthetic invoice reconciled", NotCharged: free}
			if !free {
				in.Usage = &domain.Usage{Input: 10, Output: 3}
			}
			first, err := s.Reconcile(t.Context(), in)
			require.NoError(t, err)
			again, err := s.Reconcile(t.Context(), in)
			require.NoError(t, err)
			require.Equal(t, first, again)
			require.Equal(t, first.Held, first.Charged+first.Released)
			var n int64
			require.NoError(t, db.Raw("SELECT count(*) FROM ai.money_event WHERE scope_id=? AND kind='grant' AND actor_id=?", a.ScopeID, a.ID).Scan(&n).Error)
			require.EqualValues(t, 1, n)
			require.NoError(t, db.Raw("SELECT count(*) FROM ai.reconciliation WHERE request_id=? AND actor_id=?", d.Call.ID, a.ID).Scan(&n).Error)
			require.EqualValues(t, 1, n)
			in.Reason = "changed"
			_, err = s.Reconcile(t.Context(), in)
			require.ErrorIs(t, err, domain.ErrConflict)
		})
	}
}
func TestRebuiltConcurrentCredentialGroupsCannotSplitQuota(t *testing.T) {
	_, s, m, _ := fixture(t)
	credential, supplier := uuid.New(), uuid.New()
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	for range 2 {
		wg.Go(func() {
			r := domain.Route{ID: uuid.New(), ModelID: m.ID, Credential: credential, Supplier: supplier, Group: uuid.NewString(), Protocol: m.Protocol, BaseURL: "https://8.8.8.8/v1", UpstreamModel: "supplier", Purchase: m.Price}
			err := s.PublishRoute(t.Context(), r, []byte("one-key"), "fixture", 1, 100)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				success++
			} else {
				require.ErrorIs(t, err, domain.ErrConflict)
			}
		})
	}
	wg.Wait()
	require.Equal(t, 1, success)
}
