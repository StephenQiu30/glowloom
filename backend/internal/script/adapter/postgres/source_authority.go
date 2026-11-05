package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// PublicationAuthorizationFactory binds a producer's durable authority port to
// the same transaction; script never imports the producer's implementation.
type PublicationAuthorizationFactory func(*gorm.DB) application.PublicationAuthorization

// WithPublicationAuthorization injects the owning publication authorization callback.
func (s *SourceStore) WithPublicationAuthorization(f PublicationAuthorizationFactory) *SourceStore {
	s.publication = f
	return s
}
func (s *SourceStore) commandTransaction(ctx context.Context, actor identity.Principal, project, key uuid.UUID, hash string, write bool, f func(*gorm.DB, application.ProjectAccess, workspace.ProjectContentAccess) error) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if s.publication != nil {
			authority := s.publication(tx)
			if authority == nil {
				return application.ErrUnavailable
			}
			if err := authority.AuthorizeSourcePublication(ctx, actor, project, key, hash); err != nil {
				return err
			}
		}
		// Producer/root locks precede workspace and source locks. The nested owner
		// transaction uses a savepoint and keeps the original locks until commit.
		scoped := *s
		scoped.db = tx
		return scoped.transaction(ctx, actor, project, write, f)
	})
}
