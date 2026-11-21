package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// SavePending durably remembers supplier acceptance before permitting any status query.
func (s *Store) SavePending(ctx context.Context, d application.Dispatch, receipt domain.Receipt) (domain.Record, error) {
	var out domain.Record
	if !receipt.Pending || receipt.UpstreamID == "" || len(receipt.UpstreamID) > 256 || d.Call.Model.Protocol != "media" {
		return out, domain.ErrInvalid
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT id FROM ai.request WHERE id=? FOR UPDATE", d.Call.ID).Error; err != nil {
			return err
		}
		var prior string
		q := tx.Raw("SELECT task_id FROM ai.pending WHERE request_id=?", d.Call.ID).Scan(&prior)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if prior != receipt.UpstreamID {
				return domain.ErrConflict
			}
			var err error
			out, err = scanRecord(tx, d.Call.ID)
			return err
		}
		q = tx.Exec("INSERT INTO ai.pending(request_id,attempt_id,task_id,receipt) SELECT ?,?,?,?::jsonb WHERE EXISTS(SELECT 1 FROM ai.attempt WHERE id=? AND request_id=? AND state='sending' AND receipt IS NULL)", d.Call.ID, d.Attempt, receipt.UpstreamID, string(mustJSON(receipt)), d.Attempt, d.Call.ID)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return domain.ErrConflict
		}
		if err := tx.Exec("UPDATE ai.attempt SET state='pending' WHERE id=?", d.Attempt).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE ai.request SET state='pending',updated_at=clock_timestamp() WHERE id=?", d.Call.ID).Error; err != nil {
			return err
		}
		if err := taskruntime.Enqueue(ctx, tx, "ai.poll", d.Call.ID.String(), time.Now().Add(time.Second)); err != nil {
			return err
		}
		var err error
		out, err = scanRecord(tx, d.Call.ID)
		return err
	})
	return out, err
}

// PendingDispatch is a query-only capability bound to the original supplier task and key.
func (s *Store) PendingDispatch(ctx context.Context, id uuid.UUID) (application.Dispatch, error) {
	var d application.Dispatch
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct {
			ScopeID, ActorID uuid.UUID
			ProjectID, RunID *uuid.UUID
			Model            []byte
			Deadline         time.Time
			State            string
			Attempt          uuid.UUID
			TaskID           string
			Route            []byte
			KeyID            string
			Ciphertext       []byte
			HasReceipt       bool
		}
		q := tx.Raw(`SELECT r.scope_id,r.actor_id,r.project_id,r.run_id,r.contract AS model,r.deadline,r.state,p.attempt_id AS attempt,p.task_id,a.contract AS route,k.key_id,k.ciphertext,a.receipt IS NOT NULL AS has_receipt FROM ai.request r JOIN ai.pending p ON p.request_id=r.id JOIN ai.attempt a ON a.id=p.attempt_id JOIN ai.credential k ON k.id=a.credential_id WHERE r.id=?`, id).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return domain.ErrUnavailable
		}
		if err := s.check(ctx, tx, row.ScopeID, uuid.Nil, false); err != nil {
			return err
		}
		d.Call = domain.Call{ID: id, Scope: row.ScopeID, Actor: row.ActorID, Deadline: row.Deadline}
		if row.ProjectID != nil {
			d.Call.Project = *row.ProjectID
		}
		if row.RunID != nil {
			d.Call.Run = *row.RunID
		}
		if json.Unmarshal(row.Model, &d.Call.Model) != nil || json.Unmarshal(row.Route, &d.Route) != nil {
			return domain.ErrInvalid
		}
		d.Attempt = row.Attempt
		d.Route.KeyID = row.KeyID
		d.Route.Ciphertext = row.Ciphertext
		d.PollTask = row.TaskID
		d.Send = !row.HasReceipt && (row.State == "pending" || row.State == "unknown")
		var err error
		d.Record, err = scanRecord(tx, id)
		return err
	})
	return d, err
}
