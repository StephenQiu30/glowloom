package bible_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/StephenQiu30/lanverse/backend/api/openapi"
	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	biblehttp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/httpapi"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

const reviewBibleID = "00000000-0000-0000-0000-000000000001"

func TestBibleReviewDecisionSchemaMatchesHandler(t *testing.T) {
	t.Parallel()
	requestSchema := reviewDecisionSchema(t, "requestBody/content/application~1json/schema")
	successSchema := reviewDecisionSchema(t, "responses/200/content/application~1json/schema")
	_ = reviewDecisionSchema(t, "responses/422")
	problemSchema := reviewProblemSchema(t)

	for _, action := range []string{"accepted", "rejected"} {
		t.Run(action, func(t *testing.T) {
			body := validReviewBody()
			body["action"] = action
			service := &reviewDecisionService{}
			response := callReviewDecision(t, body, service, bibleHTTPAuthenticator{})
			if err := requestSchema.Validate(body); err != nil {
				t.Fatalf("valid request rejected by OpenAPI: %v", err)
			}
			if response.Code != http.StatusOK || service.calls != 1 {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, service.calls, response.Body.String())
			}
			if service.command != (bibleapp.DecideReviewIssueCommand{
				BibleID: reviewBibleID, IssueKey: "issue-1", Action: action,
				ExpectedRevision: 2, IdempotencyKey: "review:issue-1",
			}) || service.actor.UserID != "user-1" {
				t.Fatalf("command or actor drift: %#v %#v", service.command, service.actor)
			}
			payload := reviewJSON(t, response.Body.Bytes())
			if err := successSchema.Validate(payload); err != nil {
				t.Fatalf("Handler success violates OpenAPI: %v", err)
			}
			data := payload.(map[string]any)["data"].(map[string]any)
			if data["revision"] != float64(3) || data["review_decisions"].(map[string]any)["issue-1"] != action {
				t.Fatalf("response lost persisted decision: %#v", data)
			}
		})
	}

	invalid := map[string]map[string]any{}
	for _, field := range []string{"issue_key", "action", "expected_revision", "idempotency_key"} {
		body := validReviewBody()
		delete(body, field)
		invalid["missing_"+field] = body
	}
	for name, change := range map[string]struct {
		field string
		value any
	}{
		"empty_issue":      {"issue_key", ""},
		"long_issue":       {"issue_key", strings.Repeat("界", 101)},
		"unknown_action":   {"action", "approved"},
		"zero_revision":    {"expected_revision", 0},
		"decimal_revision": {"expected_revision", 1.5},
		"null_revision":    {"expected_revision", nil},
		"empty_key":        {"idempotency_key", ""},
		"long_key":         {"idempotency_key", strings.Repeat("a", 201)},
		"extra_field":      {"workspace_id", reviewBibleID},
	} {
		body := validReviewBody()
		body[change.field] = change.value
		invalid[name] = body
	}
	for name, body := range invalid {
		t.Run(name, func(t *testing.T) {
			if requestSchema.Validate(body) == nil {
				t.Fatal("OpenAPI accepts invalid review decision")
			}
			service := &reviewDecisionService{}
			response := callReviewDecision(t, body, service, bibleHTTPAuthenticator{})
			if response.Code != http.StatusUnprocessableEntity || service.calls != 0 {
				t.Fatalf("status=%d calls=%d; invalid request reached Owner", response.Code, service.calls)
			}
			if err := problemSchema.Validate(reviewJSON(t, response.Body.Bytes())); err != nil {
				t.Fatalf("validation response violates OpenAPI: %v", err)
			}
		})
	}
}

func TestBibleReviewDecisionErrorsRemainFailures(t *testing.T) {
	t.Parallel()
	for _, status := range []int{401, 403, 404, 409, 500} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			// Compilation fails if the operation stops documenting this status.
			_ = reviewDecisionSchema(t, "responses/"+strconv.Itoa(status))
			service := &reviewDecisionService{failure: &bibleapp.Error{Code: "resource_conflict", Status: status, Message: "request failed"}}
			var authenticator biblehttp.Authenticator = bibleHTTPAuthenticator{}
			if status == 401 {
				authenticator = reviewDeniedAuthenticator{}
			}
			response := callReviewDecision(t, validReviewBody(), service, authenticator)
			if response.Code != status || (status == 401 && service.calls != 0) {
				t.Fatalf("status=%d calls=%d", response.Code, service.calls)
			}
			payload := reviewJSON(t, response.Body.Bytes())
			if err := reviewProblemSchema(t).Validate(payload); err != nil {
				t.Fatalf("error response violates OpenAPI: %v", err)
			}
			if _, ok := payload.(map[string]any)["data"]; ok {
				t.Fatal("failed decision exposed success data")
			}
		})
	}
}

func reviewDecisionSchema(t *testing.T, fragment string) *jsonschema.Schema {
	t.Helper()
	return compileReviewSchema(t, "paths/~1api~1production-bibles~1{bible_id}~1review-decisions/post/"+fragment)
}

func reviewProblemSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	return compileReviewSchema(t, "components/responses/Problem/content/application~1json/schema")
}

func compileReviewSchema(t *testing.T, fragment string) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	const location = "https://lanverse.test/openapi.json"
	if err := compiler.AddResource(location, reviewJSON(t, openapi.Document())); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(location + "#/" + fragment)
	if err != nil {
		t.Fatalf("missing or invalid review contract: %v", err)
	}
	return schema
}

func reviewJSON(t *testing.T, encoded []byte) any {
	t.Helper()
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func validReviewBody() map[string]any {
	return map[string]any{"issue_key": "issue-1", "action": "accepted", "expected_revision": 2, "idempotency_key": "review:issue-1"}
}

func callReviewDecision(t *testing.T, body map[string]any, service *reviewDecisionService, authenticator biblehttp.Authenticator) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	biblehttp.New(service, authenticator).Register(mux)
	request := httptest.NewRequest(http.MethodPost, "/api/production-bibles/"+reviewBibleID+"/review-decisions", bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}

type reviewDecisionService struct {
	bibleHTTPService
	command bibleapp.DecideReviewIssueCommand
	actor   bibleapp.Actor
	calls   int
	failure error
}

func (service *reviewDecisionService) DecideReviewIssue(_ context.Context, actor bibleapp.Actor, command bibleapp.DecideReviewIssueCommand) (domain.Bible, error) {
	service.calls++
	service.command, service.actor = command, actor
	if service.failure != nil {
		return domain.Bible{}, service.failure
	}
	return domain.Bible{
		ID: reviewBibleID, WorkspaceID: reviewBibleID, ProjectID: reviewBibleID, DocumentRevisionID: reviewBibleID, TaskID: reviewBibleID,
		Status: "needs_review", InputHash: strings.Repeat("a", 64), Revision: command.ExpectedRevision + 1,
		Candidate:       domain.Candidate{ReviewIssues: []domain.ReviewIssue{}},
		ReviewDecisions: map[string]string{command.IssueKey: command.Action},
		CreatedAt:       time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(2, 0).UTC(),
	}, nil
}

type reviewDeniedAuthenticator struct{}

func (reviewDeniedAuthenticator) Authenticate(*http.Request) (authentication.Claims, error) {
	return authentication.Claims{}, errors.New("session expired")
}
