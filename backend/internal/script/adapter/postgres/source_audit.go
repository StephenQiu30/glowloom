package postgres

import (
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
)

func sourceAudit(tx *gorm.DB, actor identityapp.Principal, p application.WritePlan, result application.SourceReceipt) error {
	const topic = "lanverse.audit.recorded.v1"
	id := uuid.NewSHA1(p.Command.Key, []byte("script-source-audit/"+actor.ID.String()))
	summary := struct {
		ScriptRevision       int64     `json:"script_revision"`
		ProjectRevision      int64     `json:"project_revision"`
		VersionID            uuid.UUID `json:"version_id"`
		SplitSetID           uuid.UUID `json:"split_set_id"`
		SourceCount          int       `json:"source_count"`
		ContentHash          string    `json:"content_hash"`
		DocumentSHA256       string    `json:"document_sha256"`
		SourceManifestSHA256 string    `json:"source_manifest_sha256"`
	}{result.ScriptRevision, result.ProjectRevision, result.VersionID, result.SplitSetID, len(p.Version.SourceIDs), p.Version.ContentHash, p.Version.DocumentSHA256, p.Version.SourceManifestSHA256}
	data, err := json.Marshal(map[string]any{"event_id": id, "event_type": topic, "occurred_at": p.CreatedAt, "org_id": actor.OrgID, "project_id": p.Command.ProjectID, "actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "audit", "id": id}, "data": map[string]any{"action": "script.source_" + p.Command.Action, "object": map[string]any{"type": "script_version", "id": result.VersionID.String()}, "before": nil, "after": summary, "request_id": p.Command.RequestID.String()}})
	if err != nil {
		return err
	}
	return exactlyOne(tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, id, topic, p.Command.ProjectID.String(), string(data)))
}
