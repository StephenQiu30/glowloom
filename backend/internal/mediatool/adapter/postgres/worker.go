package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

func (s *Store) validateSources(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, row jobRow) error {
	var frozen domain.FrozenExport
	if json.Unmarshal(row.Frozen, &frozen) != nil {
		return application.ErrInvalidExport
	}
	kind := domain.OutputKind(row.OutputKind).Effective()
	if !kind.Valid() || kind != frozen.OutputKind.Effective() || kind == domain.OutputAudio && !application.HasAudibleClip(frozen.Timeline) {
		return application.ErrInvalidExport
	}
	if kind == domain.OutputJianying && (frozen.ExportID == nil || *frozen.ExportID != row.ID) {
		return application.ErrConflict
	}
	if kind.IsArtifact() {
		if err := application.ValidateProjectExport(frozen); err != nil {
			return err
		}
		if frozen.Source == nil || frozen.Source.CanvasID != row.CanvasID || frozen.Source.NodeID != row.NodeID || frozen.Source.Revision != row.SourceRevision {
			return application.ErrConflict
		}
	}
	ids := make([]uuid.UUID, 0, len(frozen.Inputs))
	for _, input := range frozen.Inputs {
		ids = append(ids, input.AssetID)
	}
	assets, err := s.media(tx).Sources(ctx, actor, row.ProjectID, ids)
	if err != nil {
		return normalize(err)
	}
	current, err := application.FreezeInputs(frozen.Timeline, assets)
	if err != nil {
		return err
	}
	if len(current.Inputs) != len(frozen.Inputs) {
		return application.ErrConflict
	}
	for i, input := range frozen.Inputs {
		other := current.Inputs[i]
		if input.AssetID != other.AssetID || input.Revision != other.Revision || input.SHA256 != other.SHA256 || input.ObjectKey != other.ObjectKey || input.ByteSize != other.ByteSize || input.MIMEType != other.MIMEType {
			return application.ErrConflict
		}
	}
	return nil
}

// Claim is the atomic pre-dispatch cancellation gate; it also rechecks live sources.
func (s *Store) Claim(ctx context.Context, id application.WorkID) (application.Work, error) {
	var work application.Work
	cancelled := false
	if s == nil || s.db == nil || id.JobID == uuid.Nil || id.Attempt < 1 {
		return work, application.ErrInvalidExport
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := s.lockWorkerJob(ctx, tx, id.JobID, false)
		if err != nil {
			return err
		}
		if row.Attempt != id.Attempt {
			return application.ErrConflict
		}
		if row.Status == "cancel_requested" {
			if row.ActiveWorker != nil {
				cancelled = true
				return nil
			}
			row.Status = "cancelled"
			row.Stage = "cancelled"
			row.Revision++
			row.UpdatedAt = time.Now().UTC()
			cancelled = true
			return updateJob(tx, row)
		}
		if row.Status == "review_required" || row.Status == "succeeded" {
			work.Job = row.public()
			return nil
		}
		if row.Status != "queued" && row.Status != "running" {
			return application.ErrConflict
		}
		if row.ActiveWorker != nil && (id.ExecutionID == uuid.Nil || *row.ActiveWorker != id.ExecutionID) {
			return application.ErrWorkerBusy
		}
		if id.ExecutionID == uuid.Nil {
			return application.ErrInvalidExport
		}
		row.ActiveWorker = &id.ExecutionID
		if err := s.validateSources(ctx, tx, row.actor(), row); err != nil {
			return err
		}
		if row.Status == "queued" {
			row.Status = "running"
			row.Stage = "downloading"
			row.Progress = 5
			row.Revision++
			row.UpdatedAt = time.Now().UTC()
			if err := updateJob(tx, row); err != nil {
				return err
			}
		}
		if err := updateJob(tx, row); err != nil {
			return err
		}
		work.Job = row.public()
		work.Actor = row.actor()
		if err := json.Unmarshal(row.Frozen, &work.Frozen); err != nil {
			return err
		}
		if s.storage != nil {
			ids := make([]uuid.UUID, 0, len(work.Frozen.Inputs))
			for _, i := range work.Frozen.Inputs {
				ids = append(ids, i.AssetID)
			}
			work.SourceAssets, err = s.media(tx).Sources(ctx, row.actor(), row.ProjectID, ids)
			return err
		}
		return nil
	})
	if err == nil && cancelled {
		return application.Work{}, application.ErrCancelled
	}
	return work, err
}

