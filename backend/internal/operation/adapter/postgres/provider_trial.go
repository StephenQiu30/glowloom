package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

type trialBinding struct {
	ReleaseID uuid.UUID
	Input     application.ProviderTrialQuote
}

// CreateProviderTrialQuote uses the same input validator, quote persistence,
// confirmation, budget and workflow as ordinary creation. It never sends a request.
func (s *Store) CreateProviderTrialQuote(ctx context.Context, actor identityapp.Principal, releaseID uuid.UUID, input application.ProviderTrialQuote) (application.CreateFreeQuoteResult, error) {
	if !catalogapp.IsPlatformAdmin(actor) {
		return application.CreateFreeQuoteResult{}, identityapp.ErrForbidden
	}
	if input.RequestID == uuid.Nil || releaseID == uuid.Nil || input.ProjectID == uuid.Nil || input.MaxChargeMicros < 0 || input.SupplierMaxMicros < 0 || input.SupplierMaxMicros > 1_000_000_000 || (input.SupplierCurrency != "CNY" && input.SupplierCurrency != "USD") {
		return application.CreateFreeQuoteResult{}, application.ErrInvalidFreeQuote
	}
	raw, err := json.Marshal(struct {
		ReleaseID uuid.UUID
		Input     application.ProviderTrialQuote
	}{releaseID, input})
	if err != nil {
		return application.CreateFreeQuoteResult{}, err
	}
	fingerprint := sha256.Sum256(raw)
	var result application.CreateFreeQuoteResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requirePlatform(tx, actor); err != nil {
			return err
		}
		if err := s.requireProject(tx, actor, input.ProjectID, true); err != nil {
			return err
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, actor.ID.String()+"/trial.quote/"+releaseID.String()+"/"+input.RequestID.String()).Error; err != nil {
			return err
		}
		if err := lockQuoteRequest(tx, actor, input.RequestID); err != nil {
			return err
		}
		project, err := lockFreeQuoteProject(tx, actor, input.ProjectID)
		if err != nil {
			return err
		}
		stored, found, err := readQuoteRequest(tx, actor, input.RequestID, fingerprint)
		if err != nil {
			return err
		}
		if found {
			return json.Unmarshal(stored, &result)
		}
		var row freeQuoteCatalogRow
		var model struct{ Key, Capability string }
		r := tx.Raw(`SELECT m.model_key AS key,m.capability FROM catalog.model_release r JOIN catalog.model_profile m ON m.id=r.model_id WHERE r.id=?`, releaseID).Scan(&model)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrFreeQuoteModelMissing
		}
		r = tx.Raw(`SELECT r.id AS release_id,m.id AS model_id,v.id AS version_id,v.provider_model_id,p.region,to_jsonb(v.modes)::text AS modes,to_jsonb(cap.input_roles)::text AS input_roles,
   v.limits::text AS limits,v.param_schema::text AS param_schema,price.id AS price_id,price.version_no AS price_version_no,
   price.unit AS price_unit,price.rule::text AS price_rule,price.currency::text AS price_currency,price.fx_rate_to_cny::text AS price_fx_rate_to_cny,price.effective_from AS price_effective_from
   FROM catalog.model_release r JOIN catalog.model_profile m ON m.id=r.model_id
   JOIN catalog.model_profile_version v ON v.id=r.model_version_id JOIN catalog.price_rule_version price ON price.id=r.price_version_id
   JOIN catalog.provider p ON p.id=m.provider_id JOIN catalog.capability cap ON cap.key=m.capability
   WHERE r.id=? AND r.status='draft' AND NOT m.is_delete AND NOT p.is_delete AND NOT v.is_delete AND NOT price.is_delete
   AND price.effective_from<=now() AND (? OR p.region='domestic') FOR SHARE OF m,p,r`, releaseID, project.AllowOverseasModels).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrFreeQuoteModelMissing
		}
		catalog, err := freeCatalogFromRow(row)
		if err != nil {
			return err
		}
		requested := application.CreateFreeQuoteInput{ProjectID: input.ProjectID, RequestID: input.RequestID.String(), FreeQuoteItemInput: application.FreeQuoteItemInput{ModelKey: model.Key, Capability: model.Capability, Mode: input.Mode, Prompt: input.Prompt, Params: input.Params, OutputCount: 1, ForceRegenerate: true}}
		if len(requested.Params) == 0 {
			requested.Params = json.RawMessage(`{}`)
		}
		for _, m := range input.MediaInputs {
			requested.MediaInputs = append(requested.MediaInputs, application.FreeQuoteMediaInput{Role: m.Role, MediaAssetID: m.MediaAssetID})
		}
		media, err := s.readQuoteMedia(tx, actor, input.ProjectID, requested.MediaInputs)
		if err != nil {
			return err
		}
		prepared, err := application.PrepareFreeQuote(requested, catalog, media, time.Now().UTC(), nil)
		if err != nil {
			return err
		}
		if *prepared.Operation.QuoteMicros > input.MaxChargeMicros {
			return application.ErrInvalidFreeQuote
		}
		if err := s.withDB(tx).CreateQuoteSnapshot(ctx, actor, nil, []QuoteItem{{Operation: prepared.Operation, Inputs: prepared.Inputs, trial: &trialBinding{ReleaseID: releaseID, Input: input}}}); err != nil {
			return err
		}
		budget, err := s.billingStore(tx).FindBudget(ctx, actor, input.ProjectID)
		if err != nil {
			return err
		}
		available, err := budget.AvailableMicros()
		if err != nil {
			return err
		}
		result = application.CreateFreeQuoteResult{OperationID: prepared.Operation.ID, QuoteMicros: *prepared.Operation.QuoteMicros, QuoteDetail: prepared.Operation.QuoteDetail, AvailableMicros: available, ExpiresAt: *prepared.Operation.QuoteExpiresAt, Confirmable: available >= *prepared.Operation.QuoteMicros}
		if err := insertQuoteRequest(tx, actor, input.RequestID, fingerprint, result); err != nil {
			return err
		}
		receipt, err := json.Marshal(map[string]string{"operation_id": result.OperationID.String(), "project_id": input.ProjectID.String()})
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO catalog.config_request(actor_id,action,target_id,request_id,fingerprint,result) VALUES(?,'trial.quote',?,?,?,?::jsonb)`, actor.ID, releaseID, input.RequestID, hex.EncodeToString(fingerprint[:]), string(receipt)).Error
	})
	return result, err
}

func freezeTrialRelease(tx *gorm.DB, actor identityapp.Principal, op domain.Operation, trial trialBinding) error {
	if !catalogapp.IsPlatformAdmin(actor) {
		return identityapp.ErrForbidden
	}
	var present int
	r := tx.Raw(`SELECT 1 FROM identity."user" actor JOIN catalog.model_release r ON r.id=?
 JOIN catalog.model_profile m ON m.id=r.model_id JOIN catalog.provider p ON p.id=m.provider_id
 JOIN catalog.provider_config_version config ON config.id=r.config_version_id
 JOIN catalog.provider_preset_version pv ON pv.id=config.preset_version_id JOIN catalog.provider_preset preset ON preset.id=pv.preset_id
 JOIN catalog.provider_credential key ON key.provider_id=p.id AND key.status='active' AND NOT key.is_delete
 WHERE actor.id=? AND actor.platform_role='platform_admin' AND actor.status='active' AND NOT actor.is_delete
 AND r.status='draft' AND r.model_version_id=? AND r.price_version_id=? AND NOT preset.emergency_disabled
 AND r.upstream_cost->>'currency'=? AND (r.upstream_cost->>'max_micros')::bigint<=?
 FOR SHARE OF actor,preset,key`, trial.ReleaseID, actor.ID, op.ModelProfileVersionID, op.PriceRuleVersionID, trial.Input.SupplierCurrency, trial.Input.SupplierMaxMicros).Scan(&present)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return application.ErrFreeQuoteModelMissing
	}
	if err := tx.Exec(`INSERT INTO operation.provider_binding(operation_id,release_id) VALUES(?,?)`, op.ID, trial.ReleaseID).Error; err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO operation.provider_trial(operation_id,actor_id,max_charge_micros,supplier_max_micros,supplier_currency) VALUES(?,?,?,?,?)`, op.ID, actor.ID, trial.Input.MaxChargeMicros, trial.Input.SupplierMaxMicros, trial.Input.SupplierCurrency).Error
}

