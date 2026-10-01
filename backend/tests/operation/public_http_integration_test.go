package operation_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	cataloghttp "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/http"
	pgcatalog "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/postgres"
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	operationhttp "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/http"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

func publicOperationRouter(actor identityapp.Principal, database *gorm.DB) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	router.Use(func(c *gin.Context) { c.Set("principal", actor) })
	group := router.Group("/api")
	store := pgoperation.NewStore(database)
	operationhttp.NewHandler(operationhttp.Dependencies{FreeQuote: operationapp.NewCreateFreeQuoteCommand(store), BatchQuote: operationapp.NewCreateBatchFreeQuoteCommand(store), Confirm: operationapp.NewConfirmSingleQuoteCommand(store), ConfirmBatch: operationapp.NewConfirmBatchQuoteCommand(store), Query: operationapp.NewPublicQuery(store), Control: operationapp.NewWorkflowControlCommand(store)}).Register(group)
	cataloghttp.NewHandler(catalogapp.NewListModelsQuery(pgcatalog.NewStore(database))).Register(group)
	return router
}

func publicOperationRequest(t *testing.T, router *gin.Engine, method, path, body, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	return res
}

func TestPublicOperationHTTPRealQuotesConfirmationRefreshAndCancellation(t *testing.T) {
	database := operationStoreDB(t)
	actor, project := operationStoreProject(t, database)
	quoted, modelID, _ := seedQuoteCatalog(t, database, project, time.Now().UTC())
	setPublicQuoteLimits(t, database, quoted.ModelProfileVersionID)
	router := publicOperationRouter(actor, database)
	res := publicOperationRequest(t, router, "GET", "/api/models?project_id="+project.String()+"&capability="+quoted.Capability, "", "")
	if res.Code != 200 || !strings.Contains(res.Body.String(), `"param_schema":[]`) || strings.Contains(res.Body.String(), "test-ciphertext") {
		t.Fatalf("public model catalog=%d %s", res.Code, res.Body)
	}
	quoteBody := fmt.Sprintf(`{"model_key":"quote-model-%s","capability":"%s","mode":"text_to_image","prompt":"晨光","params":{},"output_count":1}`, modelID, quoted.Capability)
	key := uuid.NewString()
	path := "/api/projects/" + project.String() + "/free-operations"
	res = publicOperationRequest(t, router, "POST", path, quoteBody, key)
	var quote operationhttp.QuoteResponse
	if res.Code != 201 || json.Unmarshal(res.Body.Bytes(), &quote) != nil || quote.OperationID == uuid.Nil || quote.QuoteMicros != 1 {
		t.Fatalf("real quote=%d %s", res.Code, res.Body)
	}
	replay := publicOperationRequest(t, router, "POST", path, quoteBody, key)
	if replay.Code != 201 || !samePublicHTTPJSON(replay.Body.Bytes(), res.Body.Bytes()) {
		t.Fatalf("quote replay=%d %s", replay.Code, replay.Body)
	}
	confirmBody := `{"project_id":"` + project.String() + `"}`
	confirmKey := uuid.NewString()
	res = publicOperationRequest(t, router, "POST", "/api/operations/"+quote.OperationID.String()+"/confirm", confirmBody, confirmKey)
	if res.Code != 200 || !strings.Contains(res.Body.String(), `"available_micros":99`) {
		t.Fatalf("real confirm=%d %s", res.Code, res.Body)
	}
	replay = publicOperationRequest(t, router, "POST", "/api/operations/"+quote.OperationID.String()+"/confirm", confirmBody, confirmKey)
	if replay.Code != 200 || !samePublicHTTPJSON(replay.Body.Bytes(), res.Body.Bytes()) {
		t.Fatalf("confirmation replay=%d %s", replay.Code, replay.Body)
	}
	res = publicOperationRequest(t, router, "GET", "/api/operations/"+quote.OperationID.String()+"?project_id="+project.String(), "", "")
	if res.Code != 200 || !strings.Contains(res.Body.String(), `"status":"confirmed"`) || !strings.Contains(res.Body.String(), `"text":"晨光"`) || strings.Contains(res.Body.String(), "test-ciphertext") {
		t.Fatalf("task restore=%d %s", res.Code, res.Body)
	}
	res = publicOperationRequest(t, router, "GET", "/api/projects/"+project.String()+"/operations?status=confirmed&limit=1", "", "")
	if res.Code != 200 || !strings.Contains(res.Body.String(), quote.OperationID.String()) {
		t.Fatalf("task page=%d %s", res.Code, res.Body)
	}
	cancelKey := uuid.NewString()
	cancelPath := "/api/operations/" + quote.OperationID.String() + "/cancel"
	res = publicOperationRequest(t, router, "POST", cancelPath, confirmBody, cancelKey)
	if res.Code != 202 || strings.Contains(res.Body.String(), `"status":"cancelled"`) {
		t.Fatalf("cancel must report intent=%d %s", res.Code, res.Body)
	}
	replay = publicOperationRequest(t, router, "POST", cancelPath, confirmBody, cancelKey)
	if replay.Code != 202 || !samePublicHTTPJSON(replay.Body.Bytes(), res.Body.Bytes()) {
		t.Fatalf("cancel replay=%d %s", replay.Code, replay.Body)
	}
	var count int64
	if err := database.Raw(`SELECT count(*) FROM infra.outbox WHERE topic='lanverse.workflow.control_requested.v1' AND payload->'aggregate'->>'id'=?`, quote.OperationID.String()).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("durable single control=%d %v", count, err)
	}
	res = publicOperationRequest(t, router, "GET", "/api/operations/"+quote.OperationID.String()+"?project_id="+project.String(), "", "")
	if res.Code != 200 || !strings.Contains(res.Body.String(), `"status":"confirmed"`) || !strings.Contains(res.Body.String(), `"cancel_requested":true`) {
		t.Fatalf("cancelled request forged terminal state=%d %s", res.Code, res.Body)
	}
}