// Progress records observed renderer stages without releasing cancellation evidence.
func (s *Store) Progress(ctx context.Context, id application.WorkID, progress int, stage string) error {
	if s == nil || s.db == nil || progress < 0 || progress > 99 || len(stage) > 32 {
		return application.ErrInvalidExport
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := s.lockWorkerJob(ctx, tx, id.JobID, false)
		if err != nil {
			return err
		}
		if row.Attempt != id.Attempt {
			return application.ErrConflict
		}
		if row.ActiveWorker == nil || *row.ActiveWorker != id.ExecutionID {
			return application.ErrConflict
		}
		if row.Status == "cancel_requested" || row.Status == "cancelled" {
			return application.ErrCancelled
		}
		if row.Status != "running" {
			return application.ErrConflict
		}
		if progress < row.Progress {
			return nil
		}
		row.Progress = progress
		row.Stage = stage
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		return updateJob(tx, row)
	})
}

// Commit records verified private output as review_required, never as ready.
func (s *Store) Commit(ctx context.Context, id application.WorkID, asset mediadomain.MediaAsset, renditions []mediadomain.Rendition) (domain.ExportJob, error) {
	var job domain.ExportJob
	if s == nil || s.db == nil {
		return job, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := s.lockWorkerJob(ctx, tx, id.JobID, true)
		if err != nil {
			return err
		}
		if row.Attempt != id.Attempt {
			return application.ErrConflict
		}
		if row.ActiveWorker == nil || *row.ActiveWorker != id.ExecutionID {
			return application.ErrConflict
		}
		if row.Status == "review_required" || row.Status == "succeeded" {
			if row.AssetID == nil || *row.AssetID != asset.ID || row.SHA256 == nil || asset.SHA256 == nil || *row.SHA256 != *asset.SHA256 {
				return application.ErrConflict
			}
			job = row.public()
			return nil
		}
		if row.Status == "cancel_requested" || row.Status == "cancelled" {
			return application.ErrCancelled
		}
		if row.Status != "running" || asset.ProjectID != row.ProjectID || string(asset.Kind) != string(domain.OutputKind(row.OutputKind).Effective()) {
			return application.ErrConflict
		}
		if err := s.validateSources(ctx, tx, row.actor(), row); err != nil {
			return err
		}
		if err := acceptToolMedia(ctx, tx, s.storage, application.StorageOwner("export", row.ProjectID, row.ID, row.Attempt), &asset, renditions); err != nil {
			return err
		}
		if err := s.media(tx).Store(ctx, row.actor(), asset, renditions); err != nil {
			return normalize(err)
		}

		row.Status = "review_required"
		row.Stage = "review"
		row.Progress = 95
		var frozen domain.FrozenExport
		if err := json.Unmarshal(row.Frozen, &frozen); err != nil {
			return err
		}
		if frozen.Settings != nil {
			finalizer, ok := s.media(tx).(interface {
				Finalize(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, string, []uuid.UUID, time.Time) error
			})
			if !ok {
				return application.ErrUnavailable
			}
			ids, err := application.SourceIDs(frozen.Timeline)
			if err != nil {
				return err
			}
			if err = finalizer.Finalize(ctx, row.actor(), row.ProjectID, asset.ID, *asset.SHA256, ids, time.Now().UTC()); err != nil {
				return normalize(err)
			}
			row.Status = "succeeded"
			row.Stage = "complete"
			row.Progress = 100
		}
		row.AssetID = &asset.ID
		row.SHA256 = asset.SHA256
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		if err := updateJob(tx, row); err != nil {
			return err
		}
		job = row.public()
		return nil
	})
	return job, err
}

// Finish records failure or confirmed worker cessation for one fenced attempt.
func (s *Store) Finish(ctx context.Context, id application.WorkID, cancelled bool, code string) error {
	if s == nil || s.db == nil || (code != "" && code != "render_failed" && code != "source_unavailable" && code != "dependency_unavailable" && code != "no_audio_stream") {
		return application.ErrInvalidExport
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := readJob(tx, uuid.Nil, id.JobID, true)
		if err != nil {
			return err
		}
		if row.Attempt != id.Attempt {
			return application.ErrConflict
		}
		if row.Status == "review_required" || row.Status == "succeeded" || row.Status == "failed" || row.Status == "cancelled" {
			return nil
		}
		if row.ActiveWorker == nil || *row.ActiveWorker != id.ExecutionID {
			return application.ErrConflict
		}
		if s.storage != nil {
			ledger := s.storage(tx)
			intent, err := ledger.ReadStorageWork(ctx, application.StorageOwner("export", row.ProjectID, row.ID, row.Attempt))
			if err != nil {
				return err
			}
			if err = ledger.CancelStorageWork(ctx, intent.ID); err != nil {
				return err
			}
		}
		row.ActiveWorker = nil
		if cancelled || row.Status == "cancel_requested" {
			row.Status = "cancelled"
			row.Stage = "cancelled"
			row.FailureCode = nil
		} else {
			row.Status = "failed"
			row.Stage = "failed"
			row.FailureCode = &code
		}
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		return updateJob(tx, row)
	})
}

