package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	workspacedomain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

func importDeliveryProof(tx *gorm.DB, d application.ImportDelivery) error {
	if d.JobID == uuid.Nil || d.ProjectID == uuid.Nil || d.OrgID == uuid.Nil || d.ActorID == uuid.Nil || d.EventID == uuid.Nil || d.RequestID == uuid.Nil || d.Attempt < 1 || d.Action != domain.ImportActionStart && d.Action != domain.ImportActionCancel && d.Action != domain.ImportActionReconcile {
		return domain.ErrInvalidSource
	}
	var command scriptImportCommandRow
	if err := tx.Where("actor_id = ? AND request_id = ?", d.ActorID, d.RequestID).Take(&command).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return application.ErrIdempotencyConflict
		}
		return err
	}
	if command.EventID != d.EventID || command.EventAction != string(d.Action) || command.EventAttempt != d.Attempt || command.JobID != d.JobID || command.ProjectID != d.ProjectID || command.OrgID != d.OrgID {
		return application.ErrIdempotencyConflict
	}
	var outbox scriptOutboxRow
	if err := tx.Where("id = ? AND topic = ? AND partition_key = ?", d.EventID, ImportCommandTopic, d.ProjectID.String()).Take(&outbox).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return application.ErrIdempotencyConflict
		}
		return err
	}
	var envelope struct {
		EventID   uuid.UUID                  `json:"event_id"`
		EventType string                     `json:"event_type"`
		OrgID     uuid.UUID                  `json:"org_id"`
		Data      application.ImportDelivery `json:"data"`
	}
	if err := json.Unmarshal(outbox.Payload, &envelope); err != nil {
		return application.ErrIdempotencyConflict
	}
	if envelope.EventID != d.EventID || envelope.EventType != ImportCommandTopic || envelope.OrgID != d.OrgID || envelope.Data != d {
		return application.ErrIdempotencyConflict
	}
	return nil
}

// VerifyImportDelivery verifies permanent command and exact persisted envelope facts.
func (s *ImportStore) VerifyImportDelivery(ctx context.Context, d application.ImportDelivery) (bool, error) {
	if err := s.configured(); err != nil {
		return false, err
	}
	var valid bool
	err := taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		if err := importDeliveryProof(tx, d); err != nil {
			return err
		}
		actor := identityapp.Principal{ID: d.ActorID, OrgID: d.OrgID}
		scoped := &ImportStore{db: tx, workerAccess: s.workerAccess}
		err := scoped.workerTransaction(ctx, actor, d.ProjectID, func(tx *gorm.DB) error {
			r, err := readImport(tx, actor, d.ProjectID, d.JobID, false)
			if err != nil {
				return err
			}
			if r.Job.Attempt != d.Attempt || r.Job.Status == domain.ImportStatusSucceeded || r.Job.Status == domain.ImportStatusCancelled {
				return nil
			}
			switch d.Action {
			case domain.ImportActionCancel:
				valid = r.Job.CancellationRequested
			case domain.ImportActionReconcile:
				valid = r.Job.ReconciliationRequested
			default:
				valid = r.Actor.ID == d.ActorID && (r.Job.Status == domain.ImportStatusQueued || r.Job.Status == domain.ImportStatusRunning && !r.Job.NeedsReconciliation)
			}
			return nil
		})
		if errors.Is(err, workspace.ErrScopeNotFound) || errors.Is(err, workspace.ErrProjectNotFound) || errors.Is(err, workspacedomain.ErrProjectStateConflict) || errors.Is(err, identityapp.ErrForbidden) {
			valid = false
			return nil
		}
		return err
	})
	return valid, err
}

func importActor(tx *gorm.DB, d application.ImportDelivery) (identityapp.Principal, error) {
	var row scriptImportJobRow
	if err := tx.Where("id = ? AND org_id = ? AND project_id = ?", d.JobID, d.OrgID, d.ProjectID).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return identityapp.Principal{}, application.ErrNotFound
		}
		return identityapp.Principal{}, err
	}
	return identityapp.Principal{ID: row.ActorID, OrgID: d.OrgID}, nil
}

