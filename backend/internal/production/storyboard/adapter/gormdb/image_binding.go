package gormdb

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	storyboarddomain "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
)

func (store *Store) WithinShotImageBindingTransaction(
	ctx context.Context,
	operation func(application.ShotImageBindingRepository) error,
) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&repository{database: transaction})
	})
}

func (repo *repository) LockShotImageTarget(
	ctx context.Context,
	actor application.Actor,
	shotID string,
	write bool,
) (storyboarddomain.Shot, error) {
	id, err := uuid.Parse(shotID)
	if err != nil {
		return storyboarddomain.Shot{}, application.ErrNotFound
	}
	var record model.StoryboardShot
	query := repo.database.WithContext(ctx)
	if write {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err = query.First(&record, "id = ?", id).Error; err != nil {
		return storyboarddomain.Shot{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, record.ProjectID, write); err != nil {
		return storyboarddomain.Shot{}, err
	}
	return shotDomain(record)
}

func (repo *repository) FindCurrentShotImageBinding(
	ctx context.Context,
	shotID string,
) (storyboarddomain.ShotImageBindingVersion, error) {
	id, err := uuid.Parse(shotID)
	if err != nil {
		return storyboarddomain.ShotImageBindingVersion{}, application.ErrNotFound
	}
	var record model.StoryboardShotImageBindingVersion
	if err = repo.database.WithContext(ctx).Where("shot_id = ?", id).
		Order("binding_revision DESC").First(&record).Error; err != nil {
		return storyboarddomain.ShotImageBindingVersion{}, normalizeNotFound(err)
	}
	return shotImageBindingDomain(record), nil
}

func (repo *repository) GetShotImageBinding(
	ctx context.Context,
	bindingID string,
) (storyboarddomain.ShotImageBindingVersion, error) {
	id, err := uuid.Parse(bindingID)
	if err != nil {
		return storyboarddomain.ShotImageBindingVersion{}, application.ErrNotFound
	}
	var record model.StoryboardShotImageBindingVersion
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return storyboarddomain.ShotImageBindingVersion{}, normalizeNotFound(err)
	}
	return shotImageBindingDomain(record), nil
}

func (repo *repository) CreateShotImageBinding(
	ctx context.Context,
	value storyboarddomain.ShotImageBindingVersion,
) error {
	record, err := shotImageBindingRecord(value)
	if err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return conflict("Shot image binding version already exists")
		}
		return err
	}
	return nil
}

func shotImageBindingRecord(
	value storyboarddomain.ShotImageBindingVersion,
) (model.StoryboardShotImageBindingVersion, error) {
	identifiers := []string{
		value.ID, value.WorkspaceID, value.ProjectID, value.EpisodeID, value.ShotID,
		value.CandidateSelectionID, value.CandidateID, value.ArtifactID, value.CreatedBy,
	}
	parsed := make([]uuid.UUID, len(identifiers))
	for index, identifier := range identifiers {
		id, err := uuid.Parse(identifier)
		if err != nil {
			return model.StoryboardShotImageBindingVersion{}, err
		}
		parsed[index] = id
	}
	return model.StoryboardShotImageBindingVersion{
		ID: parsed[0], WorkspaceID: parsed[1], ProjectID: parsed[2], EpisodeID: parsed[3], ShotID: parsed[4],
		ShotRevision: value.ShotRevision, ShotContentHash: value.ShotContentHash,
		CandidateSelectionID: parsed[5], CandidateSelectionRevision: value.CandidateSelectionRevision,
		CandidateSelectionContentHash: value.CandidateSelectionContentHash,
		CandidateID:                   parsed[6], CandidateRevision: value.CandidateRevision,
		ArtifactID: parsed[7], ArtifactRevision: value.ArtifactRevision, ArtifactSHA256: value.ArtifactSHA256,
		BindingRevision: value.Revision, ContentHash: value.ContentHash,
		CreatedBy: parsed[8], CreatedAt: value.CreatedAt,
	}, nil
}

func shotImageBindingDomain(record model.StoryboardShotImageBindingVersion) storyboarddomain.ShotImageBindingVersion {
	return storyboarddomain.ShotImageBindingVersion{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		EpisodeID: record.EpisodeID.String(), ShotID: record.ShotID.String(),
		ShotRevision: record.ShotRevision, ShotContentHash: record.ShotContentHash,
		CandidateSelectionID:          record.CandidateSelectionID.String(),
		CandidateSelectionRevision:    record.CandidateSelectionRevision,
		CandidateSelectionContentHash: record.CandidateSelectionContentHash,
		CandidateID:                   record.CandidateID.String(), CandidateRevision: record.CandidateRevision,
		ArtifactID: record.ArtifactID.String(), ArtifactRevision: record.ArtifactRevision,
		ArtifactSHA256: record.ArtifactSHA256, Revision: record.BindingRevision, ContentHash: record.ContentHash,
		CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt,
	}
}

var _ application.ShotImageBindingTransactionManager = (*Store)(nil)