func currentTrialCatalog(tx *gorm.DB, actor identityapp.Principal, op domain.Operation, allowOverseas bool) (bool, *uuid.UUID, *uuid.UUID, error) {
	var trial struct{ ActorID uuid.UUID }
	r := tx.Raw(`SELECT actor_id FROM operation.provider_trial WHERE operation_id=?`, op.ID).Scan(&trial)
	if r.Error != nil {
		return false, nil, nil, r.Error
	}
	if r.RowsAffected == 0 {
		return false, nil, nil, nil
	}
	if trial.ActorID != actor.ID || !catalogapp.IsPlatformAdmin(actor) {
		return true, nil, nil, nil
	}
	var row struct{ ModelID, PriceID uuid.UUID }
	r = tx.Raw(`SELECT r.model_version_id AS model_id,r.price_version_id AS price_id
 FROM operation.provider_binding b JOIN catalog.model_release r ON r.id=b.release_id
 JOIN catalog.model_profile m ON m.id=r.model_id JOIN catalog.provider p ON p.id=m.provider_id
 JOIN catalog.provider_config_version c ON c.id=r.config_version_id JOIN catalog.provider_preset_version pv ON pv.id=c.preset_version_id
 JOIN catalog.provider_preset preset ON preset.id=pv.preset_id
 JOIN identity."user" actor ON actor.id=? AND actor.platform_role='platform_admin' AND actor.status='active' AND NOT actor.is_delete
 JOIN catalog.provider_credential key ON key.provider_id=p.id AND key.status='active' AND NOT key.is_delete
 WHERE b.operation_id=? AND r.status='draft' AND NOT m.is_delete AND NOT p.is_delete AND NOT preset.emergency_disabled
 AND (? OR p.region='domestic') FOR SHARE OF actor,m,p,r,key,preset`, actor.ID, op.ID, allowOverseas).Scan(&row)
	if r.Error != nil {
		return true, nil, nil, r.Error
	}
	if r.RowsAffected != 1 {
		return true, nil, nil, nil
	}
	return true, &row.ModelID, &row.PriceID, nil
}
