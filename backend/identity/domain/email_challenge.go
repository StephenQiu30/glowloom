package domain

import (
	"time"

	"github.com/google/uuid"
)

// EmailChallenge contains digests only, never the deliverable proof.
type EmailChallenge struct {
	ID, OperationID               uuid.UUID
	Purpose, EmailKey, ProofKeyID string
	UserID                        *uuid.UUID
	EmailVersion, CredentialEpoch *int64
	ProofDigest, FlowSecretDigest []byte
	Attempts                      int
	ExpiresAt, CreatedAt          time.Time
	ConsumedAt, RevokedAt         *time.Time
	InvitationID                  *uuid.UUID
	InvitationRevision            *int64
}

// Available checks proof expiry, consumption, revocation and attempt budget.
func (c EmailChallenge) Available(now time.Time) bool {
	return c.ConsumedAt == nil && c.RevokedAt == nil && c.Attempts < 5 && !now.Before(c.CreatedAt) && now.Before(c.ExpiresAt)
}

// EmailBudget is shared by all operations for one address, source, or service.
type EmailBudget struct{ SentAt, FailedAt []time.Time }

// RetryAfter returns the longest remaining interval or rolling-limit wait.
func (b EmailBudget) RetryAfter(now time.Time, hourLimit, dayLimit int, interval time.Duration) time.Duration {
	var wait time.Duration
	hour, day, failures := 0, 0, 0
	for _, at := range b.SentAt {
		if at.After(now.Add(-24 * time.Hour)) {
			day++
		}
		if at.After(now.Add(-time.Hour)) {
			hour++
		}
		if remaining := at.Add(interval).Sub(now); remaining > wait {
			wait = remaining
		}
	}
	for _, at := range b.FailedAt {
		if at.After(now.Add(-10 * time.Minute)) {
			failures++
			if remaining := at.Add(10 * time.Minute).Sub(now); failures >= 5 && remaining > wait {
				wait = remaining
			}
		}
	}
	if hour >= hourLimit {
		for _, at := range b.SentAt {
			if at.After(now.Add(-time.Hour)) {
				if remaining := at.Add(time.Hour).Sub(now); remaining > wait {
					wait = remaining
				}
				break
			}
		}
	}
	if day >= dayLimit {
		for _, at := range b.SentAt {
			if at.After(now.Add(-24 * time.Hour)) {
				if remaining := at.Add(24 * time.Hour).Sub(now); remaining > wait {
					wait = remaining
				}
				break
			}
		}
	}
	return wait
}

// Prune retains only the bounded windows relevant to this budget.
func (b *EmailBudget) Prune(now time.Time) {
	sent := make([]time.Time, 0, len(b.SentAt))
	failed := make([]time.Time, 0, len(b.FailedAt))
	for _, at := range b.SentAt {
		if at.After(now.Add(-24 * time.Hour)) {
			sent = append(sent, at)
		}
	}
	for _, at := range b.FailedAt {
		if at.After(now.Add(-10 * time.Minute)) {
			failed = append(failed, at)
		}
	}
	b.SentAt, b.FailedAt = sent, failed
}
