package application

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

var (
	// ErrProofInvalid rejects invalid, expired, revoked or consumed proofs.
	ErrProofInvalid = errors.New("invalid or expired proof")
	// ErrRateLimited reports exhausted rolling or resend budgets.
	ErrRateLimited = errors.New("identity request rate limited")
	// ErrRegistrationUnavailable keeps registration closed without required policy or keys.
	ErrRegistrationUnavailable = errors.New("registration unavailable")
)

// RateLimit carries the actual wait without revealing whether an account exists.
type RateLimit struct{ RetryAfter time.Duration }

func (e *RateLimit) Error() string { return ErrRateLimited.Error() }
func (e *RateLimit) Unwrap() error { return ErrRateLimited }

// EmailPolicy supplies accepted terms and local password and delivery limits.
type EmailPolicy struct {
	TermsVersion                         string
	SourceHourlyLimit, GlobalHourlyLimit int
	BlockedPasswords                     []string
}

// ProofProtector is owned by identity; keys are supplied outside persistence.
type ProofProtector interface {
	KeyID() string
	Digest(string, uuid.UUID, string) []byte
	Seal(uuid.UUID, []byte) ([]byte, error)
}

// QueuedMail keeps message content encrypted until delivery.
type QueuedMail struct {
	ChallengeID                *uuid.UUID
	InvitationID               *uuid.UUID
	InvitationRevision         *int64
	EventID                    uuid.UUID
	Template, Recipient, KeyID string
	EncryptedPayload           []byte
	CreatedAt, ExpiresAt       time.Time
}

// EmailStore provides atomic email identity transactions.
type EmailStore interface {
	EmailTransaction(context.Context, func(EmailTx) error) error
}

// EmailTx combines account, proof, budget and outbox writes in one transaction.
type EmailTx interface {
	AuthTx
	RegistrationEnabled() (bool, error)
	RegistrationInvitation(uuid.UUID, []byte, string, time.Time) (Invitation, error)
	AccountByEmail(string) (domain.User, error)
	EmailReserved(string) (bool, error)
	Challenge(uuid.UUID, string) (domain.EmailChallenge, error)
	InsertChallenge(domain.EmailChallenge) error
	SaveChallenge(domain.EmailChallenge) error
	Budget(string) (domain.EmailBudget, error)
	SaveBudget(string, domain.EmailBudget) error
	CreatePersonalAccount(domain.User) error
	BindVerifiedAccount(domain.User) error
	HasPlatformAdministrator() (bool, error)
	EnqueueMail(QueuedMail) error
}

type emailRuntime struct {
	store     EmailStore
	protector ProofProtector
	policy    EmailPolicy
}

// NewEmailAuth enables the new flow only with explicit policy and key dependencies.
func NewEmailAuth(store EmailStore, protector ProofProtector, policy EmailPolicy, now func() time.Time) *Auth {
	policy.BlockedPasswords = append([]string(nil), policy.BlockedPasswords...)
	return &Auth{now: now, email: &emailRuntime{store: store, protector: protector, policy: policy}}
}

// RegistrationStart acknowledges an operation and its resend delay.
type RegistrationStart struct {
	OperationID       uuid.UUID
	FlowSecret        string
	RetryAfterSeconds int
}

// RegistrationProof advances email verification without creating a login session.
type RegistrationProof struct{ Outcome, Grant string }

// CompleteRegistrationInput binds account creation to email proof and terms consent.
type CompleteRegistrationInput struct {
	OperationID                                                             uuid.UUID
	FlowSecret, Grant, DisplayName, Password, ConfirmPassword, TermsVersion string
	TermsAccepted                                                           bool
}

func (a *Auth) emailReady() bool {
	return a.email != nil && a.email.protector != nil
}

func (a *Auth) mailReady() bool {
	return a.emailReady() && a.email.policy.SourceHourlyLimit > 0 && a.email.policy.SourceHourlyLimit <= 1000000 && a.email.policy.GlobalHourlyLimit > 0 && a.email.policy.GlobalHourlyLimit <= 1000000
}

