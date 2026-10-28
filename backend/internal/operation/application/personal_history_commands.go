package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ErrHistoryCommandConflict prevents reusing a key for different personal changes.
var ErrHistoryCommandConflict = errors.New("personal history idempotency conflict")

// PersonalHistoryTarget selects a candidate, or the whole record when OutputID is null.
type PersonalHistoryTarget struct {
	OperationID      uuid.UUID  `json:"operation_id"`
	OutputID         *uuid.UUID `json:"output_id"`
	ExpectedRevision int64      `json:"expected_revision"`
}

// UnmarshalJSON requires an explicit revision, including the valid initial zero.
func (t *PersonalHistoryTarget) UnmarshalJSON(data []byte) error {
	var wire struct {
		OperationID      uuid.UUID  `json:"operation_id"`
		OutputID         *uuid.UUID `json:"output_id"`
		ExpectedRevision *int64     `json:"expected_revision"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return ErrInvalidPublicQuery
	}
	if wire.ExpectedRevision == nil || decoder.Decode(new(any)) != io.EOF {
		return ErrInvalidPublicQuery
	}
	*t = PersonalHistoryTarget{OperationID: wire.OperationID, OutputID: wire.OutputID, ExpectedRevision: *wire.ExpectedRevision}
	return nil
}

// PersonalHistoryCommand explicitly sets favorite state or removes a personal listing.
type PersonalHistoryCommand struct {
	Action    string                  `json:"action"`
	Favorited *bool                   `json:"favorited,omitempty"`
	Targets   []PersonalHistoryTarget `json:"targets"`
}

// PersonalHistoryCommandResult retains each target's outcome in a partial batch.
type PersonalHistoryCommandResult struct {
	OperationID       uuid.UUID   `json:"operation_id"`
	OutputID          *uuid.UUID  `json:"output_id"`
	OK                bool        `json:"ok"`
	Code              string      `json:"code"`
	Revision          int64       `json:"revision"`
	AffectedOutputIDs []uuid.UUID `json:"affected_output_ids"`
}

// PersonalHistoryCommandReceipt is persisted and returned identically on replay.
type PersonalHistoryCommandReceipt struct {
	RequestID uuid.UUID                      `json:"request_id"`
	Action    string                         `json:"action"`
	Results   []PersonalHistoryCommandResult `json:"results"`
}

// PersonalHistoryCommandStore owns authorization, concurrency and the durable receipt.
type PersonalHistoryCommandStore interface {
	ApplyPersonalHistoryCommand(context.Context, identity.Principal, uuid.UUID, PersonalHistoryCommand) (PersonalHistoryCommandReceipt, error)
}

// PersonalHistoryCommander validates personal result management independently of transport.
type PersonalHistoryCommander struct{ store PersonalHistoryCommandStore }

// NewPersonalHistoryCommander injects the durable personal metadata store.
func NewPersonalHistoryCommander(store PersonalHistoryCommandStore) *PersonalHistoryCommander {
	return &PersonalHistoryCommander{store: store}
}

// Apply validates the actor and selected revisions before changing personal state.
func (c *PersonalHistoryCommander) Apply(ctx context.Context, a identity.Principal, key uuid.UUID, in PersonalHistoryCommand) (PersonalHistoryCommandReceipt, error) {
	if a.ID == uuid.Nil || a.OrgID == uuid.Nil || a.ScopeID == uuid.Nil || a.SessionID == uuid.Nil || a.CredentialEpoch < 1 {
		return PersonalHistoryCommandReceipt{}, identity.ErrForbidden
	}
	if err := in.Validate(key); err != nil {
		return PersonalHistoryCommandReceipt{}, err
	}
	if c == nil || c.store == nil {
		return PersonalHistoryCommandReceipt{}, ErrInvalidPublicQuery
	}
	return c.store.ApplyPersonalHistoryCommand(ctx, a, key, in)
}

// Validate rejects ambiguous selections before storage or cursor state is changed.
func (in PersonalHistoryCommand) Validate(key uuid.UUID) error {
	if key == uuid.Nil || len(in.Targets) < 1 || len(in.Targets) > 100 || (in.Action != "favorite" && in.Action != "delete") || (in.Action == "favorite" && in.Favorited == nil) || (in.Action == "delete" && in.Favorited != nil) {
		return ErrInvalidPublicQuery
	}
	seen := map[uuid.UUID]map[uuid.UUID]bool{}
	for _, target := range in.Targets {
		if target.OperationID == uuid.Nil || target.ExpectedRevision < 0 || (target.OutputID != nil && *target.OutputID == uuid.Nil) {
			return ErrInvalidPublicQuery
		}
		id := uuid.Nil
		if target.OutputID != nil {
			id = *target.OutputID
		}
		if seen[target.OperationID] == nil {
			seen[target.OperationID] = map[uuid.UUID]bool{}
		}
		entries := seen[target.OperationID]
		if entries[id] || entries[uuid.Nil] || (id == uuid.Nil && len(entries) > 0) {
			return ErrInvalidPublicQuery
		}
		entries[id] = true
	}
	return nil
}
