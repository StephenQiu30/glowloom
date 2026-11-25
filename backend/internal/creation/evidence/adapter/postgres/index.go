package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// ScriptIndexTask identifies the existing media worker's script indexing task.
const ScriptIndexTask = "script.evidence_index"

// ScriptIndexRequest carries only the exact adopted version and its initiator.
type ScriptIndexRequest struct {
	ActorID   uuid.UUID `json:"actor_id"`
	OrgID     uuid.UUID `json:"org_id"`
	ProjectID uuid.UUID `json:"project_id"`
	VersionID uuid.UUID `json:"version_id"`
}

// BusinessID is stable per immutable version and initiator.
func (r ScriptIndexRequest) BusinessID() string {
	return r.VersionID.String() + "/" + r.ActorID.String()
}

// ScheduleScriptIndex inserts an idempotent task without blocking for object reads.
func ScheduleScriptIndex(ctx context.Context, tx *gorm.DB, r ScriptIndexRequest, now time.Time) error {
	return taskruntime.EnqueueReference(ctx, tx, ScriptIndexTask, r.BusinessID(), now, r)
}

// PendingScriptIndex locates an old adopted version that still needs indexing.
func PendingScriptIndex(tx *gorm.DB, project uuid.UUID) (uuid.UUID, error) {
	var row struct{ AdoptedVersionID uuid.UUID }
	err := tx.Raw(`SELECT s.adopted_version_id FROM script.project_state s
		WHERE s.project_id=? AND s.adopted_version_id IS NOT NULL AND NOT EXISTS
		(SELECT 1 FROM workspace.script_evidence_index i WHERE i.project_id=s.project_id AND i.version_id=s.adopted_version_id)`, project).Scan(&row).Error
	return row.AdoptedVersionID, err
}

// ScriptChunk is a bounded passage at exact source-local character offsets.
type ScriptChunk struct {
	ProjectID uuid.UUID
	VersionID uuid.UUID
	SourceID  uuid.UUID
	Part      int
	StartPos  int
	EndPos    int
	Body      string
}

// ScriptChunks deterministically segments text with a 200-character overlap.
func ScriptChunks(r ScriptIndexRequest, source uuid.UUID, plain string) []ScriptChunk {
	runes := []rune(plain)
	var result []ScriptChunk
	for start := 0; start < len(runes); start += 1000 {
		end := min(len(runes), start+1200)
		result = append(result, ScriptChunk{r.ProjectID, r.VersionID, source, len(result) + 1, start, end, string(runes[start:end])})
		if end == len(runes) {
			break
		}
	}
	return result
}

// PublishScriptIndex activates a complete batch atomically under the owning
// project lock. A duplicate batch cannot overwrite current or newer content.
func PublishScriptIndex(tx *gorm.DB, r ScriptIndexRequest, chunks []ScriptChunk) error {
	var present int
	head := tx.Raw(`SELECT 1 FROM script.project_state WHERE project_id=? AND org_id=? AND adopted_version_id=?`, r.ProjectID, r.OrgID, r.VersionID).Scan(&present)
	if head.Error != nil || head.RowsAffected == 0 {
		return head.Error
	}
	activated := tx.Exec(`INSERT INTO workspace.script_evidence_index(project_id,version_id) VALUES(?,?) ON CONFLICT DO NOTHING`, r.ProjectID, r.VersionID)
	if activated.Error != nil || activated.RowsAffected == 0 {
		return activated.Error
	}
	if len(chunks) > 0 {
		if err := tx.Table("workspace.script_evidence_chunk").Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(chunks, 128).Error; err != nil {
			return err
		}
	}
	return tx.Exec(`DELETE FROM workspace.script_evidence_index WHERE project_id=? AND version_id<>?`, r.ProjectID, r.VersionID).Error
}
