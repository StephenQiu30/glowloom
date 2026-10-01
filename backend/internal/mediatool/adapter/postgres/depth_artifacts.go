package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

func loadDepthArtifact(tx *gorm.DB, r depthRow) (application.DepthArtifact, bool, error) {
	var a application.DepthArtifact
	var row struct {
		AssetID                      uuid.UUID
		OutputSHA256, ArtifactSHA256 string
		Artifact                     []byte
	}
	res := tx.Raw(`SELECT asset_id,output_sha256,artifact_sha256,artifact FROM mediatool.depth_result_intent WHERE job_id=? AND attempt=?`, r.ID, r.Attempt).Scan(&row)
	if res.Error != nil {
		return a, false, res.Error
	}
	if res.RowsAffected == 0 {
		return a, false, nil
	}
	f, err := depthFrozen(r)
	if err != nil {
		return a, false, err
	}
	if depthJSON(row.Artifact, &a) != nil {
		return a, false, application.ErrDepthOutputInvalid
	}
	sha, _, err := application.DepthArtifactDigest(r.job(), f, a)
	if err != nil || sha != row.ArtifactSHA256 || a.Asset.ID != row.AssetID || a.Asset.SHA256 == nil || *a.Asset.SHA256 != row.OutputSHA256 {
		return a, false, application.ErrDepthOutputInvalid
	}
	return a, true, nil
}
func readDepthObjects(tx *gorm.DB, r depthRow) ([]application.DepthObject, error) {
	var rows []application.DepthObject
	if err := tx.Raw(`SELECT kind,object_key,sha256,byte_size,mime_type,write_started,delete_started,status FROM mediatool.depth_object WHERE job_id=? AND attempt=? ORDER BY CASE kind WHEN 'original' THEN 0 WHEN 'poster' THEN 1 ELSE 2 END`, r.ID, r.Attempt).Scan(&rows).Error; err != nil {
		return nil, err
	}
	a, found, err := loadDepthArtifact(tx, r)
	if err != nil {
		return nil, err
	}
	if !found {
		if len(rows) > 0 {
			return nil, application.ErrDepthOutputInvalid
		}
		return []application.DepthObject{}, nil
	}
	if len(rows) != len(a.Objects) {
		return nil, application.ErrDepthOutputInvalid
	}
	for i, o := range rows {
		want := a.Objects[i]
		if o.Kind != want.Kind || o.ObjectKey != want.ObjectKey || o.SHA256 != want.SHA256 || o.ByteSize != want.ByteSize || o.MIMEType != want.MIMEType {
			return nil, application.ErrDepthOutputInvalid
		}
	}
	return rows, nil
}
func verifyDepthObjects(tx *gorm.DB, r depthRow, status string) error {
	objects, err := readDepthObjects(tx, r)
	if err != nil {
		return err
	}
	if len(objects) != 3 && status == "verified" {
		return application.ErrConflict
	}
	for _, o := range objects {
		if o.Status != status {
			return application.ErrConflict
		}
	}
	return nil
}

func (s *DepthStore) withDepthAttempt(ctx context.Context, id application.DepthWorkID, cleanup bool, use func(*gorm.DB, depthRow) error) error {
	if s == nil || s.db == nil || id.JobID == uuid.Nil || id.Attempt < 1 || id.ExecutionID == uuid.Nil {
		return application.ErrInvalidDepthInput
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r, err := readDepth(tx, uuid.Nil, id.JobID, true)
		if err != nil {
			return err
		}
		if r.Attempt != id.Attempt || r.ActiveWorker == nil || *r.ActiveWorker != id.ExecutionID || r.ExecutionUnconfirmed {
			return application.ErrWorkerBusy
		}
		if cleanup {
			if !r.CancellationRequested || (r.ProcessState != "none" && r.ProcessState != "ended") {
				return application.ErrConflict
			}
			if _, err := s.depthControlActor(ctx, tx, r, "cleanup"); err != nil {
				return err
			}
		} else {
			if r.CancellationRequested || r.Status == "cancel_requested" {
				return application.ErrCancelled
			}
			if r.Status != "running" {
				return application.ErrConflict
			}
			if err := s.authorize(ctx, tx, r.actor(), r.ProjectID, true); err != nil {
				return err
			}
		}
		return use(tx, r)
	})
}

