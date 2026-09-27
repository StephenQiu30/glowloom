package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/pricevalidation"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// PublishPriceRuleWithAudit rechecks current rights and commits an immutable
// price, model revision, and safe audit in one transaction.
func (s *Store) PublishPriceRuleWithAudit(ctx context.Context, actorID, orgID uuid.UUID, price domain.PriceRuleVersion, expectedRevision int64, event identityapp.OutboxEvent) (domain.PriceRuleVersion, error) {
	validator, err := pricevalidation.NewValidator()
	if err != nil {
		return domain.PriceRuleVersion{}, fmt.Errorf("prepare price validation: %w", err)
	}
	if s == nil || s.db == nil || actorID == uuid.Nil || orgID == uuid.Nil ||
		price.Validate() != nil || validator.Validate(price) != nil ||
		price.CreateBy != actorID || expectedRevision < 1 ||
		!validPriceRulePublishedAudit(actorID, orgID, price, expectedRevision, event) {
		return domain.PriceRuleVersion{}, application.ErrInvalidPublishPriceRule
	}
	var saved domain.PriceRuleVersion
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, actorID, orgID); err != nil {
			return err
		}
		if err := appendPriceRuleInTx(tx, price, expectedRevision); err != nil {
			return err
		}
		result := tx.Exec(`
			INSERT INTO infra.outbox (id, topic, partition_key, payload)
			VALUES (?::uuid, ?, ?, ?::jsonb)
		`, event.ID.String(), event.Topic, event.PartitionKey, string(event.Payload))
		if result.Error != nil {
			return fmt.Errorf("insert price audit event: %w", result.Error)
		}
		if err := requireOneRow(result, "insert price audit event"); err != nil {
			return err
		}
		var row struct {
			CreateTime time.Time
		}
		result = tx.Raw(`
			SELECT create_time FROM catalog.price_rule_version
			WHERE id = ?::uuid AND model_profile_id = ?::uuid AND NOT is_delete
		`, price.ID.String(), price.ModelID.String()).Scan(&row)
		if result.Error != nil {
			return fmt.Errorf("read published price rule: %w", result.Error)
		}
		if result.RowsAffected != 1 || row.CreateTime.IsZero() {
			return ErrPriceVersionConflict
		}
		saved = price
		saved.CreateTime = row.CreateTime
		return nil
	})
	if err != nil {
		return domain.PriceRuleVersion{}, fmt.Errorf("publish price rule transaction: %w", err)
	}
	return saved, nil
}

