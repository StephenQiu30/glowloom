package generation_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	generationhttp "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/google/uuid"
)

type referenceGenerationReader struct {
	read func(context.Context, application.Actor, string, string) (application.ReferenceGenerationProgress, error)
}

func (reader referenceGenerationReader) ReadReferenceGenerationProgress(ctx context.Context, actor application.Actor, project, target string) (application.ReferenceGenerationProgress, error) {
	return reader.read(ctx, actor, project, target)
}

func TestReferenceGenerationQueryAndHTTPBoundary(t *testing.T) {
	actor := application.Actor{UserID: uuid.NewString(), TokenVersion: 3}
	project, target := uuid.NewString(), uuid.NewString()
	ref := func(id string) domain.GenerationRevisionRef {
		return domain.GenerationRevisionRef{ID: id, Revision: 1, ContentHash: strings.Repeat("a", 64)}
	}
	value := application.ReferenceGenerationProgress{WorkspaceID: uuid.NewString(), ProjectID: project, GenerationTargetRef: ref(target), PlanRef: ref(uuid.NewString()), ReferenceTargetRef: ref(uuid.NewString()), GenerationRound: 1}
	for _, test := range []struct {
		name, suffix, body string
		authErr, readErr   error
		code               int
		read               bool
	}{
		{name: "not_prepared", code: 200, read: true},
		{name: "unauthenticated", authErr: errors.New("private auth"), code: 401},
		{name: "query", suffix: "?retry=true", code: 422},
		{name: "empty_query", suffix: "?", code: 422},
		{name: "body", body: `{}`, code: 422},
		{name: "forbidden", readErr: &application.Error{Code: "forbidden", Message: "Forbidden", Status: 403}, code: 403, read: true},
		{name: "missing", readErr: &application.Error{Code: "not_found", Message: "Not found", Status: 404}, code: 404, read: true},
		{name: "drift", readErr: &application.Error{Code: "state_conflict", Message: "Drift", Status: 409}, code: 409, read: true},
		{name: "internal", readErr: errors.New("private credential"), code: 500, read: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			reads := 0
			reader := referenceGenerationReader{read: func(ctx context.Context, got application.Actor, gotProject, gotTarget string) (application.ReferenceGenerationProgress, error) {
				reads++
				if got != actor || gotProject != project || gotTarget != target {
					t.Fatal("lost query scope")
				}
				return value, test.readErr
			}}
			mux := http.NewServeMux()
			generationhttp.NewReferenceGenerationHandler(application.NewReferenceGenerationQuery(reader), referenceProgressAuthenticator{actor, test.authErr}).Register(mux)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/projects/"+project+"/reference-generation-targets/"+target+test.suffix, strings.NewReader(test.body)))
			if response.Code != test.code || response.Header().Get("Cache-Control") != "no-store" || (reads == 1) != test.read {
				t.Fatalf("code=%d reads=%d body=%s", response.Code, reads, response.Body)
			}
			if strings.Contains(response.Body.String(), "private") {
				t.Fatal("private error leaked")
			}
			if response.Code == 200 {
				var result struct {
					Data application.ReferenceGenerationProgress `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Data.Execution != nil || len(result.Data.ContentHash) != 64 || !strings.Contains(response.Body.String(), `"execution":null`) {
					t.Fatalf("invalid unprepared state: %s", response.Body)
				}
			}
		})
	}
	for _, fault := range []string{"actor", "token", "project", "target", "reader_error", "scope", "identity", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			gotActor, gotProject, gotTarget := actor, project, target
			ctx := context.Background()
			if fault == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			reads := 0
			reader := referenceGenerationReader{read: func(received context.Context, _ application.Actor, _, _ string) (application.ReferenceGenerationProgress, error) {
				reads++
				if received != ctx {
					t.Fatal("context replaced")
				}
				if fault == "cancelled" {
					return value, received.Err()
				}
				if fault == "reader_error" {
					return value, errors.New("read failed")
				}
				bad := value
				if fault == "scope" {
					bad.ProjectID = uuid.NewString()
				} else {
					bad.GenerationTargetRef.ID = uuid.NewString()
				}
				return bad, nil
			}}
			switch fault {
			case "actor":
				gotActor.UserID = "bad"
			case "token":
				gotActor.TokenVersion = 0
			case "project":
				gotProject = "bad"
			case "target":
				gotTarget = "latest"
			}
			got, err := application.NewReferenceGenerationQuery(reader).Get(ctx, gotActor, gotProject, gotTarget)
			if err == nil || !reflect.DeepEqual(got, application.ReferenceGenerationProgress{}) {
				t.Fatal("invalid query returned data")
			}
			if fault == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("lost cancellation")
			}
			if (fault == "actor" || fault == "token" || fault == "project" || fault == "target") && reads != 0 {
				t.Fatal("invalid input read facts")
			}
		})
	}
}
