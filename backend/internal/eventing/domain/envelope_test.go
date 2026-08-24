package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

const validEnvelope = `{
  "envelope_version": 2,
  "event_id": "018f67c0-3f55-7d7b-8000-111111111111",
  "event_type": "production.workspace.created.v1",
  "schema_version": 1,
  "aggregate_type": "workspace",
  "aggregate_id": "018f67c0-3f55-7d7b-8000-222222222222",
  "aggregate_version": 1,
  "workspace_id": "018f67c0-3f55-7d7b-8000-222222222222",
  "partition_key": "workspace:018f67c0-3f55-7d7b-8000-222222222222",
  "occurred_at": "2026-08-24T00:00:00Z",
  "traceparent": "00-11111111111111111111111111111111-2222222222222222-01",
  "correlation_id": "018f67c0-3f55-7d7b-8000-333333333333",
  "causation_id": null,
  "payload": {
    "workspace_name": "制作空间",
    "revision": 1,
    "enabled": true
  }
}`

func TestDecodeEnvelopeV2PreservesJSONPayloadTypes(t *testing.T) {
	envelope, err := DecodeEnvelopeV2([]byte(validEnvelope))
	if err != nil {
		t.Fatal(err)
	}
	if envelope.EnvelopeVersion != 2 || envelope.AggregateVersion != 1 {
		t.Fatalf("version fields = %#v", envelope)
	}
	var payload map[string]any
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["workspace_name"] != "制作空间" || payload["enabled"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	if payload["revision"] != float64(1) {
		t.Fatalf("numeric payload type was not preserved: %#v", payload)
	}
}

func TestDecodeEnvelopeV2RejectsUnknownOrLegacyFields(t *testing.T) {
	unknown := strings.Replace(validEnvelope, `"payload": {`, `"trace_id": "legacy", "payload": {`, 1)
	if _, err := DecodeEnvelopeV2([]byte(unknown)); err == nil {
		t.Fatal("v2 decoder accepted an unknown legacy field")
	}

	legacy := strings.Replace(validEnvelope, `"envelope_version": 2`, `"envelope_version": 1`, 1)
	if _, err := DecodeEnvelopeV2([]byte(legacy)); err == nil {
		t.Fatal("v2 decoder accepted an envelope v1 message")
	}
}

func TestDecodeEnvelopeV2RejectsInvalidRequiredContext(t *testing.T) {
	invalidTrace := strings.Replace(
		validEnvelope,
		"00-11111111111111111111111111111111-2222222222222222-01",
		"invalid",
		1,
	)
	if _, err := DecodeEnvelopeV2([]byte(invalidTrace)); err == nil {
		t.Fatal("v2 decoder accepted an invalid traceparent")
	}

	missingPartition := strings.Replace(
		validEnvelope,
		`  "partition_key": "workspace:018f67c0-3f55-7d7b-8000-222222222222",`+"\n",
		"",
		1,
	)
	if _, err := DecodeEnvelopeV2([]byte(missingPartition)); err == nil {
		t.Fatal("v2 decoder accepted a missing partition key")
	}
}
