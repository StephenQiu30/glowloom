package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
)

func (s *VoiceStore) worker(ctx context.Context, tx *gorm.DB, id uuid.UUID, lock bool) (application.VoiceWork, error) {
	row, err := readVoiceRow(tx, id, lock)
	if err != nil {
		return application.VoiceWork{}, err
	}
	work, err := row.work()
	if err != nil {
		return work, err
	}
	if s.factories.Access == nil || s.factories.Storage == nil {
		return work, application.ErrVoiceUnavailable
	}
	owner, err := s.factories.Access(tx).Worker(ctx, work)
	if err != nil {
		return work, err
	}
	if owner != work.Owner {
		return work, application.ErrVoiceJobNotFound
	}
	storage, err := s.factories.Storage(tx).Read(ctx, work.Owner.ScopeID, work.Job.ID)
	if err != nil {
		return work, err
	}
	if storage.ID != work.Storage.ID {
		return work, application.ErrVoiceUnavailable
	}
	work.Storage = storage
	return work, nil
}

// Work reauthorizes both target space and the actual personal output owner.
func (s *VoiceStore) Work(ctx context.Context, id uuid.UUID) (application.VoiceWork, error) {
	var out application.VoiceWork
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var err error; out, err = s.worker(ctx, tx, id, false); return err })
	return out, err
}

// Claim is the permanent paid-send fence, serialized with provider capacity.
func (s *VoiceStore) Claim(ctx context.Context, id, execution uuid.UUID) (application.VoiceWork, bool, error) {
	var out application.VoiceWork
	claimed := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		out, err = s.worker(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if out.DispatchStartedAt != nil || out.Job.Status != "queued" {
			return nil
		}
		if execution == uuid.Nil {
			return application.ErrInvalidVoiceJob
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "provider-voice/"+out.ProviderID.String()).Error; err != nil {
			return err
		}
		var limits struct{ ConcurrencyLimit, RateLimitPerMin int }
		r := tx.Raw(`SELECT concurrency_limit,rate_limit_per_min FROM catalog.provider WHERE id=? AND status='active' AND NOT is_delete FOR UPDATE`, out.ProviderID).Scan(&limits)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 || limits.ConcurrencyLimit < 1 || limits.RateLimitPerMin < 1 {
			return application.ErrVoiceUnavailable
		}
		if _, err := s.factories.Catalog(tx).Credential(ctx, out); err != nil {
			return err
		}
		var active, recent int64
		if err := tx.Raw(`SELECT count(*) FROM subject.voice_job WHERE provider_id=? AND status IN ('dispatching','unknown')`, out.ProviderID).Scan(&active).Error; err != nil {
			return err
		}
		if err := tx.Raw(`SELECT count(*) FROM subject.voice_job WHERE provider_id=? AND dispatch_started_at>clock_timestamp()-interval '1 minute'`, out.ProviderID).Scan(&recent).Error; err != nil {
			return err
		}
		var operations struct{ Active, Recent int64 }
		if err := tx.Raw(`SELECT count(DISTINCT op.id) FILTER(WHERE op.status NOT IN ('completed','failed','cancelled','expired')) AS active,count(DISTINCT op.id) FILTER(WHERE call.dispatch_started_at>now()-interval '1 minute') AS recent FROM operation.provider_credential_binding cb JOIN catalog.provider_credential key ON key.id=cb.credential_id JOIN operation.operation op ON op.id=cb.operation_id JOIN operation.provider_call call ON call.operation_id=op.id AND call.action='submit' AND call.dispatch_started_at IS NOT NULL AND NOT call.is_delete WHERE key.provider_id=?`, out.ProviderID).Scan(&operations).Error; err != nil {
			return err
		}
		active += operations.Active
		recent += operations.Recent
		if active >= int64(limits.ConcurrencyLimit) || recent >= int64(limits.RateLimitPerMin) {
			return application.ErrVoiceUnavailable
		}
		r = tx.Exec(`UPDATE subject.voice_job SET status='dispatching',revision=revision+1,dispatch_started_at=clock_timestamp(),execution_id=?,updated_at=clock_timestamp() WHERE id=? AND status='queued' AND dispatch_started_at IS NULL`, execution, id)
		if r.Error != nil {
			return r.Error
		}
		claimed = r.RowsAffected == 1
		out, err = s.worker(ctx, tx, id, false)
		if err != nil {
			return err
		}
		return auditVoice(tx, out, "dispatch_started")
	})
	return out, claimed, err
}

