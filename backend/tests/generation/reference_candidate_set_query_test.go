package generation_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adapter "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/httpapi"
	app "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/google/uuid"
)

type candidateSetPersistence struct {
	value domain.ReferenceCandidateSet
	err   error
	calls int
}

func (p *candidateSetPersistence) MaterializeReferenceCandidateSet(context.Context, app.Actor, app.ReferenceCandidateSetCommand) (domain.ReferenceCandidateSet, error) {
	p.calls++
	return p.value, p.err
}
func (p *candidateSetPersistence) ReadReferenceCandidateSet(context.Context, app.Actor, string, string) (domain.ReferenceCandidateSet, error) {
	p.calls++
	return p.value, p.err
}

func TestReferenceCandidateSetServiceAndHTTPFailClosed(t *testing.T) {
	value, err := domain.BuildReferenceCandidateSet(referenceBundleFacts(t, true), []domain.ReferenceCandidateBundle{})
	if err != nil {
		t.Fatal(err)
	}
	actor := app.Actor{UserID: uuid.NewString(), TokenVersion: 1}
	for _, method := range []string{"POST", "GET"} {
		for _, fault := range []string{"success", "auth", "actor", "scope", "corrupt", "error", "query", "empty_query", "body", "missing", "null", "duplicate_field", "oversize", "wrong_progress", "wrong_execution"} {
			t.Run(method+"_"+fault, func(t *testing.T) {
				p := &candidateSetPersistence{value: value}
				service := app.NewReferenceCandidateSetService(p)
				who, project := actor, value.ProjectID
				var authErr error
				want, calls, suffix := 200, 1, ""
				body := ""
				if method == "POST" {
					raw, _ := json.Marshal(map[string]any{"execution_hash": value.ExecutionRef.ContentHash, "expected_progress_hash": value.ExecutionProgressHash, "candidate_bundle_refs": []domain.GenerationRevisionRef{}})
					body, want = string(raw), 201
				}
				switch fault {
				case "auth":
					authErr = errors.New("private credential")
					want, calls = 401, 0
				case "actor":
					who.TokenVersion = 0
					want, calls = 422, 0
				case "scope":
					project = uuid.NewString()
					want = 409
				case "corrupt":
					p.value.ContentHash = strings.Repeat("e", 64)
					want = 409
				case "error":
					p.err = errors.New("private credential")
					want = 500
				case "query":
					suffix = "?latest=true"
					want, calls = 422, 0
				case "empty_query":
					suffix = "?"
					want, calls = 422, 0
				case "body":
					body = `{ "selected": true }`
					want, calls = 422, 0
				case "missing":
					if method == "GET" {
						return
					}
					body = strings.Replace(body, `"candidate_bundle_refs":[],`, "", 1)
					want, calls = 422, 0
				case "null":
					if method == "GET" {
						return
					}
					body = strings.Replace(body, `"candidate_bundle_refs":[]`, `"candidate_bundle_refs":null`, 1)
					want, calls = 422, 0
				case "duplicate_field":
					if method == "GET" {
						return
					}
					body = strings.TrimSuffix(body, "}") + `,"candidate_bundle_refs":[]}`
					want, calls = 422, 0
				case "oversize":
					body = strings.Repeat(" ", 16*1024+1)
					want, calls = 422, 0
				case "wrong_progress":
					if method == "GET" {
						return
					}
					body = strings.ReplaceAll(body, value.ExecutionProgressHash, strings.Repeat("f", 64))
					want = 409
				case "wrong_execution":
					if method == "GET" {
						return
					}
					body = strings.ReplaceAll(body, value.ExecutionRef.ContentHash, strings.Repeat("f", 64))
					want = 409
				}
				path := "/api/projects/" + project + "/reference-candidate-sets/" + value.ID
				if method == "POST" {
					path = "/api/projects/" + project + "/reference-executions/" + value.ExecutionRef.ID + "/candidate-sets"
				}
				mux := http.NewServeMux()
				adapter.NewReferenceCandidateSetHandler(service, referenceProgressAuthenticator{actor: who, err: authErr}).Register(mux)
				response := httptest.NewRecorder()
				mux.ServeHTTP(response, httptest.NewRequest(method, path+suffix, strings.NewReader(body)))
				if response.Code != want || p.calls != calls || response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), "private") {
					t.Fatalf("Set HTTP status=%d calls=%d", response.Code, p.calls)
				}
			})
		}
	}
}
