// Package codexruntime wires the explicitly selected local Codex worker.
package codexruntime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	trialwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/aitrial"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/worker"
	"gorm.io/gorm"

	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/codex"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/staging"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// Register uses the installed CLI identity for local administrator trials.
// Non-local workers retain a separate service home and strict preflight.
// Existing project dispatch keeps its own authorization and price gate.
func Register(ctx context.Context, w worker.Worker, database *gorm.DB, cfg config.Config) error {
	storage, err := objectstorage.OpenLegacy(cfg)
	if err != nil {
		return err
	}
	objects, err := subjectwire.OpenObjects(cfg, storage)
	if err != nil {
		return err
	}
	context.AfterFunc(ctx, objects.Close)
	if storage == nil && objects.Registry() == nil {
		w.RegisterActivityWithOptions(func(context.Context, operationflow.ProviderSubmitInput) (operationflow.ProviderSubmitOutput, error) {
			return operationflow.ProviderSubmitOutput{Outcome: operationflow.ProviderSubmitNotSubmitted}, mediaapp.ErrStorageNotReady
		}, activity.RegisterOptions{Name: "provider.submit"})
		return nil
	}
	var launcher *codex.ProcessLauncher
	if cfg.Env == "local" && cfg.CodexHome == "" {
		if cfg.CodexWorkRoot == "" {
			cache, e := os.UserCacheDir()
			if e != nil {
				return e
			}
			cfg.CodexWorkRoot = filepath.Join(cache, "glowloom", "codex-trials")
		}
		if err = os.MkdirAll(cfg.CodexWorkRoot, 0700); err != nil {
			return err
		}
		launcher, err = codex.NewLocalProcessLauncher(cfg.CodexBinary, cfg.CodexWorkRoot)
	} else {
		launcher, err = codex.NewIsolatedProcessLauncher(codex.IsolatedProcessConfig{Binary: cfg.CodexBinary, Home: cfg.CodexHome, WorkRoot: cfg.CodexWorkRoot})
	}
	if err != nil {
		return fmt.Errorf("configure isolated Codex worker: %w", err)
	}
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	status, err := codex.Probe(probeCtx, launcher, cfg.CodexWorkRoot)
	if err != nil {
		return fmt.Errorf("probe Codex runtime: %w", err)
	}
	if cfg.Env != "local" && (!status.Authenticated || !status.ImageGeneration) {
		return fmt.Errorf("codex service identity must be logged in with image generation available")
	}

	if cfg.Env == "local" && storage != nil {
		trialwire.RegisterCodex(w, database, launcher, cfg.CodexWorkRoot, storage, application.TrialStorageID(cfg.ObjectStorageEndpoint, cfg.ObjectStorageBucket))
	}
	store := operationwire.ProvideOperationStore(database)
	receipts := application.NewProviderStageService(staging.NewObjects(storage), store)
	if objects.Registry() != nil {
		receipts = application.NewRoutedProviderStageService(staging.NewRoutedFactory(objects.Registry(), pgmedia.NewStorageRoutes(database)), store)
	}
	adapter, err := codex.New(store, launcher, receipts, codex.Config{WorkRoot: cfg.CodexWorkRoot})
	if err != nil {
		return err
	}
	activities := NewImageActivities(store, adapter)
	w.RegisterActivityWithOptions(activities.Submit, activity.RegisterOptions{Name: "provider.submit"})
	return nil
}

type operationReader interface {
	LoadWorkflowOperation(context.Context, uuid.UUID) (application.WorkflowOperation, error)
}

// ImageActivities reloads authorized operation facts before dispatch.
type ImageActivities struct {
	store   operationReader
	adapter imageSubmitter
}

type imageSubmitter interface {
	Submit(context.Context, application.ProviderImageInput) (application.ProviderImageResult, error)
}

// NewImageActivities keeps workflow-supplied text out of the inference request.
func NewImageActivities(store operationReader, adapter imageSubmitter) *ImageActivities {
	return &ImageActivities{store: store, adapter: adapter}
}

// Submit uses the stored project, model, price and prompt, never caller overrides.
func (a *ImageActivities) Submit(ctx context.Context, input operationflow.ProviderSubmitInput) (operationflow.ProviderSubmitOutput, error) {
	refused := operationflow.ProviderSubmitOutput{Outcome: operationflow.ProviderSubmitNotSubmitted, Error: &operationflow.ProviderError{Code: "invalid_codex_input"}}
	id, err := uuid.Parse(input.OperationID)
	if err != nil {
		return refused, nil
	}
	loaded, err := a.store.LoadWorkflowOperation(ctx, id)
	if err != nil {
		return refused, err
	}
	op := loaded.Operation
	if loaded.Provider.AdapterKey != "codex" || loaded.Provider.Queue != "agent.codex" || op.Capability != "image.generate" || op.Mode != "text_to_image" || op.OutputCount != 1 || op.ModelProfileVersionID == nil || op.PriceRuleVersionID == nil || input.Attempt != 1 || op.ProjectID.String() != input.ProjectID || op.ModelProfileVersionID.String() != input.ModelProfileVersionID || op.PriceRuleVersionID.String() != input.PriceRuleVersionID || loaded.ProviderRequestKey != input.ProviderRequestKey || loaded.Provider.ProviderModelID != input.ProviderModelID {
		return refused, nil
	}
	prompt := ""
	for _, item := range loaded.Inputs {
		if item.Role != "prompt" || item.TextValue == nil || item.MediaAssetID != nil || item.MaskAssetID != nil || item.RefID != nil || prompt != "" {
			return refused, nil
		}
		prompt = *item.TextValue
	}
	request := application.ProviderImageInput{Identity: application.ProviderDispatchIdentity{ProjectID: op.ProjectID, OperationID: op.ID, ModelProfileVersionID: *op.ModelProfileVersionID, PriceRuleVersionID: *op.PriceRuleVersionID, Action: "submit", Attempt: 1, RequestKey: loaded.ProviderRequestKey}, Model: loaded.Provider.ProviderModelID, Prompt: prompt}
	result, err := a.adapter.Submit(ctx, request)
	if err != nil {
		return operationflow.ProviderSubmitOutput{}, err
	}
	output := operationflow.ProviderSubmitOutput{Outcome: operationflow.ProviderSubmitOutcome(result.Outcome), Receipt: result.Receipt}
	if result.FailureCode != "" {
		output.Error = &operationflow.ProviderError{Code: result.FailureCode}
	}
	return output, nil
}
