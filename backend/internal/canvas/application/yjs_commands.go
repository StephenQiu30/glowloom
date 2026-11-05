package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// YjsCommandInput contains semantic business operations; transport identities
// are supplied by the trusted owning use case, never model arguments.
type YjsCommandInput struct {
	CommandID         uuid.UUID          `json:"-"`
	SessionID         uuid.UUID          `json:"-"`
	ExpectedRevision  int64              `json:"expected_revision"`
	ExpectedCommitSeq *Revision          `json:"expected_commit_seq,omitempty" swaggertype:"string"`
	Operations        []domain.Operation `json:"operations"`
}

// PreparedYjsCommands has no durable effects. Acceptance must happen inside the
// owning business transaction after its current authorization is revalidated.
type PreparedYjsCommands struct {
	Base           YjsState
	Update         YjsUpdate
	Candidate      YjsCandidate
	Initialization *YjsCandidate
}

// YjsCommandRuntime applies a domain-evaluated candidate through native shared
// Yjs operations, retaining prior container identities and committed history.
type YjsCommandRuntime interface {
	YjsRuntime
	Commands(context.Context, YjsState, uuid.UUID, DocumentDTO) (YjsCandidate, error)
}

// PrepareCommands performs isolated computation outside any database transaction.
func (s *YjsService) PrepareCommands(ctx context.Context, a identity.Principal, id uuid.UUID, in YjsCommandInput) (PreparedYjsCommands, error) {
	runtime, ok := s.runtime.(YjsCommandRuntime)
	if !ok {
		return PreparedYjsCommands{}, fmt.Errorf("canvas command runtime unavailable")
	}
	if in.CommandID == uuid.Nil || in.SessionID == uuid.Nil || in.ExpectedRevision < 1 || len(in.Operations) == 0 || len(in.Operations) > 100 {
		return PreparedYjsCommands{}, domain.ErrInvalidCommand
	}
	base, err := s.store.LoadYjs(ctx, a, id)
	if err != nil {
		return PreparedYjsCommands{}, err
	}
	if int64(base.Document.Revision) != in.ExpectedRevision || in.ExpectedCommitSeq != nil && *in.ExpectedCommitSeq != base.CommitSeq {
		return PreparedYjsCommands{}, ErrYjsContention
	}
	if !base.Document.Permissions.Write {
		return PreparedYjsCommands{}, domain.ErrGraphReadOnly
	}
	if base.CommandDocument.ID != id || base.CommandDocument.Revision != in.ExpectedRevision {
		return PreparedYjsCommands{}, fmt.Errorf("missing atomic command document")
	}
	result := PreparedYjsCommands{Base: base}
	if len(base.State) == 0 {
		base.Epoch = uuid.NewSHA1(in.CommandID, []byte("canvas-command-initialization"))
		base.WriterTerm = 1
		base.CommitSeq = 0
		initialized, err := runtime.Initialize(ctx, base)
		if err != nil {
			return PreparedYjsCommands{}, err
		}
		if err = validateYjsCandidate(base, initialized); err != nil {
			return PreparedYjsCommands{}, err
		}
		result.Initialization = &initialized
		base.State = initialized.State
		base.GraphHash = initialized.GraphHash
	}
	evaluated, err := domain.ApplyOperations(base.CommandDocument, in.Operations)
	if err != nil {
		return PreparedYjsCommands{}, err
	}
	if err = ValidateSnapshotCapacity(evaluated.Document); err != nil {
		return PreparedYjsCommands{}, err
	}
	projection := Snapshot(evaluated.Document)
	projection.Name = base.Document.Name
	projection.Permissions = base.Document.Permissions
	candidate, err := runtime.Commands(ctx, base, in.CommandID, projection)
	if err != nil {
		return PreparedYjsCommands{}, err
	}
	if err = validateYjsCandidate(base, candidate); err != nil {
		return PreparedYjsCommands{}, err
	}
	update := YjsUpdate{Epoch: base.Epoch, SessionID: in.SessionID, UpdateID: in.CommandID, Update: candidate.CommandUpdate, PayloadHash: candidate.PayloadHash}
	if candidate.UpdateID != in.CommandID || validYjsUpdate(update) != nil {
		return PreparedYjsCommands{}, domain.ErrInvalidCommand
	}
	// The runtime's payload hash belongs to exactly the returned delta. The same
	// delta is committed and replayed, never recomputed against a different prefix.
	result.Base = base
	result.Update = update
	result.Candidate = candidate
	return result, nil
}

// YjsCommandsStore atomically accepts initialization, Yjs state/projection and receipt.
type YjsCommandsStore interface {
	CommitYjsCommands(context.Context, identity.Principal, PreparedYjsCommands) (YjsReceipt, error)
}

// CommitCommands accepts a prepared command under current owner permission and
// exact collaboration revision. The persistence adapter owns atomicity.
func (s *YjsService) CommitCommands(ctx context.Context, a identity.Principal, in PreparedYjsCommands) (YjsReceipt, error) {
	if err := validYjsUpdate(in.Update); err != nil {
		return YjsReceipt{}, err
	}
	if err := validateYjsCandidate(in.Base, in.Candidate); err != nil {
		return YjsReceipt{}, err
	}
	store, ok := s.store.(YjsCommandsStore)
	if !ok {
		return YjsReceipt{}, fmt.Errorf("atomic canvas command store unavailable")
	}
	return store.CommitYjsCommands(ctx, a, in)
}
