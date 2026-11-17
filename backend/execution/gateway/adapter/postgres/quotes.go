package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

// Quote is a frozen user decision, never a reservation or permission to send.
type Quote struct {
	ID        uuid.UUID   `json:"operation_id"`
	Credits   int64       `json:"credits"`
	Available int64       `json:"available"`
	Expires   time.Time   `json:"expires_at"`
	Call      domain.Call `json:"-"`
}

// Quote freezes a validated contract without reserving any money.
func (s *Store) Quote(ctx context.Context, call domain.Call) (Quote, error) {
	var result Quote
	prepared, err := application.Prepare(call.Model, call.Actor, call.Scope, call.Project, call.Run, call.Key, call.Origin, call.Raw)
	if err != nil || prepared.Hash != call.Hash || prepared.Limit != call.Limit {
		return result, domain.ErrInvalid
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, call.Scope, call.Project, false); err != nil {
			return err
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", call.Scope.String()+"/"+call.Key).Error; err != nil {
			return err
		}
		var prior struct {
			ID        uuid.UUID
			InputHash string
			Contract  []byte
			ExpiresAt time.Time
		}
		r := tx.Raw("SELECT id,input_hash,contract,expires_at FROM ai.quote WHERE scope_id=? AND idempotency_key=?", call.Scope, call.Key).Scan(&prior)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 1 {
			if prior.InputHash != call.Hash {
				return domain.ErrConflict
			}
			var original domain.Call
			if json.Unmarshal(prior.Contract, &original) != nil {
				return domain.ErrInvalid
			}
			result = Quote{ID: prior.ID, Credits: original.Limit, Expires: prior.ExpiresAt, Call: original}
		} else {
			raw, _ := json.Marshal(call)
			expires := time.Now().Add(15 * time.Minute)
			if err := tx.Exec("INSERT INTO ai.quote(id,scope_id,actor_id,idempotency_key,input_hash,contract,expires_at) VALUES(?,?,?,?,?,?::jsonb,?)", call.ID, call.Scope, call.Actor, call.Key, call.Hash, string(raw), expires).Error; err != nil {
				return err
			}
			result = Quote{ID: call.ID, Credits: call.Limit, Expires: expires, Call: call}
		}
		return tx.Raw("SELECT COALESCE((SELECT available FROM ai.account WHERE scope_id=?),0)", call.Scope).Scan(&result.Available).Error
	})
	return result, err
}

// LoadQuote verifies the original payer and scope; a confirmed replay can outlive expiry.
func (s *Store) LoadQuote(ctx context.Context, scope, id uuid.UUID) (domain.Call, error) {
	var result domain.Call
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct {
			ActorID   uuid.UUID
			Contract  []byte
			ExpiresAt time.Time
		}
		r := tx.Raw("SELECT actor_id,contract,expires_at FROM ai.quote WHERE scope_id=? AND id=?", scope, id).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 || row.ActorID != s.actor.ID || json.Unmarshal(row.Contract, &result) != nil {
			return domain.ErrForbidden
		}
		if err := s.check(ctx, tx, scope, result.Project, false); err != nil {
			return err
		}
		if row.ExpiresAt.Before(time.Now()) {
			var n int64
			if err := tx.Raw("SELECT count(*) FROM ai.request WHERE id=?", id).Scan(&n).Error; err != nil {
				return err
			}
			if n == 0 {
				return domain.ErrInvalid
			}
		}
		result.Deadline = time.Now().Add(180 * time.Second)
		return nil
	})
	return result, err
}

// RecoverReceipts repairs only persisted known outcomes; it never buys a model call.
func (s *Store) RecoverReceipts(ctx context.Context, limit int) error {
	if limit < 1 || limit > 100 {
		return domain.ErrInvalid
	}
	var ids []uuid.UUID
	if err := s.db.WithContext(ctx).Raw("SELECT id FROM ai.request WHERE state='received' ORDER BY updated_at,id LIMIT ?", limit).Scan(&ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.Settle(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
