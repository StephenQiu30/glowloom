package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"time"

	"github.com/google/uuid"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const ProjectSelectionContractID = "project-preset-selection-production"

type ProjectSelectionRelease struct {
	Key         string `json:"key"`
	Release     string `json:"release"`
	ContentHash string `json:"content_hash"`
}

type ProjectSelectionInput struct {
	WorkspaceID       string                  `json:"workspace_id"`
	ProjectID         string                  `json:"project_id"`
	Revision          int64                   `json:"revision"`
	ParentSelectionID *string                 `json:"parent_selection_id"`
	ParentContentHash *string                 `json:"parent_content_hash"`
	PresetRelease     ProjectSelectionRelease `json:"preset_release"`
	ApplicationMode   string                  `json:"application_mode"`
	SelectedBy        string                  `json:"selected_by"`
	SelectedAt        time.Time               `json:"selected_at"`
}

type ProjectSelection struct {
	ID         string `json:"id"`
	ContractID string `json:"contract_id"`
	ProjectSelectionInput
	ContentHash string `json:"content_hash"`
}

func NewProjectSelection(id string, input ProjectSelectionInput) (ProjectSelection, json.RawMessage, error) {
	selection := ProjectSelection{ID: id, ContractID: ProjectSelectionContractID, ProjectSelectionInput: input}
	if err := validateProjectSelection(selection); err != nil {
		return ProjectSelection{}, nil, err
	}
	hash, err := projectSelectionHash(selection)
	if err != nil {
		return ProjectSelection{}, nil, err
	}
	selection.ContentHash = hash
	encoded, err := encodeProjectSelection(selection)
	if err != nil {
		return ProjectSelection{}, nil, err
	}
	return selection, encoded, nil
}

func DecodeProjectSelection(raw json.RawMessage) (ProjectSelection, json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var selection ProjectSelection
	if err := decoder.Decode(&selection); err != nil {
		return ProjectSelection{}, nil, errors.New("invalid Project Preset selection")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ProjectSelection{}, nil, errors.New("invalid Project Preset selection")
	}
	if err := validateProjectSelection(selection); err != nil || !hashPattern.MatchString(selection.ContentHash) {
		return ProjectSelection{}, nil, errors.New("invalid Project Preset selection")
	}
	hash, err := projectSelectionHash(selection)
	if err != nil || hash != selection.ContentHash {
		return ProjectSelection{}, nil, errors.New("Project Preset selection content hash has drifted")
	}
	encoded, err := encodeProjectSelection(selection)
	if err != nil {
		return ProjectSelection{}, nil, err
	}
	return selection, encoded, nil
}

func validateProjectSelection(selection ProjectSelection) error {
	input := selection.ProjectSelectionInput
	if selection.ContractID != ProjectSelectionContractID || uuid.Validate(selection.ID) != nil ||
		uuid.Validate(input.WorkspaceID) != nil || uuid.Validate(input.ProjectID) != nil ||
		uuid.Validate(input.SelectedBy) != nil || input.Revision < 1 ||
		!releaseKeyPattern.MatchString(input.PresetRelease.Key) || !validReleaseDate(input.PresetRelease.Release) ||
		!hashPattern.MatchString(input.PresetRelease.ContentHash) ||
		!slices.Contains([]string{"faithful", "world_adaptation"}, input.ApplicationMode) ||
		input.SelectedAt.IsZero() || input.SelectedAt.Location() != time.UTC {
		return errors.New("invalid Project Preset selection")
	}
	if input.Revision == 1 {
		if input.ParentSelectionID != nil || input.ParentContentHash != nil {
			return errors.New("invalid Project Preset selection parent")
		}
		return nil
	}
	if input.ParentSelectionID == nil || uuid.Validate(*input.ParentSelectionID) != nil ||
		input.ParentContentHash == nil || !hashPattern.MatchString(*input.ParentContentHash) {
		return errors.New("invalid Project Preset selection parent")
	}
	return nil
}

func projectSelectionHash(selection ProjectSelection) (string, error) {
	raw, err := json.Marshal(struct {
		ContractID string `json:"contract_id"`
		ProjectSelectionInput
	}{ContractID: selection.ContractID, ProjectSelectionInput: selection.ProjectSelectionInput})
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func encodeProjectSelection(selection ProjectSelection) (json.RawMessage, error) {
	raw, err := json.Marshal(selection)
	if err != nil {
		return nil, err
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(canonical), nil
}
