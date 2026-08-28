package generation_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	generationhttp "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/httpapi"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

func TestPublishConfiguredImageProviderBindingAcceptsOnlyTheCommandIdentity(t *testing.T) {
	service := &providerBindingHTTPService{}
	handler := generationhttp.NewProviderBindingHandler(service, providerBindingHTTPAuthenticator{})
	mux := http.NewServeMux()
	handler.Register(mux)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/projects/019fb2e0-a000-7000-8000-000000000001/generation/image-provider-bindings",
		strings.NewReader(`{"idempotency_key":"publish-runware-v1"}`),
	)
	request.Header.Set("Authorization", "Bearer valid")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"provider_key":"runware"`) ||
		!strings.Contains(response.Body.String(), `"model_key":"runware:z-image@turbo"`) ||
		!strings.Contains(response.Body.String(), `"credential_ref":"env/runware_api_key"`) ||
		!strings.Contains(response.Body.String(), `"receipt_id":"019fb2e0-a000-7000-8000-000000000006"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "RUNWARE_API_KEY") || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("Provider binding response leaked a secret: %s", response.Body.String())
	}
	if service.actor.UserID != "019fb2e0-a000-7000-8000-000000000002" ||
		service.command.ProjectID != "019fb2e0-a000-7000-8000-000000000001" ||
		service.command.IdempotencyKey != "publish-runware-v1" {
		t.Fatalf("actor=%#v command=%#v", service.actor, service.command)
	}
}

func TestPublishConfiguredImageProviderBindingRejectsClientProviderConfiguration(t *testing.T) {
	service := &providerBindingHTTPService{}
	handler := generationhttp.NewProviderBindingHandler(service, providerBindingHTTPAuthenticator{})
	mux := http.NewServeMux()
	handler.Register(mux)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/projects/019fb2e0-a000-7000-8000-000000000001/generation/image-provider-bindings",
		strings.NewReader(`{"idempotency_key":"publish-runware-v1","credential_ref":"env/attacker_key"}`),
	)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(response.Body.String(), `"validation_failed"`) || service.command.ProjectID != "" {
		t.Fatalf("response = %d %s command=%#v", response.Code, response.Body.String(), service.command)
	}
}

func TestPublishConfiguredImageProviderBindingRequiresAuthentication(t *testing.T) {
	handler := generationhttp.NewProviderBindingHandler(
		&providerBindingHTTPService{},
		providerBindingHTTPAuthenticator{err: authentication.ErrUnauthenticated},
	)
	mux := http.NewServeMux()
	handler.Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(
		http.MethodPost,
		"/api/v1/projects/019fb2e0-a000-7000-8000-000000000001/generation/image-provider-bindings",
		strings.NewReader(`{"idempotency_key":"publish-runware-v1"}`),
	))
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"unauthenticated"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

type providerBindingHTTPAuthenticator struct{ err error }

func (auth providerBindingHTTPAuthenticator) Authenticate(*http.Request) (authentication.Claims, error) {
	if auth.err != nil {
		return authentication.Claims{}, auth.err
	}
	return authentication.Claims{UserID: "019fb2e0-a000-7000-8000-000000000002", TokenVersion: 1}, nil
}

type providerBindingHTTPService struct {
	actor   generationapp.Actor
	command generationapp.PublishConfiguredImageProviderBindingCommand
}

func (service *providerBindingHTTPService) PublishConfiguredImageProviderBinding(
	_ context.Context,
	actor generationapp.Actor,
	command generationapp.PublishConfiguredImageProviderBindingCommand,
) (generationapp.ProviderBindingResult, error) {
	service.actor, service.command = actor, command
	createdAt := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)
	return generationapp.ProviderBindingResult{
		Binding: generationdomain.ProviderBinding{
			ID: "019fb2e0-a000-7000-8000-000000000003", WorkspaceID: "019fb2e0-a000-7000-8000-000000000004",
			ProjectID: command.ProjectID, Capability: "generation.image", ProviderKey: "runware",
			ModelKey: "runware:z-image@turbo", CredentialRef: "env/runware_api_key", Revision: 1,
			ContentHash: strings.Repeat("a", 64), CreatedBy: actor.UserID, CreatedAt: createdAt,
		},
		Receipt: platformcommand.Receipt{ID: "019fb2e0-a000-7000-8000-000000000006"},
	}, nil
}
