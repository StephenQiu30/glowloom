// Package trial executes bounded local administrator experiments without billing.
package trial

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// Authorizer checks the current administrator session at every boundary.
type Authorizer interface {
	AuthorizeDevelopmentTrial(context.Context, identityapp.Principal) error
}

// Binding freezes provider configuration and the encrypted worker credential.
type Binding struct {
	Config                 catalogdomain.ProviderConfig
	ConfigID, CredentialID uuid.UUID
	KeyID                  string
	Ciphertext             []byte
}

// Store loads administrator-authorized provider bindings.
type Store struct {
	db   *gorm.DB
	auth Authorizer
}

// NewStore injects the database and current-session authority.
func NewStore(db *gorm.DB, auth Authorizer) *Store { return &Store{db: db, auth: auth} }

// Authorize rechecks current authority before trial execution.
func (s *Store) Authorize(ctx context.Context, actor identityapp.Principal) error {
	return s.auth.AuthorizeDevelopmentTrial(ctx, actor)
}

// Binding reads the original config and key for queries, or current versions for submission.
func (s *Store) Binding(ctx context.Context, i application.TrialInput) (Binding, error) {
	if err := s.Authorize(ctx, i.Actor); err != nil {
		return Binding{}, err
	}
	var row struct {
		Config                 []byte
		ConfigID, CredentialID uuid.UUID
		KeyID                  string
		Ciphertext             []byte
	}
	query := `SELECT c.config,c.id config_id,k.id credential_id,k.key_id,k.ciphertext FROM catalog.provider p
 JOIN catalog.provider_config_version c ON c.provider_id=p.id
 JOIN catalog.provider_credential k ON k.provider_id=p.id AND NOT k.is_delete
 LEFT JOIN catalog.provider_preset_version pv ON pv.id=c.preset_version_id
 LEFT JOIN catalog.provider_preset preset ON preset.id=pv.preset_id
 WHERE p.id=? AND NOT p.is_delete AND p.adapter_key IN ('volcengine_ark','byteplus')
 AND NOT COALESCE(preset.emergency_disabled,false)`
	args := []any{i.Request.ProviderID}
	if i.ConfigVersionID == uuid.Nil {
		query += ` AND c.id=p.current_config_version_id AND k.status='active' ORDER BY k.create_time DESC,k.id DESC LIMIT 1`
	} else {
		query += ` AND c.id=? AND k.id=? AND k.status IN ('active','draining')`
		args = append(args, i.ConfigVersionID, i.CredentialID)
	}
	result := s.db.WithContext(ctx).Raw(query, args...).Scan(&row)
	if result.Error != nil {
		return Binding{}, fmt.Errorf("load trial binding: %w", result.Error)
	}
	var config catalogdomain.ProviderConfig
	if result.RowsAffected != 1 || json.Unmarshal(row.Config, &config) != nil || config.Validate() != nil {
		return Binding{}, application.ErrInvalidTrial
	}
	return Binding{Config: config, ConfigID: row.ConfigID, CredentialID: row.CredentialID, KeyID: row.KeyID, Ciphertext: row.Ciphertext}, nil
}
