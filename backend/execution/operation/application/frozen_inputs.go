package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// Errors distinguish invalid input, identity conflict and permanent cancellation.
var (
	ErrFrozenInputInvalid   = errors.New("invalid execution frozen input")
	ErrFrozenInputConflict  = errors.New("execution frozen input conflict")
	ErrFrozenInputCancelled = errors.New("execution frozen input permanently cancelled")
	ErrFrozenInputOwner     = errors.New("execution frozen input requires execution role")
)

// FrozenInputKey contains only immutable recovery identity, never mutable draft IDs.
type FrozenInputKey struct {
	HandoffID      uuid.UUID `json:"handoff_id"`
	CommandID      uuid.UUID `json:"command_id"`
	CommandSHA256  string    `json:"command_sha256"`
	SnapshotSHA256 string    `json:"snapshot_sha256"`
}

// FrozenInput is the consumer's minimum authenticated Creation contract. The
// snapshot remains immutable input; it is not an assertion of current permission,
// a quote, an operation reservation, or a provider submission.
type FrozenInput struct {
	Key                                           FrozenInputKey
	RequestID                                     uuid.UUID
	Command                                       writefence.Command
	ActorID, SessionID, ScopeID, ProjectID, OrgID uuid.UUID
	AspectRatio                                   string
	SourceGeneration, DraftRevision               int64
	Snapshot                                      json.RawMessage
	AdoptionBlocked                               bool
	CancelCommand                                 *writefence.Command
}

// Validate checks authenticated content identity and the complete authority binding.
func (in FrozenInput) Validate() error {
	if in.Key.HandoffID == uuid.Nil || in.Key.CommandID != in.Command.ID || in.Key.CommandSHA256 != in.Command.SHA256 || in.RequestID == uuid.Nil || in.SourceGeneration < 1 || in.DraftRevision < 1 || len(in.Snapshot) == 0 || len(in.Snapshot) > 768<<10 || in.Command.Action != "execution.frozen_input.accept" || in.ActorID == uuid.Nil || in.SessionID == uuid.Nil || in.ScopeID == uuid.Nil || in.ProjectID == uuid.Nil || in.OrgID == uuid.Nil || (in.AspectRatio != "9:16" && in.AspectRatio != "16:9" && in.AspectRatio != "1:1") || in.AdoptionBlocked != (in.CancelCommand != nil) {
		return ErrFrozenInputInvalid
	}
	digest := sha256.Sum256(in.Snapshot)
	if hex.EncodeToString(digest[:]) != in.Key.SnapshotSHA256 {
		return ErrFrozenInputConflict
	}
	if _, err := in.Command.AuthorityDigest(); err != nil || len(in.Command.Authorities) != 4 {
		return ErrFrozenInputInvalid
	}
	expected := map[string]uuid.UUID{"identity/account": in.ActorID, "identity/session": in.SessionID, "creation/scope": in.ScopeID, "creation/project": in.ProjectID}
	for _, a := range in.Command.Authorities {
		if expected[string(a.Issuer)+"/"+string(a.Kind)] != a.ID {
			return ErrFrozenInputInvalid
		}
	}
	return nil
}

// FrozenInputs reads authenticated, closed Creation facts outside local SQL. It
// must verify every metadata field against the full canonical snapshot content.
type FrozenInputs interface {
	ReadFrozenInput(context.Context, FrozenInputKey) (FrozenInput, error)
}

// FrozenInputReceipt binds deterministic local identities without creating those
// operations or submissions. The existing quote/confirm use cases retain funding.
type FrozenInputReceipt struct {
	HandoffID      uuid.UUID `json:"handoff_id"`
	RequestID      uuid.UUID `json:"request_id"`
	CommandID      uuid.UUID `json:"command_id"`
	CommandSHA256  string    `json:"command_sha256"`
	SnapshotSHA256 string    `json:"snapshot_sha256"`
	OperationID    uuid.UUID `json:"operation_id"`
	SubmissionID   uuid.UUID `json:"submission_id"`
	ReceiptID      uuid.UUID `json:"receipt_id"`
}

// NewFrozenInputReceipt derives stable local identities from the original handoff.
func NewFrozenInputReceipt(in FrozenInput) FrozenInputReceipt {
	return FrozenInputReceipt{in.Key.HandoffID, in.RequestID, in.Command.ID, in.Command.SHA256, in.Key.SnapshotSHA256, uuid.NewSHA1(in.Key.HandoffID, []byte("execution.operation")), uuid.NewSHA1(in.Key.HandoffID, []byte("execution.submission")), uuid.NewSHA1(in.Key.HandoffID, []byte("execution.frozen_input.acceptance"))}
}

// FrozenInputCancelProof retains the exact old writer command evidence. An input
// ACK can be cancelled; no field here purports to refund or stop a reservation.
type FrozenInputCancelProof struct {
	Key             FrozenInputKey    `json:"key"`
	CancelCommandID uuid.UUID         `json:"cancel_command_id"`
	Outcome         string            `json:"outcome"`
	Original        writefence.Record `json:"original"`
}

// FrozenInputAcceptance is the private consumer handler's entire business port.
type FrozenInputAcceptance interface {
	AcceptFrozenInput(context.Context, FrozenInputKey) (FrozenInputReceipt, error)
	CancelFrozenInput(context.Context, FrozenInputKey, writefence.Command) (FrozenInputCancelProof, error)
}

// SameFrozenInputCommand compares the complete original authority set and identity.
func SameFrozenInputCommand(a, b writefence.Command) bool {
	ad, ae := a.AuthorityDigest()
	bd, be := b.AuthorityDigest()
	return ae == nil && be == nil && a.ID == b.ID && a.SHA256 == b.SHA256 && a.Action == b.Action && ad == bd
}
