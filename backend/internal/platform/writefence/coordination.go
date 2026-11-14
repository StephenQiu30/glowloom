package writefence

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
)

// Writer is a fixed business-effect owner, never a request-supplied URL.
type Writer string

// All four writers must durably install each control before it is complete.
const (
	WriterIdentity  Writer = "identity"
	WriterCreation  Writer = "creation"
	WriterMedia     Writer = "media"
	WriterExecution Writer = "execution"
)

// Writers returns an independent copy in the fixed propagation order.
func Writers() []Writer {
	return []Writer{WriterIdentity, WriterCreation, WriterMedia, WriterExecution}
}

// Validate rejects caller-defined services and endpoints.
func (w Writer) Validate() error {
	switch w {
	case WriterIdentity, WriterCreation, WriterMedia, WriterExecution:
		return nil
	default:
		return ErrInvalid
	}
}

// Coordination retains the exact control and durable writer acknowledgements.
// Complete is evidence of all writer ACKs, not a grant of business permission.
type Coordination struct {
	Control      Control
	Acknowledged []Writer
	Complete     bool
}

// AuthorityRepository persists closure before any RPC. Begin accepts an expected
// current epoch (zero only for initialization); history binds that exact input.
type AuthorityRepository interface {
	Begin(context.Context, Authority, uuid.UUID, bool) (Coordination, error)
	Acknowledge(context.Context, Control, Writer) (Coordination, error)
	LoadCoordination(context.Context, uuid.UUID) (Coordination, error)
}

// ControlWriter routes only to deployment-configured, authenticated writers.
// A nil error must mean the exact control was installed durably, not queued.
type ControlWriter interface {
	Install(context.Context, Writer, Control) error
}

// Coordinator closes local admission, propagates controls without holding SQL
// locks across RPCs, and resumes unknown outcomes under their original identity.
type Coordinator struct {
	repository AuthorityRepository
	writers    ControlWriter
}

// NewCoordinator requires issuer-local persistence and a closed control adapter.
func NewCoordinator(repository AuthorityRepository, writers ControlWriter) *Coordinator {
	return &Coordinator{repository: repository, writers: writers}
}

// Set coordinates both opening and blocking. A partially propagated open stays
// locally closed until every writer ACK is durable. Errors never undo closure.
func (c *Coordinator) Set(ctx context.Context, authority Authority, coordinationID uuid.UUID, blocked bool) (Coordination, error) {
	if authority.Epoch < 0 || authority.Epoch == math.MaxInt64 || coordinationID == uuid.Nil {
		return Coordination{}, ErrInvalid
	}
	validated := authority
	validated.Epoch = 1
	if err := validated.Validate(); err != nil {
		return Coordination{}, err
	}
	if c == nil || c.repository == nil || c.writers == nil {
		return Coordination{}, ErrUnavailable
	}
	state, err := c.repository.Begin(ctx, authority, coordinationID, blocked)
	if err != nil {
		return state, err
	}
	if state.Control.Key() != authority.Key() || state.Control.Epoch != authority.Epoch+1 || state.Control.CoordinationID != coordinationID || state.Control.Blocked != blocked {
		return state, ErrConflict
	}
	acknowledged := make(map[Writer]bool, len(state.Acknowledged))
	for _, writer := range state.Acknowledged {
		if writer.Validate() != nil || acknowledged[writer] {
			return state, ErrConflict
		}
		acknowledged[writer] = true
	}
	if state.Complete != (len(acknowledged) == len(Writers())) {
		return state, ErrConflict
	}
	for _, writer := range Writers() {
		if acknowledged[writer] {
			continue
		}
		if err := c.writers.Install(ctx, writer, state.Control); err != nil {
			return state, fmt.Errorf("install authority at %s: %w", writer, err)
		}
		next, err := c.repository.Acknowledge(ctx, state.Control, writer)
		if err != nil {
			return state, fmt.Errorf("persist authority ACK from %s: %w", writer, err)
		}
		state = next
	}
	return state, nil
}
