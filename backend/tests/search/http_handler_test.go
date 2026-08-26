package search_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	searchhttp "github.com/StephenQiu30/lanverse/backend/internal/search/adapter/httpapi"
	searchapp "github.com/StephenQiu30/lanverse/backend/internal/search/application"
	search "github.com/StephenQiu30/lanverse/backend/internal/search/domain"
)

func TestSearchHTTPExposesOnlyBoundedTextQueriesAndExplicitStatus(t *testing.T) {
	service := &searchHTTPService{result: search.Result{Status: search.StatusDegraded, Stale: true, ErrorCode: "search_unavailable", Hits: []search.Hit{}}}
	mux := http.NewServeMux()
	searchhttp.New(service, searchHTTPAuthenticator{}).Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+searchProjectID+"/search/storygraph?q=%E7%A0%81%E5%A4%B4&limit=20", nil))
	if response.Code != http.StatusOK || service.storyGraphCalls != 1 || service.query.Text != "码头" || service.query.Limit != 20 ||
		!strings.Contains(response.Body.String(), `"status":"degraded"`) || !strings.Contains(response.Body.String(), `"hits":[]`) {
		t.Fatalf("unexpected Search HTTP response=%d %s query=%#v", response.Code, response.Body.String(), service.query)
	}

	invalid := httptest.NewRecorder()
	mux.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+searchProjectID+"/search/scripts?q=rain", nil))
	if invalid.Code != http.StatusUnprocessableEntity || service.scriptCalls != 0 {
		t.Fatalf("unbounded Search request reached application: %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestSearchHTTPRejectsUnauthenticatedRequestsBeforeApplication(t *testing.T) {
	service := &searchHTTPService{}
	mux := http.NewServeMux()
	searchhttp.New(service, searchHTTPAuthenticator{err: errors.New("bad token")}).Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+searchProjectID+"/search/scripts?q=rain&limit=10", nil))
	if response.Code != http.StatusUnauthorized || service.scriptCalls != 0 || !strings.Contains(response.Body.String(), `"code":"unauthenticated"`) {
		t.Fatalf("unauthenticated Search response=%d %s calls=%d", response.Code, response.Body.String(), service.scriptCalls)
	}
}

type searchHTTPAuthenticator struct{ err error }

func (value searchHTTPAuthenticator) Authenticate(*http.Request) (authentication.Claims, error) {
	return authentication.Claims{UserID: searchVersionID, TokenVersion: 3}, value.err
}

type searchHTTPService struct {
	result                       search.Result
	err                          error
	query                        searchapp.Query
	scriptCalls, storyGraphCalls int
}

func (value *searchHTTPService) SearchScripts(_ context.Context, _ searchapp.Actor, query searchapp.Query) (search.Result, error) {
	value.scriptCalls++
	value.query = query
	return value.result, value.err
}

func (value *searchHTTPService) SearchStoryGraph(_ context.Context, _ searchapp.Actor, query searchapp.Query) (search.Result, error) {
	value.storyGraphCalls++
	value.query = query
	return value.result, value.err
}
