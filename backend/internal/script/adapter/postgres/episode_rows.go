package postgres

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

func readEpisode(tx *gorm.DB, org, id uuid.UUID, lock bool) (domain.Episode, error) {
	query := tx.Where("org_id = ? AND id = ? AND is_delete = ?", org, id, false)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row scriptEpisodeRow
	if err := query.Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Episode{}, application.ErrNotFound
		}
		return domain.Episode{}, fmt.Errorf("read formal episode: %w", err)
	}
	return row.episode(), nil
}
func readEpisodes(tx *gorm.DB, org, project, version uuid.UUID, lock bool) ([]domain.Episode, error) {
	query := tx.Where("org_id = ? AND project_id = ? AND script_version_id = ? AND is_delete = ?", org, project, version, false).
		Order("seq_no").Order("id")
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var rows []scriptEpisodeRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.Episode, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.episode())
	}
	return result, nil
}
func readVersionHead(tx *gorm.DB, org, project, version uuid.UUID, lock bool) (application.VersionHead, error) {
	query := tx.Where("org_id = ? AND project_id = ? AND version_id = ?", org, project, version)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row scriptVersionHeadRow
	if err := query.Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return application.VersionHead{}, application.ErrNotFound
		}
		return application.VersionHead{}, err
	}
	return application.VersionHead{VersionID: row.VersionID, SplitRevision: row.SplitRevision, CandidateSetID: row.CandidateSplitSetID, ConfirmedSetID: row.ConfirmedSplitSetID}, nil
}
func readSplitSet(tx *gorm.DB, org, project, id uuid.UUID) (domain.SplitSet, error) {
	var row scriptSplitSetRow
	if err := tx.Where("org_id = ? AND project_id = ? AND id = ?", org, project, id).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.SplitSet{}, application.ErrNotFound
		}
		return domain.SplitSet{}, err
	}
	return row.splitSet()
}

func insertEpisode(tx *gorm.DB, e domain.Episode) error {
	row := scriptEpisodeRow{
		ID: e.ID, OrgID: e.OrgID, ProjectID: e.ProjectID, VersionID: e.VersionID,
		SplitSetID: e.SplitSetID, SeqNo: e.SeqNo, Title: e.Title, Start: e.Start,
		End: e.End, Revision: e.Revision, CurrentStructureID: e.CurrentStructureID,
		ConfirmedStructureID: e.ConfirmedStructureID, PreviousEpisodeID: e.PreviousEpisodeID,
		InheritStatus: e.InheritStatus, IsDelete: e.IsDelete,
	}
	return exactlyOne(tx.Create(&row))
}
