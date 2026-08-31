package gormdb

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/script/domain"
)

func (store *Store) WithinSourceTransaction(ctx context.Context, operation func(application.SourceRepository) error) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&repository{database: transaction})
	})
}

func (repo *repository) LockSourceHead(ctx context.Context, projectID string) (domain.SourceHead, error) {
	parsedProjectID, err := uuid.Parse(projectID)
	if err != nil {
		return domain.SourceHead{}, application.ErrNotFound
	}
	var record model.ScriptSourceScopeHead
	err = repo.database.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&record, "project_id = ?", parsedProjectID).Error
	if err != nil {
		return domain.SourceHead{}, normalizeNotFound(err)
	}
	return sourceHeadDomain(record), nil
}

func (repo *repository) FindSpanIndex(ctx context.Context, documentRevisionID string) (domain.SourceSpanIndex, error) {
	revisionID, err := uuid.Parse(documentRevisionID)
	if err != nil {
		return domain.SourceSpanIndex{}, application.ErrNotFound
	}
	var record model.SourceSpanIndexVersion
	if err = repo.database.WithContext(ctx).First(&record, "document_revision_id = ?", revisionID).Error; err != nil {
		return domain.SourceSpanIndex{}, normalizeNotFound(err)
	}
	return sourceSpanIndexDomain(record), nil
}

func (repo *repository) CreateSpanIndex(ctx context.Context, value domain.SourceSpanIndex) error {
	record, err := sourceSpanIndexRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) CreateSourceHead(ctx context.Context, value domain.SourceHead, now time.Time) error {
	record, err := sourceHeadRecord(value, now)
	if err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return &application.Error{Code: "head_conflict", Message: "Script Source Head changed", Status: 409}
		}
		return err
	}
	return nil
}

