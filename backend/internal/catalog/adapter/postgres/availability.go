package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

// readModelAvailability reads bounded batches inside each caller's authorized snapshot.
// Read-only probes never contribute to verified_modes.
func readModelAvailability(tx *gorm.DB, ids []uuid.UUID) (map[uuid.UUID]application.ModelAvailability, error) {
	result := make(map[uuid.UUID]application.ModelAvailability, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var rows []struct {
		ID                                                                       uuid.UUID
		ReleaseID, ModelVersionID, PriceVersionID, ConfigVersionID, CredentialID *uuid.UUID
		Enabled, CredentialReady, PriceReady, VersionsCurrent                    bool
		ModeContractReady                                                        bool
		Modes, Verified                                                          []byte
		Config                                                                   []byte
		Capability                                                               string
	}
	err := tx.Raw(`SELECT m.id,m.capability,cfg.config,COALESCE(v.modes <@ cap.modes AND cardinality(v.modes)>0,false) AS mode_contract_ready,r.id AS release_id,r.model_version_id,r.price_version_id,r.config_version_id,c.id AS credential_id,
 (m.status='active' AND p.status='active' AND NOT p.is_delete AND NOT COALESCE(preset.emergency_disabled,false)) AS enabled,
 (c.id IS NOT NULL AND COALESCE(c.last_test_result NOT IN ('auth_failed'),true)) AS credential_ready,
 (price.id IS NOT NULL AND price.effective_from<=transaction_timestamp() AND price.source_url IS NOT NULL AND price.reviewed_at<=transaction_timestamp()
 AND procurement.id IS NOT NULL AND procurement.effective_from<=transaction_timestamp() AND procurement.source_url IS NOT NULL AND procurement.reviewed_at<=transaction_timestamp()) AS price_ready,
 COALESCE((p.current_config_version_id=r.config_version_id AND m.current_version_id=r.model_version_id),false) AS versions_current,
 COALESCE(to_jsonb(v.modes),'[]'::jsonb) AS modes,
 COALESCE((SELECT jsonb_agg(DISTINCT x.mode) FROM catalog.model_validation x
 JOIN operation.operation op ON op.id=x.operation_id AND op.status='completed' AND NOT op.is_delete
 JOIN operation.provider_binding b ON b.operation_id=op.id AND b.release_id=r.id
 JOIN operation.provider_credential_binding k ON k.operation_id=op.id AND k.credential_id=c.id
 WHERE x.release_id=r.id AND x.credential_id=c.id AND x.status='succeeded' AND x.adapter_version='ai-provider-v1'
 AND x.evidence->>'kind'='real_provider' AND op.mode=x.mode AND op.model_profile_version_id=r.model_version_id
 AND op.price_rule_version_id=r.price_version_id),'[]'::jsonb) AS verified
 FROM catalog.model_profile m JOIN catalog.provider p ON p.id=m.provider_id
 LEFT JOIN catalog.capability cap ON cap.key=m.capability AND NOT cap.is_delete
 LEFT JOIN catalog.model_release r ON r.id=m.active_release_id AND r.model_id=m.id AND r.status='active'
 LEFT JOIN catalog.model_profile_version v ON v.id=COALESCE(r.model_version_id,m.current_version_id) AND NOT v.is_delete
 LEFT JOIN catalog.provider_config_version cfg ON cfg.id=r.config_version_id AND cfg.provider_id=p.id
 LEFT JOIN catalog.provider_preset_version pv ON pv.id=cfg.preset_version_id
 LEFT JOIN catalog.provider_preset preset ON preset.id=pv.preset_id
 LEFT JOIN catalog.provider_credential c ON c.provider_id=p.id AND c.status='active' AND NOT c.is_delete
 LEFT JOIN catalog.price_rule_version price ON price.id=r.price_version_id AND price.model_profile_id=m.id AND NOT price.is_delete
 LEFT JOIN catalog.price_rule_version procurement ON procurement.id::text=price.rule->>'procurement_price_version_id' AND procurement.model_profile_id=m.id AND NOT procurement.is_delete
 WHERE m.id IN ? AND NOT m.is_delete`, ids).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("read model availability: %w", err)
	}
	for _, row := range rows {
		f := application.ModelAvailabilityFacts{Enabled: row.Enabled, CredentialReady: row.CredentialReady, PriceReady: row.PriceReady, VersionsCurrent: row.VersionsCurrent}
		if row.ReleaseID != nil {
			f.ReleaseID = *row.ReleaseID
		}
		if row.ModelVersionID != nil {
			f.ModelVersionID = *row.ModelVersionID
		}
		if row.PriceVersionID != nil {
			f.PriceVersionID = *row.PriceVersionID
		}
		if row.ConfigVersionID != nil {
			f.ConfigVersionID = *row.ConfigVersionID
		}
		if row.CredentialID != nil {
			f.CredentialID = *row.CredentialID
		}
		if json.Unmarshal(row.Modes, &f.Modes) != nil || json.Unmarshal(row.Verified, &f.VerifiedModes) != nil {
			return nil, application.ErrInvalidModelCatalog
		}
		if f.ReleaseID != uuid.Nil {
			err := validateReleaseContract(tx, row.ID, application.CreateReleaseInput{ModelVersionID: f.ModelVersionID, PriceVersionID: f.PriceVersionID, ConfigVersionID: f.ConfigVersionID})
			if err != nil && !errors.Is(err, application.ErrModelSourceUnavailable) {
				return nil, err
			}
			var config domain.ProviderConfig
			f.ContractCompatible = row.ModeContractReady && err == nil && json.Unmarshal(row.Config, &config) == nil && config.Validate() == nil && slices.Contains(config.Capabilities, strings.TrimSuffix(row.Capability, ".generate"))
		}
		result[row.ID] = application.ProjectModelAvailability(f)
	}
	return result, nil
}

