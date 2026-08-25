package gormdb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformcommandgorm "github.com/StephenQiu30/lanverse/backend/internal/platform/command/adapter/gormdb"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type Store struct{ database *gorm.DB }

type repository struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (store *Store) WithinTransaction(ctx context.Context, run func(application.Repository) error) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return run(&repository{database: transaction})
	})
}

func (repo *repository) FindReceipt(ctx context.Context, workspaceID, operation, idempotencyKey string) (platformcommand.Receipt, error) {
	return platformcommandgorm.Find(ctx, repo.database, workspaceID, operation, idempotencyKey)
}

func (repo *repository) CreateReceipt(ctx context.Context, receipt platformcommand.Receipt) error {
	if err := platformcommandgorm.Create(ctx, repo.database, receipt); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return &application.Error{Code: "resource_conflict", Message: "Idempotency key is already in use", Status: 409}
		}
		return err
	}
	return nil
}

func (repo *repository) EnsureCompilation(ctx context.Context, desired domain.CompiledFacts) (domain.CompiledFacts, error) {
	definition, snapshot, err := compilationRecords(desired)
	if err != nil {
		return domain.CompiledFacts{}, err
	}
	if err = repo.database.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "authoring_revision_id"}, {Name: "compiler_version"}}, DoNothing: true,
	}).Omit(clause.Associations).Create(&definition).Error; err != nil {
		return domain.CompiledFacts{}, err
	}
	var persistedDefinition model.WorkflowDefinitionVersion
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("authoring_revision_id = ? AND compiler_version = ?", definition.AuthoringRevisionID, definition.CompilerVersion).
		First(&persistedDefinition).Error; err != nil {
		return domain.CompiledFacts{}, normalizeNotFound(err)
	}
	if !sameDefinition(persistedDefinition, definition) {
		return domain.CompiledFacts{}, fmt.Errorf("workflow definition for authoring revision %s and compiler %s has drifted", definition.AuthoringRevisionID, definition.CompilerVersion)
	}

	snapshot.WorkflowDefinitionVersionID = persistedDefinition.ID
	if err = repo.database.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "workflow_definition_version_id"}}, DoNothing: true,
	}).Omit(clause.Associations).Create(&snapshot).Error; err != nil {
		return domain.CompiledFacts{}, err
	}
	var persistedSnapshot model.RunInputSnapshot
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workflow_definition_version_id = ?", persistedDefinition.ID).First(&persistedSnapshot).Error; err != nil {
		return domain.CompiledFacts{}, normalizeNotFound(err)
	}
	if !sameSnapshot(persistedSnapshot, snapshot) {
		return domain.CompiledFacts{}, fmt.Errorf("run input snapshot for workflow definition %s has drifted", persistedDefinition.ID)
	}
	return compiledFacts(persistedDefinition, persistedSnapshot)
}

func (repo *repository) GetCompilation(ctx context.Context, definitionID, snapshotID string) (domain.CompiledFacts, error) {
	definition, err := uuid.Parse(definitionID)
	if err != nil {
		return domain.CompiledFacts{}, application.ErrNotFound
	}
	snapshot, err := uuid.Parse(snapshotID)
	if err != nil {
		return domain.CompiledFacts{}, application.ErrNotFound
	}
	var definitionRecord model.WorkflowDefinitionVersion
	if err = repo.database.WithContext(ctx).First(&definitionRecord, "id = ?", definition).Error; err != nil {
		return domain.CompiledFacts{}, normalizeNotFound(err)
	}
	var snapshotRecord model.RunInputSnapshot
	if err = repo.database.WithContext(ctx).
		Where("id = ? AND workflow_definition_version_id = ?", snapshot, definition).
		First(&snapshotRecord).Error; err != nil {
		return domain.CompiledFacts{}, normalizeNotFound(err)
	}
	return compiledFacts(definitionRecord, snapshotRecord)
}

