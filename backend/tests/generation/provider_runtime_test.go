package generation_test

import (
	"context"
	"errors"
	"testing"

	app "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

type runtimeResolver struct{}

func (runtimeResolver) WithRuntime(ctx context.Context, s app.ProviderSubmission, use func(app.ProviderRuntimeConfig) error) error {
	return use(app.ProviderRuntimeConfig{Connection: domain.ProviderConnectionVersion{ProviderKey: "openai", AdapterContractVersion: "openai-image-api"}, Profile: domain.ProviderModelProfileVersion{Modality: "image"}})
}

type runtimeFactory struct{ submits, queries int }

func (*runtimeFactory) Descriptor() app.MediaFactoryDescriptor {
	return app.MediaFactoryDescriptor{ProviderKey: "openai", Modality: "image", AdapterContractVersion: "openai-image-api"}
}
func (f *runtimeFactory) NewRuntime(app.ProviderRuntimeConfig) (app.ProviderGateway, error) {
	return f, nil
}
func (*runtimeFactory) Preflight(context.Context, app.ProviderSubmission) error { return nil }
func (f *runtimeFactory) Submit(context.Context, app.ProviderSubmission) (app.ProviderOutcome, error) {
	f.submits++
	return app.ProviderOutcome{Status: "accepted"}, nil
}
func (f *runtimeFactory) Query(context.Context, app.ProviderSubmission) (app.ProviderOutcome, error) {
	f.queries++
	return app.ProviderOutcome{Status: "running"}, nil
}
func TestRuntimeQueryCannotDispatchOrFallback(t *testing.T) {
	f := &runtimeFactory{}
	r, err := app.NewMediaFactoryRegistry([]app.MediaAdapterFactory{f})
	if err != nil {
		t.Fatal(err)
	}
	gateway := app.NewRuntimeGateway(runtimeResolver{}, r)
	if _, err = gateway.Query(context.Background(), app.ProviderSubmission{}); err != nil {
		t.Fatal(err)
	}
	if f.queries != 1 || f.submits != 0 {
		t.Fatal("query dispatched")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = gateway.Submit(ctx, app.ProviderSubmission{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if f.submits != 0 {
		t.Fatal("cancelled submit dispatched")
	}
}
func TestDescriptorOnlyFactoryCannotExecute(t *testing.T) {
	r, err := app.NewMediaFactoryRegistry([]app.MediaAdapterFactory{controlledMediaFactory{descriptor: app.MediaFactoryDescriptor{ProviderKey: "openai", Modality: "image", AdapterContractVersion: "openai-image-api"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = app.NewRuntimeGateway(runtimeResolver{}, r).Preflight(context.Background(), app.ProviderSubmission{}); err == nil {
		t.Fatal("descriptor advertised execution")
	}
}
