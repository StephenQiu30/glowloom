package gormdb

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
)

func (store *Store) WithinEpisodeLifecycleTransaction(
	ctx context.Context,
	operation func(application.EpisodeLifecycleRepository) error,
) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&repository{database: transaction})
	})
}

func (repo *repository) GetEpisodeLifecycleProject(
	ctx context.Context,
	projectID string,
	forUpdate bool,
) (domain.Project, error) {
	return repo.Get(ctx, projectID, forUpdate)
}

func (repo *repository) GetEpisodeLifecycleSource(
	ctx context.Context,
	versionID string,
) (domain.EpisodeLifecycleSource, error) {
	id, err := uuid.Parse(versionID)
	if err != nil {
		return domain.EpisodeLifecycleSource{}, application.ErrNotFound
	}
	var revision model.DocumentRevision
	if err = repo.database.WithContext(ctx).First(&revision, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.EpisodeLifecycleSource{}, application.ErrNotFound
		}
		return domain.EpisodeLifecycleSource{}, err
	}
	var document model.ScriptDocument
	if err = repo.database.WithContext(ctx).First(&document, "id = ?", revision.DocumentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.EpisodeLifecycleSource{}, application.ErrNotFound
		}
		return domain.EpisodeLifecycleSource{}, err
	}
	var head model.ScriptSourceScopeHead
	if err = repo.database.WithContext(ctx).First(&head, "project_id = ?", document.ProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.EpisodeLifecycleSource{}, application.ErrNotFound
		}
		return domain.EpisodeLifecycleSource{}, err
	}
	if head.WorkspaceID != revision.WorkspaceID || head.DocumentLogicalID != document.ID ||
		head.CurrentDocumentRevisionID != revision.ID {
		return domain.EpisodeLifecycleSource{}, errors.New("Project Episode lifecycle Script Source Head has drifted")
	}
	return domain.EpisodeLifecycleSource{
		WorkspaceID: revision.WorkspaceID.String(), ProjectID: document.ProjectID.String(),
		DocumentID: document.ID.String(), VersionID: revision.ID.String(), Revision: int64(revision.VersionNo),
		HeadRevision: head.HeadRevision, HeadHash: head.HeadHash,
		ContentHash: revision.NormalizedHash, NormalizedText: revision.NormalizedText, CreatedAt: revision.CreatedAt.UTC(),
	}, nil
}

func (repo *repository) ListEpisodeLifecycleEpisodes(
	ctx context.Context,
	projectID string,
) ([]domain.EpisodeLifecycleEpisode, error) {
	id, err := uuid.Parse(projectID)
	if err != nil {
		return nil, application.ErrNotFound
	}
	var records []model.Episode
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("project_id = ?", id).Order("position").Order("id").Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]domain.EpisodeLifecycleEpisode, len(records))
	for index, record := range records {
		var current *string
		if record.CurrentScriptVersionID != nil {
			value := record.CurrentScriptVersionID.String()
			current = &value
		}
		result[index] = domain.EpisodeLifecycleEpisode{
			ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
			Name: record.Name, Position: record.Position, TargetDurationMS: record.TargetDurationMS,
			Status: record.Status, Revision: record.Revision, CurrentScriptVersionID: current,
			CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		}
	}
	return result, nil
}

func (repo *repository) FindEpisodeLifecycleScriptVersion(
	ctx context.Context,
	episodeID, documentRevisionID string,
	sourceStart, sourceEnd int,
	contentHash string,
) (domain.EpisodeLifecycleScriptVersion, bool, error) {
	episode, err := uuid.Parse(episodeID)
	if err != nil {
		return domain.EpisodeLifecycleScriptVersion{}, false, application.ErrNotFound
	}
	revision, err := uuid.Parse(documentRevisionID)
	if err != nil {
		return domain.EpisodeLifecycleScriptVersion{}, false, application.ErrNotFound
	}
	var record model.EpisodeScriptVersion
	err = repo.database.WithContext(ctx).Where(
		"episode_id = ? AND document_revision_id = ? AND source_start = ? AND source_end = ? AND content_hash = ? AND status = ?",
		episode, revision, sourceStart, sourceEnd, contentHash, "published",
	).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.EpisodeLifecycleScriptVersion{}, false, nil
	}
	if err != nil {
		return domain.EpisodeLifecycleScriptVersion{}, false, err
	}
	return episodeLifecycleScriptVersionDomain(record), true, nil
}

