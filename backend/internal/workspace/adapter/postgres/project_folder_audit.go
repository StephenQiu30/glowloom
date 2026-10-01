package postgres

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func folderAudit(tx *gorm.DB, actor identityapp.Principal, in application.FolderChangeInput, out application.FolderChangeResult, now time.Time) error {
	actions := map[string]string{"create": "created", "patch": "updated", "move": "moved", "recycle": "recycled"}
	summary := map[string]any{"recycled_count": len(out.RecycledProjectIDs), "name_updated": in.Name != nil, "cover_updated": in.SetCover}
	id := in.FolderID
	if out.Folder != nil {
		id = out.Folder.ID
		summary["folder_id"] = id
		summary["revision"] = out.Folder.Revision
	}
	var project any
	if out.Placement != nil {
		project = out.Placement.ProjectID
		summary["project_id"] = project
		summary["folder_id"] = out.Placement.FolderID
		summary["placement_revision"] = out.Placement.Revision
		if id == uuid.Nil {
			id = out.Placement.ProjectID
		}
	}
	eventID := uuid.New()
	const topic = "lanverse.audit.recorded.v1"
	body, err := json.Marshal(map[string]any{"event_id": eventID, "event_type": topic, "occurred_at": now.UTC(), "org_id": actor.OrgID, "project_id": project, "actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "audit", "id": eventID}, "data": map[string]any{"action": "project_folder." + actions[in.Action], "object": map[string]any{"type": "project_folder", "id": id}, "request_id": in.RequestID, "before": nil, "after": summary}})
	if err != nil {
		return err
	}
	partition := actor.OrgID.String()
	if out.Placement != nil {
		partition = out.Placement.ProjectID.String()
	}
	return tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload)VALUES(?,?,?,?::jsonb)`, eventID, topic, partition, string(body)).Error
}