func (a *Auth) registrationReady() bool {
	if !a.mailReady() {
		return false
	}
	version := a.email.policy.TermsVersion
	// Drafts can be reviewed without accidentally becoming a registration policy.
	// Publishing still requires the deployment's immutable document manifest.
	return version != "" && version == strings.TrimSpace(version) && !strings.Contains(strings.ToLower(version), "draft")
}

// StartRegistration starts email verification using the default source budget.
func (a *Auth) StartRegistration(ctx context.Context, email, invitation string) (RegistrationStart, error) {
	return a.StartRegistrationFrom(ctx, email, invitation, "local")
}

// StartRegistrationFrom receives the trusted ingress source, never a client header.
func (a *Auth) StartRegistrationFrom(ctx context.Context, email, invitation, source string) (RegistrationStart, error) {
	if !a.registrationReady() {
		return RegistrationStart{}, ErrRegistrationUnavailable
	}
	key, err := domain.NormalizeEmail(email)
	if err != nil {
		return RegistrationStart{}, err
	}
	if invitation != "" && len(invitation) != 43 {
		return RegistrationStart{}, ErrProofInvalid
	}
	flow, err := newProofToken()
	if err != nil {
		return RegistrationStart{}, err
	}
	start := RegistrationStart{OperationID: uuid.New(), FlowSecret: flow, RetryAfterSeconds: 60}
	err = a.email.store.EmailTransaction(ctx, func(tx EmailTx) error {
		enabled, err := tx.RegistrationEnabled()
		if err != nil {
			return err
		}
		now := a.now().UTC()
		var invited *Invitation
		if invitation != "" {
			sum := sha256.Sum256([]byte(invitation))
			i, err := tx.RegistrationInvitation(uuid.Nil, sum[:], key, now)
			if err != nil {
				return err
			}
			invited = &i
		} else if !enabled {
			return ErrRegistrationUnavailable
		}
		if err := a.reserveEmail(tx, key, source, now); err != nil {
			return err
		}
		return a.issueRegistrationCode(tx, start.OperationID, flow, key, now, invited)
	})
	if err != nil {
		return RegistrationStart{}, err
	}
	return start, nil
}

// ResendRegistration replaces a verification code without resetting the failure budget.
func (a *Auth) ResendRegistration(ctx context.Context, id uuid.UUID, flow string) (RegistrationStart, error) {
	return a.ResendRegistrationFrom(ctx, id, flow, "local")
}

// ResendRegistrationFrom also applies the current request-source delivery budget.
func (a *Auth) ResendRegistrationFrom(ctx context.Context, id uuid.UUID, flow, source string) (RegistrationStart, error) {
	if !a.registrationReady() {
		return RegistrationStart{}, ErrRegistrationUnavailable
	}
	err := a.email.store.EmailTransaction(ctx, func(tx EmailTx) error {
		enabled, err := tx.RegistrationEnabled()
		if err != nil {
			return err
		}
		c, err := tx.Challenge(id, "registration_code")
		if err != nil {
			return err
		}
		now := a.now().UTC()
		invited, err := registrationAdmission(tx, enabled, c, now)
		if err != nil {
			return err
		}
		if !a.matchesFlow(c, flow) || c.ConsumedAt != nil || c.RevokedAt != nil {
			return ErrProofInvalid
		}
		if err := a.reserveEmail(tx, c.EmailKey, source, now); err != nil {
			return err
		}
		c.RevokedAt = &now
		if err := tx.SaveChallenge(c); err != nil {
			return err
		}
		return a.issueRegistrationCode(tx, id, flow, c.EmailKey, now, invited)
	})
	return RegistrationStart{OperationID: id, RetryAfterSeconds: 60}, err
}

