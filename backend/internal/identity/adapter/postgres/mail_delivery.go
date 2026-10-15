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

// NewMailStore constructs MailStore with explicit caller-supplied dependencies.
func NewMailStore(db *gorm.DB) *MailStore { return &MailStore{db: db} }

// NewOperationMailStore bounds an operator retry to one explicitly selected flow.
func NewOperationMailStore(db *gorm.DB, operationID uuid.UUID) *MailStore {
	return &MailStore{db: db, operationID: &operationID}
}

// Claim leases eligible jobs and expires unusable proof messages.
func (s *MailStore) Claim(ctx context.Context, limit int, now time.Time) ([]application.MailJob, error) {
	var jobs []application.MailJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Superseded/consumed proofs must not remain queued after a resend or
		// completion. An already submitted message cannot be recalled; the proof
		// itself is independently checked when redeemed.
		if err := tx.Exec(`UPDATE identity.mail_delivery m SET status='expired',encrypted_payload=NULL,key_id=NULL,lease_until=NULL,updated_at=?
		 FROM identity.auth_challenge c WHERE m.challenge_id=c.id AND (m.status='pending' OR (m.status='leased' AND m.lease_until<=?))
		 AND (c.revoked_at IS NOT NULL OR c.consumed_at IS NOT NULL OR c.expires_at<=? OR (c.invitation_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM identity.invitation i WHERE i.id=c.invitation_id AND i.revision=c.invitation_revision AND i.status='pending' AND i.expires_at>?)))`, now, now, now, now).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE identity.mail_delivery m SET status='expired',encrypted_payload=NULL,key_id=NULL,lease_until=NULL,updated_at=?
          FROM identity.invitation i WHERE m.invitation_id=i.id AND (m.status='pending' OR (m.status='leased' AND m.lease_until<=?))
          AND (i.status<>'pending' OR i.revision<>m.invitation_revision OR i.expires_at<=?)`, now, now, now).Error; err != nil {
			return err
		}
		// Cleanup and claim use separate READ COMMITTED snapshots. Recheck proof
		// validity in the claim statement so an intervening resend/revoke cannot
		// lease an obsolete message. Generic expired jobs still reach the executor
		// to clear their terminal secrets.
		filter := ""
		args := []any{now, now, now, now, now}
		if s.operationID != nil {
			filter = " AND m.challenge_id IN (SELECT id FROM identity.auth_challenge WHERE operation_id=?)"
			args = append(args, *s.operationID)
		}
		args = append(args, limit, now.Add(2*time.Minute), now)
		return tx.Raw(`WITH due AS (
		 SELECT m.event_id FROM identity.mail_delivery m WHERE ((m.status='pending' AND m.next_attempt_at<=?) OR (m.status='leased' AND m.lease_until<=?))
		 AND (m.invitation_id IS NULL OR EXISTS (
		   SELECT 1 FROM identity.invitation i WHERE i.id=m.invitation_id AND i.revision=m.invitation_revision AND i.status='pending' AND i.expires_at>?))
		 AND (m.challenge_id IS NULL OR EXISTS (
		   SELECT 1 FROM identity.auth_challenge c WHERE c.id=m.challenge_id AND c.revoked_at IS NULL AND c.consumed_at IS NULL AND c.expires_at>?
		   AND (c.invitation_id IS NULL OR EXISTS (
		     SELECT 1 FROM identity.invitation i WHERE i.id=c.invitation_id AND i.revision=c.invitation_revision AND i.status='pending' AND i.expires_at>?))))`+filter+`
		 ORDER BY m.next_attempt_at,m.event_id LIMIT ? FOR UPDATE OF m SKIP LOCKED)
		 UPDATE identity.mail_delivery m SET status='leased',attempt_count=attempt_count+1,lease_until=?,updated_at=?
		 FROM due WHERE m.event_id=due.event_id RETURNING m.event_id,m.template,m.recipient,m.key_id,m.encrypted_payload,m.expires_at,m.lease_until,m.attempt_count`, args...).Scan(&jobs).Error
	})
	if err != nil {
		return nil, fmt.Errorf("claim identity mail: %w", err)
	}
	return jobs, nil
}

// Complete updates only the claimed attempt and clears terminal secret payloads.
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
