package operation_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func TestReadCurrentQuoteCatalogTracksPublishedVersionsAndAvailability(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	store := operationStore(database)
	now := time.Now().UTC()
	quoted, modelID, providerID := seedQuoteCatalog(t, database, projectID, now)

	assertCatalog := func(wantModel, wantPrice *uuid.UUID) {
		t.Helper()
		model, price, err := store.ReadCurrentQuoteCatalog(t.Context(), actor, quoted, now)
		if err != nil || !sameUUID(model, wantModel) || !sameUUID(price, wantPrice) {
			t.Fatalf("current catalog = %v, %v, %v; want %v, %v", model, price, err, wantModel, wantPrice)
		}
	}
	assertCatalog(quoted.ModelProfileVersionID, quoted.PriceRuleVersionID)
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
			return err
		}
		model, price, err := operationStore(tx).ReadCurrentQuoteCatalog(t.Context(), actor, quoted, now)
		if err != nil {
			return err
		}
		if !sameUUID(model, quoted.ModelProfileVersionID) || !sameUUID(price, quoted.PriceRuleVersionID) {
			t.Errorf("runtime role catalog = %v, %v", model, price)
		}
		return nil
	}); err != nil {
		t.Fatalf("read catalog with runtime role: %v", err)
	}

	version2 := uuid.New()
	if err := database.Exec(`
		INSERT INTO catalog.model_profile_version
		  (id, model_profile_id, version_no, provider_model_id, modes,
		   limits, param_schema, supports_query, supports_cancel,
		   supports_callback, expected_max_ms)
		VALUES (?::uuid, ?::uuid, 2, 'quote-model-v2', ARRAY['text_to_image'],
		        '{}'::jsonb, '[]'::jsonb, true, false, false, 30000)
	`, version2.String(), modelID.String()).Error; err != nil {
		t.Fatalf("publish model version: %v", err)
	}
	if err := database.Exec(`UPDATE catalog.model_profile SET current_version_id = ?::uuid WHERE id = ?::uuid`, version2.String(), modelID.String()).Error; err != nil {
		t.Fatalf("advance current model: %v", err)
	}
	assertCatalog(&version2, quoted.PriceRuleVersionID)

	price2 := uuid.New()
	if err := database.Exec(`
		INSERT INTO catalog.price_rule_version
		  (id, model_profile_id, version_no, unit, rule, effective_from)
		VALUES (?::uuid, ?::uuid, 2, 'per_image', '{"base_micros":2}'::jsonb, ?)
	`, price2.String(), modelID.String(), now.Add(-time.Second)).Error; err != nil {
		t.Fatalf("publish newer price: %v", err)
	}
	assertCatalog(&version2, &price2)
	futurePrice := uuid.New()
	if err := database.Exec(`
		INSERT INTO catalog.price_rule_version
		  (id, model_profile_id, version_no, unit, rule, effective_from)
		VALUES (?::uuid, ?::uuid, 3, 'per_image', '{"base_micros":3}'::jsonb, ?)
	`, futurePrice.String(), modelID.String(), now.Add(time.Hour)).Error; err != nil {
		t.Fatalf("publish future price: %v", err)
	}
	assertCatalog(&version2, &price2)
	if err := database.Exec(`UPDATE catalog.model_profile SET status = 'disabled' WHERE id = ?::uuid`, modelID.String()).Error; err != nil {
		t.Fatalf("disable model: %v", err)
	}
	assertCatalog(nil, nil)
	if err := database.Exec(`UPDATE catalog.model_profile SET status = 'active' WHERE id = ?::uuid`, modelID.String()).Error; err != nil {
		t.Fatalf("restore model: %v", err)
	}

	if err := database.Exec(`UPDATE catalog.provider_credential SET status = 'disabled' WHERE provider_id = ?::uuid`, providerID.String()).Error; err != nil {
		t.Fatalf("disable credential: %v", err)
	}
	assertCatalog(nil, nil)
	if err := database.Exec(`UPDATE catalog.provider_credential SET status = 'active' WHERE provider_id = ?::uuid`, providerID.String()).Error; err != nil {
		t.Fatalf("restore credential: %v", err)
	}
	if err := database.Exec(`UPDATE catalog.provider SET status = 'disabled' WHERE id = ?::uuid`, providerID.String()).Error; err != nil {
		t.Fatalf("disable provider: %v", err)
	}
	assertCatalog(nil, nil)
	if err := database.Exec(`UPDATE catalog.provider SET status = 'active', region = 'overseas' WHERE id = ?::uuid`, providerID.String()).Error; err != nil {
		t.Fatalf("move provider region: %v", err)
	}
	assertCatalog(nil, nil)
	if err := database.Exec(`UPDATE workspace.project SET allow_overseas_models = true WHERE id = ?::uuid`, projectID.String()).Error; err != nil {
		t.Fatalf("allow overseas models: %v", err)
	}
	assertCatalog(&version2, &price2)

	foreign, _ := operationStoreProject(t, database)
	if _, _, err := store.ReadCurrentQuoteCatalog(t.Context(), foreign, quoted, now); !errors.Is(err, workspaceapp.ErrProjectNotFound) {
		t.Fatalf("foreign organization catalog read = %v", err)
	}
	if err := database.Exec(`UPDATE identity."user" SET status = 'disabled' WHERE id = ?::uuid`, actor.ID.String()).Error; err != nil {
		t.Fatalf("disable actor: %v", err)
	}
	if _, _, err := store.ReadCurrentQuoteCatalog(t.Context(), actor, quoted, now); !errors.Is(err, identityapp.ErrSessionInvalid) {
		t.Fatalf("revoked actor catalog read = %v", err)
	}
}

