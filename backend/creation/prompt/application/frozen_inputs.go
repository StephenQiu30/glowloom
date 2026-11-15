package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// FreezeInputRequest selects an existing source, never an inline substitute.
// Command must already be admitted by the Identity/Creation policy pipeline.
type FreezeInputRequest struct {
	HandoffID               uuid.UUID          `json:"handoff_id"`
	RequestID               uuid.UUID          `json:"request_id"`
	Command                 writefence.Command `json:"command"`
	SourceCommand           writefence.Command `json:"source_command"`
	ActorID                 uuid.UUID          `json:"actor_id"`
	SessionID               uuid.UUID          `json:"session_id"`
	ScopeID                 uuid.UUID          `json:"scope_id"`
	ProjectID               uuid.UUID          `json:"project_id"`
	Source                  domain.InputSource `json:"source"`
	SourceGeneration        int64              `json:"source_generation"`
	ExpectedScopeRevision   int64              `json:"expected_scope_revision"`
	ExpectedProjectRevision int64              `json:"expected_project_revision"`
	Pins                    []domain.FrozenPin `json:"pins"`
	TemplateSelection       *TemplateRequest   `json:"template_selection,omitempty"`
}

// SourceCommandSHA256 binds Creation's separate local effect to the original
// Execution command and complete source selection, request key and pin IDs.
// The command digest and returned Media receipt digests are omitted: Media's
// receipt itself binds the consumer command digest. The final snapshot hash binds
// those independently authenticated receipt digests after acquisition.
func (r FreezeInputRequest) SourceCommandSHA256() (string, error) {
	return frozenInputSourceSHA256(r)
}

// FrozenInputPins resolves the exact pre-acquired Media pins outside SQL. The
// implementation must authenticate Media and check consumer=creation, original
// command/hash, handoff resource, scope/project and immutable pin facts. A nil
// port permits only inputs without media, never trusts request pin assertions.
type FrozenInputPins interface {
	VerifyFrozenInputPins(context.Context, FreezeInputRequest) ([]domain.FrozenPin, error)
}

// FrozenInputKey binds recovery to all original immutable identities.
type FrozenInputKey struct {
	HandoffID      uuid.UUID `json:"handoff_id"`
	CommandID      uuid.UUID `json:"command_id"`
	CommandSHA256  string    `json:"command_sha256"`
	SnapshotSHA256 string    `json:"snapshot_sha256"`
}

