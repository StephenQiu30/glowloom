package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	"github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformcommandgorm "github.com/StephenQiu30/lanverse/backend/internal/platform/command/adapter/gormdb"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

type repository struct{ database *gorm.DB }

func (store *Store) WithinTransaction(ctx context.Context, run func(application.Repository) error) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return run(&repository{database: transaction})
	})
}

func (repo *repository) ProjectScope(ctx context.Context, actor application.Actor, projectID string, write bool) (string, error) {
	id, err := uuid.Parse(projectID)
	if err != nil {
		return "", application.ErrNotFound
	}
	project, err := authorizeProject(ctx, repo.database, actor, id, write)
	if err != nil {
		return "", err
	}
	return project.WorkspaceID.String(), nil
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

func (repo *repository) Catalog(ctx context.Context, key, version string) (string, domain.Catalog, error) {
	var catalogRecord model.NodeCatalogVersion
	if err := repo.database.WithContext(ctx).Where("key = ? AND version = ? AND status = ?", key, version, "published").First(&catalogRecord).Error; err != nil {
		return "", domain.Catalog{}, normalizeNotFound(err)
	}
	var references []definitionReference
	if err := json.Unmarshal(catalogRecord.Definitions, &references); err != nil || len(references) == 0 {
		return "", domain.Catalog{}, errors.New("persisted node catalog references are invalid")
	}
	definitions := make([]domain.NodeDefinition, 0, len(references))
	for _, reference := range references {
		definitionID, err := uuid.Parse(reference.ID)
		if err != nil {
			return "", domain.Catalog{}, errors.New("persisted node definition reference is invalid")
		}
		var record model.NodeDefinitionVersion
		if err = repo.database.WithContext(ctx).First(&record, "id = ?", definitionID).Error; err != nil {
			return "", domain.Catalog{}, normalizeNotFound(err)
		}
		if record.Key != reference.Key || record.Version != reference.Version || record.ContentHash != reference.ContentHash {
			return "", domain.Catalog{}, errors.New("persisted node definition reference has drifted")
		}
		var inputs, outputs []domain.PortDefinition
		if err = json.Unmarshal(record.InputPorts, &inputs); err != nil {
			return "", domain.Catalog{}, err
		}
		if err = json.Unmarshal(record.OutputPorts, &outputs); err != nil {
			return "", domain.Catalog{}, err
		}
		definitions = append(definitions, domain.NodeDefinition{
			Key: record.Key, Version: record.Version, Name: record.Name, Category: record.Category,
			Executor: record.Executor, InputPorts: inputs, OutputPorts: outputs,
			ConfigSchema: append([]byte(nil), record.ConfigSchema...), CachePolicy: record.CachePolicy,
			RiskLevel: record.RiskLevel, Executable: record.Executable,
		})
	}
	catalog, err := domain.NewCatalog(catalogRecord.Key, catalogRecord.Version, definitions)
	if err != nil {
		return "", domain.Catalog{}, err
	}
	if catalog.ContentHash != catalogRecord.ContentHash || catalog.ExecutionHash != catalogRecord.ExecutionHash {
		return "", domain.Catalog{}, errors.New("persisted node catalog content has drifted")
	}
	return catalogRecord.ID.String(), catalog, nil
}

func (repo *repository) VerifyFrozenInputs(ctx context.Context, projectID string, inputs []domain.FrozenReference) error {
	project, err := uuid.Parse(projectID)
	if err != nil {
		return application.ErrNotFound
	}
	if len(inputs) == 0 {
		return &application.Error{Code: "validation_failed", Message: "At least one frozen input is required", Status: 422}
	}
	for _, input := range inputs {
		if input.Kind != "script_revision" {
			return &application.Error{Code: "validation_failed", Message: "Unsupported frozen input kind", Status: 422}
		}
		revisionID, parseErr := uuid.Parse(input.ID)
		if parseErr != nil {
			return &application.Error{Code: "validation_failed", Message: "Invalid frozen input", Status: 422}
		}
		var revision model.DocumentRevision
		if err = repo.database.WithContext(ctx).First(&revision, "id = ?", revisionID).Error; err != nil {
			return normalizeNotFound(err)
		}
		var document model.ScriptDocument
		if err = repo.database.WithContext(ctx).First(&document, "id = ?", revision.DocumentID).Error; err != nil {
			return normalizeNotFound(err)
		}
		if document.ProjectID != project || input.Version != strconv.Itoa(revision.VersionNo) || input.Hash != revision.NormalizedHash {
			return &application.Error{Code: "resource_conflict", Message: "Frozen input differs from the published script revision", Status: 409}
		}
	}
	return nil
}

func (repo *repository) CreateDraft(ctx context.Context, value domain.Draft) error {
	record, err := draftRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) GetDraft(ctx context.Context, actor application.Actor, draftID string, forUpdate bool) (domain.Draft, error) {
	id, err := uuid.Parse(draftID)
	if err != nil {
		return domain.Draft{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx).Where("id = ?", id)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.AuthoringDraft
	if err = query.First(&record).Error; err != nil {
		return domain.Draft{}, normalizeNotFound(err)
	}
	if _, err = authorizeProject(ctx, repo.database, actor, record.ProjectID, forUpdate); err != nil {
		return domain.Draft{}, err
	}
	var catalog model.NodeCatalogVersion
	if err = repo.database.WithContext(ctx).First(&catalog, "id = ?", record.NodeCatalogVersionID).Error; err != nil {
		return domain.Draft{}, normalizeNotFound(err)
	}
	return draftDomain(record, catalog)
}

func (repo *repository) UpdateDraft(ctx context.Context, value domain.Draft, expectedRevision int) error {
	record, err := draftRecord(value)
	if err != nil {
		return err
	}
	result := repo.database.WithContext(ctx).Model(&model.AuthoringDraft{}).
		Where("id = ? AND revision = ?", record.ID, expectedRevision).
		Updates(map[string]any{
			"graph": record.Graph, "layout": record.Layout, "frozen_inputs": record.FrozenInputs,
			"node_catalog_version_id": record.NodeCatalogVersionID, "revision": record.Revision, "updated_at": record.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return &application.Error{Code: "resource_conflict", Message: "Authoring draft changed before update", Status: 409}
	}
	return nil
}

func (repo *repository) CreateRevision(ctx context.Context, draft domain.Draft, value domain.Revision) error {
	record, err := revisionRecord(value)
	if err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return &application.Error{Code: "resource_conflict", Message: "Authoring draft revision is already published", Status: 409}
		}
		return err
	}
	result := repo.database.WithContext(ctx).Model(&model.AuthoringDraft{}).
		Where("id = ? AND revision = ?", record.DraftID, draft.Revision).
		Update("current_published_revision_id", record.ID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return &application.Error{Code: "resource_conflict", Message: "Authoring draft changed before publish", Status: 409}
	}
	return nil
}

func (repo *repository) GetRevision(ctx context.Context, actor application.Actor, revisionID string) (domain.Revision, error) {
	id, err := uuid.Parse(revisionID)
	if err != nil {
		return domain.Revision{}, application.ErrNotFound
	}
	var record model.AuthoringRevision
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return domain.Revision{}, normalizeNotFound(err)
	}
	if _, err = authorizeProject(ctx, repo.database, actor, record.ProjectID, false); err != nil {
		return domain.Revision{}, err
	}
	var catalog model.NodeCatalogVersion
	if err = repo.database.WithContext(ctx).First(&catalog, "id = ?", record.NodeCatalogVersionID).Error; err != nil {
		return domain.Revision{}, normalizeNotFound(err)
	}
	return revisionDomain(record, catalog)
}

