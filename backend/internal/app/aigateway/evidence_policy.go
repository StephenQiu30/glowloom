package aigateway

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	evidencepg "github.com/StephenQiu30/glowloom-server/backend/creation/evidence/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// ConfigureEvidence is an offline operator command; the service role cannot publish policy.
func ConfigureEvidence(ctx context.Context, db *gorm.DB, project, actor uuid.UUID, credits int64, enabled bool) error {
	if project == uuid.Nil || enabled && (actor == uuid.Nil || credits <= 0) {
		return domain.ErrInvalid
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "evidence/"+project.String()).Error; err != nil {
			return err
		}
		if !enabled {
			if err := tx.Exec("UPDATE workspace.evidence_semantic_policy SET enabled=false,active_release_id=NULL,active_profile='' WHERE project_id=?", project).Error; err != nil {
				return err
			}
			return tx.Exec("DELETE FROM workspace.evidence_embedding WHERE project_id=?", project).Error
		}
		var org uuid.UUID
		if err := tx.Raw("SELECT org_id FROM workspace.project WHERE id=? AND status='active' AND NOT is_delete", project).Row().Scan(&org); err != nil {
			return err
		}
		access, err := scopewire.ProvideWorkerProjectContentAccess(tx).Authorize(ctx, identity.Principal{ID: actor, OrgID: org}, project, true)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO workspace.evidence_semantic_policy(project_id,actor_id,limit_credits,generation,budget_id) VALUES(?,?,?,?,?)
   ON CONFLICT(project_id) DO UPDATE SET generation=CASE WHEN evidence_semantic_policy.enabled AND evidence_semantic_policy.actor_id=excluded.actor_id
   THEN evidence_semantic_policy.generation ELSE excluded.generation END,enabled=true,actor_id=excluded.actor_id,limit_credits=excluded.limit_credits`,
			project, actor, credits, uuid.New(), uuid.New()).Error; err != nil {
			return err
		}
		policy, err := evidencepg.ReadPolicy(tx, project)
		if err != nil {
			return err
		}
		if policy.ReleaseID == uuid.Nil {
			return fmt.Errorf("configure an available text.embed model in project model defaults: %w", domain.ErrUnavailable)
		}
		var over bool
		if err := tx.Raw("SELECT EXISTS(SELECT 1 FROM ai.budget WHERE scope_id=? AND id=? AND held+spent>?)", access.OwnerScopeID, policy.BudgetID, credits).Row().Scan(&over); err != nil {
			return err
		}
		if over {
			return errors.New("embedding allowance is below committed credits")
		}
		if err := taskruntime.EnqueueReference(ctx, tx, evidencepg.IndexTask, policy.Generation.String(), time.Now(), project); err != nil {
			return err
		}
		return tx.Exec(`UPDATE infra.task_work SET failed_at=NULL,failures=0,next_run_at=clock_timestamp() WHERE kind=? AND business_id=? AND failed_at IS NOT NULL`, evidencepg.IndexTask, policy.Generation.String()).Error
	})
}
