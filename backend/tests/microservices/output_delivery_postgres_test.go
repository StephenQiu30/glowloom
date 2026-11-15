package microservices_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	billingpg "github.com/StephenQiu30/glowloom-server/backend/execution/billing/adapter/postgres"
	executionpg "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/settlement"
	executionapp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/outputpublication"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func outputTestHash(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = writefence.CanonicalReceipt(raw)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type executionOutputFixture struct {
	*frozenFixture
	store *executionpg.OutputDeliveryStore
	req   executionapp.OutputDeliveryRequest
	ack   contract.OperationOutputReceipt
}

func newExecutionOutputFixture(t *testing.T) *executionOutputFixture {
	t.Helper()
	f := newFrozenFixture(t)
	envelope := f.freeze(t)
	accepted, err := executionpg.NewFrozenInputStore(f.execution, localFrozenSource(f.store)).AcceptFrozenInput(t.Context(), executionKey(envelope))
	if err != nil {
		t.Fatal(err)
	}
	provider, model, version, price, reservation := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	config, credential, release := uuid.New(), uuid.New(), uuid.New()
	providerKey, modelKey := "output-provider-"+provider.String(), "output-model-"+model.String()
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO catalog.provider(id,key,name,adapter_key,region) VALUES(?,?,?,'fixture','domestic')`, []any{provider, providerKey, providerKey}},
		{`INSERT INTO catalog.model_profile(id,model_key,provider_id,capability,display_name) VALUES(?,?,?,'image.generate','output fixture')`, []any{model, modelKey, provider}},
		{`INSERT INTO catalog.model_profile_version(id,model_profile_id,version_no,provider_model_id,modes,limits,param_schema,supports_query,supports_cancel,supports_callback,expected_max_ms) VALUES(?,?,1,'output fixture',ARRAY['text_to_image'],'{}','[]',false,false,false,1000)`, []any{version, model}},
		{`INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,effective_from) VALUES(?,?,1,'per_image','{}',now())`, []any{price, model}},
		{`INSERT INTO catalog.provider_config_version(id,provider_id,version_no,config,content_hash) VALUES(?,?,1,'{}',?)`, []any{config, provider, strings.Repeat("c", 64)}},
		{`INSERT INTO catalog.provider_credential(id,provider_id,label,ciphertext,key_id,last4) VALUES(?,?,'synthetic output fixture','fixture','fixture','0000')`, []any{credential, provider}},
		{`INSERT INTO catalog.model_release(id,model_id,model_version_id,price_version_id,config_version_id,upstream_cost) VALUES(?,?,?,?,?,'{}')`, []any{release, model, version, price, config}},
		{`INSERT INTO operation.operation(id,project_id,target_type,capability,mode,model_profile_version_id,price_rule_version_id,input_hash,origin,status,quote_micros,region,confirmed_by,confirmed_at,provider_request_key) VALUES(?,?,'free','image.generate','text_to_image',?,?,'fixture','canvas','ingesting',2000,'domestic',?,now(),?)`, []any{accepted.OperationID, f.request.ProjectID, version, price, f.request.ActorID, "original/" + accepted.OperationID.String()}},
		{`INSERT INTO operation.provider_binding(operation_id,release_id) VALUES(?,?)`, []any{accepted.OperationID, release}},
		{`INSERT INTO operation.provider_credential_binding(operation_id,credential_id) VALUES(?,?)`, []any{accepted.OperationID, credential}},
		{`INSERT INTO billing.budget(id,project_id,limit_micros,reserved_micros) VALUES(?,?,10000,2000)`, []any{uuid.New(), f.request.ProjectID}},
		{`INSERT INTO billing.reservation(id,project_id,operation_id,amount_micros,status) VALUES(?,?,?,2000,'held')`, []any{reservation, f.request.ProjectID, accepted.OperationID}},
		{`UPDATE operation.operation SET reservation_id=? WHERE id=?`, []any{reservation, accepted.OperationID}},
		{`INSERT INTO infra.task_work(service_owner,kind,business_id,next_run_at,owner,epoch,lease_until,attempt,reference) VALUES('execution','operation.run',?,now(),'output-worker',1,now()+interval '1 hour',1,'null')`, []any{accepted.OperationID.String()}},
	}
	for _, s := range statements {
		if err := f.admin.Exec(s.sql, s.args...).Error; err != nil {
			t.Fatal("output fixture", err)
		}
	}
	result := executionapp.ManagedResult{Identity: executionapp.ProviderDispatchIdentity{ProjectID: f.request.ProjectID, OperationID: accepted.OperationID, Action: "submit", Attempt: 1, RequestKey: "original/" + accepted.OperationID.String(), ModelProfileVersionID: version, PriceRuleVersionID: price}, State: executionapp.ManagedResultSucceeded, Artifacts: []executionapp.ManagedArtifact{{Sequence: 1, Size: 12, MIME: "image/png", SHA256: strings.Repeat("b", 64)}}, Usage: json.RawMessage(`{"output_count":1}`), SupplierUsage: json.RawMessage(`{"original":9007199254740993}`)}
	if err := result.Validate(); err != nil {
		t.Fatal("managed fixture", err)
	}
	raw, _ := json.Marshal(result)
	if err := f.admin.Exec(`INSERT INTO operation.provider_result(operation_id,result) VALUES(?,?::jsonb)`, accepted.OperationID, string(raw)).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.admin.Exec(`INSERT INTO operation.provider_call(id,project_id,operation_id,attempt,action,provider_key,request_summary,response_summary,outcome,cost_micros,region,request_key,model_profile_version_id,price_rule_version_id,dispatch_started_at) VALUES(?,?,?,1,'submit',?,'{"dispatch_contract":"v1"}','{"state":"completed"}','ok',1234,'domestic',?,?,?,now())`, uuid.New(), f.request.ProjectID, accepted.OperationID, providerKey, result.Identity.RequestKey, version, price).Error; err != nil {
		t.Fatal(err)
	}
	mediaFixture := newPinFixture(t)
	if err := f.admin.Exec(`UPDATE media.media_asset SET project_id=? WHERE id=?`, f.request.ProjectID, mediaFixture.acquire.Identity.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	req := executionapp.OutputDeliveryRequest{Command: writefence.Command{ID: uuid.New(), Action: "execution.operation_output.adopt", SHA256: strings.Repeat("0", 64), Authorities: f.request.Command.Authorities}, Publication: contract.Request{Command: writefence.Command{ID: uuid.New(), Action: "media.operation_output.publish", SHA256: strings.Repeat("a", 64), Authorities: f.request.Command.Authorities}, Output: contract.OperationOutputIdentity{OperationID: accepted.OperationID, OutputID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("operation-output/"+accepted.OperationID.String()+"/1")), ProjectID: f.request.ProjectID, ScopeID: f.request.ScopeID, Sequence: 1, Sequences: []int{1}, ByteSize: 12, SHA256: strings.Repeat("b", 64), MIMEType: "image/png", Kind: "image", ProviderKey: providerKey, ModelKey: modelKey, Region: "domestic", AspectRatio: "16:9", Provider: contract.OutputProviderEvidence{RequestKey: result.Identity.RequestKey, Attempt: 1, ManifestSHA256: outputTestHash(t, result), UsageSHA256: outputTestHash(t, result.Usage), SupplierUsageSHA256: outputTestHash(t, result.SupplierUsage), ActualCostMicros: 1234}, Worker: contract.OutputWorkerEvidence{Kind: "operation.run", TaskID: accepted.OperationID, Owner: "output-worker", Epoch: 1}}}}
	req.Command.SHA256 = req.Digest()
	if err := executionapp.ValidateOutputDelivery(req); err != nil {
		t.Fatal(err)
	}
	finalizer := settlement.NewOperationFinalizer(f.execution, executionpg.NewStore(f.execution), billingpg.NewStore(f.execution))
	ack := contract.OperationOutputReceipt{CommandID: req.Publication.Command.ID, RequestSHA256: req.Publication.Command.SHA256, PayloadSHA256: req.Publication.Output.Digest(), Output: req.Publication.Output, AssetID: mediaFixture.acquire.Identity.AssetID, AssetRevision: 1, State: "published", ModerationStatus: "passed", ModerationDetail: json.RawMessage(`{"source":"durable Media receipt fixture"}`)}
	return &executionOutputFixture{f, executionpg.NewOutputDeliveryStore(f.execution, finalizer), req, ack}
}

func TestExecutionOutputPostgresAdoptionSettlesOriginalCostAtomicallyAndReplays(t *testing.T) {
	f := newExecutionOutputFixture(t)
	if _, err := f.store.PrepareOutputDelivery(t.Context(), f.req); err != nil {
		t.Fatal(err)
	}
	started, err := f.store.StartOutputDelivery(t.Context(), f.req)
	if err != nil || !started {
		t.Fatalf("start=%v err=%v", started, err)
	}
	// Reconstruct the owner store as after an Execution process restart.
	finalizer := settlement.NewOperationFinalizer(f.execution, executionpg.NewStore(f.execution), billingpg.NewStore(f.execution))
	recovered := executionpg.NewOutputDeliveryStore(f.execution, finalizer)
	original, err := recovered.ReadOutputDelivery(t.Context(), f.req.Publication.Output.OperationID, 1)
	if err != nil || original == nil || !original.Attempted || original.Receipt != nil {
		t.Fatalf("recovered=%+v err=%v", original, err)
	}
	if err := recovered.AdoptOutputDelivery(t.Context(), original.Request, f.ack); err != nil {
		t.Fatal(err)
	}
	if err := recovered.AdoptOutputDelivery(t.Context(), original.Request, f.ack); err != nil {
		t.Fatal("ACK replay", err)
	}
	var facts struct {
		Status                                   string
		SettledMicros                            int64
		OutputCount, SettleCount, TerminalEvents int
	}
	if err := f.execution.Raw(`SELECT status,settled_micros,(SELECT count(*) FROM operation.operation_output WHERE operation_id=o.id) AS output_count,(SELECT count(*) FROM billing.ledger_entry WHERE operation_id=o.id AND entry_type='settle') AS settle_count,(SELECT count(*) FROM infra.outbox WHERE topic='glowloom.operation.completed.v1' AND payload->'aggregate'->>'id'=o.id::text) AS terminal_events FROM operation.operation o WHERE id=?`, f.req.Publication.Output.OperationID).Scan(&facts).Error; err != nil {
		t.Fatal(err)
	}
	if facts.Status != "completed" || facts.SettledMicros != 1234 || facts.OutputCount != 1 || facts.SettleCount != 1 || facts.TerminalEvents != 1 {
		t.Fatalf("atomic output/billing/terminal facts=%+v", facts)
	}
}

func TestExecutionOutputPostgresRejectsOldEpochAndPreservesPaidCost(t *testing.T) {
	f := newExecutionOutputFixture(t)
	if _, err := f.store.PrepareOutputDelivery(t.Context(), f.req); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.StartOutputDelivery(t.Context(), f.req); err != nil {
		t.Fatal(err)
	}
	if err := f.execution.Exec(`UPDATE infra.task_work SET epoch=epoch+1,owner='replacement-worker' WHERE kind='operation.run' AND business_id=?`, f.req.Publication.Output.OperationID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.store.AdoptOutputDelivery(t.Context(), f.req, f.ack); !errors.Is(err, taskruntime.ErrLeaseLost) {
		t.Fatalf("stale epoch accepted: %v", err)
	}
	var facts struct {
		Status           string
		Cost             int64
		Outputs, Settles int
	}
	if err := f.execution.Raw(`SELECT status,(SELECT sum(cost_micros) FROM operation.provider_call WHERE operation_id=o.id) AS cost,(SELECT count(*) FROM operation.operation_output WHERE operation_id=o.id) AS outputs,(SELECT count(*) FROM billing.ledger_entry WHERE operation_id=o.id AND entry_type='settle') AS settles FROM operation.operation o WHERE id=?`, f.req.Publication.Output.OperationID).Scan(&facts).Error; err != nil {
		t.Fatal(err)
	}
	if facts.Status != "ingesting" || facts.Cost != 1234 || facts.Outputs != 0 || facts.Settles != 0 {
		t.Fatalf("stale epoch changed paid facts: %+v", facts)
	}
}

func TestExecutionOutputPostgresWrongOwnersCannotReadOrWriteLedger(t *testing.T) {
	f := newExecutionOutputFixture(t)
	for _, owner := range []string{"identity", "creation", "media"} {
		t.Run(owner, func(t *testing.T) {
			db := authorityDatabase(t, owner)
			for _, query := range []string{`SELECT * FROM operation.output_delivery LIMIT 0`, `UPDATE operation.output_delivery SET attempted=true WHERE false`, `DELETE FROM operation.output_delivery WHERE false`} {
				if err := db.Exec(query).Error; err == nil {
					t.Fatal("foreign owner statement accepted", query)
				}
			}
			finalizer := settlement.NewOperationFinalizer(f.execution, executionpg.NewStore(f.execution), billingpg.NewStore(f.execution))
			if _, err := executionpg.NewOutputDeliveryStore(db, finalizer).PrepareOutputDelivery(t.Context(), f.req); !errors.Is(err, contract.ErrOutputPublicationOwner) {
				t.Fatalf("owner=%s err=%v", owner, err)
			}
		})
	}
	// The output effect's original command cannot acquire another writer's identity.
	record, err := pgfence.New(f.execution).Load(t.Context(), f.req.Command.ID)
	if err == nil && record.Command.ID != f.req.Command.ID {
		t.Fatal("original command identity changed")
	}
}
