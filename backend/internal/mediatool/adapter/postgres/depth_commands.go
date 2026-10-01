package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

const depthTopic = "lanverse.mediatool.depth_command.v1"

func recordDepthCommand(tx *gorm.DB, actor identityapp.Principal, key uuid.UUID, hash, action string, job domain.DepthJob, emit string) error {
	now := time.Now().UTC()
	body, err := json.Marshal(job)
	if err != nil {
		return err
	}
	var eventID *uuid.UUID
	var eventAction *string
	var eventAttempt *int
	if emit != "" {
		id := uuid.NewSHA1(key, []byte("media-depth/"+actor.ID.String()+"/"+emit))
		eventID = &id
		eventAction = &emit
		eventAttempt = &job.Attempt
		d := application.DepthDelivery{DepthWorkID: application.DepthWorkID{JobID: job.ID, Attempt: job.Attempt, Reconcile: emit == "reconcile"}, EventID: id, RequestID: key, ActorID: actor.ID, OrgID: actor.OrgID, ProjectID: job.ProjectID, Action: emit}
		payload, err := json.Marshal(struct {
			EventID    uuid.UUID                 `json:"event_id"`
			EventType  string                    `json:"event_type"`
			OccurredAt time.Time                 `json:"occurred_at"`
			OrgID      uuid.UUID                 `json:"org_id"`
			Data       application.DepthDelivery `json:"data"`
		}{id, depthTopic, now, actor.OrgID, d})
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, id, depthTopic, job.ID.String(), string(payload)).Error; err != nil {
			return err
		}
	}
	if err := tx.Exec(`INSERT INTO mediatool.depth_command(actor_id,request_id,job_id,request_hash,response,event_id,event_action,event_attempt,created_at) VALUES(?,?,?,?,?::jsonb,?,?,?,?)`, actor.ID, key, job.ID, hash, string(body), eventID, eventAction, eventAttempt, now).Error; err != nil {
		return err
	}
	return depthAudit(tx, actor, key, action, job)
}
func depthAudit(tx *gorm.DB, actor identityapp.Principal, key uuid.UUID, action string, j domain.DepthJob) error {
	id := uuid.NewSHA1(key, []byte("media-depth-audit/"+actor.ID.String()+"/"+action))
	const topic = "lanverse.audit.recorded.v1"
	summary := map[string]any{"id": j.ID, "project_id": j.ProjectID, "canvas_id": j.Source.CanvasID, "node_id": j.Source.NodeID, "source_revision": j.Source.Revision, "source_asset_id": j.SourceAssetID, "source_asset_revision": j.SourceAssetRevision, "profile_id": j.ProfileID, "status": j.Status, "stage": j.Stage, "attempt": j.Attempt, "revision": j.Revision, "asset_id": j.AssetID, "sha256": j.SHA256, "failure_code": j.FailureCode, "needs_reconciliation": j.NeedsReconciliation, "execution_unconfirmed": j.ExecutionUnconfirmed}
	data, err := json.Marshal(map[string]any{"event_id": id, "event_type": topic, "occurred_at": time.Now().UTC(), "org_id": actor.OrgID, "project_id": j.ProjectID, "actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "audit", "id": id}, "data": map[string]any{"action": "media.depth_" + action, "object": map[string]any{"type": "media_depth", "id": j.ID.String()}, "before": nil, "after": summary, "request_id": key.String()}})
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, id, topic, j.ProjectID.String(), string(data)).Error
}

// Control atomically saves a revision-bound command, keeping unknown work fenced.
func (s *DepthStore) Control(ctx context.Context, actor identityapp.Principal, project, id, key uuid.UUID, revision int64, action string) (domain.DepthJob, error) {
	var job domain.DepthJob
	if s == nil || s.db == nil {
		return job, application.ErrUnavailable
	}
	if revision < 1 || (action != "cancel" && action != "retry" && action != "reconcile") {
		return job, application.ErrInvalidDepthInput
	}
	hash, err := commandHash("depth_"+action, project, id, revision)
	if err != nil {
		return job, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, project, true); err != nil {
			return err
		}
		if err := depthCommandLock(tx, actor.ID, key); err != nil {
			return err
		}
		prior, found, err := depthReplay(tx, actor.ID, key, hash)
		if err != nil {
			return err
		}
		if found {
			job = prior
			return nil
		}
		r, err := readDepth(tx, project, id, true)
		if err != nil {
			return err
		}
		if r.OrgID != actor.OrgID {
			return application.ErrNotFound
		}
		if r.Revision != revision {
			return application.ErrConflict
		}
		before := r
		state := r.state()
		emit := action
		switch action {
		case "cancel":
			if err := state.RequestCancel(); err != nil {
				return application.ErrConflict
			}
		case "retry":
			if s.source == nil {
				return application.ErrUnavailable
			}
			if err := depthQuota(tx, project); err != nil {
				return err
			}
			if err := s.validateSource(ctx, tx, r); err != nil {
				return err
			}
			if err := state.Retry(); err != nil {
				return application.ErrConflict
			}
			emit = "start"
		case "reconcile":
			if r.Status != "failed" || !r.NeedsReconciliation || r.ExecutionUnconfirmed || r.ActiveWorker != nil || r.ProcessState != "ended" || r.ReconciliationRequested {
				return application.ErrConflict
			}
			r.ReconciliationRequested = true
			state.Job.Revision++
		}
		r.apply(state)
		if err := saveDepth(tx, before, r); err != nil {
			return err
		}
		job = r.job()
		return recordDepthCommand(tx, actor, key, hash, action, job, emit)
	})
	return job, err
}

