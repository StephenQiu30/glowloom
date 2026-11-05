package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

type exportLaunchRow struct {
	TokenHash  string `gorm:"primaryKey"`
	JobID      uuid.UUID
	ArtifactID uuid.UUID
	ProjectID  uuid.UUID
	Revision   int64
	Actor      jsonDocument
	Envelope   jsonDocument
	ExpiresAt  time.Time
}

func (exportLaunchRow) TableName() string { return "mediatool.export_launch" }

// SaveLaunch rechecks authority and the final artifact inside the write transaction.
func (s *Store) SaveLaunch(ctx context.Context, r application.LaunchReceipt) error {
	if len(r.TokenHash) != 64 || len(r.Body) > 16384 || !r.ExpiresAt.After(time.Now()) || r.ExpiresAt.After(time.Now().Add(10*time.Minute)) {
		return application.ErrInvalidExport
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.authorize(ctx, tx, r.Actor, r.ProjectID, false); e != nil {
			return e
		}
		job, e := readJob(tx, r.ProjectID, r.JobID, true)
		if e != nil {
			return e
		}
		if job.Status != domain.Succeeded || job.OutputKind != "jianying" || job.Revision != r.Revision || job.ArtifactID == nil || *job.ArtifactID != r.ArtifactID {
			return application.ErrConflict
		}
		if e = s.validateSources(ctx, tx, r.Actor, job); e != nil {
			return e
		}
		if _, e = s.readArtifact(ctx, tx, job); e != nil {
			return e
		}
		actor, e := json.Marshal(r.Actor)
		if e != nil {
			return e
		}
		return tx.Create(&exportLaunchRow{
			TokenHash: r.TokenHash, JobID: r.JobID, ArtifactID: r.ArtifactID, ProjectID: r.ProjectID,
			Revision: r.Revision, Actor: jsonDocument(actor), Envelope: jsonDocument(r.Body), ExpiresAt: r.ExpiresAt,
		}).Error
	})
}

// ReadLaunch reads the authorized launch intent for a saved export job.
func (s *Store) ReadLaunch(ctx context.Context, hash string, now time.Time) ([]byte, error) {
	var body []byte
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var r exportLaunchRow
		if err := tx.Where("token_hash = ? AND expires_at > ?", hash, now).Take(&r).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return application.ErrNotFound
			}
			return err
		}
		var actor identity.Principal
		if json.Unmarshal(r.Actor, &actor) != nil {
			return application.ErrNotFound
		}
		if e := s.authorize(ctx, tx, actor, r.ProjectID, false); e != nil {
			return application.ErrNotFound
		}
		job, e := readJob(tx, r.ProjectID, r.JobID, false)
		if e != nil {
			return application.ErrNotFound
		}
		if job.Status != domain.Succeeded || job.OutputKind != "jianying" || job.Revision != r.Revision || job.ArtifactID == nil || *job.ArtifactID != r.ArtifactID {
			return application.ErrNotFound
		}
		if e = s.validateSources(ctx, tx, actor, job); e != nil {
			return application.ErrNotFound
		}
		if _, e = s.readArtifact(ctx, tx, job); e != nil {
			return application.ErrNotFound
		}
		if !r.ExpiresAt.After(time.Now().UTC()) {
			return application.ErrNotFound
		}
		body = r.Envelope
		return nil
	})
	return body, err
}

// PruneLaunches participates in bounded background receipt cleanup.
func (s *Store) PruneLaunches(ctx context.Context, now time.Time, limit int) (int64, error) {
	if limit < 1 || limit > 1000 {
		return 0, application.ErrInvalidExport
	}
	var hashes []string
	if err := s.db.WithContext(ctx).Model(&exportLaunchRow{}).
		Where("expires_at <= ?", now).Order("expires_at ASC").Limit(limit).Pluck("token_hash", &hashes).Error; err != nil {
		return 0, err
	}
	if len(hashes) == 0 {
		return 0, nil
	}
	result := s.db.WithContext(ctx).Where("token_hash IN ?", hashes).Delete(&exportLaunchRow{})
	return result.RowsAffected, result.Error
}
