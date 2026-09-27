package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrInvalidBatch means the persisted batch facts violate its invariants.
	ErrInvalidBatch = errors.New("invalid operation batch")
	// ErrInvalidBatchStatus means a batch status is not defined.
	ErrInvalidBatchStatus = errors.New("invalid batch status")
	// ErrIllegalBatchTransition means the requested batch lifecycle change is unsafe.
	ErrIllegalBatchTransition = errors.New("illegal batch transition")
	// ErrInvalidOperation means the operation facts cannot represent a valid operation.
	ErrInvalidOperation = errors.New("invalid operation")
	// ErrInvalidOperationInput means a frozen input lacks its parent or content.
	ErrInvalidOperationInput = errors.New("invalid operation input")
	// ErrQuoteExpired means the quote has reached its expiration instant.
	ErrQuoteExpired = errors.New("quote expired")
	// ErrQuoteNotConfirmable means the operation is not in a quoted state.
	ErrQuoteNotConfirmable = errors.New("quote not confirmable")
)

// BatchStatus is the persisted lifecycle state of a batch.
type BatchStatus string

// Batch lifecycle values match operation.batch.status.
const (
	BatchStatusQuoted    BatchStatus = "quoted"
	BatchStatusConfirmed BatchStatus = "confirmed"
	BatchStatusRunning   BatchStatus = "running"
	BatchStatusFinished  BatchStatus = "finished"
	BatchStatusCancelled BatchStatus = "cancelled"
	BatchStatusExpired   BatchStatus = "expired"
)

func (s BatchStatus) valid() bool {
	switch s {
	case BatchStatusQuoted, BatchStatusConfirmed, BatchStatusRunning,
		BatchStatusFinished, BatchStatusCancelled, BatchStatusExpired:
		return true
	default:
		return false
	}
}

// CanTransitionTo checks a batch lifecycle change. An identical state is
// accepted for replay, but the caller must still use a conditional database write.
func (s BatchStatus) CanTransitionTo(next BatchStatus) error {
	if !s.valid() || !next.valid() {
		return fmt.Errorf("%w: %q -> %q", ErrInvalidBatchStatus, s, next)
	}
	if s == next {
		return nil
	}
	switch s {
	case BatchStatusQuoted:
		if next == BatchStatusConfirmed || next == BatchStatusExpired {
			return nil
		}
	case BatchStatusConfirmed:
		if next == BatchStatusRunning || next == BatchStatusFinished || next == BatchStatusCancelled {
			return nil
		}
	case BatchStatusRunning:
		if next == BatchStatusFinished || next == BatchStatusCancelled {
			return nil
		}
	}
	return fmt.Errorf("%w: %q -> %q", ErrIllegalBatchTransition, s, next)
}

// Batch groups quoted operations within one project. Counts include only
// confirmed items after user exclusions and stale quotes are removed.
type Batch struct {
	ID               uuid.UUID
	ProjectID        uuid.UUID
	Kind             string
	Scope            json.RawMessage
	Status           BatchStatus
	TotalCount       int32
	SucceededCount   int32
	FailedCount      int32
	UnknownCount     int32
	QuoteTotalMicros int64
}

// Validate checks project scope, quote amount, and bounded progress counts.
func (b Batch) Validate() error {
	if b.ID == uuid.Nil || b.ProjectID == uuid.Nil || !validBatchKind(b.Kind) ||
		!b.Status.valid() || !jsonObject(b.Scope) || b.TotalCount < 1 ||
		b.SucceededCount < 0 || b.FailedCount < 0 || b.UnknownCount < 0 ||
		b.QuoteTotalMicros < 0 ||
		int64(b.SucceededCount)+int64(b.FailedCount)+int64(b.UnknownCount) > int64(b.TotalCount) {
		return ErrInvalidBatch
	}
	return nil
}

// Operation holds the quote and frozen selection required to confirm and run
// one generation. Optional fields reflect nullable columns and nonpaid uploads.
type Operation struct {
	ID                    uuid.UUID
	ProjectID             uuid.UUID
	BatchID               *uuid.UUID
	TargetType            string
	TargetID              *uuid.UUID
	TargetKey             *string
	TargetVersionNo       *int32
	Capability            string
	Mode                  string
	ModelProfileVersionID *uuid.UUID
	PriceRuleVersionID    *uuid.UUID
	Params                json.RawMessage
	OutputCount           int32
	InputHash             string
	Origin                string
	Status                Status
	QuoteMicros           *int64
	QuoteDetail           json.RawMessage
	QuoteExpiresAt        *time.Time
	ReusedFromID          *uuid.UUID
	ForceRegenerate       bool
	ReservationID         *uuid.UUID
	ConfirmedAt           *time.Time
	SettledMicros         *int64
	Region                *string
	CreateTime            time.Time
}

