package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformcommandgorm "github.com/StephenQiu30/lanverse/backend/internal/platform/command/adapter/gormdb"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

type Store struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

type repository struct{ database *gorm.DB }

func (store *Store) WithinTransaction(ctx context.Context, run func(application.Repository) error) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(tx *gorm.DB) error {
		return run(&repository{database: tx})
	})
}

func (repo *repository) ProjectScope(ctx context.Context, actor application.Actor, projectID string, write bool) (string, error) {
	id, err := uuid.Parse(projectID)
	if err != nil {
		return "", notFound()
	}
	userID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return "", unauthenticated()
	}
	var user model.UserAccount
	if err = repo.database.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil || user.Status != "active" || user.TokenVersion != actor.TokenVersion {
		return "", unauthenticated()
	}
	query := repo.database.WithContext(ctx)
	if write {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var project model.Project
	if err = query.First(&project, "id = ?", id).Error; err != nil {
		return "", normalizeNotFound(err)
	}
	var membership model.Membership
	if err = repo.database.WithContext(ctx).Where("workspace_id = ? AND user_id = ? AND status = ?", project.WorkspaceID, userID, "active").First(&membership).Error; err != nil {
		return "", notFound()
	}
	if write {
		var workspace model.Workspace
		if err = repo.database.WithContext(ctx).First(&workspace, "id = ?", project.WorkspaceID).Error; err != nil {
			return "", normalizeNotFound(err)
		}
		if membership.Role == "viewer" || workspace.Status != "active" || project.Status != "active" {
			return "", &application.Error{Code: "forbidden", Message: "Canvas editing is not allowed", Status: 403}
		}
	}
	return project.WorkspaceID.String(), nil
}

func (repo *repository) FindDocument(ctx context.Context, projectID string, forUpdate bool) (domain.Document, bool, error) {
	id, err := uuid.Parse(projectID)
	if err != nil {
		return domain.Document{}, false, notFound()
	}
	query := repo.database.WithContext(ctx)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.CanvasDocument
	if err = query.First(&record, "project_id = ?", id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Document{}, false, nil
	} else if err != nil {
		return domain.Document{}, false, err
	}
	var document domain.Document
	if err = json.Unmarshal(record.Content, &document); err != nil || document.ProjectID != projectID || document.Revision != record.Revision || document.SchemaVersion != domain.SchemaVersion {
		return domain.Document{}, false, errors.New("stored canvas document is invalid")
	}
	return document, true, nil
}

func (repo *repository) SaveDocument(ctx context.Context, workspaceID string, document domain.Document) error {
	projectID, err := uuid.Parse(document.ProjectID)
	if err != nil {
		return notFound()
	}
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return notFound()
	}
	content, err := json.Marshal(document)
	if err != nil {
		return err
	}
	if len(content) > 8<<20 {
		return &application.Error{Code: "invalid_request", Message: "Canvas document exceeds the size limit", Status: 422}
	}
	now := time.Now().UTC()
	if document.Revision == 1 {
		record := model.CanvasDocument{ProjectID: projectID, WorkspaceID: workspace, Revision: 1, Content: datatypes.JSON(content), CreatedAt: now, UpdatedAt: now}
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; errors.Is(err, gorm.ErrDuplicatedKey) {
			return conflict()
		}
		return err
	}
	result := repo.database.WithContext(ctx).Model(&model.CanvasDocument{}).
		Where("project_id = ? AND workspace_id = ? AND revision = ?", projectID, workspace, document.Revision-1).
		Updates(map[string]any{"revision": document.Revision, "content": datatypes.JSON(content), "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return conflict()
	}
	return nil
}

func (repo *repository) ValidateMediaVersion(ctx context.Context, workspaceID, versionID, kind string) error {
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return notFound()
	}
	id, err := uuid.Parse(versionID)
	if err != nil {
		return &application.Error{Code: "invalid_request", Message: "Invalid media version", Status: 422}
	}
	var version model.MediaVersion
	if err = repo.database.WithContext(ctx).First(&version, "id = ? AND workspace_id = ?", id, workspace).Error; err != nil {
		return normalizeNotFound(err)
	}
	var object model.MediaObject
	if err = repo.database.WithContext(ctx).First(&object, "id = ? AND workspace_id = ?", version.MediaObjectID, workspace).Error; err != nil {
		return normalizeNotFound(err)
	}
	if object.Kind != kind || object.Status != "active" || version.ProbeStatus != "ready" {
		return &application.Error{Code: "resource_conflict", Message: "Media version is not ready for this canvas node", Status: 409}
	}
	return nil
}

func (repo *repository) FindReceipt(ctx context.Context, workspaceID, operation, idempotencyKey string) (platformcommand.Receipt, error) {
	return platformcommandgorm.Find(ctx, repo.database, workspaceID, operation, idempotencyKey)
}

func (repo *repository) CreateReceipt(ctx context.Context, receipt platformcommand.Receipt) error {
	if err := platformcommandgorm.Create(ctx, repo.database, receipt); errors.Is(err, gorm.ErrDuplicatedKey) {
		return conflict()
	} else {
		return err
	}
}

func normalizeNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound()
	}
	return err
}

func notFound() error {
	return &application.Error{Code: "not_found", Message: "Canvas project or resource not found", Status: 404}
}
func unauthenticated() error {
	return &application.Error{Code: "unauthenticated", Message: "Invalid credentials", Status: 401, NextAction: "login"}
}
func conflict() error {
	return &application.Error{Code: "resource_conflict", Message: "Canvas document changed", Status: 409, NextAction: "reload_canvas"}
}
