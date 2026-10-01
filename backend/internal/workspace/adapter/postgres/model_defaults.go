package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

type defaultsRow struct {
	ID                  uuid.UUID
	Revision            int64
	Status              string
	AllowOverseasModels bool
	DefaultModels       string
}

// ReadModelDefaults reads the same canonical map consumed by generation quotes.
func (s *Store) ReadModelDefaults(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID) (application.ModelDefaults, error) {
	if s == nil || s.db == nil {
		return application.ModelDefaults{}, ErrUnavailable
	}
	var snapshot application.ModelDefaults
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		row, err := readDefaultsRow(tx, actor.OrgID, projectID, false)
		if err != nil {
			return err
		}
		snapshot, err = row.snapshot()
		return err
	})
	if err != nil {
		return application.ModelDefaults{}, fmt.Errorf("read defaults transaction: %w", err)
	}
	return snapshot, nil
}

// SaveModelDefaults commits the project revision, audit, and durable receipt together.
func (s *Store) SaveModelDefaults(ctx context.Context, actor identityapp.Principal, input application.ModelDefaultsChange, event identityapp.OutboxEvent) (application.ModelDefaults, error) {
	if s == nil || s.db == nil {
		return application.ModelDefaults{}, ErrUnavailable
	}
	if input.ProjectID == uuid.Nil || input.IdempotencyKey == uuid.Nil || input.ExpectedRevision < 1 ||
		event.ID == uuid.Nil || event.Topic != "lanverse.audit.recorded.v1" || event.PartitionKey != input.ProjectID.String() || len(event.Payload) == 0 {
		return application.ModelDefaults{}, application.ErrInvalidModelDefaults
	}
	body, err := json.Marshal(struct {
		Contract string
		OrgID    uuid.UUID
		Project  uuid.UUID
		Revision int64
		Models   map[string]string
	}{"project.model-defaults.v1", actor.OrgID, input.ProjectID, input.ExpectedRevision, input.DefaultModels})
	if err != nil {
		return application.ModelDefaults{}, fmt.Errorf("encode defaults fingerprint: %w", err)
	}
	hash := sha256.Sum256(body)
	fingerprint := hex.EncodeToString(hash[:])
	var saved application.ModelDefaults
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		var locked int
		if err := tx.Raw(`SELECT 1 FROM pg_advisory_xact_lock(69360,hashtext(?))`, actor.ID.String()+":"+input.IdempotencyKey.String()).Scan(&locked).Error; err != nil {
			return fmt.Errorf("lock defaults request: %w", err)
		}
		row, err := readDefaultsRow(tx, actor.OrgID, input.ProjectID, true)
		if err != nil {
			return err
		}
		if row.Status != "active" {
			return domain.ErrProjectStateConflict
		}
		found, err := replayDefaults(tx, actor.ID, input.IdempotencyKey, fingerprint, &saved)
		if err != nil || found {
			return err
		}
		if row.Revision != input.ExpectedRevision {
			return domain.ErrProjectRevisionConflict
		}
		for capability, model := range input.DefaultModels {
			var present int
			result := tx.Raw(`SELECT 1 FROM catalog.model_profile m
			 JOIN catalog.provider p ON p.id=m.provider_id
			 JOIN catalog.capability cap ON cap.key=m.capability
			 JOIN catalog.model_profile_version v ON v.id=m.current_version_id AND v.model_profile_id=m.id
			 WHERE m.model_key=? AND m.capability=? AND m.status='active' AND NOT m.is_delete
			 AND p.status='active' AND NOT p.is_delete AND NOT cap.is_delete AND NOT v.is_delete
			 AND (? OR p.region='domestic') AND EXISTS(SELECT 1 FROM catalog.price_rule_version pr
			 WHERE pr.model_profile_id=m.id AND NOT pr.is_delete AND pr.effective_from<=transaction_timestamp())
			 FOR SHARE OF m,p`, model, capability, row.AllowOverseasModels).Scan(&present)
			if result.Error != nil {
				return fmt.Errorf("check default model availability: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return application.ErrDefaultModelUnavailable
			}
		}
		models, err := json.Marshal(input.DefaultModels)
		if err != nil {
			return fmt.Errorf("encode default models: %w", err)
		}
		result := tx.Exec(`UPDATE workspace.project SET default_models=?::jsonb,revision=revision+1,update_time=statement_timestamp()
		 WHERE id=? AND org_id=? AND revision=? AND status='active' AND NOT is_delete`, string(models), input.ProjectID, actor.OrgID, input.ExpectedRevision)
		if result.Error != nil {
			return fmt.Errorf("update project defaults: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return domain.ErrProjectRevisionConflict
		}
		saved = application.ModelDefaults{ProjectID: input.ProjectID, Revision: input.ExpectedRevision + 1, DefaultModels: maps.Clone(input.DefaultModels)}
		if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, event.ID, event.Topic, event.PartitionKey, string(event.Payload)).Error; err != nil {
			return fmt.Errorf("record defaults audit: %w", err)
		}
		return recordDefaults(tx, actor.ID, input.IdempotencyKey, fingerprint, saved)
	})
	if err != nil {
		return application.ModelDefaults{}, fmt.Errorf("save defaults transaction: %w", err)
	}
	return saved, nil
}

