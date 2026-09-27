package audit_test

import (
	"encoding/json"
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

func TestIdentityAuditActionPolicy(t *testing.T) {
	parser := auditapp.NewIdentityActionParser()
	orgID, actorID := uuid.NewString(), uuid.NewString()
	tests := []struct {
		name   string
		action string
		after  map[string]any
		valid  bool
	}{
		{"login without summary", "auth.login_succeeded", nil, true},
		{"login with summary", "auth.login_succeeded", map[string]any{"reason": "accepted"}, false},
		{"unknown login reason", "auth.login_failed", map[string]any{"reason": "login_not_found"}, true},
		{"wrong password reason", "auth.login_failed", map[string]any{"reason": "password_mismatch"}, true},
		{"disabled account reason", "auth.login_failed", map[string]any{"reason": "account_disabled"}, true},
		{"IP limit reason", "auth.login_failed", map[string]any{"reason": "ip_rate_limited"}, true},
		{"generic public error is not an audit reason", "auth.login_failed", map[string]any{"reason": "invalid_credentials"}, false},
		{"failed login free text", "auth.login_failed", map[string]any{"reason": "password was secret123"}, false},
		{"account status", "user.disabled", map[string]any{"status": "disabled"}, true},
		{"account password flag", "user.password_reset", map[string]any{"must_change_password": true}, true},
		{"account password flag as text", "user.password_reset", map[string]any{"must_change_password": "true"}, false},
		{"account password flag as null", "user.password_reset", map[string]any{"must_change_password": nil}, false},
		{"password hash", "user.password_reset", map[string]any{"password_hash": "secret"}, false},
		{"unregistered action", "budget.changed", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			record := auditRecord(t, uuid.NewString(), orgID, "", actorID, time.Now().UTC(), nil)
			var envelope map[string]any
			if err := json.Unmarshal(record.Value, &envelope); err != nil {
				t.Fatal(err)
			}
			data := envelope["data"].(map[string]any)
			data["action"] = tc.action
			data["object"] = map[string]any{"type": "user", "id": actorID}
			delete(data, "before")
			if tc.after == nil {
				delete(data, "after")
			} else {
				data["after"] = tc.after
			}
			var err error
			record.Value, err = json.Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			_, err = parser.Parse(record)
			if tc.valid && err != nil {
				t.Fatalf("valid identity audit event: %v", err)
			}
			if !tc.valid && !errors.Is(err, auditapp.ErrInvalidEvent) {
				t.Fatalf("invalid identity audit event error = %v", err)
			}
		})
	}
}
