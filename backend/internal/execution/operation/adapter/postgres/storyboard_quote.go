package postgres

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/domain"
)

// ReadStoryboardQuoteShot locks the source document and returns the exact current shot.
func ReadStoryboardQuoteShot(tx *gorm.DB, projectID uuid.UUID, source domain.CanvasSource, fromCurrentSnapshot bool) (canvasdomain.StoryboardShot, error) {
	if !source.Valid() || source.ShotID == nil {
		return canvasdomain.StoryboardShot{}, application.ErrInvalidFreeQuote
	}
	node, err := readQuoteCanvasNode(tx, projectID, source, fromCurrentSnapshot)
	if err != nil {
		return canvasdomain.StoryboardShot{}, err
	}
	return readQuoteStoryboardShot(node, source)
}

func readQuoteStoryboardShot(node quoteCanvasNode, source domain.CanvasSource) (canvasdomain.StoryboardShot, error) {
	if node.DocumentKind != "canvas" || node.NodeType != canvasdomain.NodeTypeStoryboard || source.ShotID == nil {
		return canvasdomain.StoryboardShot{}, application.ErrInvalidFreeQuote
	}
	var config canvasdomain.NodeConfig
	if json.Unmarshal([]byte(node.Config), &config) != nil || config.IsOpaque() || config.Storyboard == nil {
		return canvasdomain.StoryboardShot{}, application.ErrInvalidFreeQuote
	}
	for _, shot := range config.Storyboard.Shots {
		if shot.ID != *source.ShotID {
			continue
		}
		if shot.Revision != *source.ShotRevision {
			break
		}
		return shot, nil
	}
	return canvasdomain.StoryboardShot{}, application.ErrQuoteSourceStale
}

func storyboardShotPrompt(shot canvasdomain.StoryboardShot) string {
	return application.StoryboardShotPrompt(shot.Title, shot.Action, shot.Dialogue, shot.Camera, shot.Sound)
}

// storyboardSourceChanged keeps the source locked through admission. A reorder
// or an edit to another shot can change the document snapshot independently.
func storyboardSourceChanged(tx *gorm.DB, quoted domain.Operation) (bool, error) {
	shot, err := ReadStoryboardQuoteShot(tx, quoted.ProjectID, *quoted.Source, false)
	if errors.Is(err, application.ErrPublicNotFound) || errors.Is(err, application.ErrInvalidFreeQuote) || errors.Is(err, application.ErrQuoteSourceStale) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !application.StoryboardShotDurationMatches(shot.DurationMS, quoted.Params) {
		return true, nil
	}
	var inputs []struct {
		Role, RefType string
		TextValue     *string
	}
	read := tx.Raw(`SELECT role,ref_type,text_value FROM operation.operation_input
		WHERE operation_id=? AND NOT is_delete`, quoted.ID).Scan(&inputs)
	if read.Error != nil {
		return false, fmt.Errorf("read frozen storyboard quote inputs: %w", read.Error)
	}
	return len(inputs) != 1 || inputs[0].Role != "prompt" || inputs[0].RefType != "text" ||
		inputs[0].TextValue == nil || *inputs[0].TextValue != storyboardShotPrompt(shot), nil
}