func (repo *repository) AuthorizeRevisionExecution(
	ctx context.Context,
	actor application.Actor,
	revisionID string,
) error {
	id, err := uuid.Parse(revisionID)
	if err != nil {
		return application.ErrNotFound
	}
	var record model.AuthoringRevision
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return normalizeNotFound(err)
	}
	_, err = authorizeProject(ctx, repo.database, actor, record.ProjectID, true)
	return err
}

func authorizeProject(ctx context.Context, database *gorm.DB, actor application.Actor, projectID uuid.UUID, write bool) (model.Project, error) {
	userID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return model.Project{}, unauthenticated()
	}
	var user model.UserAccount
	if err = database.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil || user.Status != "active" || user.TokenVersion != actor.TokenVersion {
		return model.Project{}, unauthenticated()
	}
	var project model.Project
	if err = database.WithContext(ctx).First(&project, "id = ?", projectID).Error; err != nil {
		return model.Project{}, normalizeNotFound(err)
	}
	var workspace model.Workspace
	if err = database.WithContext(ctx).First(&workspace, "id = ?", project.WorkspaceID).Error; err != nil {
		return model.Project{}, normalizeNotFound(err)
	}
	var membership model.Membership
	if err = database.WithContext(ctx).Where("workspace_id = ? AND user_id = ? AND status = ?", project.WorkspaceID, userID, "active").First(&membership).Error; err != nil {
		return model.Project{}, application.ErrNotFound
	}
	if write && (membership.Role == "viewer" || workspace.Status != "active" || project.Status != "active") {
		return model.Project{}, &application.Error{Code: "forbidden", Message: "Action is not allowed", Status: 403}
	}
	return project, nil
}