// AvailableForNewExecution shares the list projection with authorized operation
// transactions. The caller owns project access; this reader never starts a
// transaction or treats a credential check as execution evidence.
func (s *Store) AvailableForNewExecution(ctx context.Context, versionID, releaseID uuid.UUID, mode string) (bool, error) {
	if s == nil || s.db == nil || versionID == uuid.Nil || releaseID == uuid.Nil || mode == "" {
		return false, nil
	}
	if _, ok := s.db.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return false, application.ErrInvalidModelCatalog
	}
	tx := s.db.WithContext(ctx)
	var row struct{ ID uuid.UUID }
	read := tx.Raw(`SELECT m.id FROM catalog.model_profile m
 JOIN catalog.model_profile_version v ON v.model_profile_id=m.id AND v.id=?
 JOIN catalog.model_release r ON r.id=m.active_release_id AND r.id=? AND r.model_id=m.id
 JOIN catalog.provider p ON p.id=m.provider_id
 JOIN catalog.provider_credential c ON c.provider_id=p.id AND c.status='active' AND NOT c.is_delete
 JOIN catalog.provider_config_version cfg ON cfg.id=r.config_version_id AND cfg.provider_id=p.id
 JOIN catalog.provider_preset_version pv ON pv.id=cfg.preset_version_id
 JOIN catalog.provider_preset preset ON preset.id=pv.preset_id
 WHERE NOT m.is_delete FOR SHARE OF preset,p,m,r,c`, versionID, releaseID).Scan(&row)
	if read.Error != nil {
		return false, read.Error
	}
	if read.RowsAffected != 1 {
		return false, nil
	}
	states, err := readModelAvailability(tx, []uuid.UUID{row.ID})
	if err != nil {
		return false, err
	}
	state := states[row.ID]
	return state.ModelVersionID == versionID && state.ReleaseID == releaseID && state.AvailableForMode(mode), nil
}
