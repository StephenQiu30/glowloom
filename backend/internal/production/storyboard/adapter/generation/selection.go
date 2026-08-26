package generation

import (
	"context"
	"errors"

	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	storyboardapp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
)

type SelectionReader interface {
	RequireSelected(context.Context, generationapp.Actor, string) (generationdomain.CandidateSelection, error)
}

type SelectedImageSource struct{ selections SelectionReader }

func NewSelectedImageSource(selections SelectionReader) *SelectedImageSource {
	return &SelectedImageSource{selections: selections}
}

func (source *SelectedImageSource) RequireSelectedImage(
	ctx context.Context,
	actor storyboardapp.Actor,
	selectionID string,
) (storyboardapp.SelectedImageSnapshot, error) {
	if source == nil || source.selections == nil {
		return storyboardapp.SelectedImageSnapshot{}, errors.New("Generation selection source is unavailable")
	}
	selection, err := source.selections.RequireSelected(ctx, generationapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, selectionID)
	if err != nil {
		return storyboardapp.SelectedImageSnapshot{}, err
	}
	var selected generationdomain.CandidateReference
	for _, candidate := range selection.Candidates {
		if candidate.ID == selection.SelectedCandidateID {
			if selected.ID != "" {
				return storyboardapp.SelectedImageSnapshot{}, errors.New("Generation selection has duplicate selected candidates")
			}
			selected = candidate
		}
	}
	if selected.ID == "" || selected.ArtifactID != selection.SelectedArtifactID ||
		selected.ArtifactSHA256 != selection.SelectedArtifactSHA256 {
		return storyboardapp.SelectedImageSnapshot{}, errors.New("Generation selection artifact snapshot has drifted")
	}
	return storyboardapp.SelectedImageSnapshot{
		ID: selection.ID, WorkspaceID: selection.WorkspaceID, ProjectID: selection.ProjectID,
		Revision: selection.Revision, ContentHash: selection.ContentHash,
		CandidateID: selected.ID, CandidateRevision: selected.Revision,
		ArtifactID: selected.ArtifactID, ArtifactRevision: selected.ArtifactRevision,
		ArtifactSHA256: selected.ArtifactSHA256,
	}, nil
}

var _ storyboardapp.SelectedImageSource = (*SelectedImageSource)(nil)
