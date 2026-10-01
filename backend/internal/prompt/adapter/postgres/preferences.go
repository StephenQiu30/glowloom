// Package postgres persists prompt customizations in the canonical workspace database.
package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/prompt/application"
	"github.com/StephenQiu30/lanverse/backend/internal/prompt/domain"
)

var errUnavailable = errors.New("prompt preference storage unavailable")

// Store keeps private preferences behind an explicitly injected database handle.
type Store struct{ db *gorm.DB }

// NewStore injects the canonical PostgreSQL database.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// ReadCustomizations obtains only the currently authorized workspace actor's data.
func (s *Store) ReadCustomizations(ctx context.Context, actor identityapp.Principal) ([]domain.Customization, error) {
	if s == nil || s.db == nil {
		return nil, errUnavailable
	}
	customizations := []domain.Customization{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		return tx.Raw(`SELECT id,operation,mode,content,base_template_id,revision,update_time
		 FROM workspace.prompt_customization WHERE org_id=? AND owner_id=? ORDER BY operation`, actor.OrgID, actor.ID).Scan(&customizations).Error
	})
	if err != nil {
		return nil, fmt.Errorf("read prompt preferences transaction: %w", err)
	}
	return customizations, nil
}

// SaveCustomization rechecks current rights and serializes both first and subsequent writes.
func (s *Store) SaveCustomization(ctx context.Context, actor identityapp.Principal, input application.SaveInput, desired domain.Customization, event identityapp.OutboxEvent) (domain.Customization, error) {
	if s == nil || s.db == nil {
		return domain.Customization{}, errUnavailable
	}
	if desired.Validate() != nil || desired.Revision != input.ExpectedRevision+1 || input.IdempotencyKey == uuid.Nil ||
		event.ID == uuid.Nil || event.Topic != "lanverse.audit.recorded.v1" || event.PartitionKey != actor.OrgID.String() || len(event.Payload) == 0 {
		return domain.Customization{}, domain.ErrInvalidCustomization
	}
	body, err := json.Marshal(struct {
		Contract  string
		OrgID     uuid.UUID
		Operation string
		Mode      domain.Mode
		Content   string
		Baseline  uuid.UUID
		Revision  int64
	}{"workspace.prompt-customization.v1", actor.OrgID, input.Operation, input.Mode, input.Content, input.BaseTemplateID, input.ExpectedRevision})
	if err != nil {
		return domain.Customization{}, fmt.Errorf("encode prompt fingerprint: %w", err)
	}
	hash := sha256.Sum256(body)
	fingerprint := hex.EncodeToString(hash[:])
	var saved domain.Customization
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		var locked int
		if err := tx.Raw(`SELECT 1 FROM pg_advisory_xact_lock(69360,hashtext(?))`, actor.ID.String()+":"+input.IdempotencyKey.String()).Scan(&locked).Error; err != nil {
			return fmt.Errorf("lock prompt request: %w", err)
		}
		found, err := replayPreference(tx, actor.ID, input.IdempotencyKey, fingerprint, &saved)
		if err != nil || found {
			return err
		}
		if err := tx.Raw(`SELECT 1 FROM pg_advisory_xact_lock(69361,hashtext(?))`, actor.OrgID.String()+"/"+actor.ID.String()+"/"+input.Operation).Scan(&locked).Error; err != nil {
			return fmt.Errorf("lock personal prompt operation: %w", err)
		}
		var current struct {
			ID       uuid.UUID
			Revision int64
		}
		read := tx.Raw(`SELECT id,revision FROM workspace.prompt_customization
		 WHERE org_id=? AND owner_id=? AND operation=? FOR UPDATE`, actor.OrgID, actor.ID, input.Operation).Scan(&current)
		if read.Error != nil {
			return fmt.Errorf("read guarded prompt preference: %w", read.Error)
		}
		if current.Revision != input.ExpectedRevision || (read.RowsAffected == 1 && current.ID != desired.ID) {
			return application.ErrRevisionConflict
		}
		result := tx.Raw(`INSERT INTO workspace.prompt_customization(id,org_id,owner_id,operation,mode,content,base_template_id,revision)
		 VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(org_id,owner_id,operation) DO UPDATE SET
		 mode=excluded.mode,content=excluded.content,base_template_id=excluded.base_template_id,revision=excluded.revision,update_time=statement_timestamp()
		 WHERE workspace.prompt_customization.revision=?
		 RETURNING id,operation,mode,content,base_template_id,revision,update_time`, desired.ID, actor.OrgID, actor.ID,
			desired.Operation, desired.Mode, desired.Content, desired.BaseTemplateID, desired.Revision, input.ExpectedRevision).Scan(&saved)
		if result.Error != nil {
			return fmt.Errorf("write prompt preference: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return application.ErrRevisionConflict
		}
		if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, event.ID, event.Topic, event.PartitionKey, string(event.Payload)).Error; err != nil {
			return fmt.Errorf("record prompt preference audit: %w", err)
		}
		return recordPreference(tx, actor.ID, input.IdempotencyKey, fingerprint, saved)
	})
	if err != nil {
		return domain.Customization{}, fmt.Errorf("save prompt preference transaction: %w", err)
	}
	return saved, nil
}

