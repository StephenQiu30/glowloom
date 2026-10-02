package postgres

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// requireNewMediaVisible is for new ordinary bindings, not previously frozen
// immutable evidence. Callers hold project then library locks before originals.
func requireNewMediaVisible(tx *gorm.DB, library uuid.UUID, ids []uuid.UUID) error {
	var rows []struct {
		ID           uuid.UUID
		CatalogState string
		Purged       bool
	}
	if err := tx.Raw(`SELECT id,catalog_state,purged_at IS NOT NULL AS purged FROM media.library_item WHERE library_id=? AND id IN ? ORDER BY id FOR SHARE`, library, ids).Scan(&rows).Error; err != nil {
		return fmt.Errorf("%w: check new media visibility: %w", application.ErrUnavailable, err)
	}
	for _, row := range rows {
		if row.CatalogState != "active" || row.Purged {
			return application.ErrNotFound
		}
	}
	return requireNoNewMediaPurge(tx, ids)
}

func requireNoNewMediaPurge(tx *gorm.DB, ids []uuid.UUID) error {
	if err := requireNoPurgeReservation(tx, ids); err != nil {
		if errors.Is(err, domain.ErrPurgeConflict) {
			return application.ErrNotFound
		}
		return fmt.Errorf("%w: check new media reservation: %w", application.ErrUnavailable, err)
	}
	return nil
}

func projectMediaLibrary(tx *gorm.DB, actor identityapp.Principal, project uuid.UUID) (libraryRow, error) {
	if err := requireProject(tx, actor, project, false); err != nil {
		return libraryRow{}, err
	}
	return readLibrary(tx, actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, false)
}