// VerifyDelivery binds the outbox event to a committed command and current attempt.
func (s *Store) verifyDelivery(ctx context.Context, d application.Delivery) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	if d.EventID == uuid.Nil || d.JobID == uuid.Nil || d.RequestID == uuid.Nil || d.ActorID == uuid.Nil || d.OrgID == uuid.Nil || d.ProjectID == uuid.Nil || d.Attempt < 1 || (d.Action != "start" && d.Action != "cancel") {
		return false, application.ErrInvalidExport
	}
	var exists bool
	err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM mediatool.export_command c JOIN mediatool.export_job j ON j.id=c.job_id WHERE c.actor_id=? AND c.request_id=? AND c.event_id=? AND c.event_action=? AND c.event_attempt=? AND j.id=? AND j.project_id=? AND j.org_id=?)`, d.ActorID, d.RequestID, d.EventID, d.Action, d.Attempt, d.JobID, d.ProjectID, d.OrgID).Scan(&exists).Error
	if err != nil {
		return false, err
	}
	if !exists {
		return false, application.ErrInvalidExport
	}
	row, err := readJob(s.db.WithContext(ctx), uuid.Nil, d.JobID, false)
	if err != nil {
		return false, err
	}
	if row.Attempt != d.Attempt {
		return false, nil
	}
	if d.Action == "start" {
		return row.Status == "queued" || row.Status == "running" || row.Status == "cancel_requested", nil
	}
	return row.Status == "cancel_requested", nil
}

// FailWorkflow records failed orchestration without claiming unknown worker
// cessation. A pending cancellation stays pending until a real activity receipt.
func (s *Store) FailWorkflow(ctx context.Context, id application.WorkID) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := readJob(tx, uuid.Nil, id.JobID, true)
		if err != nil {
			return err
		}
		if row.Attempt != id.Attempt {
			return application.ErrConflict
		}
		if row.Status != "queued" && row.Status != "running" {
			return nil
		}
		code := "activity_failed"
		row.Status = "failed"
		row.Stage = "failed"
		row.FailureCode = &code
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		return updateJob(tx, row)
	})
}

// Release confirms this worker has actually returned before a storage retry claims
// the same attempt. It cannot release or cancel another active execution.
func (s *Store) Release(ctx context.Context, id application.WorkID) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := readJob(tx, uuid.Nil, id.JobID, true)
		if err != nil {
			return err
		}
		if row.Attempt != id.Attempt {
			return application.ErrConflict
		}
		if row.ActiveWorker == nil {
			return nil
		}
		if *row.ActiveWorker != id.ExecutionID {
			return application.ErrConflict
		}
		row.ActiveWorker = nil
		return updateJob(tx, row)
	})
}

// VerifyDelivery retains live scope authority until durable command validation completes.
func (s *Store) VerifyDelivery(ctx context.Context, d application.Delivery) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	valid := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		scoped := *s
		scoped.db = tx
		var err error
		valid, err = scoped.verifyDelivery(ctx, d)
		if err != nil || !valid {
			return err
		}
		row, err := readJob(tx, d.ProjectID, d.JobID, false)
		if err != nil {
			return err
		}
		if err := s.authorize(ctx, tx, identityapp.Principal{ID: d.ActorID, OrgID: d.OrgID}, d.ProjectID, true); err != nil {
			return err
		}
		if d.Action == "start" {
			return s.authorize(ctx, tx, row.actor(), d.ProjectID, true)
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return valid, nil
}

// AuthorizeObjectWrite checks the durable attempt and live account/scope immediately
// before each external Put. It never changes prior object or cessation evidence.
func (s *Store) AuthorizeObjectWrite(ctx context.Context, id application.WorkID) error {
	if s == nil || s.db == nil || id.ExecutionID == uuid.Nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := s.lockWorkerJob(ctx, tx, id.JobID, false)
		if err != nil {
			return err
		}
		if row.Attempt != id.Attempt || row.ActiveWorker == nil || *row.ActiveWorker != id.ExecutionID {
			return application.ErrConflict
		}
		if row.Status == "cancel_requested" || row.Status == "cancelled" {
			return application.ErrCancelled
		}
		if row.Status != "running" {
			return application.ErrConflict
		}
		return nil
	})
}
