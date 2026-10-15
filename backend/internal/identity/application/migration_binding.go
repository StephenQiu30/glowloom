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

// ErrAlreadyBootstrapped prevents granting a second bootstrap administrator.
var ErrAlreadyBootstrapped = errors.New("platform administrator already initialized")

// StartMigrationBinding is an operator-only command, never a public API. The
// explicitly selected legacy account remains unchanged until email proof succeeds.
func (a *Auth) StartMigrationBinding(ctx context.Context, login, email string, platformAdmin bool) (RegistrationStart, error) {
	if !a.mailReady() {
		return RegistrationStart{}, ErrRegistrationUnavailable
	}
	key, err := domain.NormalizeEmail(email)
	if err != nil {
		return RegistrationStart{}, err
	}
	legacy, err := domain.NormalizeLogin(login)
	if err != nil {
		return RegistrationStart{}, err
	}
	id := uuid.New()
	err = a.email.store.EmailTransaction(ctx, func(tx EmailTx) error {
		if _, err := tx.RegistrationEnabled(); err != nil {
			return err
		}
		u, err := tx.AccountByLogin(legacy)
		if err != nil {
			return err
		}
		if u.Email != "" || u.Status != domain.StatusActive {
			return ErrProofInvalid
		}
		if platformAdmin {
			exists, err := tx.HasPlatformAdministrator()
			if err != nil {
				return err
			}
			if exists {
				return ErrAlreadyBootstrapped
			}
		}
		if _, err := tx.AccountByEmail(key); err == nil {
			return ErrProofInvalid
		} else if !errors.Is(err, domain.ErrUserNotFound) {
			return err
		}
		now := a.now().UTC()
		if err := a.reserveEmail(tx, key, "operator_migration", now); err != nil {
			return err
		}
		n, err := rand.Int(rand.Reader, big.NewInt(1000000))
		if err != nil {
			return err
		}
		code := fmt.Sprintf("%06d", n.Int64())
		c := domain.EmailChallenge{ID: uuid.New(), OperationID: id, Purpose: "migration_bind", EmailKey: key, UserID: &u.ID, EmailVersion: &u.EmailVersion, CredentialEpoch: &u.SessionEpoch, ProofKeyID: a.email.protector.KeyID(), CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
		c.ProofDigest = a.email.protector.Digest(c.Purpose, c.ID, code)
		c.FlowSecretDigest = a.email.protector.Digest("migration_role", c.ID, migrationRole(platformAdmin))
		if err := tx.InsertChallenge(c); err != nil {
			return err
		}
		return a.queueEmail(tx, "migration_bind", key, map[string]string{"code": code}, now, c.ExpiresAt, c.ID)
	})
	if err != nil {
		return RegistrationStart{}, err
	}
	return RegistrationStart{OperationID: id, RetryAfterSeconds: 60}, nil
}

// CompleteMigrationBinding consumes a purpose/authority-bound proof and preserves
// the original account ID and authored-resource references.
func (a *Auth) CompleteMigrationBinding(ctx context.Context, id uuid.UUID, code string, platformAdmin bool) (uuid.UUID, error) {
	if !a.emailReady() {
		return uuid.Nil, ErrRegistrationUnavailable
	}
	var userID uuid.UUID
	var rejected error
	err := a.email.store.EmailTransaction(ctx, func(tx EmailTx) error {
		if _, err := tx.RegistrationEnabled(); err != nil {
			return err
		}
		c, err := tx.Challenge(id, "migration_bind")
		if err != nil {
			return err
		}
		now := a.now().UTC()
		if !c.Available(now) || c.UserID == nil || c.CredentialEpoch == nil || c.EmailVersion == nil || c.ProofKeyID != a.email.protector.KeyID() || !hmac.Equal(c.FlowSecretDigest, a.email.protector.Digest("migration_role", c.ID, migrationRole(platformAdmin))) {
			return ErrProofInvalid
		}
		budget, err := tx.Budget(a.emailBucket(c.EmailKey))
		if err != nil {
			return err
		}
		budget.Prune(now)
		if len(budget.FailedAt) >= 5 {
			return ErrProofInvalid
		}
		if !hmac.Equal(c.ProofDigest, a.email.protector.Digest(c.Purpose, c.ID, code)) {
			c.Attempts++
			budget.FailedAt = append(budget.FailedAt, now)
			if err := tx.SaveChallenge(c); err != nil {
				return err
			}
			if err := tx.SaveBudget(a.emailBucket(c.EmailKey), budget); err != nil {
				return err
			}
			rejected = ErrProofInvalid
			return nil
		}
		u, err := tx.AccountByID(*c.UserID)
		if err != nil {
			return err
		}
		if u.Status != domain.StatusActive || u.Email != "" || u.SessionEpoch != *c.CredentialEpoch || u.EmailVersion != *c.EmailVersion {
			return ErrProofInvalid
		}
		if u.SessionEpoch >= math.MaxInt32 || u.Revision >= math.MaxInt32 {
			return domain.ErrInvalidAccountState
		}
		if platformAdmin {
			exists, err := tx.HasPlatformAdministrator()
			if err != nil {
				return err
			}
			if exists {
				return ErrAlreadyBootstrapped
			}
		}
		u.Email = c.EmailKey
		u.EmailVerifiedAt = now
		u.PlatformRole = migrationRole(platformAdmin)
		u.SessionEpoch++
		u.Revision++
		if err := tx.BindVerifiedAccount(u); err != nil {
			return err
		}
		if err := tx.RevokeAccountSessions(u.ID, now); err != nil {
			return err
		}
		c.ConsumedAt = &now
		if err := tx.SaveChallenge(c); err != nil {
			return err
		}
		userID = u.ID
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	return userID, rejected
}

func migrationRole(admin bool) string {
	if admin {
		return "platform_admin"
	}
	return "creator"
}
