package application

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// ErrEmailUnavailable reports an address already reserved by an identity.
var ErrEmailUnavailable = errors.New("email unavailable")

// EmailChangeTx preserves identity, session, proofs and notifications atomically.
type EmailChangeTx interface {
	EmailTx
	Account() domain.User
	Session() domain.AuthSession
	ProofSubmitted(uuid.UUID) (bool, error)
	RevokeEmailChanges(time.Time) error
	ChangeEmail(string, int64, time.Time) error
}

// EmailChangeStore locks the current account before the proof operation.
type EmailChangeStore interface {
	EmailChangeTransaction(context.Context, Principal, time.Time, func(EmailChangeTx) error) error
}

// EmailChange owns one session-bound pair of mailbox proofs.
type EmailChange struct {
	auth  *Auth
	store EmailChangeStore
	now   func() time.Time
}

// NewEmailChange injects existing identity proof and encrypted mail dependencies.
func NewEmailChange(auth *Auth, store EmailChangeStore, now func() time.Time) *EmailChange {
	return &EmailChange{auth: auth, store: store, now: now}
}
func (s *EmailChange) recent(tx EmailChangeTx, now time.Time) error {
	if !tx.Session().RecentlyAuthenticated(tx.Account(), now) {
		return ErrReauthenticationRequired
	}
	return nil
}
func (s *EmailChange) binding(c domain.EmailChallenge, tx EmailChangeTx) []byte {
	return s.auth.email.protector.Digest("email_change_session_revision", c.OperationID, fmt.Sprintf("%s/%d", tx.Session().ID, tx.Account().Revision))
}
func (s *EmailChange) bound(c domain.EmailChallenge, tx EmailChangeTx, now time.Time) bool {
	u := tx.Account()
	return c.UserID != nil && *c.UserID == u.ID && c.EmailVersion != nil && *c.EmailVersion == u.EmailVersion && c.CredentialEpoch != nil && *c.CredentialEpoch == u.SessionEpoch && c.ProofKeyID == s.auth.email.protector.KeyID() && c.RevokedAt == nil && c.Attempts < 5 && !now.Before(c.CreatedAt) && now.Before(c.ExpiresAt) && hmac.Equal(c.FlowSecretDigest, s.binding(c, tx))
}

// Start replaces prior pending operations and sends two distinct bounded proofs.
func (s *EmailChange) Start(ctx context.Context, a Principal, email string, revision int64) (uuid.UUID, error) {
	return s.StartFrom(ctx, a, email, revision, "local")
}

