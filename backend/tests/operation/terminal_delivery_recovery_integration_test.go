package operation_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"gorm.io/gorm"

	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/managed"

	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/testsuite"
	"google.golang.org/grpc"

	inboxpg "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/adapter/postgres"
	inbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	operationevent "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/event"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

type originalDeliveryStarter struct {
	options client.StartWorkflowOptions
	input   application.WorkflowControlDelivery
	starts  int
}

func (s *originalDeliveryStarter) SignalWorkflow(context.Context, string, string, string, interface{}) error {
	return fmt.Errorf("terminal recovery must not signal original generation workflow")
}
func (s *originalDeliveryStarter) ExecuteWorkflow(_ context.Context, options client.StartWorkflowOptions, name interface{}, args ...interface{}) (client.WorkflowRun, error) {
	s.starts++
	s.options = options
	s.input = args[0].(application.WorkflowControlDelivery)
	if name != "OriginalDeliveryRecoveryWorkflow" {
		return nil, fmt.Errorf("unexpected workflow")
	}
	if s.starts > 1 {
		return nil, serviceerror.NewWorkflowExecutionAlreadyStarted("fixture replay", options.ID, "fixture-run")
	}
	return nil, nil
}

func TestTerminalPaidManifestRestoresOriginalDeliveryWithoutNewGeneration(t *testing.T) {
	for _, tc := range []struct {
		name  string
		price int
	}{{"partial_release", 4}, {"overage", 16}} {
		t.Run(tc.name, func(t *testing.T) {
			db := operationStoreDB(t)
			f := seedManagedFixture(t, db)
			i, store := f.identity, f.store
			require.NoError(t, db.Exec(`UPDATE catalog.model_profile SET capability='text.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, i.ModelProfileVersionID).Error)
			require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text_to_text'],queue='provider',supports_cancel=false,supports_query=false,moderation='provider' WHERE id=?`, i.ModelProfileVersionID).Error)
			require.NoError(t, db.Exec(`UPDATE catalog.price_rule_version SET unit='per_request',rule=jsonb_build_object('base_micros',?::int) WHERE id=?`, tc.price, i.PriceRuleVersionID).Error)
			require.NoError(t, db.Exec(`UPDATE operation.operation SET capability='text.generate',mode='text_to_text',params='{}' WHERE id=?`, i.OperationID).Error)
			require.NoError(t, db.Exec(`INSERT INTO operation.operation_input(id,operation_id,seq_no,role,ref_type,text_value) VALUES(?,?,0,'prompt','text','original fixture prompt') ON CONFLICT(operation_id,seq_no) DO NOTHING`, uuid.New(), i.OperationID).Error)
			objects := &stageTestObjects{entries: map[string]stageTestObject{}}
			results := application.NewManagedResults(objects, store)
			network := &fixtureGeneration{}
			worker := managed.NewActivities(store, fixtureUnsealer{}, network, results, nil, nil)
			_, err := worker.Step(t.Context(), application.ManagedStep{OperationID: i.OperationID, Action: "submit"})
			require.NoError(t, err)
			require.Equal(t, 1, network.calls)
			require.NoError(t, store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{OperationID: i.OperationID, Action: "submit", Attempt: 1, Outcome: "ok", State: "completed", Managed: true}))
			_, err = store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusSucceeded})
			require.NoError(t, err)
			_, err = store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSucceeded}, To: domain.StatusIngesting})
			require.NoError(t, err)
			finalizer := app.NewOperationFinalizer(db, store, billingpg.NewStore(db))
			require.NoError(t, db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=(SELECT confirmed_by FROM operation.operation WHERE id=?)`, i.OperationID).Error)
			require.NoError(t, finalizer.ResolveRevokedOperation(t.Context(), i.OperationID.String()))
			require.NoError(t, db.Exec(`UPDATE identity."user" SET status='active' WHERE id=(SELECT confirmed_by FROM operation.operation WHERE id=?)`, i.OperationID).Error)

			var actor identityapp.Principal
			require.NoError(t, db.Raw(`SELECT u.id,u.org_id,u.role FROM identity."user" u JOIN operation.operation o ON o.confirmed_by=u.id WHERE o.id=?`, i.OperationID).Scan(&actor).Error)
			actor = identityfixture.Session(t, db, actor)
			before, err := readDispatchBudget(context.Background(), db, i.OperationID)
			require.NoError(t, err)
			router := publicOperationRouter(actor, db)
			key := uuid.NewString()
			path := "/api/operations/" + i.OperationID.String() + "/recover"
			body := fmt.Sprintf(`{"project_id":%q}`, i.ProjectID)
			res := publicOperationRequest(t, router, "POST", path, body, key)
			require.Equal(t, 202, res.Code, res.Body.String())
			var event struct {
				Topic, PartitionKey string
				Payload             []byte
			}
			require.NoError(t, db.Raw(`SELECT topic,partition_key,payload FROM infra.outbox WHERE topic=? AND payload->'data'->>'request_id'=?`, operationevent.WorkflowControlTopic, key).Scan(&event).Error)
			starter := &originalDeliveryStarter{}
			control := operationflow.NewControl(starter, store)
			handler := operationevent.NewWorkflowControlHandler(inboxpg.NewStore(db), store, control)
			record := inbox.Record{Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload}
			require.NoError(t, handler.Handle(t.Context(), record))
			require.NoError(t, handler.Handle(t.Context(), record))
			require.Equal(t, 1, starter.starts)
			require.Equal(t, "operation-delivery/"+i.OperationID.String(), starter.options.ID)
			require.Error(t, control.SignalControl(t.Context(), starter.input)) // already running/succeeded is idempotent
			missingStage := application.NewManagedResults(&stageTestObjects{entries: map[string]stageTestObject{}}, store)
			missingIngest := managed.NewScopedIngestActivities(db, nil, missingStage, app.NewMediaWorkerScopeAccess, store)
			require.Error(t, missingIngest.IngestOriginal(t.Context(), starter.input), "terminal delivery must verify private staged manifest")
			ingest := managed.NewScopedIngestActivities(db, nil, results, app.NewMediaWorkerScopeAccess, store)
			env := (&testsuite.WorkflowTestSuite{}).NewTestWorkflowEnvironment()
			env.SetStartWorkflowOptions(starter.options)
			env.RegisterActivityWithOptions(ingest.IngestOriginal, activity.RegisterOptions{Name: "media.IngestOriginal"})
			env.SetTestTimeout(10 * time.Second)
			env.ExecuteWorkflow(operationflow.OriginalDeliveryRecoveryWorkflow, starter.input)
			require.NoError(t, env.GetWorkflowError())
			require.NoError(t, ingest.IngestOriginal(t.Context(), starter.input))
			detail := publicOperationRequest(t, router, "GET", fmt.Sprintf("/api/operations/%s?project_id=%s", i.OperationID, i.ProjectID), "", "")
			require.Equal(t, 200, detail.Code, detail.Body.String())
			require.Contains(t, detail.Body.String(), `"status":"failed"`)
			require.Contains(t, detail.Body.String(), `"delivery_state":"ready"`)
			require.Contains(t, detail.Body.String(), fmt.Sprintf(`"charged_micros":%d`, tc.price))
			require.Contains(t, detail.Body.String(), "fixture generated content")
			require.NotContains(t, detail.Body.String(), "fixture-only-not-real")
			noop := publicOperationRequest(t, router, "POST", path, body, uuid.NewString())
			require.Equal(t, 202, noop.Code)
			require.Contains(t, noop.Body.String(), `"accepted":false`)
			after, err := readDispatchBudget(context.Background(), db, i.OperationID)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, 1, network.calls)
		})
	}
}

// A loopback S3 wire keeps actual media ingest/probe/renderer/PG boundaries in
// this fixture. It never contacts an object-storage service.
func originalDeliveryObjects(t *testing.T, second uuid.UUID) (*objectstorage.Client, func(func() error)) {
	t.Helper()
	var mu sync.Mutex
	entries := map[string]stageTestObject{}
	interrupted := true
	var afterWrite func() error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		key := r.URL.Path
		if r.Method == http.MethodPut {
			if interrupted && strings.Contains(key, second.String()) {
				http.Error(w, "fixture second transfer interrupted", http.StatusForbidden)
				return
			}
			if _, exists := entries[key]; exists {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			data, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(400)
				return
			}
			if r.Header.Get("X-Amz-Decoded-Content-Length") != "" {
				reader := bufio.NewReader(bytes.NewReader(data))
				var decoded bytes.Buffer
				for {
					line, e := reader.ReadString('\n')
					if e != nil {
						w.WriteHeader(400)
						return
					}
					size, e := strconv.ParseInt(strings.Split(strings.TrimSpace(line), ";")[0], 16, 64)
					if e != nil {
						w.WriteHeader(400)
						return
					}
					if size == 0 {
						break
					}
					if _, e = io.CopyN(&decoded, reader, size); e != nil {
						w.WriteHeader(400)
						return
					}
					_, _ = reader.ReadString('\n')
				}
				data = decoded.Bytes()
			}
			// MinIO signs streaming request bodies; the decoded content length is the
			// authoritative object length, while checksums remain real SDK metadata.
			entries[key] = stageTestObject{data: data, info: application.ProviderStagedObjectInfo{Size: int64(len(data)), ContentType: r.Header.Get("Content-Type"), SHA256: r.Header.Get("X-Amz-Meta-Sha256")}}
			if afterWrite != nil && strings.Contains(key, second.String()) {
				hook := afterWrite
				afterWrite = nil
				if err := hook(); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
			}
			w.Header().Set("ETag", `"fixture"`)
			return
		}
		value, found := entries[key]
		if !found {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Length", strconv.FormatInt(value.info.Size, 10))
		w.Header().Set("Content-Type", value.info.ContentType)
		w.Header().Set("X-Amz-Meta-Sha256", value.info.SHA256)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("ETag", `"fixture"`)
		if r.Method == http.MethodGet {
			_, _ = w.Write(value.data)
		}
	}))
	t.Cleanup(server.Close)
	objects, err := objectstorage.Open(server.URL, "original-delivery-fixture", "fixture", "fixture", "us-east-1")
	require.NoError(t, err)
	return objects, func(hook func() error) { mu.Lock(); interrupted = false; afterWrite = hook; mu.Unlock() }
}

type originalImageGeneration struct {
	fixtureGeneration
	images []application.GeneratedImage
}

func (f *originalImageGeneration) Submit(context.Context, application.GenerationRequest, string) (application.GenerationResult, error) {
	f.calls++
	return application.GenerationResult{Outcome: "completed", State: "succeeded", Images: f.images}, nil
}

func TestTerminalMultiArtifactDeliveryKeepsPartialRecoverableUntilAllArchived(t *testing.T) {
	db := operationStoreDB(t)
	f := seedManagedFixture(t, db)
	i, store := f.identity, f.store
	require.NoError(t, db.Exec(`UPDATE billing.budget SET limit_micros=100000 WHERE project_id=?`, i.ProjectID).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET moderation='provider' WHERE id=?`, i.ModelProfileVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE operation.operation SET output_count=2 WHERE id=?`, i.OperationID).Error)
	source := domain.CanvasSource{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1}
	encoded, _ := json.Marshal(source)
	require.NoError(t, db.Exec(`UPDATE operation.operation SET source_context=?::jsonb WHERE id=?`, string(encoded), i.OperationID).Error)
	var images []application.GeneratedImage
	for index := 0; index < 2; index++ {
		img := image.NewRGBA(image.Rect(0, 0, 64, 48))
		img.Set(index, 0, color.RGBA{R: uint8(100 + index), A: 255})
		var b bytes.Buffer
		require.NoError(t, png.Encode(&b, img))
		images = append(images, application.GeneratedImage{Bytes: b.Bytes(), MediaType: "image/png"})
	}
	results := application.NewManagedResults(&stageTestObjects{entries: map[string]stageTestObject{}}, store)
	network := &originalImageGeneration{images: images}
	worker := managed.NewActivities(store, fixtureUnsealer{}, network, results, nil, nil)
	_, err := worker.Step(t.Context(), application.ManagedStep{OperationID: i.OperationID, Action: "submit"})
	require.NoError(t, err)
	require.Equal(t, 1, network.calls)
	require.NoError(t, store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{OperationID: i.OperationID, Action: "submit", Attempt: 1, Outcome: "ok", State: "completed", Managed: true}))
	_, err = store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusSucceeded})
	require.NoError(t, err)
	_, err = store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSucceeded}, To: domain.StatusIngesting})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=(SELECT confirmed_by FROM operation.operation WHERE id=?)`, i.OperationID).Error)
	require.NoError(t, app.NewOperationFinalizer(db, store, billingpg.NewStore(db)).ResolveRevokedOperation(t.Context(), i.OperationID.String()))
	require.NoError(t, db.Exec(`UPDATE identity."user" SET status='active' WHERE id=(SELECT confirmed_by FROM operation.operation WHERE id=?)`, i.OperationID).Error)
	var actor identityapp.Principal
	require.NoError(t, db.Raw(`SELECT u.id,u.org_id,u.role FROM identity."user" u JOIN operation.operation o ON o.confirmed_by=u.id WHERE o.id=?`, i.OperationID).Scan(&actor).Error)
	actor = identityfixture.Session(t, db, actor)
	second := uuid.NewSHA1(uuid.NameSpaceOID, []byte("media-asset/"+i.OperationID.String()+"/2"))
	objects, resume := originalDeliveryObjects(t, second)
	ingest := managed.NewScopedIngestActivities(db, objects, results, app.NewMediaWorkerScopeAccess, store)
	before, err := readDispatchBudget(t.Context(), db, i.OperationID)
	require.NoError(t, err)
	receipt, err := store.RequestWorkflowControl(t.Context(), actor, application.WorkflowControlInput{ProjectID: i.ProjectID, TargetID: i.OperationID, TargetType: "operation", Action: "recover", RequestID: uuid.NewString()})
	require.NoError(t, err)
	require.True(t, receipt.Accepted)
	delivery := application.WorkflowControlDelivery{EventID: receipt.EventID, RequestID: receipt.RequestID, ActorID: actor.ID, OrgID: actor.OrgID, ProjectID: i.ProjectID, TargetID: i.OperationID, TargetType: "operation", Action: "recover"}
	err = ingest.IngestOriginal(t.Context(), delivery)
	require.Error(t, err, "second transfer interrupts after first output archive")
	t.Logf("interrupted ingest: %v", err)
	var count int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM operation.operation_output WHERE operation_id=? AND moderation_status='passed' AND NOT is_delete`, i.OperationID).Scan(&count).Error)
	require.EqualValues(t, 1, count)
	detail, err := store.ReadPublicTask(t.Context(), actor, i.ProjectID, i.OperationID)
	require.NoError(t, err)
	require.Equal(t, "recoverable", detail.BillingOutcome.DeliveryState)
	require.Equal(t, "blocked", detail.BillingOutcome.Regenerate)
	quoted, _, _ := seedQuoteCatalog(t, db, i.ProjectID, time.Now().UTC())
	item := quotedSnapshotItem(i.ProjectID, 1)
	item.Operation = quoted
	item.Inputs[0].OperationID = quoted.ID
	require.NoError(t, store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{item}))
	require.NoError(t, db.Exec(`UPDATE operation.operation SET source_context=?::jsonb WHERE id=?`, string(encoded), quoted.ID).Error)
	_, err = store.ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: i.ProjectID, OperationID: quoted.ID, RequestID: uuid.NewString()})
	require.ErrorIs(t, err, application.ErrPendingConsumption)
	// Evidence changes after private object transfer but before the second
	// output write. The media transaction must see the conflict itself.
	resume(func() error {
		return db.Exec(`UPDATE billing.reservation SET amount_micros=amount_micros+1 WHERE operation_id=?`, i.OperationID).Error
	})
	require.Error(t, ingest.IngestOriginal(t.Context(), delivery))
	require.NoError(t, db.Raw(`SELECT count(*) FROM operation.operation_output WHERE operation_id=?`, i.OperationID).Scan(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Exec(`UPDATE billing.reservation SET amount_micros=amount_micros-1 WHERE operation_id=?`, i.OperationID).Error)
	// An older pending observation must not permanently veto a newer settlement.
	require.NoError(t, db.Exec(`UPDATE operation.provider_call SET response_summary=response_summary||'{"settlement_state":"pending"}'::jsonb WHERE operation_id=? AND action='submit'`, i.OperationID).Error)
	require.Error(t, ingest.IngestOriginal(t.Context(), delivery))
	require.NoError(t, db.Exec(`INSERT INTO operation.provider_call(id,project_id,operation_id,attempt,action,provider_key,request_summary,response_summary,outcome,region,create_time)
 SELECT ?,project_id,operation_id,1,'query',provider_key,'{}','{"settlement_state":"settled"}','ok',region,create_time+interval '1 second' FROM operation.provider_call WHERE operation_id=? AND action='submit'`, uuid.New(), i.OperationID).Error)

	starter := &originalDeliveryStarter{}
	control := operationflow.NewControl(starter, store)
	require.NoError(t, control.SignalControl(t.Context(), delivery))
	env := (&testsuite.WorkflowTestSuite{}).NewTestWorkflowEnvironment()
	env.SetStartWorkflowOptions(starter.options)
	env.RegisterActivityWithOptions(ingest.IngestOriginal, activity.RegisterOptions{Name: "media.IngestOriginal"})
	env.ExecuteWorkflow(operationflow.OriginalDeliveryRecoveryWorkflow, starter.input)
	require.NoError(t, env.GetWorkflowError())
	require.NoError(t, ingest.IngestOriginal(t.Context(), starter.input))
	detail, err = store.ReadPublicTask(t.Context(), actor, i.ProjectID, i.OperationID)
	require.NoError(t, err)
	require.Equal(t, "ready", detail.BillingOutcome.DeliveryState)
	require.Equal(t, "confirm", detail.BillingOutcome.Regenerate)
	require.Len(t, detail.Outputs, 2)
	// A previously archived asset cannot prove ready after its current media
	// authority changes, even though the original manifest remains successful.
	for _, change := range []string{"is_delete=true", "moderation_status='rejected'", "status='processing'", "source_operation_id=NULL", "mime_type='image/jpeg'", "byte_size=byte_size+1", "sha256='ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff'"} {
		require.NoError(t, db.Exec(`UPDATE media.media_asset SET `+change+` WHERE id=?`, second).Error)
		changed, err := store.ReadPublicTask(t.Context(), actor, i.ProjectID, i.OperationID)
		require.NoError(t, err)
		require.NotEqual(t, "ready", changed.BillingOutcome.DeliveryState)
		require.Equal(t, "blocked", changed.BillingOutcome.Regenerate)
		_, err = store.ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: i.ProjectID, OperationID: quoted.ID, RequestID: uuid.NewString()})
		require.ErrorIs(t, err, application.ErrPendingConsumption)
		require.NoError(t, db.Exec(`UPDATE media.media_asset SET is_delete=false,moderation_status='passed',status='ready',source_operation_id=?,mime_type='image/png',byte_size=(SELECT (result->'artifacts'->1->>'size')::bigint FROM operation.provider_result WHERE operation_id=?),sha256=(SELECT result->'artifacts'->1->>'sha256' FROM operation.provider_result WHERE operation_id=?) WHERE id=?`, i.OperationID, i.OperationID, i.OperationID, second).Error)
	}

	loc := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO media.storage_profile(id,provider,endpoint,signing_endpoint,region,bucket,addressing_style,resource_identity) VALUES('review-original','tos','https://example.invalid','https://example.invalid','cn-test','review-fixture','',repeat('a',64)) ON CONFLICT (id) DO NOTHING`).Error)
	require.NoError(t, db.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state,byte_size,sha256,mime_type) SELECT ?,'review-original','review-fixture',object_key,'unknown',byte_size,sha256,mime_type FROM media.media_asset WHERE id=?`, loc, second).Error)
	require.NoError(t, db.Exec(`UPDATE media.media_asset SET object_location_id=? WHERE id=?`, loc, second).Error)
	detail, err = store.ReadPublicTask(t.Context(), actor, i.ProjectID, i.OperationID)
	require.NoError(t, err)
	_, err = store.ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: i.ProjectID, OperationID: quoted.ID, RequestID: uuid.NewString()})
	t.Logf("original physical location=unknown; GET delivery=%s action=%s; same target confirm error=%v", detail.BillingOutcome.DeliveryState, detail.BillingOutcome.Regenerate, err)
	require.ErrorIs(t, err, application.ErrPendingConsumption)
	require.NotEqual(t, "ready", detail.BillingOutcome.DeliveryState)
	require.NoError(t, db.Exec(`UPDATE media.object_location SET state='present' WHERE id=?`, loc).Error)
	detail, err = store.ReadPublicTask(t.Context(), actor, i.ProjectID, i.OperationID)
	require.NoError(t, err)
	require.Equal(t, "ready", detail.BillingOutcome.DeliveryState)
	// The composite FK rejects dangling or mismatched bindings on new rows.
	require.Error(t, db.Exec(`UPDATE media.media_asset SET object_key=object_key||'/wrong' WHERE id=?`, second).Error)

	for _, mutation := range []string{
		fmt.Sprintf(`DELETE FROM operation.provider_result WHERE operation_id='%s'`, i.OperationID),
		fmt.Sprintf(`UPDATE operation.operation_output SET seq_no=0 WHERE operation_id='%s' AND seq_no=1`, i.OperationID),
		fmt.Sprintf(`UPDATE operation.provider_result SET result=jsonb_set(result,'{identity,request_key}','"different-original-request"') WHERE operation_id='%s'`, i.OperationID),
	} {
		rollback := errors.New("strict manifest fixture rollback")
		err := db.Transaction(func(tx *gorm.DB) error {
			require.NoError(t, tx.Exec(mutation).Error)
			changed, err := operationStore(tx).ReadPublicTask(t.Context(), actor, i.ProjectID, i.OperationID)
			require.NoError(t, err)
			require.NotEqual(t, "ready", changed.BillingOutcome.DeliveryState)
			require.Equal(t, "blocked", changed.BillingOutcome.Regenerate)
			_, err = operationStore(tx).ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: i.ProjectID, OperationID: quoted.ID, RequestID: uuid.NewString()})
			require.ErrorIs(t, err, application.ErrPendingConsumption)
			return rollback
		})
		require.ErrorIs(t, err, rollback)
	}

	receipt, err = store.RequestWorkflowControl(t.Context(), actor, application.WorkflowControlInput{ProjectID: i.ProjectID, TargetID: i.OperationID, TargetType: "operation", Action: "recover", RequestID: uuid.NewString()})
	require.NoError(t, err)
	require.False(t, receipt.Accepted)
	after, err := readDispatchBudget(t.Context(), db, i.OperationID)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, 1, network.calls)
	_, err = store.ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: i.ProjectID, OperationID: quoted.ID, RequestID: uuid.NewString()})
	require.NoError(t, err)
}
func TestFinalReviewTerminalLedgerConflictMustRefuseArchive(t *testing.T) {
	db := operationStoreDB(t)
	f := seedManagedFixture(t, db)
	i, store := f.identity, f.store
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile SET capability='text.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, i.ModelProfileVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text_to_text'],queue='provider',supports_cancel=false,supports_query=false,moderation='provider' WHERE id=?`, i.ModelProfileVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.price_rule_version SET unit='per_request',rule='{"base_micros":10}' WHERE id=?`, i.PriceRuleVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE operation.operation SET capability='text.generate',mode='text_to_text',params='{}' WHERE id=?`, i.OperationID).Error)
	require.NoError(t, db.Exec(`INSERT INTO operation.operation_input(id,operation_id,seq_no,role,ref_type,text_value) VALUES(?,?,0,'prompt','text','original fixture prompt') ON CONFLICT(operation_id,seq_no) DO NOTHING`, uuid.New(), i.OperationID).Error)
	objects := &stageTestObjects{entries: map[string]stageTestObject{}}
	results := application.NewManagedResults(objects, store)
	network := &fixtureGeneration{}
	worker := managed.NewActivities(store, fixtureUnsealer{}, network, results, nil, nil)
	_, err := worker.Step(t.Context(), application.ManagedStep{OperationID: i.OperationID, Action: "submit"})
	require.NoError(t, err)
	require.Equal(t, 1, network.calls)
	require.NoError(t, store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{OperationID: i.OperationID, Action: "submit", Attempt: 1, Outcome: "ok", State: "completed", Managed: true}))
	_, err = store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusSucceeded})
	require.NoError(t, err)
	_, err = store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSucceeded}, To: domain.StatusIngesting})
	require.NoError(t, err)
	finalizer := app.NewOperationFinalizer(db, store, billingpg.NewStore(db))
	require.NoError(t, db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=(SELECT confirmed_by FROM operation.operation WHERE id=?)`, i.OperationID).Error)
	require.NoError(t, finalizer.ResolveRevokedOperation(t.Context(), i.OperationID.String()))
	require.NoError(t, db.Exec(`UPDATE identity."user" SET status='active' WHERE id=(SELECT confirmed_by FROM operation.operation WHERE id=?)`, i.OperationID).Error)

	var actor identityapp.Principal
	require.NoError(t, db.Raw(`SELECT u.id,u.org_id,u.role FROM identity."user" u JOIN operation.operation o ON o.confirmed_by=u.id WHERE o.id=?`, i.OperationID).Scan(&actor).Error)
	actor = identityfixture.Session(t, db, actor)
	router := publicOperationRouter(actor, db)
	key := uuid.NewString()
	path := "/api/operations/" + i.OperationID.String() + "/recover"
	body := fmt.Sprintf(`{"project_id":%q}`, i.ProjectID)

	accepted, err := store.RequestWorkflowControl(t.Context(), actor, application.WorkflowControlInput{ProjectID: i.ProjectID, TargetID: i.OperationID, TargetType: "operation", Action: "recover", RequestID: uuid.NewString()})
	require.NoError(t, err)
	d := application.WorkflowControlDelivery{EventID: accepted.EventID, RequestID: accepted.RequestID, ActorID: actor.ID, OrgID: actor.OrgID, ProjectID: i.ProjectID, TargetID: i.OperationID, TargetType: "operation", Action: "recover"}
	require.NoError(t, db.Exec(`UPDATE billing.reservation SET amount_micros=amount_micros+1 WHERE operation_id=?`, i.OperationID).Error)
	detail := publicOperationRequest(t, router, "GET", "/api/operations/"+i.OperationID.String()+"?project_id="+i.ProjectID.String(), "", "")
	require.Contains(t, detail.Body.String(), `"reason":"settlement_conflict"`)
	res := publicOperationRequest(t, router, "POST", path, body, key)
	ingest := managed.NewScopedIngestActivities(db, nil, results, app.NewMediaWorkerScopeAccess, store)
	err = ingest.IngestOriginal(t.Context(), d)
	t.Logf("GET=settlement_conflict, POST=%d, direct worker error=%v", res.Code, err)
	require.Equal(t, 409, res.Code)
	require.Error(t, err)
}
func TestFinalReviewAuthorizedSurvivingTeamMemberCanRecover(t *testing.T) {
	db := operationStoreDB(t)
	f := seedManagedFixture(t, db)
	i, store := f.identity, f.store
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile SET capability='text.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, i.ModelProfileVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text_to_text'],queue='provider',supports_cancel=false,supports_query=false,moderation='provider' WHERE id=?`, i.ModelProfileVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.price_rule_version SET unit='per_request',rule='{"base_micros":10}' WHERE id=?`, i.PriceRuleVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE operation.operation SET capability='text.generate',mode='text_to_text',params='{}' WHERE id=?`, i.OperationID).Error)
	require.NoError(t, db.Exec(`INSERT INTO operation.operation_input(id,operation_id,seq_no,role,ref_type,text_value) VALUES(?,?,0,'prompt','text','original fixture prompt') ON CONFLICT(operation_id,seq_no) DO NOTHING`, uuid.New(), i.OperationID).Error)
	var original identityapp.Principal
	require.NoError(t, db.Raw(`SELECT u.id,u.org_id,u.role FROM identity."user" u JOIN operation.operation o ON o.confirmed_by=u.id WHERE o.id=?`, i.OperationID).Scan(&original).Error)
	member := identityapp.Principal{ID: uuid.New(), OrgID: original.OrgID, Role: original.Role}
	require.NoError(t, db.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password) VALUES (?,?,?,'Review fixture','producer','test-hash',false)`, member.ID, member.OrgID, "review-"+member.ID.String()).Error)
	member = identityfixture.Session(t, db, member)
	team, scope := uuid.New(), uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'Review team',?)`, team, member.ID).Error)
	require.NoError(t, db.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',now(),'created'),(?,?,'member','active',now(),'created')`, team, member.ID, team, original.ID).Error)
	require.NoError(t, db.Exec(`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, scope, team, member.OrgID).Error)
	require.NoError(t, db.Exec(`UPDATE workspace.project SET owner_scope_id=? WHERE id=?`, scope, i.ProjectID).Error)
	member.ScopeID = scope
	objects := &stageTestObjects{entries: map[string]stageTestObject{}}
	results := application.NewManagedResults(objects, store)
	network := &fixtureGeneration{}
	worker := managed.NewActivities(store, fixtureUnsealer{}, network, results, nil, nil)
	_, err := worker.Step(t.Context(), application.ManagedStep{OperationID: i.OperationID, Action: "submit"})
	require.NoError(t, err)
	require.Equal(t, 1, network.calls)
	require.NoError(t, store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{OperationID: i.OperationID, Action: "submit", Attempt: 1, Outcome: "ok", State: "completed", Managed: true}))
	_, err = store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusSucceeded})
	require.NoError(t, err)
	_, err = store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSucceeded}, To: domain.StatusIngesting})
	require.NoError(t, err)
	finalizer := app.NewOperationFinalizer(db, store, billingpg.NewStore(db))
	require.NoError(t, db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=(SELECT confirmed_by FROM operation.operation WHERE id=?)`, i.OperationID).Error)
	require.NoError(t, finalizer.ResolveRevokedOperation(t.Context(), i.OperationID.String()))

	router := publicOperationRouter(member, db)
	key := uuid.NewString()
	body := fmt.Sprintf(`{"project_id":%q}`, i.ProjectID)
	path := "/api/operations/" + i.OperationID.String() + "/recover"
	res := publicOperationRequest(t, router, "POST", path, body, key)
	require.Equal(t, 202, res.Code, res.Body.String())
	var e struct {
		Topic, PartitionKey string
		Payload             []byte
	}
	require.NoError(t, db.Raw(`SELECT topic,partition_key,payload FROM infra.outbox WHERE topic=? AND payload->'data'->>'request_id'=?`, operationevent.WorkflowControlTopic, key).Scan(&e).Error)
	// Let the installed SDK handle AlreadyStarted itself. Only its wire service
	// is simulated; the inbox, receipts, authorization and archive use PostgreSQL.
	running := false
	var starts []application.WorkflowControlDelivery
	attempts := 0
	firstRun := uuid.NewString()
	rpc := func(_ context.Context, method string, req, reply any, _ *grpc.ClientConn, _ grpc.UnaryInvoker, _ ...grpc.CallOption) error {
		switch method {
		case workflowservice.WorkflowService_GetSystemInfo_FullMethodName:
			return nil
		case workflowservice.WorkflowService_StartWorkflowExecution_FullMethodName:
			attempts++
			in := req.(*workflowservice.StartWorkflowExecutionRequest)
			require.Equal(t, "operation-delivery/"+i.OperationID.String(), in.WorkflowId)
			require.Equal(t, enums.WORKFLOW_ID_CONFLICT_POLICY_FAIL, in.WorkflowIdConflictPolicy)
			require.Equal(t, enums.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE_FAILED_ONLY, in.WorkflowIdReusePolicy)
			if running {
				return serviceerror.ToStatus(serviceerror.NewWorkflowExecutionAlreadyStarted("original archive in progress", in.RequestId, firstRun)).Err()
			}
			var delivery application.WorkflowControlDelivery
			if err := converter.GetDefaultDataConverter().FromPayloads(in.Input, &delivery); err != nil {
				return err
			}
			starts = append(starts, delivery)
			run := firstRun
			if len(starts) > 1 {
				run = uuid.NewString()
			}
			running = true
			reply.(*workflowservice.StartWorkflowExecutionResponse).RunId = run
			return nil
		default:
			return fmt.Errorf("unexpected Temporal RPC %s", method)
		}
	}
	sdk, err := client.NewLazyClient(client.Options{ConnectionOptions: client.ConnectionOptions{DialOptions: []grpc.DialOption{grpc.WithChainUnaryInterceptor(rpc), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return nil, errors.New("offline Temporal SDK fixture")
	})}}})
	require.NoError(t, err)
	t.Cleanup(sdk.Close)
	h := operationevent.NewWorkflowControlHandler(inboxpg.NewStore(db), store, operationflow.NewControl(sdk, store))
	first := inbox.Record{Topic: e.Topic, Key: []byte(e.PartitionKey), Value: e.Payload}
	require.NoError(t, h.Handle(t.Context(), first))
	require.NoError(t, h.Handle(t.Context(), first))
	require.Len(t, starts, 1)
	require.Equal(t, 1, attempts)
	require.Equal(t, key, starts[0].RequestID.String())
	ingest := managed.NewScopedIngestActivities(db, nil, results, app.NewMediaWorkerScopeAccess, store)
	require.NoError(t, db.Exec(`UPDATE workspace.team_member SET status='suspended' WHERE team_id=? AND user_id=?`, team, member.ID).Error)
	require.Error(t, ingest.IngestOriginal(t.Context(), starts[0]))
	require.NoError(t, db.Exec(`UPDATE workspace.team_member SET status='active' WHERE team_id=? AND user_id=?`, team, member.ID).Error)
	require.NoError(t, db.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, member.SessionID).Error)
	require.Error(t, ingest.IngestOriginal(t.Context(), starts[0]))
	member = identityfixture.Session(t, db, identityapp.Principal{ID: member.ID, OrgID: member.OrgID, Role: member.Role})
	member.ScopeID = scope
	secondKey := uuid.NewString()
	current := publicOperationRouter(member, db)
	secondResponse := publicOperationRequest(t, current, "POST", path, body, secondKey)
	require.Equal(t, 202, secondResponse.Code, secondResponse.Body.String())
	replay := publicOperationRequest(t, current, "POST", path, body, secondKey)
	require.JSONEq(t, secondResponse.Body.String(), replay.Body.String())
	var receipt application.WorkflowControlResult
	require.NoError(t, json.Unmarshal(secondResponse.Body.Bytes(), &receipt))
	require.True(t, receipt.Accepted)
	require.NoError(t, db.Raw(`SELECT topic,partition_key,payload FROM infra.outbox WHERE id=?`, receipt.EventID).Scan(&e).Error)
	next := inbox.Record{Topic: e.Topic, Key: []byte(e.PartitionKey), Value: e.Payload}
	var occupied *serviceerror.WorkflowExecutionAlreadyStarted
	require.ErrorAs(t, h.Handle(t.Context(), next), &occupied)
	require.Equal(t, 2, attempts)
	require.Len(t, starts, 1)
	var markers int
	require.NoError(t, db.Raw(`SELECT count(*) FROM infra.processed_event WHERE consumer='workflow-control' AND event_id=?`, receipt.EventID).Scan(&markers).Error)
	require.Zero(t, markers)
	// The old worker actually rejects its revoked request. Once Temporal
	// records that run failed, the redelivered event may start a fresh run.
	require.Error(t, ingest.IngestOriginal(t.Context(), starts[0]))
	running = false
	require.NoError(t, h.Handle(t.Context(), next))
	require.Len(t, starts, 2)
	require.Equal(t, receipt.RequestID, starts[1].RequestID)
	require.Equal(t, 3, attempts)
	require.NoError(t, h.Handle(t.Context(), next))
	require.Equal(t, 3, attempts)
	require.NoError(t, ingest.IngestOriginal(t.Context(), starts[1]))
	noop := publicOperationRequest(t, current, "POST", path, body, uuid.NewString())
	require.Equal(t, 202, noop.Code)
	require.Contains(t, noop.Body.String(), `"accepted":false`)
	require.Equal(t, 1, network.calls)
	require.Error(t, store.AuthorizeWorker(t.Context(), i.OperationID, true))
	detail, err := store.ReadPublicTask(t.Context(), member, i.ProjectID, i.OperationID)
	require.NoError(t, err)
	require.Equal(t, "ready", detail.BillingOutcome.DeliveryState)
}

