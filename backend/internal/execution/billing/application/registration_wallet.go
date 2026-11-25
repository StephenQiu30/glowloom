package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/google/uuid"
)

// ErrRegistrationWallet rejects a conflicting or incomplete original preparation.
var ErrRegistrationWallet = errors.New("registration wallet preparation conflict")

// RegistrationWallet fixes the wallet's source registration and resource identities.
// It carries no user credential, starting credit grant or permission to spend.
type RegistrationWallet struct {
	OperationID, AccountID, OrganizationID, ScopeID uuid.UUID
	InputSHA256                                     string
	CreationReady                                   bool
}

// Validate requires the complete original command and exact Creation ready proof.
func (p RegistrationWallet) Validate() error {
	b, err := hex.DecodeString(p.InputSHA256)
	if !p.CreationReady || p.OperationID == uuid.Nil || p.AccountID == uuid.Nil || p.OrganizationID == uuid.Nil || p.ScopeID == uuid.Nil || err != nil || len(b) != 32 || hex.EncodeToString(b) != p.InputSHA256 {
		return ErrRegistrationWallet
	}
	return nil
}

// CreationReceiptSHA256 records Identity's attestation of the exact Creation ready
// receipt. The transport rejects missing or retargeted receipt fields first.
func (p RegistrationWallet) CreationReceiptSHA256() string {
	canonical := "glowloom.creation.PersonalRegistration.ready\n" + p.OperationID.String() + "\n" + p.AccountID.String() + "\n" + p.OrganizationID.String() + "\n" + p.ScopeID.String() + "\n" + p.InputSHA256
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

// RegistrationWalletStore commits the wallet and original receipt in one local transaction.
type RegistrationWalletStore interface {
	PrepareRegistrationWallet(context.Context, RegistrationWallet) (RegistrationWallet, error)
	ResolveRegistrationWallet(context.Context, RegistrationWallet) (RegistrationWallet, error)
}

// RegistrationWallets owns the private registration participant, not a Billing service.
type RegistrationWallets struct{ store RegistrationWalletStore }

// NewRegistrationWallets injects Execution's local wallet participant.
func NewRegistrationWallets(store RegistrationWalletStore) (*RegistrationWallets, error) {
	if store == nil {
		return nil, ErrRegistrationWallet
	}
	return &RegistrationWallets{store: store}, nil
}

// Prepare confirms the exact accepted source command after local preparation.
func (s *RegistrationWallets) Prepare(ctx context.Context, p RegistrationWallet) (RegistrationWallet, error) {
	if err := p.Validate(); err != nil {
		return RegistrationWallet{}, err
	}
	r, err := s.store.PrepareRegistrationWallet(ctx, p)
	if err != nil {
		return RegistrationWallet{}, err
	}
	if r != p {
		return RegistrationWallet{}, ErrRegistrationWallet
	}
	return r, nil
}

// Resolve checks the exact durable receipt without creating or spending credits.
func (s *RegistrationWallets) Resolve(ctx context.Context, p RegistrationWallet) (RegistrationWallet, error) {
	if err := p.Validate(); err != nil {
		return RegistrationWallet{}, err
	}
	r, err := s.store.ResolveRegistrationWallet(ctx, p)
	if err != nil {
		return RegistrationWallet{}, err
	}
	if r != p {
		return RegistrationWallet{}, ErrRegistrationWallet
	}
	return r, nil
}
