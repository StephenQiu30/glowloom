package aigateway

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

// Control preserves one authorized business command and its durable public receipt.
// Recovery only reopens original archival work; it never creates another execution.
func (c Creation) Control(ctx context.Context, a identity.Principal, p, id uuid.UUID, target, action, key string) (operation.WorkflowControlResult, error) {
	var out operation.WorkflowControlResult
	k, err := uuid.Parse(key)
	if err != nil || k == uuid.Nil || (target != "operation" && target != "batch") || (action != "cancel" && action != "recover" && action != "resume") {
		return out, domain.ErrInvalid
	}
	command := uuid.NewSHA1(k, []byte("ai/control"))
	hash := domain.Digest(canonical(struct {
		Scope, Project, Target uuid.UUID
		Type, Action           string
	}{a.ScopeID, p, id, target, action}))
	err = c.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := c.auth()(ctx, tx, a, a.ScopeID, p, false); err != nil {
			return err
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", a.ID.String()+command.String()).Error; err != nil {
			return err
		}
		var prior struct {
			InputHash string
			Response  []byte
		}
		q := tx.Raw("SELECT input_hash,response FROM ai.project_command WHERE actor_id=? AND command_key=?", a.ID, command).Scan(&prior)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 1 {
			if prior.InputHash != hash {
				return domain.ErrConflict
			}
			return json.Unmarshal(prior.Response, &out)
		}
		runtime := *c.Runtime
		runtime.DB = tx
		inner := c
		inner.Runtime = &runtime
		ids := []uuid.UUID{id}
		if target == "batch" {
			if err := tx.Exec("SELECT id FROM ai.batch WHERE id=? FOR UPDATE", id).Error; err != nil {
				return err
			}
			var err error
			ids, err = c.batchIDs(ctx, tx, a, p, id)
			if err != nil {
				return err
			}
		}
		out = operation.WorkflowControlResult{RequestID: k, EventID: uuid.New(), TargetID: id, TargetType: target, Action: action}
		for _, oid := range ids {
			if err := tx.Exec("SELECT id FROM ai.creation WHERE id=? FOR UPDATE", oid).Error; err != nil {
				return err
			}
			row, err := inner.row(tx, a, p, oid)
			if err != nil {
				return err
			}
			if row.ActorID != a.ID {
				return domain.ErrForbidden
			}
			if row.State == "closed" {
				continue
			}
			if row.State == "quoted" {
				if target != "batch" || action != "cancel" {
					return domain.ErrConflict
				}
				if err := tx.Exec("UPDATE ai.creation SET state='closed' WHERE id=? AND state='quoted'", oid).Error; err != nil {
					return err
				}
				out.Accepted = true
				continue
			}
			store := inner.store(tx, a)
			record, err := store.Read(ctx, a.ScopeID, oid)
			if err != nil {
				return err
			}
			if action == "cancel" {
				if record.State == "settled" || record.State == "released" {
					continue
				}
				call, err := store.LoadQuote(ctx, a.ScopeID, oid)
				if err != nil {
					return err
				}
				if _, err = store.CancelQueued(ctx, call); err != nil {
					return err
				}
				out.Accepted = true
				continue
			}
			if action == "resume" {
				continue
			} // Terminal original requests never regenerate.
			if record.State == "received" {
				if _, err = store.Settle(ctx, oid); err != nil {
					return err
				}
				out.Accepted = true
			}
			// The original persisted kind and business ID are the only recovery target.
			// An active lease, successful completion, or pending review is not reopened.
			q = tx.Exec(`UPDATE infra.task_work SET failed_at=NULL,failures=0,owner=NULL,lease_until=NULL,
    stop_requested_at=NULL,epoch=epoch+1,next_run_at=?
    WHERE kind='ai.deliver' AND business_id=? AND failed_at IS NOT NULL AND completed_at IS NULL
    AND (lease_until IS NULL OR lease_until<=clock_timestamp())
    AND EXISTS(SELECT 1 FROM ai.request r WHERE r.id=? AND r.state='settled'
     AND r.contract->>'protocol'='media' AND EXISTS(SELECT 1 FROM ai.attempt a WHERE a.request_id=r.id AND a.receipt IS NOT NULL))`, time.Now(), oid.String(), oid)
			if q.Error != nil {
				return q.Error
			}
			out.Accepted = out.Accepted || q.RowsAffected == 1
		}
		return tx.Exec("INSERT INTO ai.project_command(actor_id,command_key,input_hash,response) VALUES(?,?,?,?::jsonb)", a.ID, command, hash, string(canonical(out))).Error
	})
	return out, err
}
