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

// ExecutionKind is owned by the Go Execution provider worker.
const ExecutionKind = "ai.execute"

// Queue atomically freezes funds and a route, then exposes only an identifier to Go Execution.
func (s *Store) Queue(ctx context.Context, call domain.Call) (domain.Record, error) {
	var out domain.Record
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		inner := *s
		inner.db = tx
		d, err := inner.Admit(ctx, call)
		if err != nil {
			return err
		}
		out = d.Record
		if !d.Send {
			return nil
		}
		if err := tx.Exec("UPDATE ai.attempt SET state='ready' WHERE id=? AND state='sending'", d.Attempt).Error; err != nil {
			return err
		}
		proof := json.RawMessage(`{}`)
		if s.workerPermit != nil {
			proof, err = s.workerPermit(ctx, tx, call)
			if err != nil {
				return err
			}
		}
		if err := tx.Exec("INSERT INTO ai.job(request_id,actor,worker_proof) VALUES(?,?::jsonb,?::jsonb) ON CONFLICT DO NOTHING", call.ID, string(mustJSON(s.actor)), string(proof)).Error; err != nil {
			return err
		}
		return taskruntime.Enqueue(ctx, tx, ExecutionKind, call.ID.String(), time.Now())
	})
	return out, err
}

// ClaimQueued consumes the persisted send permission once; lost leases never reopen it.
func (s *Store) ClaimQueued(ctx context.Context, call domain.Call) (application.Dispatch, error) {
	var out application.Dispatch
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := verifyCallIdentity(tx, call); err != nil {
			return err
		}
		if err := s.check(ctx, tx, call.Scope, call.Project, false); err != nil {
			return err
		}
		if err := tx.Exec("SELECT id FROM ai.request WHERE id=? FOR UPDATE", call.ID).Error; err != nil {
			return err
		}
		record, err := scanRecord(tx, call.ID)
		if err != nil {
			return err
		}
		out = application.Dispatch{Call: call, Record: record}
		if record.State != "sending" {
			return nil
		}
		var a struct {
			ID              uuid.UUID
			Contract        []byte
			State           string
			KeyID           string
			Ciphertext      []byte
			CredentialState string
		}
		q := tx.Raw(`SELECT a.id,a.contract,a.state,k.key_id,k.ciphertext,k.state AS credential_state FROM ai.attempt a JOIN ai.credential k ON k.id=a.credential_id WHERE a.request_id=? ORDER BY a.number DESC LIMIT 1 FOR UPDATE OF a FOR SHARE OF k`, call.ID).Scan(&a)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return domain.ErrConflict
		}
		out.Attempt = a.ID
		if a.State != "ready" {
			return nil
		}
		out.Attempt = a.ID
		if json.Unmarshal(a.Contract, &out.Route) != nil {
			return domain.ErrInvalid
		}
		out.Route.KeyID = a.KeyID
		out.Route.Ciphertext = a.Ciphertext
		if err := tx.Exec("UPDATE ai.attempt SET state='sending' WHERE id=?", a.ID).Error; err != nil {
			return err
		}
		out.Send = true
		if a.CredentialState != "active" || call.Deadline.Before(time.Now()) {
			return domain.ErrUnavailable
		}
		return nil
	})
	return out, err
}

// CancelQueued can release only a request whose durable send claim was never consumed.
func (s *Store) CancelQueued(ctx context.Context, call domain.Call) (domain.Record, error) {
	var out domain.Record
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := verifyCallIdentity(tx, call); err != nil {
			return err
		}
		if err := s.check(ctx, tx, call.Scope, call.Project, false); err != nil {
			return err
		}
		if err := tx.Exec("SELECT scope_id FROM ai.account WHERE scope_id=? FOR UPDATE", call.Scope).Error; err != nil {
			return err
		}
		if err := tx.Exec("SELECT id FROM ai.request WHERE id=? FOR UPDATE", call.ID).Error; err != nil {
			return err
		}
		var a struct{ ID uuid.UUID }
		q := tx.Raw("SELECT id FROM ai.attempt WHERE request_id=? AND state='ready' FOR UPDATE", call.ID).Scan(&a)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return domain.ErrUnknown
		}
		if err := tx.Exec("UPDATE ai.attempt SET state='sending' WHERE id=?", a.ID).Error; err != nil {
			return err
		}
		inner := *s
		inner.db = tx
		var err error
		out, err = inner.ReleaseNotSent(ctx, application.Dispatch{Call: call, Attempt: a.ID})
		return err
	})
	return out, err
}
