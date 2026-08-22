package database

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type workspaceContextKey struct{}

// WithWorkspaceID binds the authenticated workspace to the current use-case
// context. A nil UUID is deliberately not treated as a valid tenant.
func WithWorkspaceID(ctx context.Context, workspaceID uuid.UUID) context.Context {
	return context.WithValue(ctx, workspaceContextKey{}, workspaceID)
}

func WorkspaceID(ctx context.Context) (uuid.UUID, bool) {
	workspaceID, ok := ctx.Value(workspaceContextKey{}).(uuid.UUID)
	return workspaceID, ok && workspaceID != uuid.Nil
}

// WithWorkspaceTransaction establishes the PostgreSQL transaction-local tenant
// setting before any module repository query runs. It prevents a pooled
// connection from retaining one request's workspace for the next request.
func WithWorkspaceTransaction(ctx context.Context, orm *gorm.DB, fn func(*gorm.DB) error) error {
	if orm == nil {
		return fmt.Errorf("database ORM is not configured")
	}
	workspaceID, ok := WorkspaceID(ctx)
	if !ok {
		return fmt.Errorf("workspace context is missing")
	}
	if fn == nil {
		return fmt.Errorf("database transaction callback is nil")
	}
	return orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config(?, ?, true)", "app.workspace_id", workspaceID.String()).Error; err != nil {
			return fmt.Errorf("set workspace database context: %w", err)
		}
		return fn(tx.WithContext(ctx))
	})
}
