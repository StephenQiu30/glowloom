package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

// Claim assigns one physical owner and never treats a missing heartbeat as cessation.
func (s *DepthStore) Claim(ctx context.Context, id application.DepthWorkID) (application.DepthWork, error) {
	var work application.DepthWork
	if s == nil || s.db == nil {
		return work, application.ErrUnavailable
	}
	if id.JobID == uuid.Nil || id.Attempt < 1 || id.ExecutionID == uuid.Nil {
		return work, application.ErrInvalidDepthInput
	}
	var authorityErr error
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r, err := readDepth(tx, uuid.Nil, id.JobID, true)
		if err != nil {
			return err
		}
		if r.Attempt != id.Attempt {
			return application.ErrConflict
		}
		if r.ActiveWorker != nil || r.ExecutionUnconfirmed {
			return application.ErrWorkerBusy
		}
		actor := r.actor()
		if r.CancellationRequested || id.Reconcile {
			action := "reconcile"
			if r.CancellationRequested {
				action = "cleanup"
			}
			actor, err = s.depthControlActor(ctx, tx, r, action)
		} else {
			err = s.authorize(ctx, tx, actor, r.ProjectID, true)
		}
		if err != nil {
			if errors.Is(err, identityapp.ErrForbidden) && r.Status == "cancel_requested" && r.CancellationRequested && r.ProcessState == "none" {
				if empty, checkErr := depthHasNoOutput(tx, r); checkErr != nil {
					return checkErr
				} else if empty {
					// Reject only the proved current intent. Its permanent
					// response remains available, and a new authorized actor
					// may explicitly cancel this undispatched attempt.
					before := r
					r.Status, r.Stage = "failed", "failed"
					r.CancellationRequested, r.Retryable, r.NeedsReconciliation = false, false, false
					r.Revision++
					code := "depth_control_forbidden"
					r.FailureCode = &code
					if err := saveDepth(tx, before, r); err != nil {
						return err
					}
					authorityErr = err
					return depthAudit(tx, actor, depthRuntimeAuditKey(r), "failed", r.job())
				}
			}
			if errors.Is(err, identityapp.ErrForbidden) && id.Reconcile && r.ReconciliationRequested {
				// A fully proved intent can be rejected without borrowing its
				// revoked authority. No physical owner or object is changed.
				before := r
				r.ReconciliationRequested = false
				r.Status, r.Stage = "failed", "awaiting_reconciliation"
				r.Revision++
				code := "depth_cleanup_unknown"
				r.FailureCode = &code
				if err := saveDepth(tx, before, r); err != nil {
					return err
				}
				authorityErr = err
				return depthAudit(tx, actor, depthRuntimeAuditKey(r), "failed", r.job())
			}
			return err
		}
		if r.Status == "review_required" || r.Status == "succeeded" || r.Status == "cancelled" {
			j, err := depthPublic(tx, r)
			work.Job = j
			return err
		}
		if id.Reconcile && !r.ReconciliationRequested {
			return application.ErrConflict
		}
		if !r.CancellationRequested && !id.Reconcile {
			if err := s.validateSource(ctx, tx, r); err != nil {
				return err
			}
		}
		before := r
		state := r.state()
		if err := state.Claim(id.ExecutionID, id.Reconcile); err != nil {
			return application.ErrConflict
		}
		r.apply(state)
		r.ReconciliationRequested = false
		if err := saveDepth(tx, before, r); err != nil {
			return err
		}
		f, err := depthFrozen(r)
		if err != nil {
			return err
		}
		work = application.DepthWork{Job: r.job(), Actor: actor, Frozen: f, ProcessState: state.ProcessState}
		return nil
	})
	return work, errors.Join(err, authorityErr)
}

// Phase records only an observed closed execution phase and checks cancellation.
func (s *DepthStore) Phase(ctx context.Context, id application.DepthWorkID, stage string) error {
	switch stage {
	case "downloading", "checking", "preparing", "loading", "inferring", "encoding", "verifying", "previews", "storing", "reconciling":
	default:
		return application.ErrInvalidDepthInput
	}
	return s.withDepthAttempt(ctx, id, false, func(tx *gorm.DB, r depthRow) error {
		if stage == r.Stage {
			return nil
		}
		before := r
		r.Stage = stage
		r.Revision++
		r.UpdatedAt = time.Now().UTC()
		return saveDepth(tx, before, r)
	})
}

// StartProcess durably records dispatch before the fixed native call begins.
func (s *DepthStore) StartProcess(ctx context.Context, id application.DepthWorkID) error {
	return s.withDepthAttempt(ctx, id, false, func(tx *gorm.DB, r depthRow) error {
		before := r
		state := r.state()
		if err := state.StartProcess(id.ExecutionID); err != nil {
			return application.ErrConflict
		}
		r.apply(state)
		return saveDepth(tx, before, r)
	})
}

