package audit_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
)

func TestMediaUploadRecordedPolicy(t *testing.T) {
	parser := auditapp.NewRecordedActionParser()
	after := map[string]any{"kind": "document", "byte_size": 128, "sha256": "frozen content digest", "review_method": "local_workspace_owner_review", "reused": false}
	record := auditRecord(t, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), time.Now().UTC(), after)
	var body map[string]any
	if err := json.Unmarshal(record.Value, &body); err != nil {
		t.Fatal(err)
	}
	data := body["data"].(map[string]any)
	id := uuid.NewString()
	data["action"] = "media.uploaded"
	data["object"] = map[string]any{"type": "media_asset", "id": id}
	data["before"] = nil
	data["after"] = after
	data["request_id"] = uuid.NewString()
	record.Value, _ = json.Marshal(body)
	parsed, err := parser.Parse(record)
	if err != nil || parsed.Action != "media.uploaded" || parsed.ObjectID != id {
		t.Fatal("safe original upload summary rejected", err)
	}
	for _, field := range []string{"file_name", "object_key", "signed_url", "content", "rich_document", "actor_id"} {
		after[field] = "private value"
		record.Value, _ = json.Marshal(body)
		if _, err := parser.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
			t.Fatal("unreviewed original upload field accepted", field, err)
		}
		delete(after, field)
	}
}
