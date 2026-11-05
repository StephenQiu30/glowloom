package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	task "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

const transcriptionTopic = "glowloom.mediatool.transcription_command.v1"

func recordTranscriptionCommand(tx *gorm.DB, actor identityapp.Principal, key uuid.UUID, hash, action string, job domain.TranscriptionJob, emit string) error {
	now := time.Now().UTC()
	response, err := json.Marshal(job)
	if err != nil {
		return err
	}
	var eventID *uuid.UUID
	var eventAction *string
	var eventAttempt *int
	if emit != "" {
		id := uuid.NewSHA1(key, []byte("media-transcription/"+actor.ID.String()+"/"+emit))
		eventID = &id
		eventAction = &emit
		eventAttempt = &job.Attempt
		delivery := application.TranscriptionDelivery{TranscriptionWorkID: application.TranscriptionWorkID{JobID: job.ID, Attempt: job.Attempt}, EventID: id, RequestID: key, ActorID: actor.ID, OrgID: actor.OrgID, ProjectID: job.ProjectID, Action: emit}
		payload, err := json.Marshal(struct {
			EventID    uuid.UUID                         `json:"event_id"`
			EventType  string                            `json:"event_type"`
			OccurredAt time.Time                         `json:"occurred_at"`
			OrgID      uuid.UUID                         `json:"org_id"`
			Data       application.TranscriptionDelivery `json:"data"`
		}{id, transcriptionTopic, now, actor.OrgID, delivery})
		if err != nil {
			return err
		}
		if emit == "cancel" {
			if err := taskruntime.RequestStop(tx.Statement.Context, tx, task.Transcription, map[string]any{"job_id": job.ID, "attempt": job.Attempt}); err != nil {
				return err
			}
		}
		if err := taskruntime.EnqueueReference(tx.Statement.Context, tx, task.Transcription, id.String(), time.Now().UTC(), delivery.TranscriptionWorkID); err != nil {
			return err
		}
		if err := tx.Create(&mediatoolOutboxRecord{ID: id, Topic: transcriptionTopic, PartitionKey: actor.OrgID.String(), Payload: jsonDocument(payload)}).Error; err != nil {
			return err
		}
	}
	if err := tx.Create(&transcriptionCommandRecord{ActorID: actor.ID, RequestID: key, JobID: job.ID, RequestHash: hash, Response: jsonDocument(response), EventID: eventID, EventAction: eventAction, EventAttempt: eventAttempt, CreatedAt: now}).Error; err != nil {
		return err
	}
	return recordTranscriptionAudit(tx, actor, key, action, job)
}
func recordTranscriptionAudit(tx *gorm.DB, actor identityapp.Principal, key uuid.UUID, action string, job domain.TranscriptionJob) error {
	now := time.Now().UTC()
	summary := map[string]any{"id": job.ID, "project_id": job.ProjectID, "canvas_id": job.Source.CanvasID, "node_id": job.Source.NodeID, "source_revision": job.Source.Revision, "language": job.Language, "status": job.Status, "stage": job.Stage, "progress": job.Progress, "attempt": job.Attempt, "revision": job.Revision, "result_sha256": job.ResultSHA256, "failure_code": job.FailureCode, "created_at": job.CreatedAt.Format(time.RFC3339Nano), "updated_at": job.UpdatedAt.Format(time.RFC3339Nano)}
	auditID := uuid.NewSHA1(key, []byte("media-transcription-audit/"+actor.ID.String()+"/"+action))
	const topic = "glowloom.audit.recorded.v1"
	payload, err := json.Marshal(map[string]any{"event_id": auditID, "event_type": topic, "occurred_at": now, "org_id": actor.OrgID, "project_id": job.ProjectID, "actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "audit", "id": auditID}, "data": map[string]any{"action": "media.transcription_" + action, "object": map[string]any{"type": "media_transcription", "id": job.ID.String()}, "before": nil, "after": summary, "request_id": key.String()}})
	if err != nil {
		return err
	}
	return tx.Create(&mediatoolOutboxRecord{ID: auditID, Topic: topic, PartitionKey: job.ProjectID.String(), Payload: jsonDocument(payload)}).Error
}

// Control records soft cancellation or an explicit retry only after known cessation.
func (s *TranscriptionStore) Control(ctx context.Context, actor identityapp.Principal, project, id, key uuid.UUID, revision int64, action string) (domain.TranscriptionJob, error) {
	var job domain.TranscriptionJob
	if s == nil || s.db == nil {
		return job, application.ErrUnavailable
	}
	if revision < 1 || (action != "cancel" && action != "retry") {
		return job, application.ErrInvalidTranscription
	}
	hash, err := commandHash("transcription_"+action, project, id, revision)
	if err != nil {
		return job, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, project, true); err != nil {
			return err
		}
		if err := transcriptionLock(tx, actor.ID, key); err != nil {
			return err
		}
		previous, found, err := transcriptionReplay(tx, actor.ID, key, hash)
		if err != nil {
			return err
		}
		if found {
			job = previous
			return nil
		}
		if action == "retry" && s.source == nil {
			return application.ErrUnavailable
		}
		if action == "retry" {
			if err := transcriptionQuota(tx, project); err != nil {
				return err
			}
		}
		row, err := readTranscription(tx, project, id, true)
		if err != nil {
			return err
		}
		if row.Revision != revision {
			return application.ErrConflict
		}
		emit := "cancel"
		if action == "cancel" {
			if row.Status != domain.TranscriptionQueued && row.Status != domain.TranscriptionRunning {
				return application.ErrConflict
			}
			row.Status = domain.TranscriptionCancelRequested
			row.Stage = domain.TranscriptionStageCancelling
			if row.InferenceState == domain.TranscriptionInferenceUnknown {
				row.Stage = domain.TranscriptionStageAwaitingReconciliation
			}
		} else {
			if (row.Status != domain.TranscriptionFailed && row.Status != domain.TranscriptionCancelled) || row.Attempt >= 100 || row.ActiveWorker != nil || row.InferenceState == domain.TranscriptionInferenceSubmitted || row.InferenceState == domain.TranscriptionInferenceUnknown {
				return application.ErrConflict
			}
			if err := s.validateSource(ctx, tx, actor, row); err != nil {
				return err
			}
			row.Status = domain.TranscriptionQueued
			row.Stage = domain.TranscriptionStageQueued
			row.Progress = 0
			row.Attempt++
			if err := pinTool(ctx, tx, s.storage, mediaapp.StorageWorkKindTranscription, row.ProjectID, row.ID, row.Attempt); err != nil {
				return err
			}
			row.InferenceState = domain.TranscriptionInferenceNone
			row.Result = nil
			row.ResultSHA256 = nil
			row.FailureCode = nil
			row.ActorID = actor.ID
			row.ActorRole = nil
			emit = "start"
		}
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		if err := updateTranscription(tx, row); err != nil {
			return err
		}
		job = row.public()
		return recordTranscriptionCommand(tx, actor, key, hash, action, job, emit)
	})
	return job, err
}