// Validate checks facts that remain true without reading current target,
// model, price, project membership, or balance state from another module.
func (o Operation) Validate() error {
	if o.ID == uuid.Nil || o.ProjectID == uuid.Nil || o.CreateTime.IsZero() ||
		!o.Status.valid() || strings.TrimSpace(o.Capability) == "" ||
		strings.TrimSpace(o.Mode) == "" || strings.TrimSpace(o.InputHash) == "" ||
		!jsonObject(o.Params) || o.OutputCount < 1 || o.OutputCount > 8 ||
		!validOrigin(o.Origin) || !validTargetType(o.TargetType) ||
		!validUUID(o.BatchID) || !validUUID(o.TargetID) ||
		!validUUID(o.ModelProfileVersionID) || !validUUID(o.PriceRuleVersionID) ||
		!validUUID(o.ReusedFromID) || !validUUID(o.ReservationID) ||
		(o.TargetVersionNo != nil && *o.TargetVersionNo < 1) ||
		(o.SettledMicros != nil && *o.SettledMicros < 0) ||
		(o.QuoteMicros != nil && *o.QuoteMicros < 0) ||
		(len(o.QuoteDetail) > 0 && !jsonObject(o.QuoteDetail)) ||
		(o.Region != nil && *o.Region != "domestic" && *o.Region != "overseas") {
		return ErrInvalidOperation
	}
	if o.Origin != "upload" && (o.Region == nil ||
		(o.ModelProfileVersionID == nil && o.TargetType != "agent_session")) {
		return ErrInvalidOperation
	}
	if o.TargetType == "agent_session" &&
		(o.ModelProfileVersionID != nil || o.PriceRuleVersionID != nil) {
		return ErrInvalidOperation
	}
	if o.ReusedFromID != nil && (*o.ReusedFromID == o.ID || o.ForceRegenerate ||
		(o.QuoteMicros != nil && *o.QuoteMicros != 0)) {
		return ErrInvalidOperation
	}
	if o.Status == StatusQuoted || (o.Status != StatusDraft && o.Origin != "upload") {
		if o.QuoteMicros == nil || o.QuoteExpiresAt == nil ||
			!o.QuoteExpiresAt.After(o.CreateTime) {
			return ErrInvalidOperation
		}
		if *o.QuoteMicros > 0 && o.PriceRuleVersionID == nil && o.TargetType != "agent_session" {
			return ErrInvalidOperation
		}
	}
	return nil
}

// CanConfirmAt requires a quoted, unexpired operation. The application must
// recheck current target, model, price, authorization, and balance in the lock.
func (o Operation) CanConfirmAt(now time.Time) error {
	if err := o.Validate(); err != nil {
		return err
	}
	if o.Status != StatusQuoted || o.Origin == "upload" {
		return ErrQuoteNotConfirmable
	}
	if !now.Before(*o.QuoteExpiresAt) {
		return ErrQuoteExpired
	}
	return nil
}

// OperationInput records one ordered, immutable input of its parent operation.
// Project scope must be checked through OperationID because this row has none.
type OperationInput struct {
	ID           uuid.UUID
	OperationID  uuid.UUID
	SeqNo        int32
	Role         string
	RefType      string
	RefID        *uuid.UUID
	RefVersion   *string
	TextValue    *string
	MediaAssetID *uuid.UUID
	MaskAssetID  *uuid.UUID
}

// Validate rejects missing parent scope and empty input content.
func (i OperationInput) Validate() error {
	if i.ID == uuid.Nil || i.OperationID == uuid.Nil || i.SeqNo < 0 ||
		strings.TrimSpace(i.Role) == "" || strings.TrimSpace(i.RefType) == "" ||
		!validUUID(i.RefID) || !validUUID(i.MediaAssetID) || !validUUID(i.MaskAssetID) ||
		(i.TextValue != nil && strings.TrimSpace(*i.TextValue) == "") ||
		(i.RefVersion != nil && strings.TrimSpace(*i.RefVersion) == "") ||
		(i.RefID == nil && i.TextValue == nil && i.MediaAssetID == nil && i.MaskAssetID == nil) ||
		(i.Role == "prompt" && i.TextValue == nil && i.RefID == nil) {
		return ErrInvalidOperationInput
	}
	return nil
}

func validUUID(id *uuid.UUID) bool { return id == nil || *id != uuid.Nil }

func jsonObject(value json.RawMessage) bool {
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) < 2 || trimmed[0] != '{' {
		return false
	}
	return json.Valid(trimmed)
}

func validOrigin(value string) bool {
	switch value {
	case "pipeline", "batch", "canvas", "agent", "upload", "system":
		return true
	default:
		return false
	}
}

func validBatchKind(value string) bool {
	switch value {
	case "keyframe", "video", "tts", "reference", "parse", "storyboard", "mixed":
		return true
	default:
		return false
	}
}

func validTargetType(value string) bool {
	switch value {
	case "", "shot_frame", "shot_take", "reference_slot", "dialogue_audio",
		"voice_preview", "episode_split", "episode_parse", "bible_extract",
		"scene_storyboard", "agent_session", "free":
		return true
	default:
		return false
	}
}
