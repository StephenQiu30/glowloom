package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

func (s *TranscriptionStore) validateSource(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, row transcriptionRow) error {
	var frozen domain.FrozenTranscription
	if json.Unmarshal(row.Frozen, &frozen) != nil || frozen.Language != row.Language || !domain.ValidTranscriptionLanguage(frozen.Language) {
		return application.ErrInvalidTranscription
	}
	assets, err := s.media(tx).Sources(ctx, actor, row.ProjectID, []uuid.UUID{frozen.Input.AssetID})
	if err != nil {
		return normalize(err)
	}
	if len(assets) != 1 {
		return application.ErrConflict
	}
	current, err := transcriptionInput(assets[0])
	if err != nil {
		return err
	}
	expected, _ := json.Marshal(frozen.Input)
	actual, _ := json.Marshal(current)
	if string(expected) != string(actual) {
		return application.ErrConflict
	}
	return nil
}

// Worker gates follow the same authority-before-job order as request commands.
// The unlocked candidate supplies authority only; all work uses the locked row.
func (s *TranscriptionStore) lockAuthorizedTranscription(ctx context.Context, tx *gorm.DB, id uuid.UUID) (transcriptionRow, error) {
	candidate, err := readTranscription(tx, uuid.Nil, id, false)
	if err != nil {
		return transcriptionRow{}, err
	}
	if err := s.authorize(ctx, tx, candidate.actor(), candidate.ProjectID, true); err != nil {
		return transcriptionRow{}, err
	}
	return lockTranscriptionBinding(tx, candidate)
}

func lockTranscriptionBinding(tx *gorm.DB, candidate transcriptionRow) (transcriptionRow, error) {
	row, err := readTranscription(tx, uuid.Nil, candidate.ID, true)
	if err != nil {
		return row, err
	}
	if row.ActorID != candidate.ActorID || row.OrgID != candidate.OrgID || row.ProjectID != candidate.ProjectID {
		return row, application.ErrConflict
	}
	return row, nil
}

// Claim serializes physical local owners and never restarts uncertain inference.
func (s *TranscriptionStore) Claim(ctx context.Context, id application.TranscriptionWorkID) (application.TranscriptionWork, error) {
	var work application.TranscriptionWork
	cancelled := false
	uncertain := false
	if s == nil || s.db == nil || id.JobID == uuid.Nil || id.Attempt < 1 {
		return work, application.ErrInvalidTranscription
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := s.lockAuthorizedTranscription(ctx, tx, id.JobID)
		if err != nil {
			return err
		}
		if row.Attempt != id.Attempt {
			return application.ErrConflict
		}
		if row.Status == "succeeded" {
			work.Job = row.public()
			return nil
		}
		if row.InferenceState == "unknown" || row.InferenceState == "submitted" {
			uncertain = true
			if row.Stage != "awaiting_reconciliation" {
				row.Stage = "awaiting_reconciliation"
				code := "inference_unknown"
				row.FailureCode = &code
				row.Revision++
				row.UpdatedAt = time.Now().UTC()
				if row.ActiveWorker == nil {
					row.InferenceState = "unknown"
				}
				return updateTranscription(tx, row)
			}
			return nil
		}
		if row.Status == "cancel_requested" {
			cancelled = true
			if row.ActiveWorker == nil && row.InferenceState == "none" {
				row.Status = "cancelled"
				row.Stage = "cancelled"
				row.Revision++
				row.UpdatedAt = time.Now().UTC()
				if err := updateTranscription(tx, row); err != nil {
					return err
				}
				return transcriptionRuntimeAudit(tx, row, "cancelled")
			}
			return nil
		}
		if row.Status != "queued" && row.Status != "running" {
			return application.ErrConflict
		}
		if row.ActiveWorker != nil && (id.ExecutionID == uuid.Nil || *row.ActiveWorker != id.ExecutionID) {
			return application.ErrWorkerBusy
		}
		if id.ExecutionID == uuid.Nil {
			return application.ErrInvalidTranscription
		}
		if err := s.validateSource(ctx, tx, row.actor(), row); err != nil {
			return err
		}
		row.ActiveWorker = &id.ExecutionID
		row.Status = "running"
		row.Stage = "downloading"
		row.Progress = max(row.Progress, 5)
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		if err := updateTranscription(tx, row); err != nil {
			return err
		}
		work.Job, work.Actor, work.InferenceState = row.public(), row.actor(), row.InferenceState
		if err := json.Unmarshal(row.Frozen, &work.Frozen); err != nil {
			return err
		}
		if s.storage != nil {
			work.SourceAssets, err = s.media(tx).Sources(ctx, row.actor(), row.ProjectID, []uuid.UUID{work.Frozen.Input.AssetID})
			if err != nil {
				return normalize(err)
			}
			if len(work.SourceAssets) != 1 || work.SourceAssets[0].ID != work.Frozen.Input.AssetID {
				return application.ErrConflict
			}
		}
		return nil
	})
	if err == nil && cancelled {
		return application.TranscriptionWork{}, application.ErrCancelled
	}
	if err == nil && uncertain {
		return application.TranscriptionWork{}, application.ErrInferenceUncertain
	}
	return work, err
}
func ownTranscription(row transcriptionRow, id application.TranscriptionWorkID) error {
	if row.Attempt != id.Attempt || row.ActiveWorker == nil || *row.ActiveWorker != id.ExecutionID {
		return application.ErrConflict
	}
	return nil
}

