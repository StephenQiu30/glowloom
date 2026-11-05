package postgres

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// requireNewMediaVisible is for new ordinary bindings, not previously frozen
// immutable evidence. Callers hold project then library locks before originals.
func requireNewMediaVisible(tx *gorm.DB, library uuid.UUID, ids []uuid.UUID) error {
	var rows []libraryItemRow
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Select("id, catalog_state, purged_at").Where("library_id = ? AND id IN ?", library, ids).Order("id").Find(&rows).Error; err != nil {
		return fmt.Errorf("%w: check new media visibility: %w", application.ErrUnavailable, err)
	}
	for _, row := range rows {
		if row.CatalogState != domain.LibraryItemStateActive || row.PurgedAt != nil {
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

func (s scopeAuthority) projectMediaLibrary(tx *gorm.DB, actor identityapp.Principal, project uuid.UUID) (libraryRow, error) {
	if err := s.requireProject(tx, actor, project, false); err != nil {
		return libraryRow{}, err
	}
	return readLibrary(tx, actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, false)
}
