package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// DocumentSourceStore retains source locks in its injected admission transaction.
type DocumentSourceStore struct{ *Store }

// NewDocumentSourceStore injects the script owner's transaction or a read database.
func NewDocumentSourceStore(db *gorm.DB) *DocumentSourceStore {
	return &DocumentSourceStore{Store: NewStore(db)}
}

// FindAsset reads an existing exact frozen original. Catalog hiding does not
// invalidate immutable source bytes; new source admission uses Freeze below.
func (s *DocumentSourceStore) FindAsset(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (domain.MediaAsset, error) {
	if s == nil || s.Store == nil {
		return domain.MediaAsset{}, ErrUnavailable
	}
	return s.findAsset(ctx, actor, project, id, false)
}

// FreezeDocumentSources rechecks current actor/project and locks all source assets.
func (s *DocumentSourceStore) FreezeDocumentSources(ctx context.Context, actor identityapp.Principal, project uuid.UUID, ids []uuid.UUID) ([]domain.MediaAsset, error) {
	if s == nil || s.Store == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if project == uuid.Nil || len(ids) < 1 || len(ids) > 200 {
		return nil, application.ErrInvalidQuery
	}
	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if id == uuid.Nil || seen[id] {
			return nil, application.ErrInvalidQuery
		}
		seen[id] = true
	}
	var rows []assetRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := requireProject(tx, actor, project, false); err != nil {
			return err
		}
		library, err := readLibrary(tx, actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, false)
		if err != nil {
			return err
		}
		if err := requireNewMediaVisible(tx, library.ID, ids); err != nil {
			return err
		}
		read := tx.Raw(`SELECT * FROM media.media_asset WHERE project_id=? AND id IN ? ORDER BY id FOR SHARE`, project, ids).Scan(&rows)
		if read.Error != nil {
			return fmt.Errorf("freeze document source rows: %w", read.Error)
		}
		if len(rows) != len(ids) {
			return ErrNotFound
		}
		for _, row := range rows {
			asset := row.domain()
			if asset.Validate() != nil {
				return application.ErrUnavailable
			}
			if asset.Kind != domain.KindDocument || !asset.CanReference() || asset.ContainsRealPerson || asset.ConsentRecordID != nil {
				return ErrNotFound
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]domain.MediaAsset, len(rows))
	for _, row := range rows {
		byID[row.ID] = row.domain()
	}
	assets := make([]domain.MediaAsset, len(ids))
	for i, id := range ids {
		assets[i] = byID[id]
	}
	return assets, nil
}
