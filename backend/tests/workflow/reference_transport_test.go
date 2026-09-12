package workflow_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	openaiadapter "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/openai"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

// Exercise every persisted Brief -> Target -> compiled Call against transport
// preflight. This is deliberately not a send: no committed claim is consumed.
func assertPersistedReferenceTransportPreflight(t *testing.T, ctx context.Context, target generationapp.ReferenceGenerationTarget, compiled generationapp.ReferenceImageCompilation, calls []domain.ReferenceProviderCall, profile domain.ProviderModelProfileVersion) {
	t.Helper()
	forbidden := &referencePreflightIO{}
	factory := openaiadapter.NewFactory(&http.Client{Transport: forbidden}, forbidden, nil)
	runtime, err := factory.NewReferenceImageRuntime(generationapp.ProviderRuntimeConfig{
		Connection: domain.ProviderConnectionVersion{ProviderKey: "openai", AdapterContractVersion: "openai-image-api"},
		Profile:    profile, Credentials: []byte(`{"api_key":"synthetic-preflight-only"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, call := range calls {
		for _, request := range compiled.Requests {
			if request.BundleIndex != call.BundleIndex || request.SlotKey != call.SlotKey {
				continue
			}
			for _, slot := range target.OutputContract.Slots {
				if slot.SlotKey != call.SlotKey {
					continue
				}
				input := generationapp.ReferenceImageSubmission{WorkspaceID: target.WorkspaceID, ProjectID: target.ProjectID, Call: call, Request: request, Slot: slot}
				if err := runtime.Preflight(ctx, input); err != nil {
					t.Fatalf("compiled slot %s rejected by transport: %v", call.SlotKey, err)
				}
				checked++
			}
		}
	}
	if checked != len(calls) || forbidden.calls != 0 {
		t.Fatal("incomplete preflight or unexpected external IO")
	}
}

type referencePreflightIO struct{ calls int }

func (io *referencePreflightIO) RoundTrip(*http.Request) (*http.Response, error) {
	io.calls++
	return nil, errors.New("preflight must not send")
}

func (io *referencePreflightIO) EnsurePrivateObject(context.Context, string, []byte, string, string) error {
	io.calls++
	return errors.New("preflight must not stage")
}
