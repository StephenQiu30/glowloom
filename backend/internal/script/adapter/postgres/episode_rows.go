package postgres

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

const episodeColumns = `id,org_id,project_id,script_version_id AS version_id,split_set_id,seq_no,title,span_start AS start,span_end AS "end",revision,current_structure_id,confirmed_structure_id,previous_episode_id,inherit_status,is_delete`

func readEpisode(tx *gorm.DB, org, id uuid.UUID, lock bool) (domain.Episode, error) {
	query := `SELECT ` + episodeColumns + ` FROM script.episode WHERE org_id=? AND id=?`
	if lock {
		query += ` AND NOT is_delete FOR UPDATE`
	}
	var episode domain.Episode
	read := tx.Raw(query, org, id).Scan(&episode)
	if read.Error != nil {
		return episode, fmt.Errorf("read formal episode: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return episode, application.ErrNotFound
	}
	return episode, nil
}
func readEpisodes(tx *gorm.DB, org, project, version uuid.UUID, lock bool) ([]domain.Episode, error) {
	query := `SELECT ` + episodeColumns + ` FROM script.episode WHERE org_id=? AND project_id=? AND script_version_id=? AND NOT is_delete ORDER BY seq_no,id`
	if lock {
		query += ` FOR UPDATE`
	}
	result := make([]domain.Episode, 0)
	if err := tx.Raw(query, org, project, version).Scan(&result).Error; err != nil {
		return nil, err
	}
	return result, nil
}
func readVersionHead(tx *gorm.DB, org, project, version uuid.UUID, lock bool) (application.VersionHead, error) {
	query := `SELECT version_id,split_revision,candidate_split_set_id AS candidate_set_id,confirmed_split_set_id AS confirmed_set_id FROM script.version_head WHERE org_id=? AND project_id=? AND version_id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	var result application.VersionHead
	read := tx.Raw(query, org, project, version).Scan(&result)
	if read.Error != nil {
		return result, read.Error
	}
	if read.RowsAffected != 1 {
		return result, application.ErrNotFound
	}
	return result, nil
}
func readSplitSet(tx *gorm.DB, org, project, id uuid.UUID) (domain.SplitSet, error) {
	var row struct {
		ID, OrgID, ProjectID, VersionID             uuid.UUID
		Kind, Origin, Preface, Boundaries, Warnings string
		CreatedAt                                   time.Time
	}
	read := tx.Raw(`SELECT id,org_id,project_id,version_id,kind,origin,preface::text,boundaries::text,warnings::text,created_at FROM script.split_set WHERE org_id=? AND project_id=? AND id=?`, org, project, id).Scan(&row)
	if read.Error != nil {
		return domain.SplitSet{}, read.Error
	}
	if read.RowsAffected != 1 {
		return domain.SplitSet{}, application.ErrNotFound
	}
	result := domain.SplitSet{ID: row.ID, OrgID: row.OrgID, ProjectID: row.ProjectID, VersionID: row.VersionID, Kind: row.Kind, Origin: row.Origin, CreatedAt: row.CreatedAt.UTC()}
	for _, value := range []struct {
		data string
		out  any
	}{{row.Preface, &result.Preface}, {row.Boundaries, &result.Boundaries}, {row.Warnings, &result.Warnings}} {
		if err := json.Unmarshal([]byte(value.data), value.out); err != nil {
			return domain.SplitSet{}, application.ErrUnavailable
		}
	}
	return result, nil
}

func insertEpisode(tx *gorm.DB, e domain.Episode) error {
	return exactlyOne(tx.Exec(`INSERT INTO script.episode(id,org_id,project_id,script_version_id,split_set_id,seq_no,title,span_start,span_end,revision,current_structure_id,confirmed_structure_id,previous_episode_id,inherit_status,is_delete) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, e.ID, e.OrgID, e.ProjectID, e.VersionID, e.SplitSetID, e.SeqNo, e.Title, e.Start, e.End, e.Revision, e.CurrentStructureID, e.ConfirmedStructureID, e.PreviousEpisodeID, e.InheritStatus, e.IsDelete))
}
