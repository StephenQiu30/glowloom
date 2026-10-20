package postgres

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func saveInputs(tx *gorm.DB, actor uuid.UUID, inputs []domain.InputSnapshot) error {
	for _, input := range inputs {
		draft, err := json.Marshal(input.Draft)
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

func replaySave(tx *gorm.DB, actor, document uuid.UUID, key, hash string, out *application.Result) (bool, error) {
	var locked int
	if err := tx.Raw(`SELECT 1 FROM pg_advisory_xact_lock(69360,hashtext(?))`, actor.String()+":"+key).Scan(&locked).Error; err != nil {
		return false, err
	}
	var row struct {
		DocumentID   uuid.UUID
		RequestHash  string
		ResponseBody []byte
	}
	r := tx.Raw(`SELECT document_id,request_hash,response_body FROM canvas.save_receipt WHERE actor_id=? AND idem_key=?`, actor, key).Scan(&row)
	if r.Error != nil {
		return false, fmt.Errorf("read canvas save receipt: %w", r.Error)
	}
	if r.RowsAffected == 0 {
		return false, nil
	}
	if row.DocumentID != document || row.RequestHash != hash {
		return false, application.ErrIdempotencyConflict
	}
	if err := json.Unmarshal(row.ResponseBody, out); err != nil {
		return false, fmt.Errorf("decode canvas save receipt: %w", err)
	}
	if out.ID != document || out.Revision < 2 {
		return false, domain.ErrInvalidCommand
	}
	return true, nil
}

func recordSave(tx *gorm.DB, actor uuid.UUID, key, hash string, result application.Result) error {
	body, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode canvas save receipt: %w", err)
	}
	r := tx.Exec(`INSERT INTO canvas.save_receipt(actor_id,idem_key,document_id,request_hash,result_revision,response_body) VALUES(?,?,?,?,?,?::jsonb)`, actor, key, result.ID, hash, result.Revision, string(body))
	if r.Error != nil {
		return fmt.Errorf("record canvas save receipt: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return fmt.Errorf("record canvas save receipt: no committed row")
	}
	return nil
}

func savedAudit(tx *gorm.DB, a identityapp.Principal, doc domain.Document, key string) error {
	id := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"event_id": id, "event_type": "lanverse.audit.recorded.v1", "occurred_at": time.Now().UTC(), "org_id": a.OrgID, "project_id": doc.ProjectID,
		"actor": map[string]any{"kind": "user", "id": a.ID}, "aggregate": map[string]any{"type": "audit", "id": id},
		"data": map[string]any{"action": "canvas.saved", "object": map[string]any{"type": "canvas", "id": doc.ID}, "request_id": key, "after": map[string]any{"revision": doc.Revision, "node_count": len(doc.Nodes)}}})
	if err != nil {
		return fmt.Errorf("encode canvas audit: %w", err)
	}
	r := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,'lanverse.audit.recorded.v1',?,?::jsonb)`, id, doc.ProjectID.String(), string(payload))
	if r.Error != nil {
		return fmt.Errorf("record canvas audit: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return fmt.Errorf("record canvas audit: no committed row")
	}
	return nil
}
