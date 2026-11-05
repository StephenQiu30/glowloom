package operation_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/http"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

type publicQuoteStore struct {
	input operationapp.CreateFreeQuoteInput
	calls int
}

func (s *publicQuoteStore) CreateFreeQuote(_ context.Context, _ identityapp.Principal, input operationapp.CreateFreeQuoteInput) (operationapp.CreateFreeQuoteResult, error) {
	s.input = input
	s.calls++
	return operationapp.CreateFreeQuoteResult{OperationID: uuid.New(), QuoteMicros: 12, QuoteDetail: json.RawMessage(`{"outputs":1}`), AvailableMicros: 100, ExpiresAt: time.Now().Add(time.Minute), Confirmable: true}, nil
}

func TestPublicFreeQuotePreservesRequestIdentityAndReferenceRoles(t *testing.T) {
	project, key, asset := uuid.New(), uuid.NewString(), uuid.New()
	canvasID, nodeID := uuid.New(), uuid.New()
	store := &publicQuoteStore{}
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	router.Use(func(c *gin.Context) {
		c.Set("principal", identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: identitydomain.RoleProducer})
	})
	operationhttp.NewHandler(operationhttp.Dependencies{FreeQuote: operationapp.NewCreateFreeQuoteCommand(store)}).Register(router.Group("/api"))
	body := `{"model_key":"seedream","capability":"image.generate","mode":"image_to_image","prompt":"清晨", "params":{"seed":5},"output_count":1,"media_inputs":[{"role":"subject","media_asset_id":"` + asset.String() + `"}],"source":{"canvas_id":"` + canvasID.String() + `","node_id":"` + nodeID.String() + `","revision":7}}`
	request := httptest.NewRequestWithContext(t.Context(), "POST", "/api/projects/"+project.String()+"/free-operations", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Idempotency-Key", key)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 201 || store.calls != 1 || store.input.ProjectID != project || store.input.RequestID != key || len(store.input.MediaInputs) != 1 || store.input.MediaInputs[0].MediaAssetID != asset || store.input.Source == nil || store.input.Source.CanvasID != canvasID || store.input.Source.NodeID != nodeID || store.input.Source.Revision != 7 || !strings.Contains(response.Body.String(), `"quote_micros":12`) {
		t.Fatalf("quote=%d %s input=%+v", response.Code, response.Body, store.input)
	}
}

func TestPublicFreeQuoteRejectsUnknownFieldsAndWrongOrigin(t *testing.T) {
	store := &publicQuoteStore{}
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	operationhttp.NewHandler(operationhttp.Dependencies{FreeQuote: operationapp.NewCreateFreeQuoteCommand(store)}).Register(router.Group("/api"))
	for _, tc := range []struct {
		body, origin string
		want         int
	}{{`{"provider_secret":"must-not-be-accepted"}`, "http://localhost:3000", 422}, {`{}`, "https://other.invalid", 403}} {
		req := httptest.NewRequestWithContext(t.Context(), "POST", "/api/projects/"+uuid.NewString()+"/free-operations", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Idempotency-Key", uuid.NewString())
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != tc.want || store.calls != 0 {
			t.Fatalf("unsafe quote reached persistence: %d %s", res.Code, res.Body)
		}
	}
}

func TestCurrentQuoteEndpointsRejectRemovedOrchestrationFields(t *testing.T) {
	store := &publicQuoteStore{}
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	router.Use(func(c *gin.Context) {
		c.Set("principal", identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: identitydomain.RoleProducer})
	})
	operationhttp.NewHandler(operationhttp.Dependencies{FreeQuote: operationapp.NewCreateFreeQuoteCommand(store)}).Register(router.Group("/api"))
	for _, field := range []string{
		`"agent_context":{"session_id":"11111111-1111-4111-8111-111111111111","turn_id":"22222222-2222-4222-8222-222222222222","phase":"image","policy":"image-agent-json-v1"}`,
		`"single_image_contract":"image-agent-json-v1"`,
	} {
		for _, endpoint := range []string{"free-operations", "quotes"} {
			body := `{"model_key":"fixture","capability":"image.generate","mode":"text_to_image","prompt":"fixture","params":{},"output_count":1,"force_regenerate":true,` + field + `}`
			if endpoint == "quotes" {
				body = `{"items":[` + body + `]}`
			}
			req := httptest.NewRequestWithContext(t.Context(), "POST", "/api/projects/"+uuid.NewString()+"/"+endpoint, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://localhost:3000")
			req.Header.Set("Idempotency-Key", uuid.NewString())
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			if res.Code != 422 || store.calls != 0 {
				t.Fatalf("removed input accepted by %s: %d %s", endpoint, res.Code, res.Body)
			}
		}
	}
}
