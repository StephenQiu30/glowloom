package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// SubjectSourceStore loads new adoptions through their library owner. Bound
// reads must be composed after a subject owner selects the exact immutable pin.
type SubjectSourceStore struct {
	db      *gorm.DB
	library *LibraryStore
}

// NewSubjectSourceStore requires a caller-owned transaction and library authority.
func NewSubjectSourceStore(tx *gorm.DB, library *LibraryStore) *SubjectSourceStore {
	return &SubjectSourceStore{db: tx, library: library}
}

func (s *SubjectSourceStore) inTransaction() bool {
	if s == nil || s.db == nil || s.db.Statement == nil {
		return false
	}
	_, ok := s.db.Statement.ConnPool.(gorm.TxCommitter)
	return ok
}

// FindSubjectSource rejects trashed/unavailable entries before the ordinary
// current-owner media loader verifies all private file and rendition facts.
func (s *SubjectSourceStore) FindSubjectSource(ctx context.Context, actor identity.Principal, scope domain.LibraryScope, id uuid.UUID) (application.LibraryMediaFile, error) {
	if !s.inTransaction() || s.library == nil || id == uuid.Nil {
		return application.LibraryMediaFile{}, application.ErrUnavailable
	}
	// A formal selection names the asset, not a potentially different legacy
	// metadata row. Keep the owning transaction and active-library authorization.
	bound := *s.library
	bound.db = s.db
	return NewSubjectCatalogueStore(s.db, &bound, nil).FindLibraryMedia(ctx, actor, scope, id)
}

// ReadBoundSubjectSource accepts one server-selected pin, never client authority.
// Historical subjects retain their media even when it leaves the current catalog.
func (s *SubjectSourceStore) ReadBoundSubjectSource(ctx context.Context, id uuid.UUID) (application.LibraryMediaFile, error) {
	if !s.inTransaction() || id == uuid.Nil {
		return application.LibraryMediaFile{}, application.ErrUnavailable
	}
	if err := requireNoNewMediaPurge(s.db, []uuid.UUID{id}); err != nil {
		return application.LibraryMediaFile{}, err
	}
	var row assetRow
	r := s.db.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Where(map[string]any{"id": id, "is_delete": false, "status": domain.StatusReady, "moderation_status": domain.ModerationPassed, "contains_real_person": false, "consent_record_id": nil}).Take(&row)
	if r.Error != nil {
		if errors.Is(r.Error, gorm.ErrRecordNotFound) {
			return application.LibraryMediaFile{}, application.ErrNotFound
		}
		return application.LibraryMediaFile{}, r.Error
	}
	if err := row.loadLocation(s.db); err != nil {
		return application.LibraryMediaFile{}, err
	}
	a := row.domain()
	if a.Validate() != nil {
		return application.LibraryMediaFile{}, application.ErrUnavailable
	}
	return application.LibraryMediaFile{Asset: a, Renditions: []domain.Rendition{}}, nil
}