func compilationRecords(value domain.CompiledFacts) (model.WorkflowDefinitionVersion, model.RunInputSnapshot, error) {
	definitionID, err := uuid.Parse(value.DefinitionID)
	if err != nil {
		return model.WorkflowDefinitionVersion{}, model.RunInputSnapshot{}, err
	}
	snapshotID, err := uuid.Parse(value.RunInputSnapshotID)
	if err != nil {
		return model.WorkflowDefinitionVersion{}, model.RunInputSnapshot{}, err
	}
	workspaceID, err := uuid.Parse(value.Definition.WorkspaceID)
	if err != nil {
		return model.WorkflowDefinitionVersion{}, model.RunInputSnapshot{}, err
	}
	projectID, err := uuid.Parse(value.Definition.ProjectID)
	if err != nil {
		return model.WorkflowDefinitionVersion{}, model.RunInputSnapshot{}, err
	}
	revisionID, err := uuid.Parse(value.Definition.AuthoringRevisionID)
	if err != nil {
		return model.WorkflowDefinitionVersion{}, model.RunInputSnapshot{}, err
	}
	catalogID, err := uuid.Parse(value.Definition.NodeCatalogVersionID)
	if err != nil {
		return model.WorkflowDefinitionVersion{}, model.RunInputSnapshot{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.WorkflowDefinitionVersion{}, model.RunInputSnapshot{}, err
	}
	encodedDefinition, err := json.Marshal(value.Definition)
	if err != nil {
		return model.WorkflowDefinitionVersion{}, model.RunInputSnapshot{}, err
	}
	encodedSnapshot, err := json.Marshal(value.RunInputSnapshot)
	if err != nil {
		return model.WorkflowDefinitionVersion{}, model.RunInputSnapshot{}, err
	}
	definition := model.WorkflowDefinitionVersion{
		ID: definitionID, WorkspaceID: workspaceID, ProjectID: projectID, AuthoringRevisionID: revisionID,
		NodeCatalogVersionID: catalogID, CompilerVersion: value.Definition.CompilerVersion,
		WorkflowType: value.Definition.WorkflowType, WorkflowTypeVersion: value.Definition.WorkflowTypeVersion,
		RuntimeContractVersion: value.Definition.RuntimeContractVersion, Definition: datatypes.JSON(encodedDefinition),
		ContentHash: value.Definition.ContentHash, CreatedBy: createdBy, CreatedAt: value.CreatedAt,
	}
	snapshot := model.RunInputSnapshot{
		ID: snapshotID, WorkspaceID: workspaceID, ProjectID: projectID, WorkflowDefinitionVersionID: definitionID,
		AuthoringRevisionID: revisionID, Snapshot: datatypes.JSON(encodedSnapshot),
		ContentHash: value.RunInputSnapshot.ContentHash, CreatedBy: createdBy, CreatedAt: value.CreatedAt,
	}
	return definition, snapshot, nil
}

func compiledFacts(definitionRecord model.WorkflowDefinitionVersion, snapshotRecord model.RunInputSnapshot) (domain.CompiledFacts, error) {
	var definition domain.WorkflowDefinitionVersion
	if err := json.Unmarshal(definitionRecord.Definition, &definition); err != nil {
		return domain.CompiledFacts{}, err
	}
	var snapshot domain.RunInputSnapshot
	if err := json.Unmarshal(snapshotRecord.Snapshot, &snapshot); err != nil {
		return domain.CompiledFacts{}, err
	}
	if definition.ContentHash != definitionRecord.ContentHash || snapshot.ContentHash != snapshotRecord.ContentHash ||
		definition.AuthoringRevisionID != definitionRecord.AuthoringRevisionID.String() ||
		snapshot.AuthoringRevisionID != snapshotRecord.AuthoringRevisionID.String() ||
		snapshot.WorkflowDefinitionHash != definition.ContentHash {
		return domain.CompiledFacts{}, errors.New("persisted workflow compilation metadata has drifted")
	}
	return domain.CompiledFacts{
		DefinitionID: definitionRecord.ID.String(), RunInputSnapshotID: snapshotRecord.ID.String(),
		Compilation: domain.Compilation{Definition: definition, RunInputSnapshot: snapshot},
		CreatedBy:   definitionRecord.CreatedBy.String(), CreatedAt: definitionRecord.CreatedAt,
	}, nil
}

func sameDefinition(left, right model.WorkflowDefinitionVersion) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.AuthoringRevisionID == right.AuthoringRevisionID && left.NodeCatalogVersionID == right.NodeCatalogVersionID &&
		left.CompilerVersion == right.CompilerVersion && left.WorkflowType == right.WorkflowType &&
		left.WorkflowTypeVersion == right.WorkflowTypeVersion && left.RuntimeContractVersion == right.RuntimeContractVersion &&
		left.ContentHash == right.ContentHash && equalJSON(left.Definition, right.Definition)
}

func sameSnapshot(left, right model.RunInputSnapshot) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.WorkflowDefinitionVersionID == right.WorkflowDefinitionVersionID &&
		left.AuthoringRevisionID == right.AuthoringRevisionID && left.ContentHash == right.ContentHash &&
		equalJSON(left.Snapshot, right.Snapshot)
}

func equalJSON(left, right []byte) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	leftCanonical, leftErr := json.Marshal(leftValue)
	rightCanonical, rightErr := json.Marshal(rightValue)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftCanonical, rightCanonical)
}

func normalizeNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrNotFound
	}
	return err
}
