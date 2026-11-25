package identity_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type deliveryStore struct {
	jobs     []identity.MailJob
	states   []identity.MailCompletion
	maxClaim int
}

func (s *deliveryStore) Claim(_ context.Context, limit int, _ time.Time) ([]identity.MailJob, error) {
	s.maxClaim = max(s.maxClaim, limit)
	n := min(limit, len(s.jobs))
	jobs := s.jobs[:n]
	s.jobs = s.jobs[n:]
	return jobs, nil
}
func (s *deliveryStore) Complete(_ context.Context, _ identity.MailJob, c identity.MailCompletion) error {
	s.states = append(s.states, c)
	return nil
}

type mailSender struct {
	calls int
	err   error
}

func (s *mailSender) Send(_ context.Context, _ uuid.UUID, _, _ string, _ []byte) (string, error) {
	s.calls++
	return "smtp-accepted", s.err
}

func TestMailDeliveryFailureStates(t *testing.T) {
	now := time.Now().UTC()
	protector, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	job := identity.MailJob{EventID: uuid.New(), Template: "registration_code", Recipient: "fixture@example.invalid", KeyID: "test-only", ExpiresAt: now.Add(time.Minute), AttemptCount: 1}
	job.EncryptedPayload, err = protector.Seal(job.EventID, []byte(`{"code":"123456"}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                     string
		err                      error
		expires                  time.Time
		attempt                  int
		wantStatus, wantCategory string
		wantCalls                int
	}{
		{"submitted is not delivered", nil, now.Add(time.Minute), 1, "submitted", "", 1},
		{"unknown outcome retries", identity.ErrMailOutcomeUnknown, now.Add(time.Minute), 1, "pending", "outcome_unknown", 1},
		{"bounded failures", errors.New("transport failure"), now.Add(time.Minute), 5, "failed", "transport_failed", 1},
		{"exact expiry", nil, now, 1, "expired", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := job
			current.ExpiresAt = tc.expires
			current.AttemptCount = tc.attempt
			store := &deliveryStore{jobs: []identity.MailJob{current}}
			sender := &mailSender{err: tc.err}
			delivery := identity.NewMailDelivery(store, protector, sender, func() time.Time { return now })
			if _, err := delivery.RunOnce(t.Context(), 10); err != nil {
				t.Fatal(err)
			}
			if sender.calls != tc.wantCalls || len(store.states) != 1 || store.states[0].Status != tc.wantStatus || store.states[0].ErrorCategory != tc.wantCategory {
				t.Fatal("incorrect delivery state")
			}
			if store.maxClaim != 1 {
				t.Fatal("sequential sender must not lease waiting jobs before it can send them")
			}
		})
	}
	if _, err := protector.Open(uuid.New(), job.KeyID, job.EncryptedPayload); err == nil {
		t.Fatal("ciphertext moved to another event")
	}
}
