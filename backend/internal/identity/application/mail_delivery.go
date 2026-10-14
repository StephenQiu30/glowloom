package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrMailOutcomeUnknown means the remote service may have accepted the message.
var ErrMailOutcomeUnknown = errors.New("mail submission outcome unknown")

type MailSender interface {
	Send(context.Context, uuid.UUID, string, string, []byte) (string, error)
}
type MailOpener interface {
	Open(uuid.UUID, string, []byte) ([]byte, error)
}
type MailJob struct {
	EventID                    uuid.UUID
	Template, Recipient, KeyID string
	EncryptedPayload           []byte
	ExpiresAt, LeaseUntil      time.Time
	AttemptCount               int
}
type MailCompletion struct {
	Status, ProviderMessageID, ErrorCategory string
	NextAttemptAt, CompletedAt               time.Time
}
type MailDeliveryStore interface {
	Claim(context.Context, int, time.Time) ([]MailJob, error)
	Complete(context.Context, MailJob, MailCompletion) error
}
type MailDelivery struct {
	store  MailDeliveryStore
	opener MailOpener
	sender MailSender
	now    func() time.Time
}

func NewMailDelivery(store MailDeliveryStore, opener MailOpener, sender MailSender, now func() time.Time) *MailDelivery {
	return &MailDelivery{store: store, opener: opener, sender: sender, now: now}
}

// RunOnce releases database locks before any decryption or SMTP network IO.
func (d *MailDelivery) RunOnce(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		return 0, errors.New("mail batch must contain 1 to 100 jobs")
	}
	count := 0
	for count < limit {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		jobs, err := d.store.Claim(ctx, 1, d.now().UTC())
		if err != nil {
			return count, err
		}
		if len(jobs) == 0 {
			break
		}
		job := jobs[0]
		now := d.now().UTC()
		result := MailCompletion{Status: "expired", CompletedAt: now, NextAttemptAt: now}
		if now.Before(job.ExpiresAt) {
			payload, err := d.opener.Open(job.EventID, job.KeyID, job.EncryptedPayload)
			if err != nil {
				result.Status = "failed"
				result.ErrorCategory = "payload_unavailable"
			} else {
				provider, sendErr := d.sender.Send(ctx, job.EventID, job.Template, job.Recipient, payload)
				clear(payload)
				result.Status = "submitted"
				result.ProviderMessageID = provider
				if sendErr != nil {
					result.Status = "pending"
					result.ErrorCategory = "transport_failed"
					if errors.Is(sendErr, ErrMailOutcomeUnknown) {
						result.ErrorCategory = "outcome_unknown"
					}
					result.NextAttemptAt = d.now().UTC().Add(time.Duration(1<<min(job.AttemptCount, 10)) * time.Second)
					if job.AttemptCount >= 5 {
						result.Status = "failed"
					} else if !result.NextAttemptAt.Before(job.ExpiresAt) {
						result.Status = "expired"
					}
				}
			}
		}
		result.CompletedAt = d.now().UTC()
		if err := d.store.Complete(ctx, job, result); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