// Credential checks emergency disable and the exact frozen key immediately before send.
func (s *VoiceStore) Credential(ctx context.Context, work application.VoiceWork) (application.VoiceCredential, error) {
	var out application.VoiceCredential
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := s.worker(ctx, tx, work.Job.ID, false)
		if err != nil {
			return err
		}
		if current.ExecutionID != work.ExecutionID || current.Job.Status != "dispatching" {
			return application.ErrVoiceExecutionUnknown
		}
		out, err = s.factories.Catalog(tx).Credential(ctx, current)
		return err
	})
	return out, err
}

// RecordReceipt durably freezes terminal success before any retryable ingest.
func (s *VoiceStore) RecordReceipt(ctx context.Context, work application.VoiceWork, receipt application.VoiceReceipt) (application.VoiceWork, error) {
	var out application.VoiceWork
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		out, err = s.worker(ctx, tx, work.Job.ID, true)
		if err != nil {
			return err
		}
		if receipt.Validate(out) != nil || receipt.AudioLocationID == uuid.Nil || receipt.ManifestLocationID != uuid.Nil {
			return application.ErrVoiceExecutionUnknown
		}
		if out.Receipt != nil {
			if out.Receipt.Digest() != receipt.Digest() {
				return application.ErrVoiceExecutionUnknown
			}
			return nil
		}
		if (out.Job.Status != "dispatching" && out.Job.Status != "unknown") || out.ExecutionID != work.ExecutionID || out.ExecutionID == uuid.Nil {
			return application.ErrVoiceExecutionUnknown
		}
		var count int64
		if err := tx.Raw(`SELECT count(*) FROM media.object_location WHERE id=? AND owner_intent_id=? AND profile_id=? AND bucket=? AND object_key=? AND state='present' AND sha256=? AND byte_size=? AND mime_type='audio/mpeg'`, receipt.AudioLocationID, out.Storage.ID, out.Storage.ProfileID, out.Storage.Bucket, receipt.AudioKey, receipt.AudioSHA256, receipt.AudioByteSize).Scan(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return application.ErrVoiceExecutionUnknown
		}
		usage := receipt.Usage
		if len(usage) == 0 {
			usage = json.RawMessage(`{}`)
		}
		if err := tx.Exec(`UPDATE subject.voice_job SET status='ingesting',revision=revision+1,audio_location_id=?,audio_sha256=?,audio_byte_size=?,provider_log_id=?,usage=?::jsonb,error_code='',updated_at=clock_timestamp() WHERE id=?`, receipt.AudioLocationID, receipt.AudioSHA256, receipt.AudioByteSize, receipt.ProviderLogID, string(usage), out.Job.ID).Error; err != nil {
			return err
		}
		out, err = s.worker(ctx, tx, out.Job.ID, false)
		if err != nil {
			return err
		}
		return auditVoice(tx, out, "receipt_recorded")
	})
	return out, err
}

func (s *VoiceStore) mark(ctx context.Context, work application.VoiceWork, status, code string) (application.VoiceJob, error) {
	var out application.VoiceJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := s.worker(ctx, tx, work.Job.ID, true)
		if err != nil {
			return err
		}
		out = current.Job
		if current.Receipt != nil || current.Job.Status == "succeeded" || current.Job.Status == "failed" || current.Job.Status == status {
			return nil
		}
		if current.ExecutionID != work.ExecutionID || current.ExecutionID == uuid.Nil {
			return application.ErrVoiceExecutionUnknown
		}
		if err := tx.Exec(`UPDATE subject.voice_job SET status=?,error_code=?,revision=revision+1,updated_at=clock_timestamp(),completed_at=CASE WHEN ?='failed' THEN clock_timestamp() ELSE NULL END WHERE id=?`, status, code, status, current.Job.ID).Error; err != nil {
			return err
		}
		row, err := readVoiceRow(tx, current.Job.ID, false)
		if err != nil {
			return err
		}
		next, err := row.work()
		out = next.Job
		if err != nil {
			return err
		}
		return auditVoice(tx, next, status)
	})
	return out, err
}

// MarkUnknown preserves an irreversible uncertain call without resubmission.
func (s *VoiceStore) MarkUnknown(ctx context.Context, w application.VoiceWork, code string) (application.VoiceJob, error) {
	return s.mark(ctx, w, "unknown", code)
}

