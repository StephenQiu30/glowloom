package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// Versions pages immutable chronological metadata without retrieving private bodies.
func (s *SourceStore) Versions(ctx context.Context, actor identityapp.Principal, project uuid.UUID, before int64, limit int) (application.VersionPage, error) {
	result := application.VersionPage{Items: []application.VersionSummary{}}
	err := s.transaction(ctx, actor, project, false, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		query := tx.Model(&scriptVersionSummaryRow{}).Where("org_id = ? AND project_id = ?", actor.OrgID, project)
		if before > 0 {
			query = query.Where("version_no < ?", before)
		}
		var rows []scriptVersionSummaryRow
		if err := query.Order("version_no DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			result.Items = append(result.Items, application.VersionSummary{ID: row.ID, VersionNo: row.VersionNo, ContentHash: row.ContentHash, DocumentSHA256: row.DocumentSHA256, SourceManifestSHA256: row.SourceManifestSHA256, CharCount: row.CharCount, SourceCount: len(row.SourceIDs), CreatedAt: row.CreatedAt.UTC()})
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
		query := tx.Model(&scriptSourceSummaryRow{}).Where("org_id = ? AND project_id = ? AND source_lineage_id = ?", actor.OrgID, project, lineage)
		if before > 0 {
			query = query.Where("source_revision < ?", before)
		}
		var rows []scriptSourceSummaryRow
		if err := query.Order("source_revision DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			result.Items = append(result.Items, application.SourceSummary{ID: row.ID, LineageID: row.LineageID, PreviousID: row.PreviousID, Revision: row.Revision, Kind: row.Kind, Title: row.Title, Status: row.Status, Origin: row.Origin, CharCount: row.CharCount, ContentHash: row.ContentHash, RichSHA256: row.RichSHA256, MediaAssetID: row.MediaAssetID})
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
		query := tx.Model(&scriptStructureSummaryRow{}).Where("org_id = ? AND project_id = ? AND episode_id = ?", actor.OrgID, e.ProjectID, id)
		if before > 0 {
			query = query.Where("version_no < ?", before)
		}
		var rows []scriptStructureSummaryRow
		if err := query.Order("version_no DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			result.Items = append(result.Items, application.StructureSummary{ID: row.ID, EpisodeID: row.EpisodeID, VersionNo: row.VersionNo, SourceHash: row.SourceHash, CreatedAt: row.CreatedAt.UTC()})
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
			var row scriptStructureSummaryRow
			if err := tx.Select("id").Where("org_id = ? AND project_id = ? AND episode_id = ? AND version_no = ?", actor.OrgID, e.ProjectID, id, version).Take(&row).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return application.ErrNotFound
				}
				return err
			}
			if row.ID == uuid.Nil {
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
