package operation_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"

	"gorm.io/gorm"

	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

func TestBillingBarrierSerializesCanvasTargetDespiteInputChanges(t *testing.T) {
	db := operationStoreDB(t)
	actor, pid := operationStoreProject(t, db)
	quoted, _, _ := seedQuoteCatalog(t, db, pid, time.Now().UTC())
	store := operationStore(db)
	source := domain.CanvasSource{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1}
	encoded, _ := json.Marshal(source)
	ids := []uuid.UUID{quoted.ID, uuid.New()}
	for index, id := range ids {
		item := quotedSnapshotItem(pid, 1)
		item.Operation = quoted
		item.Operation.ID = id
		item.Inputs[0].OperationID = id
		item.Inputs[0].TextValue = billingTextPointer(fmt.Sprintf("different %d", index))
		require.NoError(t, store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{item}))
		require.NoError(t, db.Exec(`UPDATE operation.operation SET source_context=?::jsonb WHERE id=?`, string(encoded), id).Error)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range ids {
		wg.Add(1)
		go func(id uuid.UUID) {
			defer wg.Done()
			_, err := store.ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: pid, OperationID: id, RequestID: uuid.NewString()})
			errs <- err
		}(id)
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		}
	}
	require.Equal(t, 1, success, "two quotes for the same canvas node must not both reserve")
	var held int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM billing.reservation WHERE project_id=? AND status='held'`, pid).Scan(&held).Error)
	require.Equal(t, int64(1), held)
}
func TestRecoverHTTPPreservesOriginalOperationAndReceipt(t *testing.T) {
	db := operationStoreDB(t)
	id, _ := seedProviderCallOperation(t, db)
	var actor identityapp.Principal
	var op struct{ ProjectID uuid.UUID }
	require.NoError(t, db.Raw(`SELECT project_id FROM operation.operation WHERE id=?`, id).Scan(&op).Error)
	pid := op.ProjectID
	require.NoError(t, db.Raw(`SELECT u.id,u.org_id,u.role FROM identity."user" u JOIN operation.operation o ON o.confirmed_by=u.id WHERE o.id=?`, id).Scan(&actor).Error)
	actor = identityfixture.Session(t, db, actor)
	router := publicOperationRouter(actor, db)
	body := fmt.Sprintf(`{"project_id":%q}`, pid)
	path := "/api/operations/" + id.String() + "/recover"
	res := publicOperationRequest(t, router, "POST", path, body, uuid.NewString())
	require.Equal(t, 409, res.Code, res.Body.String())
	require.NoError(t, db.Exec(`UPDATE operation.operation SET status='manual' WHERE id=?`, id).Error)
	require.NoError(t, db.Exec(`INSERT INTO operation.provider_call(id,project_id,operation_id,provider_key,action,attempt,region,outcome,provider_task_id,response_summary,request_summary) VALUES(?,?,?,'mock','submit',1,'domestic','ok','original-task','{"state":"accepted"}','{}')`, uuid.New(), pid, id).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET supports_query=false WHERE id=(SELECT model_profile_version_id FROM operation.operation WHERE id=?)`, id).Error)
	res = publicOperationRequest(t, router, "POST", path, body, uuid.NewString())
	require.Equal(t, 409, res.Code)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET supports_query=true WHERE id=(SELECT model_profile_version_id FROM operation.operation WHERE id=?)`, id).Error)
	key := uuid.NewString()
	res = publicOperationRequest(t, router, "POST", path, body, key)
	require.Equal(t, 202, res.Code, res.Body.String())
	replay := publicOperationRequest(t, router, "POST", path, body, key)
	require.Equal(t, 202, replay.Code)
	require.JSONEq(t, res.Body.String(), replay.Body.String())
	var count int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM infra.outbox WHERE topic='glowloom.workflow.control_requested.v1' AND payload->'aggregate'->>'id'=?`, id.String()).Scan(&count).Error)
	require.Equal(t, int64(1), count)
	detail := publicOperationRequest(t, router, "GET", "/api/operations/"+id.String()+"?project_id="+pid.String(), "", "")
	require.Equal(t, 200, detail.Code)
	require.Contains(t, detail.Body.String(), `"charge_state":"pending"`)
	require.NotContains(t, detail.Body.String(), "original-task")
	require.Contains(t, detail.Body.String(), `"cancel_requested":false`)
	other, _ := operationStoreProject(t, db)
	res = publicOperationRequest(t, publicOperationRouter(other, db), "POST", path, body, uuid.NewString())
	require.Equal(t, 404, res.Code)
}
func billingTextPointer(s string) *string { return &s }

