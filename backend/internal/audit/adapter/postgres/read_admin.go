package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	"github.com/StephenQiu30/lanverse/backend/internal/audit/domain"
)

// ListForAdmin locks the actor's current account until the scoped page is read.
func (s *Store) ListForAdmin(ctx context.Context, actorID, orgID uuid.UUID, filter domain.Filter) (domain.Page, error) {
	if s == nil || s.db == nil {
		return domain.Page{}, application.ErrInvalidRead
	}
	var page domain.Page
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentAuditAdmin(tx, actorID, orgID); err != nil {
			return err
		}
		var err error
		page, err = NewStore(tx).List(ctx, orgID, filter)
		return err
	})
	if err != nil {
		return domain.Page{}, fmt.Errorf("read administrator audit page: %w", err)
	}
	return page, nil
}

// GetForAdmin locks the actor's current account until the scoped detail is read.
func (s *Store) GetForAdmin(ctx context.Context, actorID, orgID, recordID uuid.UUID) (domain.Record, error) {
	if s == nil || s.db == nil {
		return domain.Record{}, application.ErrInvalidRead
	}
	var record domain.Record
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentAuditAdmin(tx, actorID, orgID); err != nil {
			return err
		}
		var err error
		record, err = NewStore(tx).Get(ctx, orgID, recordID)
		return err
	})
	if err != nil {
		return domain.Record{}, fmt.Errorf("read administrator audit detail: %w", err)
	}
	return record, nil
}

func requireCurrentAuditAdmin(tx *gorm.DB, actorID, orgID uuid.UUID) error {
	if actorID == uuid.Nil || orgID == uuid.Nil {
		return application.ErrForbidden
	}
	var authorized int
	result := tx.Raw(`
		SELECT 1 FROM identity."user"
		WHERE id = ?::uuid AND org_id = ?::uuid AND role = 'admin'
		  AND status = 'active' AND NOT must_change_password AND NOT is_delete
		FOR SHARE
	`, actorID.String(), orgID.String()).Scan(&authorized)
	if result.Error != nil {
		return fmt.Errorf("check current audit administrator: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return application.ErrForbidden
	}
	return nil
}

var _ application.ReadStore = (*Store)(nil)
