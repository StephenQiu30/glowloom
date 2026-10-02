package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// LibraryProjectAccessFactory binds the workspace consumer to the same transaction.
type LibraryProjectAccessFactory func(*gorm.DB) application.LibraryProjectAccess

// LibraryStore owns catalog metadata and permanent commands, not media execution.
type LibraryStore struct {
	db      *gorm.DB
	project LibraryProjectAccessFactory
	clock   func() time.Time
}

// NewLibraryStore explicitly injects workspace authorization and its clock.
func NewLibraryStore(db *gorm.DB, project LibraryProjectAccessFactory, clock func() time.Time) *LibraryStore {
	if clock == nil {
		clock = time.Now
	}
	return &LibraryStore{db: db, project: project, clock: clock}
}

type libraryRow struct {
	ID              uuid.UUID
	Kind            string
	OrgID           uuid.UUID
	ProjectID       *uuid.UUID
	PersonalActorID *uuid.UUID
	Revision        int64
}

func (s *LibraryStore) authorize(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, scope domain.LibraryScope, write bool) (application.LibraryProjectFacts, error) {
	if scope.Kind == domain.LibraryPersonal {
		return application.LibraryProjectFacts{}, nil
	}
	if s.project == nil {
		return application.LibraryProjectFacts{}, application.ErrUnavailable
	}
	access := s.project(tx)
	if access == nil {
		return application.LibraryProjectFacts{}, application.ErrUnavailable
	}
	facts, err := access.Authorize(ctx, actor, *scope.ProjectID, write)
	if err != nil {
		return application.LibraryProjectFacts{}, err
	}
	if facts.ProjectID != *scope.ProjectID || facts.OrgID != actor.OrgID || facts.Revision < 1 {
		return application.LibraryProjectFacts{}, application.ErrUnavailable
	}
	return facts, nil
}

func readLibrary(tx *gorm.DB, actor identityapp.Principal, scope domain.LibraryScope, write bool) (libraryRow, error) {
	id, err := scope.Identity(actor.OrgID, actor.ID)
	if err != nil {
		return libraryRow{}, err
	}
	lock := " FOR SHARE"
	if write {
		lock = " FOR UPDATE"
	}
	var row libraryRow
	read := tx.Raw(`SELECT id,kind,org_id,project_id,personal_actor_id,revision FROM media.library WHERE id=?`+lock, id).Scan(&row)
	if read.Error != nil {
		return libraryRow{}, fmt.Errorf("read media library: %w", read.Error)
	}
	if read.RowsAffected == 0 {
		return libraryRow{ID: id, Kind: string(scope.Kind), OrgID: actor.OrgID, ProjectID: scope.ProjectID}, nil
	}
	if row.ID != id || row.Kind != string(scope.Kind) || row.OrgID != actor.OrgID || row.Revision < 0 ||
		scope.Kind == domain.LibraryProject && (row.ProjectID == nil || *row.ProjectID != *scope.ProjectID || row.PersonalActorID != nil) ||
		scope.Kind == domain.LibraryPersonal && (row.ProjectID != nil || row.PersonalActorID == nil || *row.PersonalActorID != actor.ID) {
		return libraryRow{}, application.ErrUnavailable
	}
	return row, nil
}

type libraryFolderRow struct {
	ID          uuid.UUID
	LibraryID   uuid.UUID
	LibraryKind string
	ParentID    *uuid.UUID
	Name        string
	Position    int64
	Style       string
	Theme       string
	Revision    int64
	CreateTime  time.Time
	UpdateTime  time.Time
}

func libraryFolders(tx *gorm.DB, id uuid.UUID, write bool) ([]domain.LibraryFolder, error) {
	lock := " FOR SHARE"
	if write {
		lock = " FOR UPDATE"
	}
	var rows []libraryFolderRow
	if err := tx.Raw(`SELECT * FROM media.library_folder WHERE library_id=? ORDER BY id LIMIT 4097`+lock, id).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read media library folders: %w", err)
	}
	if len(rows) > 4096 {
		return nil, application.ErrUnavailable
	}
	folders := make([]domain.LibraryFolder, 0, len(rows))
	for _, row := range rows {
		folder := domain.LibraryFolder{ID: row.ID, LibraryID: row.LibraryID, Kind: domain.LibraryKind(row.LibraryKind), ParentID: row.ParentID, Name: row.Name, Position: row.Position, Style: row.Style, Theme: row.Theme, Revision: row.Revision, CreatedAt: row.CreateTime, UpdatedAt: row.UpdateTime}
		if folder.Validate() != nil {
			return nil, application.ErrUnavailable
		}
		folders = append(folders, folder)
	}
	// Validate every existing ancestry. A corrupt tree is never presented as a
	// valid partial tree or silently repaired by an unrelated command.
	if len(folders) != 0 && domain.ValidateLibraryFolderPlacement(folders[0], folders) != nil {
		return nil, application.ErrUnavailable
	}
	return folders, nil
}

func libraryChanged(tx *gorm.DB, query string, args ...any) error {
	result := tx.Exec(query, args...)
	if result.Error != nil {
		return fmt.Errorf("persist media library change: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return application.ErrLibraryConflict
	}
	return nil
}

func (s *LibraryStore) read(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, consume func(*gorm.DB, libraryRow) error) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	if scope.Validate() != nil {
		return domain.ErrInvalidLibrary
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if _, err := s.authorize(ctx, tx, actor, scope, false); err != nil {
			return err
		}
		row, err := readLibrary(tx, actor, scope, false)
		if err != nil {
			return err
		}
		return consume(tx, row)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
}