func requireCurrentActor(tx *gorm.DB, actor identityapp.Principal) error {
	var present int
	result := tx.Raw(`SELECT 1 FROM identity."user" u JOIN workspace.organization o ON o.id=u.org_id
	 WHERE u.id=? AND u.org_id=? AND u.role IN ('admin','producer') AND u.status='active'
	 AND NOT u.is_delete AND NOT u.must_change_password AND o.status='active' AND NOT o.is_delete
	 FOR SHARE OF u,o`, actor.ID, actor.OrgID).Scan(&present)
	if result.Error != nil {
		return fmt.Errorf("check current prompt actor: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return identityapp.ErrForbidden
	}
	return nil
}

func replayPreference(tx *gorm.DB, actor, key uuid.UUID, fingerprint string, saved *domain.Customization) (bool, error) {
	var row struct {
		RequestHash  string
		StatusCode   int
		ResponseBody []byte
	}
	result := tx.Raw(`SELECT request_hash,status_code,response_body FROM infra.idempotency_record
	 WHERE actor_id=? AND idem_key=? AND NOT is_delete AND expires_at>statement_timestamp()`, actor, key.String()).Scan(&row)
	if result.Error != nil {
		return false, fmt.Errorf("read prompt preference receipt: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	if row.RequestHash != fingerprint || row.StatusCode != 200 {
		return false, application.ErrIdempotencyConflict
	}
	if err := json.Unmarshal(row.ResponseBody, saved); err != nil || saved.Validate() != nil {
		return false, domain.ErrInvalidCustomization
	}
	return true, nil
}

func recordPreference(tx *gorm.DB, actor, key uuid.UUID, fingerprint string, saved domain.Customization) error {
	body, err := json.Marshal(saved)
	if err != nil {
		return fmt.Errorf("encode prompt preference receipt: %w", err)
	}
	result := tx.Exec(`INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at)
	 VALUES(?,?,?,?,200,?::jsonb,statement_timestamp()+interval '24 hours')
	 ON CONFLICT(actor_id,idem_key) DO UPDATE SET request_hash=excluded.request_hash,status_code=excluded.status_code,
	 response_body=excluded.response_body,expires_at=excluded.expires_at,is_delete=false,update_time=statement_timestamp()
	 WHERE infra.idempotency_record.expires_at<=statement_timestamp() OR infra.idempotency_record.is_delete`,
		uuid.New(), actor, key.String(), fingerprint, string(body))
	if result.Error != nil {
		return fmt.Errorf("record prompt preference receipt: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return application.ErrIdempotencyConflict
	}
	return nil
}
