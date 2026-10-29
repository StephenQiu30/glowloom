package audit_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	auditapp "github.com/StephenQiu30/glowloom-server/backend/internal/audit/application"
	"github.com/google/uuid"
)

func TestTimelineExportRecordedPolicyAcceptsArtifactIdentityWithoutPrivatePayload(t *testing.T) {
	for _, action := range []string{"media.export_requested", "media.export_cancel", "media.export_retry", "media.export_reviewed"} {
		t.Run(action, func(t *testing.T) {
			for _, artifact := range []any{nil, uuid.NewString()} {
				after := map[string]any{"id": uuid.NewString(), "project_id": uuid.NewString(), "output_kind": "xmeml", "status": "succeeded", "artifact_id": artifact, "asset_id": nil, "revision": 2, "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
				record := auditRecord(t, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), time.Now().UTC(), after)
				var body map[string]any
				if err := json.Unmarshal(record.Value, &body); err != nil {
					t.Fatal(err)
				}
				data := body["data"].(map[string]any)
				data["action"] = action
				data["object"] = map[string]any{"type": "media_export", "id": after["id"]}
				data["before"], data["after"], data["request_id"] = nil, after, uuid.NewString()
				parser := auditapp.NewRecordedActionParser()
				record.Value, _ = json.Marshal(body)
				if parsed, err := parser.Parse(record); err != nil || parsed.Action != action {
					t.Fatal("export artifact summary rejected", artifact, err)
				}
				for _, field := range []string{"object_key", "signed_url", "frozen", "manifest", "file_name", "settings"} {
					after[field] = "private payload"
					record.Value, _ = json.Marshal(body)
					if _, err := parser.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
						t.Fatal("private export payload accepted", field, err)
					}
					delete(after, field)
				}
			}
		})
	}
}
