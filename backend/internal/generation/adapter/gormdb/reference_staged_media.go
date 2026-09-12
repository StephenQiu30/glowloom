package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func (store *Store) WithinReferenceStagedMedia(ctx context.Context, operation func(application.ReferenceStagedMediaRepository) error) error {
	if store == nil || store.database == nil || store.database.Config == nil || store.database.Statement == nil || operation == nil {
		return errors.New("staged media requires standalone transactions")
	}
	if _, nested := store.database.Statement.ConnPool.(gorm.TxCommitter); nested {
		return errors.New("staged media cannot use an outer transaction")
	}
	return platformdatabase.WithinSerializableTransaction(ctx, store.database, func(tx *gorm.DB) error {
		targets := &referenceTargetRepository{referenceAuthorizationRepository{repository{database: tx}}}
		return operation(&referenceExecutionRepository{referenceExecutionAuthorizationRepository{targets, &providerConfigurationRepository{database: tx}}, tx})
	})
}

func (repo *referenceExecutionRepository) FindReferenceStagedMedia(ctx context.Context, workspace, project, key string) (domain.ReferenceStagedMedia, error) {
	var record model.GenerationReferenceStagedMedia
	if err := repo.database.WithContext(ctx).Where("workspace_id = ? AND project_id = ? AND call_key = ?", workspace, project, key).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ReferenceStagedMedia{}, application.ErrReferenceStagedMediaNotFound
		}
		return domain.ReferenceStagedMedia{}, err
	}
	v, err := domain.DecodeReferenceStagedMedia(json.RawMessage(record.Content))
	if err != nil {
		return domain.ReferenceStagedMedia{}, err
	}
	if v.ID != record.ID.String() || v.WorkspaceID != record.WorkspaceID.String() || v.ProjectID != record.ProjectID.String() || v.Call.CallKey != record.CallKey || v.ReceiptRef.ContentHash != record.ReceiptHash || v.State != record.State || v.Revision != record.Revision || v.ContentHash != record.ContentHash {
		return domain.ReferenceStagedMedia{}, errors.New("staged media persisted metadata has drifted")
	}
	return v, nil
}

func (repo *referenceExecutionRepository) InsertReferenceStagedMedia(ctx context.Context, value domain.ReferenceStagedMedia) error {
	initial, err := domain.InitialReferenceStagedMedia(value)
	if err != nil || !reflect.DeepEqual(value, initial) {
		return errors.New("only quarantined media can be registered")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Create(&model.GenerationReferenceStagedMedia{ID: uuid.MustParse(value.ID), WorkspaceID: uuid.MustParse(value.WorkspaceID), ProjectID: uuid.MustParse(value.ProjectID), CallKey: value.Call.CallKey, ReceiptHash: value.ReceiptRef.ContentHash, State: value.State, Revision: value.Revision, ContentHash: value.ContentHash, Content: datatypes.JSON(raw)}).Error
}

func (repo *referenceExecutionRepository) UpdateReferenceStagedMedia(ctx context.Context, before, after domain.ReferenceStagedMedia) error {
	if err := domain.ValidateReferenceStagedMediaTransition(before, after); err != nil {
		return err
	}
	raw, err := json.Marshal(after)
	if err != nil {
		return err
	}
	updated := repo.database.WithContext(ctx).Model(&model.GenerationReferenceStagedMedia{}).
		Where("id = ? AND workspace_id = ? AND project_id = ? AND call_key = ? AND revision = ? AND content_hash = ?", before.ID, before.WorkspaceID, before.ProjectID, before.Call.CallKey, before.Revision, before.ContentHash).
		UpdateColumns(map[string]any{"state": after.State, "revision": after.Revision, "content_hash": after.ContentHash, "content": datatypes.JSON(raw)})
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected != 1 {
		return application.ErrReferenceStagedMediaConflict
	}
	return nil
}

var _ application.ReferenceStagedMediaTransactions = (*Store)(nil)
