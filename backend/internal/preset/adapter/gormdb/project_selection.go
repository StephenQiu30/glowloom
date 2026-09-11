package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	commandgorm "github.com/StephenQiu30/lanverse/backend/internal/platform/command/adapter/gormdb"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	presetapp "github.com/StephenQiu30/lanverse/backend/internal/preset/application"
	presetdomain "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
)

type ProjectSelectionStore struct{ database *gorm.DB }
type projectSelectionRepository struct{ database *gorm.DB }

func NewProjectSelectionStore(database *gorm.DB) *ProjectSelectionStore {
	return &ProjectSelectionStore{database: database}
}

func (store *ProjectSelectionStore) Current(
	ctx context.Context,
	workspaceID string,
	projectID string,
) (presetdomain.ProjectSelection, error) {
	if store == nil || store.database == nil {
		return presetdomain.ProjectSelection{}, presetapp.ErrProjectSelectionNotFound
	}
	return (&projectSelectionRepository{database: store.database}).CurrentProjectSelection(
		ctx,
		workspaceID,
		projectID,
		false,
	)
}

func (store *ProjectSelectionStore) WithinSerializableTransaction(
	ctx context.Context,
	operation func(presetapp.ProjectSelectionRepository) error,
) error {
	return platformdatabase.WithinSerializableTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&projectSelectionRepository{database: transaction})
	})
}

func (repository *projectSelectionRepository) VerifySelectionAccess(
	ctx context.Context,
	workspaceID string,
	projectID string,
	selectedBy string,
) error {
	workspace, workspaceErr := uuid.Parse(workspaceID)
	project, projectErr := uuid.Parse(projectID)
	actor, actorErr := uuid.Parse(selectedBy)
	if workspaceErr != nil || projectErr != nil || actorErr != nil {
		return &presetapp.ProjectSelectionError{Code: "project_preset_selection_scope_invalid", Message: "Project Preset selection scope is invalid"}
	}
	var projectRecord model.Project
	if err := repository.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND workspace_id = ? AND status = ?", project, workspace, "active").First(&projectRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &presetapp.ProjectSelectionError{Code: "project_preset_selection_scope_invalid", Message: "Project is unavailable"}
		}
		return err
	}
	var user model.UserAccount
	if err := repository.database.WithContext(ctx).Where("id = ? AND status = ?", actor, "active").First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &presetapp.ProjectSelectionError{Code: "project_preset_selection_forbidden", Message: "Project Preset selection is forbidden"}
		}
		return err
	}
	var membership model.Membership
	if err := repository.database.WithContext(ctx).
		Where("workspace_id = ? AND user_id = ? AND status = ?", workspace, actor, "active").First(&membership).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &presetapp.ProjectSelectionError{Code: "project_preset_selection_forbidden", Message: "Project Preset selection is forbidden"}
		}
		return err
	}
	if membership.Role != "owner" && membership.Role != "editor" {
		return &presetapp.ProjectSelectionError{Code: "project_preset_selection_forbidden", Message: "Project Preset selection is forbidden"}
	}
	return nil
}

func (repository *projectSelectionRepository) FindReceipt(
	ctx context.Context,
	workspaceID string,
	operation string,
	idempotencyKey string,
) (platformcommand.Receipt, error) {
	return commandgorm.Find(ctx, repository.database, workspaceID, operation, idempotencyKey)
}

func (repository *projectSelectionRepository) EnsureReceipt(
	ctx context.Context,
	receipt platformcommand.Receipt,
) (platformcommand.Receipt, error) {
	return commandgorm.Ensure(ctx, repository.database, receipt)
}

func (repository *projectSelectionRepository) CurrentProjectSelection(
	ctx context.Context,
	workspaceID string,
	projectID string,
	forUpdate bool,
) (presetdomain.ProjectSelection, error) {
	workspace, workspaceErr := uuid.Parse(workspaceID)
	project, projectErr := uuid.Parse(projectID)
	if workspaceErr != nil || projectErr != nil {
		return presetdomain.ProjectSelection{}, presetapp.ErrProjectSelectionNotFound
	}
	query := repository.database.WithContext(ctx).Where("workspace_id = ? AND project_id = ?", workspace, project)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var head model.ProjectPresetSelectionHead
	if err := query.First(&head).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return presetdomain.ProjectSelection{}, presetapp.ErrProjectSelectionNotFound
		}
		return presetdomain.ProjectSelection{}, err
	}
	var record model.ProjectPresetSelection
	if err := repository.database.WithContext(ctx).First(&record, "id = ?", head.CurrentSelectionID).Error; err != nil {
		return presetdomain.ProjectSelection{}, err
	}
	selection, _, err := presetdomain.DecodeProjectSelection(json.RawMessage(record.Selection))
	if err != nil || !projectSelectionRecordMatches(record, head, selection) {
		return presetdomain.ProjectSelection{}, errors.New("Project Preset selection persistence has drifted")
	}
	return selection, nil
}