// FrozenInputCancellationSHA256 binds a separately admitted cancellation to the
// exact original handoff/command/snapshot, preventing one command closing two inputs.
func FrozenInputCancellationSHA256(key FrozenInputKey, command writefence.Command) (string, error) {
	command.SHA256 = ""
	raw, err := json.Marshal(struct {
		Key     FrozenInputKey     `json:"key"`
		Command writefence.Command `json:"command"`
	}{key, command})
	if err != nil {
		return "", domain.ErrFrozenInputInvalid
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}

// FrozenInputEnvelope includes the permanent source-side adoption gate. Reading
// immutable facts is recovery only; it never grants new policy authority.
type FrozenInputEnvelope struct {
	Input           domain.FrozenInput      `json:"input"`
	SnapshotSHA256  string                  `json:"snapshot_sha256"`
	AdoptionBlocked bool                    `json:"adoption_blocked"`
	CancelCommand   *writefence.Command     `json:"cancel_command,omitempty"`
	CancelProof     *FrozenInputCancelProof `json:"cancel_proof,omitempty"`
}

// Key returns the immutable identity required for every recovery operation.
func (e FrozenInputEnvelope) Key() FrozenInputKey {
	return FrozenInputKey{e.Input.HandoffID, e.Input.Command.ID, e.Input.Command.SHA256, e.SnapshotSHA256}
}

// FrozenInputCancelProof is the consumer's closed, original-command evidence.
// cancelled_input means a received snapshot was closed; it never asserts a refund
// or cancellation of a reserved execution. Actual money remains in existing use cases.
type FrozenInputCancelProof struct {
	Key             FrozenInputKey    `json:"key"`
	CancelCommandID uuid.UUID         `json:"cancel_command_id"`
	Outcome         string            `json:"outcome"`
	Original        writefence.Record `json:"original"`
}

// FrozenInputStore owns all Creation transactions; no port accepts a caller Tx.
type FrozenInputStore interface {
	FreezeInput(context.Context, FreezeInputRequest) (FrozenInputEnvelope, error)
	ReadFrozenInput(context.Context, FrozenInputKey) (FrozenInputEnvelope, error)
	BlockFrozenInput(context.Context, FrozenInputKey, writefence.Command) (FrozenInputEnvelope, error)
	CompleteFrozenInputCancellation(context.Context, FrozenInputKey, FrozenInputCancelProof) (FrozenInputEnvelope, error)
}

// FrozenInputCancellationTarget closes only input adoption at the consumer.
type FrozenInputCancellationTarget interface {
	CancelFrozenInput(context.Context, FrozenInputKey, writefence.Command) (FrozenInputCancelProof, error)
}

// FrozenInputCancellation persists the prohibition before making any network
// call. Lost ACK recovery keeps the same command and exact target receipt.
type FrozenInputCancellation struct {
	store  FrozenInputStore
	target FrozenInputCancellationTarget
}

// NewFrozenInputCancellation injects the local source and authenticated target.
func NewFrozenInputCancellation(store FrozenInputStore, target FrozenInputCancellationTarget) *FrozenInputCancellation {
	return &FrozenInputCancellation{store, target}
}

// Cancel durably blocks adoption before requesting and sealing the target proof.
func (c *FrozenInputCancellation) Cancel(ctx context.Context, key FrozenInputKey, command writefence.Command) (FrozenInputEnvelope, error) {
	if c == nil || c.store == nil || c.target == nil {
		return FrozenInputEnvelope{}, writefence.ErrUnavailable
	}
	envelope, err := c.store.BlockFrozenInput(ctx, key, command)
	if err != nil || envelope.CancelProof != nil {
		return envelope, err
	}
	proof, err := c.target.CancelFrozenInput(ctx, key, command)
	if err != nil {
		return envelope, err
	}
	return c.store.CompleteFrozenInputCancellation(ctx, key, proof)
}

// SameFrozenCommand compares complete authority bindings without trusting order.
func SameFrozenCommand(a, b writefence.Command) bool {
	ad, ae := a.AuthorityDigest()
	bd, be := b.AuthorityDigest()
	return ae == nil && be == nil && a.ID == b.ID && a.SHA256 == b.SHA256 && a.Action == b.Action && ad == bd
}

// ValidateFrozenCancelProof distinguishes a permanently frozen old command from
// a mere state string or an accepted snapshot whose adoption was later closed.
func ValidateFrozenCancelProof(e FrozenInputEnvelope, p FrozenInputCancelProof) error {
	if e.CancelCommand == nil || !e.AdoptionBlocked || p.Key != e.Key() || p.CancelCommandID != e.CancelCommand.ID || !SameFrozenCommand(e.Input.Command, p.Original.Command) {
		return domain.ErrFrozenInputConflict
	}
	switch p.Outcome {
	case "not_received_frozen":
		if p.Original.State != writefence.Frozen || len(p.Original.Receipt) != 0 {
			return domain.ErrFrozenInputConflict
		}
	case "cancelled_input":
		if p.Original.State != writefence.Committed {
			return domain.ErrFrozenInputConflict
		}
		var r struct {
			HandoffID      uuid.UUID `json:"handoff_id"`
			RequestID      uuid.UUID `json:"request_id"`
			CommandID      uuid.UUID `json:"command_id"`
			CommandSHA256  string    `json:"command_sha256"`
			SnapshotSHA256 string    `json:"snapshot_sha256"`
			OperationID    uuid.UUID `json:"operation_id"`
			SubmissionID   uuid.UUID `json:"submission_id"`
			ReceiptID      uuid.UUID `json:"receipt_id"`
		}
		if json.Unmarshal(p.Original.Receipt, &r) != nil || r.HandoffID != p.Key.HandoffID || r.RequestID != e.Input.RequestID || r.CommandID != p.Key.CommandID || r.CommandSHA256 != p.Key.CommandSHA256 || r.SnapshotSHA256 != p.Key.SnapshotSHA256 || r.OperationID != uuid.NewSHA1(e.Input.HandoffID, []byte("execution.operation")) || r.SubmissionID != uuid.NewSHA1(e.Input.HandoffID, []byte("execution.submission")) || r.ReceiptID != uuid.NewSHA1(e.Input.HandoffID, []byte("execution.frozen_input.acceptance")) {
			return domain.ErrFrozenInputConflict
		}
	default:
		return domain.ErrFrozenInputConflict
	}
	return nil
}
