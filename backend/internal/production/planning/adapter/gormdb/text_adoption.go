package gormdb

import (
	"context"
	"errors"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AcceptTextPlanning writes formal planning facts in the caller's adoption transaction.
func AcceptTextPlanning(ctx context.Context, tx *gorm.DB, input application.TextPlanningInput) (application.TextPlanningResult, error) {
	var result application.TextPlanningResult
	var project model.Project
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, "id = ? AND workspace_id = ?", input.ProjectID, input.WorkspaceID).Error; err != nil {
		return result, err
	}
	input.TargetDurationMS = project.TargetDurationMS
	result, err := application.BuildTextPlanning(input)
	if err != nil {
		return result, err
	}
	if input.Stage == "map_manuscript" {
		if project.Revision != input.ExpectedProjectRevision {
			return result, &application.Error{Code: "revision_conflict", Message: "Project changed before episode map adoption", Status: 409}
		}
		var count int64
		if err = tx.Model(&model.Episode{}).Where("project_id = ?", input.ProjectID).Count(&count).Error; err != nil {
			return result, err
		}
		if count != 0 {
			return result, &application.Error{Code: "episode_map_conflict", Message: "Project already has formal episodes", Status: 409}
		}
		for i, episode := range result.Episodes {
			record, convertErr := episodeRecord(episode)
			if convertErr != nil {
				return result, convertErr
			}
			if err = tx.Omit(clause.Associations).Create(&record).Error; err != nil {
				return result, err
			}
			version, convertErr := versionRecord(result.Versions[i])
			if convertErr != nil {
				return result, convertErr
			}
			if err = tx.Omit(clause.Associations).Create(&version).Error; err != nil {
				return result, err
			}
		}
		events, eventErr := application.TextPublicationEvents(input, result.Versions)
		if eventErr != nil {
			return result, eventErr
		}
		for _, event := range events {
			record, convertErr := outboxRecord(event)
			if convertErr != nil {
				return result, convertErr
			}
			if err = tx.Omit(clause.Associations).Create(&record).Error; err != nil {
				return result, err
			}
		}
		err = tx.Model(&project).Updates(map[string]any{"revision": project.Revision + 1, "updated_at": input.CreatedAt}).Error
		return result, err
	}
	if result.Structure == nil {
		return result, errors.New("text structure is missing")
	}
	var source model.EpisodeScriptVersion
	if err = tx.First(&source, "id = ? AND episode_id = ? AND project_id = ? AND document_revision_id = ? AND status = ?", result.Structure.ScriptVersionID, result.Structure.EpisodeID, input.ProjectID, input.SourceRevisionID, "published").Error; err != nil {
		return result, err
	}
	for _, scene := range result.Structure.Scenes {
		if scene.SourceStart < source.SourceStart || scene.SourceEnd > source.SourceEnd {
			return result, errors.New("scene escapes adopted episode source")
		}
	}
	var count int64
	if err = tx.Model(&model.EpisodeStructure{}).Where("script_version_id = ?", source.ID).Count(&count).Error; err != nil {
		return result, err
	}
	if count > 0 {
		return result, &application.Error{Code: "structure_conflict", Message: "Episode already has an adopted structure", Status: 409}
	}
	record, err := structureRecord(*result.Structure)
	if err != nil {
		return result, err
	}
	return result, tx.Omit(clause.Associations).Create(&record).Error
}
