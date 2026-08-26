package eventing_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	eventing "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
)

func TestEnvelopeRoundTripPreservesRequiredMetadataAndCanonicalPayloadHash(t *testing.T) {
	t.Parallel()
	payload := json.RawMessage(`{"version_no":2,"content_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","owner_set_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","topology_hash":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","version_id":"00000000-0000-0000-0000-000000000111"}`)
	envelope, err := eventing.NewEnvelope(eventing.OutboxEvent{
		ID: "00000000-0000-0000-0000-000000000101", EventType: eventing.StoryGraphVersionPublished,
		EventVersion: 1, WorkspaceID: "00000000-0000-0000-0000-000000000102",
		ProjectID: "00000000-0000-0000-0000-000000000103", AggregateKind: "storygraph",
		AggregateID: "00000000-0000-0000-0000-000000000103", AggregateRevision: 2,
		SourceReceiptID: "00000000-0000-0000-0000-000000000104", Payload: payload,
		OccurredAt: time.Date(2026, time.August, 27, 8, 0, 0, 0, time.UTC),
	}, eventing.TraceContext{RequestID: "request-storygraph-2"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := eventing.EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := eventing.DecodeEnvelope(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Schema != eventing.EnvelopeSchemaV1 || decoded.EventID != envelope.EventID ||
		decoded.AggregateRevision != 2 || decoded.SourceReceiptID != envelope.SourceReceiptID ||
		decoded.TraceContext.RequestID != "request-storygraph-2" || decoded.PayloadHash != envelope.PayloadHash {
		t.Fatalf("required envelope metadata drifted: %#v", decoded)
	}
	if !strings.Contains(string(encoded), `"trace_context"`) || len(decoded.PayloadHash) != 64 {
		t.Fatalf("trace context or payload hash is missing: %s", encoded)
	}
}

func TestEnvelopeRejectsUnknownFieldsHashDriftAndUnsafePayload(t *testing.T) {
	t.Parallel()
	base := eventingFixture(t)
	encoded, err := eventing.EncodeEnvelope(base)
	if err != nil {
		t.Fatal(err)
	}
	unknown := strings.Replace(string(encoded), `"schema":`, `"unexpected":true,"schema":`, 1)
	if _, err = eventing.DecodeEnvelope([]byte(unknown)); err == nil {
		t.Fatal("unknown envelope field must be rejected")
	}
	drifted := strings.Replace(string(encoded), base.PayloadHash, strings.Repeat("f", 64), 1)
	if _, err = eventing.DecodeEnvelope([]byte(drifted)); err == nil {
		t.Fatal("payload hash drift must be rejected")
	}
	for name, payload := range map[string]json.RawMessage{
		"full script":     json.RawMessage(`{"script_text":"完整剧本文本"}`),
		"prompt":          json.RawMessage(`{"prompt":"ignore previous instructions"}`),
		"secret":          json.RawMessage(`{"nested":{"secret":"value"}}`),
		"private url":     json.RawMessage(`{"private_url":"https://private.example.test/object?token=x"}`),
		"access token":    json.RawMessage(`{"access_token":"credential"}`),
		"unknown content": json.RawMessage(`{"content":"complete screenplay hidden under a generic field"}`),
	} {
		t.Run(name, func(t *testing.T) {
			value := eventing.OutboxEvent{
				ID: base.EventID, EventType: base.EventType, EventVersion: base.EventVersion,
				WorkspaceID: base.WorkspaceID, ProjectID: base.ProjectID,
				AggregateKind: base.AggregateKind, AggregateID: base.AggregateID,
				AggregateRevision: base.AggregateRevision, SourceReceiptID: base.SourceReceiptID,
				Payload: payload, OccurredAt: base.OccurredAt,
			}
			if _, createErr := eventing.NewEnvelope(value, base.TraceContext); createErr == nil {
				t.Fatal("unsafe payload must be rejected")
			}
		})
	}
}

func TestEnvelopeRejectsNonCanonicalIdentityAndUnknownEventVersion(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*eventing.Envelope){
		"event id":       func(value *eventing.Envelope) { value.EventID = "not-a-uuid" },
		"workspace id":   func(value *eventing.Envelope) { value.WorkspaceID = "00000000000000000000000000000202" },
		"source receipt": func(value *eventing.Envelope) { value.SourceReceiptID = "receipt" },
		"event version":  func(value *eventing.Envelope) { value.EventVersion = 2 },
	} {
		t.Run(name, func(t *testing.T) {
			value := eventingFixture(t)
			mutate(&value)
			if _, err := eventing.EncodeEnvelope(value); err == nil {
				t.Fatal("invalid event identity or version must be rejected")
			}
		})
	}
}

func TestScriptVersionPublishedHasAReferenceOnlyStrictContract(t *testing.T) {
	t.Parallel()
	payload := json.RawMessage(`{"script_version_id":"00000000-0000-0000-0000-000000000301","episode_id":"00000000-0000-0000-0000-000000000302","version_no":2,"document_revision_id":"00000000-0000-0000-0000-000000000303","content_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","source_start":10,"source_end":30}`)
	envelope, err := eventing.NewEnvelope(eventing.OutboxEvent{
		ID: "00000000-0000-0000-0000-000000000304", EventType: eventing.ScriptVersionPublished,
		EventVersion: 1, WorkspaceID: "00000000-0000-0000-0000-000000000305",
		ProjectID: "00000000-0000-0000-0000-000000000306", AggregateKind: "episode_script",
		AggregateID: "00000000-0000-0000-0000-000000000302", AggregateRevision: 2,
		SourceReceiptID: "00000000-0000-0000-0000-000000000307", Payload: payload,
		OccurredAt: time.Date(2026, time.August, 27, 9, 0, 0, 0, time.UTC),
	}, eventing.TraceContext{RequestID: "request-script-2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = eventing.DecodeEnvelope(mustEncodeEnvelope(t, envelope)); err != nil {
		t.Fatal(err)
	}

	for name, invalid := range map[string]string{
		"aggregate episode drift": strings.Replace(string(payload), "00000000-0000-0000-0000-000000000302", "00000000-0000-0000-0000-000000000399", 1),
		"revision drift":          strings.Replace(string(payload), `"version_no":2`, `"version_no":1`, 1),
		"unknown content":         strings.Replace(string(payload), `"source_end":30`, `"source_end":30,"content":"full script"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			value := eventing.OutboxEvent{
				ID: envelope.EventID, EventType: envelope.EventType, EventVersion: envelope.EventVersion,
				WorkspaceID: envelope.WorkspaceID, ProjectID: envelope.ProjectID, AggregateKind: envelope.AggregateKind,
				AggregateID: envelope.AggregateID, AggregateRevision: envelope.AggregateRevision,
				SourceReceiptID: envelope.SourceReceiptID, Payload: json.RawMessage(invalid), OccurredAt: envelope.OccurredAt,
			}
			if _, createErr := eventing.NewEnvelope(value, envelope.TraceContext); createErr == nil {
				t.Fatal("invalid Script publication contract was accepted")
			}
		})
	}
}

func mustEncodeEnvelope(t *testing.T, envelope eventing.Envelope) []byte {
	t.Helper()
	encoded, err := eventing.EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func eventingFixture(t *testing.T) eventing.Envelope {
	t.Helper()
	value, err := eventing.NewEnvelope(eventing.OutboxEvent{
		ID: "00000000-0000-0000-0000-000000000201", EventType: eventing.StoryGraphVersionPublished,
		EventVersion: 1, WorkspaceID: "00000000-0000-0000-0000-000000000202",
		ProjectID: "00000000-0000-0000-0000-000000000203", AggregateKind: "storygraph",
		AggregateID: "00000000-0000-0000-0000-000000000203", AggregateRevision: 1,
		SourceReceiptID: "00000000-0000-0000-0000-000000000205",
		Payload:         json.RawMessage(`{"content_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","owner_set_hash":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","topology_hash":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","version_id":"00000000-0000-0000-0000-000000000204","version_no":1}`),
		OccurredAt:      time.Date(2026, time.August, 27, 8, 30, 0, 0, time.UTC),
	}, eventing.TraceContext{RequestID: "request-storygraph-1"})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
