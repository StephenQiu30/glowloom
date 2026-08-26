package bible_test

import (
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
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/production-bibles/bible-1", nil))
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

type bibleHTTPAuthenticator struct{}

func (bibleHTTPAuthenticator) Authenticate(*http.Request) (authentication.Claims, error) {
	return authentication.Claims{UserID: "user-1", TokenVersion: 1}, nil
}

type bibleHTTPService struct{ bible domain.Bible }

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
