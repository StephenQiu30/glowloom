package gormdb

import (
	"context"
	"encoding/json"
	"errors"

	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	planning "github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AcceptTextWorld participates in the caller's adoption transaction. There is no
// independent commit between the formal version and the acceptance receipt.
func AcceptTextWorld(ctx context.Context, tx *gorm.DB, input app.TextWorldInput) (domain.TextWorldVersion, error) {
	value, err := app.BuildTextWorld(input)
	if err != nil {
		return value, err
	}
	fail := errors.New("text world owner references are not accepted in this project")
	var decision model.ReviewDecision
	if err = tx.WithContext(ctx).First(&decision, "id = ? AND workspace_id = ? AND decision = ?", value.DecisionID, value.WorkspaceID, "approved").Error; err != nil {
		return value, err
	}
	var review model.HumanTask
	if err = tx.WithContext(ctx).First(&review, "id = ? AND project_id = ? AND subject_id = ?", decision.HumanTaskID, value.ProjectID, value.ProposalID).Error; err != nil {
		return value, err
	}
	var task contract.TextExecutionTask
	if err = json.Unmarshal(input.Task, &task); err != nil {
		return value, err
	}
	for _, analysis := range task.Analyses {
		var structure model.EpisodeStructure
		if err = tx.WithContext(ctx).First(&structure, "id = ? AND workspace_id = ? AND project_id = ? AND episode_id = ? AND status = ?", value.IDMapping["structure/"+analysis.EpisodeKey], value.WorkspaceID, value.ProjectID, value.IDMapping["episode/"+analysis.EpisodeKey], "confirmed").Error; err != nil {
			return value, err
		}
		var script model.EpisodeScriptVersion
		if err = tx.WithContext(ctx).First(&script, "id = ? AND document_revision_id = ?", structure.ScriptVersionID, value.SourceRevisionID).Error; err != nil {
			return value, err
		}
		var scenes []planning.Scene
		if err = json.Unmarshal(structure.Scenes, &scenes); err != nil {
			return value, err
		}
		for _, expected := range analysis.Scenes {
			var actual *planning.Scene
			for i := range scenes {
				if scenes[i].ID == value.IDMapping["scene/"+analysis.EpisodeKey+"/"+expected.Key] {
					actual = &scenes[i]
					break
				}
			}
			if actual == nil || actual.TemporaryKey != expected.Key || actual.TextFacts == nil || actual.TextFacts.TimeBranch != expected.TimeBranch {
				return value, fail
			}
			for _, mention := range expected.Mentions {
				found := false
				for _, formal := range actual.TextFacts.Mentions {
					if formal.ID == value.IDMapping["mention/"+analysis.EpisodeKey+"/"+expected.Key+"/"+mention.Key] && formal.TemporaryKey == mention.Key && formal.Kind == mention.Kind && formal.Name == mention.Name {
						found = true
						break
					}
				}
				if !found {
					return value, fail
				}
			}
		}
	}
	body, err := json.Marshal(value)
	if err != nil {
		return value, err
	}
	record := model.TextWorldVersion{ID: uuid.MustParse(value.ID), RunID: uuid.MustParse(value.RunID), ProposalID: uuid.MustParse(value.ProposalID), DecisionID: uuid.MustParse(value.DecisionID), WorkspaceID: uuid.MustParse(value.WorkspaceID), ProjectID: uuid.MustParse(value.ProjectID), SourceRevisionID: uuid.MustParse(value.SourceRevisionID), SourceHash: value.SourceHash, Revision: value.Revision, ContentHash: value.ContentHash, Body: datatypes.JSON(body), CreatedBy: uuid.MustParse(value.CreatedBy), CreatedAt: value.CreatedAt}
	if err = tx.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		return value, err
	}
	return value, nil
}
