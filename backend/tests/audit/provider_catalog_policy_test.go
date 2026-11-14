package audit_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/glowloom-server/backend/internal/audit/application"
	inbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
)

func providerCatalogRecord(t *testing.T, action string, target uuid.UUID) inbox.Record {
	t.Helper()
	eventID, orgID, actorID := uuid.New(), uuid.New(), uuid.New()
	body := map[string]any{"event_id": eventID, "event_type": "glowloom.audit.recorded.v1", "occurred_at": time.Now().UTC(), "org_id": orgID,
		"actor": map[string]any{"kind": "user", "id": actorID}, "aggregate": map[string]any{"type": "audit", "id": eventID},
		"data": map[string]any{"action": action, "object": map[string]any{"type": "catalog", "id": target}, "request_id": uuid.New()}}
	value, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return inbox.Record{Topic: "glowloom.audit.recorded.v1", Key: []byte(orgID.String()), Value: value}
}

func TestProviderCatalogAuditRecordedEnvelopes(t *testing.T) {
	parser := auditapp.NewRecordedActionParser()
	for _, tc := range []struct {
		name, action string
		target       uuid.UUID
	}{
		{"existing custom receipt target", "provider.custom.connect", uuid.Nil},
		{"new custom provider target", "provider.custom.connect", uuid.New()},
		{"logo provider target", "provider.logo.set", uuid.New()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := providerCatalogRecord(t, tc.action, tc.target)
			parsed, err := parser.Parse(record)
			if err != nil {
				t.Fatal("recorded catalog envelope rejected", err)
			}
			if parsed.Action != tc.action || parsed.ObjectID != tc.target.String() {
				t.Fatal("audit identity changed", parsed)
			}
			var body map[string]any
			if err := json.Unmarshal(record.Value, &body); err != nil {
				t.Fatal(err)
			}
			data := body["data"].(map[string]any)
			for _, side := range []string{"before", "after"} {
				for _, field := range []string{"name", "config", "credential", "api_key", "logo_asset_id", "signed_url"} {
					data[side] = map[string]any{field: "private"}
					record.Value, _ = json.Marshal(body)
					if _, err := parser.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
						t.Fatalf("unreviewed %s.%s accepted: %v", side, field, err)
					}
				}
				delete(data, side)
			}
			data["action"] = "provider.custom.unregistered"
			record.Value, _ = json.Marshal(body)
			if _, err := parser.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
				t.Fatal("unknown provider action accepted", err)
			}
		})
	}
}
