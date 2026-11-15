package source

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// InitialScopeControl binds the only zero-epoch opening to an accepted registration.
// It cannot reopen a changed, blocked or previously used resource generation.
func InitialScopeControl(registration PersonalRegistration) writefence.Control {
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte("creation.registration.scope/"+registration.OperationID.String()+"/"+registration.ScopeID.String()))
	return writefence.Control{Authority: writefence.Authority{Issuer: writefence.Creation, Kind: writefence.Scope, ID: registration.ScopeID, Epoch: 1}, CoordinationID: id, Blocked: false}
}

// PersonalScopeAuthorityStore verifies the immutable local preparation and scope
// policy before committing the initial control, then checks current exact readiness.
type PersonalScopeAuthorityStore interface {
	PreparePersonalScopeAuthority(context.Context, PersonalRegistration) (writefence.Control, error)
	CheckPersonalScopeAuthority(context.Context, PersonalRegistration, writefence.Control) error
}

// ScopeAuthorityInstaller is the existing fixed four-writer coordinator. Network
// propagation begins only after the source's policy transaction has committed.
type ScopeAuthorityInstaller interface {
	Set(context.Context, writefence.Authority, uuid.UUID, bool) (writefence.Coordination, error)
}

// PreparedPersonalRegistrations is the actual accepted-resource preparation use case.
// Its small port also lets the authenticated protocol depend on complete readiness.
type PreparedPersonalRegistrations interface {
	Prepare(context.Context, PersonalRegistration) (PersonalRegistration, error)
	Resolve(context.Context, PersonalRegistration) (PersonalRegistration, error)
}

// ReadyPersonalRegistrations reports a personal scope ready only after its original
// source control is durably installed at every writer. It adds no public state.
type ReadyPersonalRegistrations struct {
	resources PreparedPersonalRegistrations
	authority PersonalScopeAuthorityStore
	writers   ScopeAuthorityInstaller
}

// NewReadyPersonalRegistrations requires complete preparation and source propagation.
func NewReadyPersonalRegistrations(resources PreparedPersonalRegistrations, authority PersonalScopeAuthorityStore, writers ScopeAuthorityInstaller) (*ReadyPersonalRegistrations, error) {
	if resources == nil || authority == nil || writers == nil {
		return nil, ErrPersonalRegistration
	}
	return &ReadyPersonalRegistrations{resources: resources, authority: authority, writers: writers}, nil
}

// Prepare finishes the exact accepted resource and its original scope authority.
func (s *ReadyPersonalRegistrations) Prepare(ctx context.Context, registration PersonalRegistration) (PersonalRegistration, error) {
	if s == nil {
		return PersonalRegistration{}, ErrPersonalRegistration
	}
	if err := registration.Validate(); err != nil {
		return PersonalRegistration{}, err
	}
	prepared, err := s.resources.Prepare(ctx, registration)
	if err != nil {
		return PersonalRegistration{}, err
	}
	return s.ready(ctx, registration, prepared)
}

// Resolve recovers only a previously accepted local preparation, including missing
// writer ACKs under the original deterministic control identity. It creates no scope.
func (s *ReadyPersonalRegistrations) Resolve(ctx context.Context, registration PersonalRegistration) (PersonalRegistration, error) {
	if s == nil {
		return PersonalRegistration{}, ErrPersonalRegistration
	}
	if err := registration.Validate(); err != nil {
		return PersonalRegistration{}, err
	}
	prepared, err := s.resources.Resolve(ctx, registration)
	if err != nil {
		return PersonalRegistration{}, err
	}
	return s.ready(ctx, registration, prepared)
}
func (s *ReadyPersonalRegistrations) ready(ctx context.Context, requested, prepared PersonalRegistration) (PersonalRegistration, error) {
	if requested != prepared {
		return PersonalRegistration{}, ErrPersonalRegistration
	}
	control, err := s.authority.PreparePersonalScopeAuthority(ctx, prepared)
	if err != nil {
		return PersonalRegistration{}, fmt.Errorf("prepare accepted personal scope authority: %w", err)
	}
	if control != InitialScopeControl(prepared) {
		return PersonalRegistration{}, writefence.ErrConflict
	}
	expected := control.Authority
	expected.Epoch = 0
	state, err := s.writers.Set(ctx, expected, control.CoordinationID, false)
	if err != nil {
		return PersonalRegistration{}, fmt.Errorf("install accepted personal scope authority: %w", err)
	}
	if state.Control != control || !state.Complete || len(state.Acknowledged) != len(writefence.Writers()) {
		return PersonalRegistration{}, writefence.ErrConflict
	}
	seen := map[writefence.Writer]bool{}
	for _, writer := range state.Acknowledged {
		if writer.Validate() != nil || seen[writer] {
			return PersonalRegistration{}, writefence.ErrConflict
		}
		seen[writer] = true
	}
	if err := s.authority.CheckPersonalScopeAuthority(ctx, prepared, control); err != nil {
		return PersonalRegistration{}, err
	}
	return prepared, nil
}