func TestBillingBarrierTerminalEvidenceAndNormalCompletedCounterexample(t *testing.T) {
	db := operationStoreDB(t)
	actor, pid := operationStoreProject(t, db)
	quoted, _, _ := seedQuoteCatalog(t, db, pid, time.Now().UTC())
	store := operationStore(db)
	source := domain.CanvasSource{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1}
	encoded, _ := json.Marshal(source)
	first := quotedSnapshotItem(pid, 1)
	first.Operation = quoted
	first.Inputs[0].OperationID = quoted.ID
	require.NoError(t, store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{first}))
	require.NoError(t, db.Exec(`UPDATE operation.operation SET source_context=?::jsonb WHERE id=?`, string(encoded), quoted.ID).Error)
	result, err := store.ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: pid, OperationID: quoted.ID, RequestID: uuid.NewString()})
	require.NoError(t, err)
	second := first
	second.Operation.ID = uuid.New()
	second.Inputs = append([]domain.OperationInput(nil), first.Inputs...)
	second.Inputs[0].ID = uuid.New()
	second.Inputs[0].OperationID = second.Operation.ID
	require.NoError(t, store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{second}))
	require.NoError(t, db.Exec(`UPDATE operation.operation SET source_context=?::jsonb WHERE id=?`, string(encoded), second.Operation.ID).Error)
	next := application.ConfirmSingleQuoteInput{ProjectID: pid, OperationID: second.Operation.ID, RequestID: uuid.NewString()}
	// A terminal label does not close a held reservation or prove free execution.
	require.NoError(t, db.Exec(`UPDATE operation.operation SET status='failed' WHERE id=?`, quoted.ID).Error)
	_, err = store.ConfirmSingleQuote(t.Context(), actor, next)
	require.ErrorIs(t, err, application.ErrPendingConsumption)
	// Closed reservation without its immutable settle fact still blocks.
	require.NoError(t, db.Exec(`UPDATE billing.reservation SET status='settled',closed_at=now() WHERE id=?`, result.ReservationID).Error)
	_, err = store.ConfirmSingleQuote(t.Context(), actor, next)
	require.ErrorIs(t, err, application.ErrPendingConsumption)
	require.NoError(t, db.Exec(`INSERT INTO billing.ledger_entry(id,project_id,operation_id,entry_type,amount_micros) VALUES(?,?,?,'settle',1)`, uuid.New(), pid, quoted.ID).Error)
	_, err = store.ConfirmSingleQuote(t.Context(), actor, next)
	require.ErrorIs(t, err, application.ErrPendingConsumption, "paid with unknown delivery")
	// Normal completed and archived output permits a separately confirmed quote.
	require.NoError(t, db.Exec(`UPDATE operation.operation SET status='completed',settled_micros=1 WHERE id=?`, quoted.ID).Error)
	require.NoError(t, db.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,json_payload,moderation_status) VALUES(?,?,?,1,'json','{"text":"archived fixture"}','passed')`, uuid.New(), pid, quoted.ID).Error)
	_, err = store.ConfirmSingleQuote(t.Context(), actor, next)
	require.NoError(t, err)
}

func TestCatalogPricingPreviewKeepsMissingPriceUnknownWithoutCreatingOperation(t *testing.T) {
	db := operationStoreDB(t)
	actor, pid := operationStoreProject(t, db)
	quoted, model, _ := seedQuoteCatalog(t, db, pid, time.Now().UTC())
	setPublicQuoteLimits(t, db, quoted.ModelProfileVersionID)
	router := publicOperationRouter(actor, db)
	path := "/api/models?project_id=" + pid.String() + "&capability=" + quoted.Capability
	known := publicOperationRequest(t, router, "GET", path, "", "")
	require.Equal(t, 200, known.Code)
	require.Contains(t, known.Body.String(), `"amount_kind":"unit_only"`)
	require.Contains(t, known.Body.String(), `"amount_micros":null`)
	require.Contains(t, known.Body.String(), `"confirmable":false`)
	require.NoError(t, db.Exec(`UPDATE catalog.price_rule_version SET is_delete=true WHERE id=?`, quoted.PriceRuleVersionID).Error)
	missing := publicOperationRequest(t, router, "GET", path, "", "")
	require.Equal(t, 200, missing.Code)
	require.Contains(t, missing.Body.String(), `"amount_kind":"unknown"`)
	require.NotContains(t, missing.Body.String(), `"amount_micros":0`)
	body := fmt.Sprintf(`{"model_key":"quote-model-%s","capability":%q,"mode":"text_to_image","prompt":"fixture prompt","params":{},"output_count":1}`, model, quoted.Capability)
	rejected := publicOperationRequest(t, router, "POST", "/api/projects/"+pid.String()+"/free-operations", body, uuid.NewString())
	require.NotEqual(t, 201, rejected.Code)
	var count int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM operation.operation WHERE project_id=?`, pid).Scan(&count).Error)
	require.Equal(t, int64(0), count, "catalog unknown must not create a zero-priced quote")
}

