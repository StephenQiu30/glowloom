package application

import (
	"context"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"

	"github.com/google/uuid"
)

// ErrRegistrationReceipt rejects an incomplete or unrelated owner acknowledgement.
var ErrRegistrationReceipt = errors.New("registration preparation receipt mismatch")

// RegistrationPreparation fixes the identity and both resource commands before delivery.
// Readiness is internal recovery state and is never a public account status.
type RegistrationPreparation struct {
	OperationID, AccountID, OrganizationID, ScopeID uuid.UUID
	InputSHA256                                     string
	CreationReady, WalletReady, Activated           bool
}

// RegistrationReady is an exact acknowledgement from one authenticated owner.
type RegistrationReady struct {
	OperationID, AccountID, OrganizationID, ScopeID uuid.UUID
	InputSHA256, Owner                              string
}

// ExpectedReady binds acknowledgements to the original command and resource identities.
func (p RegistrationPreparation) ExpectedReady(owner string) RegistrationReady {
	return RegistrationReady{OperationID: p.OperationID, AccountID: p.AccountID, OrganizationID: p.OrganizationID, ScopeID: p.ScopeID, InputSHA256: p.InputSHA256, Owner: owner}
}

// AccountControl is the original registration's only zero-epoch account opening.
// Its stable coordination identity can recover an ACK but cannot reopen a later head.
func (p RegistrationPreparation) AccountControl() writefence.Control {
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte("identity.registration.account/"+p.OperationID.String()+"/"+p.AccountID.String()))
	return writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: p.AccountID, Epoch: 1}, CoordinationID: id, Blocked: false}
}

// RegistrationPreparationStore owns accepted proof and activation transactions.
// Load must verify the immutable input proof; activation must recheck durable receipts
// under the original registration policy lock and commit account plus audit together.
type RegistrationPreparationStore interface {
	LoadRegistrationPreparation(context.Context, uuid.UUID, []byte) (RegistrationPreparation, error)
	RecordRegistrationReady(context.Context, RegistrationPreparation, RegistrationReady) error
	PrepareRegistrationAccount(context.Context, RegistrationPreparation) (writefence.Control, error)
	ActivateRegistration(context.Context, RegistrationPreparation) (uuid.UUID, error)
}

// RegistrationPreparationTx reserves a mailbox and consumes its verified grant in
// the same Identity transaction. Pending users are stored outside the account table.
type RegistrationPreparationTx interface {
	EmailTx
	FindRegistrationPreparation(uuid.UUID, []byte) (RegistrationPreparation, bool, error)
	InsertRegistrationPreparation(RegistrationPreparation, domain.User, []byte, time.Time) error
}

// WithRegistrationPreparation enables durable owner preparation on the existing
// complete-registration business entry. All dependencies are mandatory.
func (a *Auth) WithRegistrationPreparation(store RegistrationPreparationStore, creation, wallet RegistrationParticipant, writers SessionFenceCoordinator) error {
	r, err := NewRegistrationPreparation(store, creation, wallet, writers)
	if err != nil {
		return err
	}
	a.registration = r
	return nil
}

// RegistrationRecoveryTask carries only the original accepted operation UUID.
const RegistrationRecoveryTask = "identity.registration_prepare"

// RegistrationParticipant prepares only its own resource using the original command.
// A lost reply is recovered by repeating this same idempotent operation, never a new ID.
type RegistrationParticipant interface {
	PrepareRegistration(context.Context, RegistrationPreparation) (RegistrationReady, error)
}

// RegistrationPreparationCoordinator completes accepted registration responsibilities.
type RegistrationPreparationCoordinator struct {
	store            RegistrationPreparationStore
	creation, wallet RegistrationParticipant
	writers          SessionFenceCoordinator
}

// NewRegistrationPreparation refuses a partially configured owner transition.
func NewRegistrationPreparation(store RegistrationPreparationStore, creation, wallet RegistrationParticipant, writers SessionFenceCoordinator) (*RegistrationPreparationCoordinator, error) {
	if store == nil || creation == nil || wallet == nil || writers == nil {
		return nil, ErrRegistrationUnavailable
	}
	return &RegistrationPreparationCoordinator{store: store, creation: creation, wallet: wallet, writers: writers}, nil
}

// Resume recovers the same accepted input without opening a transaction across RPC.
func (r *RegistrationPreparationCoordinator) Resume(ctx context.Context, operation uuid.UUID, proof []byte) (uuid.UUID, error) {
	if r == nil || operation == uuid.Nil || len(proof) != 32 {
		return uuid.Nil, ErrProofInvalid
	}
	p, err := r.store.LoadRegistrationPreparation(ctx, operation, proof)
	if err != nil {
		return uuid.Nil, err
	}
	if p.OperationID != operation {
		return uuid.Nil, ErrRegistrationReceipt
	}
	return r.finish(ctx, p)
}

// AcceptedRegistrationReader is the trusted worker's original-responsibility
// capability. It never verifies a new user credential or accepts arbitrary users.
type AcceptedRegistrationReader interface {
	LoadAcceptedRegistration(context.Context, uuid.UUID) (RegistrationPreparation, error)
}

