package postgres

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// collaborationCommitted uses the same event identity in the row and envelope;
// the relay rejects bare collaboration payloads before publishing them.
func collaborationCommitted(tx *gorm.DB, actor identity.Principal, documentID, projectID, epoch uuid.UUID, seq, revision int64) error {
	const topic = "glowloom.canvas.collaboration_committed.v1"
	eventID := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"event_id": eventID, "event_type": topic, "occurred_at": time.Now().UTC(),
		"org_id": actor.OrgID, "project_id": projectID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "canvas_document", "id": documentID, "revision": revision},
		"data":      map[string]any{"document_id": documentID, "epoch": epoch, "commit_seq": strconv.FormatInt(seq, 10), "revision": strconv.FormatInt(revision, 10)},
	})
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, eventID, topic, documentID.String(), string(payload)).Error
}
