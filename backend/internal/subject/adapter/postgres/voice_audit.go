package postgres

import (
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
)

// auditVoice never includes secret-bearing configuration, text or object keys.
// Its insert shares the job-state transaction, including the paid-send fence.
func auditVoice(tx *gorm.DB, work application.VoiceWork, action string) error {
	raw, err := json.Marshal(struct {
		ScopeID        uuid.UUID `json:"scope_id"`
		DraftIntentID  uuid.UUID `json:"draft_intent_id"`
		ReleaseID      uuid.UUID `json:"release_id"`
		ModelVersionID uuid.UUID `json:"model_version_id"`
		AssetID        uuid.UUID `json:"asset_id"`
		Status         string    `json:"status"`
	}{work.Owner.ScopeID, work.Job.DraftIntentID, work.Job.ReleaseID, work.Job.ModelVersionID, work.AssetID, work.Job.Status})
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO audit.audit_log(id,org_id,actor_id,actor_kind,action,object_type,object_id,after,request_id) VALUES(?,?,?,'user',?,'subject_voice_job',?,?::jsonb,?)`, uuid.New(), work.Owner.PhysicalScopeID, work.Job.CurrentActorID, "subject.voice_"+action, work.Job.ID.String(), string(raw), work.Job.ID.String()).Error
}