func samePublicHTTPJSON(first, second []byte) bool {
	var a, b any
	if json.Unmarshal(first, &a) != nil || json.Unmarshal(second, &b) != nil {
		return false
	}
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(x) == string(y)
}

func TestPublicTaskHTTPHidesForeignScopeAndRevokedActor(t *testing.T) {
	database := operationStoreDB(t)
	actor, project := operationStoreProject(t, database)
	_, operation, _, _ := operationStoreRows(t, database, actor, project)
	other, otherProject := operationStoreProject(t, database)
	for _, tc := range []struct {
		actor   identityapp.Principal
		project uuid.UUID
	}{{other, project}, {actor, otherProject}} {
		res := publicOperationRequest(t, publicOperationRouter(tc.actor, database), "GET", "/api/operations/"+operation.String()+"?project_id="+tc.project.String(), "", "")
		if res.Code != 404 {
			t.Fatalf("foreign task exposed: %d %s", res.Code, res.Body)
		}
	}
	if err := database.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?::uuid`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	res := publicOperationRequest(t, publicOperationRouter(actor, database), "GET", "/api/projects/"+project.String()+"/operations", "", "")
	if res.Code != 403 {
		t.Fatalf("revoked actor=%d %s", res.Code, res.Body)
	}
}

func TestPublicBatchHTTPConfirmsQueriesAndResumesOnlyPausedBatch(t *testing.T) {
	database := operationStoreDB(t)
	actor, project := operationStoreProject(t, database)
	quoted, modelID, _ := seedQuoteCatalog(t, database, project, time.Now().UTC())
	setPublicQuoteLimits(t, database, quoted.ModelProfileVersionID)
	router := publicOperationRouter(actor, database)
	item := fmt.Sprintf(`{"model_key":"quote-model-%s","capability":"%s","mode":"text_to_image","prompt":"风吹麦浪","params":{},"output_count":1}`, modelID, quoted.Capability)
	res := publicOperationRequest(t, router, "POST", "/api/projects/"+project.String()+"/quotes", `{"items":[`+item+`,`+item+`]}`, uuid.NewString())
	var batch operationapp.CreateBatchFreeQuoteResult
	if res.Code != 201 || json.Unmarshal(res.Body.Bytes(), &batch) != nil || batch.BatchID == nil || len(batch.Items) != 2 {
		t.Fatalf("batch quote=%d %s", res.Code, res.Body)
	}
	body := `{"project_id":"` + project.String() + `"}`
	res = publicOperationRequest(t, router, "POST", "/api/batches/"+batch.BatchID.String()+"/confirm", body, uuid.NewString())
	if res.Code != 200 {
		t.Fatalf("batch confirm=%d %s", res.Code, res.Body)
	}
	res = publicOperationRequest(t, router, "GET", "/api/batches/"+batch.BatchID.String()+"?project_id="+project.String(), "", "")
	if res.Code != 200 || !strings.Contains(res.Body.String(), `"total_count":2`) || !strings.Contains(res.Body.String(), batch.Items[0].OperationID.String()) {
		t.Fatalf("batch restore=%d %s", res.Code, res.Body)
	}
	res = publicOperationRequest(t, router, "POST", "/api/batches/"+batch.BatchID.String()+"/resume", body, uuid.NewString())
	if res.Code != 409 {
		t.Fatalf("unpaused batch resumed=%d %s", res.Code, res.Body)
	}
	if err := database.Exec(`UPDATE operation.batch SET status='running',paused_reason='budget_guard' WHERE id=?::uuid`, batch.BatchID).Error; err != nil {
		t.Fatal(err)
	}
	res = publicOperationRequest(t, router, "POST", "/api/batches/"+batch.BatchID.String()+"/resume", body, uuid.NewString())
	if res.Code != 202 {
		t.Fatalf("paused batch resume=%d %s", res.Code, res.Body)
	}
}

func TestPublicCancelIntentBlocksNewDispatchButPreservesClaimedEvidence(t *testing.T) {
	database := operationStoreDB(t)
	identity, store := seedDispatchCall(t, database, true)
	if err := database.Exec(`INSERT INTO operation.operation_event(id,operation_id,from_status,to_status,reason) VALUES(?::uuid,?::uuid,'confirmed','confirmed','user_cancel_requested')`, uuid.New(), identity.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	if result, err := store.ClaimProviderDispatch(t.Context(), identity); !errors.Is(err, operationapp.ErrProviderDispatchCancelled) || result.Claimed {
		t.Fatalf("cancelled request received dispatch right: %+v %v", result, err)
	}
	identity2, store2 := seedDispatchCall(t, database, true)
	claimed, err := store2.ClaimProviderDispatch(t.Context(), identity2)
	if err != nil || !claimed.Claimed {
		t.Fatalf("first claim=%+v %v", claimed, err)
	}
	if err := database.Exec(`INSERT INTO operation.operation_event(id,operation_id,from_status,to_status,reason) VALUES(?::uuid,?::uuid,'submitting','submitting','user_cancel_requested')`, uuid.New(), identity2.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	result, err := store2.ClaimProviderDispatch(t.Context(), identity2)
	if err != nil || result.Claimed || result.DispatchStartedAt == nil {
		t.Fatalf("cancel changed already-dispatched evidence: %+v %v", result, err)
	}
}

func TestPublicCancelAndSubmissionRaceHasOneWinner(t *testing.T) {
	database := operationStoreDB(t)
	operation, store := seedProviderCallOperation(t, database)
	var scope struct {
		ProjectID uuid.UUID
		OrgID     uuid.UUID
		ActorID   uuid.UUID
	}
	if err := database.Raw(`SELECT o.project_id, p.org_id, o.confirmed_by AS actor_id FROM operation.operation o JOIN workspace.project p ON p.id=o.project_id WHERE o.id=?::uuid`, operation).Scan(&scope).Error; err != nil {
		t.Fatal(err)
	}
	actor := identityapp.Principal{ID: scope.ActorID, OrgID: scope.OrgID, Role: "producer"}
	project := scope.ProjectID
	requestKey := "operation/" + operation.String()
	var cancelErr, submitErr error
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, cancelErr = store.RequestWorkflowControl(t.Context(), actor, operationapp.WorkflowControlInput{ProjectID: project, TargetID: operation, TargetType: "operation", Action: "cancel", RequestID: uuid.NewString()})
	}()
	go func() {
		defer wg.Done()
		<-start
		_, submitErr = store.TransitionWorkflowOperation(t.Context(), operationapp.TransitionInput{OperationID: operation, From: []domain.Status{domain.StatusConfirmed}, To: domain.StatusSubmitting, ProviderRequestKey: &requestKey})
	}()
	close(start)
	wg.Wait()
	if submitErr != nil {
		t.Fatalf("submission transition=%v", submitErr)
	}
	if cancelErr != nil && !errors.Is(cancelErr, operationapp.ErrWorkflowControlConflict) {
		t.Fatalf("cancel race=%v", cancelErr)
	}
	if err := store.BeginProviderCall(t.Context(), operationapp.BeginProviderCallInput{OperationID: operation, Action: "submit", Attempt: 1, DispatchRequired: true}); err != nil {
		t.Fatal(err)
	}
	var identity operationapp.ProviderDispatchIdentity
	if err := database.Raw(`SELECT project_id,id AS operation_id,provider_request_key AS request_key,model_profile_version_id,price_rule_version_id FROM operation.operation WHERE id=?::uuid`, operation).Scan(&identity).Error; err != nil {
		t.Fatal(err)
	}
	identity.Action = "submit"
	identity.Attempt = 1
	dispatch, err := store.ClaimProviderDispatch(t.Context(), identity)
	if cancelErr == nil {
		if !errors.Is(err, operationapp.ErrProviderDispatchCancelled) || dispatch.Claimed {
			t.Fatalf("accepted cancel raced into paid send: %+v %v", dispatch, err)
		}
	} else if err != nil || !dispatch.Claimed {
		t.Fatalf("rejected cancel blocked permitted send: %+v %v", dispatch, err)
	}
}

func setPublicQuoteLimits(t *testing.T, database *gorm.DB, versionID *uuid.UUID) {
	t.Helper()
	if err := database.Exec(`UPDATE catalog.model_profile_version SET limits='{"max_outputs":4}'::jsonb WHERE id=?::uuid`, versionID).Error; err != nil {
		t.Fatal(err)
	}
}

func TestPublicCanvasBatchSourceSurvivesRefreshAndEnforcesSavedCapacity(t *testing.T) {
	database := operationStoreDB(t)
	actor, project := operationStoreProject(t, database)
	quoted, modelID, _ := seedQuoteCatalog(t, database, project, time.Now().UTC())
	setPublicQuoteLimits(t, database, quoted.ModelProfileVersionID)
	canvasID, nodeID, rowA, rowB := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if err := database.Exec(`INSERT INTO canvas.document(id,project_id,name,revision) VALUES(?::uuid,?::uuid,'来源恢复',3)`, canvasID, project).Error; err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf(`{"batch_table":{"version":1,"operation":"creative","mode":"text_to_image","output_count":1,"global_prompt":"","concurrency":1,"model_profile_id":"%s","params":{},"reference_columns":[{"id":"%s","label":"图1"}],"rows":[{"id":"%s","enabled":true,"prompt":"A","input_node_ids":[null]},{"id":"%s","enabled":true,"prompt":"B","input_node_ids":[null]}]}}`, modelID, uuid.New(), rowA, rowB)
	if err := database.Exec(`INSERT INTO canvas.node(id,document_id,node_type,node_action,config,x,y) VALUES(?::uuid,?::uuid,'batch_table','resource',?::jsonb,0,0)`, nodeID, canvasID, config).Error; err != nil {
		t.Fatal(err)
	}
	item := func(row uuid.UUID, revision int) string {
		prompt := "A"
		if row == rowB {
			prompt = "B"
		}
		return fmt.Sprintf(`{"model_key":"quote-model-%s","capability":"%s","mode":"text_to_image","prompt":"%s","params":{},"output_count":1,"source":{"canvas_id":"%s","node_id":"%s","row_id":"%s","revision":%d}}`, modelID, quoted.Capability, prompt, canvasID, nodeID, row, revision)
	}
	router := publicOperationRouter(actor, database)
	res := publicOperationRequest(t, router, "POST", "/api/projects/"+project.String()+"/quotes", `{"items":[`+item(rowA, 3)+`,`+item(rowB, 3)+`]}`, uuid.NewString())
	var batch operationapp.CreateBatchFreeQuoteResult
	if res.Code != 201 || json.Unmarshal(res.Body.Bytes(), &batch) != nil || batch.BatchID == nil {
		var requested operationhttp.QuotesRequest
		if err := json.Unmarshal([]byte(`{"items":[`+item(rowA, 3)+`,`+item(rowB, 3)+`]}`), &requested); err != nil {
			t.Fatal(err)
		}
		inputs := make([]operationapp.FreeQuoteItemInput, 0, len(requested.Items))
		for _, r := range requested.Items {
			inputs = append(inputs, operationapp.FreeQuoteItemInput{ModelKey: r.ModelKey, Capability: r.Capability, Mode: r.Mode, Prompt: r.Prompt, Params: r.Params, OutputCount: r.OutputCount, Source: r.Source})
		}
		_, err := pgoperation.NewStore(database).CreateBatchFreeQuote(t.Context(), actor, operationapp.CreateBatchFreeQuoteInput{ProjectID: project, RequestID: uuid.NewString(), Items: inputs})
		t.Fatalf("source quote=%d %s: %v", res.Code, res.Body, err)
	}
	res = publicOperationRequest(t, router, "GET", "/api/projects/"+project.String()+"/operations?canvas_id="+canvasID.String()+"&node_id="+nodeID.String()+"&row_id="+rowA.String(), "", "")
	var page operationhttp.TaskPageResponse
	if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Items[0].Source == nil || page.Items[0].Source.Revision != 3 || *page.Items[0].Source.RowID != rowA {
		t.Fatalf("row restore=%d %s", res.Code, res.Body)
	}
	if err := database.Exec(`UPDATE canvas.document SET revision=4 WHERE id=?::uuid`, canvasID).Error; err != nil {
		t.Fatal(err)
	}
	res = publicOperationRequest(t, router, "POST", "/api/projects/"+project.String()+"/free-operations", item(rowA, 3), uuid.NewString())
	if res.Code != 409 {
		t.Fatalf("stale canvas quoted=%d %s", res.Code, res.Body)
	}
	res = publicOperationRequest(t, router, "POST", "/api/batches/"+batch.BatchID.String()+"/confirm", `{"project_id":"`+project.String()+`"}`, uuid.NewString())
	if res.Code != 200 {
		t.Fatalf("frozen batch confirm=%d %s", res.Code, res.Body)
	}
	if err := database.Exec(`UPDATE operation.batch SET status='running' WHERE id=?::uuid`, batch.BatchID).Error; err != nil {
		t.Fatal(err)
	}
	store := pgoperation.NewStore(database)
	if acquired, err := store.AcquireBatchLaunch(t.Context(), *batch.BatchID, *batch.Items[0].OperationID); err != nil || !acquired {
		t.Fatalf("first saved slot=%t %v", acquired, err)
	}
	if acquired, err := store.AcquireBatchLaunch(t.Context(), *batch.BatchID, *batch.Items[1].OperationID); err != nil || acquired {
		t.Fatalf("saved concurrency bypassed=%t %v", acquired, err)
	}
}
