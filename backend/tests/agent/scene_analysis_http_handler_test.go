package agent_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	agenthttp "github.com/StephenQiu30/lanverse/backend/internal/agent/adapter/httpapi"
	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
)

const (
	queryCandidateProjectID  = "42000000-0000-4000-8000-000000000001"
	queryCandidateRevisionID = "42000000-0000-4000-8000-000000000002"
)

func TestSceneAnalysisCandidateHTTPAuthorizesProjectAndReturnsExactRevision(t *testing.T) {
	candidates := &sceneCandidateHTTPSource{candidate: agentapp.Candidate{
		ID: queryCandidateRevisionID, WorkspaceID: "42000000-0000-4000-8000-000000000003",
		ProjectID: queryCandidateProjectID, StageKey: "extract_scene_facts", ProfileKey: "default",
		StageInstanceKey: strings.Repeat("a", 64), Revision: 1, CandidateType: "scene_fact_candidate",
		Candidate:            []byte(`{"source_version_id":"42000000-0000-4000-8000-000000000004","source_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","span_candidate_revision_id":"42000000-0000-4000-8000-000000000005","span_candidate_revision_hash":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","scenes":[],"review_issues":[]}`),
		CandidateContentHash: strings.Repeat("d", 64), CandidateRevisionHash: strings.Repeat("e", 64),
		SourceInvocationID: "42000000-0000-4000-8000-000000000006", SourceResultID: "42000000-0000-4000-8000-000000000007",
		SourceResultHash: strings.Repeat("f", 64), CreatedAt: time.Unix(10, 0).UTC(),
	}}
	projects := &sceneCandidateProjectSource{}
	mux := http.NewServeMux()
	agenthttp.New(candidates, projects, sceneCandidateHTTPAuthenticator{}).Register(mux)

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(
		http.MethodGet,
		"/api/projects/"+queryCandidateProjectID+"/scene-analysis-candidates/"+queryCandidateRevisionID,
		nil,
	))
	if response.Code != http.StatusOK || projects.actor.TokenVersion != 5 ||
		projects.projectID != queryCandidateProjectID || candidates.projectID != queryCandidateProjectID ||
		candidates.candidateID != queryCandidateRevisionID ||
		!strings.Contains(response.Body.String(), `"candidate_type":"scene_fact_candidate"`) {
		t.Fatalf("response=%d %s projects=%#v candidates=%#v", response.Code, response.Body.String(), projects, candidates)
	}
}

func TestSceneAnalysisCandidateHTTPDoesNotQueryAcrossFailedAuthorization(t *testing.T) {
	candidates := &sceneCandidateHTTPSource{}
	projects := &sceneCandidateProjectSource{err: &projectapp.Error{Code: "not_found", Message: "Project not found", Status: 404}}
	mux := http.NewServeMux()
	agenthttp.New(candidates, projects, sceneCandidateHTTPAuthenticator{}).Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(
		http.MethodGet,
		"/api/projects/"+queryCandidateProjectID+"/scene-analysis-candidates/"+queryCandidateRevisionID,
		nil,
	))
	if response.Code != http.StatusNotFound || candidates.calls != 0 {
		t.Fatalf("response=%d %s candidate calls=%d", response.Code, response.Body.String(), candidates.calls)
	}
}

type sceneCandidateHTTPAuthenticator struct{ err error }

func (authenticator sceneCandidateHTTPAuthenticator) Authenticate(*http.Request) (authentication.Claims, error) {
	if authenticator.err != nil {
		return authentication.Claims{}, authenticator.err
	}
	return authentication.Claims{UserID: "42000000-0000-4000-8000-000000000008", TokenVersion: 5}, nil
}

type sceneCandidateHTTPSource struct {
	candidate              agentapp.Candidate
	projectID, candidateID string
	calls                  int
}

func (source *sceneCandidateHTTPSource) GetCandidate(
	_ context.Context,
	projectID, candidateID string,
) (agentapp.Candidate, error) {
	source.projectID, source.candidateID, source.calls = projectID, candidateID, source.calls+1
	return source.candidate, nil
}

type sceneCandidateProjectSource struct {
	actor     projectapp.Actor
	projectID string
	err       error
}

func (source *sceneCandidateProjectSource) Get(
	_ context.Context,
	actor projectapp.Actor,
	projectID string,
) (projectdomain.Project, error) {
	source.actor, source.projectID = actor, projectID
	if source.err != nil {
		return projectdomain.Project{}, source.err
	}
	return projectdomain.Project{ID: projectID}, nil
}