func readDefaultsRow(tx *gorm.DB, orgID, projectID uuid.UUID, write bool) (defaultsRow, error) {
	query := `SELECT id,revision,status,allow_overseas_models,default_models::text AS default_models
	 FROM workspace.project WHERE id=? AND org_id=? AND NOT is_delete`
	if write {
		query += ` FOR UPDATE`
	} else {
		query += ` FOR SHARE`
	}
	var row defaultsRow
	result := tx.Raw(query, projectID, orgID).Scan(&row)
	if result.Error != nil {
		return defaultsRow{}, fmt.Errorf("read project defaults row: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return defaultsRow{}, ErrProjectNotFound
	}
	return row, nil
}

func (r defaultsRow) snapshot() (application.ModelDefaults, error) {
	snapshot := application.ModelDefaults{ProjectID: r.ID, Revision: r.Revision}
	if err := json.Unmarshal([]byte(r.DefaultModels), &snapshot.DefaultModels); err != nil || snapshot.DefaultModels == nil {
		return application.ModelDefaults{}, application.ErrInvalidModelDefaults
	}
	return snapshot, nil
}

func replayDefaults(tx *gorm.DB, actor, key uuid.UUID, fingerprint string, saved *application.ModelDefaults) (bool, error) {
	var row struct {
		RequestHash  string
		StatusCode   int
		ResponseBody []byte
	}
	result := tx.Raw(`SELECT request_hash,status_code,response_body FROM infra.idempotency_record
	 WHERE actor_id=? AND idem_key=? AND NOT is_delete AND expires_at>statement_timestamp()`, actor, key.String()).Scan(&row)
	if result.Error != nil {
		return false, fmt.Errorf("read defaults receipt: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	if row.RequestHash != fingerprint || row.StatusCode != 200 {
		return false, application.ErrIdempotencyConflict
	}
	if err := json.Unmarshal(row.ResponseBody, saved); err != nil || saved.ProjectID == uuid.Nil || saved.DefaultModels == nil {
		return false, application.ErrInvalidModelDefaults
	}
	return true, nil
}

func recordDefaults(tx *gorm.DB, actor, key uuid.UUID, fingerprint string, saved application.ModelDefaults) error {
	body, err := json.Marshal(saved)
	if err != nil {
		return fmt.Errorf("encode defaults receipt: %w", err)
	}
	result := tx.Exec(`INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at)
	 VALUES(?,?,?,?,200,?::jsonb,statement_timestamp()+interval '24 hours')
	 ON CONFLICT(actor_id,idem_key) DO UPDATE SET request_hash=excluded.request_hash,status_code=excluded.status_code,
	 response_body=excluded.response_body,expires_at=excluded.expires_at,is_delete=false,update_time=statement_timestamp()
	 WHERE infra.idempotency_record.expires_at<=statement_timestamp() OR infra.idempotency_record.is_delete`,
		uuid.New(), actor, key.String(), fingerprint, string(body))
	if result.Error != nil {
		return fmt.Errorf("record defaults receipt: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return application.ErrIdempotencyConflict
	}
	return nil
}