// EndProcess records physical truth under the original fence even after revocation.
// It grants no business write, object access or publication authorization.
func (s *DepthStore) EndProcess(ctx context.Context, id application.DepthWorkID) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r, err := readDepth(tx, uuid.Nil, id.JobID, true)
		if err != nil {
			return err
		}
		if r.Attempt != id.Attempt || r.ActiveWorker == nil || *r.ActiveWorker != id.ExecutionID {
			return application.ErrConflict
		}
		before := r
		state := r.state()
		if err := state.EndProcess(id.ExecutionID); err != nil {
			return application.ErrConflict
		}
		r.apply(state)
		return saveDepth(tx, before, r)
	})
}

// Commit registers only complete verified output as private pending media.
func (s *DepthStore) Commit(ctx context.Context, id application.DepthWorkID) (domain.DepthJob, error) {
	var job domain.DepthJob
	err := s.withDepthAttempt(ctx, id, false, func(tx *gorm.DB, r depthRow) error {
		if r.ProcessState != "ended" {
			return application.ErrConflict
		}
		if err := s.validateSource(ctx, tx, r); err != nil {
			return err
		}
		a, found, err := loadDepthArtifact(tx, r)
		if err != nil {
			return err
		}
		if !found {
			return application.ErrConflict
		}
		if err := verifyDepthObjects(tx, r, "verified"); err != nil {
			return err
		}
		if err := s.media(tx).Store(ctx, r.actor(), a.Asset, a.Renditions); err != nil {
			return normalize(err)
		}
		before := r
		r.Status = "review_required"
		r.Stage = "review"
		r.ActiveWorker = nil
		r.Revision++
		r.NeedsReconciliation = false
		r.Retryable = false
		r.FailureCode = nil
		r.UpdatedAt = time.Now().UTC()
		if err := saveDepth(tx, before, r); err != nil {
			return err
		}
		job = r.job()
		job.AssetID = &a.Asset.ID
		job.SHA256 = a.Asset.SHA256
		return depthRuntimeAudit(tx, r, "rendered", job)
	})
	return job, err
}

// Finish records synchronous cessation separately from uncertain object results.
// stopped=false preserves the old worker; cancellation still needs exact cleanup.
func (s *DepthStore) Finish(ctx context.Context, id application.DepthWorkID, stopped, uncertain bool, code string) (domain.DepthJob, error) {
	var job domain.DepthJob
	if s == nil || s.db == nil {
		return job, application.ErrUnavailable
	}
	if !depthFailureCode(code) {
		return job, application.ErrInvalidDepthInput
	}
	var authorityErr error
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r, err := readDepth(tx, uuid.Nil, id.JobID, true)
		if err != nil {
			return err
		}
		if r.Attempt != id.Attempt {
			return application.ErrConflict
		}
		if r.Status == "review_required" || r.Status == "succeeded" || r.Status == "cancelled" {
			job, err = depthPublic(tx, r)
			return err
		}
		if r.ActiveWorker == nil || *r.ActiveWorker != id.ExecutionID {
			return application.ErrConflict
		}
		before := r
		state := r.state()
		action := "failed"
		actor := r.actor()
		if !stopped {
			if err := state.Interrupt(id.ExecutionID); err != nil {
				return application.ErrConflict
			}
			code = "depth_execution_unknown"
		} else {
			if state.ProcessState == domain.DepthProcessStarted || state.ProcessState == domain.DepthProcessUnknown {
				if err := state.EndProcess(id.ExecutionID); err != nil {
					return application.ErrConflict
				}
				state.Job.Revision--
			}
			if state.Job.CancellationRequested && (!uncertain || state.ProcessState == domain.DepthProcessNone) {
				actor, err = s.depthControlActor(ctx, tx, r, "cleanup")
				if errors.Is(err, identityapp.ErrForbidden) {
					authorityErr = err
					code = "depth_cleanup_unknown"
					if state.ProcessState == domain.DepthProcessNone {
						empty, checkErr := depthHasNoOutput(tx, r)
						if checkErr != nil {
							return checkErr
						}
						if !empty {
							return err
						}
						state.Job.CancellationRequested = false
						uncertain, code = false, "depth_control_forbidden"
					} else {
						uncertain = true
						actor = r.actor()
					}
				} else if err != nil {
					return err
				}
			}
			if state.Job.CancellationRequested && !uncertain && authorityErr == nil {
				if err := verifyDepthObjects(tx, r, "removed"); err != nil {
					return err
				}
				a, found, err := loadDepthArtifact(tx, r)
				if err != nil {
					return err
				}
				if found {
					existing, err := s.media(tx).Asset(ctx, actor, r.ProjectID, a.Asset.ID)
					if err == nil {
						if existing.CanReference() || existing.SHA256 == nil || a.Asset.SHA256 == nil || *existing.SHA256 != *a.Asset.SHA256 {
							return application.ErrConflict
						}
						if err := s.media(tx).Reject(ctx, actor, r.ProjectID, a.Asset.ID, "depth_cancelled", time.Now().UTC()); err != nil {
							return normalize(err)
						}
					} else if !errors.Is(normalize(err), application.ErrNotFound) {
						return err
					}
				}
				if err := state.FinishCancelled(id.ExecutionID); err != nil {
					return application.ErrConflict
				}
				action = "cancelled"
			} else {
				state.WorkerID = uuid.Nil
				state.Job.Status = domain.DepthFailed
				state.Job.Stage = "failed"
				state.Job.Retryable = !uncertain && authorityErr == nil
				state.Job.NeedsReconciliation = uncertain
				state.Job.ExecutionUnconfirmed = false
				if uncertain {
					state.Job.Stage = "awaiting_reconciliation"
				}
				state.Job.Revision++
			}
		}
		if action != "cancelled" {
			state.Job.FailureCode = &code
		}
		r.apply(state)
		if err := saveDepth(tx, before, r); err != nil {
			return err
		}
		job = r.job()
		key := depthRuntimeAuditKey(r)
		return depthAudit(tx, actor, key, action, job)
	})
	return job, errors.Join(err, authorityErr)
}

