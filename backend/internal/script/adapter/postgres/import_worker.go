package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

func importDeliveryProof(tx *gorm.DB, d application.ImportDelivery) error {
	if d.JobID == uuid.Nil || d.ProjectID == uuid.Nil || d.OrgID == uuid.Nil || d.ActorID == uuid.Nil || d.EventID == uuid.Nil || d.RequestID == uuid.Nil || d.Attempt < 1 || d.Action != "start" && d.Action != "cancel" && d.Action != "reconcile" {
		return domain.ErrInvalidSource
	}
	var valid bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM script.import_command c JOIN infra.outbox o ON o.id=c.event_id JOIN script.import_job j ON j.id=c.job_id WHERE c.actor_id=? AND c.request_id=? AND c.event_id=? AND c.event_action=? AND c.event_attempt=? AND c.job_id=? AND c.project_id=? AND c.org_id=? AND o.topic=? AND o.partition_key=? AND o.payload->>'event_id'=? AND o.payload->'data'->>'job_id'=? AND o.payload->'data'->>'attempt'=? AND o.payload->'data'->>'request_id'=? AND o.payload->'data'->>'actor_id'=? AND o.payload->'data'->>'org_id'=? AND o.payload->'data'->>'project_id'=? AND o.payload->'data'->>'action'=?)`, d.ActorID, d.RequestID, d.EventID, d.Action, d.Attempt, d.JobID, d.ProjectID, d.OrgID, ImportCommandTopic, d.ProjectID.String(), d.EventID.String(), d.JobID.String(), d.Attempt, d.RequestID.String(), d.ActorID.String(), d.OrgID.String(), d.ProjectID.String(), d.Action).Scan(&valid).Error; err != nil {
		return err
	}
	if !valid {
		return application.ErrIdempotencyConflict
	}
	return nil
}

// VerifyImportDelivery verifies permanent command and exact persisted envelope facts.
func (s *ImportStore) VerifyImportDelivery(ctx context.Context, d application.ImportDelivery) (bool, error) {
	if err := s.configured(); err != nil {
		return false, err
	}
	tx := s.db.WithContext(ctx)
	if err := importDeliveryProof(tx, d); err != nil {
		return false, err
	}
	r, err := readImport(tx, identityapp.Principal{ID: d.ActorID, OrgID: d.OrgID}, d.ProjectID, d.JobID, false)
	if err != nil {
		return false, err
	}
	if r.Job.Attempt != d.Attempt || r.Job.Status == "succeeded" || r.Job.Status == "cancelled" {
		return false, nil
	}
	switch d.Action {
	case "cancel":
		return r.Job.CancellationRequested, nil
	case "reconcile":
		return r.Job.ReconciliationRequested, nil
	default:
		return r.Job.Status == "queued" || r.Job.Status == "running" && !r.Job.NeedsReconciliation, nil
	}
}

func importActor(tx *gorm.DB, d application.ImportDelivery) (identityapp.Principal, error) {
	var row struct {
		ActorID   uuid.UUID
		ActorRole string
	}
	read := tx.Raw(`SELECT actor_id,actor_role FROM script.import_job WHERE id=? AND org_id=? AND project_id=?`, d.JobID, d.OrgID, d.ProjectID).Scan(&row)
	if read.Error != nil {
		return identityapp.Principal{}, read.Error
	}
	if read.RowsAffected != 1 {
		return identityapp.Principal{}, application.ErrNotFound
	}
	return identityapp.Principal{ID: row.ActorID, OrgID: d.OrgID, Role: identitydomain.Role(row.ActorRole)}, nil
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
		if r.OwnerID != nil || r.IOState == "unknown" || r.IOState == "running" {
			return application.ErrNeedsReconciliation
		}
		if d.Action == "start" && (r.Job.Status != "queued" || r.Job.NeedsReconciliation) || d.Action == "reconcile" && !r.Job.ReconciliationRequested {
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
		j.Status = "running"
		j.Stage = "extracting"
		j.ReconciliationRequested = false
		j.UpdatedAt = now
		if err := updateImportState(tx, j, &worker, "running"); err != nil {
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
	var row struct {
		ProjectID, OrgID, ActorID uuid.UUID
		ActorRole                 string
	}
	read := s.db.WithContext(ctx).Raw(`SELECT project_id,org_id,actor_id,actor_role FROM script.import_job WHERE id=?`, work.JobID).Scan(&row)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 {
		return application.ErrNotFound
	}
	actor := identityapp.Principal{ID: row.ActorID, OrgID: row.OrgID, Role: identitydomain.Role(row.ActorRole)}
	return s.creatorTransaction(ctx, actor, row.ProjectID, func(tx *gorm.DB, current identityapp.Principal) error {
		actor = current
		r, err := readImport(tx, actor, row.ProjectID, work.JobID, true)
		if err != nil {
			return err
		}
		if r.Job.Attempt != work.Attempt || r.OwnerID == nil || *r.OwnerID != worker || r.IOState != "running" {
			return application.ErrConflict
		}
		r.Actor = actor
		return f(tx, r)
	})
}

func (s *ImportStore) creatorTransaction(ctx context.Context, seed identityapp.Principal, project uuid.UUID, f func(*gorm.DB, identityapp.Principal) error) error {
	actor := seed
	actor.Role = identitydomain.RoleProducer
	run := func() error {
		return s.transaction(ctx, actor, project, true, func(tx *gorm.DB) error { return f(tx, actor) })
	}
	err := run()
	if errors.Is(err, identityapp.ErrForbidden) {
		actor.Role = identitydomain.RoleAdmin
		return run()
	}
	return err
}

// ReportImportPhase records actual current work, never an estimated percentage.
func (s *ImportStore) ReportImportPhase(ctx context.Context, work application.ImportWork, worker uuid.UUID, phase string, now time.Time) error {
	if !slices.Contains([]string{"extracting", "normalizing", "storing", "committing"}, phase) {
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
	if result.Position < 0 || result.Position > 199 || result.Status != "succeeded" && result.Status != "failed" || result.Status == "succeeded" && (len(result.RichSHA256) != 64 || len(result.ContentHash) != 64 || result.CharCount < 0 || result.CharCount > domain.MaxScalarCount) {
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
		return exactlyOne(tx.Exec(`INSERT INTO script.import_file_result(job_id,attempt,position,result,created_at) VALUES(?,?,?,?::jsonb,?)`, work.JobID, work.Attempt, result.Position, string(data), now))
	})
}

// FinishImport records a known local failure; pending object evidence keeps reconciliation.
func (s *ImportStore) FinishImport(ctx context.Context, work application.ImportWork, worker uuid.UUID, code string, uncertain bool, now time.Time) error {
	// This nonce proves this owning call actually returned. It does not publish,
	// delete bytes, or borrow changed caller authorization after revocation.
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct{ OrgID, ProjectID, ActorID uuid.UUID }
		if err := tx.Raw(`SELECT org_id,project_id,actor_id FROM script.import_job WHERE id=?`, work.JobID).Scan(&row).Error; err != nil {
			return err
		}
		r, err := readImport(tx, identityapp.Principal{ID: row.ActorID, OrgID: row.OrgID}, row.ProjectID, work.JobID, true)
		if err != nil {
			return err
		}
		if r.Job.Attempt != work.Attempt || r.OwnerID == nil || *r.OwnerID != worker {
			return application.ErrConflict
		}
		if r.Job.Status == "partial" || r.Job.Status == "succeeded" || r.Job.Status == "cancelled" {
			return nil
		}
		var pending bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM script.command c JOIN script.command_state s ON s.actor_id=c.actor_id AND s.request_id=c.request_id WHERE c.actor_id=? AND c.request_id=? AND s.status='pending')`, r.Actor.ID, r.PublicationKey).Scan(&pending).Error; err != nil {
			return err
		}
		j := r.Job
		j.Revision++
		j.FailureCode = code
		j.NeedsReconciliation = uncertain || pending
		j.Stage = "failed"
		j.Status = "failed"
		if j.CancellationRequested {
			j.Status = "cancel_requested"
			j.Stage = "cancelling"
		}
		if j.NeedsReconciliation {
			j.Stage = "awaiting_reconciliation"
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
	return exactlyOne(s.db.WithContext(ctx).Exec(`UPDATE script.import_state SET io_owner_id=NULL,io_state='ended',needs_reconciliation=needs_reconciliation OR ?,revision=revision+1,updated_at=? WHERE job_id=? AND attempt=? AND io_owner_id=? AND io_state='running'`, uncertain, now, work.JobID, work.Attempt, worker))
}

