package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

// Pin errors distinguish a closed acquisition gate from invalid or conflicting evidence.
var (
	ErrInvalidReferencePin  = errors.New("invalid media reference pin")
	ErrReferencePinConflict = errors.New("media reference pin conflicts with durable facts")
	ErrReferencePinClosed   = errors.New("media reference acquisition is closed or stale")
	ErrReferencePinNotFound = errors.New("media reference pin not found")
	ErrReferencePinOwner    = errors.New("media reference pins require the Media database owner")
	ErrReferencePinProof    = errors.New("exact consumer reference proof required")
)

// ReferencePinIdentity is immutable. IDs in another domain are opaque facts,
// never foreign keys or permission to query that domain's tables.
type ReferencePinIdentity struct {
	PinID                 uuid.UUID `json:"pin_id"`
	Consumer              string    `json:"consumer"`
	ConsumerCommandID     uuid.UUID `json:"consumer_command_id"`
	ConsumerCommandSHA256 string    `json:"consumer_command_sha256"`
	ConsumerResourceID    uuid.UUID `json:"consumer_resource_id"`
	ScopeID               uuid.UUID `json:"scope_id"`
	ProjectID             uuid.UUID `json:"project_id"`
	AssetID               uuid.UUID `json:"asset_id"`
	LocationID            uuid.UUID `json:"location_id"`
	ByteSize              int64     `json:"byte_size"`
	SHA256                string    `json:"sha256"`
	AssetRevision         int64     `json:"asset_revision"`
	GateEpoch             int64     `json:"gate_epoch"`
}

// Validate requires precise, project-owned bytes and a fixed consumer domain.
func (i ReferencePinIdentity) Validate() error {
	if i.PinID == uuid.Nil || i.ConsumerCommandID == uuid.Nil || i.ConsumerResourceID == uuid.Nil || i.ScopeID == uuid.Nil || i.ProjectID == uuid.Nil || i.AssetID == uuid.Nil || i.LocationID == uuid.Nil || i.ByteSize < 0 || i.AssetRevision < 1 || i.GateEpoch < 1 || !ReferencePinDigestValid(i.SHA256) || !ReferencePinDigestValid(i.ConsumerCommandSHA256) {
		return ErrInvalidReferencePin
	}
	switch i.Consumer {
	case "identity", "creation", "execution":
		return nil
	default:
		return ErrInvalidReferencePin
	}
}

// ReferencePinDigestValid accepts only lowercase SHA-256 hex.
func ReferencePinDigestValid(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// ReferencePin retains the actual moderation facts under the asset revision.
// Physical routing keys and credentials never enter a pin or its receipts.
type ReferencePin struct {
	Identity              ReferencePinIdentity `json:"identity"`
	AcquireCommandID      uuid.UUID            `json:"acquire_command_id"`
	Kind                  string               `json:"kind,omitempty"`
	ModerationStatus      string               `json:"moderation_status"`
	ModerationDetail      json.RawMessage      `json:"moderation_detail"`
	AIGCMarked            bool                 `json:"aigc_marked"`
	ContainsRealPerson    bool                 `json:"contains_real_person"`
	ConsentRecordID       *uuid.UUID           `json:"consent_record_id"`
	State                 string               `json:"state"`
	AdoptionReceiptSHA256 string               `json:"adoption_receipt_sha256,omitempty"`
	ReleaseReceiptSHA256  string               `json:"release_receipt_sha256,omitempty"`
	ReleaseOutcome        string               `json:"release_outcome,omitempty"`
}

// SnapshotSHA256 binds proof to all immutable facts, excluding later lifecycle state.
func (p ReferencePin) SnapshotSHA256() string {
	p.State = ""
	p.AdoptionReceiptSHA256 = ""
	p.ReleaseReceiptSHA256 = ""
	p.ReleaseOutcome = ""
	body, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	// JSONB may reorder nested review detail keys and whitespace after storage.
	// Preserve numbers exactly while canonicalizing that immutable JSON evidence.
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return ""
	}
	body, err = json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// ReferencePinProof is returned by an authenticated consumer adapter after it
// checks the original command and exact reference. A caller-supplied status,
// missing record, timeout or expiry is never such proof.
type ReferencePinProof struct {
	Identity         ReferencePinIdentity
	AcquireCommandID uuid.UUID
	SnapshotSHA256   string
	Outcome          string
	Receipt          json.RawMessage
}

// ReferenceAcquisition records the permanent closure of one asset's new pins.
type ReferenceAcquisition struct {
	AssetID        uuid.UUID  `json:"asset_id"`
	Epoch          int64      `json:"epoch"`
	Closed         bool       `json:"closed"`
	CloseCommandID *uuid.UUID `json:"close_command_id"`
}