func (a *Auth) issueRegistrationCode(tx EmailTx, id uuid.UUID, flow, email string, now time.Time, invited *Invitation) error {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	c := domain.EmailChallenge{ID: uuid.New(), OperationID: id, Purpose: "registration_code", EmailKey: email, ProofKeyID: a.email.protector.KeyID(), CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
	if invited != nil {
		c.InvitationID = &invited.ID
		c.InvitationRevision = &invited.Revision
	}
	c.ProofDigest = a.email.protector.Digest(c.Purpose, c.ID, code)
	c.FlowSecretDigest = a.email.protector.Digest("registration_flow", id, flow)
	if err := tx.InsertChallenge(c); err != nil {
		return err
	}
	return a.queueEmail(tx, "registration_code", email, map[string]string{"code": code}, now, c.ExpiresAt, c.ID)
}

// VerifyRegistration consumes the code and returns a single-use account-creation grant.
func (a *Auth) VerifyRegistration(ctx context.Context, id uuid.UUID, flow, code string) (RegistrationProof, error) {
	if !a.registrationReady() {
		return RegistrationProof{}, ErrRegistrationUnavailable
	}
	var result RegistrationProof
	var rejected error
	err := a.email.store.EmailTransaction(ctx, func(tx EmailTx) error {
		enabled, err := tx.RegistrationEnabled()
		if err != nil {
			return err
		}
		c, err := tx.Challenge(id, "registration_code")
		if err != nil {
			return err
		}
		now := a.now().UTC()
		if _, err := registrationAdmission(tx, enabled, c, now); err != nil {
			return err
		}
		if !c.Available(now) || !a.matchesFlow(c, flow) {
			return ErrProofInvalid
		}
		key := a.emailBucket(c.EmailKey)
		budget, err := tx.Budget(key)
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
			if err := tx.SaveBudget(key, budget); err != nil {
				return err
			}
			rejected = ErrProofInvalid
			return nil // Wrong guesses must commit the budget.
		}
		c.ConsumedAt = &now
		if err := tx.SaveChallenge(c); err != nil {
			return err
		}
		reserved, err := tx.EmailReserved(c.EmailKey)
		if err != nil {
			return err
		}
		if reserved {
			result.Outcome = "login_required"
			return nil
		}
		grant, err := newProofToken()
		if err != nil {
			return err
		}
		proof := domain.EmailChallenge{ID: uuid.New(), OperationID: id, Purpose: "registration_grant", EmailKey: c.EmailKey, ProofKeyID: c.ProofKeyID, FlowSecretDigest: c.FlowSecretDigest, CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
		proof.InvitationID, proof.InvitationRevision = c.InvitationID, c.InvitationRevision
		proof.ProofDigest = a.email.protector.Digest(proof.Purpose, proof.ID, grant)
		if err := tx.InsertChallenge(proof); err != nil {
			return err
		}
		result = RegistrationProof{Outcome: "set_password", Grant: grant}
		return nil
	})
	if err != nil {
		return RegistrationProof{}, err
	}
	return result, rejected
}

// CompleteRegistration creates an account after proof and exact terms-version checks.
func (a *Auth) CompleteRegistration(ctx context.Context, input CompleteRegistrationInput) (uuid.UUID, error) {
	if !a.registrationReady() {
		return uuid.Nil, ErrRegistrationUnavailable
	}
	name := strings.TrimSpace(input.DisplayName)
	if !validDisplayName(name) || input.Password != input.ConfirmPassword || !input.TermsAccepted || input.TermsVersion != a.email.policy.TermsVersion {
		return uuid.Nil, domain.ErrInvalidProfile
	}
	if err := a.validateNewPassword(input.Password); err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err := a.email.store.EmailTransaction(ctx, func(tx EmailTx) error {
		enabled, err := tx.RegistrationEnabled()
		if err != nil {
			return err
		}
		c, err := tx.Challenge(input.OperationID, "registration_grant")
		if err != nil {
			return err
		}
		now := a.now().UTC()
		if _, err := registrationAdmission(tx, enabled, c, now); err != nil {
			return err
		}
		if !c.Available(now) || !a.matchesFlow(c, input.FlowSecret) || !hmac.Equal(c.ProofDigest, a.email.protector.Digest(c.Purpose, c.ID, input.Grant)) {
			return ErrProofInvalid
		}
		if reserved, err := tx.EmailReserved(c.EmailKey); err != nil {
			return err
		} else if reserved {
			return ErrProofInvalid
		}
		hash, err := domain.HashPassword(input.Password, "")
		if err != nil {
			return err
		}
		id = uuid.New()
		u := domain.User{ID: id, OrgID: uuid.New(), LoginName: id.String(), Email: c.EmailKey, EmailVerifiedAt: now, EmailVersion: 1, DisplayName: name, Role: domain.RoleProducer, PlatformRole: "creator", PlatformEpoch: 1, Status: domain.StatusActive, PasswordHash: hash, PasswordChangedAt: now, SessionEpoch: 1, Revision: 1, TermsVersion: input.TermsVersion, TermsAcceptedAt: now, Theme: "system"}
		if err := tx.CreatePersonalAccount(u); err != nil {
			return err
		}
		c.ConsumedAt = &now
		if err := tx.SaveChallenge(c); err != nil {
			return err
		}
		return tx.AppendAuthEvent(id, "registered", now)
	})
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (a *Auth) validateNewPassword(password string) error {
	if err := domain.ValidateNewPassword(password); err != nil {
		return err
	}
	for _, blocked := range a.email.policy.BlockedPasswords {
		if strings.EqualFold(password, blocked) {
			return domain.ErrWeakPassword
		}
	}
	return nil
}

func (a *Auth) matchesFlow(c domain.EmailChallenge, flow string) bool {
	return flow != "" && c.ProofKeyID == a.email.protector.KeyID() && hmac.Equal(c.FlowSecretDigest, a.email.protector.Digest("registration_flow", c.OperationID, flow))
}

func (a *Auth) emailBucket(email string) string {
	return hex.EncodeToString(a.email.protector.Digest("email_budget", uuid.Nil, email))
}

func (a *Auth) reserveEmail(tx EmailTx, email, source string, now time.Time) error {
	for _, rule := range []struct {
		key       string
		hour, day int
		interval  time.Duration
	}{
		{"global", a.email.policy.GlobalHourlyLimit, a.email.policy.GlobalHourlyLimit * 24, 0},
		{"source/" + hex.EncodeToString(a.email.protector.Digest("email_source", uuid.Nil, source)), a.email.policy.SourceHourlyLimit, a.email.policy.SourceHourlyLimit * 24, 0},
		{a.emailBucket(email), 5, 10, time.Minute},
	} {
		b, err := tx.Budget(rule.key)
		if err != nil {
			return err
		}
		b.Prune(now)
		if wait := b.RetryAfter(now, rule.hour, rule.day, rule.interval); wait > 0 {
			return &RateLimit{RetryAfter: wait}
		}
		b.SentAt = append(b.SentAt, now)
		if err := tx.SaveBudget(rule.key, b); err != nil {
			return err
		}
	}
	return nil
}

func (a *Auth) queueEmail(tx EmailTx, template, email string, payload any, now, expires time.Time, challengeID ...uuid.UUID) error {
	id := uuid.New()
	plain, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	defer clear(plain)
	var challenge *uuid.UUID
	if len(challengeID) > 0 {
		challenge = &challengeID[0]
	}
	encrypted, err := a.email.protector.Seal(id, plain)
	if err != nil {
		return err
	}
	return tx.EnqueueMail(QueuedMail{EventID: id, ChallengeID: challenge, Template: template, Recipient: email, KeyID: a.email.protector.KeyID(), EncryptedPayload: encrypted, CreatedAt: now, ExpiresAt: expires})
}

func newProofToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func validDisplayName(name string) bool {
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 32 {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// registrationAdmission rechecks the immutable invitation revision at every proof transition.
func registrationAdmission(tx EmailTx, enabled bool, c domain.EmailChallenge, now time.Time) (*Invitation, error) {
	if c.InvitationID == nil {
		if !enabled {
			return nil, ErrRegistrationUnavailable
		}
		return nil, nil
	}
	if c.InvitationRevision == nil {
		return nil, ErrProofInvalid
	}
	i, err := tx.RegistrationInvitation(*c.InvitationID, nil, c.EmailKey, now)
	if err != nil {
		return nil, err
	}
	if i.Revision != *c.InvitationRevision {
		return nil, ErrProofInvalid
	}
	return &i, nil
}
