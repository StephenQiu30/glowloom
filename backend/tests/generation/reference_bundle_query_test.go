package generation_test

import (
	"context"
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

type referenceBundleReader struct {
	read func(context.Context, app.Actor, string, string) (domain.ReferenceBundleInputCollection, error)
}

func (r referenceBundleReader) ReadReferenceBundleInputs(ctx context.Context, actor app.Actor, project, execution string) (domain.ReferenceBundleInputCollection, error) {
	return r.read(ctx, actor, project, execution)
}

func TestReferenceBundleQueryAndHTTPBoundary(t *testing.T) {
	facts := referenceBundleFacts(t, false)
	result, err := domain.BuildReferenceBundleInputs(facts)
	if err != nil {
		t.Fatal(err)
	}
	actor := app.Actor{UserID: uuid.NewString(), TokenVersion: 1}
	for _, test := range []struct {
		name, suffix, body string
		authErr, readErr   error
		code, reads        int
	}{
		{name: "read", code: 200, reads: 1},
		{name: "query", suffix: "?retry=true", code: 422},
		{name: "empty_query", suffix: "?", code: 422},
		{name: "body", body: "{}", code: 422},
		{name: "auth", authErr: errors.New("private auth"), code: 401},
		{name: "forbidden", readErr: &app.Error{Code: "forbidden", Message: "Forbidden", Status: 403}, code: 403, reads: 1},
		{name: "unresolved", readErr: &app.Error{Code: "reference_results_unresolved", Message: "Unresolved", Status: 409}, code: 409, reads: 1},
		{name: "failure", readErr: errors.New("private credential"), code: 500, reads: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			reads := 0
			reader := referenceBundleReader{read: func(ctx context.Context, who app.Actor, project, execution string) (domain.ReferenceBundleInputCollection, error) {
				reads++
				if who != actor || project != facts.ProjectID || execution != facts.Job.ExecutionRef.ID {
					t.Fatal("query lost scope")
				}
				return result, test.readErr
			}}
			mux := http.NewServeMux()
			adapter.NewReferenceBundleHandler(app.NewReferenceBundleQuery(reader), referenceProgressAuthenticator{actor: actor, err: test.authErr}).Register(mux)
			request := httptest.NewRequest(http.MethodGet, "/api/projects/"+facts.ProjectID+"/reference-executions/"+facts.Job.ExecutionRef.ID+"/bundle-inputs"+test.suffix, strings.NewReader(test.body))
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != test.code || reads != test.reads || response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), "private") {
				t.Fatalf("query boundary: %d reads=%d", response.Code, reads)
			}
		})
	}
	for _, fault := range []string{"actor", "token", "project", "execution", "identity", "content", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			who, project, execution := actor, facts.ProjectID, facts.Job.ExecutionRef.ID
			ctx := context.Background()
			if fault == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			read := false
			reader := referenceBundleReader{read: func(got context.Context, _ app.Actor, _, _ string) (domain.ReferenceBundleInputCollection, error) {
				read = true
				if got != ctx {
					t.Fatal("context replaced")
				}
				copy := result
				if fault == "content" {
					copy.ContentHash = strings.Repeat("f", 64)
				}
				return copy, got.Err()
			}}
			switch fault {
			case "actor":
				who.UserID = "invalid"
			case "token":
				who.TokenVersion = 0
			case "project":
				project = "invalid"
			case "execution":
				execution = "latest"
			case "identity":
				execution = uuid.NewString()
			}
			value, err := app.NewReferenceBundleQuery(reader).Get(ctx, who, project, execution)
			if err == nil || len(value.Bundles) != 0 {
				t.Fatal("invalid query returned data")
			}
			if fault == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost")
			}
			if fault != "identity" && fault != "content" && fault != "cancelled" && read {
				t.Fatal("invalid query read facts")
			}
		})
	}
}
