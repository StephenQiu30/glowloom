package gormdb

import (
	"context"
	"reflect"

	"gorm.io/gorm"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	presetgorm "github.com/StephenQiu30/lanverse/backend/internal/preset/adapter/gormdb"
	presetcatalog "github.com/StephenQiu30/lanverse/backend/internal/preset/catalog"
	worldgorm "github.com/StephenQiu30/lanverse/backend/internal/production/world/adapter/gormdb"
	storygraphgorm "github.com/StephenQiu30/lanverse/backend/internal/storygraph/adapter/gormdb"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
)

func ValidateCurrentVisualFoundationInput(
	ctx context.Context,
	database *gorm.DB,
	input contract.VisualFoundationInput,
) error {
	world, err := storygraphgorm.New(database).GetCurrentVisualFoundationWorld(
		ctx,
		input.WorkspaceID,
		input.ProjectID,
	)
	if err != nil {
		return staleVisualFoundationInput(err)
	}
	source, err := worldgorm.NewVisualFoundationSourceRepository(database).
		GetCurrentVisualFoundationSource(ctx, input.WorkspaceID, input.ProjectID)
	if err != nil {
		return staleVisualFoundationInput(err)
	}
	release, found, err := presetcatalog.FindCuratedRelease(input.PresetRelease.Key, input.PresetRelease.Release)
	if err != nil || !found || release.ContentHash != input.PresetRelease.ContentHash {
		return staleVisualFoundationInput(err)
	}
	selection, err := presetgorm.NewProjectSelectionStore(database).Current(
		ctx,
		input.WorkspaceID,
		input.ProjectID,
	)
	if err != nil {
		return staleVisualFoundationInput(err)
	}
	rebuilt, _, err := workflowapp.CompileFaithfulVisualFoundationInput(
		workflowapp.FaithfulVisualFoundationInputCommand{
			World: world, Source: source, Selection: selection, PresetRelease: release,
		},
	)
	if err != nil || !reflect.DeepEqual(rebuilt, input) {
		return staleVisualFoundationInput(err)
	}
	return nil
}

func staleVisualFoundationInput(cause error) error {
	message := "Visual Foundation input changed before Candidate acceptance"
	if cause != nil {
		message += ": " + cause.Error()
	}
	return &agentapp.Error{Code: "stale_visual_foundation_input", Message: message}
}
