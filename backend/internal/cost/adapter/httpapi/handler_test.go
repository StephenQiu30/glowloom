package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	"github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	"github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
)

func TestSetBudgetPreservesCostContract(t *testing.T) {
	service := &stubService{}
	handler := New(service, stubAuthenticator{})
	mux := http.NewServeMux()
	handler.Register(mux)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/019fb2e0-a000-7000-8000-000000000001/cost-budget", strings.NewReader(`{"limit_amount":"100.125","currency":"USD","expected_revision":0,"idempotency_key":"set-budget-1"}`))
	request.Header.Set("Authorization", "Bearer valid")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"limit_amount":"100.125000"`) ||
		!strings.Contains(response.Body.String(), `"currency":"USD"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.actor.UserID != "019fb2e0-a000-7000-8000-000000000002" ||
		service.command.LimitAmount != "100.125" || service.command.ExpectedRevision != 0 {
		t.Fatalf("actor=%#v command=%#v", service.actor, service.command)
	}
}

func TestSetBudgetRejectsUnknownFieldsBeforeApplication(t *testing.T) {
	service := &stubService{}
	handler := New(service, stubAuthenticator{})
	mux := http.NewServeMux()
	handler.Register(mux)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/019fb2e0-a000-7000-8000-000000000001/cost-budget", strings.NewReader(`{"limit_amount":"100","currency":"USD","expected_revision":0,"idempotency_key":"set-budget-1","budget_limit":"100"}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"validation_failed"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.command.ProjectID != "" {
		t.Fatal("invalid budget request reached application")
	}
}

func TestSetBudgetRequiresDecimalString(t *testing.T) {
	service := &stubService{}
	handler := New(service, stubAuthenticator{})
	mux := http.NewServeMux()
	handler.Register(mux)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/019fb2e0-a000-7000-8000-000000000001/cost-budget", strings.NewReader(`{"limit_amount":100.125,"currency":"USD","expected_revision":0,"idempotency_key":"set-budget-1"}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"validation_failed"`) {
		t.Fatalf("numeric budget response = %d %s", response.Code, response.Body.String())
	}
	if service.command.ProjectID != "" {
		t.Fatal("inexact numeric budget request reached application")
	}
}

func TestBudgetRouteRejectsMissingBearerToken(t *testing.T) {
	handler := New(&stubService{}, stubAuthenticator{err: authentication.ErrUnauthenticated})
	mux := http.NewServeMux()
	handler.Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/projects/019fb2e0-a000-7000-8000-000000000001/cost-budget", nil))
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"unauthenticated"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

type stubAuthenticator struct{ err error }

func (auth stubAuthenticator) Authenticate(*http.Request) (authentication.Claims, error) {
	if auth.err != nil {
		return authentication.Claims{}, auth.err
	}
	return authentication.Claims{UserID: "019fb2e0-a000-7000-8000-000000000002", TokenVersion: 1}, nil
}

type stubService struct {
	actor   application.Actor
	command application.SetBudgetCommand
}

func (service *stubService) SetBudget(
	_ context.Context,
	actor application.Actor,
	command application.SetBudgetCommand,
) (application.BudgetResult, error) {
	service.actor, service.command = actor, command
	value := domain.BudgetPolicy{
		ID: "019fb2e0-a000-7000-8000-000000000003", WorkspaceID: "019fb2e0-a000-7000-8000-000000000004",
		ProjectID: command.ProjectID, LimitAmount: decimal.RequireFromString(command.LimitAmount), Currency: command.Currency,
		Revision: 1, CreatedBy: actor.UserID, UpdatedBy: actor.UserID,
		CreatedAt: time.Unix(10, 0).UTC(), UpdatedAt: time.Unix(10, 0).UTC(),
	}
	return application.BudgetResult{Policy: value}, nil
}

func (*stubService) GetBudget(context.Context, application.Actor, string) (domain.BudgetPolicy, error) {
	return domain.BudgetPolicy{}, errors.New("not implemented")
}