func TestRecoveredAudioShowsOnlyOriginalSubtitle(t *testing.T) {
	db := operationStoreDB(t)
	f := seedManagedFixture(t, db)
	i, store := f.identity, f.store
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile SET capability='audio.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, i.ModelProfileVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['omni_audio'],moderation='provider' WHERE id=?`, i.ModelProfileVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.price_rule_version SET unit='per_request',rule='{"base_micros":8}' WHERE id=?`, i.PriceRuleVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE operation.operation SET capability='audio.generate',mode='omni_audio' WHERE id=?`, i.OperationID).Error)
	claim, err := store.ClaimProviderDispatch(t.Context(), i)
	require.NoError(t, err)
	require.True(t, claim.Claimed)
	data, err := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-f", "mp3", "pipe:1").Output()
	require.NoError(t, err)
	results := application.NewManagedResults(&stageTestObjects{entries: map[string]stageTestObject{}}, store)
	artifact, err := results.StageArtifact(t.Context(), i, 1, data, "audio/mpeg")
	require.NoError(t, err)
	subtitle := &application.AudioSubtitle{Text: "原始字幕"}
	require.NoError(t, results.Save(t.Context(), application.ManagedResult{Identity: i, State: "succeeded", Artifacts: []application.ManagedArtifact{artifact}, Usage: json.RawMessage(`{"billable_requests":1,"output_count":1}`), Subtitle: subtitle}))
	require.NoError(t, store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{OperationID: i.OperationID, Action: "submit", Attempt: 1, Outcome: "ok", State: "completed", Managed: true}))
	_, err = store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusSucceeded})
	require.NoError(t, err)
	_, err = store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSucceeded}, To: domain.StatusIngesting})
	require.NoError(t, err)
	var actor identityapp.Principal
	require.NoError(t, db.Raw(`SELECT u.id,u.org_id,u.role FROM identity."user" u JOIN operation.operation o ON u.id=o.confirmed_by WHERE o.id=?`, i.OperationID).Scan(&actor).Error)
	require.NoError(t, db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error)
	require.NoError(t, app.NewOperationFinalizer(db, store, billingpg.NewStore(db)).ResolveRevokedOperation(t.Context(), i.OperationID.String()))
	require.NoError(t, db.Exec(`UPDATE identity."user" SET status='active' WHERE id=?`, actor.ID).Error)
	actor = identityfixture.Session(t, db, actor)
	before, err := store.ReadPublicTask(t.Context(), actor, i.ProjectID, i.OperationID)
	require.NoError(t, err)
	require.Nil(t, before.AudioSubtitle)
	receipt, err := store.RequestWorkflowControl(t.Context(), actor, application.WorkflowControlInput{ProjectID: i.ProjectID, TargetID: i.OperationID, TargetType: "operation", Action: "recover", RequestID: uuid.NewString()})
	require.NoError(t, err)
	d := application.WorkflowControlDelivery{TargetID: i.OperationID, ProjectID: i.ProjectID, OrgID: actor.OrgID, ActorID: actor.ID, Action: "recover", TargetType: "operation", RequestID: receipt.RequestID, EventID: receipt.EventID}
	objects, _ := originalDeliveryObjects(t, uuid.Nil)
	ingest := managed.NewScopedIngestActivities(db, objects, results, app.NewMediaWorkerScopeAccess, store)
	require.NoError(t, ingest.IngestOriginal(t.Context(), d))
	after, err := store.ReadPublicTask(t.Context(), actor, i.ProjectID, i.OperationID)
	require.NoError(t, err)
	require.Equal(t, "ready", after.BillingOutcome.DeliveryState)
	require.Equal(t, subtitle, after.AudioSubtitle)
	require.NoError(t, db.Exec(`UPDATE operation.provider_result SET result=result-'subtitle' WHERE operation_id=?`, i.OperationID).Error)
	after, err = store.ReadPublicTask(t.Context(), actor, i.ProjectID, i.OperationID)
	require.NoError(t, err)
	require.Nil(t, after.AudioSubtitle)
}
