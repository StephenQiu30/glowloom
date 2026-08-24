package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

const envelopeVersionV2 = 2

var (
	uuidPattern = regexp.MustCompile(
		`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`,
	)
	eventTypePattern = regexp.MustCompile(
		`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+\.v[1-9][0-9]*$`,
	)
	traceparentPattern = regexp.MustCompile(
		`^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`,
	)
)

type EnvelopeV2 struct {
	EnvelopeVersion  int             `json:"envelope_version"`
	EventID          string          `json:"event_id"`
	EventType        string          `json:"event_type"`
	SchemaVersion    int             `json:"schema_version"`
	AggregateType    string          `json:"aggregate_type"`
	AggregateID      string          `json:"aggregate_id"`
	AggregateVersion int             `json:"aggregate_version"`
	WorkspaceID      string          `json:"workspace_id"`
	PartitionKey     string          `json:"partition_key"`
	OccurredAt       time.Time       `json:"occurred_at"`
	Traceparent      string          `json:"traceparent"`
	CorrelationID    string          `json:"correlation_id"`
	CausationID      *string         `json:"causation_id"`
	Payload          json.RawMessage `json:"payload"`
}

func DecodeEnvelopeV2(content []byte) (EnvelopeV2, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()

	var envelope EnvelopeV2
	if err := decoder.Decode(&envelope); err != nil {
		return EnvelopeV2{}, fmt.Errorf("decode event envelope v2: %w", err)
	}
	if err := ensureJSONDocumentEnded(decoder); err != nil {
		return EnvelopeV2{}, err
	}
	if err := envelope.Validate(); err != nil {
		return EnvelopeV2{}, err
	}
	return envelope, nil
}

func (envelope EnvelopeV2) Validate() error {
	if envelope.EnvelopeVersion != envelopeVersionV2 {
		return errors.New("envelope_version must equal 2")
	}
	for name, value := range map[string]string{
		"event_id":       envelope.EventID,
		"aggregate_id":   envelope.AggregateID,
		"workspace_id":   envelope.WorkspaceID,
		"correlation_id": envelope.CorrelationID,
	} {
		if !uuidPattern.MatchString(value) {
			return fmt.Errorf("%s must be a UUID", name)
		}
	}
	if envelope.CausationID != nil && !uuidPattern.MatchString(*envelope.CausationID) {
		return errors.New("causation_id must be null or a UUID")
	}
	if !eventTypePattern.MatchString(envelope.EventType) || len(envelope.EventType) > 100 {
		return errors.New("event_type must use the versioned domain event format")
	}
	if envelope.SchemaVersion < 1 {
		return errors.New("schema_version must be positive")
	}
	if strings.TrimSpace(envelope.AggregateType) == "" || len(envelope.AggregateType) > 100 {
		return errors.New("aggregate_type is required")
	}
	if envelope.AggregateVersion < 1 {
		return errors.New("aggregate_version must be positive")
	}
	if strings.TrimSpace(envelope.PartitionKey) == "" || len(envelope.PartitionKey) > 200 {
		return errors.New("partition_key is required")
	}
	if envelope.OccurredAt.IsZero() {
		return errors.New("occurred_at is required")
	}
	if !validTraceparent(envelope.Traceparent) {
		return errors.New("traceparent is invalid")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil || payload == nil {
		return errors.New("payload must be a JSON object")
	}
	return nil
}

func ensureJSONDocumentEnded(decoder *json.Decoder) error {
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("event envelope must contain one JSON document")
		}
		return fmt.Errorf("decode trailing event envelope data: %w", err)
	}
	return nil
}

func validTraceparent(value string) bool {
	if !traceparentPattern.MatchString(value) {
		return false
	}
	parts := strings.Split(value, "-")
	return parts[1] != strings.Repeat("0", 32) && parts[2] != strings.Repeat("0", 16)
}
