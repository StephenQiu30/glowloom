package kafka

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

func TestRetryDelayIsExponentialAndBounded(t *testing.T) {
	policy := FailurePolicy{RetryBase: 250 * time.Millisecond, RetryCap: 5 * time.Second}
	want := []time.Duration{
		250 * time.Millisecond,
		500 * time.Millisecond,
		time.Second,
		2 * time.Second,
		4 * time.Second,
		5 * time.Second,
		5 * time.Second,
	}
	for attempt, expected := range want {
		if actual := retryDelay(policy, attempt+1); actual != expected {
			t.Errorf("retryDelay(%d) = %s, want %s", attempt+1, actual, expected)
		}
	}
}

func TestEncodeDeadLetterKeepsStableSourceIdentityAndPayload(t *testing.T) {
	record := &kgo.Record{
		Topic: "glowloom.billing.command.v1", Partition: 3, Offset: 17,
		Key: []byte("scope-123"), Value: []byte(`{"command_id":"cmd-123"}`),
	}
	deadLetter, err := encodeDeadLetter(record, 12, "handler_failed")
	if err != nil {
		t.Fatalf("encode dead-letter event: %v", err)
	}
	if deadLetter.FailureID != "glowloom.billing.command.v1:3:17" || deadLetter.Attempts != 12 || deadLetter.ErrorClass != "handler_failed" {
		t.Fatalf("dead-letter identity or retry facts = %+v", deadLetter)
	}
	payload, err := deadLetter.Payload()
	if err != nil {
		t.Fatalf("marshal dead-letter event: %v", err)
	}
	var decoded deadLetterEnvelope
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode dead-letter payload: %v", err)
	}
	if string(mustDecodeBase64(t, decoded.KeyBase64)) != string(record.Key) || string(mustDecodeBase64(t, decoded.ValueBase64)) != string(record.Value) {
		t.Fatalf("dead-letter payload did not preserve the original record: %+v", decoded)
	}
}

func mustDecodeBase64(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("decode base64 payload: %v", err)
	}
	return decoded
}
