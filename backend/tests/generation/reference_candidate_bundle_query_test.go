package generation_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adapter "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/httpapi"
	app "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/google/uuid"
)

type candidateBundlePersistence struct {
	value domain.ReferenceCandidateBundle
	err   error
	calls int
}

func (p *candidateBundlePersistence) MaterializeReferenceCandidateBundle(context.Context, app.Actor, app.ReferenceCandidateBundleCommand) (domain.ReferenceCandidateBundle, error) {
	p.calls++
	return p.value, p.err
}
func (p *candidateBundlePersistence) ReadReferenceCandidateBundle(context.Context, app.Actor, string, string) (domain.ReferenceCandidateBundle, error) {
	p.calls++
	return p.value, p.err
}

func TestReferenceCandidateBundleServiceAndHTTPFailClosed(t *testing.T) {
	facts := referenceBundleFacts(t, false)
	inputs, err := domain.BuildReferenceBundleInputs(facts)
	if err != nil {
		t.Fatal(err)
	}
	value, err := domain.BuildReferenceCandidateBundle(facts.WorkspaceID, facts.ProjectID, inputs, 0, domain.GenerationRevisionRef{ID: uuid.NewString(), Revision: 1, ContentHash: strings.Repeat("d", 64)}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	actor := app.Actor{UserID: uuid.NewString(), TokenVersion: 1}
	for _, mode := range []string{"success", "body", "query", "empty_query", "auth", "wrong_id", "wrong_project", "corrupt", "error", "invalid_actor"} {
		t.Run(mode, func(t *testing.T) {
			p := &candidateBundlePersistence{value: value}
			service := app.NewReferenceCandidateBundleService(p)
			who := actor
			project, id, body, suffix := value.ProjectID, value.ID, "", ""
			var authErr error
			want, calls := 200, 1
			switch mode {
			case "body":
				body = "{}"
				want, calls = 422, 0
			case "query":
				suffix = "?approve=true"
				want, calls = 422, 0
			case "empty_query":
				suffix = "?"
				want, calls = 422, 0
			case "auth":
				authErr = errors.New("private credential")
				want, calls = 401, 0
			case "wrong_id":
				id = uuid.NewString()
				want = 409
			case "wrong_project":
				project = uuid.NewString()
				want = 409
			case "corrupt":
				p.value.ContentHash = strings.Repeat("f", 64)
				want = 409
			case "error":
				p.err = errors.New("private credential")
				want = 500
			case "invalid_actor":
				who.TokenVersion = 0
				want, calls = 422, 0
			}
			mux := http.NewServeMux()
			adapter.NewReferenceCandidateBundleHandler(service, referenceProgressAuthenticator{actor: who, err: authErr}).Register(mux)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/projects/"+project+"/reference-candidate-bundles/"+id+suffix, strings.NewReader(body)))
			if response.Code != want || p.calls != calls || response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), "private") {
				t.Fatalf("HTTP boundary status=%d calls=%d", response.Code, p.calls)
			}
		})
	}
	for _, fault := range []string{"actor", "token", "execution", "review", "scope", "returned_scope", "returned_review", "returned_hash"} {
		p := &candidateBundlePersistence{value: value}
		command := app.ReferenceCandidateBundleCommand{WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, ExecutionRef: value.ExecutionRef, VisionReviewRef: value.VisionReviewRef}
		who := actor
		switch fault {
		case "actor":
			who.UserID = "bad"
		case "token":
			who.TokenVersion = 0
		case "execution":
			command.ExecutionRef.Revision = 2
		case "review":
			command.VisionReviewRef.Revision = 2
		case "scope":
			command.WorkspaceID = "bad"
		case "returned_scope":
			command.WorkspaceID = uuid.NewString()
		case "returned_review":
			command.VisionReviewRef.ID = uuid.NewString()
		case "returned_hash":
			p.value.ContentHash = strings.Repeat("a", 64)
		}
		if _, err := app.NewReferenceCandidateBundleService(p).Materialize(context.Background(), who, command); err == nil {
			t.Fatalf("accepted %s", fault)
		}
	}
}
