package source

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"
)

// ProjectLifecycleEvents is the single legacy/native lifecycle event encoder.
// Source controls use stable IDs; the legacy transaction supplies its original IDs.
func ProjectLifecycleEvents(actorID, orgID uuid.UUID, action string, before, after domain.Project, requestID string, now time.Time, changedID, auditID uuid.UUID) ([]ProjectCreationEvent, error) {
	changes := map[string]string{"archive": "archived", "unarchive": "unarchived", "delete": "deleted", "restore": "restored"}
	change, ok := changes[action]
	if !ok {
		return nil, domain.ErrProjectStateConflict
	}
	changed, err := json.Marshal(map[string]any{"event_id": changedID, "event_type": ProjectChangedTopic, "occurred_at": now, "org_id": orgID, "project_id": after.ID, "actor": map[string]any{"kind": "user", "id": actorID}, "aggregate": map[string]any{"type": "project", "id": after.ID, "revision": after.Revision}, "data": map[string]any{"change": change}})
	if err != nil {
		return nil, fmt.Errorf("encode project lifecycle event: %w", err)
	}
	audit, err := json.Marshal(map[string]any{"event_id": auditID, "event_type": ProjectAuditTopic, "occurred_at": now, "org_id": orgID, "project_id": after.ID, "actor": map[string]any{"kind": "user", "id": actorID}, "aggregate": map[string]any{"type": "audit", "id": auditID}, "data": map[string]any{"action": "project." + change, "object": map[string]any{"type": "project", "id": after.ID}, "request_id": requestID, "before": projectLifecycleSummary(before), "after": projectLifecycleSummary(after)}})
	if err != nil {
		return nil, fmt.Errorf("encode project lifecycle audit: %w", err)
	}
	return []ProjectCreationEvent{{ID: changedID, Topic: ProjectChangedTopic, PartitionKey: after.ID.String(), Payload: changed}, {ID: auditID, Topic: ProjectAuditTopic, PartitionKey: after.ID.String(), Payload: audit}}, nil
}
func projectLifecycleSummary(p domain.Project) map[string]any {
	return map[string]any{"revision": p.Revision, "status": p.Status, "is_delete": p.IsDelete, "archived_at": p.ArchivedAt, "delete_time": p.DeleteTime, "purge_after": p.PurgeAfter}
}
