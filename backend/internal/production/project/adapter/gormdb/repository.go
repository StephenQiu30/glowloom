package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
)

type Store struct{ database *gorm.DB }
type repository struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (store *Store) WithinTransaction(ctx context.Context, operation func(application.Repository) error) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&repository{database: transaction})
	})
}

func (repo *repository) Authorize(ctx context.Context, actor application.Actor, workspaceID string, capability application.Capability) error {
	userID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return unauthenticated()
	}
	workspaceUUID, err := uuid.Parse(workspaceID)
	if err != nil {
		return notFound("Workspace not found")
	}
	var user model.UserAccount
	if err = repo.database.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFound("Workspace not found")
		}
		return err
	}
	var workspace model.Workspace
	if err = repo.database.WithContext(ctx).First(&workspace, "id = ?", workspaceUUID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFound("Workspace not found")
		}
		return err
	}
	var membership model.Membership
	if err = repo.database.WithContext(ctx).
		Where("workspace_id = ? AND user_id = ? AND status = ?", workspaceUUID, userID, "active").
		First(&membership).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFound("Workspace not found")
		}
		return err
	}
	if user.Status != "active" || user.TokenVersion != actor.TokenVersion {
		return unauthenticated()
	}
	if !allowed(membership.Role, capability) || (workspace.Status == "archived" && capability != application.ContentRead) {
		return &application.Error{Code: application.CodeForbidden, Message: "Insufficient workspace capability", Status: 403}
	}
	return nil
}

func allowed(role string, capability application.Capability) bool {
	switch role {
	case "owner":
		return true
	case "editor":
		return capability == application.ContentRead || capability == application.ContentWrite
	case "viewer":
		return capability == application.ContentRead
	default:
		return false
	}
}

func (repo *repository) Create(ctx context.Context, project domain.Project) error {
	record, err := projectRecord(project)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) Get(ctx context.Context, id string, forUpdate bool) (domain.Project, error) {
	projectID, err := uuid.Parse(id)
	if err != nil {
		return domain.Project{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx).Where("id = ?", projectID)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.Project
	if err = query.First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Project{}, application.ErrNotFound
		}
		return domain.Project{}, err
	}
	return projectDomain(record), nil
}

func (repo *repository) Save(ctx context.Context, project domain.Project) error {
	record, err := projectRecord(project)
	if err != nil {
		return err
	}
	result := repo.database.WithContext(ctx).
		Model(&model.Project{}).
		Where("id = ?", record.ID).
		Select("name", "description", "aspect_ratio", "language", "visual_style", "target_duration_ms", "status", "revision", "archived_at", "archived_by", "updated_at").
		Updates(&record)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return application.ErrNotFound
	}
	return nil
}

func (repo *repository) Delete(ctx context.Context, id string) error {
	projectID, err := uuid.Parse(id)
	if err != nil {
		return application.ErrNotFound
	}
	return repo.database.WithContext(ctx).Delete(&model.Project{}, "id = ?", projectID).Error
}

func (repo *repository) List(ctx context.Context, query application.ListQuery) ([]domain.Project, int, error) {
	workspaceID, err := uuid.Parse(query.WorkspaceID)
	if err != nil {
		return nil, 0, notFound("Workspace not found")
	}
	databaseQuery := repo.database.WithContext(ctx).Model(&model.Project{}).Where("workspace_id = ?", workspaceID)
	if !query.IncludeArchived {
		databaseQuery = databaseQuery.Where("status = ?", string(domain.StatusActive))
	}
	if query.Search != "" {
		pattern := "%" + query.Search + "%"
		databaseQuery = databaseQuery.Where("name ILIKE ? OR description ILIKE ?", pattern, pattern)
	}
	var total int64
	if err = databaseQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	sortColumn := map[string]string{"name": "name", "created_at": "created_at", "updated_at": "updated_at"}[query.Sort]
	order := clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: sortColumn}, Desc: query.Order == "desc"}}}
	var records []model.Project
	if err = databaseQuery.Clauses(order).Order("id").Limit(query.Limit).Offset(query.Offset).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	items := make([]domain.Project, len(records))
	for index, record := range records {
		items[index] = projectDomain(record)
	}
	return items, int(total), nil
}

func (repo *repository) Dependencies(ctx context.Context, projectID string) (application.DependencySummary, error) {
	id, err := uuid.Parse(projectID)
	if err != nil {
		return application.DependencySummary{}, application.ErrNotFound
	}
	result := application.DependencySummary{}
	if repo.database.Migrator().HasTable("prj_episodes") {
		var count int64
		if err = repo.database.WithContext(ctx).Table("prj_episodes").Where("project_id = ?", id).Count(&count).Error; err != nil {
			return application.DependencySummary{}, err
		}
		result.Episodes = int(count)
	}
	if repo.database.Migrator().HasTable("ast_assets") {
		var count int64
		if err = repo.database.WithContext(ctx).Table("ast_assets").Where("project_id = ?", id).Count(&count).Error; err != nil {
			return application.DependencySummary{}, err
		}
		result.Assets = int(count)
	}
	var costBudgetCount int64
	if err = repo.database.WithContext(ctx).Table("cst_budget_policies").Where("project_id = ?", id).Count(&costBudgetCount).Error; err != nil {
		return application.DependencySummary{}, err
	}
	result.CostBudgets = int(costBudgetCount)
	return result, nil
}

