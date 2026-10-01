package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

const transcriptionTopic = "lanverse.mediatool.transcription_command.v1"

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
		if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, id, transcriptionTopic, actor.OrgID.String(), string(payload)).Error; err != nil {
			return err
		}
	}
	if err := tx.Exec(`INSERT INTO mediatool.transcription_command(actor_id,request_id,job_id,request_hash,response,event_id,event_action,event_attempt,created_at) VALUES(?,?,?,?,?::jsonb,?,?,?,?)`, actor.ID, key, job.ID, hash, string(response), eventID, eventAction, eventAttempt, now).Error; err != nil {
		return err
	}
	return recordTranscriptionAudit(tx, actor, key, action, job)
}
func recordTranscriptionAudit(tx *gorm.DB, actor identityapp.Principal, key uuid.UUID, action string, job domain.TranscriptionJob) error {
	now := time.Now().UTC()
	summary := map[string]any{"id": job.ID, "project_id": job.ProjectID, "canvas_id": job.Source.CanvasID, "node_id": job.Source.NodeID, "source_revision": job.Source.Revision, "language": job.Language, "status": job.Status, "stage": job.Stage, "progress": job.Progress, "attempt": job.Attempt, "revision": job.Revision, "result_sha256": job.ResultSHA256, "failure_code": job.FailureCode, "created_at": job.CreatedAt.Format(time.RFC3339Nano), "updated_at": job.UpdatedAt.Format(time.RFC3339Nano)}
	auditID := uuid.NewSHA1(key, []byte("media-transcription-audit/"+actor.ID.String()+"/"+action))
	const topic = "lanverse.audit.recorded.v1"
	payload, err := json.Marshal(map[string]any{"event_id": auditID, "event_type": topic, "occurred_at": now, "org_id": actor.OrgID, "project_id": job.ProjectID, "actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "audit", "id": auditID}, "data": map[string]any{"action": "media.transcription_" + action, "object": map[string]any{"type": "media_transcription", "id": job.ID.String()}, "before": nil, "after": summary, "request_id": key.String()}})
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, auditID, topic, job.ProjectID.String(), string(payload)).Error
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
			if row.Status != "queued" && row.Status != "running" {
				return application.ErrConflict
			}
			row.Status = "cancel_requested"
			row.Stage = "cancelling"
			if row.InferenceState == "unknown" {
				row.Stage = "awaiting_reconciliation"
			}
		} else {
			if (row.Status != "failed" && row.Status != "cancelled") || row.Attempt >= 100 || row.ActiveWorker != nil || row.InferenceState == "submitted" || row.InferenceState == "unknown" {
				return application.ErrConflict
			}
			if err := s.validateSource(ctx, tx, row); err != nil {
				return err
			}
			row.Status = "queued"
			row.Stage = "queued"
			row.Progress = 0
			row.Attempt++
			row.InferenceState = "none"
			row.Result = nil
			row.ResultSHA256 = nil
			row.FailureCode = nil
			row.ActorID = actor.ID
			row.ActorRole = string(actor.Role)
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
func (s *TranscriptionStore) VerifyDelivery(ctx context.Context, d application.TranscriptionDelivery) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	if d.EventID == uuid.Nil || d.RequestID == uuid.Nil || d.JobID == uuid.Nil || d.ActorID == uuid.Nil || d.OrgID == uuid.Nil || d.ProjectID == uuid.Nil || d.Attempt < 1 || (d.Action != "start" && d.Action != "cancel") {
		return false, application.ErrInvalidTranscription
	}
	var exists bool
	err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM mediatool.transcription_command c JOIN mediatool.transcription_job j ON j.id=c.job_id WHERE c.actor_id=? AND c.request_id=? AND c.event_id=? AND c.event_action=? AND c.event_attempt=? AND j.id=? AND j.project_id=? AND j.org_id=?)`, d.ActorID, d.RequestID, d.EventID, d.Action, d.Attempt, d.JobID, d.ProjectID, d.OrgID).Scan(&exists).Error
	if err != nil {
		return false, err
	}
	if !exists {
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
		return row.Status == "cancel_requested", nil
	}
	return (row.Status == "queued" || row.Status == "running" || row.Status == "cancel_requested") && row.InferenceState != "unknown", nil
}
func transcriptionRuntimeAudit(tx *gorm.DB, row transcriptionRow, action string) error {
	key := uuid.NewSHA1(row.ID, []byte(fmt.Sprintf("transcription-%d-%s", row.Attempt, action)))
	return recordTranscriptionAudit(tx, row.actor(), key, action, row.public())
}