func draftRecord(value domain.Draft) (model.AuthoringDraft, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.AuthoringDraft{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.AuthoringDraft{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.AuthoringDraft{}, err
	}
	catalogID, err := uuid.Parse(value.CatalogID)
	if err != nil {
		return model.AuthoringDraft{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.AuthoringDraft{}, err
	}
	graph, err := json.Marshal(value.Graph)
	if err != nil {
		return model.AuthoringDraft{}, err
	}
	inputs, err := json.Marshal(value.FrozenInputs)
	if err != nil {
		return model.AuthoringDraft{}, err
	}
	var current *uuid.UUID
	if value.CurrentPublishedRevisionID != nil {
		parsed, parseErr := uuid.Parse(*value.CurrentPublishedRevisionID)
		if parseErr != nil {
			return model.AuthoringDraft{}, parseErr
		}
		current = &parsed
	}
	return model.AuthoringDraft{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, AuthoringMode: value.AuthoringMode,
		Graph: datatypes.JSON(graph), Layout: datatypes.JSON(value.Layout), FrozenInputs: datatypes.JSON(inputs),
		NodeCatalogVersionID: catalogID, Status: value.Status, Revision: value.Revision,
		CurrentPublishedRevisionID: current, CreatedBy: createdBy, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func draftDomain(record model.AuthoringDraft, catalog model.NodeCatalogVersion) (domain.Draft, error) {
	var graph domain.Graph
	if err := json.Unmarshal(record.Graph, &graph); err != nil {
		return domain.Draft{}, err
	}
	var inputs []domain.FrozenReference
	if err := json.Unmarshal(record.FrozenInputs, &inputs); err != nil {
		return domain.Draft{}, err
	}
	var current *string
	if record.CurrentPublishedRevisionID != nil {
		value := record.CurrentPublishedRevisionID.String()
		current = &value
	}
	return domain.Draft{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		CatalogID: record.NodeCatalogVersionID.String(), AuthoringMode: record.AuthoringMode,
		Graph: graph, Layout: append([]byte(nil), record.Layout...), FrozenInputs: inputs,
		CatalogKey: catalog.Key, CatalogVersion: catalog.Version, Status: record.Status, Revision: record.Revision,
		CurrentPublishedRevisionID: current, CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}, nil
}

func revisionRecord(value domain.Revision) (model.AuthoringRevision, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.AuthoringRevision{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.AuthoringRevision{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.AuthoringRevision{}, err
	}
	draftID, err := uuid.Parse(value.DraftID)
	if err != nil {
		return model.AuthoringRevision{}, err
	}
	catalogID, err := uuid.Parse(value.CatalogID)
	if err != nil {
		return model.AuthoringRevision{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.AuthoringRevision{}, err
	}
	graph, err := json.Marshal(value.Graph)
	if err != nil {
		return model.AuthoringRevision{}, err
	}
	inputs, err := json.Marshal(value.FrozenInputs)
	if err != nil {
		return model.AuthoringRevision{}, err
	}
	return model.AuthoringRevision{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, DraftID: draftID, RevisionNo: value.RevisionNo,
		AuthoringMode: value.AuthoringMode, Graph: datatypes.JSON(graph), Layout: datatypes.JSON(value.Layout),
		FrozenInputs: datatypes.JSON(inputs), NodeCatalogVersionID: catalogID,
		CatalogContentHash: value.CatalogHash, CatalogExecutionHash: value.CatalogExecutionHash,
		ExecutionHash: value.ExecutionHash, ContentHash: value.ContentHash, CreatedBy: createdBy, CreatedAt: value.CreatedAt,
	}, nil
}

func revisionDomain(record model.AuthoringRevision, catalog model.NodeCatalogVersion) (domain.Revision, error) {
	var graph domain.Graph
	if err := json.Unmarshal(record.Graph, &graph); err != nil {
		return domain.Revision{}, err
	}
	var inputs []domain.FrozenReference
	if err := json.Unmarshal(record.FrozenInputs, &inputs); err != nil {
		return domain.Revision{}, err
	}
	return domain.Revision{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		DraftID: record.DraftID.String(), CatalogID: record.NodeCatalogVersionID.String(), RevisionNo: record.RevisionNo,
		RevisionSnapshot: domain.RevisionSnapshot{
			AuthoringMode: record.AuthoringMode, Graph: graph, Layout: append([]byte(nil), record.Layout...), FrozenInputs: inputs,
			CatalogKey: catalog.Key, CatalogVersion: catalog.Version, CatalogHash: record.CatalogContentHash,
			CatalogExecutionHash: record.CatalogExecutionHash, ExecutionHash: record.ExecutionHash, ContentHash: record.ContentHash,
		},
		CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt,
	}, nil
}

func normalizeNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrNotFound
	}
	return err
}

func unauthenticated() error {
	return &application.Error{Code: "unauthenticated", Message: "Invalid credentials", Status: 401, NextAction: "login"}
}
