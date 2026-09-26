package audit_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
)

func TestAuditParserRejectsUnsafeSummaries(t *testing.T) {
	parser := auditapp.NewParser(map[string][]string{
		"budget.changed": {"limit_micros", "secret_key", "secretKey", "download_url", "prompt", "details"},
	})
	orgID, projectID, actorID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	tests := []struct {
		name  string
		after map[string]any
	}{
		{"credential field", map[string]any{"secret_key": "must-not-store"}},
		{"camel-case credential field", map[string]any{"secretKey": "must-not-store"}},
		{"signed URL field", map[string]any{"download_url": "https://example.invalid/file"}},
		{"signed URL value", map[string]any{"prompt": "https://example.invalid/?X-Amz-Signature=secret"}},
		{"long prompt", map[string]any{"prompt": strings.Repeat("x", 501)}},
		{"nested summary", map[string]any{"details": map[string]any{"secret": "value"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			record := auditRecord(t, uuid.NewString(), orgID, projectID, actorID, time.Now().UTC(), tc.after)
			if _, err := parser.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
				t.Fatalf("unsafe summary error = %v", err)
			}
		})
	}
	record := auditRecord(t, uuid.NewString(), orgID, projectID, actorID, time.Now().UTC(), map[string]any{"limit_micros": 100})
	if _, err := parser.Parse(record); err != nil {
		t.Fatalf("declared scalar summary: %v", err)
	}
	record.Key = []byte(uuid.NewString())
	if _, err := parser.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
		t.Fatalf("misrouted audit event error = %v", err)
	}
}
