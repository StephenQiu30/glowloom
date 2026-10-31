package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// CanvasUsageFactory binds billing reads to the current authorized transaction.
type CanvasUsageFactory func(*gorm.DB) application.CanvasUsageReader

// WithCanvasUsage supplies the accounting owner for the information read model.
func (s *Store) WithCanvasUsage(factory CanvasUsageFactory) *Store { s.usage = factory; return s }

// Information reads document metadata and charges from one repeatable-read snapshot.
func (s *Store) Information(ctx context.Context, a identity.Principal, id uuid.UUID, page int) (application.CanvasInformation, error) {
	var result application.CanvasInformation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		project, err := documentProject(tx, id)
		if err != nil {
			return err
		}
		if err = s.authorize(tx, a, project, false); err != nil {
			return err
		}
		presentation, err := s.presentation(tx, a, project)
		if err != nil {
			return err
		}
		var metadata struct {
			ID, ProjectID        uuid.UUID
			Revision             int64
			CreatedAt, UpdatedAt time.Time
		}
		read := tx.Raw(`SELECT id,project_id,revision,create_time AS created_at,update_time AS updated_at FROM canvas.document WHERE id=? AND project_id=? AND NOT is_delete`, id, project).Scan(&metadata)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return application.ErrNotFound
		}
		result = application.CanvasInformation{ID: metadata.ID, ProjectID: metadata.ProjectID, Revision: application.Revision(metadata.Revision), CreatedAt: metadata.CreatedAt, UpdatedAt: metadata.UpdatedAt, OwnerName: presentation.OwnerName}
		if s.usage == nil || s.usage(tx) == nil {
			return fmt.Errorf("canvas usage reader unavailable")
		}
		result.Usage, err = s.usage(tx).ReadCanvasUsage(ctx, a, project, id, page)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	return result, err
}