// Progress is observed local work, never an inferred native inference percentage.
func (s *TranscriptionStore) Progress(ctx context.Context, id application.TranscriptionWorkID, percentage int, stage string) error {
	if s == nil || s.db == nil || percentage < 0 || percentage > 99 || len(stage) > 32 {
		return application.ErrInvalidTranscription
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := readTranscription(tx, uuid.Nil, id.JobID, true)
		if err != nil {
			return err
		}
		if err := ownTranscription(row, id); err != nil {
			return err
		}
		if row.Status == "cancel_requested" || row.Status == "cancelled" {
			return application.ErrCancelled
		}
		if row.Status != "running" || row.InferenceState == "unknown" {
			return application.ErrConflict
		}
		row.Progress = max(row.Progress, percentage)
		row.Stage = stage
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		return updateTranscription(tx, row)
	})
}

// StartInference is the atomic dispatch gate against cancellation before HTTP submission.
func (s *TranscriptionStore) StartInference(ctx context.Context, id application.TranscriptionWorkID) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := s.lockAuthorizedTranscription(ctx, tx, id.JobID)
		if err != nil {
			return err
		}
		if err := ownTranscription(row, id); err != nil {
			return err
		}
		if row.Status == "cancel_requested" {
			return application.ErrCancelled
		}
		if row.Status != "running" || row.InferenceState == "submitted" || row.InferenceState == "unknown" {
			return application.ErrConflict
		}
		row.InferenceState = "submitted"
		row.Stage = "transcribing"
		row.Progress = 40
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		return updateTranscription(tx, row)
	})
}

// EndInference records a terminal native response or a proved connection failure
// before any request was sent. Neither client detachment nor timeout qualifies.
func (s *TranscriptionStore) EndInference(ctx context.Context, id application.TranscriptionWorkID) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := readTranscription(tx, uuid.Nil, id.JobID, true)
		if err != nil {
			return err
		}
		if err := ownTranscription(row, id); err != nil {
			return err
		}
		if row.InferenceState != "submitted" && row.InferenceState != "terminal" {
			return application.ErrConflict
		}
		row.InferenceState = "terminal"
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		return updateTranscription(tx, row)
	})
}

// Complete persists exact validated cues, or discards them after a soft cancellation.
func (s *TranscriptionStore) Complete(ctx context.Context, id application.TranscriptionWorkID, draft domain.Transcript) (domain.TranscriptionJob, error) {
	var job domain.TranscriptionJob
	if s == nil || s.db == nil {
		return job, application.ErrUnavailable
	}
	if draft.Validate() != nil {
		return job, application.ErrInvalidTranscription
	}
	raw, err := json.Marshal(draft)
	if err != nil {
		return job, err
	}
	digest := sha256.Sum256(raw)
	sha := hex.EncodeToString(digest[:])
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		candidate, err := readTranscription(tx, uuid.Nil, id.JobID, false)
		if err != nil {
			return err
		}
		var authorizationErr error
		if candidate.Status == "running" {
			authorizationErr = s.authorize(ctx, tx, candidate.actor(), candidate.ProjectID, true)
		}
		row, err := lockTranscriptionBinding(tx, candidate)
		if err != nil {
			return err
		}
		if err := ownTranscription(row, id); err != nil {
			return err
		}
		if row.Status == "succeeded" {
			if row.ResultSHA256 == nil || *row.ResultSHA256 != sha {
				return application.ErrConflict
			}
			job = row.public()
			return nil
		}
		if row.InferenceState != "terminal" {
			return application.ErrConflict
		}
		if row.Status == "cancel_requested" {
			row.Status = "cancelled"
			row.Stage = "cancelled"
			row.Result = nil
			row.ResultSHA256 = nil
			row.FailureCode = nil
		} else {
			if row.Status != "running" {
				return application.ErrConflict
			}
			// A cancellation or terminal replay needs no new authority. A row
			// that became running must retry with authority acquired first.
			if candidate.Status != "running" {
				return application.ErrConflict
			}
			if authorizationErr != nil {
				return authorizationErr
			}
			if err := s.validateSource(ctx, tx, row.actor(), row); err != nil {
				return err
			}
			row.Result = raw
			if err := s.acceptTranscript(ctx, tx, row); err != nil {
				return err
			}
			row.Status = "succeeded"
			row.Stage = "complete"
			row.Progress = 100
			row.Result = raw
			row.ResultSHA256 = &sha
			row.FailureCode = nil
		}
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		if err := updateTranscription(tx, row); err != nil {
			return err
		}
		job = row.public()
		action := "completed"
		if row.Status == "cancelled" {
			action = "cancelled"
		}
		return transcriptionRuntimeAudit(tx, row, action)
	})
	return job, err
}

