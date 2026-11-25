package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application"
	workspacesource "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/domain"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ProjectCoverFactory binds the media owner's reference locks to the project transaction.
type ProjectCoverFactory func(*gorm.DB) application.ProjectCoverReference

// NewStoreWithProjectCover injects project lifecycle guards and cover ownership evidence.
func NewStoreWithProjectCover(db *gorm.DB, work ProjectWorkFactory, cover ProjectCoverFactory) *Store {
	return &Store{db: db, work: work, cover: cover}
}

type projectCoverPatch struct{ AssetID *uuid.UUID }

func (s *Store) requireProjectCover(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, project domain.Project) error {
	if project.CoverAssetID == nil {
		return nil
	}
	if s.cover == nil {
		return workspacesource.ErrProjectDependencyUnavailable
	}
	return application.FreezeProjectCover(ctx, s.cover(tx), actor, project.ID, project.CoverAssetID)
}

func (s *Store) projectCoverUnavailable(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, project domain.Project) (bool, error) {
	err := s.requireProjectCover(ctx, tx, actor, project)
	if errors.Is(err, application.ErrProjectCoverUnavailable) {
		return true, nil
	}
	return false, err
}
