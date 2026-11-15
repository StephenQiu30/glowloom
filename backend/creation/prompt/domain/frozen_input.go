package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// Errors distinguish malformed contracts, conflicting identity and permanent closure.
var (
	ErrFrozenInputInvalid  = errors.New("invalid frozen input contract")
	ErrFrozenInputConflict = errors.New("frozen input identity conflict")
	ErrFrozenInputBlocked  = errors.New("frozen input adoption permanently blocked")
	ErrFrozenInputNotFound = errors.New("frozen input not found")
	ErrFrozenInputOwner    = errors.New("frozen input requires its actual owner role")
	ErrFrozenInputReserved = errors.New("frozen input already has a reserved execution")
)

// FrozenPin carries only stable Media facts, never an object key, URL or bucket.
// The application obtains these facts from its authenticated Media consumer port.
type FrozenPin struct {
	ReferenceID      string    `json:"reference_id"`
	PinID            uuid.UUID `json:"pin_id"`
	AcquireCommandID uuid.UUID `json:"acquire_command_id"`
	AssetID          uuid.UUID `json:"asset_id"`
	AssetRevision    int64     `json:"asset_revision"`
	Kind             string    `json:"kind"`
	SnapshotSHA256   string    `json:"snapshot_sha256"`
}

// FrozenInput is Creation's immutable, closed input handoff. Source generation,
// source revision and authority epochs are deliberately independent values.
// Worker attempt/lease epochs belong exclusively to the existing task ledger.
type FrozenInput struct {
	HandoffID         uuid.UUID                `json:"handoff_id"`
	RequestID         uuid.UUID                `json:"request_id"`
	Command           writefence.Command       `json:"command"`
	SourceCommand     writefence.Command       `json:"source_command"`
	ActorID           uuid.UUID                `json:"actor_id"`
	SessionID         uuid.UUID                `json:"session_id"`
	ScopeID           uuid.UUID                `json:"scope_id"`
	ProjectID         uuid.UUID                `json:"project_id"`
	OrgID             uuid.UUID                `json:"org_id"`
	AspectRatio       string                   `json:"aspect_ratio"`
	Source            InputSource              `json:"source"`
	SourceGeneration  int64                    `json:"source_generation"`
	DraftRevision     int64                    `json:"draft_revision"`
	ScopeRevision     int64                    `json:"scope_revision"`
	ProjectRevision   int64                    `json:"project_revision"`
	TeamEpoch         int64                    `json:"team_epoch"`
	MemberEpoch       int64                    `json:"member_epoch"`
	DraftHash         string                   `json:"draft_hash"`
	InputHash         string                   `json:"input_hash"`
	Draft             Draft                    `json:"draft"`
	Pins              []FrozenPin              `json:"pins"`
	CompilationInputs *FrozenCompilationInputs `json:"compilation_inputs,omitempty"`
}

// Validate checks the full line contract, including exact identity authorities and
// one stable pin for each supported physical reference. Subject expansion must be
// supplied by its Creation owner before it can enter this handoff contract.
func (f FrozenInput) Validate() error {
	if f.HandoffID == uuid.Nil || f.RequestID == uuid.Nil || f.ActorID == uuid.Nil || f.SessionID == uuid.Nil || f.ScopeID == uuid.Nil || f.ProjectID == uuid.Nil || f.OrgID == uuid.Nil || (f.AspectRatio != "9:16" && f.AspectRatio != "16:9" && f.AspectRatio != "1:1") || !f.Source.Valid() || f.SourceGeneration < 1 || f.DraftRevision < 1 || f.ScopeRevision < 1 || f.ProjectRevision < 1 || f.TeamEpoch < 0 || f.MemberEpoch < 0 || (f.TeamEpoch == 0) != (f.MemberEpoch == 0) || f.Pins == nil {
		return ErrFrozenInputInvalid
	}
	if f.Source.Kind == "prompt_draft" && f.DraftRevision != f.Source.Revision || f.Source.Kind == "canvas_input" && f.DraftRevision != f.Source.InputRevision {
		return ErrFrozenInputInvalid
	}
	if f.Command.Action != "execution.frozen_input.accept" || f.SourceCommand.Action != "creation.frozen_input.freeze" || f.SourceCommand.ID != uuid.NewSHA1(f.HandoffID, []byte("creation.frozen_input.freeze")) || f.SourceCommand.ID == f.Command.ID || ValidateFrozenAuthorities(f.SourceCommand, f.ActorID, f.SessionID, f.ScopeID, f.ProjectID) != nil || ValidateFrozenAuthorities(f.Command, f.ActorID, f.SessionID, f.ScopeID, f.ProjectID) != nil || f.Draft.Validate() != nil {
		return ErrFrozenInputInvalid
	}
	sourceAuthorities, _ := f.SourceCommand.AuthorityDigest()
	targetAuthorities, _ := f.Command.AuthorityDigest()
	if sourceAuthorities != targetAuthorities {
		return ErrFrozenInputInvalid
	}
	if f.CompilationInputs != nil && f.CompilationInputs.Validate() != nil {
		return ErrFrozenInputInvalid
	}
	dh, ih := f.Draft.Hashes()
	if f.DraftHash != dh || f.InputHash != ih || len(f.Pins) != len(f.Draft.References) {
		return ErrFrozenInputInvalid
	}
	seen := map[uuid.UUID]bool{}
	for i, ref := range f.Draft.References {
		pin := f.Pins[i]
		if ref.Source.Kind != "media" || ref.Source.SourceID != pin.AssetID.String() || ref.Source.SourceVersion != strconv.FormatInt(pin.AssetRevision, 10) || ref.MediaKind != pin.Kind || ref.ID != pin.ReferenceID || pin.PinID == uuid.Nil || pin.AcquireCommandID == uuid.Nil || pin.AssetID == uuid.Nil || seen[pin.PinID] || !validFrozenHash(pin.SnapshotSHA256) {
			return ErrFrozenInputInvalid
		}
		seen[pin.PinID] = true
	}
	raw, err := json.Marshal(f)
	if err != nil || len(raw) > 768<<10 {
		return ErrFrozenInputInvalid
	}
	return nil
}

// ValidateFrozenAuthorities forbids extra, substituted and duplicate authorities.
func ValidateFrozenAuthorities(c writefence.Command, actor, session, scope, project uuid.UUID) error {
	if _, err := c.AuthorityDigest(); err != nil || len(c.Authorities) != 4 {
		return ErrFrozenInputInvalid
	}
	expected := map[string]uuid.UUID{"identity/account": actor, "identity/session": session, "creation/scope": scope, "creation/project": project}
	for _, a := range c.Authorities {
		if expected[string(a.Issuer)+"/"+string(a.Kind)] != a.ID {
			return ErrFrozenInputInvalid
		}
	}
	return nil
}

func validFrozenHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	b, err := hex.DecodeString(s)
	return err == nil && hex.EncodeToString(b) == s
}

// Snapshot returns the same bytes before and after a JSONB round trip. Draft's
// supported scalar parameter representation is already canonicalized by Hashes.
func (f FrozenInput) Snapshot() (json.RawMessage, string, error) {
	if err := f.Validate(); err != nil {
		return nil, "", err
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return nil, "", ErrFrozenInputInvalid
	}
	canonical, err := writefence.CanonicalReceipt(raw)
	if err != nil || len(canonical) > 768<<10 {
		return nil, "", ErrFrozenInputInvalid
	}
	hash := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(hash[:]), nil
}
