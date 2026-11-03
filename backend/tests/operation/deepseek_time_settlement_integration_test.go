package operation_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/managed"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

func TestDeepSeekTimeSettlementUsesOriginalEvidenceAndSingleLedger(t *testing.T) {
	for _, tc := range []struct {
		name, start, end string
		want             int64
		valid            bool
	}{
		{"off peak original result", "2026-11-04T12:01:00+08:00", "2026-11-04T12:01:01+08:00", 43, true},
		{"peak original result", "2026-11-04T09:01:00+08:00", "2026-11-04T09:01:01+08:00", 85, true},
		{"boundary ambiguous", "2026-11-04T11:59:59+08:00", "2026-11-04T12:00:01+08:00", 0, false},
		{"missing response clock", "2026-11-04T12:01:00+08:00", "", 0, false},
		{"unreviewed year", "2027-01-04T12:01:00+08:00", "2027-01-04T12:01:01+08:00", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := operationStoreDB(t)
			f := seedManagedFixture(t, db)
			i := f.identity
			require.NoError(t, db.Exec(`UPDATE catalog.provider SET adapter_key='deepseek',region='domestic' WHERE id=?`, f.provider).Error)
			require.NoError(t, db.Exec(`UPDATE catalog.model_profile SET capability='text.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, i.ModelProfileVersionID).Error)
			require.NoError(t, db.Exec(`UPDATE catalog.price_rule_version SET unit='per_1k_tokens',rule=?::jsonb WHERE id=?`, deepSeekTimeRule, i.PriceRuleVersionID).Error)
			require.NoError(t, db.Exec(`UPDATE operation.operation SET capability='text.generate',mode='text_to_text',params='{}',quote_micros=100 WHERE id=?`, i.OperationID).Error)
			require.NoError(t, db.Exec(`UPDATE billing.budget SET reserved_micros=100 WHERE project_id=?`, i.ProjectID).Error)
			require.NoError(t, db.Exec(`UPDATE billing.reservation SET amount_micros=100 WHERE operation_id=?`, i.OperationID).Error)
			require.NoError(t, db.Exec(`INSERT INTO operation.provider_credential_binding(operation_id,credential_id) VALUES(?,?)`, i.OperationID, f.credential).Error)
			start, err := time.Parse(time.RFC3339, tc.start)
			require.NoError(t, err)
			require.NoError(t, db.Exec(`UPDATE operation.provider_call SET dispatch_started_at=? WHERE operation_id=? AND action='submit'`, start, i.OperationID).Error)
			var end *time.Time
			if tc.end != "" {
				value, err := time.Parse(time.RFC3339, tc.end)
				require.NoError(t, err)
				end = &value
			}
			original := application.ManagedResult{Identity: i, State: "succeeded", Text: "original stored reply", ReceivedAt: end, Usage: json.RawMessage(`{"input_tokens":10,"cached_input_tokens":8,"output_tokens":10}`)}
			results := application.NewManagedResults(&stageTestObjects{entries: map[string]stageTestObject{}}, f.store)
			require.NoError(t, results.Save(t.Context(), original))
			// A later unrelated price must not be selected when recovering this result.
			require.NoError(t, db.Exec(`INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,currency,effective_from) SELECT ?,model_profile_id,version_no+1,unit,'{"input_micros_per_1k":99999,"output_micros_per_1k":99999}',currency,now() FROM catalog.price_rule_version WHERE id=?`, uuid.New(), i.PriceRuleVersionID).Error)
			recovered, err := results.Recover(t.Context(), i)
			require.NoError(t, err)
			require.Equal(t, end, recovered.ReceivedAt)
			completion := application.CompleteProviderCallInput{OperationID: i.OperationID, Action: "submit", Attempt: 1, Outcome: "ok", State: "completed", Managed: true}
			err = f.store.CompleteProviderCall(t.Context(), completion)
			if !tc.valid {
				require.ErrorIs(t, err, application.ErrProviderCostUnknown)
				activityErr := operationflow.NewActivities(f.store, nil).CompleteProviderCall(t.Context(), completion)
				var typed *temporal.ApplicationError
				require.ErrorAs(t, activityErr, &typed)
				require.Equal(t, "provider_cost_unknown", typed.Type())
				require.True(t, typed.NonRetryable())
				_, err = f.store.LoadProviderCost(t.Context(), i.OperationID)
				require.Error(t, err)
				var held int64
				require.NoError(t, db.Raw(`SELECT amount_micros FROM billing.reservation WHERE operation_id=? AND status='held'`, i.OperationID).Scan(&held).Error)
				require.Equal(t, int64(100), held)
				return
			}
			require.NoError(t, err)
			require.NoError(t, f.store.CompleteProviderCall(t.Context(), completion))
			cost, err := f.store.LoadProviderCost(t.Context(), i.OperationID)
			require.NoError(t, err)
			require.Equal(t, tc.want, cost.ActualCostMicros)
			_, err = f.store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusSucceeded})
			require.NoError(t, err)
			_, err = f.store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSucceeded}, To: domain.StatusIngesting})
			require.NoError(t, err)
			require.NoError(t, f.store.IngestManagedText(t.Context(), i))
			finalizer := app.NewOperationFinalizer(db, f.store, billingpg.NewStore(db))
			finish := operationflow.SettlementInput{OperationID: i.OperationID.String(), From: domain.StatusIngesting, To: domain.StatusCompleted, ActualCostMicros: tc.want}
			require.NoError(t, finalizer.FinalizeOperation(t.Context(), finish))
			require.NoError(t, finalizer.FinalizeOperation(t.Context(), finish))
			var ledger struct {
				Count  int64
				Amount int64
			}
			require.NoError(t, db.Raw(`SELECT count(*) AS count,sum(amount_micros) AS amount FROM billing.ledger_entry WHERE operation_id=? AND entry_type='settle'`, i.OperationID).Scan(&ledger).Error)
			require.Equal(t, int64(1), ledger.Count)
			require.Equal(t, tc.want, ledger.Amount)
			var released int64
			require.NoError(t, db.Raw(`SELECT sum(amount_micros) FROM billing.ledger_entry WHERE operation_id=? AND entry_type='release'`, i.OperationID).Scan(&released).Error)
			require.Equal(t, int64(100)-tc.want, released)
		})
	}
}

func TestDeepSeekWorkerPersistsOriginalResponseClock(t *testing.T) {
	db := operationStoreDB(t)
	f := seedManagedFixture(t, db)
	i := f.identity
	require.NoError(t, db.Exec(`UPDATE catalog.provider SET adapter_key='deepseek',region='domestic' WHERE id=?`, f.provider).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.provider_config_version SET config=(SELECT payload->'config' FROM catalog.provider_preset_version WHERE id='6da07a11-0000-4000-8000-000000000004') WHERE id=?`, f.config).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile SET capability='text.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, i.ModelProfileVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text_to_text'],queue='provider',supports_cancel=false,supports_query=false,moderation='provider' WHERE id=?`, i.ModelProfileVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE operation.operation SET capability='text.generate',mode='text_to_text',params='{}' WHERE id=?`, i.OperationID).Error)
	require.NoError(t, db.Exec(`INSERT INTO operation.operation_input(id,operation_id,seq_no,role,ref_type,text_value) VALUES(?,?,0,'prompt','text','cached prompt') ON CONFLICT(operation_id,seq_no) DO NOTHING`, uuid.New(), i.OperationID).Error)
	results := application.NewManagedResults(&stageTestObjects{entries: map[string]stageTestObject{}}, f.store)
	network := &cachedUsageGeneration{}
	worker := managed.NewActivities(f.store, fixtureUnsealer{}, network, results, nil, nil)
	before := time.Now().UTC()
	got, err := worker.Step(t.Context(), application.ManagedStep{OperationID: i.OperationID, Action: "submit"})
	require.NoError(t, err)
	require.Equal(t, "succeeded", got.State)
	after := time.Now().UTC()
	evidence, err := f.store.ReadManagedResult(t.Context(), i)
	require.NoError(t, err)
	require.NotNil(t, evidence.ReceivedAt)
	require.False(t, evidence.ReceivedAt.Before(before))
	require.False(t, evidence.ReceivedAt.After(after))
	replay, err := worker.Step(t.Context(), application.ManagedStep{OperationID: i.OperationID, Action: "recover"})
	require.NoError(t, err)
	require.Equal(t, got, replay)
	recovered, err := f.store.ReadManagedResult(t.Context(), i)
	require.NoError(t, err)
	require.Equal(t, evidence.ReceivedAt, recovered.ReceivedAt)
	require.Equal(t, 1, network.calls)
}

func TestDeepSeekDispatchRejectsUnreviewedTimePricing(t *testing.T) {
	db := operationStoreDB(t)
	f := seedManagedFixture(t, db)
	require.NoError(t, db.Exec(`UPDATE catalog.provider SET adapter_key='deepseek',region='domestic' WHERE id=?`, f.provider).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.price_rule_version SET unit='per_1k_tokens',rule='{"input_micros_per_1k":2000,"cached_input_micros_per_1k":40,"output_micros_per_1k":8000,"time_pricing":"deepseek_cn_2027"}' WHERE id=?`, f.identity.PriceRuleVersionID).Error)
	claimed, err := f.store.ClaimProviderDispatch(t.Context(), f.identity)
	require.ErrorIs(t, err, application.ErrWorkflowModelUnavailable)
	require.False(t, claimed.Claimed)
	var count int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM operation.provider_call WHERE operation_id=? AND dispatch_started_at IS NOT NULL`, f.identity.OperationID).Scan(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Raw(`SELECT count(*) FROM operation.provider_credential_binding WHERE operation_id=?`, f.identity.OperationID).Scan(&count).Error)
	require.Zero(t, count)
}
