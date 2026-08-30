package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	CommittedEnvelopeSchema    = "lanverse.event.committed"
	ScriptVersionPublished     = "ScriptVersionPublished"
	StoryGraphVersionPublished = "StoryGraphVersionPublished"
	maximumEnvelopeBytes       = 64 << 10
	maximumPayloadNestingDepth = 16
)

var lowercaseHexHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

type TraceContext struct {
	RequestID   string `json:"request_id"`
	TraceParent string `json:"traceparent,omitempty"`
	TraceState  string `json:"tracestate,omitempty"`
}

type OutboxEvent struct {
	ID                string
	EventType         string
	EventVersion      int
	WorkspaceID       string
	ProjectID         string
	AggregateKind     string
	AggregateID       string
	AggregateRevision int64
	SourceReceiptID   string
	Payload           json.RawMessage
	PayloadHash       string
	OccurredAt        time.Time
}

type Envelope struct {
	Schema            string          `json:"schema"`
	EventID           string          `json:"event_id"`
	EventType         string          `json:"event_type"`
	EventVersion      int             `json:"event_version"`
	OccurredAt        time.Time       `json:"occurred_at"`
	WorkspaceID       string          `json:"workspace_id"`
	ProjectID         string          `json:"project_id"`
	AggregateKind     string          `json:"aggregate_kind"`
	AggregateID       string          `json:"aggregate_id"`
	AggregateRevision int64           `json:"aggregate_revision"`
	SourceReceiptID   string          `json:"source_receipt_id"`
	TraceContext      TraceContext    `json:"trace_context"`
	Payload           json.RawMessage `json:"payload"`
	PayloadHash       string          `json:"payload_hash"`
}

func NewEnvelope(event OutboxEvent, trace TraceContext) (Envelope, error) {
	payloadHash, err := HashPayload(event.Payload)
	if err != nil {
		return Envelope{}, err
	}
	if event.PayloadHash != "" && event.PayloadHash != payloadHash {
		return Envelope{}, errors.New("outbox payload hash does not match payload")
	}
	value := Envelope{
		Schema: CommittedEnvelopeSchema, EventID: strings.TrimSpace(event.ID),
		EventType: strings.TrimSpace(event.EventType), EventVersion: event.EventVersion,
		OccurredAt: event.OccurredAt.UTC(), WorkspaceID: strings.TrimSpace(event.WorkspaceID),
		ProjectID: strings.TrimSpace(event.ProjectID), AggregateKind: strings.TrimSpace(event.AggregateKind),
		AggregateID: strings.TrimSpace(event.AggregateID), AggregateRevision: event.AggregateRevision,
		SourceReceiptID: strings.TrimSpace(event.SourceReceiptID), TraceContext: trace,
		Payload: append(json.RawMessage(nil), event.Payload...), PayloadHash: payloadHash,
	}
	if err = value.Validate(); err != nil {
		return Envelope{}, err
	}
	return value, nil
}

func EncodeEnvelope(value Envelope) ([]byte, error) {
	if err := value.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode event envelope: %w", err)
	}
	if len(encoded) > maximumEnvelopeBytes {
		return nil, errors.New("event envelope exceeds size limit")
	}
	return encoded, nil
}

