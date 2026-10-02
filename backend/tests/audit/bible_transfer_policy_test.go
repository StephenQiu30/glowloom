package audit_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
)

func TestBibleRecordedPolicyRejectsPrivateContentAndUnownedActions(t *testing.T) {
	for _, action := range []string{"bible.character_create", "bible.character_update", "bible.character_confirm", "bible.character_delete", "bible.character_restore", "bible.character_merge", "bible.character_split", "bible.character_look_create", "bible.character_look_update", "bible.character_look_delete", "bible.character_look_default", "bible.character_references", "bible.character_voice_bind", "bible.character_voice_unbind", "bible.character_adopt_result", "bible.character_create_result", "bible.location_create", "bible.prop_create"} {
		after := map[string]any{"revision": 2, "project_revision": 3, "version_id": uuid.NewString(), "content_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
		checkBibleTransferAudit(t, action, "character", after)
	}
}

func TestMediaTransferRecordedPolicyRejectsPrivateObjectAndCommandFields(t *testing.T) {
	for _, action := range []string{"create", "cancel", "retry", "reconcile"} {
		after := map[string]any{"id": uuid.NewString(), "revision": 2, "status": "queued", "count": 2}
		checkBibleTransferAudit(t, "media.transfer."+action, "media.transfer", after)
	}
}

func checkBibleTransferAudit(t *testing.T, action, kind string, after map[string]any) {
	t.Helper()
	record := auditRecord(t, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), time.Now().UTC(), after)
	var body map[string]any
	if err := json.Unmarshal(record.Value, &body); err != nil {
		t.Fatal(err)
	}
	data := body["data"].(map[string]any)
	data["action"], data["object"], data["before"], data["after"], data["request_id"] = action, map[string]any{"type": kind, "id": uuid.NewString()}, nil, after, uuid.NewString()
	parser := auditapp.NewRecordedActionParser()
	record.Value, _ = json.Marshal(body)
	if _, err := parser.Parse(record); err != nil {
		t.Fatal("safe actual producer summary rejected", action, err)
	}
	data["action"] = "bible.location_voice_unbind"
	record.Value, _ = json.Marshal(body)
	if _, err := parser.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
		t.Fatal("unowned action accepted", err)
	}
	data["action"] = action
	for _, field := range []string{"content", "definition", "instructions", "voice_key", "params", "object_key", "signed_url", "manifest", "worker_fence", "request_key", "target_name"} {
		after[field] = "private payload"
		record.Value, _ = json.Marshal(body)
		if _, err := parser.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
			t.Fatal("private field accepted", action, field, err)
		}
		delete(after, field)
	}
}
