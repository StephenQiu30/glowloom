package identity_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

type registrationPrepareStore struct {
	value     identity.RegistrationPreparation
	activated bool
	saves     int
}

func (s *registrationPrepareStore) LoadRegistrationPreparation(context.Context, uuid.UUID, []byte) (identity.RegistrationPreparation, error) {
	return s.value, nil
}
func (s *registrationPrepareStore) RecordRegistrationReady(_ context.Context, p identity.RegistrationPreparation, r identity.RegistrationReady) error {
	if r != p.ExpectedReady(r.Owner) {
		return identity.ErrRegistrationReceipt
	}
	if r.Owner == "creation" {
		s.value.CreationReady = true
	} else {
		s.value.WalletReady = true
	}
	s.saves++
	return nil
}
func (s *registrationPrepareStore) PrepareRegistrationAccount(_ context.Context, p identity.RegistrationPreparation) (writefence.Control, error) {
	return p.AccountControl(), nil
}

type registrationFence struct{}

func (registrationFence) Set(_ context.Context, a writefence.Authority, id uuid.UUID, blocked bool) (writefence.Coordination, error) {
	a.Epoch++
	return writefence.Coordination{Control: writefence.Control{Authority: a, CoordinationID: id, Blocked: blocked}, Acknowledged: writefence.Writers(), Complete: true}, nil
}

func (s *registrationPrepareStore) ActivateRegistration(_ context.Context, p identity.RegistrationPreparation) (uuid.UUID, error) {
	if !s.value.CreationReady || !s.value.WalletReady {
		return uuid.Nil, errors.New("activation before durable ready receipts")
	}
	s.activated = true
	return p.AccountID, nil
}

type registrationParticipant struct {
	owner  string
	calls  int
	fail   error
	mutate func(*identity.RegistrationReady)
}

func (p *registrationParticipant) PrepareRegistration(_ context.Context, v identity.RegistrationPreparation) (identity.RegistrationReady, error) {
	p.calls++
	r := v.ExpectedReady(p.owner)
	if p.mutate != nil {
		p.mutate(&r)
	}
	return r, p.fail
}

func TestRegistrationPreparationUnknownRetainsOriginalCommand(t *testing.T) {
	p := identity.RegistrationPreparation{OperationID: uuid.New(), AccountID: uuid.New(), OrganizationID: uuid.New(), ScopeID: uuid.New(), InputSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	s := &registrationPrepareStore{value: p}
	c := &registrationParticipant{owner: "creation"}
	w := &registrationParticipant{owner: "execution", fail: errors.New("ACK lost")}
	r, err := identity.NewRegistrationPreparation(s, c, w, registrationFence{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resume(t.Context(), p.OperationID, make([]byte, 32)); err == nil || s.activated || !s.value.CreationReady || s.value.WalletReady {
		t.Fatalf("unknown result lost durable preparation: %v %#v", err, s)
	}
	w.fail = nil
	id, err := r.Resume(t.Context(), p.OperationID, make([]byte, 32))
	if err != nil || id != p.AccountID || !s.activated || c.calls != 1 || w.calls != 2 {
		t.Fatalf("resume = %s, %v; creation=%d wallet=%d", id, err, c.calls, w.calls)
	}
}

func TestRegistrationPreparationRejectsInexactReceipt(t *testing.T) {
	for _, change := range []func(*identity.RegistrationReady){
		func(r *identity.RegistrationReady) { r.OperationID = uuid.New() },
		func(r *identity.RegistrationReady) { r.AccountID = uuid.New() },
		func(r *identity.RegistrationReady) { r.ScopeID = uuid.New() },
		func(r *identity.RegistrationReady) { r.OrganizationID = uuid.New() },
		func(r *identity.RegistrationReady) {
			r.InputSHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		},
		func(r *identity.RegistrationReady) { r.Owner = "execution" },
	} {
		p := identity.RegistrationPreparation{OperationID: uuid.New(), AccountID: uuid.New(), OrganizationID: uuid.New(), ScopeID: uuid.New(), InputSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
		s := &registrationPrepareStore{value: p}
		c := &registrationParticipant{owner: "creation", mutate: change}
		w := &registrationParticipant{owner: "execution"}
		r, err := identity.NewRegistrationPreparation(s, c, w, registrationFence{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Resume(t.Context(), p.OperationID, make([]byte, 32)); !errors.Is(err, identity.ErrRegistrationReceipt) || s.activated || s.saves != 0 || w.calls != 0 {
			t.Fatalf("inexact receipt accepted: %v", err)
		}
	}
}

func TestRegistrationPreparationRequiresEveryDependency(t *testing.T) {
	s := &registrationPrepareStore{}
	p := &registrationParticipant{}
	for _, test := range []struct {
		store            identity.RegistrationPreparationStore
		creation, wallet identity.RegistrationParticipant
		writers          identity.SessionFenceCoordinator
	}{{nil, p, p, registrationFence{}}, {s, nil, p, registrationFence{}}, {s, p, nil, registrationFence{}}, {s, p, p, nil}} {
		if _, err := identity.NewRegistrationPreparation(test.store, test.creation, test.wallet, test.writers); err == nil {
			t.Fatal("partial registration preparation enabled")
		}
	}
}
