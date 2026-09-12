package generation_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	generationhttp "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/google/uuid"
)

type referenceProgressReader struct {
	read func(context.Context, application.Actor, string, string) (domain.ReferenceJobProgress, error)
}

func (reader referenceProgressReader) ReadReferenceExecutionProgress(ctx context.Context, actor application.Actor, project, execution string) (domain.ReferenceJobProgress, error) {
	return reader.read(ctx, actor, project, execution)
}

type referenceProgressAuthenticator struct {
	actor application.Actor
	err   error
}

func (auth referenceProgressAuthenticator) Authenticate(*http.Request) (authentication.Claims, error) {
	return authentication.Claims{UserID: auth.actor.UserID, TokenVersion: auth.actor.TokenVersion}, auth.err
}

func TestReferenceExecutionQueryAndHTTPBoundary(t *testing.T) {
	job, calls, states := referenceProgressFixture(t, []string{"SUCCEEDED", "PENDING", "PENDING"})
	progress, err := domain.BuildReferenceJobProgress(job, calls, states)
	if err != nil {
		t.Fatal(err)
	}
	actor := application.Actor{UserID: uuid.NewString(), TokenVersion: 7}
	project := uuid.NewString()
	for _, test := range []struct {
		name, suffix, body string
		authErr, readErr   error
		code               int
		read               bool
	}{
		{name: "success", code: 200, read: true},
		{name: "unauthenticated", authErr: errors.New("private auth"), code: 401},
		{name: "query", suffix: "?retry=true", code: 422},
		{name: "empty_query", suffix: "?", code: 422},
		{name: "body", body: `{}`, code: 422},
		{name: "forbidden", readErr: &application.Error{Code: "forbidden", Message: "Forbidden", Status: 403}, code: 403, read: true},
		{name: "internal", readErr: errors.New("private staging/reference/credential"), code: 500, read: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			reads := 0
			reader := referenceProgressReader{read: func(ctx context.Context, gotActor application.Actor, gotProject, execution string) (domain.ReferenceJobProgress, error) {
				reads++
				if gotActor != actor || gotProject != project || execution != job.ExecutionRef.ID {
					t.Fatal("lost query identity")
				}
				return progress, test.readErr
			}}
			mux := http.NewServeMux()
			generationhttp.NewReferenceExecutionHandler(application.NewReferenceExecutionQuery(reader), referenceProgressAuthenticator{actor, test.authErr}).Register(mux)
			request := httptest.NewRequest(http.MethodGet, "/api/projects/"+project+"/reference-executions/"+job.ExecutionRef.ID+test.suffix, strings.NewReader(test.body))
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != test.code || response.Header().Get("Cache-Control") != "no-store" || (reads == 1) != test.read {
				t.Fatalf("code=%d reads=%d body=%s", response.Code, reads, response.Body)
			}
			if strings.Contains(response.Body.String(), "private") {
				t.Fatal("private error leaked")
			}
		})
	}
	for _, fault := range []string{"actor", "token", "project", "execution", "reader_error", "identity", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			gotActor, gotProject, execution := actor, project, job.ExecutionRef.ID
			read := false
			ctx := context.Background()
			if fault == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			reader := referenceProgressReader{read: func(received context.Context, _ application.Actor, _, _ string) (domain.ReferenceJobProgress, error) {
				read = true
				if received != ctx {
					t.Fatal("context replaced")
				}
				if fault == "cancelled" {
					return progress, received.Err()
				}
				if fault == "reader_error" {
					return progress, errors.New("read failed")
				}
				bad := progress
				bad.ExecutionRef.ID = uuid.NewString()
				return bad, nil
			}}
			switch fault {
			case "actor":
				gotActor.UserID = "bad"
			case "token":
				gotActor.TokenVersion = 0
			case "project":
				gotProject = "bad"
			case "execution":
				execution = "latest"
			}
			got, err := application.NewReferenceExecutionQuery(reader).Get(ctx, gotActor, gotProject, execution)
			if err == nil || !reflect.DeepEqual(got, domain.ReferenceJobProgress{}) {
				t.Fatal("invalid query returned data")
			}
			if fault == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("lost cancellation")
			}
			if fault != "reader_error" && fault != "identity" && fault != "cancelled" && read {
				t.Fatal("invalid query read facts")
			}
		})
	}
}