func (repo *repository) NextEpisodeLifecycleScriptVersion(ctx context.Context, episodeID string) (int, error) {
	id, err := uuid.Parse(episodeID)
	if err != nil {
		return 0, application.ErrNotFound
	}
	var latest model.EpisodeScriptVersion
	err = repo.database.WithContext(ctx).Where("episode_id = ?", id).Order("version_no DESC").First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	return latest.VersionNo + 1, nil
}

func (repo *repository) CreateEpisodeLifecycleEpisode(
	ctx context.Context,
	value domain.EpisodeLifecycleEpisode,
) error {
	record, err := episodeLifecycleEpisodeRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) SaveEpisodeLifecycleEpisode(
	ctx context.Context,
	value domain.EpisodeLifecycleEpisode,
) error {
	record, err := episodeLifecycleEpisodeRecord(value)
	if err != nil {
		return err
	}
	result := repo.database.WithContext(ctx).Model(&model.Episode{}).Where("id = ?", record.ID).Updates(map[string]any{
		"name": record.Name, "position": record.Position, "target_duration_ms": record.TargetDurationMS,
		"status": record.Status, "revision": record.Revision,
		"current_script_version_id": record.CurrentScriptVersionID, "updated_at": record.UpdatedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return application.ErrNotFound
	}
	return nil
}

func (repo *repository) CreateEpisodeLifecycleScriptVersion(
	ctx context.Context,
	value domain.EpisodeLifecycleScriptVersion,
) error {
	record, err := episodeLifecycleScriptVersionRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) SaveEpisodeLifecycleProject(ctx context.Context, value domain.Project) error {
	return repo.Save(ctx, value)
}

func episodeLifecycleEpisodeRecord(value domain.EpisodeLifecycleEpisode) (model.Episode, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.Episode{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.Episode{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.Episode{}, err
	}
	var current *uuid.UUID
	if value.CurrentScriptVersionID != nil {
		parsed, parseErr := uuid.Parse(*value.CurrentScriptVersionID)
		if parseErr != nil {
			return model.Episode{}, parseErr
		}
		current = &parsed
	}
	return model.Episode{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, Name: value.Name,
		Position: value.Position, TargetDurationMS: value.TargetDurationMS,
		Status: value.Status, Revision: value.Revision, CurrentScriptVersionID: current,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func episodeLifecycleScriptVersionRecord(
	value domain.EpisodeLifecycleScriptVersion,
) (model.EpisodeScriptVersion, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.EpisodeScriptVersion{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.EpisodeScriptVersion{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.EpisodeScriptVersion{}, err
	}
	episodeID, err := uuid.Parse(value.EpisodeID)
	if err != nil {
		return model.EpisodeScriptVersion{}, err
	}
	revisionID, err := uuid.Parse(value.DocumentRevisionID)
	if err != nil {
		return model.EpisodeScriptVersion{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.EpisodeScriptVersion{}, err
	}
	return model.EpisodeScriptVersion{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID,
		VersionNo: value.VersionNo, DocumentRevisionID: revisionID,
		SourceStart: value.SourceStart, SourceEnd: value.SourceEnd,
		Content: value.Content, ContentHash: value.ContentHash, Status: value.Status,
		CreatedBy: createdBy, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func episodeLifecycleScriptVersionDomain(
	record model.EpisodeScriptVersion,
) domain.EpisodeLifecycleScriptVersion {
	return domain.EpisodeLifecycleScriptVersion{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		EpisodeID: record.EpisodeID.String(), DocumentRevisionID: record.DocumentRevisionID.String(),
		VersionNo: record.VersionNo, SourceStart: record.SourceStart, SourceEnd: record.SourceEnd,
		Content: record.Content, ContentHash: record.ContentHash, Status: record.Status,
		CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

var _ application.EpisodeLifecycleTransactionManager = (*Store)(nil)
var _ application.EpisodeLifecycleRepository = (*repository)(nil)
