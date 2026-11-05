package operation_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/managed"
	task "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

func operationTestWorker(t *testing.T, db *gorm.DB, id uuid.UUID, h map[string]taskruntime.Handler) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	worker := taskruntime.Worker{Store: taskruntime.NewStore(db).ForBusiness(id.String()), Handlers: h, Concurrency: 4, Poll: 10 * time.Millisecond, Lease: time.Second}
	go func() { done <- worker.Run(ctx) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(10 * time.Second):
				t.Error("operation task worker did not drain")
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

type interruptedTextIngest struct {
	inner *managed.IngestActivities
	calls int
	fail  bool
}

func (i *interruptedTextIngest) Ingest(ctx context.Context, id uuid.UUID) error {
	i.calls++
	if i.fail {
		i.fail = false
		return errors.New("interrupted local ingest")
	}
	return i.inner.Ingest(ctx, id)
}
func (i *interruptedTextIngest) IngestOriginal(ctx context.Context, d application.WorkflowControlDelivery) error {
	return i.inner.IngestOriginal(ctx, d)
}

func TestOperationTaskRuntimePersistsStagesAndRecoversWithoutPaidReplay(t *testing.T) {
	for _, scenario := range []string{"normal", "submit_reply_interrupted", "ingest_interrupted"} {
		t.Run(scenario, func(t *testing.T) {
			database := operationStoreDB(t)
			f := seedManagedFixture(t, database)
			id := f.identity.OperationID
			require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET capability='text.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, f.identity.ModelProfileVersionID).Error)
			require.NoError(t, database.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text_to_text'],queue='provider',supports_cancel=false,supports_query=false,moderation='provider' WHERE id=?`, f.identity.ModelProfileVersionID).Error)
			require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET unit='per_request',rule='{"base_micros":10}' WHERE id=?`, f.identity.PriceRuleVersionID).Error)
			require.NoError(t, database.Exec(`UPDATE operation.operation SET capability='text.generate',mode='text_to_text',params='{}' WHERE id=?`, id).Error)
			results := application.NewManagedResults(&stageTestObjects{entries: map[string]stageTestObject{}}, f.store)
			network := &fixtureGeneration{}
			provider := managed.NewActivities(f.store, fixtureUnsealer{}, network, results, nil)
			ingest := &interruptedTextIngest{inner: managed.NewRoutedIngestActivities(database, nil, results, nil, f.store), fail: scenario == "ingest_interrupted"}
			h := map[string]taskruntime.Handler{}
			task.RegisterFlow(h, f.store, app.NewOperationFinalizer(database, f.store, billingpg.NewStore(database)), nil)
			var interrupted bool
			task.RegisterProvider(h, f.store, managedStepFunc(func(ctx context.Context, in application.ManagedStep) (application.ManagedProgress, error) {
				out, err := provider.Step(ctx, in)
				if scenario == "submit_reply_interrupted" && in.Action == "submit" && !interrupted && err == nil {
					interrupted = true
					return application.ManagedProgress{}, errors.New("lost local step reply")
				}
				return out, err
			}))
			task.RegisterMedia(h, f.store, nil, ingest)
			require.NoError(t, taskruntime.Enqueue(t.Context(), database, "operation.run", id.String(), time.Now()))
			stop := operationTestWorker(t, database, id, h)
			ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
			defer cancel()
			waitForOperationStatus(ctx, t, database, id, "completed")
			stop()
			require.Equal(t, 1, network.calls)
			var row struct{ Settled, Outputs, Submits, Settlements int64 }
			require.NoError(t, database.Raw(`SELECT settled_micros AS settled,(SELECT count(*) FROM operation.operation_output WHERE operation_id=o.id) AS outputs,(SELECT count(*) FROM operation.provider_call WHERE operation_id=o.id AND action='submit') AS submits,(SELECT count(*) FROM billing.ledger_entry WHERE operation_id=o.id AND entry_type='settle') AS settlements FROM operation.operation o WHERE id=?`, id).Scan(&row).Error)
			require.Equal(t, int64(10), row.Settled)
			require.EqualValues(t, 1, row.Outputs)
			require.EqualValues(t, 1, row.Submits)
			require.EqualValues(t, 1, row.Settlements)
			if scenario == "ingest_interrupted" {
				require.Equal(t, 2, ingest.calls)
			}
			if scenario == "submit_reply_interrupted" {
				require.True(t, interrupted)
			}
		})
	}
}
