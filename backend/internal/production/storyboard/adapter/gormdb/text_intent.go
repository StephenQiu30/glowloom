package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	bible "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	planning "github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func AcceptTextIntent(ctx context.Context, tx *gorm.DB, input app.TextIntentInput) (domain.TextIntentVersion, error) {
	value, err := app.BuildTextIntent(input)
	if err != nil {
		return value, err
	}
	fail := errors.New("text intent formal references changed before acceptance")
	var decision model.ReviewDecision
	if err = tx.WithContext(ctx).First(&decision, "id = ? AND workspace_id = ? AND decision = ?", value.DecisionID, value.WorkspaceID, "approved").Error; err != nil {
		return value, err
	}
	var review model.HumanTask
	if err = tx.WithContext(ctx).First(&review, "id = ? AND project_id = ? AND subject_id = ?", decision.HumanTaskID, value.ProjectID, value.ProposalID).Error; err != nil {
		return value, err
	}
	var structure model.EpisodeStructure
	if err = tx.WithContext(ctx).First(&structure, "id = ? AND workspace_id = ? AND project_id = ? AND episode_id = ? AND status = ?", value.StructureID, value.WorkspaceID, value.ProjectID, value.EpisodeID, "confirmed").Error; err != nil {
		return value, err
	}
	var script model.EpisodeScriptVersion
	if err = tx.WithContext(ctx).First(&script, "id = ? AND document_revision_id = ?", structure.ScriptVersionID, value.SourceRevisionID).Error; err != nil {
		return value, err
	}
	var task contract.TextExecutionTask
	if err = json.Unmarshal(input.Task, &task); err != nil {
		return value, err
	}
	var scenes []planning.Scene
	if err = json.Unmarshal(structure.Scenes, &scenes); err != nil {
		return value, err
	}
	var scene *planning.Scene
	for i := range scenes {
		if scenes[i].ID == value.SceneID {
			scene = &scenes[i]
			break
		}
	}
	if scene == nil || scene.TextFacts == nil {
		return value, fail
	}
	var expected *contract.TextScene
	for _, analysis := range task.Analyses {
		if analysis.EpisodeKey == *task.EpisodeKey {
			for i := range analysis.Scenes {
				if analysis.Scenes[i].Key == *task.SceneKey {
					expected = &analysis.Scenes[i]
					break
				}
			}
		}
	}
	if expected == nil || scene.TemporaryKey != expected.Key || scene.TextFacts.TimeBranch != expected.TimeBranch {
		return value, fail
	}
	prefix := *task.EpisodeKey + "/" + *task.SceneKey
	for _, beat := range expected.Beats {
		found := false
		for _, unit := range scene.NarrativeUnits {
			if unit.ID == value.IDMapping["beat/"+prefix+"/"+beat.Key] && unit.TemporaryKey == beat.Key && unit.Text == beat.Action && unit.Required != nil && *unit.Required == beat.Required {
				found = true
				break
			}
		}
		if !found {
			return value, fail
		}
	}
	for _, dialogue := range expected.Dialogues {
		found := false
		for _, formal := range scene.Dialogues {
			if formal.ID == value.IDMapping["dialogue/"+prefix+"/"+dialogue.Key] && formal.TemporaryKey == dialogue.Key && formal.Text == dialogue.Text && formal.Channel == dialogue.Channel {
				found = true
				break
			}
		}
		if !found {
			return value, fail
		}
	}
	for _, mention := range expected.Mentions {
		found := false
		for _, formal := range scene.TextFacts.Mentions {
			if formal.ID == value.IDMapping["mention/"+prefix+"/"+mention.Key] && formal.TemporaryKey == mention.Key && formal.Kind == mention.Kind && formal.Presence == mention.Presence {
				found = true
				break
			}
		}
		if !found {
			return value, fail
		}
	}
	var worldRecord model.TextWorldVersion
	if err = tx.WithContext(ctx).First(&worldRecord, "id = ? AND project_id = ? AND workspace_id = ? AND source_revision_id = ? AND source_hash = ? AND run_id = ?", value.WorldVersionID, value.ProjectID, value.WorkspaceID, value.SourceRevisionID, value.SourceHash, value.RunID).Error; err != nil {
		return value, err
	}
	var world bible.TextWorldVersion
	if err = json.Unmarshal(worldRecord.Body, &world); err != nil {
		return value, err
	}
	if len(task.World.Entities) != len(world.Entities) {
		return value, fail
	}
	for _, entity := range task.World.Entities {
		var formal *bible.TextWorldEntity
		for i := range world.Entities {
			if world.Entities[i].ID == value.IDMapping["entity/"+entity.Key] && world.Entities[i].Key == entity.Key {
				formal = &world.Entities[i]
				break
			}
		}
		if formal == nil || formal.Kind != entity.Kind || formal.Label != entity.Label || len(formal.MentionIDs) != len(entity.Mentions) {
			return value, fail
		}
		for _, mention := range entity.Mentions {
			if !slices.Contains(formal.MentionIDs, value.IDMapping["mention/"+mention.EpisodeKey+"/"+mention.SceneKey+"/"+mention.MentionKey]) {
				return value, fail
			}
		}
	}
	body, err := json.Marshal(value)
	if err != nil {
		return value, err
	}
	record := model.TextIntentVersion{ID: uuid.MustParse(value.ID), RunID: uuid.MustParse(value.RunID), ProposalID: uuid.MustParse(value.ProposalID), DecisionID: uuid.MustParse(value.DecisionID), WorkspaceID: uuid.MustParse(value.WorkspaceID), ProjectID: uuid.MustParse(value.ProjectID), SourceRevisionID: uuid.MustParse(value.SourceRevisionID), SourceHash: value.SourceHash, EpisodeID: uuid.MustParse(value.EpisodeID), StructureID: uuid.MustParse(value.StructureID), SceneID: uuid.MustParse(value.SceneID), WorldVersionID: uuid.MustParse(value.WorldVersionID), Revision: value.Revision, ContentHash: value.ContentHash, AssetReadiness: value.AssetReadiness, Body: datatypes.JSON(body), CreatedBy: uuid.MustParse(value.CreatedBy), CreatedAt: value.CreatedAt}
	if err = tx.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		return value, err
	}
	return value, nil
}
