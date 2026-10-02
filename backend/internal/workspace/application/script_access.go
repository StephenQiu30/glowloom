package application

import (
	"encoding/json"
	"math"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// ProjectContentAccess contains current workspace facts for a content owner.
// The owning transaction retains its project lock until content and events commit.
type ProjectContentAccess struct {
	ProjectID uuid.UUID
	OrgID     uuid.UUID
	Revision  int64
}

// ProjectContentChangedEvent invalidates current project content without exposing it.
func ProjectContentChangedEvent(actor identityapp.Principal, project uuid.UUID, revision int64, now time.Time) (identityapp.OutboxEvent, error) {
	if !canManageModelDefaults(actor) {
		return identityapp.OutboxEvent{}, identityapp.ErrForbidden
	}
	if project == uuid.Nil || revision < 1 || revision > math.MaxInt32 || now.IsZero() {
		return identityapp.OutboxEvent{}, ErrInvalidProjectChange
	}
	id := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"event_id": id, "event_type": projectChangedTopic, "occurred_at": now.UTC(),
		"org_id": actor.OrgID, "project_id": project,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "project", "id": project, "revision": revision},
		"data":      map[string]any{"change": "updated"},
	})
	if err != nil {
		return identityapp.OutboxEvent{}, err
	}
	return identityapp.OutboxEvent{ID: id, Topic: projectChangedTopic, PartitionKey: project.String(), Payload: payload}, nil
}
