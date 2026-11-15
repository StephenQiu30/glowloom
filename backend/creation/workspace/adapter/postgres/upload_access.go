package postgres

import (
	"context"
	"errors"
	"fmt"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

// AuthorizeUploadProject retains the current ownership and active-project write
// locks in the supplied transaction before returning the rendering aspect ratio.
func (s *ProjectContentAccessStore) AuthorizeUploadProject(ctx context.Context, actor identity.Principal, projectID uuid.UUID) (string, error) {
	if _, err := s.Authorize(ctx, actor, projectID, true); err != nil {
		return "", err
	}
	// Authorize already holds UPDATE, including at body admission. Reading the
	// aspect does not release or downgrade that lock, or start another transaction.
	var row struct{ AspectRatio string }
	read := s.tx.WithContext(ctx).Table("workspace.project").Select("aspect_ratio").Where("id = ? AND org_id = ?", projectID, actor.OrgID).Take(&row)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return "", workspacesource.ErrProjectNotFound
	}
	if read.Error != nil {
		return "", fmt.Errorf("read upload project aspect: %w", read.Error)
	}
	return row.AspectRatio, nil
}

// AwaitUploadProjectCommit waits for a possible media commit without granting
// new content access. Deleted or inactive projects still serialize cleanup, as
// revoked callers can have committed assets whose objects must not be removed.
func (s *ProjectContentAccessStore) AwaitUploadProjectCommit(ctx context.Context, projectID uuid.UUID) error {
	if s == nil || s.tx == nil || s.tx.Statement == nil || projectID == uuid.Nil {
		return workspacesource.ErrProjectDependencyUnavailable
	}
	if _, transaction := s.tx.Statement.ConnPool.(gorm.TxCommitter); !transaction {
		return workspacesource.ErrProjectDependencyUnavailable
	}
	var row struct{ ID uuid.UUID }
	read := s.tx.WithContext(ctx).Table("workspace.project").Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", projectID).Take(&row)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return workspacesource.ErrProjectDependencyUnavailable
	}
	if read.Error != nil {
		return fmt.Errorf("await project upload commit: %w", read.Error)
	}
	return nil
}
