package source

import (
	"context"
	"encoding/hex"
	"errors"

	"github.com/google/uuid"
)

// ErrPersonalRegistration rejects an incomplete or conflicting accepted registration.
var ErrPersonalRegistration = errors.New("personal registration preparation conflict")

// PersonalRegistration contains only the resource identities accepted by Identity.
// The private transport authenticates Identity; these IDs are not user credentials.
type PersonalRegistration struct {
	OperationID, AccountID, OrganizationID, ScopeID uuid.UUID
	InputSHA256                                     string
}

// Validate requires a complete original command with a canonical digest.
func (p PersonalRegistration) Validate() error {
	if p.OperationID == uuid.Nil || p.AccountID == uuid.Nil || p.OrganizationID == uuid.Nil || p.ScopeID == uuid.Nil {
		return ErrPersonalRegistration
	}
	b, err := hex.DecodeString(p.InputSHA256)
	if err != nil || len(b) != 32 || hex.EncodeToString(b) != p.InputSHA256 {
		return ErrPersonalRegistration
	}
	return nil
}

// PersonalRegistrationStore prepares and resolves only Creation-owned resources.
// Implementations commit original command, organization and personal scope together.
type PersonalRegistrationStore interface {
	PreparePersonalRegistration(context.Context, PersonalRegistration) (PersonalRegistration, error)
	ResolvePersonalRegistration(context.Context, PersonalRegistration) (PersonalRegistration, error)
}

// PersonalRegistrations owns the two private accepted-registration operations.
type PersonalRegistrations struct{ store PersonalRegistrationStore }

// NewPersonalRegistrations injects Creation's local preparation persistence.
func NewPersonalRegistrations(store PersonalRegistrationStore) (*PersonalRegistrations, error) {
	if store == nil {
		return nil, ErrPersonalRegistration
	}
	return &PersonalRegistrations{store: store}, nil
}

// Prepare commits or recovers the original Creation resource preparation.
func (s *PersonalRegistrations) Prepare(ctx context.Context, p PersonalRegistration) (PersonalRegistration, error) {
	if err := p.Validate(); err != nil {
		return PersonalRegistration{}, err
	}
	r, err := s.store.PreparePersonalRegistration(ctx, p)
	if err != nil {
		return PersonalRegistration{}, err
	}
	if r != p {
		return PersonalRegistration{}, ErrPersonalRegistration
	}
	return r, nil
}

// Resolve returns only the exact already prepared Creation receipt.
func (s *PersonalRegistrations) Resolve(ctx context.Context, p PersonalRegistration) (PersonalRegistration, error) {
	if err := p.Validate(); err != nil {
		return PersonalRegistration{}, err
	}
	r, err := s.store.ResolvePersonalRegistration(ctx, p)
	if err != nil {
		return PersonalRegistration{}, err
	}
	if r != p {
		return PersonalRegistration{}, ErrPersonalRegistration
	}
	return r, nil
}