// Interrupt retains a timed-out execution fence until the synchronous owner exits.
func (s *DepthStore) Interrupt(ctx context.Context, id application.DepthWorkID) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r, err := readDepth(tx, uuid.Nil, id.JobID, true)
		if err != nil {
			return err
		}
		if r.Attempt != id.Attempt {
			return application.ErrConflict
		}
		if r.ActiveWorker == nil {
			return nil
		}
		if *r.ActiveWorker != id.ExecutionID {
			return application.ErrWorkerBusy
		}
		before := r
		state := r.state()
		if err := state.Interrupt(id.ExecutionID); err != nil {
			return application.ErrConflict
		}
		code := "depth_execution_unknown"
		if state.ProcessState == domain.DepthProcessEnded {
			code = "depth_object_unknown"
		}
		state.Job.FailureCode = &code
		r.apply(state)
		if err := saveDepth(tx, before, r); err != nil {
			return err
		}
		return depthRuntimeAudit(tx, r, "failed", r.job())
	})
}

func depthFailureCode(code string) bool {
	switch code {
	case "", "depth_runtime_unavailable", "depth_model_mismatch", "depth_budget_exceeded", "depth_device_unavailable", "depth_input_invalid", "depth_output_invalid", "depth_inference_failed", "depth_execution_unknown", "depth_object_unknown", "depth_object_absence_unknown", "depth_object_conflict", "depth_cleanup_unknown", "depth_control_forbidden", "depth_result_incomplete", "source_unavailable", "dependency_unavailable":
		return true
	default:
		return false
	}
}
func depthRuntimeAudit(tx *gorm.DB, r depthRow, action string, j domain.DepthJob) error {
	key := depthRuntimeAuditKey(r)
	return depthAudit(tx, r.actor(), key, action, j)
}

func depthRuntimeAuditKey(r depthRow) uuid.UUID {
	return uuid.NewSHA1(r.ID, []byte(fmt.Sprintf("depth-runtime/%d/%d", r.Attempt, r.Revision)))
}

// VerifyDelivery proves the immutable command and exact committed event first.
func (s *DepthStore) VerifyDelivery(ctx context.Context, d application.DepthDelivery) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	if d.EventID == uuid.Nil || d.JobID == uuid.Nil || d.RequestID == uuid.Nil || d.ActorID == uuid.Nil || d.OrgID == uuid.Nil || d.ProjectID == uuid.Nil || d.Attempt < 1 || d.ExecutionID != uuid.Nil || d.Reconcile != (d.Action == "reconcile") || (d.Action != "start" && d.Action != "cancel" && d.Action != "reconcile") {
		return false, application.ErrInvalidDepthInput
	}
	body, err := json.Marshal(d)
	if err != nil {
		return false, err
	}
	var found bool
	err = s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM mediatool.depth_command c JOIN mediatool.depth_job j ON j.id=c.job_id JOIN infra.outbox o ON o.id=c.event_id WHERE c.actor_id=? AND c.request_id=? AND c.event_id=? AND c.event_action=? AND c.event_attempt=? AND j.id=? AND j.project_id=? AND j.org_id=? AND o.topic=? AND o.partition_key=? AND o.payload->'data'=?::jsonb)`, d.ActorID, d.RequestID, d.EventID, d.Action, d.Attempt, d.JobID, d.ProjectID, d.OrgID, depthTopic, d.JobID.String(), string(body)).Scan(&found).Error
	if err != nil {
		return false, err
	}
	if !found {
		return false, application.ErrInvalidDepthInput
	}
	r, err := readDepth(s.db.WithContext(ctx), d.ProjectID, d.JobID, false)
	if err != nil {
		return false, err
	}
	if r.Attempt != d.Attempt || r.Status == "succeeded" || r.Status == "cancelled" || r.Status == "review_required" {
		return false, nil
	}
	return true, nil
}

var _ application.DepthWorkerStore = (*DepthStore)(nil)
