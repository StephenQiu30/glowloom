package gormdb

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

func (store *Store) WithinSelectionTransaction(
	ctx context.Context,
	operation func(application.SelectionRepository) error,
) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&repository{database: transaction})
	})
}

func (repo *repository) EnsureSelection(
	ctx context.Context,
	desired domain.CandidateSelection,
) (domain.CandidateSelection, error) {
	record, err := selectionRecord(desired)
	if err != nil {
		return domain.CandidateSelection{}, err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).
		Clauses(clause.OnConflict{DoNothing: true}).Create(&record).Error; err != nil {
		return domain.CandidateSelection{}, err
	}
	var persisted model.GenerationCandidateSelection
	err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("human_task_id = ?", record.HumanTaskID).First(&persisted).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("review_decision_id = ?", record.ReviewDecisionID).First(&persisted).Error
	}
	if err != nil {
		return domain.CandidateSelection{}, err
	}
	result, err := selectionDomain(persisted)
	if err != nil {
		return domain.CandidateSelection{}, err
	}
	if !domain.SameSelectionBinding(result, desired) {
		return domain.CandidateSelection{}, platformcommand.ErrInputMismatch
	}
	return result, nil
}

func (repo *repository) GetSelection(ctx context.Context, selectionID string) (domain.CandidateSelection, error) {
	parsed, err := uuid.Parse(selectionID)
	if err != nil {
		return domain.CandidateSelection{}, application.ErrSelectionNotFound
	}
	var record model.GenerationCandidateSelection
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", parsed).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.CandidateSelection{}, application.ErrSelectionNotFound
		}
		return domain.CandidateSelection{}, err
	}
	return selectionDomain(record)
}

func selectionRecord(value domain.CandidateSelection) (model.GenerationCandidateSelection, error) {
	identifiers := []string{
		value.ID, value.WorkspaceID, value.ProjectID, value.WorkflowRunID, value.NodeRunID,
		value.HumanTaskID, value.ReviewDecisionID, value.SubjectID, value.SelectedCandidateID,
		value.SelectedArtifactID, value.ReviewerID, value.CreatedBy,
	}
	parsed := make([]uuid.UUID, len(identifiers))
	for index, identifier := range identifiers {
		id, err := uuid.Parse(identifier)
		if err != nil {
			return model.GenerationCandidateSelection{}, err
		}
		parsed[index] = id
	}
	candidates, err := json.Marshal(value.Candidates)
	if err != nil {
		return model.GenerationCandidateSelection{}, err
	}
	return model.GenerationCandidateSelection{
		ID: parsed[0], WorkspaceID: parsed[1], ProjectID: parsed[2], WorkflowRunID: parsed[3], NodeRunID: parsed[4],
		HumanTaskID: parsed[5], ReviewDecisionID: parsed[6], SubjectType: value.SubjectType, SubjectID: parsed[7],
		SubjectRevision: value.SubjectRevision, Candidates: datatypes.JSON(candidates), CandidateSetHash: value.CandidateSetHash,
		SelectedCandidateID: parsed[8], SelectedArtifactID: parsed[9], SelectedArtifactSHA256: value.SelectedArtifactSHA256,
		ReviewerID: parsed[10], ContentHash: value.ContentHash, Revision: value.Revision,
		CreatedBy: parsed[11], CreatedAt: value.CreatedAt,
	}, nil
}

func selectionDomain(value model.GenerationCandidateSelection) (domain.CandidateSelection, error) {
	var candidates []domain.CandidateReference
	if err := json.Unmarshal(value.Candidates, &candidates); err != nil {
		return domain.CandidateSelection{}, err
	}
	return domain.CandidateSelection{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		WorkflowRunID: value.WorkflowRunID.String(), NodeRunID: value.NodeRunID.String(),
		HumanTaskID: value.HumanTaskID.String(), ReviewDecisionID: value.ReviewDecisionID.String(),
		SubjectType: value.SubjectType, SubjectID: value.SubjectID.String(), SubjectRevision: value.SubjectRevision,
		Candidates: candidates, CandidateSetHash: value.CandidateSetHash,
		SelectedCandidateID: value.SelectedCandidateID.String(), SelectedArtifactID: value.SelectedArtifactID.String(),
		SelectedArtifactSHA256: value.SelectedArtifactSHA256, ReviewerID: value.ReviewerID.String(),
		ContentHash: value.ContentHash, Revision: value.Revision,
		CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt,
	}, nil
}

var _ application.SelectionTransactionManager = (*Store)(nil)
var _ application.SelectionRepository = (*repository)(nil)