// ResumeAccepted is deliberately separate from the public original-proof lookup.
// Only the owner task worker receives this capability; the task supplies its fixed ID.
func (r *RegistrationPreparationCoordinator) ResumeAccepted(ctx context.Context, id uuid.UUID, reader AcceptedRegistrationReader) (uuid.UUID, error) {
	if r == nil || reader == nil || id == uuid.Nil {
		return uuid.Nil, ErrRegistrationUnavailable
	}
	p, err := reader.LoadAcceptedRegistration(ctx, id)
	if err != nil {
		return uuid.Nil, err
	}
	if p.OperationID != id {
		return uuid.Nil, ErrRegistrationReceipt
	}
	return r.finish(ctx, p)
}

func (r *RegistrationPreparationCoordinator) finish(ctx context.Context, p RegistrationPreparation) (uuid.UUID, error) {
	digest, err := hex.DecodeString(p.InputSHA256)
	if p.OperationID == uuid.Nil || p.AccountID == uuid.Nil || p.OrganizationID == uuid.Nil || p.ScopeID == uuid.Nil || err != nil || len(digest) != 32 || hex.EncodeToString(digest) != p.InputSHA256 {
		return uuid.Nil, ErrRegistrationReceipt
	}
	if p.Activated {
		return r.store.ActivateRegistration(ctx, p)
	}
	for _, target := range []struct {
		owner       string
		ready       bool
		participant RegistrationParticipant
	}{{"creation", p.CreationReady, r.creation}, {"execution", p.WalletReady, r.wallet}} {
		if target.ready {
			continue
		}
		receipt, err := target.participant.PrepareRegistration(ctx, p)
		if err != nil {
			return uuid.Nil, fmt.Errorf("prepare registration %s: %w", target.owner, err)
		}
		if receipt != p.ExpectedReady(target.owner) {
			return uuid.Nil, ErrRegistrationReceipt
		}
		if err := r.store.RecordRegistrationReady(ctx, p, receipt); err != nil {
			return uuid.Nil, err
		}
		if target.owner == "creation" {
			p.CreationReady = true
		} else {
			p.WalletReady = true
		}
	}
	control, err := r.store.PrepareRegistrationAccount(ctx, p)
	if err != nil {
		return uuid.Nil, err
	}
	if control != p.AccountControl() {
		return uuid.Nil, ErrRegistrationReceipt
	}
	expected := control.Authority
	expected.Epoch = 0
	state, err := r.writers.Set(ctx, expected, control.CoordinationID, false)
	if err != nil {
		return uuid.Nil, err
	}
	if !exactWriterCompletion(state, control) {
		return uuid.Nil, ErrRegistrationReceipt
	}
	return r.store.ActivateRegistration(ctx, p)
}

func exactWriterCompletion(state writefence.Coordination, control writefence.Control) bool {
	if !state.Complete || state.Control != control || len(state.Acknowledged) != len(writefence.Writers()) {
		return false
	}
	seen := map[writefence.Writer]bool{}
	for _, writer := range state.Acknowledged {
		if writer.Validate() != nil || seen[writer] {
			return false
		}
		seen[writer] = true
	}
	return true
}

func (a *Auth) completePreparedRegistration(ctx context.Context, input CompleteRegistrationInput) (uuid.UUID, error) {
	if input.OperationID == uuid.Nil {
		return uuid.Nil, ErrProofInvalid
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return uuid.Nil, err
	}
	proof := a.email.protector.Digest("registration_preparation", input.OperationID, string(encoded))
	clear(encoded)
	defer clear(proof)
	if len(proof) != 32 {
		return uuid.Nil, ErrProofInvalid
	}
	err = a.email.store.EmailTransaction(ctx, func(email EmailTx) error {
		tx, ok := email.(RegistrationPreparationTx)
		if !ok {
			return ErrRegistrationUnavailable
		}
		enabled, err := tx.RegistrationEnabled()
		if err != nil {
			return err
		}
		if _, found, err := tx.FindRegistrationPreparation(input.OperationID, proof); err != nil || found {
			return err
		}
		// Only a matching persisted input may recover an already accepted command.
		// New requests still satisfy the deployment's current terms and password policy.
		if !a.registrationReady() {
			return ErrRegistrationUnavailable
		}
		if !validDisplayName(input.DisplayName) || input.Password != input.ConfirmPassword || !input.TermsAccepted || input.TermsVersion != a.email.policy.TermsVersion {
			return domain.ErrInvalidProfile
		}
		if err := a.validateNewPassword(input.Password); err != nil {
			return err
		}
		c, err := tx.Challenge(input.OperationID, "registration_grant")
		if err != nil {
			return err
		}
		now := a.now().UTC().Truncate(time.Microsecond)
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
		id, org, scope := uuid.New(), uuid.New(), uuid.New()
		u := domain.User{ID: id, OrgID: org, LoginName: id.String(), Email: c.EmailKey, EmailVerifiedAt: now, EmailVersion: 1, DisplayName: input.DisplayName, Role: domain.RoleProducer, PlatformRole: "creator", PlatformEpoch: 1, Status: domain.StatusActive, PasswordHash: hash, PasswordChangedAt: now, SessionEpoch: 1, Revision: 1, TermsVersion: input.TermsVersion, TermsAcceptedAt: now, Theme: domain.DefaultTheme}
		p := RegistrationPreparation{OperationID: input.OperationID, AccountID: id, OrganizationID: org, ScopeID: scope, InputSHA256: hex.EncodeToString(proof)}
		if err := tx.InsertRegistrationPreparation(p, u, proof, now); err != nil {
			return err
		}
		c.ConsumedAt = &now
		return tx.SaveChallenge(c)
	})
	if err != nil {
		return uuid.Nil, err
	}
	return a.registration.Resume(ctx, input.OperationID, proof)
}
