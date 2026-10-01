package operation_test

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	operationhttp "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/http"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	operationdomain "github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	promptdomain "github.com/StephenQiu30/lanverse/backend/internal/prompt/domain"
)

func templateQuoteRouter(actor identityapp.Principal, database *gorm.DB) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	router.Use(func(c *gin.Context) { c.Set("principal", actor) })
	store := templateQuoteStore(database)
	operationhttp.NewHandler(operationhttp.Dependencies{FreeQuote: operationapp.NewCreateFreeQuoteCommand(store), BatchQuote: operationapp.NewCreateBatchFreeQuoteCommand(store), Query: operationapp.NewPublicQuery(store)}).Register(router.Group("/api"))
	return router
}

func TestTemplateQuoteHTTPClosedContractAndFrozenPreview(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	input := templateQuoteInput(projectID, seedTemplateQuoteCatalog(t, database, projectID))
	router := templateQuoteRouter(actor, database)
	request := operationhttp.QuoteItemRequest{ModelKey: input.ModelKey, Capability: input.Capability, Mode: input.Mode, Prompt: input.Prompt, Params: json.RawMessage(`{}`), OutputCount: 1, PromptTemplate: input.PromptTemplate}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/projects/" + projectID.String() + "/free-operations"
	response := publicOperationRequest(t, router, "POST", path, string(body), input.RequestID)
	var quote operationhttp.QuoteResponse
	if response.Code != 201 || json.Unmarshal(response.Body.Bytes(), &quote) != nil || quote.PromptPreparation == nil || quote.FinalPrompt == input.Prompt || !strings.Contains(quote.FinalPrompt, input.Prompt) {
		t.Fatal("HTTP quote lost explicit template preview", response.Code)
	}
	replay := publicOperationRequest(t, router, "POST", path, string(body), input.RequestID)
	if replay.Code != 201 || !samePublicHTTPJSON(response.Body.Bytes(), replay.Body.Bytes()) {
		t.Fatal("HTTP retry changed frozen template preview", replay.Code)
	}
	request.PromptTemplate = nil
	plainBody, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	changed := publicOperationRequest(t, router, "POST", path, string(plainBody), input.RequestID)
	if changed.Code != 409 || !strings.Contains(changed.Body.String(), "idempotency_conflict") {
		t.Fatal("same key switched from compiled to plain policy", changed.Code)
	}
	for _, malformed := range []string{
		strings.Replace(string(body), `"operation":"skill_draft"`, `"operation":"skill_draft","variables":{"项目名称":"伪造项目"}`, 1),
		strings.Replace(string(body), `"expected_customization_revision":0`, `"expected_customization_revision":null`, 1),
		strings.Replace(string(body), `"operation":"skill_draft"`, `"operation":"skill_draft","outline":{"chapter_count":"5","injected":"unsupported"}`, 1),
	} {
		bad := publicOperationRequest(t, router, "POST", path, malformed, uuid.NewString())
		if bad.Code != 422 {
			t.Fatal("HTTP accepted arbitrary variables, missing revision, or nested unknown fields", bad.Code)
		}
	}
	missing, _ := promptdomain.DefinitionFor("character_extract")
	request.PromptTemplate = input.PromptTemplate
	selection := *request.PromptTemplate
	selection.Operation, selection.ExpectedTemplateID = missing.Operation, missing.TemplateID
	request.PromptTemplate = &selection
	body, err = json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	missingEntity := publicOperationRequest(t, router, "POST", path, string(body), uuid.NewString())
	if missingEntity.Code != 409 || !strings.Contains(missingEntity.Body.String(), "context_unavailable") {
		t.Fatal("HTTP disguised missing real context", missingEntity.Code)
	}
	detail := publicOperationRequest(t, router, "GET", "/api/operations/"+quote.OperationID.String()+"?project_id="+projectID.String(), "", "")
	if detail.Code != 200 || !strings.Contains(detail.Body.String(), quote.PromptPreparation.ContentSHA256) {
		t.Fatal("HTTP refresh lost persisted preparation proof", detail.Code)
	}
}

func TestTemplateQuoteValidatesOriginalCanvasBeforeCompilation(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	modelKey := seedTemplateQuoteCatalog(t, database, projectID)
	input := templateQuoteInput(projectID, modelKey)
	modelID := uuid.MustParse(strings.TrimPrefix(modelKey, "quote-model-"))
	canvasID, nodeID := uuid.New(), uuid.New()
	config := canvasdomain.NodeConfig{Generation: &canvasdomain.GenerationConfig{
		Version: 1, Capability: input.Capability, Mode: input.Mode, ModelProfileID: &modelID,
		Prompt: input.Prompt, Params: json.RawMessage(`{}`), OutputCount: 1, Inputs: []canvasdomain.GenerationReference{},
	}}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO canvas.document(id,project_id,name,revision) VALUES(?,?,'合成模板来源',4)`, canvasID, projectID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO canvas.node(id,document_id,node_type,node_action,config,x,y) VALUES(?,?,'generation','generate',?::jsonb,0,0)`, nodeID, canvasID, string(encoded)).Error; err != nil {
		t.Fatal(err)
	}
	input.Params = json.RawMessage(`{}`)
	input.Source = &operationdomain.CanvasSource{CanvasID: canvasID, NodeID: nodeID, Revision: 4}
	quote, err := templateQuoteStore(database).CreateFreeQuote(t.Context(), actor, input)
	if err != nil || quote.PromptPreparation == nil || quote.FinalPrompt == input.Prompt {
		t.Fatal("source verification ran against compiled text rather than saved original", err)
	}
	input.RequestID = uuid.NewString()
	input.Prompt = "客户端篡改保存输入"
	if _, err := templateQuoteStore(database).CreateFreeQuote(t.Context(), actor, input); !errors.Is(err, operationapp.ErrInvalidFreeQuote) {
		t.Fatal("template compilation bypassed saved source prompt", err)
	}
	input.Prompt = config.Generation.Prompt
	input.Source.Revision = 3
	if _, err := templateQuoteStore(database).CreateFreeQuote(t.Context(), actor, input); !errors.Is(err, operationapp.ErrQuoteSourceStale) {
		t.Fatal("template compilation bypassed canvas revision", err)
	}
}

func TestPlainQuoteRequestRetainsHistoricalFingerprintBytes(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	input := templateQuoteInput(projectID, seedTemplateQuoteCatalog(t, database, projectID))
	input.PromptTemplate = nil
	input.Params = json.RawMessage(`{}`)
	if _, err := templateQuoteStore(database).CreateFreeQuote(t.Context(), actor, input); err != nil {
		t.Fatal(err)
	}
	// This is the pre-template public request encoding, including its null fields and order.
	legacy := fmt.Sprintf(`{"kind":"free_single","project_id":"%s","model_key":"%s","capability":"text.structured","mode":"structured","prompt":"用户原始技能要求","media_inputs":null,"params":{},"output_count":1,"force_regenerate":false,"source":null}`, projectID, input.ModelKey)
	want := sha256.Sum256([]byte(legacy))
	var got struct{ Fingerprint []byte }
	if err := database.Raw(`SELECT fingerprint FROM operation.quote_request WHERE org_id=? AND actor_id=? AND request_id=?`, actor.OrgID, actor.ID, input.RequestID).Scan(&got).Error; err != nil {
		t.Fatal(err)
	}
	if string(got.Fingerprint) != string(want[:]) {
		t.Fatal("nil template changed historical request fingerprint bytes")
	}
}
