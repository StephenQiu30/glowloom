package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

type artifactRow struct {
	ID, JobID, ProjectID                                         uuid.UUID
	Attempt                                                      int
	Format, FormatVersion, FileName, MIMEType, ObjectKey, SHA256 string
	ByteSize                                                     int64
	ObjectLocationID                                             *uuid.UUID
	CreatedAt                                                    time.Time
}

func (artifactRow) TableName() string { return "mediatool.export_artifact" }

func (r artifactRow) public() domain.Artifact {
	return domain.Artifact{ID: r.ID, JobID: r.JobID, ProjectID: r.ProjectID, Attempt: r.Attempt, Format: domain.OutputKind(r.Format), FormatVersion: r.FormatVersion, FileName: r.FileName, MIMEType: r.MIMEType, ByteSize: r.ByteSize, SHA256: r.SHA256, ObjectKey: r.ObjectKey, CreatedAt: r.CreatedAt}
}
func artifactIdentity(row jobRow) (uuid.UUID, string) {
	id := uuid.NewSHA1(row.ID, []byte("export-artifact/"+strconv.Itoa(row.Attempt)))
	return id, path.Join("projects", row.ProjectID.String(), "exports", row.CreatedAt.UTC().Format("2006/01"), id.String()+".zip")
}
func validArtifact(row jobRow, a domain.Artifact) bool {
	id, key := artifactIdentity(row)
	kind := domain.OutputKind(row.OutputKind)
	return kind.IsArtifact() && a.ID == id && a.JobID == row.ID && a.ProjectID == row.ProjectID && a.Attempt == row.Attempt && a.Format == kind && a.FormatVersion == kind.FormatVersion() && a.MIMEType == "application/zip" && a.FileName == "timeline-"+row.ID.String()+".zip" && a.ObjectKey == key && a.ByteSize > 0 && a.ByteSize <= application.MaxProjectBytes && len(a.SHA256) == 64 && strings.Trim(a.SHA256, "0123456789abcdef") == "" && !a.CreatedAt.IsZero()
}
func (s *Store) readArtifact(ctx context.Context, tx *gorm.DB, row jobRow) (domain.Artifact, error) {
	if row.ArtifactID == nil || row.AssetID != nil || row.SHA256 == nil {
		return domain.Artifact{}, application.ErrConflict
	}
	var stored artifactRow
	if err := tx.Where("id = ? AND job_id = ? AND project_id = ? AND attempt = ?", *row.ArtifactID, row.ID, row.ProjectID, row.Attempt).Take(&stored).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Artifact{}, application.ErrConflict
		}
		return domain.Artifact{}, err
	}
	a := stored.public()
	if !validArtifact(row, a) || a.SHA256 != *row.SHA256 {
		return domain.Artifact{}, application.ErrConflict
	}
	if stored.ObjectLocationID != nil {
		if s.storage == nil {
			return domain.Artifact{}, application.ErrUnavailable
		}
		location, err := s.storage(tx).ReadLocation(ctx, *stored.ObjectLocationID)
		if err != nil {
			return domain.Artifact{}, err
		}
		if location.State != mediadomain.ObjectLocationAccepted || location.Ref.Key != a.ObjectKey || location.ByteSize == nil || *location.ByteSize != a.ByteSize || location.SHA256 == nil || *location.SHA256 != a.SHA256 || location.MIMEType == nil || *location.MIMEType != a.MIMEType {
			return domain.Artifact{}, mediaapp.ErrStorageIdentityConflict
		}
		a.ObjectLocation = &location
	} else if s.storage != nil {
		return domain.Artifact{}, mediaapp.ErrStorageIdentityConflict
	}
	return a, nil
}
func (s *Store) acceptArtifact(ctx context.Context, tx *gorm.DB, row jobRow, a *domain.Artifact) error {
	if s.storage == nil {
		return nil
	}
	ledger := s.storage(tx)
	owner := application.StorageOwner(mediaapp.StorageWorkKindExport, row.ProjectID, row.ID, row.Attempt)
	intent, err := ledger.ReadStorageWork(ctx, owner)
	if err != nil {
		return err
	}
	if intent.Owner != owner || intent.State != mediaapp.StorageWorkOpen {
		return mediaapp.ErrStorageIdentityConflict
	}
	location, err := ledger.ReserveWorkLocation(ctx, intent.ID, a.ObjectKey)
	if err != nil {
		return err
	}
	if location.ID == uuid.Nil || location.Ref != (mediadomain.ObjectRef{ProfileID: intent.ProfileID, Bucket: intent.Bucket, Key: a.ObjectKey}) || location.State != mediadomain.ObjectLocationPresent || location.ByteSize == nil || *location.ByteSize != a.ByteSize || location.SHA256 == nil || *location.SHA256 != a.SHA256 || location.MIMEType == nil || *location.MIMEType != a.MIMEType {
		return mediaapp.ErrStorageIdentityConflict
	}
	a.ObjectLocation = &location
	return ledger.AcceptStorageWork(ctx, intent.ID, []uuid.UUID{location.ID})
}

