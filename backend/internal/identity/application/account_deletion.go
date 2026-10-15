package application

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

var (
	// ErrReauthenticationRequired requires a fresh password proof on this session.
	ErrReauthenticationRequired = errors.New("recent password authentication required")
	// ErrDeletionDuties prevents leaving a platform or team without an administrator.
	ErrDeletionDuties = errors.New("administrator duties must be transferred")
)

// SoftDeletionPolicy identifies immediate revocation with retained records and resources.
const SoftDeletionPolicy = "soft-delete-v1"

// DeletionImpact lists the current account version, teams and blocking duties.
type DeletionImpact struct {
	Revision       int64
	TeamIDs        []uuid.UUID
	BlockingDuties []string
}

// AccountDeletionTx holds the identity locks, email proof and soft-deletion mutation.
type AccountDeletionTx interface {
	EmailTx
	Account() domain.User
	Session() domain.AuthSession
	Impact() (DeletionImpact, error)
	SoftDelete(time.Time) error
}

// AccountDeletionStore serializes identity revocation and retained-resource soft deletion.
type AccountDeletionStore interface {
	DeletionTransaction(context.Context, Principal, func() time.Time, func(AccountDeletionTx) error) error
}

// AccountDeletion retains records and resources; only identity and access change.
type AccountDeletion struct {
	auth  *Auth
	store AccountDeletionStore
	now   func() time.Time
}

// NewAccountDeletion constructs AccountDeletion with explicit caller-supplied dependencies.
func NewAccountDeletion(auth *Auth, store AccountDeletionStore, now func() time.Time) *AccountDeletion {
	return &AccountDeletion{auth: auth, store: store, now: now}
}

// Preview reports duties and the version required for confirmation.
func (s *AccountDeletion) Preview(ctx context.Context, actor Principal) (DeletionImpact, error) {
	var impact DeletionImpact
	err := s.store.DeletionTransaction(ctx, actor, s.now, func(tx AccountDeletionTx) error {
		var err error
		impact, err = tx.Impact()
		return err
	})
	return impact, err
}

func checkDeletion(tx AccountDeletionTx, now time.Time) error {
	if !tx.Session().RecentlyAuthenticated(tx.Account(), now) {
		return ErrReauthenticationRequired
	}
	impact, err := tx.Impact()
	if err != nil {
		return err
	}
	if len(impact.BlockingDuties) > 0 {
		return ErrDeletionDuties
	}
	return nil
}

// Start queues a session-bound email code after recent password verification.
func (s *AccountDeletion) Start(ctx context.Context, actor Principal, source string) (RegistrationStart, error) {
	if !s.auth.mailReady() {
		return RegistrationStart{}, ErrRegistrationUnavailable
	}
	op := uuid.New()
	err := s.store.DeletionTransaction(ctx, actor, s.now, func(tx AccountDeletionTx) error {
		now := s.now().UTC()
		if err := checkDeletion(tx, now); err != nil {
			return err
		}
		u := tx.Account()
		if err := s.auth.reserveEmail(tx, u.Email, source, now); err != nil {
			return err
		}
		n, err := rand.Int(rand.Reader, big.NewInt(1000000))
		if err != nil {
			return err
		}
		code := fmt.Sprintf("%06d", n.Int64())
		p := s.auth.email.protector
		c := domain.EmailChallenge{ID: uuid.New(), OperationID: op, Purpose: "deletion_confirm", EmailKey: u.Email, UserID: &u.ID, EmailVersion: &u.EmailVersion, CredentialEpoch: &u.SessionEpoch, ProofKeyID: p.KeyID(), CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
		c.ProofDigest = p.Digest(c.Purpose, c.ID, code)
		c.FlowSecretDigest = p.Digest("deletion_session", c.ID, actor.SessionID.String())
		if err := tx.InsertChallenge(c); err != nil {
			return err
		}
		return s.auth.queueEmail(tx, "deletion_confirm", u.Email, map[string]string{"code": code}, now, c.ExpiresAt, c.ID)
	})
	if err != nil {
		return RegistrationStart{}, err
	}
	return RegistrationStart{OperationID: op, RetryAfterSeconds: 60}, nil
}

// Confirm consumes the proof and soft-deletes the identity atomically.
func (s *AccountDeletion) Confirm(ctx context.Context, actor Principal, revision int64, op uuid.UUID, code string) error {
	if !s.auth.emailReady() {
		return ErrRegistrationUnavailable
	}
	var rejected error
	err := s.store.DeletionTransaction(ctx, actor, s.now, func(tx AccountDeletionTx) error {
		now := s.now().UTC()
		u := tx.Account()
		if revision != u.Revision {
			return domain.ErrRevisionConflict
		}
		if err := checkDeletion(tx, now); err != nil {
			return err
		}
		c, err := tx.Challenge(op, "deletion_confirm")
		if err != nil {
			return err
		}
		p := s.auth.email.protector
		if !c.Available(now) || c.UserID == nil || *c.UserID != u.ID || c.EmailVersion == nil || *c.EmailVersion != u.EmailVersion || c.CredentialEpoch == nil || *c.CredentialEpoch != u.SessionEpoch || c.EmailKey != u.Email || c.ProofKeyID != p.KeyID() || !hmac.Equal(c.FlowSecretDigest, p.Digest("deletion_session", c.ID, actor.SessionID.String())) {
			return ErrProofInvalid
		}
		bucket := s.auth.emailBucket(u.Email)
		budget, err := tx.Budget(bucket)
		if err != nil {
			return err
		}
		now = s.now().UTC()
		if !tx.Session().RecentlyAuthenticated(u, now) {
			return ErrReauthenticationRequired
		}
		if !c.Available(now) {
			return ErrProofInvalid
		}
		budget.Prune(now)
		if len(budget.FailedAt) >= 5 {
			return ErrProofInvalid
		}
		if !hmac.Equal(c.ProofDigest, p.Digest(c.Purpose, c.ID, code)) {
			c.Attempts++
			budget.FailedAt = append(budget.FailedAt, now)
			if err := tx.SaveChallenge(c); err != nil {
				return err
			}
			if err := tx.SaveBudget(bucket, budget); err != nil {
				return err
			}
			rejected = ErrProofInvalid
			return nil
		}
		c.ConsumedAt = &now
		if err := tx.SaveChallenge(c); err != nil {
			return err
		}
		return tx.SoftDelete(now)
	})
	if err != nil {
		return err
	}
	return rejected
}
