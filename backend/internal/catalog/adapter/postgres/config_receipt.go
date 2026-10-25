package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ReadConfigReceipt serializes with the corresponding in-flight write and scopes
// recovery to its original actor. No secret-bearing columns are selected.
func (s *Store) ReadConfigReceipt(ctx context.Context, actor identityapp.Principal, action string, target, id uuid.UUID) (application.ConfigReceipt, error) {
	result := application.ConfigReceipt{Action: action, TargetID: target, RequestID: id}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requirePlatformCatalogAdmin(tx, s.actors, actor); err != nil {
			return err
		}

		if action == "credential.test" {
			if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, actor.ID.String()+"/"+id.String()).Error; err != nil {
				return err
			}
			var row struct{ ResponseBody []byte }
			r := tx.Raw(`SELECT response_body FROM infra.idempotency_record WHERE actor_id=? AND idem_key=? AND status_code=202 AND NOT is_delete`, actor.ID, id.String()).Scan(&row)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return application.ErrProviderNotFound
			}
			var accepted application.CredentialTestAccepted
			if err := json.Unmarshal(row.ResponseBody, &accepted); err != nil {
				return err
			}
			if accepted.ProviderID != target {
				return application.ErrProviderNotFound
			}
			data, err := json.Marshal(accepted)
			result.Result = data
			return err
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, actor.ID.String()+"/"+action+"/"+target.String()+"/"+id.String()).Error; err != nil {
			return err
		}
		var row struct{ Result []byte }
		r := tx.Raw(`SELECT result FROM catalog.config_request WHERE actor_id=? AND action=? AND target_id=? AND request_id=?`, actor.ID, action, target, id).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrProviderNotFound
		}
		result.Result = json.RawMessage(row.Result)
		return nil
	})
	return result, err
}