// FailImportWorkflow preserves active ownership after Temporal timeout or worker loss.
func (s *ImportStore) FailImportWorkflow(ctx context.Context, work application.ImportWork, now time.Time) error {
	if err := s.configured(); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Exec(`UPDATE script.import_state SET needs_reconciliation=true,stage='awaiting_reconciliation',failure_code='import_execution_unconfirmed',revision=revision+1,updated_at=? WHERE job_id=? AND attempt=? AND status IN ('queued','running','cancel_requested')`, now, work.JobID, work.Attempt).Error
}

func (s *ImportStore) controlTransaction(ctx context.Context, d application.ImportDelivery, f func(*gorm.DB, application.ImportRecord, identityapp.Principal) error) error {
	if err := s.configured(); err != nil {
		return err
	}
	if err := importDeliveryProof(s.db.WithContext(ctx), d); err != nil {
		return err
	}
	// Existing project owner compares exact current Principal role. Probe these two
	// closed allowed roles; neither constructed role grants access by itself.
	actor := identityapp.Principal{ID: d.ActorID, OrgID: d.OrgID, Role: identitydomain.RoleProducer}
	run := func() error {
		return s.transaction(ctx, actor, d.ProjectID, true, func(tx *gorm.DB) error {
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
	err := run()
	if errors.Is(err, identityapp.ErrForbidden) {
		actor.Role = identitydomain.RoleAdmin
		return run()
	}
	return err
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
		if r.OwnerID != nil || r.IOState != "idle" && r.IOState != "ended" {
			uncertain = true
		}
		var pending bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM script.command_state WHERE actor_id=? AND request_id=? AND status='pending')`, r.Actor.ID, r.PublicationKey).Scan(&pending).Error; err != nil {
			return err
		}
		if r.Job.CancellationRequested && pending {
			uncertain = true
		}
		j := r.Job
		j.Revision++
		j.UpdatedAt = now
		j.NeedsReconciliation = uncertain
		j.ReconciliationRequested = false
		switch {
		case uncertain:
			j.Stage = "awaiting_reconciliation"
		case j.CancellationRequested:
			j.Status = "cancelled"
			j.Stage = "completed"
			j.FailureCode = ""
		default:
			j.Status = "queued"
			j.Stage = "queued"
			j.FailureCode = ""
		}
		if err := updateImportState(tx, j, r.OwnerID, r.IOState); err != nil {
			return err
		}
		if j.Status == "cancelled" {
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
	var row struct{ OrgID, ProjectID, ActorID uuid.UUID }
	read := s.db.WithContext(ctx).Raw(`SELECT org_id,project_id,actor_id FROM script.import_job WHERE id=?`, work.JobID).Scan(&row)
	if read.Error != nil {
		return application.ImportJob{}, read.Error
	}
	if read.RowsAffected != 1 {
		return application.ImportJob{}, application.ErrNotFound
	}
	r, err := readImport(s.db.WithContext(ctx), identityapp.Principal{ID: row.ActorID, OrgID: row.OrgID}, row.ProjectID, work.JobID, false)
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
		if r.OwnerID == nil && r.IOState == "ended" {
			return nil
		}
		if r.OwnerID == nil || *r.OwnerID != owner {
			return application.ErrNeedsReconciliation
		}
		return exactlyOne(tx.Exec(`UPDATE script.import_state SET io_owner_id=NULL,io_state='ended',revision=revision+1,updated_at=? WHERE job_id=? AND attempt=? AND io_owner_id=? AND io_state IN ('running','unknown')`, now, d.JobID, d.Attempt, owner))
	})
}
