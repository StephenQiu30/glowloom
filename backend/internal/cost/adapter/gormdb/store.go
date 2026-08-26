package gormdb

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	"github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	commandgorm "github.com/StephenQiu30/lanverse/backend/internal/platform/command/adapter/gormdb"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

type Store struct{ database *gorm.DB }
type repository struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (store *Store) WithinCostTransaction(ctx context.Context, operation func(application.Repository) error) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&repository{database: transaction})
	})
}

func (repo *repository) AuthorizeProject(
	ctx context.Context,
	actor application.Actor,
	projectID, access string,
) (application.ProjectScope, error) {
	actorID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return application.ProjectScope{}, unauthenticated()
	}
	project, err := uuid.Parse(projectID)
	if err != nil {
		return application.ProjectScope{}, notFound("Project not found")
	}
	var projectRecord model.Project
	if err = repo.database.WithContext(ctx).First(&projectRecord, "id = ?", project).Error; err != nil {
		return application.ProjectScope{}, normalizeProjectNotFound(err)
	}
	var user model.UserAccount
	if err = repo.database.WithContext(ctx).First(&user, "id = ?", actorID).Error; err != nil {
		return application.ProjectScope{}, normalizeProjectNotFound(err)
	}
	var workspace model.Workspace
	if err = repo.database.WithContext(ctx).First(&workspace, "id = ?", projectRecord.WorkspaceID).Error; err != nil {
		return application.ProjectScope{}, normalizeProjectNotFound(err)
	}
	var membership model.Membership
	if err = repo.database.WithContext(ctx).Where(
		"workspace_id = ? AND user_id = ? AND status = ?", projectRecord.WorkspaceID, actorID, "active",
	).First(&membership).Error; err != nil {
		return application.ProjectScope{}, normalizeProjectNotFound(err)
	}
	if user.Status != "active" || user.TokenVersion != actor.TokenVersion {
		return application.ProjectScope{}, unauthenticated()
	}
	if access == "read" {
		return application.ProjectScope{WorkspaceID: projectRecord.WorkspaceID.String(), ProjectID: projectRecord.ID.String()}, nil
	}
	if workspace.Status != "active" || projectRecord.Status != "active" {
		return application.ProjectScope{}, forbidden()
	}
	if access == "owner" && membership.Role != "owner" {
		return application.ProjectScope{}, forbidden()
	}
	if access == "write" && membership.Role != "owner" && membership.Role != "editor" {
		return application.ProjectScope{}, forbidden()
	}
	if access != "owner" && access != "write" {
		return application.ProjectScope{}, forbidden()
	}
	return application.ProjectScope{WorkspaceID: projectRecord.WorkspaceID.String(), ProjectID: projectRecord.ID.String()}, nil
}

func (repo *repository) FindReceipt(ctx context.Context, workspaceID, operation, key string) (platformcommand.Receipt, error) {
	return commandgorm.Find(ctx, repo.database, workspaceID, operation, key)
}

func (repo *repository) EnsureReceipt(ctx context.Context, receipt platformcommand.Receipt) (platformcommand.Receipt, error) {
	return commandgorm.Ensure(ctx, repo.database, receipt)
}

func (repo *repository) FindBudget(ctx context.Context, projectID string) (domain.BudgetPolicy, error) {
	project, err := uuid.Parse(projectID)
	if err != nil {
		return domain.BudgetPolicy{}, application.ErrBudgetNotFound
	}
	var record model.CostBudgetPolicy
	if err = repo.database.WithContext(ctx).Where("project_id = ?", project).First(&record).Error; err != nil {
		return domain.BudgetPolicy{}, normalizeBudgetNotFound(err)
	}
	return budgetDomain(record), nil
}

func (repo *repository) GetBudgetForUpdate(ctx context.Context, projectID string) (domain.BudgetPolicy, error) {
	project, err := uuid.Parse(projectID)
	if err != nil {
		return domain.BudgetPolicy{}, application.ErrBudgetNotFound
	}
	var record model.CostBudgetPolicy
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("project_id = ?", project).First(&record).Error; err != nil {
		return domain.BudgetPolicy{}, normalizeBudgetNotFound(err)
	}
	return budgetDomain(record), nil
}

