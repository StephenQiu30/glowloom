package postgres

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

func reviewAudit(tx *gorm.DB, actor identityapp.Principal, project, key, request uuid.UUID, action string, revision int64, version uuid.UUID, episode *uuid.UUID, now time.Time) error {
	const topic = "lanverse.audit.recorded.v1"
	id := uuid.NewSHA1(key, []byte("script-review-audit/"+actor.ID.String()+"/"+action))
	summary := struct {
		ScriptRevision int64      `json:"script_revision"`
		VersionID      uuid.UUID  `json:"version_id"`
		EpisodeID      *uuid.UUID `json:"episode_id,omitempty"`
	}{revision, version, episode}
	data, err := json.Marshal(map[string]any{"event_id": id, "event_type": topic, "occurred_at": now, "org_id": actor.OrgID, "project_id": project, "actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "audit", "id": id}, "data": map[string]any{"action": action, "object": map[string]any{"type": "script_version", "id": version.String()}, "before": nil, "after": summary, "request_id": request.String()}})
	if err != nil {
		return err
	}
	return exactlyOne(tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, id, topic, project.String(), string(data)))
}
