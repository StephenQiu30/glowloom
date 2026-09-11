package application

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	presetdomain "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
)

var ErrProjectSelectionNotFound = errors.New("Project Preset selection not found")

type ProjectSelectionError struct {
	Code    string
	Message string
}

func (err *ProjectSelectionError) Error() string { return err.Message }

func IsProjectSelectionConflict(err error) bool {
	var target *ProjectSelectionError
	return errors.As(err, &target) && target.Code == "project_preset_selection_conflict"
}

type CuratedReleaseFinder func(key, release string) (presetdomain.Release, bool, error)

type ProjectSelectionTransactions interface {
	WithinSerializableTransaction(context.Context, func(ProjectSelectionRepository) error) error
}

type ProjectSelectionRepository interface {
	VerifySelectionAccess(context.Context, string, string, string) error
	CurrentProjectSelection(context.Context, string, string, bool) (presetdomain.ProjectSelection, error)
	CreateProjectSelection(context.Context, presetdomain.ProjectSelection, []byte) error
	AdvanceProjectSelectionHead(context.Context, presetdomain.ProjectSelection, int64) error
}

type SelectProjectPresetCommand struct {
	WorkspaceID, ProjectID, SelectedBy        string
	PresetKey, PresetRelease, ApplicationMode string
	ExpectedRevision                          int64
}

type ProjectSelectionService struct {
	transactions ProjectSelectionTransactions
	findRelease  CuratedReleaseFinder
	now          func() time.Time
	newID        func() string
}

func NewProjectSelectionService(
	transactions ProjectSelectionTransactions,
	findRelease CuratedReleaseFinder,
	now func() time.Time,
	newID func() string,
) *ProjectSelectionService {
	return &ProjectSelectionService{transactions: transactions, findRelease: findRelease, now: now, newID: newID}
}

func (service *ProjectSelectionService) Select(
	ctx context.Context,
	command SelectProjectPresetCommand,
) (presetdomain.ProjectSelection, error) {
	if service == nil || service.transactions == nil || service.findRelease == nil || service.now == nil || service.newID == nil ||
		command.ExpectedRevision < 0 {
		return presetdomain.ProjectSelection{}, invalidProjectSelection("invalid Project Preset selection command")
	}
	release, found, err := service.findRelease(command.PresetKey, command.PresetRelease)
	if err != nil {
		return presetdomain.ProjectSelection{}, fmt.Errorf("resolve curated Preset release: %w", err)
	}
	if !found {
		return presetdomain.ProjectSelection{}, invalidProjectSelection("curated Preset release not found")
	}
	var selected presetdomain.ProjectSelection
	err = service.transactions.WithinSerializableTransaction(ctx, func(repository ProjectSelectionRepository) error {
		if verifyErr := repository.VerifySelectionAccess(ctx, command.WorkspaceID, command.ProjectID, command.SelectedBy); verifyErr != nil {
			return verifyErr
		}
		current, currentErr := repository.CurrentProjectSelection(ctx, command.WorkspaceID, command.ProjectID, true)
		if currentErr != nil && !errors.Is(currentErr, ErrProjectSelectionNotFound) {
			return currentErr
		}
		if currentErr == nil {
			if current.Revision != command.ExpectedRevision {
				return projectSelectionConflict()
			}
			if current.PresetRelease.Key == release.Key && current.PresetRelease.Release == release.Release &&
				current.PresetRelease.ContentHash == release.ContentHash && current.ApplicationMode == command.ApplicationMode {
				selected = current
				return nil
			}
		} else if command.ExpectedRevision != 0 {
			return projectSelectionConflict()
		}

		input := presetdomain.ProjectSelectionInput{
			WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
			Revision: command.ExpectedRevision + 1,
			PresetRelease: presetdomain.ProjectSelectionRelease{
				Key: release.Key, Release: release.Release, ContentHash: release.ContentHash,
			},
			ApplicationMode: command.ApplicationMode, SelectedBy: command.SelectedBy,
			SelectedAt: service.now().UTC().Truncate(time.Microsecond),
		}
		if currentErr == nil {
			input.ParentSelectionID = &current.ID
			input.ParentContentHash = &current.ContentHash
		}
		selection, encoded, buildErr := presetdomain.NewProjectSelection(service.newID(), input)
		if buildErr != nil {
			return invalidProjectSelection("invalid Project Preset selection")
		}
		rebuilt, _, buildErr := presetdomain.NewRelease(release.ReleaseInput)
		if buildErr != nil || !reflect.DeepEqual(rebuilt, release) {
			return invalidProjectSelection("curated Preset release has drifted")
		}
		if createErr := repository.CreateProjectSelection(ctx, selection, encoded); createErr != nil {
			return createErr
		}
		if advanceErr := repository.AdvanceProjectSelectionHead(ctx, selection, command.ExpectedRevision); advanceErr != nil {
			return advanceErr
		}
		selected = selection
		return nil
	})
	return selected, err
}

func (service *ProjectSelectionService) Current(
	ctx context.Context,
	workspaceID string,
	projectID string,
) (presetdomain.ProjectSelection, error) {
	if service == nil || service.transactions == nil {
		return presetdomain.ProjectSelection{}, invalidProjectSelection("Project Preset selection service is unavailable")
	}
	var current presetdomain.ProjectSelection
	err := service.transactions.WithinSerializableTransaction(ctx, func(repository ProjectSelectionRepository) error {
		selection, loadErr := repository.CurrentProjectSelection(ctx, workspaceID, projectID, false)
		current = selection
		return loadErr
	})
	return current, err
}

func invalidProjectSelection(message string) error {
	return &ProjectSelectionError{Code: "invalid_project_preset_selection", Message: message}
}

func projectSelectionConflict() error {
	return &ProjectSelectionError{
		Code: "project_preset_selection_conflict", Message: "Project Preset selection changed before update",
	}
}
