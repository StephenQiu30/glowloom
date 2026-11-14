package microservices_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

type coordinationRepository struct {
	state writefence.Coordination
	begun bool
	acks  []writefence.Writer
}

func (r *coordinationRepository) Begin(_ context.Context, authority writefence.Authority, id uuid.UUID, blocked bool) (writefence.Coordination, error) {
	if !r.begun {
		authority.Epoch++
		r.state.Control = writefence.Control{Authority: authority, CoordinationID: id, Blocked: blocked}
		r.begun = true
	}
	return r.state, nil
}

func (r *coordinationRepository) Acknowledge(_ context.Context, control writefence.Control, writer writefence.Writer) (writefence.Coordination, error) {
	if control != r.state.Control {
		return writefence.Coordination{}, writefence.ErrConflict
	}
	r.acks = append(r.acks, writer)
	r.state.Acknowledged = append(r.state.Acknowledged, writer)
	r.state.Complete = len(r.state.Acknowledged) == len(writefence.Writers())
	return r.state, nil
}

func (r *coordinationRepository) LoadCoordination(context.Context, uuid.UUID) (writefence.Coordination, error) {
	return r.state, nil
}

type coordinationTransport struct {
	repository *coordinationRepository
	fail       writefence.Writer
	installed  []writefence.Writer
}

func (w *coordinationTransport) Install(_ context.Context, writer writefence.Writer, control writefence.Control) error {
	if !w.repository.begun || control != w.repository.state.Control {
		return errors.New("control sent before durable closure")
	}
	w.installed = append(w.installed, writer)
	if writer == w.fail {
		return writefence.ErrUnavailable
	}
	return nil
}

func TestAuthorityCoordinatorResumesOnlyUnacknowledgedWriters(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "open", true: "block"}[blocked], func(t *testing.T) {
			repository := &coordinationRepository{}
			transport := &coordinationTransport{repository: repository, fail: writefence.WriterMedia}
			authority := writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: uuid.New()}
			id := uuid.New()
			state, err := writefence.NewCoordinator(repository, transport).Set(t.Context(), authority, id, blocked)
			if !errors.Is(err, writefence.ErrUnavailable) || state.Complete || len(repository.acks) != 2 {
				t.Fatalf("unknown install falsely acknowledged: %+v %v, ACKs=%v", state, err, repository.acks)
			}
			transport.fail = ""
			transport.installed = nil
			state, err = writefence.NewCoordinator(repository, transport).Set(t.Context(), authority, id, blocked)
			if err != nil || !state.Complete || state.Control.Epoch != 1 {
				t.Fatalf("resume original coordination: %+v %v", state, err)
			}
			if !reflect.DeepEqual(transport.installed, []writefence.Writer{writefence.WriterMedia, writefence.WriterExecution}) {
				t.Fatalf("resent durable ACKs or changed writer set: %v", transport.installed)
			}
			transport.installed = nil
			if _, err := writefence.NewCoordinator(repository, transport).Set(t.Context(), authority, id, blocked); err != nil || len(transport.installed) != 0 {
				t.Fatalf("completed history propagated stale control: %v", err)
			}
		})
	}
}

func TestAuthorityWritersAreClosedAndCopied(t *testing.T) {
	writers := writefence.Writers()
	if !reflect.DeepEqual(writers, []writefence.Writer{writefence.WriterIdentity, writefence.WriterCreation, writefence.WriterMedia, writefence.WriterExecution}) {
		t.Fatalf("writer contract changed: %v", writers)
	}
	writers[0] = "forged"
	for _, writer := range writefence.Writers() {
		if err := writer.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, writer := range []writefence.Writer{"", "legacy", "https://attacker.invalid"} {
		if !errors.Is(writer.Validate(), writefence.ErrInvalid) {
			t.Fatalf("accepted writer %q", writer)
		}
	}
}

func TestAuthorityCoordinatorRejectsInvalidInputBeforeRepository(t *testing.T) {
	repository := &coordinationRepository{}
	transport := &coordinationTransport{repository: repository}
	coordinator := writefence.NewCoordinator(repository, transport)
	authority := writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: uuid.New()}
	for _, change := range []func(*writefence.Authority){
		func(a *writefence.Authority) { a.Epoch = -1 },
		func(a *writefence.Authority) { a.Issuer = "media" },
		func(a *writefence.Authority) { a.Kind = writefence.Project },
		func(a *writefence.Authority) { a.ID = uuid.Nil },
	} {
		invalid := authority
		change(&invalid)
		if _, err := coordinator.Set(t.Context(), invalid, uuid.New(), false); !errors.Is(err, writefence.ErrInvalid) {
			t.Fatalf("invalid coordination: %v", err)
		}
	}
	if _, err := coordinator.Set(t.Context(), authority, uuid.Nil, false); !errors.Is(err, writefence.ErrInvalid) {
		t.Fatal(err)
	}
	if repository.begun {
		t.Fatal("invalid input reached persistence")
	}
	if _, err := writefence.NewCoordinator(nil, nil).Set(t.Context(), authority, uuid.New(), false); !errors.Is(err, writefence.ErrUnavailable) {
		t.Fatal(err)
	}
	if !errors.Is(authority.Validate(), writefence.ErrInvalid) {
		t.Fatal("command authority accepted epoch zero")
	}
}

func TestAuthorityCoordinatorDoesNotTrustIncompleteOrForeignEvidence(t *testing.T) {
	authority := writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: uuid.New()}
	id := uuid.New()
	for _, change := range []func(*writefence.Coordination){
		func(c *writefence.Coordination) { c.Control.ID = uuid.New() },
		func(c *writefence.Coordination) { c.Control.CoordinationID = uuid.New() },
		func(c *writefence.Coordination) { c.Control.Epoch++ },
		func(c *writefence.Coordination) { c.Control.Blocked = true },
		func(c *writefence.Coordination) { c.Complete = true },
		func(c *writefence.Coordination) { c.Acknowledged = []writefence.Writer{"forged"} },
		func(c *writefence.Coordination) {
			c.Acknowledged = []writefence.Writer{writefence.WriterMedia, writefence.WriterMedia}
		},
	} {
		repository := &coordinationRepository{begun: true, state: writefence.Coordination{Control: writefence.Control{Authority: authority, CoordinationID: id}}}
		repository.state.Control.Epoch = 1
		change(&repository.state)
		transport := &coordinationTransport{repository: repository}
		if _, err := writefence.NewCoordinator(repository, transport).Set(t.Context(), authority, id, false); !errors.Is(err, writefence.ErrConflict) {
			t.Fatalf("foreign or incomplete durable evidence accepted: %v", err)
		}
		if len(transport.installed) != 0 || len(repository.acks) != 0 {
			t.Fatal("inexact evidence reached a writer")
		}
	}
}
