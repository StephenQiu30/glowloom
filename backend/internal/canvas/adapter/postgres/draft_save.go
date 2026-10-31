package postgres

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func saveInputs(tx *gorm.DB, actor uuid.UUID, inputs []domain.InputSnapshot) error {
	for _, input := range inputs {
		var value any = input.Draft
		if input.Subject != nil {
			value = subjectInput{Kind: "subject", Title: input.Title, Subject: *input.Subject}
		}
		draft, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encode canvas input: %w", err)
		}
		r := tx.Exec(`INSERT INTO canvas.node_input(node_id,input_revision,draft,created_by) VALUES(?,?,?::jsonb,?)`, input.NodeID, input.Revision, string(draft), actor)
		if r.Error != nil {
			return fmt.Errorf("append canvas input: %w", r.Error)
		}
		if r.RowsAffected != 1 {
			return fmt.Errorf("append canvas input: no committed row")
		}
	}
	return nil
}

func savedAudit(tx *gorm.DB, a identityapp.Principal, doc domain.Document, key string) error {
	id := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"event_id": id, "event_type": "glowloom.audit.recorded.v1", "occurred_at": time.Now().UTC(), "org_id": a.OrgID, "project_id": doc.ProjectID,
		"actor": map[string]any{"kind": "user", "id": a.ID}, "aggregate": map[string]any{"type": "audit", "id": id},
		"data": map[string]any{"action": "canvas.saved", "object": map[string]any{"type": "canvas", "id": doc.ID}, "request_id": key, "after": map[string]any{"revision": doc.Revision, "node_count": len(doc.Nodes)}}})
	if err != nil {
		return fmt.Errorf("encode canvas audit: %w", err)
	}
	r := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,'glowloom.audit.recorded.v1',?,?::jsonb)`, id, doc.ProjectID.String(), string(payload))
	if r.Error != nil {
		return fmt.Errorf("record canvas audit: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return fmt.Errorf("record canvas audit: no committed row")
	}
	return nil
}

// subjectInput discriminates immutable local-subject facts from generation drafts.
type subjectInput struct {
	Kind    string               `json:"kind"`
	Title   string               `json:"title"`
	Subject domain.SubjectConfig `json:"subject"`
}
