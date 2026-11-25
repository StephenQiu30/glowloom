package aigateway

import (
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/domain"
	workspacepg "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/adapter/http"
	operationpg "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/adapter/postgres"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/domain"
)

func checkStoryboardQuoteInput(tx *gorm.DB, projectID uuid.UUID, in operationhttp.QuoteItemRequest) error {
	if in.InputSource != nil || in.ExpectedSubmissionEpoch != nil || in.PromptTemplate != nil ||
		in.Capability != "video.generate" || in.Mode != "text2video" || in.OutputCount != 1 || len(in.MediaInputs) != 0 {
		return domain.ErrInvalid
	}
	shot, err := operationpg.ReadStoryboardQuoteShot(tx, projectID, *in.Source, true)
	switch {
	case errors.Is(err, operation.ErrPublicNotFound):
		return operation.ErrQuoteSourceStale
	case errors.Is(err, operation.ErrInvalidFreeQuote):
		return domain.ErrInvalid
	case err != nil:
		return err
	}
	if !storyboardInputMatches(shot, in.Prompt, in.Params) {
		return domain.ErrInvalid
	}
	return checkStoryboardConsumption(tx, projectID, uuid.Nil, *in.Source)
}

type storyboardQuoteSource struct {
	operationdomain.CanvasSource
	MemoryVersion string `json:"memory_version"`
}

func checkStoryboardConfirmation(tx *gorm.DB, call domain.Call, frozenSource storyboardQuoteSource) error {
	source := frozenSource.CanvasSource
	shot, err := operationpg.ReadStoryboardQuoteShot(tx, call.Project, source, false)
	switch {
	case errors.Is(err, operation.ErrPublicNotFound), errors.Is(err, operation.ErrInvalidFreeQuote):
		return operation.ErrQuoteSourceStale
	case err != nil:
		return err
	}
	prompt, memoryVersion, err := storyboardMemoryPrompt(tx, call.Project, source.ShotID, storyboardPrompt(shot))
	if err != nil {
		return err
	}
	var frozen domain.MediaInput
	if frozenSource.MemoryVersion != memoryVersion || call.Model.Protocol != "media" || json.Unmarshal(call.Raw, &frozen) != nil ||
		frozen.Capability != "video.generate" || frozen.Mode != "text2video" || frozen.OutputCount != 1 ||
		len(frozen.References) != 0 || frozen.Prompt != prompt ||
		!frozenStoryboardDurationMatches(shot.DurationMS, call.Model, frozen.Params) {
		return operation.ErrQuoteSourceStale
	}
	if err := checkStoryboardConsumption(tx, call.Project, call.ID, source); err != nil {
		return err
	}
	// Queue checks the frozen price and model contract. New storyboard admission
	// also requires that the model still be enabled.
	var active struct{ ID uuid.UUID }
	read := tx.Raw("SELECT id FROM ai.model WHERE id=? AND enabled FOR SHARE", call.Model.ID).Scan(&active)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 {
		return operation.ErrQuoteStale
	}
	return nil
}

// storyboardMemoryPrompt freezes the applicable confirmed decisions with the
// exact generation input. A shot decision overrides the same project key.
func storyboardMemoryPrompt(tx *gorm.DB, project uuid.UUID, shot *uuid.UUID, prompt string) (string, string, error) {
	defaults, err := workspacepg.ReadActiveProjectMemories(tx, project)
	if err != nil {
		return "", "", err
	}
	local, err := workspacepg.ReadActiveMemoriesForShot(tx, project, shot)
	if err != nil {
		return "", "", err
	}
	byKey := make(map[string]workspace.ProjectMemory, len(defaults)+len(local))
	for _, memory := range defaults {
		byKey[memory.Key] = memory
	}
	for _, memory := range local {
		byKey[memory.Key] = memory
	}
	effective := make([]workspace.ProjectMemory, 0, len(byKey))
	for _, memory := range byKey {
		effective = append(effective, memory)
	}
	slices.SortFunc(effective, func(a, b workspace.ProjectMemory) int { return strings.Compare(a.Key, b.Key) })
	if len(effective) == 0 {
		return prompt, "", nil
	}
	var out strings.Builder
	out.WriteString(prompt)
	out.WriteString("\n\n已确认的创作约定")
	for _, memory := range effective {
		out.WriteString("\n- ")
		out.WriteString(memory.Key)
		out.WriteString("：")
		out.WriteString(memory.Statement)
	}
	return out.String(), workspacepg.ProjectMemoryVersion(effective), nil
}

func storyboardInputMatches(shot canvasdomain.StoryboardShot, prompt string, params json.RawMessage) bool {
	return prompt == storyboardPrompt(shot) && operation.StoryboardShotDurationMatches(shot.DurationMS, params)
}

func storyboardPrompt(shot canvasdomain.StoryboardShot) string {
	return operation.StoryboardShotPrompt(shot.Title, shot.Action, shot.Dialogue, shot.Camera, shot.Sound)
}

func storyboardModelParams(model domain.Model, params json.RawMessage) (json.RawMessage, error) {
	var values map[string]json.RawMessage
	if json.Unmarshal(params, &values) != nil || values["duration"] != nil {
		return nil, domain.ErrInvalid
	}
	if model.Parameters["duration"].Type == "integer" {
		var milliseconds int64
		if json.Unmarshal(values["duration_ms"], &milliseconds) != nil || milliseconds <= 0 || milliseconds%1000 != 0 {
			return nil, domain.ErrInvalid
		}
		delete(values, "duration_ms")
		values["duration"] = json.RawMessage(strconv.FormatInt(milliseconds/1000, 10))
		return json.Marshal(values)
	}
	if model.Parameters["duration_ms"].Type != "integer" {
		return nil, domain.ErrUnavailable
	}
	return params, nil
}

func frozenStoryboardDurationMatches(milliseconds int64, model domain.Model, params map[string]any) bool {
	if model.Parameters["duration"].Type == "integer" {
		return milliseconds > 0 && milliseconds%1000 == 0 && params["duration"] == float64(milliseconds/1000)
	}
	return model.Parameters["duration_ms"].Type == "integer" && milliseconds > 0 && params["duration_ms"] == float64(milliseconds)
}

func checkStoryboardConsumption(tx *gorm.DB, projectID, operationID uuid.UUID, source operationdomain.CanvasSource) error {
	lock := "storyboard-admission/" + projectID.String() + "/" + source.CanvasID.String() + "/" +
		source.NodeID.String() + "/" + source.ShotID.String()
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", lock).Error; err != nil {
		return err
	}
	var pending bool
	err := tx.Raw(`SELECT EXISTS (
		SELECT 1 FROM ai.creation c JOIN ai.request r ON r.id=c.id
		WHERE c.project_id=? AND c.id<>? AND c.state='accepted'
		  AND c.source->>'canvas_id'=? AND c.source->>'node_id'=? AND c.source->>'shot_id'=?
		  AND r.state IN ('sending','pending','received','unknown')
	)`, projectID, operationID, source.CanvasID.String(), source.NodeID.String(), source.ShotID.String()).Row().Scan(&pending)
	if err != nil {
		return err
	}
	if pending {
		return domain.ErrUnknown
	}
	return nil
}
