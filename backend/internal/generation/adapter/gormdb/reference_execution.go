package gormdb

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	commandgorm "github.com/StephenQiu30/lanverse/backend/internal/platform/command/adapter/gormdb"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

type referenceExecutionRepository struct {
	referenceExecutionAuthorizationRepository
	database *gorm.DB
}

func (store *Store) WithinReferenceExecution(ctx context.Context, operation func(application.ReferenceExecutionRepository) error) error {
	if store == nil || store.database == nil || operation == nil {
		return errors.New("Reference execution store is unavailable")
	}
	return platformdatabase.WithinSerializableTransaction(ctx, store.database, func(tx *gorm.DB) error {
		targets := &referenceTargetRepository{referenceAuthorizationRepository{repository{database: tx}}}
		return operation(&referenceExecutionRepository{referenceExecutionAuthorizationRepository{targets, &providerConfigurationRepository{database: tx}}, tx})
	})
}

func (repo *referenceExecutionRepository) PublishInitialReferenceExecution(ctx context.Context, value domain.ReferenceExecution) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err = domain.DecodeReferenceExecution(raw); err != nil {
		return err
	}
	ids := make([]uuid.UUID, 5)
	for i, text := range []string{value.ID, value.WorkspaceID, value.ProjectID, value.ReadSet.TargetRef.ID, value.CreatedBy} {
		ids[i], err = uuid.Parse(text)
		if err != nil {
			return err
		}
	}
	record := model.GenerationReferenceExecution{ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], TargetID: ids[3], TargetHash: value.ReadSet.TargetRef.ContentHash, Revision: value.Revision, ContentHash: value.ContentHash, Content: raw, CreatedBy: ids[4], CreatedAt: value.CreatedAt}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		return err
	}
	head := model.GenerationReferenceExecutionHead{WorkspaceID: record.WorkspaceID, ProjectID: record.ProjectID, TargetID: record.TargetID, TargetHash: record.TargetHash, CurrentExecutionID: record.ID, CurrentExecutionHash: record.ContentHash, Revision: 1}
	created := repo.database.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&head)
	if created.Error != nil {
		return created.Error
	}
	if created.RowsAffected != 1 {
		return errors.New("Reference execution Head conflicts with expected initial revision")
	}
	return nil
}

func (repo *referenceExecutionRepository) FindReferenceExecution(ctx context.Context, workspace, project, id string) (domain.ReferenceExecution, error) {
	var record model.GenerationReferenceExecution
	if err := repo.database.WithContext(ctx).Where("id = ? AND workspace_id = ? AND project_id = ?", id, workspace, project).First(&record).Error; err != nil {
		return domain.ReferenceExecution{}, err
	}
	value, err := domain.DecodeReferenceExecution(json.RawMessage(record.Content))
	if err != nil {
		return domain.ReferenceExecution{}, err
	}
	if value.ID != record.ID.String() || value.WorkspaceID != record.WorkspaceID.String() || value.ProjectID != record.ProjectID.String() || value.ReadSet.TargetRef.ID != record.TargetID.String() || value.ReadSet.TargetRef.ContentHash != record.TargetHash || value.Revision != record.Revision || value.ContentHash != record.ContentHash || value.CreatedBy != record.CreatedBy.String() || !value.CreatedAt.Equal(record.CreatedAt) {
		return domain.ReferenceExecution{}, errors.New("persisted Reference execution identity has drifted")
	}
	return value, nil
}

func (repo *referenceExecutionRepository) FindReferenceExecutionReceipt(ctx context.Context, workspace, id string) (platformcommand.Receipt, error) {
	var records []model.CommandReceipt
	if err := repo.database.WithContext(ctx).Select("id").Where("workspace_id = ? AND operation = ? AND resource_id = ?", workspace, application.PrepareInitialReferenceExecutionOperation, id).Limit(2).Find(&records).Error; err != nil {
		return platformcommand.Receipt{}, err
	}
	if len(records) != 1 {
		return platformcommand.Receipt{}, errors.New("Reference execution publication is missing or ambiguous")
	}
	return commandgorm.FindByID(ctx, repo.database, records[0].ID.String())
}

func (repo *referenceExecutionRepository) ValidateReferenceExecutionHead(ctx context.Context, value domain.ReferenceExecution) error {
	var head model.GenerationReferenceExecutionHead
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Where("workspace_id = ? AND project_id = ? AND target_id = ?", value.WorkspaceID, value.ProjectID, value.ReadSet.TargetRef.ID).First(&head).Error; err != nil {
		return err
	}
	if head.TargetHash != value.ReadSet.TargetRef.ContentHash || head.CurrentExecutionID.String() != value.ID || head.CurrentExecutionHash != value.ContentHash || head.Revision != value.Revision {
		return errors.New("Reference execution Head has drifted")
	}
	return nil
}

var _ application.ReferenceExecutionTransactions = (*Store)(nil)