func appendPriceRuleInTx(tx *gorm.DB, price domain.PriceRuleVersion, expectedRevision int64) error {
	row, err := lockModelProfile(tx, price.ModelID)
	if err != nil {
		return err
	}
	if row.Revision != expectedRevision {
		return domain.ErrModelRevisionConflict
	}
	if row.CurrentVersionID == nil {
		return domain.ErrModelNotPublishable
	}
	var count int64
	if err := tx.Raw(`
		SELECT count(*) FROM catalog.price_rule_version WHERE model_profile_id = ?::uuid
	`, price.ModelID.String()).Scan(&count).Error; err != nil {
		return fmt.Errorf("count price versions: %w", err)
	}
	if int64(price.VersionNo) != count+1 {
		return ErrPriceVersionConflict
	}
	var coefficients struct {
		ByMode       map[string]json.Number `json:"by_mode"`
		ByResolution map[string]json.Number `json:"by_resolution"`
	}
	if err := json.Unmarshal(price.Rule, &coefficients); err != nil {
		return domain.ErrInvalidPriceRule
	}
	modeKeys := make([]string, 0, len(coefficients.ByMode))
	for mode := range coefficients.ByMode {
		modeKeys = append(modeKeys, mode)
	}
	resolutionKeys := make([]string, 0, len(coefficients.ByResolution))
	for resolution := range coefficients.ByResolution {
		resolutionKeys = append(resolutionKeys, resolution)
	}
	modesJSON, err := json.Marshal(modeKeys)
	if err != nil {
		return fmt.Errorf("encode price modes: %w", err)
	}
	resolutionsJSON, err := json.Marshal(resolutionKeys)
	if err != nil {
		return fmt.Errorf("encode price resolutions: %w", err)
	}
	var supported struct {
		ModesSupported       bool
		ResolutionsSupported bool
	}
	check := tx.Raw(`
		SELECT ARRAY(SELECT jsonb_array_elements_text(?::jsonb)) <@ v.modes AS modes_supported,
		       ARRAY(SELECT jsonb_array_elements_text(?::jsonb)) <@
		         ARRAY(SELECT jsonb_array_elements_text(COALESCE(v.limits->'resolutions', '[]'::jsonb))) AS resolutions_supported
		FROM catalog.model_profile_version AS v
		WHERE v.id = ?::uuid AND v.model_profile_id = ?::uuid AND NOT v.is_delete
	`, string(modesJSON), string(resolutionsJSON), row.CurrentVersionID.String(), price.ModelID.String()).Scan(&supported)
	if check.Error != nil {
		return fmt.Errorf("check price rule coefficients: %w", check.Error)
	}
	if check.RowsAffected != 1 {
		return domain.ErrModelNotPublishable
	}
	if !supported.ModesSupported || !supported.ResolutionsSupported {
		return domain.ErrInvalidPriceRule
	}
	result := tx.Exec(`
		INSERT INTO catalog.price_rule_version
		  (id, model_profile_id, version_no, unit, rule, currency,
		   fx_rate_to_cny, effective_from, create_by)
		VALUES (?::uuid, ?::uuid, ?, ?, ?::jsonb, ?, NULLIF(?, '')::numeric, ?, ?::uuid)
	`, price.ID.String(), price.ModelID.String(), price.VersionNo,
		string(price.Unit), string(price.Rule), price.Currency,
		price.FXRateToCNY, price.EffectiveFrom, price.CreateBy.String())
	if isUniqueViolation(result.Error, "uq_price_rule_version") {
		return ErrPriceVersionConflict
	}
	if result.Error != nil {
		return fmt.Errorf("insert price version: %w", result.Error)
	}
	if err := requireOneRow(result, "insert price version"); err != nil {
		return err
	}
	result = tx.Exec(`
		UPDATE catalog.model_profile
		SET revision = revision + 1, update_time = now()
		WHERE id = ?::uuid AND revision = ? AND NOT is_delete
	`, price.ModelID.String(), expectedRevision)
	if result.Error != nil {
		return fmt.Errorf("advance model price revision: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return domain.ErrModelRevisionConflict
	}
	return nil
}

type priceRulePublishedAuditEvent struct {
	EventID    uuid.UUID `json:"event_id"`
	EventType  string    `json:"event_type"`
	OccurredAt time.Time `json:"occurred_at"`
	OrgID      uuid.UUID `json:"org_id"`
	Actor      struct {
		Kind string    `json:"kind"`
		ID   uuid.UUID `json:"id"`
	} `json:"actor"`
	Aggregate struct {
		Type string    `json:"type"`
		ID   uuid.UUID `json:"id"`
	} `json:"aggregate"`
	Data struct {
		Action string `json:"action"`
		Object struct {
			Type string    `json:"type"`
			ID   uuid.UUID `json:"id"`
		} `json:"object"`
		RequestID string `json:"request_id"`
		After     struct {
			VersionID     uuid.UUID        `json:"version_id"`
			VersionNo     int              `json:"version_no"`
			Unit          domain.PriceUnit `json:"unit"`
			Currency      string           `json:"currency"`
			EffectiveFrom time.Time        `json:"effective_from"`
			Revision      int64            `json:"revision"`
		} `json:"after"`
	} `json:"data"`
}

func validPriceRulePublishedAudit(actorID, orgID uuid.UUID, price domain.PriceRuleVersion, expectedRevision int64, event identityapp.OutboxEvent) bool {
	if event.ID == uuid.Nil || event.Topic != "lanverse.audit.recorded.v1" ||
		event.PartitionKey != orgID.String() || len(event.Payload) == 0 || len(event.Payload) > 32*1024 {
		return false
	}
	var payload priceRulePublishedAuditEvent
	if !decodeCredentialEvent(event.Payload, &payload) || payload.EventID != event.ID ||
		payload.EventType != event.Topic || payload.OccurredAt.IsZero() || payload.OrgID != orgID ||
		payload.Actor.Kind != "user" || payload.Actor.ID != actorID ||
		payload.Aggregate.Type != "audit" || payload.Aggregate.ID != event.ID ||
		payload.Data.Action != "price.published" ||
		payload.Data.Object.Type != "model_profile" || payload.Data.Object.ID != price.ModelID ||
		payload.Data.After.VersionID != price.ID || payload.Data.After.VersionNo != price.VersionNo ||
		payload.Data.After.Unit != price.Unit || payload.Data.After.Currency != price.Currency ||
		!payload.Data.After.EffectiveFrom.Equal(price.EffectiveFrom) ||
		payload.Data.After.Revision != expectedRevision+1 {
		return false
	}
	requestID, err := uuid.Parse(payload.Data.RequestID)
	return err == nil && requestID != uuid.Nil && requestID.String() == payload.Data.RequestID
}

var _ application.PublishPriceRuleStore = (*Store)(nil)
