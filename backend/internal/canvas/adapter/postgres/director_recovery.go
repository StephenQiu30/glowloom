package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func (s *Store) authorizeDirectorRecovery(tx *gorm.DB, a identity.Principal, c, n uuid.UUID, write bool) error {
	project, err := documentProject(tx, c)
	if err != nil {
		return err
	}
	if err = s.authorize(tx, a, project, write); err != nil {
		return err
	}
	var found int
	result := tx.Raw(`SELECT 1 FROM canvas.node WHERE document_id=? AND id=? AND node_type='director' AND NOT is_delete FOR SHARE`, c, n).Scan(&found)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return application.ErrNotFound
	}
	return nil
}

// ReadDirectorRecovery does not reveal another account's private retained edit.
func (s *Store) ReadDirectorRecovery(ctx context.Context, a identity.Principal, c, n uuid.UUID) (application.DirectorRecoveryResult, error) {
	var result application.DirectorRecoveryResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorizeDirectorRecovery(tx, a, c, n, false); err != nil {
			return err
		}
		var row struct {
			Revision        int64
			Epoch           uuid.UUID
			BaseFingerprint string
			Director        []byte
			UpdatedAt       time.Time
		}
		read := tx.Raw(`SELECT revision,epoch,base_fingerprint,director,updated_at FROM canvas.director_recovery WHERE actor_id=? AND document_id=? AND node_id=? AND updated_at>clock_timestamp()-interval '30 days'`, a.ID, c, n).Scan(&row)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected == 0 {
			return nil
		}
		var config domain.DirectorConfig
		if err := json.Unmarshal(row.Director, &config); err != nil {
			return err
		}
		if err := config.Validate(); err != nil {
			return err
		}
		result.Draft = &application.DirectorRecovery{Revision: row.Revision, Epoch: row.Epoch, BaseFingerprint: row.BaseFingerprint, Director: config, UpdatedAt: row.UpdatedAt}
		return nil
	})
	return result, err
}

// SaveDirectorRecovery retains at most fifty private scenes per actor with CAS.
func (s *Store) SaveDirectorRecovery(ctx context.Context, a identity.Principal, c, n uuid.UUID, input application.SaveDirectorRecoveryInput) (application.DirectorRecovery, error) {
	result := application.DirectorRecovery{}
	raw, err := json.Marshal(input.Director)
	if err != nil {
		return result, err
	}
	if len(raw) > 512<<10 {
		return result, domain.ErrInvalidCommand
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorizeDirectorRecovery(tx, a, c, n, true); err != nil {
			return err
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 1730))`, a.ID.String()).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM canvas.director_recovery WHERE actor_id=? AND updated_at<=clock_timestamp()-interval '30 days'`, a.ID).Error; err != nil {
			return err
		}
		var current int64
		if err := tx.Raw(`SELECT revision FROM canvas.director_recovery WHERE actor_id=? AND document_id=? AND node_id=? FOR UPDATE`, a.ID, c, n).Scan(&current).Error; err != nil {
			return err
		}
		if current != input.ExpectedRevision {
			return &application.RevisionConflict{CurrentRevision: current}
		}
		if current == 0 {
			var count int64
			if err := tx.Raw(`SELECT count(*) FROM canvas.director_recovery WHERE actor_id=?`, a.ID).Scan(&count).Error; err != nil {
				return err
			}
			if count >= 50 {
				return domain.ErrInvalidCommand
			}
		}
		result = application.DirectorRecovery{Revision: 0, Epoch: input.Epoch, BaseFingerprint: input.BaseFingerprint, Director: input.Director}
		var receipt struct {
			Revision  int64
			UpdatedAt time.Time
		}
		err := tx.Raw(`INSERT INTO canvas.director_recovery(actor_id,document_id,node_id,revision,epoch,base_fingerprint,director) VALUES(?,?,?,nextval('canvas.director_recovery_revision'),?,?,?::jsonb) ON CONFLICT(actor_id,document_id,node_id) DO UPDATE SET revision=excluded.revision,epoch=excluded.epoch,base_fingerprint=excluded.base_fingerprint,director=excluded.director,updated_at=clock_timestamp() RETURNING revision,updated_at`, a.ID, c, n, input.Epoch, input.BaseFingerprint, string(raw)).Scan(&receipt).Error
		result.Revision = receipt.Revision
		result.UpdatedAt = receipt.UpdatedAt
		return err
	})
	return result, err
}

// DeleteDirectorRecovery preserves a draft replaced concurrently in another tab.
func (s *Store) DeleteDirectorRecovery(ctx context.Context, a identity.Principal, c, n uuid.UUID, revision int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorizeDirectorRecovery(tx, a, c, n, true); err != nil {
			return err
		}
		result := tx.Exec(`DELETE FROM canvas.director_recovery WHERE actor_id=? AND document_id=? AND node_id=? AND revision=?`, a.ID, c, n, revision)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return &application.RevisionConflict{}
		}
		return nil
	})
}