// StartFrom charges the trusted ingress source and both outgoing messages.
func (s *EmailChange) StartFrom(ctx context.Context, a Principal, email string, revision int64, source string) (uuid.UUID, error) {
	if !s.auth.mailReady() {
		return uuid.Nil, ErrRegistrationUnavailable
	}
	key, err := domain.NormalizeEmail(email)
	if err != nil {
		return uuid.Nil, err
	}
	op := uuid.New()
	now := s.now().UTC()
	err = s.store.EmailChangeTransaction(ctx, a, now, func(tx EmailChangeTx) error {
		u := tx.Account()
		if u.Revision != revision {
			return domain.ErrRevisionConflict
		}
		if key == u.Email {
			return domain.ErrInvalidEmail
		}
		if err := s.recent(tx, now); err != nil {
			return err
		}
		if used, err := tx.EmailReserved(key); err != nil {
			return err
		} else if used {
			return ErrEmailUnavailable
		}
		for _, address := range []string{u.Email, key} {
			if err := s.auth.reserveEmail(tx, address, source, now); err != nil {
				return err
			}
		}
		if err := tx.RevokeEmailChanges(now); err != nil {
			return err
		}
		codes := map[string]string{}
		for _, side := range []string{"old", "new"} {
			n, err := rand.Int(rand.Reader, big.NewInt(1000000))
			if err != nil {
				return err
			}
			code := fmt.Sprintf("%06d", n.Int64())
			if side == "new" && code == codes["old"] {
				code = fmt.Sprintf("%06d", (n.Int64()+1)%1000000)
			}
			codes[side] = code
			address := u.Email
			if side == "new" {
				address = key
			}
			c := domain.EmailChallenge{ID: uuid.New(), OperationID: op, Purpose: "email_" + side, EmailKey: address, UserID: &u.ID, EmailVersion: &u.EmailVersion, CredentialEpoch: &u.SessionEpoch, ProofKeyID: s.auth.email.protector.KeyID(), CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
			c.ProofDigest = s.auth.email.protector.Digest(c.Purpose, c.ID, code)
			c.FlowSecretDigest = s.binding(c, tx)
			if err := tx.InsertChallenge(c); err != nil {
				return err
			}
			if err := s.auth.queueEmail(tx, c.Purpose, address, map[string]string{"code": code}, now, c.ExpiresAt, c.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	return op, nil
}

// Verify consumes only the requested side after a successful delivery submission.
func (s *EmailChange) Verify(ctx context.Context, a Principal, op uuid.UUID, side, code string) error {
	if !s.auth.emailReady() {
		return ErrRegistrationUnavailable
	}
	if (side != "old" && side != "new") || op == uuid.Nil {
		return ErrProofInvalid
	}
	now := s.now().UTC()
	var rejected error
	err := s.store.EmailChangeTransaction(ctx, a, now, func(tx EmailChangeTx) error {
		if err := s.recent(tx, now); err != nil {
			return err
		}
		c, err := tx.Challenge(op, "email_"+side)
		if err != nil {
			return err
		}
		if !s.bound(c, tx, now) || c.ConsumedAt != nil || (side == "old" && c.EmailKey != tx.Account().Email) {
			return ErrProofInvalid
		}
		submitted, err := tx.ProofSubmitted(c.ID)
		if err != nil {
			return err
		}
		if !submitted {
			return ErrProofInvalid
		}
		bucket := s.auth.emailBucket(c.EmailKey)
		b, err := tx.Budget(bucket)
		if err != nil {
			return err
		}
		b.Prune(now)
		if len(b.FailedAt) >= 5 {
			return ErrProofInvalid
		}
		if !hmac.Equal(c.ProofDigest, s.auth.email.protector.Digest(c.Purpose, c.ID, code)) {
			c.Attempts++
			b.FailedAt = append(b.FailedAt, now)
			if err := tx.SaveChallenge(c); err != nil {
				return err
			}
			if err := tx.SaveBudget(bucket, b); err != nil {
				return err
			}
			rejected = ErrProofInvalid
			return nil
		}
		c.ConsumedAt = &now
		return tx.SaveChallenge(c)
	})
	if err != nil {
		return err
	}
	return rejected
}

// Complete changes only credentials, preserving all resource ownership and memberships.
func (s *EmailChange) Complete(ctx context.Context, a Principal, op uuid.UUID, revision int64) error {
	if !s.auth.emailReady() {
		return ErrRegistrationUnavailable
	}
	now := s.now().UTC()
	return s.store.EmailChangeTransaction(ctx, a, now, func(tx EmailChangeTx) error {
		u := tx.Account()
		if u.Revision != revision {
			return domain.ErrRevisionConflict
		}
		if err := s.recent(tx, now); err != nil {
			return err
		}
		old, err := tx.Challenge(op, "email_old")
		if err != nil {
			return err
		}
		next, err := tx.Challenge(op, "email_new")
		if err != nil {
			return err
		}
		if !s.bound(old, tx, now) || !s.bound(next, tx, now) || old.ConsumedAt == nil || next.ConsumedAt == nil || !old.CreatedAt.Equal(next.CreatedAt) || !old.ExpiresAt.Equal(next.ExpiresAt) || old.EmailKey != u.Email || next.EmailKey == u.Email {
			return ErrProofInvalid
		}
		for _, c := range []domain.EmailChallenge{old, next} {
			ok, err := tx.ProofSubmitted(c.ID)
			if err != nil {
				return err
			}
			if !ok {
				return ErrProofInvalid
			}
		}
		if used, err := tx.EmailReserved(next.EmailKey); err != nil {
			return err
		} else if used {
			return ErrEmailUnavailable
		}
		if u.EmailVersion >= math.MaxInt32 || u.SessionEpoch >= math.MaxInt32 || u.Revision >= math.MaxInt32 {
			return domain.ErrInvalidAccountState
		}
		if err := tx.ChangeEmail(next.EmailKey, revision, now); err != nil {
			return err
		}
		for _, address := range []string{old.EmailKey, next.EmailKey} {
			if err := s.auth.queueEmail(tx, "email_changed", address, map[string]string{}, now, now.Add(24*time.Hour)); err != nil {
				return err
			}
		}
		return nil
	})
}

// Cancel revokes the matching operation without changing the account.
func (s *EmailChange) Cancel(ctx context.Context, a Principal, op uuid.UUID) error {
	if !s.auth.emailReady() {
		return ErrRegistrationUnavailable
	}
	now := s.now().UTC()
	return s.store.EmailChangeTransaction(ctx, a, now, func(tx EmailChangeTx) error {
		for _, side := range []string{"old", "new"} {
			c, err := tx.Challenge(op, "email_"+side)
			if err != nil {
				return err
			}
			if c.UserID == nil || *c.UserID != a.ID || !hmac.Equal(c.FlowSecretDigest, s.binding(c, tx)) {
				return ErrProofInvalid
			}
			c.RevokedAt = &now
			if err := tx.SaveChallenge(c); err != nil {
				return err
			}
		}
		return nil
	})
}