// MarkFailed records a proven pre-submit or explicit supplier rejection.
func (s *VoiceStore) MarkFailed(ctx context.Context, w application.VoiceWork, code string) (application.VoiceJob, error) {
	return s.mark(ctx, w, "failed", code)
}

// CompleteWith joins real media acceptance and job success in one SQL transaction.
func (s *VoiceStore) CompleteWith(ctx context.Context, work application.VoiceWork, receipt application.VoiceReceipt, accept func(*gorm.DB) (application.VoiceJobResult, error)) (application.VoiceJob, error) {
	var out application.VoiceJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := s.worker(ctx, tx, work.Job.ID, true)
		if err != nil {
			return err
		}
		out = current.Job
		if current.Receipt == nil || current.Receipt.Digest() != receipt.Digest() {
			return application.ErrVoiceExecutionUnknown
		}
		if out.Status == "succeeded" {
			return nil
		}
		if out.Status != "ingesting" || accept == nil {
			return application.ErrVoiceExecutionUnknown
		}
		result, err := accept(tx)
		if err != nil {
			return err
		}
		if result.AssetID != current.AssetID || result.AssetRevision < 1 || result.LibraryItemID != current.AssetID || result.Kind != "audio" || result.MIMEType != "audio/mpeg" || result.LibrarySource != "subject" {
			return application.ErrVoiceExecutionUnknown
		}
		raw, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE subject.voice_job SET status='succeeded',output_asset_id=?,result=?::jsonb,revision=revision+1,error_code='',updated_at=clock_timestamp(),completed_at=clock_timestamp() WHERE id=?`, result.AssetID, string(raw), out.ID).Error; err != nil {
			return err
		}
		row, err := readVoiceRow(tx, out.ID, false)
		if err != nil {
			return err
		}
		next, err := row.work()
		out = next.Job
		if err != nil {
			return err
		}
		return auditVoice(tx, next, "succeeded")
	})
	return out, err
}

// VerifyDelivery rejects injected messages unless their exact committed outbox exists.
func (s *VoiceStore) VerifyDelivery(ctx context.Context, d application.VoiceDelivery) (bool, error) {
	if d.EventID == uuid.Nil || d.JobID == uuid.Nil || d.ActorID == uuid.Nil || d.ScopeID == uuid.Nil {
		return false, application.ErrInvalidVoiceJob
	}
	var valid bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		w, err := s.worker(ctx, tx, d.JobID, false)
		if err != nil {
			return err
		}
		if w.Job.CurrentActorID != d.ActorID || w.Owner.ScopeID != d.ScopeID {
			return application.ErrVoiceJobNotFound
		}
		var raw struct{ Payload []byte }
		r := tx.Raw(`SELECT payload FROM infra.outbox WHERE id=? AND topic=? AND partition_key=?`, d.EventID, application.VoiceTopic, d.JobID.String()).Scan(&raw)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrInvalidVoiceJob
		}
		var body struct {
			Data application.VoiceDelivery `json:"data"`
		}
		if json.Unmarshal(raw.Payload, &body) != nil || body.Data != d {
			return application.ErrInvalidVoiceJob
		}
		valid = w.Job.Status != "succeeded" && w.Job.Status != "failed"
		return nil
	})
	return valid, err
}

// Recoverable returns original queued deliveries and fresh ingest-only deliveries.
func (s *VoiceStore) Recoverable(ctx context.Context, limit int) ([]application.VoiceDelivery, error) {
	if limit < 1 || limit > 120 {
		return nil, application.ErrInvalidVoiceJob
	}
	out := []application.VoiceDelivery{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []uuid.UUID
		if err := tx.Raw(`SELECT id FROM subject.voice_job WHERE status IN ('queued','ingesting') AND updated_at<? ORDER BY updated_at,id LIMIT ?`, time.Now().UTC().Add(-2*time.Minute), limit).Scan(&ids).Error; err != nil {
			return err
		}
		for _, id := range ids {
			w, err := s.worker(ctx, tx, id, true)
			if err != nil {
				continue
			}
			event := uuid.New()
			if err := enqueueVoice(tx, w, event, w.Receipt != nil); err != nil {
				return err
			}
			out = append(out, application.VoiceDelivery{EventID: event, JobID: id, ActorID: w.Job.CurrentActorID, ScopeID: w.Owner.ScopeID, RecoveryOnly: w.Receipt != nil})
		}
		return nil
	})
	return out, err
}

var _ application.VoiceWorkerStore = (*VoiceStore)(nil)
