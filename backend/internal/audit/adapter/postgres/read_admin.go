package postgres

import (
	"context"
	"fmt"
	"time"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/audit/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/audit/domain"
)

// ListForAdmin locks the actor's current account until the scoped page is read.
func (s *Store) ListForAdmin(ctx context.Context, actor identityapp.Principal, orgID uuid.UUID, filter domain.Filter) (domain.Page, error) {
	if s == nil || s.db == nil {
		return domain.Page{}, application.ErrInvalidRead
	}
	var page domain.Page
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentAuditAdmin(tx, actor); err != nil {
			return err
		}
		var err error
		page, err = NewStore(tx).List(ctx, orgID, filter)
		return err
	})
	if err != nil {
		return domain.Page{}, fmt.Errorf("read administrator audit page: %w", err)
	}
	for i := range page.Items {
		page.Items[i] = metadata(page.Items[i])
	}
	return page, nil
}

// GetForAdmin locks the actor's current account until the scoped detail is read.
func (s *Store) GetForAdmin(ctx context.Context, actor identityapp.Principal, orgID, recordID uuid.UUID) (domain.Record, error) {
	if s == nil || s.db == nil {
		return domain.Record{}, application.ErrInvalidRead
	}
	var record domain.Record
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentAuditAdmin(tx, actor); err != nil {
			return err
		}
		var err error
		record, err = NewStore(tx).Get(ctx, orgID, recordID)
		return err
	})
	if err != nil {
		return domain.Record{}, fmt.Errorf("read administrator audit detail: %w", err)
	}
	return metadata(record), nil
}

func (s *Store) requireCurrentAuditAdmin(tx *gorm.DB, actor identityapp.Principal) error {
	if s.actors == nil || s.actors(tx) == nil {
		return application.ErrForbidden
	}
	return s.actors(tx).CheckPlatformSession(tx.Statement.Context, actor, time.Now)
}

// NewScopedStore injects current platform-session authority for metadata reads.
func NewScopedStore(db *gorm.DB, actors func(*gorm.DB) application.ActorAccess) *Store {
	return &Store{db: db, actors: actors}
}

func metadata(record domain.Record) domain.Record {
	record.Before = nil
	record.After = nil
	record.IP = ""
	return record
}

var _ application.ReadStore = (*Store)(nil)