// ClaimImport refuses timeout/lost-owner inference; only queued or proven stopped work starts.
func (s *ImportStore) ClaimImport(ctx context.Context, d application.ImportDelivery, worker uuid.UUID, now time.Time) (application.ImportRecord, error) {
	var result application.ImportRecord
	if err := s.configured(); err != nil {
		return result, err
	}
	if worker == uuid.Nil {
		return result, domain.ErrInvalidSource
	}
	actor, err := importActor(s.db.WithContext(ctx), d)
	if err != nil {
		return result, err
	}
	err = s.creatorTransaction(ctx, actor, d.ProjectID, func(tx *gorm.DB, current identityapp.Principal) error {
		actor = current
		if err := importDeliveryProof(tx, d); err != nil {
			return err
		}
		r, err := readImport(tx, actor, d.ProjectID, d.JobID, true)
		if err != nil {
			return err
		}
		if r.Job.Attempt != d.Attempt || r.Job.CancellationRequested {
			return application.ErrConflict
		}
		if r.OwnerID != nil || r.IOState == domain.ImportIOStateUnknown || r.IOState == domain.ImportIOStateRunning {
			return application.ErrNeedsReconciliation
		}
		if d.Action == domain.ImportActionStart && (r.Job.Status != domain.ImportStatusQueued || r.Job.NeedsReconciliation) || d.Action == domain.ImportActionReconcile && !r.Job.ReconciliationRequested {
			return application.ErrConflict
		}
		state, err := readState(tx, actor.OrgID, d.ProjectID, true)
		if err != nil {
			return err
		}
		if state.Revision != r.Job.LatestScriptRevision || !sameOptionalUUID(state.DraftVersionID, r.BaseVersionID) {
			return application.ErrConflict
		}
		j := r.Job
		j.Revision++
		j.Status = domain.ImportStatusRunning
		j.Stage = domain.ImportStageExtracting
		j.ReconciliationRequested = false
		j.UpdatedAt = now
		if err := updateImportState(tx, j, &worker, domain.ImportIOStateRunning); err != nil {
			return err
		}
		result, err = readImport(tx, actor, d.ProjectID, d.JobID, false)
		result.Actor = actor
		return err
	})
	return result, err
}

func (s *ImportStore) withWorker(ctx context.Context, work application.ImportWork, worker uuid.UUID, f func(*gorm.DB, application.ImportRecord) error) error {
	if err := s.configured(); err != nil {
		return err
	}
	var job scriptImportJobRow
	if err := s.db.WithContext(ctx).Where("id = ?", work.JobID).Take(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return application.ErrNotFound
		}
		return err
	}
	actor := identityapp.Principal{ID: job.ActorID, OrgID: job.OrgID}
	return s.creatorTransaction(ctx, actor, job.ProjectID, func(tx *gorm.DB, current identityapp.Principal) error {
		actor = current
		r, err := readImport(tx, actor, job.ProjectID, work.JobID, true)
		if err != nil {
			return err
		}
		if r.Job.Attempt != work.Attempt || r.OwnerID == nil || *r.OwnerID != worker || r.IOState != domain.ImportIOStateRunning {
			return application.ErrConflict
		}
		r.Actor = actor
		return f(tx, r)
	})
}

func (s *ImportStore) creatorTransaction(ctx context.Context, seed identityapp.Principal, project uuid.UUID, f func(*gorm.DB, identityapp.Principal) error) error {
	return s.workerTransaction(ctx, seed, project, func(tx *gorm.DB) error { return f(tx, seed) })
}

// ReportImportPhase records actual current work, never an estimated percentage.
func (s *ImportStore) ReportImportPhase(ctx context.Context, work application.ImportWork, worker uuid.UUID, phase domain.ImportStage, now time.Time) error {
	if phase != domain.ImportStageExtracting && phase != domain.ImportStageNormalizing && phase != domain.ImportStageStoring && phase != domain.ImportStageCommitting {
		return domain.ErrInvalidSource
	}
	return s.withWorker(ctx, work, worker, func(tx *gorm.DB, r application.ImportRecord) error {
		if r.Job.CancellationRequested {
			return application.ErrConflict
		}
		j := r.Job
		j.Revision++
		j.Stage = phase
		j.UpdatedAt = now
		return updateImportState(tx, j, r.OwnerID, r.IOState)
	})
}

// RecordImportFile preserves each original attempt outcome without editable bodies in SQL.
func (s *ImportStore) RecordImportFile(ctx context.Context, work application.ImportWork, worker uuid.UUID, result application.ImportFileResult, now time.Time) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if result.Position < 0 || result.Position > 199 || result.Status != domain.ImportFileStatusSucceeded && result.Status != domain.ImportFileStatusFailed || result.Status == domain.ImportFileStatusSucceeded && (len(result.RichSHA256) != 64 || len(result.ContentHash) != 64 || result.CharCount < 0 || result.CharCount > domain.MaxScalarCount) {
		return domain.ErrInvalidSource
	}
	return s.withWorker(ctx, work, worker, func(tx *gorm.DB, r application.ImportRecord) error {
		if r.Job.CancellationRequested {
			return application.ErrConflict
		}
		positions, err := importPositions(tx, work)
		if err != nil {
			return err
		}
		if !slices.Contains(positions, result.Position) {
			return application.ErrConflict
		}
		if old, found := findImportFileResult(r.Results, result.Position); found {
			saved, _ := json.Marshal(old)
			if !slices.Equal(saved, data) {
				return application.ErrObjectMismatch
			}
			return nil
		}
		row := scriptImportFileResultRow{JobID: work.JobID, Attempt: work.Attempt, Position: result.Position, Result: scriptJSON(data), CreatedAt: now}
		return exactlyOne(tx.Create(&row))
	})
}