// FreezeArtifact binds every result byte and native receipt before object dispatch.
func (s *DepthStore) FreezeArtifact(ctx context.Context, id application.DepthWorkID, a application.DepthArtifact) error {
	return s.withDepthAttempt(ctx, id, false, func(tx *gorm.DB, r depthRow) error {
		if r.ProcessState != "ended" {
			return application.ErrConflict
		}
		f, err := depthFrozen(r)
		if err != nil {
			return err
		}
		sha, body, err := application.DepthArtifactDigest(r.job(), f, a)
		if err != nil {
			return err
		}
		prior, found, err := loadDepthArtifact(tx, r)
		if err != nil {
			return err
		}
		if found {
			priorSHA, _, err := application.DepthArtifactDigest(r.job(), f, prior)
			if err != nil || priorSHA != sha {
				return application.ErrConflict
			}
			return nil
		}
		now := time.Now().UTC()
		if err := tx.Exec(`INSERT INTO mediatool.depth_result_intent(job_id,attempt,asset_id,output_sha256,artifact_sha256,artifact,created_at) VALUES(?,?,?,?,?,?::jsonb,?)`, r.ID, r.Attempt, a.Asset.ID, *a.Asset.SHA256, sha, string(body), now).Error; err != nil {
			return err
		}
		for _, o := range a.Objects {
			if err := tx.Exec(`INSERT INTO mediatool.depth_object(job_id,attempt,kind,object_key,sha256,byte_size,mime_type,updated_at) VALUES(?,?,?,?,?,?,?,?)`, r.ID, r.Attempt, o.Kind, o.ObjectKey, o.SHA256, o.ByteSize, o.MIMEType, now).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Artifact reads only the current fenced attempt's immutable private manifest.
func (s *DepthStore) Artifact(ctx context.Context, id application.DepthWorkID) (application.DepthArtifact, bool, error) {
	var a application.DepthArtifact
	var found bool
	err := s.withDepthReadAttempt(ctx, id, func(tx *gorm.DB, r depthRow) error {
		var err error
		a, found, err = loadDepthArtifact(tx, r)
		return err
	})
	if errors.Is(err, application.ErrCancelled) {
		err = s.withDepthAttempt(ctx, id, true, func(tx *gorm.DB, r depthRow) error {
			var err error
			a, found, err = loadDepthArtifact(tx, r)
			return err
		})
	}
	return a, found, err
}

// Objects reads exact identities under a live write or cancellation fence.
func (s *DepthStore) Objects(ctx context.Context, id application.DepthWorkID) ([]application.DepthObject, error) {
	var objects []application.DepthObject
	err := s.withDepthReadAttempt(ctx, id, func(tx *gorm.DB, r depthRow) error { var err error; objects, err = readDepthObjects(tx, r); return err })
	if errors.Is(err, application.ErrCancelled) {
		err = s.withDepthAttempt(ctx, id, true, func(tx *gorm.DB, r depthRow) error { var err error; objects, err = readDepthObjects(tx, r); return err })
	}
	return objects, err
}

func (s *DepthStore) changeDepthObject(ctx context.Context, id application.DepthWorkID, key string, action string) error {
	cleanup := action == "remove" || action == "removed"
	use := func(tx *gorm.DB, r depthRow) error {
		objects, err := readDepthObjects(tx, r)
		if err != nil {
			return err
		}
		var found *application.DepthObject
		for i := range objects {
			if objects[i].ObjectKey == key {
				found = &objects[i]
				break
			}
		}
		if found == nil {
			return application.ErrConflict
		}
		o := *found
		switch action {
		case "write":
			if o.Status != "pending" || o.WriteStarted || o.DeleteStarted {
				return application.ErrConflict
			}
			return tx.Exec(`UPDATE mediatool.depth_object SET write_started=true,updated_at=statement_timestamp() WHERE job_id=? AND attempt=? AND object_key=?`, r.ID, r.Attempt, key).Error
		case "verified":
			if o.Status == "verified" {
				return nil
			}
			if o.Status != "pending" || !o.WriteStarted || o.DeleteStarted {
				return application.ErrConflict
			}
			return tx.Exec(`UPDATE mediatool.depth_object SET status='verified',updated_at=statement_timestamp() WHERE job_id=? AND attempt=? AND object_key=?`, r.ID, r.Attempt, key).Error
		case "remove":
			a, exists, err := loadDepthArtifact(tx, r)
			if err != nil {
				return err
			}
			if !exists || a.Asset.SHA256 == nil {
				return application.ErrConflict
			}
			actor, err := s.depthControlActor(ctx, tx, r, "cleanup")
			if err != nil {
				return err
			}
			if err := s.media(tx).AuthorizeRemoval(ctx, actor, mediaapp.DerivedRemoval{ProjectID: r.ProjectID, AssetID: a.Asset.ID, ObjectKey: o.ObjectKey, PrimarySHA256: *a.Asset.SHA256, Kind: o.Kind, ByteSize: o.ByteSize}); err != nil {
				return normalize(err)
			}
			if o.Status == "removed" {
				return nil
			}
			if o.Status != "verified" {
				return application.ErrConflict
			}
			return tx.Exec(`UPDATE mediatool.depth_object SET delete_started=true,updated_at=statement_timestamp() WHERE job_id=? AND attempt=? AND object_key=?`, r.ID, r.Attempt, key).Error
		case "removed":
			if o.Status == "removed" {
				return nil
			}
			if (o.WriteStarted && !o.DeleteStarted) || o.Status == "pending" && o.WriteStarted {
				return application.ErrConflict
			}
			return tx.Exec(`UPDATE mediatool.depth_object SET status='removed',updated_at=statement_timestamp() WHERE job_id=? AND attempt=? AND object_key=?`, r.ID, r.Attempt, key).Error
		default:
			return application.ErrConflict
		}
	}
	var err error
	if action == "verified" {
		err = s.withDepthReadAttempt(ctx, id, use)
	} else {
		err = s.withDepthAttempt(ctx, id, cleanup, use)
	}
	if action == "verified" && errors.Is(err, application.ErrCancelled) {
		err = s.withDepthAttempt(ctx, id, true, use)
	}
	return err
}

// BeginWrite persists dispatch before the conditional private object Put.
func (s *DepthStore) BeginWrite(ctx context.Context, id application.DepthWorkID, key string) error {
	return s.changeDepthObject(ctx, id, key, "write")
}

// ConfirmObject records only actual full byte readback under this attempt.
func (s *DepthStore) ConfirmObject(ctx context.Context, id application.DepthWorkID, key string) error {
	return s.changeDepthObject(ctx, id, key, "verified")
}

// BeginRemove keeps deletion uncertainty separate from an unknown write.
func (s *DepthStore) BeginRemove(ctx context.Context, id application.DepthWorkID, key string) error {
	return s.changeDepthObject(ctx, id, key, "remove")
}

// ConfirmRemoved records absence only after a known removal or no dispatch.
func (s *DepthStore) ConfirmRemoved(ctx context.Context, id application.DepthWorkID, key string) error {
	return s.changeDepthObject(ctx, id, key, "removed")
}
