package gormdb

import (
	"context"
	"errors"
	"fmt"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	"gorm.io/gorm"
)

func (s *Store) ReadTextWorld(ctx context.Context, workspaceID, projectID, id string) (domain.TextWorldVersion, error) {
	var row model.TextWorldVersion
	err := s.database.WithContext(ctx).First(&row, "id = ? AND workspace_id = ? AND project_id = ?", id, workspaceID, projectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.TextWorldVersion{}, app.ErrNotFound
	}
	if err != nil {
		return domain.TextWorldVersion{}, fmt.Errorf("read formal version: %w", err)
	}
	var value domain.TextWorldVersion
	if canonical.Decode(row.Body, &value) != nil || value.ID != row.ID.String() || value.ProjectID != row.ProjectID.String() || value.WorkspaceID != row.WorkspaceID.String() || value.RunID != row.RunID.String() || value.ProposalID != row.ProposalID.String() || value.DecisionID != row.DecisionID.String() || value.SourceRevisionID != row.SourceRevisionID.String() || value.SourceHash != row.SourceHash || value.ContentHash != row.ContentHash || value.Revision != row.Revision || value.CreatedBy != row.CreatedBy.String() || app.ValidateTextWorldVersion(value) != nil {
		return domain.TextWorldVersion{}, &app.Error{Code: "formal_version_drift", Message: "Stored version does not match its evidence", Status: 409}
	}
	return value, nil
}
