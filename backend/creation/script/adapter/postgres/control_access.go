package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/script/application"
	workspacedomain "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"
)

// readWithControls derives action flags from the same live project authority as
// commands. Try the strongest lock first so a read never upgrades FOR SHARE.
func readWithControls(ctx context.Context, db *gorm.DB, factory ProjectAccessFactory, actor application.Actor, project uuid.UUID, use func(*gorm.DB, bool) error) error {
	if db == nil || factory == nil {
		return application.ErrUnavailable
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		access := factory(tx)
		if access == nil {
			return application.ErrUnavailable
		}
		facts, err := access.Authorize(ctx, actor, project, true)
		writable := err == nil
		if err != nil {
			if !errors.Is(err, application.ErrNotFound) && !errors.Is(err, workspacedomain.ErrProjectStateConflict) {
				return err
			}
			facts, err = access.Authorize(ctx, actor, project, false)
		}
		if err != nil {
			return err
		}
		if facts.ProjectID != project || facts.OrgID != actor.OrgID {
			return application.ErrUnavailable
		}
		return use(tx, writable)
	})
}
