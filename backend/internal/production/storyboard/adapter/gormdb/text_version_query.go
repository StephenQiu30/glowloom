package gormdb

import (
	"context"
	"errors"
	"fmt"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
	"gorm.io/gorm"
)

func (s *Store) ReadTextIntent(ctx context.Context, workspaceID, projectID, id string) (domain.TextIntentVersion, error) {
	var row model.TextIntentVersion
	err := s.database.WithContext(ctx).First(&row, "id = ? AND workspace_id = ? AND project_id = ?", id, workspaceID, projectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.TextIntentVersion{}, app.ErrNotFound
	}
	if err != nil {
		return domain.TextIntentVersion{}, fmt.Errorf("read formal version: %w", err)
	}
	var value domain.TextIntentVersion
	if canonical.Decode(row.Body, &value) != nil || value.ID != row.ID.String() || value.ProjectID != row.ProjectID.String() || value.WorkspaceID != row.WorkspaceID.String() || value.RunID != row.RunID.String() || value.ProposalID != row.ProposalID.String() || value.DecisionID != row.DecisionID.String() || value.SourceRevisionID != row.SourceRevisionID.String() || value.SourceHash != row.SourceHash || value.ContentHash != row.ContentHash || value.Revision != row.Revision || value.CreatedBy != row.CreatedBy.String() || value.EpisodeID != row.EpisodeID.String() || value.SceneID != row.SceneID.String() || value.StructureID != row.StructureID.String() || value.WorldVersionID != row.WorldVersionID.String() || value.AssetReadiness != row.AssetReadiness || app.ValidateTextIntentVersion(value) != nil {
		return domain.TextIntentVersion{}, &app.Error{Code: "formal_version_drift", Message: "Stored version does not match its evidence", Status: 409}
	}
	return value, nil
}