func (repository *projectSelectionRepository) CreateProjectSelection(
	ctx context.Context,
	selection presetdomain.ProjectSelection,
	encoded []byte,
) error {
	record, err := projectSelectionRecord(selection, encoded)
	if err != nil {
		return err
	}
	return repository.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repository *projectSelectionRepository) AdvanceProjectSelectionHead(
	ctx context.Context,
	selection presetdomain.ProjectSelection,
	expectedRevision int64,
) error {
	workspaceID, err := uuid.Parse(selection.WorkspaceID)
	if err != nil {
		return err
	}
	projectID, err := uuid.Parse(selection.ProjectID)
	if err != nil {
		return err
	}
	selectionID, err := uuid.Parse(selection.ID)
	if err != nil {
		return err
	}
	head := model.ProjectPresetSelectionHead{
		WorkspaceID: workspaceID, ProjectID: projectID, CurrentSelectionID: selectionID,
		CurrentContentHash: selection.ContentHash, Revision: selection.Revision, UpdatedAt: selection.SelectedAt,
	}
	if expectedRevision == 0 {
		return repository.database.WithContext(ctx).Omit(clause.Associations).Create(&head).Error
	}
	result := repository.database.WithContext(ctx).Model(&model.ProjectPresetSelectionHead{}).
		Where("workspace_id = ? AND project_id = ? AND revision = ?", workspaceID, projectID, expectedRevision).
		Updates(map[string]any{
			"current_selection_id": selectionID, "current_content_hash": selection.ContentHash,
			"revision": selection.Revision, "updated_at": selection.SelectedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return &presetapp.ProjectSelectionError{Code: "project_preset_selection_conflict", Message: "Project Preset selection changed before update"}
	}
	return nil
}

func projectSelectionRecord(selection presetdomain.ProjectSelection, encoded []byte) (model.ProjectPresetSelection, error) {
	id, err := uuid.Parse(selection.ID)
	if err != nil {
		return model.ProjectPresetSelection{}, err
	}
	workspaceID, err := uuid.Parse(selection.WorkspaceID)
	if err != nil {
		return model.ProjectPresetSelection{}, err
	}
	projectID, err := uuid.Parse(selection.ProjectID)
	if err != nil {
		return model.ProjectPresetSelection{}, err
	}
	selectedBy, err := uuid.Parse(selection.SelectedBy)
	if err != nil {
		return model.ProjectPresetSelection{}, err
	}
	var parentID *uuid.UUID
	if selection.ParentSelectionID != nil {
		parsed, parseErr := uuid.Parse(*selection.ParentSelectionID)
		if parseErr != nil {
			return model.ProjectPresetSelection{}, parseErr
		}
		parentID = &parsed
	}
	return model.ProjectPresetSelection{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, Revision: selection.Revision,
		ParentSelectionID: parentID, ParentContentHash: selection.ParentContentHash,
		PresetKey: selection.PresetRelease.Key, PresetRelease: selection.PresetRelease.Release,
		PresetContentHash: selection.PresetRelease.ContentHash, ApplicationMode: selection.ApplicationMode,
		Selection: datatypes.JSON(encoded), ContentHash: selection.ContentHash, SelectedBy: selectedBy,
		SelectedAt: selection.SelectedAt, CreatedAt: selection.SelectedAt,
	}, nil
}

func projectSelectionRecordMatches(
	record model.ProjectPresetSelection,
	head model.ProjectPresetSelectionHead,
	selection presetdomain.ProjectSelection,
) bool {
	parentSelectionMatches := record.ParentSelectionID == nil && selection.ParentSelectionID == nil
	if record.ParentSelectionID != nil && selection.ParentSelectionID != nil {
		parentSelectionMatches = record.ParentSelectionID.String() == *selection.ParentSelectionID
	}
	return record.ID.String() == selection.ID && record.WorkspaceID.String() == selection.WorkspaceID &&
		record.ProjectID.String() == selection.ProjectID && record.Revision == selection.Revision &&
		record.PresetKey == selection.PresetRelease.Key && record.PresetRelease == selection.PresetRelease.Release &&
		record.PresetContentHash == selection.PresetRelease.ContentHash && record.ApplicationMode == selection.ApplicationMode &&
		record.ContentHash == selection.ContentHash && record.SelectedBy.String() == selection.SelectedBy &&
		record.SelectedAt.Equal(selection.SelectedAt) && head.CurrentSelectionID == record.ID &&
		head.CurrentContentHash == record.ContentHash && head.Revision == record.Revision &&
		parentSelectionMatches && reflect.DeepEqual(record.ParentContentHash, selection.ParentContentHash)
}