func (repo *repository) AdvanceSourceHead(ctx context.Context, value domain.SourceHead, expectedRevision int64, expectedHash string, now time.Time) error {
	record, err := sourceHeadRecord(value, now)
	if err != nil {
		return err
	}
	result := repo.database.WithContext(ctx).Model(&model.ScriptSourceScopeHead{}).
		Where("project_id = ? AND head_revision = ? AND head_hash = ?", record.ProjectID, expectedRevision, expectedHash).
		Updates(map[string]any{
			"document_logical_id": value.DocumentLogicalID, "current_document_revision_id": record.CurrentDocumentRevisionID,
			"current_span_index_id": record.CurrentSpanIndexID, "head_revision": value.HeadRevision,
			"head_hash": value.HeadHash, "updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return &application.Error{Code: "head_conflict", Message: "Script Source Head changed", Status: 409}
	}
	return nil
}

func (repo *repository) CreateSourceCollectionReceipt(ctx context.Context, value domain.SourceCollectionReceipt) error {
	record, err := sourceCollectionReceiptRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) GetAcceptedSource(ctx context.Context, projectID, revisionID string) (domain.AcceptedSource, error) {
	parsedProjectID, err := uuid.Parse(projectID)
	if err != nil {
		return domain.AcceptedSource{}, application.ErrNotFound
	}
	parsedRevisionID, err := uuid.Parse(revisionID)
	if err != nil {
		return domain.AcceptedSource{}, application.ErrNotFound
	}
	var collection model.ScriptSourceCollectionReceipt
	if err = repo.database.WithContext(ctx).
		Where("project_id = ? AND document_revision_id = ?", parsedProjectID, parsedRevisionID).
		Order("created_at DESC").First(&collection).Error; err != nil {
		return domain.AcceptedSource{}, normalizeNotFound(err)
	}
	var index model.SourceSpanIndexVersion
	if err = repo.database.WithContext(ctx).First(&index, "id = ?", collection.SpanIndexID).Error; err != nil {
		return domain.AcceptedSource{}, normalizeNotFound(err)
	}
	var revision model.DocumentRevision
	if err = repo.database.WithContext(ctx).First(&revision, "id = ?", parsedRevisionID).Error; err != nil {
		return domain.AcceptedSource{}, normalizeNotFound(err)
	}
	var commandReceipt model.CommandReceipt
	if err = repo.database.WithContext(ctx).
		Where("workspace_id = ? AND operation = ? AND idempotency_key = ?", collection.WorkspaceID, "script_source.accept", collection.SourceAcceptanceRef).
		First(&commandReceipt).Error; err != nil {
		return domain.AcceptedSource{}, normalizeNotFound(err)
	}
	return domain.AcceptedSource{
		Identity: domain.SourceVersionIdentity{
			OwnerKind: "production/script", LogicalID: revision.DocumentID.String(), VersionID: revision.ID.String(),
			Revision: int64(revision.VersionNo), ContentHash: revision.NormalizedHash, CreatedAt: revision.CreatedAt.UTC(),
		},
		SpanIndexID: index.ID.String(), SpanIndexHash: index.ContentHash, CodepointCount: index.CodepointCount,
		UTF8ByteCount: index.UTF8ByteCount, NewlineNormalization: index.NewlineNormalization,
		CodepointIndexRule: index.CodepointIndexRule, HeadRevision: collection.HeadRevision, HeadHash: collection.HeadHash,
		CollectionRootHash: collection.CollectionRootHash, CollectionReceiptID: collection.ID.String(),
		CommandReceiptID: commandReceipt.ID.String(),
	}, nil
}

func sourceSpanIndexRecord(value domain.SourceSpanIndex) (model.SourceSpanIndexVersion, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.SourceSpanIndexVersion{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.SourceSpanIndexVersion{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.SourceSpanIndexVersion{}, err
	}
	revisionID, err := uuid.Parse(value.DocumentRevisionID)
	if err != nil {
		return model.SourceSpanIndexVersion{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.SourceSpanIndexVersion{}, err
	}
	return model.SourceSpanIndexVersion{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, DocumentRevisionID: revisionID,
		SourceHash: value.SourceHash, NewlineNormalization: value.NewlineNormalization,
		CodepointIndexRule: value.CodepointIndexRule, CodepointCount: value.CodepointCount,
		UTF8ByteCount: value.UTF8ByteCount, IndexManifest: datatypes.JSON(value.IndexManifest),
		ContentHash: value.ContentHash, CreatedBy: createdBy, CreatedAt: value.CreatedAt,
	}, nil
}

func sourceSpanIndexDomain(value model.SourceSpanIndexVersion) domain.SourceSpanIndex {
	return domain.SourceSpanIndex{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		DocumentRevisionID: value.DocumentRevisionID.String(), SourceHash: value.SourceHash,
		NewlineNormalization: value.NewlineNormalization, CodepointIndexRule: value.CodepointIndexRule,
		CodepointCount: value.CodepointCount, UTF8ByteCount: value.UTF8ByteCount,
		IndexManifest: append([]byte(nil), value.IndexManifest...), ContentHash: value.ContentHash,
		CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt,
	}
}

func sourceHeadRecord(value domain.SourceHead, now time.Time) (model.ScriptSourceScopeHead, error) {
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.ScriptSourceScopeHead{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.ScriptSourceScopeHead{}, err
	}
	documentID, err := uuid.Parse(value.DocumentLogicalID)
	if err != nil {
		return model.ScriptSourceScopeHead{}, err
	}
	revisionID, err := uuid.Parse(value.DocumentRevisionID)
	if err != nil {
		return model.ScriptSourceScopeHead{}, err
	}
	indexID, err := uuid.Parse(value.SpanIndexID)
	if err != nil {
		return model.ScriptSourceScopeHead{}, err
	}
	return model.ScriptSourceScopeHead{
		ProjectID: projectID, WorkspaceID: workspaceID, DocumentLogicalID: documentID,
		CurrentDocumentRevisionID: revisionID, CurrentSpanIndexID: indexID,
		HeadRevision: value.HeadRevision, HeadHash: value.HeadHash, UpdatedAt: now,
	}, nil
}

func sourceHeadDomain(value model.ScriptSourceScopeHead) domain.SourceHead {
	return domain.SourceHead{
		Exists: true, ProjectID: value.ProjectID.String(), WorkspaceID: value.WorkspaceID.String(),
		DocumentLogicalID: value.DocumentLogicalID.String(), DocumentRevisionID: value.CurrentDocumentRevisionID.String(),
		SpanIndexID: value.CurrentSpanIndexID.String(), HeadRevision: value.HeadRevision, HeadHash: value.HeadHash,
	}
}

func sourceCollectionReceiptRecord(value domain.SourceCollectionReceipt) (model.ScriptSourceCollectionReceipt, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.ScriptSourceCollectionReceipt{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.ScriptSourceCollectionReceipt{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.ScriptSourceCollectionReceipt{}, err
	}
	revisionID, err := uuid.Parse(value.DocumentRevisionID)
	if err != nil {
		return model.ScriptSourceCollectionReceipt{}, err
	}
	indexID, err := uuid.Parse(value.SpanIndexID)
	if err != nil {
		return model.ScriptSourceCollectionReceipt{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.ScriptSourceCollectionReceipt{}, err
	}
	return model.ScriptSourceCollectionReceipt{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, DocumentRevisionID: revisionID,
		SpanIndexID: indexID, HeadRevision: value.HeadRevision, HeadHash: value.HeadHash,
		Members: datatypes.JSON(value.Members), MembersHash: value.MembersHash,
		CollectionRootHash: value.CollectionRootHash, SourceAcceptanceRef: value.SourceAcceptanceRef,
		ReceiptContentHash: value.ReceiptContentHash, CreatedBy: createdBy, CreatedAt: value.CreatedAt,
	}, nil
}