func seedQuoteCatalog(t *testing.T, database *gorm.DB, projectID uuid.UUID, now time.Time) (domain.Operation, uuid.UUID, uuid.UUID) {
	t.Helper()
	quoted, model, provider := seedUnfundedQuoteCatalog(t, database, projectID, now)
	fundOperationProject(t, database, projectID)
	seedFixtureProcurement(t, database, *quoted.PriceRuleVersionID, *quoted.ModelProfileVersionID)
	return quoted, model, provider
}

func seedUnfundedQuoteCatalog(t *testing.T, database *gorm.DB, projectID uuid.UUID, now time.Time) (domain.Operation, uuid.UUID, uuid.UUID) {
	t.Helper()
	providerID, modelID, versionID, priceID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	capability := "image.generate." + uuid.NewString()
	if err := database.Exec(`
		INSERT INTO catalog.provider (id, key, name, adapter_key, region)
		VALUES (?::uuid, ?, 'Quote Provider', 'quote-test', 'domestic')
	`, providerID.String(), "quote-provider-"+providerID.String()).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO catalog.provider_credential
		  (id, provider_id, label, ciphertext, key_id, last4)
		VALUES (?::uuid, ?::uuid, 'quote', ?::bytea, 'quote-test-key', '1234')
	`, uuid.NewString(), providerID.String(), []byte("test-ciphertext")).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO catalog.capability (id, key, output_type, modes, input_roles)
		VALUES (?::uuid, ?, 'image', ARRAY['text_to_image'], ARRAY['prompt'])
	`, uuid.NewString(), capability).Error; err != nil {
		t.Fatalf("create capability: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO catalog.model_profile
		  (id, model_key, provider_id, capability, display_name, status)
		VALUES (?::uuid, ?, ?::uuid, ?, 'Quote Model', 'active')
	`, modelID.String(), "quote-model-"+modelID.String(), providerID.String(), capability).Error; err != nil {
		t.Fatalf("create model profile: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO catalog.model_profile_version
		  (id, model_profile_id, version_no, provider_model_id, modes,
		   limits, param_schema, supports_query, supports_cancel,
		   supports_callback, expected_max_ms)
		VALUES (?::uuid, ?::uuid, 1, 'quote-model-v1', ARRAY['text_to_image'],
		        '{}'::jsonb, '[]'::jsonb, true, false, false, 30000)
	`, versionID.String(), modelID.String()).Error; err != nil {
		t.Fatalf("create model version: %v", err)
	}
	if err := database.Exec(`UPDATE catalog.model_profile SET current_version_id = ?::uuid WHERE id = ?::uuid`, versionID.String(), modelID.String()).Error; err != nil {
		t.Fatalf("set current model version: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO catalog.price_rule_version
		  (id, model_profile_id, version_no, unit, rule, effective_from)
		VALUES (?::uuid, ?::uuid, 1, 'per_image', '{"base_micros":1}'::jsonb, ?)
	`, priceID.String(), modelID.String(), now.Add(-time.Minute)).Error; err != nil {
		t.Fatalf("create price rule: %v", err)
	}
	quoted := quotedSnapshotItem(projectID, 1).Operation
	quoted.Capability = capability
	quoted.ModelProfileVersionID = &versionID
	quoted.PriceRuleVersionID = &priceID
	return quoted, modelID, providerID
}

func sameUUID(got, want *uuid.UUID) bool {
	return got == nil && want == nil || got != nil && want != nil && *got == *want
}

// fundOperationProject equips confirmation fixtures without raising their project budgets.
func fundOperationProject(t *testing.T, database *gorm.DB, projectID uuid.UUID) {
	t.Helper()
	var scope, lot uuid.UUID
	require.NoError(t, database.Raw(`SELECT billing.bind_project_wallet(?)`, projectID).Row().Scan(&scope))
	require.NoError(t, database.Raw(`SELECT billing.issue_promotional_credit(?,?,1000000,now()+interval '1 hour',NULL,?)`,
		scope, uuid.New(), "isolated confirmation fixture").Row().Scan(&lot))
}

// seedFixtureProcurement supplies synthetic reviewed pricing to isolated tests.
func seedFixtureProcurement(t *testing.T, db *gorm.DB, price, version uuid.UUID) {
	t.Helper()
	procurement := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,currency,fx_rate_to_cny,effective_from,is_delete)
		SELECT ?,model_profile_id,2147483647,unit,rule,currency,fx_rate_to_cny,effective_from-interval '1 hour',true
		FROM catalog.price_rule_version WHERE id=?`, procurement, price).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.price_rule_version SET
		rule=jsonb_set(rule,'{procurement_price_version_id}',to_jsonb(?::text)) WHERE id=?`, procurement, price).Error)
	var hasRelease bool
	require.NoError(t, db.Raw(`SELECT EXISTS(SELECT 1 FROM catalog.model_release WHERE price_version_id=? AND status='active')`,
		price).Row().Scan(&hasRelease))
	if hasRelease {
		return
	}
	var config uuid.UUID
	require.NoError(t, db.Raw(`INSERT INTO catalog.provider_config_version(id,provider_id,preset_version_id,version_no,config,content_hash)
		SELECT ?,m.provider_id,pv.id,1,pv.payload->'config',pv.content_hash
		FROM catalog.model_profile_version v JOIN catalog.model_profile m ON m.id=v.model_profile_id
		CROSS JOIN catalog.provider_preset_version pv
		WHERE v.id=? AND pv.id='6da07a11-0000-4000-8000-000000000002' RETURNING id`, uuid.New(), version).Row().Scan(&config))
	require.NoError(t, db.Exec(`INSERT INTO catalog.model_release(id,model_id,model_version_id,price_version_id,config_version_id,upstream_cost,status)
		SELECT ?,model_profile_id,id,?,?,'{}','active' FROM catalog.model_profile_version WHERE id=?`,
		uuid.New(), price, config, version).Error)
}
