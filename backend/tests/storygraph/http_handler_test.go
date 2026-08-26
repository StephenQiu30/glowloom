package storygraph_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	storygraphhttp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/adapter/httpapi"
	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

const queryProjectID = "50000000-0000-0000-0000-000000000001"

func TestStoryGraphHTTPExposesBoundedVersionLensTraceAndDiffQueries(t *testing.T) {
	service := &storyGraphHTTPService{
		version:  storygraphapp.VersionResult{Version: storygraph.Version{ID: "50000000-0000-0000-0000-000000000002", ProjectID: queryProjectID, VersionNo: 2, ContentHash: hashText("version"), Nodes: []storygraph.Node{{StoryNodeKey: "sgn_node"}}}, CompiledFrom: []storygraph.OwnerHeadRef{}, Stale: true},
		subgraph: storygraphapp.SubgraphResult{VersionID: "50000000-0000-0000-0000-000000000002", VersionNo: 2, ContentHash: hashText("version"), Nodes: []storygraph.Node{}, Edges: []storygraph.Edge{}, ResultHash: hashText("lens")},
		diff:     storygraphapp.DiffResult{BaseVersionID: "50000000-0000-0000-0000-000000000003", TargetVersionID: "50000000-0000-0000-0000-000000000002", NodeChanges: []storygraphapp.NodeChange{}, EdgeChanges: []storygraphapp.EdgeChange{}, ResultHash: hashText("diff")},
	}
	mux := http.NewServeMux()
	storygraphhttp.New(service, storyGraphHTTPAuthenticator{}).Register(mux)

	versionResponse := httptest.NewRecorder()
	mux.ServeHTTP(versionResponse, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+queryProjectID+"/storygraph/current", nil))
	if versionResponse.Code != http.StatusOK || strings.Contains(versionResponse.Body.String(), `"nodes"`) || !strings.Contains(versionResponse.Body.String(), `"node_count":1`) || !strings.Contains(versionResponse.Body.String(), `"stale":true`) {
		t.Fatalf("version response=%d %s", versionResponse.Code, versionResponse.Body.String())
	}

	lensResponse := httptest.NewRecorder()
	mux.ServeHTTP(lensResponse, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+queryProjectID+"/storygraph/versions/current/lens?lens=outline&scope_kind=project&scope_id="+queryProjectID+"&depth=2&limit=20", nil))
	if lensResponse.Code != http.StatusOK || service.lens.VersionRef != "current" || service.lens.Depth != 2 || service.actor.TokenVersion != 4 || !strings.Contains(lensResponse.Body.String(), `"result_hash"`) {
		t.Fatalf("lens response=%d %s query=%#v actor=%#v", lensResponse.Code, lensResponse.Body.String(), service.lens, service.actor)
	}

	traceResponse := httptest.NewRecorder()
	mux.ServeHTTP(traceResponse, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+queryProjectID+"/storygraph/versions/current/nodes/sgn_node/trace?direction=downstream&depth=3&limit=20", nil))
	if traceResponse.Code != http.StatusOK || service.trace.StoryNodeKey != "sgn_node" || service.trace.Direction != "downstream" {
		t.Fatalf("trace response=%d %s query=%#v", traceResponse.Code, traceResponse.Body.String(), service.trace)
	}

	diffResponse := httptest.NewRecorder()
	mux.ServeHTTP(diffResponse, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+queryProjectID+"/storygraph/diff?base_version_id=50000000-0000-0000-0000-000000000003&target_version_id=50000000-0000-0000-0000-000000000002&limit=20", nil))
	if diffResponse.Code != http.StatusOK || service.diffQuery.BaseVersionID == "" || !strings.Contains(diffResponse.Body.String(), `"node_changes":[]`) {
		t.Fatalf("diff response=%d %s query=%#v", diffResponse.Code, diffResponse.Body.String(), service.diffQuery)
	}
}

func TestStoryGraphHTTPRequiresExplicitBoundsAndAuthentication(t *testing.T) {
	service := &storyGraphHTTPService{}
	mux := http.NewServeMux()
	storygraphhttp.New(service, storyGraphHTTPAuthenticator{}).Register(mux)
	invalid := httptest.NewRecorder()
	mux.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+queryProjectID+"/storygraph/versions/current/lens?lens=outline&scope_kind=project&scope_id="+queryProjectID, nil))
	if invalid.Code != http.StatusUnprocessableEntity || service.calls != 0 {
		t.Fatalf("unbounded Lens response=%d %s calls=%d", invalid.Code, invalid.Body.String(), service.calls)
	}

	unauthorizedMux := http.NewServeMux()
	storygraphhttp.New(service, storyGraphHTTPAuthenticator{err: errors.New("invalid token")}).Register(unauthorizedMux)
	unauthorized := httptest.NewRecorder()
	unauthorizedMux.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+queryProjectID+"/storygraph/current", nil))
	if unauthorized.Code != http.StatusUnauthorized || !strings.Contains(unauthorized.Body.String(), `"code":"unauthenticated"`) {
		t.Fatalf("unauthorized response=%d %s", unauthorized.Code, unauthorized.Body.String())
	}
}

type storyGraphHTTPAuthenticator struct{ err error }

func (authenticator storyGraphHTTPAuthenticator) Authenticate(*http.Request) (authentication.Claims, error) {
	return authentication.Claims{UserID: "50000000-0000-0000-0000-000000000099", TokenVersion: 4}, authenticator.err
}

type storyGraphHTTPService struct {
	version   storygraphapp.VersionResult
	subgraph  storygraphapp.SubgraphResult
	diff      storygraphapp.DiffResult
	actor     storygraphapp.Actor
	lens      storygraphapp.LensQuery
	trace     storygraphapp.TraceQuery
	diffQuery storygraphapp.DiffQuery
	calls     int
}

func (service *storyGraphHTTPService) Version(_ context.Context, actor storygraphapp.Actor, _ storygraphapp.VersionQuery) (storygraphapp.VersionResult, error) {
	service.actor, service.calls = actor, service.calls+1
	return service.version, nil
}

func (service *storyGraphHTTPService) Lens(_ context.Context, actor storygraphapp.Actor, query storygraphapp.LensQuery) (storygraphapp.SubgraphResult, error) {
	service.actor, service.lens, service.calls = actor, query, service.calls+1
	return service.subgraph, nil
}

func (service *storyGraphHTTPService) Trace(_ context.Context, actor storygraphapp.Actor, query storygraphapp.TraceQuery) (storygraphapp.SubgraphResult, error) {
	service.actor, service.trace, service.calls = actor, query, service.calls+1
	return service.subgraph, nil
}

func (service *storyGraphHTTPService) Diff(_ context.Context, actor storygraphapp.Actor, query storygraphapp.DiffQuery) (storygraphapp.DiffResult, error) {
	service.actor, service.diffQuery, service.calls = actor, query, service.calls+1
	return service.diff, nil
}