// FinishImport records a known local failure; pending object evidence keeps reconciliation.
func (s *ImportStore) FinishImport(ctx context.Context, work application.ImportWork, worker uuid.UUID, code string, uncertain bool, now time.Time) error {
	// This nonce proves this owning call actually returned. It does not publish,
	// delete bytes, or borrow changed caller authorization after revocation.
	return taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		var job scriptImportJobRow
		if err := tx.Where("id = ?", work.JobID).Take(&job).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return application.ErrNotFound
			}
			return err
		}
		r, err := readImport(tx, identityapp.Principal{ID: job.ActorID, OrgID: job.OrgID}, job.ProjectID, work.JobID, true)
		if err != nil {
			return err
		}
		if r.Job.Attempt != work.Attempt || r.OwnerID == nil || *r.OwnerID != worker {
			return application.ErrConflict
		}
		if r.Job.Status == domain.ImportStatusPartial || r.Job.Status == domain.ImportStatusSucceeded || r.Job.Status == domain.ImportStatusCancelled {
			return nil
		}
		var pending int64
		if err := tx.Model(&scriptCommandStateRow{}).Where("actor_id = ? AND request_id = ? AND status = ?", r.Actor.ID, r.PublicationKey, scriptCommandPending).Count(&pending).Error; err != nil {
			return err
		}
		j := r.Job
		j.Revision++
		j.FailureCode = code
		j.NeedsReconciliation = uncertain || pending > 0
		j.Stage = domain.ImportStageFailed
		j.Status = domain.ImportStatusFailed
		if j.CancellationRequested {
			j.Status = domain.ImportStatusCancelRequested
			j.Stage = domain.ImportStageCancelling
		}
		if j.NeedsReconciliation {
			j.Stage = domain.ImportStageAwaitingReconciliation
		}
		j.UpdatedAt = now
		return updateImportState(tx, j, r.OwnerID, r.IOState)
	})
}

// EndImportIO records only the exact matching call's physical exit, even after revocation.
func (s *ImportStore) EndImportIO(ctx context.Context, work application.ImportWork, worker uuid.UUID, uncertain bool, now time.Time) error {
	if err := s.configured(); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state scriptImportStateRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("job_id = ? AND attempt = ? AND io_owner_id = ? AND io_state = ?", work.JobID, work.Attempt, worker, domain.ImportIOStateRunning).Take(&state).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return application.ErrConflict
			}
			return err
		}
		return exactlyOne(tx.Model(&scriptImportStateRow{}).Where("job_id = ? AND attempt = ? AND io_owner_id = ? AND io_state = ?", work.JobID, work.Attempt, worker, domain.ImportIOStateRunning).
			Updates(map[string]any{"io_owner_id": nil, "io_state": domain.ImportIOStateEnded, "needs_reconciliation": state.NeedsReconciliation || uncertain, "revision": state.Revision + 1, "updated_at": now}))
	})
}

// FailImportWorkflow preserves active ownership after an execution timeout or worker loss.
func (s *ImportStore) FailImportWorkflow(ctx context.Context, work application.ImportWork, now time.Time) error {
	if err := s.configured(); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state scriptImportStateRow
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("job_id = ? AND attempt = ? AND status IN ?", work.JobID, work.Attempt, []domain.ImportStatus{domain.ImportStatusQueued, domain.ImportStatusRunning, domain.ImportStatusCancelRequested}).Take(&state).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return exactlyOne(tx.Model(&scriptImportStateRow{}).Where("job_id = ? AND attempt = ? AND revision = ?", work.JobID, work.Attempt, state.Revision).
			Updates(map[string]any{"needs_reconciliation": true, "stage": domain.ImportStageAwaitingReconciliation, "failure_code": "import_execution_unconfirmed", "revision": state.Revision + 1, "updated_at": now}))
	})
}

func (s *ImportStore) controlTransaction(ctx context.Context, d application.ImportDelivery, f func(*gorm.DB, application.ImportRecord, identityapp.Principal) error) error {
	if err := s.configured(); err != nil {
		return err
	}
	if err := importDeliveryProof(s.db.WithContext(ctx), d); err != nil {
		return err
	}
	actor := identityapp.Principal{ID: d.ActorID, OrgID: d.OrgID}
	return s.workerTransaction(ctx, actor, d.ProjectID, func(tx *gorm.DB) error {
		if err := importDeliveryProof(tx, d); err != nil {
			return err
		}
		r, err := readImport(tx, actor, d.ProjectID, d.JobID, true)
		if err != nil {
			return err
		}
		if !r.Job.CanControl || r.Job.Attempt != d.Attempt {
			return identityapp.ErrForbidden
		}
		return f(tx, r, actor)
	})
}

