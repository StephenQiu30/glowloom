package postgres

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

// Integration identifies a session-backed, scoped SDK grant with a total budget.
type Integration struct {
	ID, Scope uuid.UUID
	Actor     identity.Principal
}

// WithGrant restricts admission to the exact scoped integration grant.
func (s *Store) WithGrant(id uuid.UUID) *Store { bound := *s; bound.grant = id; return &bound }

// IssueGrant creates no funds and returns its secret only once.
func (s *Store) IssueGrant(ctx context.Context, scope uuid.UUID, models []uuid.UUID, limit int64, seconds int) (string, error) {
	if len(models) < 1 || len(models) > 32 || limit < 0 || seconds < 60 || seconds > 3600 {
		return "", domain.ErrInvalid
	}
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(bytes[:])
	hash := domain.Digest([]byte(token))
	id := uuid.New()
	actor, _ := json.Marshal(s.actor)
	raw, _ := json.Marshal(models)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, scope, uuid.Nil, false); err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO ai.account(scope_id) VALUES(?) ON CONFLICT DO NOTHING", scope).Error; err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO ai.budget(scope_id,id,limit_credits) VALUES(?,?,?)", scope, id, limit).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO ai.integration_grant(id,token_hash,scope_id,actor,model_ids,expires_at) VALUES(?,?,?,?::jsonb,?::jsonb,?)`, id, hash, scope, string(actor), string(raw), time.Now().Add(time.Duration(seconds)*time.Second)).Error
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// ResolveGrant only reads an opaque token hash; all write checks repeat in Admit.
func ResolveGrant(ctx context.Context, db *gorm.DB, token string) (Integration, error) {
	var result Integration
	if len(token) < 32 || len(token) > 100 {
		return result, domain.ErrForbidden
	}
	var row struct {
		ID, ScopeID uuid.UUID
		Actor       []byte
	}
	r := db.WithContext(ctx).Raw("SELECT id,scope_id,actor FROM ai.integration_grant WHERE token_hash=? AND NOT revoked AND expires_at>clock_timestamp()", domain.Digest([]byte(token))).Scan(&row)
	if r.Error != nil {
		return result, r.Error
	}
	if r.RowsAffected != 1 || json.Unmarshal(row.Actor, &result.Actor) != nil {
		return result, domain.ErrForbidden
	}
	result.ID = row.ID
	result.Scope = row.ScopeID
	return result, nil
}