func (repo *repository) EnsureBudget(ctx context.Context, desired domain.BudgetPolicy) (domain.BudgetPolicy, error) {
	record, err := budgetRecord(desired)
	if err != nil {
		return domain.BudgetPolicy{}, err
	}
	created := record
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "project_id"}}, DoNothing: true,
	}).Create(&created).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.BudgetPolicy{}, err
	}
	var persisted model.CostBudgetPolicy
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("project_id = ?", record.ProjectID).First(&persisted).Error; err != nil {
		return domain.BudgetPolicy{}, fmt.Errorf("load ensured cost budget policy: %w", err)
	}
	return budgetDomain(persisted), nil
}

func (repo *repository) UpdateBudget(
	ctx context.Context,
	desired domain.BudgetPolicy,
	expectedRevision int64,
) (domain.BudgetPolicy, error) {
	record, err := budgetRecord(desired)
	if err != nil {
		return domain.BudgetPolicy{}, err
	}
	updated := repo.database.WithContext(ctx).Model(&model.CostBudgetPolicy{}).
		Where("id = ? AND revision = ?", record.ID, expectedRevision).
		Updates(map[string]any{
			"limit_amount": record.LimitAmount, "currency": record.Currency, "revision": record.Revision,
			"content_hash": record.ContentHash, "updated_by": record.UpdatedBy, "updated_at": record.UpdatedAt,
		})
	if updated.Error != nil {
		return domain.BudgetPolicy{}, updated.Error
	}
	if updated.RowsAffected != 1 {
		return domain.BudgetPolicy{}, conflict("Project budget revision has changed")
	}
	return desired, nil
}

func budgetRecord(value domain.BudgetPolicy) (model.CostBudgetPolicy, error) {
	id, workspace, project, creator, updater, err := parseBudgetIDs(value)
	if err != nil {
		return model.CostBudgetPolicy{}, err
	}
	return model.CostBudgetPolicy{
		ID: id, WorkspaceID: workspace, ProjectID: project, LimitAmount: value.LimitAmount,
		Currency: value.Currency, Revision: value.Revision, ContentHash: value.ContentHash,
		CreatedBy: creator, UpdatedBy: updater, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func budgetDomain(value model.CostBudgetPolicy) domain.BudgetPolicy {
	return domain.BudgetPolicy{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		LimitAmount: value.LimitAmount, Currency: value.Currency, Revision: value.Revision,
		ContentHash: value.ContentHash, CreatedBy: value.CreatedBy.String(), UpdatedBy: value.UpdatedBy.String(),
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}
}

func parseBudgetIDs(value domain.BudgetPolicy) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, error) {
	values := []string{value.ID, value.WorkspaceID, value.ProjectID, value.CreatedBy, value.UpdatedBy}
	parsed := make([]uuid.UUID, len(values))
	for index, raw := range values {
		id, err := uuid.Parse(raw)
		if err != nil {
			return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, err
		}
		parsed[index] = id
	}
	return parsed[0], parsed[1], parsed[2], parsed[3], parsed[4], nil
}

func normalizeBudgetNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrBudgetNotFound
	}
	return err
}

func normalizeProjectNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound("Project not found")
	}
	return err
}

func unauthenticated() error {
	return &application.Error{Code: "unauthenticated", Message: "Invalid credentials", Status: 401, NextAction: "login"}
}

func forbidden() error {
	return &application.Error{Code: "forbidden", Message: "Action is not allowed", Status: 403}
}

func notFound(message string) error {
	return &application.Error{Code: "not_found", Message: message, Status: 404}
}

func conflict(message string) error {
	return &application.Error{Code: "state_conflict", Message: message, Status: 409}
}

var _ application.TransactionManager = (*Store)(nil)
var _ application.Repository = (*repository)(nil)
