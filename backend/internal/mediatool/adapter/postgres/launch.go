package postgres

import (
	"context"
	"encoding/json"
	"time"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

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
		if job.Status != "succeeded" || job.OutputKind != "jianying" || job.Revision != r.Revision || job.ArtifactID == nil || *job.ArtifactID != r.ArtifactID {
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
		return tx.Exec(`INSERT INTO mediatool.export_launch(token_hash,job_id,artifact_id,project_id,revision,actor,envelope,expires_at) VALUES(?,?,?,?,?,?::jsonb,?::jsonb,?)`, r.TokenHash, r.JobID, r.ArtifactID, r.ProjectID, r.Revision, string(actor), string(r.Body), r.ExpiresAt).Error
	})
}
func (s *Store) ReadLaunch(ctx context.Context, hash string, now time.Time) ([]byte, error) {
	var body []byte
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var r struct {
			JobID, ArtifactID, ProjectID uuid.UUID
			Revision                     int64
			Actor, Envelope              []byte
			ExpiresAt                    time.Time
		}
		result := tx.Raw(`SELECT job_id,artifact_id,project_id,revision,actor,envelope,expires_at FROM mediatool.export_launch WHERE token_hash=? AND expires_at>?`, hash, now).Scan(&r)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return application.ErrNotFound
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
		if job.Status != "succeeded" || job.OutputKind != "jianying" || job.Revision != r.Revision || job.ArtifactID == nil || *job.ArtifactID != r.ArtifactID {
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
	r := s.db.WithContext(ctx).Exec(`DELETE FROM mediatool.export_launch WHERE token_hash IN (SELECT token_hash FROM mediatool.export_launch WHERE expires_at<=? ORDER BY expires_at LIMIT ?)`, now, limit)
	return r.RowsAffected, r.Error
}
