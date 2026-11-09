package operation_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

func TestTrustedTerminalQueryCoversOnlyEarlierReadsOfOriginalTask(t *testing.T) {
	for _, scenario := range []string{"recovered", "unknown_cancel", "unknown_submit", "other_task", "other_identity", "no_dispatch", "wrong_manifest", "no_terminal", "late_unknown"} {
		t.Run(scenario, func(t *testing.T) {
			db := operationStoreDB(t)
			f := seedManagedFixture(t, db)
			id := f.identity.OperationID
			require.NoError(t, db.Exec(`UPDATE catalog.price_rule_version SET unit='per_1k_tokens',rule='{"input_micros_per_1k":0,"output_micros_per_1k":23000}' WHERE id=?`, f.identity.PriceRuleVersionID).Error)
			dispatch, err := f.store.ClaimProviderDispatch(t.Context(), f.identity)
			require.NoError(t, err)
			require.True(t, dispatch.Claimed)
			taskID := "original-task"
			require.NoError(t, f.store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{OperationID: id, Action: "submit", Attempt: 1, Outcome: "ok", State: "accepted", ProviderTaskID: &taskID}))
			_, err = f.store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: id, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusSubmitted, ProviderTaskID: &taskID})
			require.NoError(t, err)
			for _, attempt := range []int32{1, 2} {
				require.NoError(t, f.store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{OperationID: id, Action: "query", Attempt: attempt, ProviderTaskID: &taskID}))
			}
			require.ErrorIs(t, func() error { _, e := f.store.LoadProviderCost(t.Context(), id); return e }(), application.ErrProviderCostUnknown)
			usage := json.RawMessage(`{"input_tokens":0,"output_tokens":40594,"billable_requests":1}`)
			require.NoError(t, f.store.SaveManagedResult(t.Context(), application.ManagedResult{Identity: f.identity, State: "succeeded", TaskID: taskID, Text: "original terminal output", Usage: usage}))
			for _, attempt := range []int32{3, 4} {
				require.NoError(t, f.store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{OperationID: id, Action: "query", Attempt: attempt, ProviderTaskID: &taskID}))
				require.NoError(t, f.store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{OperationID: id, Action: "query", Attempt: attempt, Outcome: "ok", State: "succeeded", ProviderTaskID: &taskID, Managed: true}))
			}
			switch scenario {
			case "unknown_cancel":
				require.NoError(t, db.Exec(`UPDATE operation.provider_call SET action='cancel' WHERE operation_id=? AND action='query' AND attempt=1`, id).Error)
			case "unknown_submit":
				require.NoError(t, db.Exec(`UPDATE operation.provider_call SET action='submit',attempt=2 WHERE operation_id=? AND action='query' AND attempt=1`, id).Error)
			case "other_task":
				require.NoError(t, db.Exec(`UPDATE operation.provider_call SET provider_task_id='other-task' WHERE operation_id=? AND action='query' AND attempt=1`, id).Error)
			case "other_identity":
				require.NoError(t, db.Exec(`UPDATE operation.provider_call SET request_key='other-key' WHERE operation_id=? AND action='query' AND attempt=1`, id).Error)
			case "no_dispatch":
				require.NoError(t, db.Exec(`UPDATE operation.provider_call SET dispatch_started_at=NULL WHERE operation_id=? AND action='submit'`, id).Error)
			case "wrong_manifest":
				require.NoError(t, db.Exec(`UPDATE operation.provider_result SET result=jsonb_set(result,'{identity,request_key}','"other-key"') WHERE operation_id=?`, id).Error)
			case "no_terminal":
				require.NoError(t, db.Exec(`UPDATE operation.provider_call SET cost_micros=NULL WHERE operation_id=? AND action='query' AND attempt>=3`, id).Error)
			case "late_unknown":
				require.NoError(t, f.store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{OperationID: id, Action: "query", Attempt: 5, ProviderTaskID: &taskID}))
			}
			var actual int64
			err = db.Transaction(func(tx *gorm.DB) error {
				if e := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; e != nil {
					return e
				}
				cost, e := operationStore(tx).LoadProviderCost(t.Context(), id)
				actual = cost.ActualCostMicros
				return e
			})
			if scenario != "recovered" {
				require.ErrorIs(t, err, application.ErrProviderCostUnknown)
				return
			}
			require.NoError(t, err)
			require.EqualValues(t, 933662, actual)
			var rows []struct {
				Attempt    int32
				CostMicros *int64
			}
			require.NoError(t, db.Raw(`SELECT attempt,cost_micros FROM operation.provider_call WHERE operation_id=? AND action='query' ORDER BY attempt`, id).Scan(&rows).Error)
			require.Len(t, rows, 4)
			require.Nil(t, rows[0].CostMicros)
			require.Nil(t, rows[1].CostMicros)
			require.EqualValues(t, 933662, *rows[2].CostMicros)
			require.Zero(t, *rows[3].CostMicros)
		})
	}
}