func DecodeEnvelope(encoded []byte) (Envelope, error) {
	if len(encoded) == 0 || len(encoded) > maximumEnvelopeBytes {
		return Envelope{}, errors.New("event envelope size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var value Envelope
	if err := decoder.Decode(&value); err != nil {
		return Envelope{}, fmt.Errorf("decode event envelope: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return Envelope{}, err
	}
	if err := value.Validate(); err != nil {
		return Envelope{}, err
	}
	return value, nil
}

func (value Envelope) Validate() error {
	if value.Schema != CommittedEnvelopeSchema || value.EventID == "" || value.EventType == "" || value.EventVersion < 1 ||
		value.OccurredAt.IsZero() || value.WorkspaceID == "" || value.ProjectID == "" || value.AggregateKind == "" ||
		value.AggregateID == "" || value.AggregateRevision < 1 || value.SourceReceiptID == "" ||
		strings.TrimSpace(value.TraceContext.RequestID) == "" || !lowercaseHexHash.MatchString(value.PayloadHash) {
		return errors.New("event envelope metadata is invalid")
	}
	if !canonicalUUID(value.EventID) || !canonicalUUID(value.WorkspaceID) || !canonicalUUID(value.ProjectID) ||
		!canonicalUUID(value.SourceReceiptID) {
		return errors.New("event envelope identity is invalid")
	}
	payloadHash, err := HashPayload(value.Payload)
	if err != nil {
		return err
	}
	if payloadHash != value.PayloadHash {
		return errors.New("event envelope payload hash does not match payload")
	}
	if err = validateEventPayload(value); err != nil {
		return err
	}
	return nil
}

func validateEventPayload(envelope Envelope) error {
	switch envelope.EventType {
	case ScriptVersionPublished:
		var value struct {
			ScriptVersionID    string `json:"script_version_id"`
			EpisodeID          string `json:"episode_id"`
			VersionNo          int64  `json:"version_no"`
			DocumentRevisionID string `json:"document_revision_id"`
			ContentHash        string `json:"content_hash"`
			SourceStart        int    `json:"source_start"`
			SourceEnd          int    `json:"source_end"`
		}
		decoder := json.NewDecoder(bytes.NewReader(envelope.Payload))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("decode Script event payload: %w", err)
		}
		if err := requireJSONEnd(decoder); err != nil {
			return err
		}
		if envelope.EventVersion != 1 || envelope.AggregateKind != "episode_script" || envelope.AggregateID != value.EpisodeID ||
			!canonicalUUID(value.ScriptVersionID) || !canonicalUUID(value.EpisodeID) ||
			!canonicalUUID(value.DocumentRevisionID) || value.VersionNo != envelope.AggregateRevision ||
			!lowercaseHexHash.MatchString(value.ContentHash) || value.SourceStart < 0 || value.SourceEnd <= value.SourceStart {
			return errors.New("Script event payload is incomplete")
		}
		return nil
	case StoryGraphVersionPublished:
		var value struct {
			VersionID       string  `json:"version_id"`
			VersionNo       int64   `json:"version_no"`
			ParentVersionID *string `json:"parent_version_id,omitempty"`
			OwnerSetHash    string  `json:"owner_set_hash"`
			TopologyHash    string  `json:"topology_hash"`
			ContentHash     string  `json:"content_hash"`
		}
		decoder := json.NewDecoder(bytes.NewReader(envelope.Payload))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("decode StoryGraph event payload: %w", err)
		}
		if err := requireJSONEnd(decoder); err != nil {
			return err
		}
		if envelope.EventVersion != 1 || envelope.AggregateKind != "storygraph" || envelope.AggregateID != envelope.ProjectID ||
			!canonicalUUID(value.VersionID) || value.VersionNo != envelope.AggregateRevision ||
			!lowercaseHexHash.MatchString(value.OwnerSetHash) ||
			!lowercaseHexHash.MatchString(value.TopologyHash) || !lowercaseHexHash.MatchString(value.ContentHash) {
			return errors.New("StoryGraph event payload is incomplete")
		}
		if value.ParentVersionID != nil && !canonicalUUID(*value.ParentVersionID) {
			return errors.New("StoryGraph parent version id is invalid")
		}
		return nil
	default:
		return fmt.Errorf("event type %q has no registered payload contract", envelope.EventType)
	}
}

func canonicalUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}

func HashPayload(payload json.RawMessage) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("decode event payload: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return "", err
	}
	if _, ok := value.(map[string]any); !ok {
		return "", errors.New("event payload must be a JSON object")
	}
	if err := validateSafePayload(value, 0); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("canonicalize event payload: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("event JSON must contain exactly one value")
	}
	return nil
}

func validateSafePayload(value any, depth int) error {
	if depth > maximumPayloadNestingDepth {
		return errors.New("event payload nesting exceeds limit")
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if unsafePayloadKey(key) {
				return fmt.Errorf("event payload field %q is not allowed", key)
			}
			if err := validateSafePayload(child, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := validateSafePayload(child, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func unsafePayloadKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, forbidden := range []string{
		"script", "script_text", "full_script", "prompt", "system_prompt", "user_prompt",
		"secret", "credential", "password", "token", "access_token", "refresh_token",
		"url", "private_url", "presigned_url",
	} {
		if key == forbidden {
			return true
		}
	}
	return false
}
