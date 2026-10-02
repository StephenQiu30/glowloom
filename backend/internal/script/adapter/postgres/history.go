package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

// Versions pages immutable chronological metadata without retrieving private bodies.
func (s *SourceStore) Versions(ctx context.Context, actor identityapp.Principal, project uuid.UUID, before int64, limit int) (application.VersionPage, error) {
	result := application.VersionPage{Items: []application.VersionSummary{}}
	err := s.transaction(ctx, actor, project, false, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		query := `SELECT id,version_no,content_hash,document_sha256,source_manifest_sha256,char_count,cardinality(source_ids) AS source_count,created_at FROM script.script_version WHERE org_id=? AND project_id=? AND (?=0 OR version_no<?) ORDER BY version_no DESC LIMIT ?`
		if err := tx.Raw(query, actor.OrgID, project, before, before, limit+1).Scan(&result.Items).Error; err != nil {
			return err
		}
		if len(result.Items) > limit {
			result.Items = result.Items[:limit]
			cursor := result.Items[limit-1].VersionNo
			result.NextVersionNo = &cursor
		}
		return nil
	})
	return result, err
}

// SourceHistory reads every snapshot in a lineage, independent of current version membership.
func (s *SourceStore) SourceHistory(ctx context.Context, actor identityapp.Principal, project, lineage uuid.UUID, before int64, limit int) (application.SourceHistoryPage, error) {
	result := application.SourceHistoryPage{Items: []application.SourceSummary{}}
	err := s.transaction(ctx, actor, project, false, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		query := `SELECT id,source_lineage_id AS lineage_id,previous_source_id AS previous_id,source_revision AS revision,source_kind AS kind,title,status,origin,char_count,content_hash,rich_sha256,media_asset_id FROM script.script_source WHERE org_id=? AND project_id=? AND source_lineage_id=? AND (?=0 OR source_revision<?) ORDER BY source_revision DESC LIMIT ?`
		if err := tx.Raw(query, actor.OrgID, project, lineage, before, before, limit+1).Scan(&result.Items).Error; err != nil {
			return err
		}
		if len(result.Items) == 0 {
			return application.ErrNotFound
		}
		if len(result.Items) > limit {
			result.Items = result.Items[:limit]
			cursor := result.Items[limit-1].Revision
			result.NextRevision = &cursor
		}
		return nil
	})
	return result, err
}

// SourceSnapshot reads a selected immutable row after current owning authorization.
func (s *SourceStore) SourceSnapshot(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (domain.SourceRecord, error) {
	var result domain.SourceRecord
	err := s.transaction(ctx, actor, project, false, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		rows, err := readSources(tx, actor.OrgID, project, []uuid.UUID{id})
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			return application.ErrNotFound
		}
		result = rows[0]
		return nil
	})
	return result, err
}

// Structures pages immutable structure metadata scoped through the episode owner.
func (s *SourceStore) Structures(ctx context.Context, actor identityapp.Principal, id uuid.UUID, before int64, limit int) (application.StructurePage, error) {
	result := application.StructurePage{Items: []application.StructureSummary{}}
	e, err := s.Episode(ctx, actor, id)
	if err != nil {
		return result, err
	}
	err = s.transaction(ctx, actor, e.ProjectID, false, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		if err := tx.Raw(`SELECT id,episode_id,version_no,source_hash,created_at FROM script.episode_structure WHERE org_id=? AND project_id=? AND episode_id=? AND (?=0 OR version_no<?) ORDER BY version_no DESC LIMIT ?`, actor.OrgID, e.ProjectID, id, before, before, limit+1).Scan(&result.Items).Error; err != nil {
			return err
		}
		if len(result.Items) > limit {
			result.Items = result.Items[:limit]
			cursor := result.Items[limit-1].VersionNo
			result.NextVersionNo = &cursor
		}
		return nil
	})
	return result, err
}

// Structure preserves every historical body and selects only the exact episode relationship.
func (s *SourceStore) Structure(ctx context.Context, actor identityapp.Principal, id uuid.UUID, version int64) (domain.EpisodeStructure, error) {
	var result domain.EpisodeStructure
	e, err := s.Episode(ctx, actor, id)
	if err != nil {
		return result, err
	}
	err = s.transaction(ctx, actor, e.ProjectID, false, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		selected := e.CurrentStructureID
		if version > 0 {
			var row struct{ ID uuid.UUID }
			q := tx.Raw(`SELECT id FROM script.episode_structure WHERE org_id=? AND project_id=? AND episode_id=? AND version_no=?`, actor.OrgID, e.ProjectID, id, version).Scan(&row)
			if q.Error != nil {
				return q.Error
			}
			if q.RowsAffected != 1 {
				return application.ErrNotFound
			}
			selected = &row.ID
		}
		if selected == nil {
			return application.ErrNotFound
		}
		result, err = readStructure(tx, actor.OrgID, e.ProjectID, *selected)
		if err != nil {
			return err
		}
		if result.EpisodeID != id {
			return application.ErrNotFound
		}
		return nil
	})
	return result, err
}