// Finish distinguishes local return from uncertain native completion; unknown
// work blocks retry and lifecycle mutations until separate reconciliation evidence.
func (s *TranscriptionStore) Finish(ctx context.Context, id application.TranscriptionWorkID, ended bool, code string) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := readTranscription(tx, uuid.Nil, id.JobID, true)
		if err != nil {
			return err
		}
		if err := ownTranscription(row, id); err != nil {
			return err
		}
		if row.Status == "succeeded" || row.Status == "failed" || row.Status == "cancelled" {
			return nil
		}
		if !ended && row.InferenceState == "unknown" && row.Stage == "awaiting_reconciliation" {
			return nil
		}
		action := "failed"
		switch {
		case !ended:
			row.InferenceState = "unknown"
			row.Stage = "awaiting_reconciliation"
			code = "inference_unknown"
			row.FailureCode = &code
			action = "uncertain"
		case row.Status == "cancel_requested":
			if row.InferenceState == "submitted" {
				row.InferenceState = "terminal"
			}
			row.Status = "cancelled"
			row.Stage = "cancelled"
			row.FailureCode = nil
			action = "cancelled"
		default:
			if row.InferenceState == "submitted" {
				row.InferenceState = "terminal"
			}
			row.Status = "failed"
			row.Stage = "failed"
			row.FailureCode = &code
		}
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		if err := updateTranscription(tx, row); err != nil {
			return err
		}
		return transcriptionRuntimeAudit(tx, row, action)
	})
}

// Release proves only that this actual activity has returned, not that an unknown
// disconnected server computation has ended.
func (s *TranscriptionStore) Release(ctx context.Context, id application.TranscriptionWorkID) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := readTranscription(tx, uuid.Nil, id.JobID, true)
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
		if row.InferenceState == "submitted" {
			row.InferenceState = "unknown"
			row.Stage = "awaiting_reconciliation"
			code := "inference_unknown"
			row.FailureCode = &code
			row.Revision++
			row.UpdatedAt = time.Now().UTC()
		}
		return updateTranscription(tx, row)
	})
}

// FailWorkflow preserves uncertain inference instead of borrowing a workflow timeout as cessation.
func (s *TranscriptionStore) FailWorkflow(ctx context.Context, id application.TranscriptionWorkID) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := readTranscription(tx, uuid.Nil, id.JobID, true)
		if err != nil {
			return err
		}
		if row.Attempt != id.Attempt {
			return application.ErrConflict
		}
		if row.Status != "queued" && row.Status != "running" && row.Status != "cancel_requested" {
			return nil
		}
		if row.InferenceState == "unknown" && row.Stage == "awaiting_reconciliation" {
			return nil
		}
		if row.ActiveWorker != nil || row.InferenceState == "submitted" || row.InferenceState == "unknown" {
			row.Stage = "awaiting_reconciliation"
			code := "activity_unknown"
			row.FailureCode = &code
		} else {
			if row.Status == "cancel_requested" {
				row.Status = "cancelled"
				row.Stage = "cancelled"
			} else {
				row.Status = "failed"
				row.Stage = "failed"
				code := "activity_failed"
				row.FailureCode = &code
			}
		}
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		return updateTranscription(tx, row)
	})
}

// AuthorizeObjectWrite rechecks the live durable actor after native termination,
// without discarding the terminal/unknown receipt or granting a second inference.
func (s *TranscriptionStore) AuthorizeObjectWrite(ctx context.Context, id application.TranscriptionWorkID) error {
	if s == nil || s.db == nil || id.ExecutionID == uuid.Nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := s.lockAuthorizedTranscription(ctx, tx, id.JobID)
		if err != nil {
			return err
		}
		if err = ownTranscription(row, id); err != nil {
			return err
		}
		if row.Status == "cancel_requested" || row.Status == "cancelled" {
			return application.ErrCancelled
		}
		if row.Status != "running" || row.InferenceState != "terminal" {
			return application.ErrConflict
		}
		return nil
	})
}
