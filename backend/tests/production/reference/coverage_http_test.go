package reference_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	referencehttp "github.com/StephenQiu30/lanverse/backend/internal/production/reference/adapter/httpapi"
	referenceapp "github.com/StephenQiu30/lanverse/backend/internal/production/reference/application"
)

type referenceCoverageHTTPQuery struct {
	matrix referenceapp.ReferenceCoverageMatrix
	detail referenceapp.ReferenceTargetDetail
}

func (query referenceCoverageHTTPQuery) GetMatrix(
	context.Context,
	referenceapp.Actor,
	string,
) (referenceapp.ReferenceCoverageMatrix, error) {
	return query.matrix, nil
}

func (query referenceCoverageHTTPQuery) GetTarget(
	context.Context,
	referenceapp.Actor,
	string,
	string,
) (referenceapp.ReferenceTargetDetail, error) {
	return query.detail, nil
}

type referenceCoverageAuthenticator struct{}

func (referenceCoverageAuthenticator) Authenticate(*http.Request) (authentication.Claims, error) {
	return authentication.Claims{UserID: uuid.NewString(), TokenVersion: 1}, nil
}

func TestReferenceCoverageHTTPIsTypedNoStoreAndRejectsSelectors(t *testing.T) {
	projectID, targetID := uuid.NewString(), uuid.NewString()
	query := referenceCoverageHTTPQuery{
		matrix: referenceapp.ReferenceCoverageMatrix{
			SchemaVersion: referenceapp.ReferenceCoverageMatrixSchema,
			PlanVersionID: uuid.NewString(), PlanRevision: 1,
			PlanContentHash: referenceCoverageHash("plan"), ContentHash: referenceCoverageHash("matrix"),
			Rows: []referenceapp.ReferenceCoverageRow{},
		},
		detail: referenceapp.ReferenceTargetDetail{
			SchemaVersion: referenceapp.ReferenceTargetDetailSchema,
			ContentHash:   referenceCoverageHash("detail"),
		},
	}
	mux := http.NewServeMux()
	referencehttp.NewReferenceCoverageHandler(query, referenceCoverageAuthenticator{}).Register(mux)

	for _, path := range []string{
		"/api/projects/" + projectID + "/reference-coverage",
		"/api/projects/" + projectID + "/reference-targets/" + targetID,
	} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("GET %s status=%d cache=%q body=%s", path, response.Code, response.Header().Get("Cache-Control"), response.Body.String())
		}
		var body struct {
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal(response.Body.Bytes(), &body) != nil || len(body.Data) == 0 {
			t.Fatalf("GET %s response=%s", path, response.Body.String())
		}
	}

	invalidRequests := []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID+"/reference-coverage?latest=true", nil),
		httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID+"/reference-coverage", strings.NewReader(`{}`)),
	}
	for _, request := range invalidRequests {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid query status=%d body=%s", response.Code, response.Body.String())
		}
	}
}
