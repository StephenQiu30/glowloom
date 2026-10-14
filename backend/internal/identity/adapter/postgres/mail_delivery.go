package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// MailStore uses the existing database; it does not create a background service.
type MailStore struct {
	db          *gorm.DB
	operationID *uuid.UUID
}

func NewMailStore(db *gorm.DB) *MailStore { return &MailStore{db: db} }

// NewOperationMailStore bounds an operator retry to one explicitly selected flow.
func NewOperationMailStore(db *gorm.DB, operationID uuid.UUID) *MailStore {
	return &MailStore{db: db, operationID: &operationID}
}

func (s *MailStore) Claim(ctx context.Context, limit int, now time.Time) ([]application.MailJob, error) {
	var jobs []application.MailJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Superseded/consumed proofs must not remain queued after a resend or
		// completion. An already submitted message cannot be recalled; the proof
		// itself is independently checked when redeemed.
		if err := tx.Exec(`UPDATE identity.mail_delivery m SET status='expired',encrypted_payload=NULL,key_id=NULL,lease_until=NULL,updated_at=?
		 FROM identity.auth_challenge c WHERE m.challenge_id=c.id AND (m.status='pending' OR (m.status='leased' AND m.lease_until<=?))
		 AND (c.revoked_at IS NOT NULL OR c.consumed_at IS NOT NULL OR c.expires_at<=?)`, now, now, now).Error; err != nil {
			return err
		}
		// Claim both expired and live rows so the executor clears terminal secrets.
		filter := ""
		args := []any{now, now}
		if s.operationID != nil {
			filter = " AND challenge_id IN (SELECT id FROM identity.auth_challenge WHERE operation_id=?)"
			args = append(args, *s.operationID)
		}
		args = append(args, limit, now.Add(2*time.Minute), now)
		return tx.Raw(`WITH due AS (
		 SELECT event_id FROM identity.mail_delivery WHERE ((status='pending' AND next_attempt_at<=?) OR (status='leased' AND lease_until<=?))`+filter+`
		 ORDER BY next_attempt_at,event_id LIMIT ? FOR UPDATE SKIP LOCKED)
		 UPDATE identity.mail_delivery m SET status='leased',attempt_count=attempt_count+1,lease_until=?,updated_at=?
		 FROM due WHERE m.event_id=due.event_id RETURNING m.event_id,m.template,m.recipient,m.key_id,m.encrypted_payload,m.expires_at,m.lease_until,m.attempt_count`, args...).Scan(&jobs).Error
	})
	if err != nil {
		return nil, fmt.Errorf("claim identity mail: %w", err)
	}
	return jobs, nil
}

func (s *MailStore) Complete(ctx context.Context, job application.MailJob, result application.MailCompletion) error {
	if result.Status != "pending" && result.Status != "submitted" && result.Status != "failed" && result.Status != "expired" {
		return fmt.Errorf("invalid mail completion state")
	}
	r := s.db.WithContext(ctx).Exec(`UPDATE identity.mail_delivery SET status=?,provider_message_id=NULLIF(?,''),error_category=NULLIF(?,''),next_attempt_at=?,updated_at=?,lease_until=NULL,
	 encrypted_payload=CASE WHEN ?='pending' THEN encrypted_payload ELSE NULL END,key_id=CASE WHEN ?='pending' THEN key_id ELSE NULL END
	 WHERE event_id=? AND status='leased' AND attempt_count=? AND lease_until=?`, result.Status, result.ProviderMessageID, result.ErrorCategory, result.NextAttemptAt, result.CompletedAt, result.Status, result.Status, job.EventID, job.AttemptCount, job.LeaseUntil)
	if r.Error != nil {
		return fmt.Errorf("complete identity mail: %w", r.Error)
	}
	// A late completion cannot overwrite a newer lease or terminal result.
	return nil
}
