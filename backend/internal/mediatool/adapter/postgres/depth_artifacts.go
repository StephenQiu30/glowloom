package postgres

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	domain "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

type depthObjectAction string

const (
	depthObjectActionWrite    depthObjectAction = "write"
	depthObjectActionVerified depthObjectAction = "verified"
	depthObjectActionRemove   depthObjectAction = "remove"
	depthObjectActionRemoved  depthObjectAction = "removed"
)

func loadDepthArtifact(tx *gorm.DB, r depthRow) (application.DepthArtifact, bool, error) {
	var a application.DepthArtifact
	var row depthResultIntentRecord
	if err := tx.Where("job_id = ? AND attempt = ?", r.ID, r.Attempt).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return a, false, nil
		}
		return a, false, err
	}
	if row.JobID == uuid.Nil {
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
	var rows []depthObjectRecord
	if err := tx.Where("job_id = ? AND attempt = ?", r.ID, r.Attempt).Find(&rows).Error; err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool {
		return depthObjectOrder(rows[i].Kind) < depthObjectOrder(rows[j].Kind)
	})
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
	objects := make([]application.DepthObject, len(rows))
	for i, row := range rows {
		o := row.object()
		want := a.Objects[i]
		if o.Kind != want.Kind || o.ObjectKey != want.ObjectKey || o.SHA256 != want.SHA256 || o.ByteSize != want.ByteSize || o.MIMEType != want.MIMEType {
			return nil, application.ErrDepthOutputInvalid
		}
		objects[i] = o
	}
	return objects, nil
}
func depthObjectOrder(kind application.DepthObjectKind) int {
	switch kind {
	case application.DepthObjectKindOriginal:
		return 0
	case application.DepthObjectKindPoster:
		return 1
	default:
		return 2
	}
}

func verifyDepthObjects(tx *gorm.DB, r depthRow, status application.DepthObjectStatus) error {
	objects, err := readDepthObjects(tx, r)
	if err != nil {
		return err
	}
	if len(objects) != 3 && status == application.DepthObjectStatusVerified {
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
		r, authority, err := s.lockDepthAuthority(ctx, tx, id.JobID, func(r depthRow) string {
			if r.Attempt != id.Attempt || r.ActiveWorker == nil || *r.ActiveWorker != id.ExecutionID || r.ExecutionUnconfirmed {
				return ""
			}
			if cleanup {
				if !r.CancellationRequested || (r.ProcessState != domain.DepthProcessNone && r.ProcessState != domain.DepthProcessEnded) {
					return ""
				}
				return "cleanup"
			}
			if r.CancellationRequested || r.Status != domain.DepthRunning {
				return ""
			}
			return "worker"
		})
		if err != nil {
			return err
		}
		if r.Attempt != id.Attempt || r.ActiveWorker == nil || *r.ActiveWorker != id.ExecutionID || r.ExecutionUnconfirmed {
			return application.ErrWorkerBusy
		}
		if cleanup {
			if !r.CancellationRequested || (r.ProcessState != domain.DepthProcessNone && r.ProcessState != domain.DepthProcessEnded) {
				return application.ErrConflict
			}
		} else {
			if r.CancellationRequested || r.Status == domain.DepthCancelRequested {
				return application.ErrCancelled
			}
			if r.Status != domain.DepthRunning {
				return application.ErrConflict
			}
		}
		if authority.err != nil {
			return authority.err
		}
		return use(tx, r)
	})
}