// CommitArtifact binds a verified immutable ZIP to the same fenced export attempt.
func (s *Store) CommitArtifact(ctx context.Context, id application.WorkID, a domain.Artifact) (domain.ExportJob, error) {
	var job domain.ExportJob
	if s == nil || s.db == nil {
		return job, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := s.lockWorkerJob(ctx, tx, id.JobID, true)
		if err != nil {
			return err
		}
		if row.Attempt != id.Attempt || row.ActiveWorker == nil || *row.ActiveWorker != id.ExecutionID || !validArtifact(row, a) {
			return application.ErrConflict
		}
		if row.Status == domain.CancelRequested || row.Status == domain.Cancelled {
			return application.ErrCancelled
		}
		if row.Status == domain.ReviewRequired || row.Status == domain.Succeeded {
			existing, e := s.readArtifact(ctx, tx, row)
			if e != nil || existing.ID != a.ID || existing.SHA256 != a.SHA256 {
				return application.ErrConflict
			}
			job = row.public()
			return nil
		}
		if row.Status != domain.Running {
			return application.ErrConflict
		}
		if err = s.validateSources(ctx, tx, row.actor(), row); err != nil {
			return err
		}
		if err = s.acceptArtifact(ctx, tx, row, &a); err != nil {
			return err
		}
		var locationID *uuid.UUID
		if a.ObjectLocation != nil {
			locationID = &a.ObjectLocation.ID
		}
		artifact := artifactRow{ID: a.ID, JobID: a.JobID, ProjectID: a.ProjectID, Attempt: a.Attempt, Format: string(a.Format), FormatVersion: a.FormatVersion, FileName: a.FileName, MIMEType: a.MIMEType, ByteSize: a.ByteSize, SHA256: a.SHA256, ObjectKey: a.ObjectKey, ObjectLocationID: locationID, CreatedAt: a.CreatedAt}
		if err = tx.Create(&artifact).Error; err != nil {
			return err
		}
		row.ArtifactID = &a.ID
		row.SHA256 = &a.SHA256
		row.Status = domain.ReviewRequired
		row.Stage = domain.ExportStageReview
		row.Progress = 95
		var frozen domain.FrozenExport
		if err := json.Unmarshal(row.Frozen, &frozen); err != nil {
			return err
		}
		if frozen.Settings != nil {
			row.Status = domain.Succeeded
			row.Stage = domain.ExportStageComplete
			row.Progress = 100
		}
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		if err = updateJob(tx, row); err != nil {
			return err
		}
		job = row.public()
		return nil
	})
	return job, err
}

// PreviewArtifact rechecks live project/source authority and returns private location only internally.
func (s *Store) PreviewArtifact(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (domain.ExportJob, domain.Artifact, error) {
	var job domain.ExportJob
	var artifact domain.Artifact
	if s == nil || s.db == nil {
		return job, artifact, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, project, false); err != nil {
			return err
		}
		row, err := readJob(tx, project, id, false)
		if err != nil {
			return err
		}
		if row.Status != domain.ReviewRequired && row.Status != domain.Succeeded {
			return application.ErrConflict
		}
		if err = s.validateSources(ctx, tx, actor, row); err != nil {
			return err
		}
		artifact, err = s.readArtifact(ctx, tx, row)
		if err == nil {
			job = row.public()
		}
		return err
	})
	return job, artifact, err
}
