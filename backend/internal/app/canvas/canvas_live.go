// Package canvas wires live canvas authorization.
package canvas

import (
	"context"
	"errors"
	"net/http"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvaslive "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/websocket"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	workspacedomain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// SocketAuthority authenticates a canvas socket and resolves its creation scope.
func SocketAuthority(db *gorm.DB, auth *identityhttp.EmailAuthHandler) canvaslive.Authorize {
	return func(ctx context.Context, r *http.Request, document, scope uuid.UUID, csrf string) (identity.Principal, bool, error) {
		actor, err := auth.AuthenticateSocket(ctx, r, csrf)
		if err != nil {
			return actor, false, err
		}
		writable := false
		err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			selected, err := scopewire.ProvideRequestScopeAccess(tx).Resolve(ctx, actor, scope, false)
			if err != nil {
				return socketScopeError(err)
			}
			actor.ScopeID, actor.OrgID = selected.ID, selected.PhysicalScopeID
			var row struct{ ProjectID uuid.UUID }
			found := tx.Raw(`SELECT project_id FROM canvas.document WHERE id=? AND NOT is_delete`, document).Scan(&row)
			if found.Error != nil {
				return found.Error
			}
			if found.RowsAffected != 1 {
				return canvasapp.ErrNotFound
			}
			// Request the strongest project lock first. Taking SHARE and then
			// upgrading to UPDATE deadlocks with concurrent project writers.
			_, err = scopewire.ProvideProjectContentAccess(tx).Authorize(ctx, actor, row.ProjectID, true)
			if err == nil {
				writable = true
				return nil
			}
			if !errors.Is(err, workspace.ErrScopeNotFound) && !errors.Is(err, workspacedomain.ErrProjectStateConflict) && !errors.Is(err, identity.ErrForbidden) {
				return socketScopeError(err)
			}
			// Archived/read-only access still needs its own positive read check.
			_, err = scopewire.ProvideProjectContentAccess(tx).Authorize(ctx, actor, row.ProjectID, false)
			return socketScopeError(err)
		})
		return actor, writable, err
	}
}
func socketScopeError(err error) error {
	if errors.Is(err, workspace.ErrScopeNotFound) || errors.Is(err, workspace.ErrProjectNotFound) {
		return canvasapp.ErrNotFound
	}
	return err
}
