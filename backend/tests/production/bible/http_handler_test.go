package bible_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	biblehttp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/httpapi"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

func TestBibleResponseIncludesGenerationError(t *testing.T) {
	service := bibleHTTPService{bible: domain.Bible{
		ID:              "bible-1",
		Error:           json.RawMessage(`{"code":"codex_unavailable","summary":"model request failed","retryable":true}`),
		ReviewDecisions: map[string]string{},
	}}
	mux := http.NewServeMux()
	biblehttp.New(service, bibleHTTPAuthenticator{}).Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/production-bibles/bible-1", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data struct {
			GenerationError map[string]any `json:"generation_error"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.GenerationError["code"] != "codex_unavailable" ||
		payload.Data.GenerationError["summary"] != "model request failed" {
		t.Fatalf("unexpected generation_error: %#v", payload.Data.GenerationError)
	}
}

func TestStoryAnalysisRecoveryResponseExposesPersistedIdentity(t *testing.T) {
	service := storyAnalysisRecoveryHTTPService{result: bibleapp.StoryAnalysisRecovery{
		ReceiptID:     "00000000-0000-0000-0000-000000000004",
		WorkflowRunID: "00000000-0000-0000-0000-000000000001",
		NodeRunID:     "00000000-0000-0000-0000-000000000002",
		InvocationID:  "00000000-0000-0000-0000-000000000003",
		Stage:         "analyze_story", ShardKey: "map:0001", Status: "queued",
		FailureCode: "execution_deadline_exceeded", PreviousClaimVersion: 2,
	}}
	mux := http.NewServeMux()
	biblehttp.NewStoryAnalysisRecovery(&service, bibleHTTPAuthenticator{}).Register(mux)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/workflow-runs/00000000-0000-0000-0000-000000000001/story-analysis-recoveries",
		bytes.NewBufferString(`{"node_run_id":"00000000-0000-0000-0000-000000000002","idempotency_key":"recover-deadline"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data bibleapp.StoryAnalysisRecovery `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if service.command.WorkflowRunID != service.result.WorkflowRunID ||
		service.command.NodeRunID != service.result.NodeRunID ||
		service.command.IdempotencyKey != "recover-deadline" ||
		payload.Data != service.result {
		t.Fatalf("recovery command=%#v response=%#v", service.command, payload.Data)
	}
}

type bibleHTTPAuthenticator struct{}

func (bibleHTTPAuthenticator) Authenticate(*http.Request) (authentication.Claims, error) {
	return authentication.Claims{UserID: "user-1", TokenVersion: 1}, nil
}

type bibleHTTPService struct{ bible domain.Bible }

type storyAnalysisRecoveryHTTPService struct {
	result  bibleapp.StoryAnalysisRecovery
	command bibleapp.StoryAnalysisRecoveryCommand
}

func (service *storyAnalysisRecoveryHTTPService) Recover(
	_ context.Context,
	_ bibleapp.Actor,
	command bibleapp.StoryAnalysisRecoveryCommand,
) (bibleapp.StoryAnalysisRecovery, error) {
	service.command = command
	return service.result, nil
}

func (service bibleHTTPService) Create(context.Context, bibleapp.Actor, bibleapp.CreateCommand) (domain.Bible, error) {
	return domain.Bible{}, errors.New("not implemented")
}
func (service bibleHTTPService) Get(context.Context, bibleapp.Actor, string) (domain.Bible, error) {
	return service.bible, nil
}
func (service bibleHTTPService) GetCurrent(context.Context, bibleapp.Actor, string) (domain.Bible, error) {
	return domain.Bible{}, errors.New("not implemented")
}
func (service bibleHTTPService) Confirm(context.Context, bibleapp.Actor, bibleapp.ConfirmCommand) (bibleapp.ConfirmResult, error) {
	return bibleapp.ConfirmResult{}, errors.New("not implemented")
}
func (service bibleHTTPService) DecideReviewIssue(context.Context, bibleapp.Actor, bibleapp.DecideReviewIssueCommand) (domain.Bible, error) {
	return domain.Bible{}, errors.New("not implemented")
}
func (service bibleHTTPService) Resume(context.Context, bibleapp.Actor, bibleapp.ResumeCommand) (domain.Bible, error) {
	return domain.Bible{}, errors.New("not implemented")
}
