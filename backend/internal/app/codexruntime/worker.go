// Package codexruntime wires the explicitly selected local Codex worker.
package codexruntime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	operationtask "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	trialtask "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/trialtask"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	trialwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/aitrial"

	"github.com/google/uuid"
	"gorm.io/gorm"

	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/codex"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/staging"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// Register uses the installed CLI identity for local administrator trials.
// Non-local workers retain a separate service home and strict preflight.
// Existing project dispatch keeps its own authorization and price gate.
func Register(ctx context.Context, handlers map[string]taskruntime.Handler, database *gorm.DB, cfg config.Config) error {
	objects, err := subjectwire.OpenObjects(cfg)
	if err != nil {
		return err
	}
	context.AfterFunc(ctx, objects.Close)
	registry := objects.Registry()
	if registry == nil {
		return objectstorage.ErrUnavailable
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

	if cfg.Env == "local" {
		handlers[trialtask.Codex] = trialwire.Codex(database, launcher, cfg.CodexWorkRoot, objects.Registry())
	}
	store := operationwire.ProvideOperationStore(database)
	receipts := application.NewRoutedProviderStageService(staging.NewRoutedFactory(registry, pgmedia.NewStorageRoutes(database)), store)
	adapter, err := codex.New(store, launcher, receipts, codex.Config{WorkRoot: cfg.CodexWorkRoot})
	if err != nil {
		return err
	}
	activities := NewImageActivities(store, adapter)
	operationtask.RegisterCodex(handlers, store, activities)
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
func (a *ImageActivities) Submit(ctx context.Context, input operationtask.ProviderSubmitInput) (operationtask.ProviderSubmitOutput, error) {
	refused := operationtask.ProviderSubmitOutput{Outcome: operationtask.ProviderSubmitNotSubmitted, Error: &operationtask.ProviderError{Code: "invalid_codex_input"}}
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
		return operationtask.ProviderSubmitOutput{}, err
	}
	output := operationtask.ProviderSubmitOutput{Outcome: operationtask.ProviderSubmitOutcome(result.Outcome), Receipt: result.Receipt}
	if result.FailureCode != "" {
		output.Error = &operationtask.ProviderError{Code: result.FailureCode}
	}
	return output, nil
}