func TestBillingConflictPersistentFactsVetoEveryPaidAdmission(t *testing.T) {
	for _, scenario := range []string{"provider_released", "provider_pending", "provider_refunded", "released_provider_settled", "legacy_rejected_zero", "operation_amount", "reservation_amount", "release_amount"} {
		t.Run(scenario, func(t *testing.T) {
			db := operationStoreDB(t)
			actor, pid, batch, items := seedConfirmableBatch(t, db, 1)
			store := operationStore(db)
			old := items[0]
			old.Operation.ID = uuid.New()
			old.Operation.BatchID = nil
			old.Inputs = append([]domain.OperationInput(nil), old.Inputs...)
			old.Inputs[0].ID = uuid.New()
			old.Inputs[0].OperationID = old.Operation.ID
			require.NoError(t, store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{old}))
			_, err := store.ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: pid, OperationID: old.Operation.ID, RequestID: uuid.NewString()})
			require.NoError(t, err)
			actual := int64(1)
			if scenario == "released_provider_settled" || scenario == "legacy_rejected_zero" {
				actual = 0
			}
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				_, err := billingpg.NewStore(tx).SettleInTransaction(t.Context(), tx, billingapp.SettleInput{ProjectID: pid, OperationID: old.Operation.ID, ActualCostMicros: actual, Capability: old.Operation.Capability, OccurredAt: time.Now()})
				if err != nil {
					return err
				}
				return tx.Exec(`UPDATE operation.operation SET status='completed',settled_micros=? WHERE id=?`, actual, old.Operation.ID).Error
			}))
			source := domain.CanvasSource{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1}
			encoded, _ := json.Marshal(source)
			require.NoError(t, db.Exec(`INSERT INTO canvas.document(id,project_id,name,revision) VALUES(?,?,'billing source fixture',1)`, source.CanvasID, pid).Error)
			require.NoError(t, db.Exec(`INSERT INTO canvas.node(id,document_id,node_type,config,x,y) VALUES(?,?,'note','{}',0,0)`, source.NodeID, source.CanvasID).Error)
			require.NoError(t, db.Exec(`UPDATE operation.operation SET source_context=?::jsonb WHERE id IN ?`, string(encoded), []uuid.UUID{old.Operation.ID, items[0].Operation.ID}).Error)
			require.NoError(t, db.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,json_payload,moderation_status) VALUES(?,?,?,1,'json','{"text":"complete fixture"}','passed')`, uuid.New(), pid, old.Operation.ID).Error)
			single := items[0]
			single.Operation.ID = uuid.New()
			single.Operation.BatchID = nil
			single.Inputs = append([]domain.OperationInput(nil), single.Inputs...)
			single.Inputs[0].ID = uuid.New()
			single.Inputs[0].OperationID = single.Operation.ID
			require.NoError(t, store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{single}))
			require.NoError(t, db.Exec(`UPDATE operation.operation SET source_context=?::jsonb WHERE id=?`, string(encoded), single.Operation.ID).Error)
			switch scenario {
			case "legacy_rejected_zero":
				require.NoError(t, db.Exec(`INSERT INTO operation.provider_call(id,project_id,operation_id,provider_key,action,attempt,region,outcome,response_summary,request_summary,cost_micros,dispatch_started_at)
 VALUES(?,?,?,'historical_cloud','submit',1,'overseas','error','{"state":"rejected"}','{}',0,now())`, uuid.New(), pid, old.Operation.ID).Error)
			case "operation_amount":
				require.NoError(t, db.Exec(`UPDATE operation.operation SET settled_micros=2 WHERE id=?`, old.Operation.ID).Error)
			case "reservation_amount":
				require.NoError(t, db.Exec(`UPDATE billing.reservation SET amount_micros=2 WHERE operation_id=?`, old.Operation.ID).Error)
			case "release_amount":
				require.NoError(t, db.Exec(`INSERT INTO billing.ledger_entry(id,project_id,operation_id,entry_type,amount_micros) VALUES(?,?,?,'release',1)`, uuid.New(), pid, old.Operation.ID).Error)
			default:
				state := strings.TrimPrefix(scenario, "provider_")
				if scenario == "released_provider_settled" {
					state = "settled"
				}
				summary := fmt.Sprintf(`{"state":"succeeded","settlement_state":%q}`, state)
				require.NoError(t, db.Exec(`INSERT INTO operation.provider_call(id,project_id,operation_id,provider_key,action,attempt,region,outcome,request_summary,response_summary) VALUES(?,?,?,'mock','query',1,'domestic','ok','{}',?::jsonb)`, uuid.New(), pid, old.Operation.ID, summary).Error)
			}
			detail, err := store.ReadPublicTask(t.Context(), actor, pid, old.Operation.ID)
			require.NoError(t, err)
			require.True(t, detail.BillingOutcome.ReconciliationRequired)
			require.Equal(t, "blocked", detail.BillingOutcome.Regenerate)
			assertPaidAdmissionBlocked(t, db, actor, pid, batch, single, source)
		})
	}
}

func assertPaidAdmissionBlocked(t *testing.T, db *gorm.DB, actor identityapp.Principal, pid, batch uuid.UUID, single pgoperation.QuoteItem, source domain.CanvasSource) {
	t.Helper()
	store := operationStore(db)
	encoded, err := json.Marshal(source)
	require.NoError(t, err)
	var model struct{ Key string }
	require.NoError(t, db.Raw(`SELECT m.model_key AS key FROM catalog.model_profile m JOIN catalog.model_profile_version v ON v.model_profile_id=m.id WHERE v.id=?`, single.Operation.ModelProfileVersionID).Scan(&model).Error)
	_, err = store.CreateFreeQuote(t.Context(), actor, application.CreateFreeQuoteInput{ProjectID: pid, RequestID: uuid.NewString(), FreeQuoteItemInput: application.FreeQuoteItemInput{ModelKey: model.Key, Capability: single.Operation.Capability, Mode: single.Operation.Mode, Prompt: "new independent quote", Params: json.RawMessage(`{}`), OutputCount: 1, Source: &source}})
	require.ErrorIs(t, err, application.ErrPendingConsumption, "new quote")
	_, err = store.ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: pid, OperationID: single.Operation.ID, RequestID: uuid.NewString()})
	require.ErrorIs(t, err, application.ErrPendingConsumption, "single confirmation")
	_, err = store.ConfirmBatchQuote(t.Context(), actor, application.ConfirmBatchQuoteInput{ProjectID: pid, BatchID: batch, RequestID: uuid.NewString()})
	require.ErrorIs(t, err, application.ErrPendingConsumption, "batch confirmation")
	dispatch := single
	dispatch.Operation.ID = uuid.New()
	dispatch.Inputs = append([]domain.OperationInput(nil), single.Inputs...)
	dispatch.Inputs[0].ID = uuid.New()
	dispatch.Inputs[0].OperationID = dispatch.Operation.ID
	require.NoError(t, store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{dispatch}))
	_, err = store.ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: pid, OperationID: dispatch.Operation.ID, RequestID: uuid.NewString()})
	require.NoError(t, err)
	var key string
	require.NoError(t, db.Raw(`SELECT provider_request_key FROM operation.operation WHERE id=?`, dispatch.Operation.ID).Scan(&key).Error)
	_, err = store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: dispatch.Operation.ID, From: []domain.Status{domain.StatusConfirmed}, To: domain.StatusSubmitting, ProviderRequestKey: &key})
	require.NoError(t, err)
	require.NoError(t, store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{OperationID: dispatch.Operation.ID, Action: "submit", Attempt: 1, DispatchRequired: true}))
	require.NoError(t, db.Exec(`UPDATE operation.operation SET source_context=?::jsonb WHERE id=?`, string(encoded), dispatch.Operation.ID).Error)
	_, err = store.ClaimProviderDispatch(t.Context(), application.ProviderDispatchIdentity{ProjectID: pid, OperationID: dispatch.Operation.ID, ModelProfileVersionID: *dispatch.Operation.ModelProfileVersionID, PriceRuleVersionID: *dispatch.Operation.PriceRuleVersionID, Action: "submit", Attempt: 1, RequestKey: key})
	require.ErrorIs(t, err, application.ErrPendingConsumption, "actual dispatch")
}

func TestBillingClosedLedgerAllowsNormalPartialReleaseAndExistingOverage(t *testing.T) {
	for _, actual := range []int64{10, 4, 15} {
		t.Run(fmt.Sprint(actual), func(t *testing.T) {
			db := operationStoreDB(t)
			actor, pid, _, items := seedConfirmableBatch(t, db, 10)
			store := operationStore(db)
			id := items[0].Operation.ID
			// Exercise a pre-migration reservation, including the historical uncapped path.
			require.NoError(t, db.Exec(`UPDATE operation.operation SET batch_id=NULL WHERE id=?`, id).Error)
			_, err := store.ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: pid, OperationID: id, RequestID: uuid.NewString()})
			require.NoError(t, err)
			require.NoError(t, db.Exec(`UPDATE billing.reservation SET cap_at_quote=false WHERE operation_id=?`, id).Error)
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				_, err := billingpg.NewStore(tx).SettleInTransaction(t.Context(), tx, billingapp.SettleInput{ProjectID: pid, OperationID: id, ActualCostMicros: actual, Capability: items[0].Operation.Capability, OccurredAt: time.Now()})
				if err != nil {
					return err
				}
				return tx.Exec(`UPDATE operation.operation SET status='completed',settled_micros=? WHERE id=?`, actual, id).Error
			}))
			require.NoError(t, db.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,json_payload,moderation_status) VALUES(?,?,?,1,'json','{"text":"archived fixture"}','passed')`, uuid.New(), pid, id).Error)
			source := domain.CanvasSource{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1}
			encoded, _ := json.Marshal(source)
			require.NoError(t, db.Exec(`UPDATE operation.operation SET source_context=?::jsonb WHERE id=?`, string(encoded), id).Error)
			detail, err := store.ReadPublicTask(t.Context(), actor, pid, id)
			require.NoError(t, err)
			require.Equal(t, "settled", detail.BillingOutcome.ChargeState)
			require.False(t, detail.BillingOutcome.ReconciliationRequired)
			require.Equal(t, "confirm", detail.BillingOutcome.Regenerate)
			require.Equal(t, actual, *detail.BillingOutcome.ChargedMicros)
			var releases int64
			require.NoError(t, db.Raw(`SELECT count(*) FROM billing.ledger_entry WHERE operation_id=? AND entry_type='release' AND NOT is_delete`, id).Scan(&releases).Error)
			if actual < 10 {
				require.EqualValues(t, 1, releases)
				require.EqualValues(t, 10-actual, *detail.BillingOutcome.ReleasedMicros)
			} else {
				require.Zero(t, releases)
			}
			next := items[0]
			next.Operation.ID = uuid.New()
			next.Operation.BatchID = nil
			next.Inputs = append([]domain.OperationInput(nil), next.Inputs...)
			next.Inputs[0].ID = uuid.New()
			next.Inputs[0].OperationID = next.Operation.ID
			require.NoError(t, store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{next}))
			require.NoError(t, db.Exec(`UPDATE operation.operation SET source_context=?::jsonb WHERE id=?`, string(encoded), next.Operation.ID).Error)
			_, err = store.ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: pid, OperationID: next.Operation.ID, RequestID: uuid.NewString()})
			require.NoError(t, err)
		})
	}
}