func (repo *repository) AppendAudit(ctx context.Context, event application.AuditEvent) error {
	workspaceID, err := uuid.Parse(event.WorkspaceID)
	if err != nil {
		return err
	}
	actorID, err := uuid.Parse(event.ActorID)
	if err != nil {
		return err
	}
	targetID, err := uuid.Parse(event.TargetID)
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return err
	}
	record := model.AuditEvent{
		ID:          uuid.New(),
		WorkspaceID: workspaceID,
		ActorID:     actorID,
		Action:      event.Action,
		TargetType:  "project",
		TargetID:    targetID,
		Result:      "succeeded",
		TraceID:     uuid.NewString(),
		Metadata:    datatypes.JSON(metadata),
		OccurredAt:  event.OccurredAt,
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) FindReceipt(ctx context.Context, workspaceID, operation, key string) (platformcommand.Receipt, error) {
	workspaceUUID, err := uuid.Parse(workspaceID)
	if err != nil {
		return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
	}
	var record model.CommandReceipt
	if err = repo.database.WithContext(ctx).Where("workspace_id = ? AND operation = ? AND idempotency_key = ?", workspaceUUID, operation, key).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
		}
		return platformcommand.Receipt{}, err
	}
	return receiptDomain(record), nil
}

func (repo *repository) FindReceiptByResource(ctx context.Context, resourceID, actorID, operation, key string) (platformcommand.Receipt, error) {
	resourceUUID, err := uuid.Parse(resourceID)
	if err != nil {
		return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
	}
	actorUUID, err := uuid.Parse(actorID)
	if err != nil {
		return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
	}
	var record model.CommandReceipt
	if err = repo.database.WithContext(ctx).Where("resource_id = ? AND created_by = ? AND operation = ? AND idempotency_key = ?", resourceUUID, actorUUID, operation, key).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
		}
		return platformcommand.Receipt{}, err
	}
	return receiptDomain(record), nil
}

func (repo *repository) CreateReceipt(ctx context.Context, receipt platformcommand.Receipt) error {
	id, err := uuid.Parse(receipt.ID)
	if err != nil {
		return err
	}
	workspaceID, err := uuid.Parse(receipt.WorkspaceID)
	if err != nil {
		return err
	}
	resourceID, err := uuid.Parse(receipt.ResourceID)
	if err != nil {
		return err
	}
	createdBy, err := uuid.Parse(receipt.CreatedBy)
	if err != nil {
		return err
	}
	record := model.CommandReceipt{ID: id, WorkspaceID: workspaceID, Operation: receipt.Operation, IdempotencyKey: receipt.IdempotencyKey, InputHash: receipt.InputHash, ResourceID: resourceID, Result: datatypes.JSON(receipt.Result), CreatedBy: createdBy, CreatedAt: receipt.CreatedAt}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return &application.Error{Code: application.CodeIdempotencyConflict, Message: "Idempotency key is already in use", Status: 409}
		}
		return err
	}
	return nil
}

func receiptDomain(record model.CommandReceipt) platformcommand.Receipt {
	return platformcommand.Receipt{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), Operation: record.Operation, IdempotencyKey: record.IdempotencyKey, InputHash: record.InputHash, ResourceID: record.ResourceID.String(), Result: append([]byte(nil), record.Result...), CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt}
}

func projectRecord(project domain.Project) (model.Project, error) {
	id, err := uuid.Parse(project.ID)
	if err != nil {
		return model.Project{}, fmt.Errorf("parse project id: %w", err)
	}
	workspaceID, err := uuid.Parse(project.WorkspaceID)
	if err != nil {
		return model.Project{}, fmt.Errorf("parse workspace id: %w", err)
	}
	var archivedBy *uuid.UUID
	if project.ArchivedBy != nil {
		parsed, parseErr := uuid.Parse(*project.ArchivedBy)
		if parseErr != nil {
			return model.Project{}, fmt.Errorf("parse project archiver: %w", parseErr)
		}
		archivedBy = &parsed
	}
	return model.Project{ID: id, WorkspaceID: workspaceID, Name: project.Name, Description: project.Description, AspectRatio: project.AspectRatio, Language: project.Language, VisualStyle: project.VisualStyle, TargetDurationMS: project.TargetDurationMS, Status: string(project.Status), Revision: project.Revision, ArchivedAt: project.ArchivedAt, ArchivedBy: archivedBy, CreatedAt: project.CreatedAt, UpdatedAt: project.UpdatedAt}, nil
}

func projectDomain(record model.Project) domain.Project {
	var archivedBy *string
	if record.ArchivedBy != nil {
		value := record.ArchivedBy.String()
		archivedBy = &value
	}
	return domain.Project{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), Name: record.Name, Description: record.Description, AspectRatio: record.AspectRatio, Language: record.Language, VisualStyle: record.VisualStyle, TargetDurationMS: record.TargetDurationMS, Status: domain.Status(record.Status), Revision: record.Revision, ArchivedAt: record.ArchivedAt, ArchivedBy: archivedBy, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}

func unauthenticated() error {
	return &application.Error{Code: application.CodeUnauthenticated, Message: "Invalid credentials", Status: 401, NextAction: "login"}
}
func notFound(message string) error {
	return &application.Error{Code: application.CodeNotFound, Message: message, Status: 404}
}
