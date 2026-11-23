package application

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

func TestEmbeddingAcceptsOnlyBoundedSingleText(t *testing.T) {
	m := domain.Model{ID: uuid.New(), Name: "project-rag", Protocol: domain.EmbeddingProtocol, Capability: "text.embed", MaxInput: 8192, MaxOutput: 1, Price: domain.Price{Input: 10, Currency: "CNY", FX: "1", Discount: "1", Multiplier: "1"}}
	raw := json.RawMessage(`{"model":"project-rag","text":"晨雾","purpose":"query"}`)
	if _, err := Prepare(m, uuid.New(), uuid.New(), uuid.New(), uuid.New(), "key", "public", raw); err == nil {
		t.Fatal("generic callers must not bypass project admission")
	}
	for _, tc := range []struct {
		payload string
		valid   bool
	}{
		{`{"model":"project-rag","text":"晨雾","purpose":"query"}`, true},
		{`{"model":"project-rag","text":"晨雾","purpose":"document"}`, true},
		{`{"model":"project-rag","text":"晨雾","purpose":"external"}`, false},
		{`{"model":"project-rag","text":"晨雾","purpose":"query","url":"https://invalid.local"}`, false},
	} {
		call, err := Prepare(m, uuid.New(), uuid.New(), uuid.New(), uuid.New(), "key", domain.EmbeddingOrigin, json.RawMessage(tc.payload))
		if (err == nil) != tc.valid || tc.valid && call.Limit < 1 {
			t.Fatalf("payload %s, limit %d, error %v", tc.payload, call.Limit, err)
		}
	}
}
