package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// CheckActive rechecks availability without granting any content or team role.
func (s *Store) CheckActive(ctx context.Context, id uuid.UUID) error {
	var present int
	r := s.db.WithContext(ctx).Raw(`SELECT 1 FROM identity."user" WHERE id=? AND status='active' AND NOT is_delete AND NOT must_change_password AND email_key IS NOT NULL AND email_verified_at IS NOT NULL FOR SHARE`, id).Scan(&present)
	if r.Error != nil {
		return fmt.Errorf("read actor availability: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return application.ErrForbidden
	}
	return nil
}