// ImportControlRecord rechecks the actual permanent controller, never the old creator.
func (s *ImportStore) ImportControlRecord(ctx context.Context, d application.ImportDelivery) (application.ImportRecord, identityapp.Principal, error) {
	var result application.ImportRecord
	var actor identityapp.Principal
	err := s.controlTransaction(ctx, d, func(_ *gorm.DB, r application.ImportRecord, current identityapp.Principal) error {
		result = r
		actor = current
		return nil
	})
	return result, actor, err
}

// FinishImportControl preserves published versions and only releases proven unpublished work.
func (s *ImportStore) FinishImportControl(ctx context.Context, d application.ImportDelivery, uncertain bool, now time.Time) error {
	return s.controlTransaction(ctx, d, func(tx *gorm.DB, r application.ImportRecord, actor identityapp.Principal) error {
		if r.OwnerID != nil || r.IOState != domain.ImportIOStateIdle && r.IOState != domain.ImportIOStateEnded {
			uncertain = true
		}
		var pending int64
		if err := tx.Model(&scriptCommandStateRow{}).Where("actor_id = ? AND request_id = ? AND status = ?", r.Actor.ID, r.PublicationKey, scriptCommandPending).Count(&pending).Error; err != nil {
			return err
		}
		if r.Job.CancellationRequested && pending > 0 {
			uncertain = true
		}
		j := r.Job
		j.Revision++
		j.UpdatedAt = now
		j.NeedsReconciliation = uncertain
		j.ReconciliationRequested = false
		switch {
		case uncertain:
			j.Stage = domain.ImportStageAwaitingReconciliation
		case j.CancellationRequested:
			j.Status = domain.ImportStatusCancelled
			j.Stage = domain.ImportStageCompleted
			j.FailureCode = ""
		default:
			j.Status = domain.ImportStatusQueued
			j.Stage = domain.ImportStageQueued
			j.FailureCode = ""
		}
		if err := updateImportState(tx, j, r.OwnerID, r.IOState); err != nil {
			return err
		}
		if j.Status == domain.ImportStatusCancelled {
			return importAudit(tx, actor, uuid.NewSHA1(d.EventID, []byte("cancelled")), d.RequestID, "cancelled", j, now)
		}
		return nil
	})
}

var _ application.ImportWorkerStore = (*ImportStore)(nil)

// ImportWorkState is an internal orchestration read of exact owning work identity.
func (s *ImportStore) ImportWorkState(ctx context.Context, work application.ImportWork) (application.ImportJob, error) {
	if err := s.configured(); err != nil {
		return application.ImportJob{}, err
	}
	var job scriptImportJobRow
	if err := s.db.WithContext(ctx).Where("id = ?", work.JobID).Take(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return application.ImportJob{}, application.ErrNotFound
		}
		return application.ImportJob{}, err
	}
	r, err := readImport(s.db.WithContext(ctx), identityapp.Principal{ID: job.ActorID, OrgID: job.OrgID}, job.ProjectID, work.JobID, false)
	if err != nil {
		return application.ImportJob{}, err
	}
	if r.Job.Attempt < work.Attempt {
		return application.ImportJob{}, application.ErrConflict
	}
	return r.Job, nil
}

// ProveImportStop requires the exact committed controller and locally joined I/O owner.
// Failed exit persistence does not lose this caller's actual cessation proof.
func (s *ImportStore) ProveImportStop(ctx context.Context, d application.ImportDelivery, owner uuid.UUID, now time.Time) error {
	if owner == uuid.Nil {
		return application.ErrNeedsReconciliation
	}
	return s.controlTransaction(ctx, d, func(tx *gorm.DB, r application.ImportRecord, _ identityapp.Principal) error {
		if r.OwnerID == nil && r.IOState == domain.ImportIOStateEnded {
			return nil
		}
		if r.OwnerID == nil || *r.OwnerID != owner {
			return application.ErrNeedsReconciliation
		}
		var state scriptImportStateRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("job_id = ? AND attempt = ? AND io_owner_id = ? AND io_state IN ?", d.JobID, d.Attempt, owner, []domain.ImportIOState{domain.ImportIOStateRunning, domain.ImportIOStateUnknown}).Take(&state).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return application.ErrConflict
			}
			return err
		}
		return exactlyOne(tx.Model(&scriptImportStateRow{}).Where("job_id = ? AND attempt = ? AND io_owner_id = ? AND io_state IN ?", d.JobID, d.Attempt, owner, []domain.ImportIOState{domain.ImportIOStateRunning, domain.ImportIOStateUnknown}).
			Updates(map[string]any{"io_owner_id": nil, "io_state": domain.ImportIOStateEnded, "revision": state.Revision + 1, "updated_at": now}))
	})
}
