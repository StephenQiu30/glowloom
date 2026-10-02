package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
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
		return application.ErrProjectDependencyUnavailable
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