// VerifyDelivery rejects injected commands and safely ignores retired attempt events.
func (s *TranscriptionStore) verifyDelivery(ctx context.Context, d application.TranscriptionDelivery) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	if d.EventID == uuid.Nil || d.RequestID == uuid.Nil || d.JobID == uuid.Nil || d.ActorID == uuid.Nil || d.OrgID == uuid.Nil || d.ProjectID == uuid.Nil || d.Attempt < 1 || (d.Action != "start" && d.Action != "cancel") {
		return false, application.ErrInvalidTranscription
	}
	var command transcriptionCommandRecord
	if err := s.db.WithContext(ctx).Where("actor_id = ? AND request_id = ? AND event_id = ? AND event_action = ? AND event_attempt = ? AND job_id = ?", d.ActorID, d.RequestID, d.EventID, d.Action, d.Attempt, d.JobID).Take(&command).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, application.ErrInvalidTranscription
		}
		return false, err
	}
	var ownedJob transcriptionRow
	if err := s.db.WithContext(ctx).Where("id = ? AND project_id = ? AND org_id = ?", d.JobID, d.ProjectID, d.OrgID).Take(&ownedJob).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, application.ErrInvalidTranscription
		}
		return false, err
	}
	if command.EventAction == nil || *command.EventAction != d.Action || command.EventAttempt == nil || *command.EventAttempt != d.Attempt {
		return false, application.ErrInvalidTranscription
	}
	row, err := readTranscription(s.db.WithContext(ctx), uuid.Nil, d.JobID, false)
	if err != nil {
		return false, err
	}
	if row.Attempt != d.Attempt {
		return false, nil
	}
	if d.Action == "cancel" {
		return row.Status == domain.TranscriptionCancelRequested, nil
	}
	return (row.Status == domain.TranscriptionQueued || row.Status == domain.TranscriptionRunning || row.Status == domain.TranscriptionCancelRequested) && row.InferenceState != domain.TranscriptionInferenceUnknown, nil
}
func transcriptionRuntimeAudit(tx *gorm.DB, row transcriptionRow, action string) error {
	key := uuid.NewSHA1(row.ID, []byte(fmt.Sprintf("transcription-%d-%s", row.Attempt, action)))
	return recordTranscriptionAudit(tx, row.actor(), key, action, row.public())
}

// VerifyDelivery retains live scope authority until durable command validation completes.
func (s *TranscriptionStore) VerifyDelivery(ctx context.Context, d application.TranscriptionDelivery) (bool, error) {
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
		row, err := readTranscription(tx, d.ProjectID, d.JobID, false)
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