// Review publishes exact inspected bytes through the media owner in one transaction.
func (s *DepthStore) Review(ctx context.Context, actor identityapp.Principal, id, key uuid.UUID, in application.ReviewInput) (domain.DepthJob, error) {
	var job domain.DepthJob
	if s == nil || s.db == nil {
		return job, application.ErrUnavailable
	}
	if in.ProjectID == uuid.Nil || in.Revision < 1 || !domain.ValidDepthSHA(in.SHA256) || !in.LocalReviewConfirmed {
		return job, application.ErrInvalidDepthInput
	}
	hash, err := commandHash("depth_review", in.ProjectID, id, in)
	if err != nil {
		return job, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, in.ProjectID, true); err != nil {
			return err
		}
		if err := depthCommandLock(tx, actor.ID, key); err != nil {
			return err
		}
		prior, found, err := depthReplay(tx, actor.ID, key, hash)
		if err != nil {
			return err
		}
		if found {
			job = prior
			return nil
		}
		r, err := readDepth(tx, in.ProjectID, id, true)
		if err != nil {
			return err
		}
		if r.OrgID != actor.OrgID {
			return application.ErrNotFound
		}
		if r.Revision != in.Revision || r.Status != "review_required" || r.CancellationRequested || r.ExecutionUnconfirmed || r.ActiveWorker != nil {
			return application.ErrConflict
		}
		a, found, err := loadDepthArtifact(tx, r)
		if err != nil {
			return err
		}
		if !found || a.Asset.SHA256 == nil || *a.Asset.SHA256 != in.SHA256 {
			return application.ErrConflict
		}
		if err := s.validateSource(ctx, tx, r); err != nil {
			return err
		}
		if err := verifyDepthObjects(tx, r, "verified"); err != nil {
			return err
		}
		if err := s.media(tx).Review(ctx, actor, in.ProjectID, a.Asset.ID, in.SHA256, time.Now().UTC()); err != nil {
			return normalize(err)
		}
		before := r
		r.Status = "succeeded"
		r.Stage = "complete"
		r.Revision++
		r.Retryable = false
		r.UpdatedAt = time.Now().UTC()
		if err := saveDepth(tx, before, r); err != nil {
			return err
		}
		job = r.job()
		job.AssetID = &a.Asset.ID
		job.SHA256 = a.Asset.SHA256
		return recordDepthCommand(tx, actor, key, hash, "reviewed", job, "")
	})
	return job, err
}
