package postgres

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

// Reconciliation is an administrator's explicit original-supplier evidence, never a new send.
type Reconciliation struct {
	Request    uuid.UUID     `json:"request_id"`
	Attempt    uuid.UUID     `json:"attempt_id"`
	Reference  string        `json:"reference"`
	Reason     string        `json:"reason"`
	NotCharged bool          `json:"not_charged"`
	Usage      *domain.Usage `json:"usage,omitempty"`
}

// Reconcile closes an unknown original attempt with immutable evidence and one funds outcome.
func (s *Store) Reconcile(ctx context.Context, in Reconciliation) (domain.Record, error) {
	var out domain.Record
	if in.Request == uuid.Nil || in.Attempt == uuid.Nil || strings.TrimSpace(in.Reference) == "" || len(in.Reference) > 1000 || strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 500 || in.NotCharged == (in.Usage != nil) {
		return out, domain.ErrInvalid
	}
	digest := domain.Digest(mustJSON(in))
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct {
			ScopeID, ActorID uuid.UUID
			ProjectID, RunID *uuid.UUID
			InputHash        string
			Contract         []byte
		}
		q := tx.Raw("SELECT scope_id,actor_id,project_id,run_id,input_hash,contract FROM ai.request WHERE id=?", in.Request).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return domain.ErrUnavailable
		}
		if err := s.check(ctx, tx, row.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		if err := tx.Exec("SELECT scope_id FROM ai.account WHERE scope_id=? FOR UPDATE", row.ScopeID).Error; err != nil {
			return err
		}
		var state string
		if err := tx.Raw("SELECT state FROM ai.request WHERE id=? FOR UPDATE", in.Request).Row().Scan(&state); err != nil {
			return err
		}
		var prior string
		q = tx.Raw("SELECT digest FROM ai.reconciliation WHERE request_id=?", in.Request).Scan(&prior)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 1 {
			if prior != digest {
				return domain.ErrConflict
			}
			var e error
			out, e = scanRecord(tx, in.Request)
			return e
		}
		var model domain.Model
		if json.Unmarshal(row.Contract, &model) != nil {
			return domain.ErrInvalid
		}
		if model.VideoMetering != nil && !in.NotCharged {
			var err error
			out, err = s.reconcileVideoUsage(ctx, tx, in, digest, state)
			return err
		}
		var attempt struct {
			Contract []byte
			State    string
		}
		q = tx.Raw("SELECT contract,state FROM ai.attempt WHERE id=? AND request_id=? AND state='unknown' FOR UPDATE", in.Attempt, in.Request).Scan(&attempt)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return domain.ErrConflict
		}
		if err := tx.Exec("INSERT INTO ai.reconciliation(request_id,attempt_id,actor_id,digest,evidence) VALUES(?,?,?,?,?::jsonb)", in.Request, in.Attempt, s.actor.ID, digest, string(mustJSON(in))).Error; err != nil {
			return err
		}
		inner := *s
		inner.db = tx
		if !in.NotCharged {
			var route domain.Route
			if in.Usage.Validate() != nil || json.Unmarshal(attempt.Contract, &route) != nil {
				return domain.ErrInvalid
			}
			cost, err := route.Purchase.Cost(*in.Usage)
			if err != nil {
				return err
			}
			receipt := domain.Receipt{Body: json.RawMessage(`{"reconciliation_only":true}`), Usage: *in.Usage, PurchaseMicros: cost, UpstreamID: in.Reference}
			if err = inner.SaveReceipt(ctx, in.Request, in.Attempt, receipt); err != nil {
				return err
			}
			out, err = inner.Settle(ctx, in.Request)
			return err
		}
		var err error
		out, err = scanRecord(tx, in.Request)
		if err != nil {
			return err
		}
		call := domain.Call{ID: in.Request, Scope: row.ScopeID, Actor: s.actor.ID, Hash: row.InputHash}
		if row.ProjectID != nil {
			call.Project = *row.ProjectID
		}
		if row.RunID != nil {
			call.Run = *row.RunID
		}
		if err := tx.Exec("UPDATE ai.attempt SET state='reconciled' WHERE request_id=? AND state='unknown'", in.Request).Error; err != nil {
			return err
		}
		return releaseHeld(tx, call, &out)
	})
	return out, err
}

// releaseHeld closes the original hold after affirmative terminal evidence.
func releaseHeld(tx *gorm.DB, call domain.Call, out *domain.Record) error {
	if err := budgetChange(tx, call, -out.Held, 0); err != nil {
		return err
	}
	if err := money(tx, call, "release", out.Held, -out.Held, 0); err != nil {
		return err
	}
	r := tx.Exec("UPDATE ai.account SET available=available+?,held=held-?,version=version+1 WHERE scope_id=? AND held>=?", out.Held, out.Held, call.Scope, out.Held)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return domain.ErrConflict
	}
	if err := tx.Exec("UPDATE ai.hold SET released=amount,closed=true WHERE request_id=? AND NOT closed", call.ID).Error; err != nil {
		return err
	}
	if err := tx.Exec("UPDATE ai.request SET state='released',updated_at=clock_timestamp() WHERE id=?", call.ID).Error; err != nil {
		return err
	}
	out.State = "released"
	out.Released = out.Held
	return nil
}