// FreezeArtifact binds every result byte and native receipt before object dispatch.
func (s *DepthStore) FreezeArtifact(ctx context.Context, id application.DepthWorkID, a application.DepthArtifact) error {
	return s.withDepthAttempt(ctx, id, false, func(tx *gorm.DB, r depthRow) error {
		if r.ProcessState != domain.DepthProcessEnded {
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
		intent := depthResultIntentRecord{JobID: r.ID, Attempt: r.Attempt, AssetID: a.Asset.ID, OutputSHA256: *a.Asset.SHA256, ArtifactSHA256: sha, Artifact: jsonDocument(body), CreatedAt: now}
		if err := tx.Create(&intent).Error; err != nil {
			return err
		}
		for _, o := range a.Objects {
			locationID, err := s.reserveDepthObject(ctx, tx, r, o.ObjectKey)
			if err != nil {
				return err
			}
			object := depthObjectRecord{JobID: r.ID, Attempt: r.Attempt, Kind: o.Kind, ObjectKey: o.ObjectKey, SHA256: o.SHA256, ByteSize: o.ByteSize, MIMEType: o.MIMEType, Status: application.DepthObjectStatusPending, ObjectLocationID: locationID, UpdatedAt: now}
			if err := tx.Create(&object).Error; err != nil {
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

func (s *DepthStore) changeDepthObject(ctx context.Context, id application.DepthWorkID, key string, action depthObjectAction) error {
	cleanup := action == depthObjectActionRemove || action == depthObjectActionRemoved
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
		case depthObjectActionWrite:
			if o.Status != application.DepthObjectStatusPending || o.WriteStarted || o.DeleteStarted {
				return application.ErrConflict
			}
			return updateDepthObject(tx, r, o, map[string]any{"write_started": true, "updated_at": time.Now().UTC()}, application.DepthObjectStatusPending)
		case depthObjectActionVerified:
			if o.Status == application.DepthObjectStatusVerified {
				return nil
			}
			if o.Status != application.DepthObjectStatusPending || !o.WriteStarted || o.DeleteStarted {
				return application.ErrConflict
			}
			return updateDepthObject(tx, r, o, map[string]any{"status": application.DepthObjectStatusVerified, "updated_at": time.Now().UTC()}, application.DepthObjectStatusPending)
		case depthObjectActionRemove:
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
			if err := s.media(tx).AuthorizeRemoval(ctx, actor, mediaapp.DerivedRemoval{ProjectID: r.ProjectID, AssetID: a.Asset.ID, ObjectKey: o.ObjectKey, PrimarySHA256: *a.Asset.SHA256, Kind: string(o.Kind), ByteSize: o.ByteSize}); err != nil {
				return normalize(err)
			}
			if o.Status == application.DepthObjectStatusRemoved {
				return nil
			}
			if o.Status != application.DepthObjectStatusVerified {
				return application.ErrConflict
			}
			return updateDepthObject(tx, r, o, map[string]any{"delete_started": true, "updated_at": time.Now().UTC()}, application.DepthObjectStatusVerified)
		case depthObjectActionRemoved:
			if o.Status == application.DepthObjectStatusRemoved {
				return nil
			}
			if (o.WriteStarted && !o.DeleteStarted) || o.Status == application.DepthObjectStatusPending && o.WriteStarted {
				return application.ErrConflict
			}
			result := tx.Model(&depthObjectRecord{}).Where("job_id = ? AND attempt = ? AND kind = ? AND object_key = ? AND status = ? AND write_started = ? AND delete_started = ?", r.ID, r.Attempt, o.Kind, key, o.Status, o.WriteStarted, o.DeleteStarted).Updates(map[string]any{"status": application.DepthObjectStatusRemoved, "updated_at": time.Now().UTC()})
			return requireDepthObjectUpdate(result)
		default:
			return application.ErrConflict
		}
	}
	var err error
	if action == depthObjectActionVerified {
		err = s.withDepthReadAttempt(ctx, id, use)
	} else {
		err = s.withDepthAttempt(ctx, id, cleanup, use)
	}
	if action == depthObjectActionVerified && errors.Is(err, application.ErrCancelled) {
		err = s.withDepthAttempt(ctx, id, true, use)
	}
	return err
}

// BeginWrite persists dispatch before the conditional private object Put.
func (s *DepthStore) BeginWrite(ctx context.Context, id application.DepthWorkID, key string) error {
	return s.changeDepthObject(ctx, id, key, depthObjectActionWrite)
}

// ConfirmObject records only actual full byte readback under this attempt.
func (s *DepthStore) ConfirmObject(ctx context.Context, id application.DepthWorkID, key string) error {
	return s.changeDepthObject(ctx, id, key, depthObjectActionVerified)
}

// BeginRemove keeps deletion uncertainty separate from an unknown write.
func (s *DepthStore) BeginRemove(ctx context.Context, id application.DepthWorkID, key string) error {
	return s.changeDepthObject(ctx, id, key, depthObjectActionRemove)
}

// ConfirmRemoved records absence only after a known removal or no dispatch.
func (s *DepthStore) ConfirmRemoved(ctx context.Context, id application.DepthWorkID, key string) error {
	return s.changeDepthObject(ctx, id, key, depthObjectActionRemoved)
}

func updateDepthObject(tx *gorm.DB, row depthRow, object application.DepthObject, values map[string]any, status application.DepthObjectStatus) error {
	result := tx.Model(&depthObjectRecord{}).Where("job_id = ? AND attempt = ? AND kind = ? AND object_key = ? AND status = ? AND write_started = ? AND delete_started = ?", row.ID, row.Attempt, object.Kind, object.ObjectKey, status, object.WriteStarted, object.DeleteStarted).Updates(values)
	return requireDepthObjectUpdate(result)
}

func requireDepthObjectUpdate(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return application.ErrConflict
	}
	return nil
}
