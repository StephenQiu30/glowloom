package operation_test

import (
	"testing"

	"gorm.io/gorm"

	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestManagedDispatchRejectsUnboundOperation(t *testing.T) {
	database := operationStoreDB(t)
	identity, store := seedDispatchCall(t, database, true)
	require.NoError(t, database.Exec(`UPDATE catalog.provider SET adapter_key='openrouter' WHERE id=(SELECT m.provider_id FROM catalog.model_profile m JOIN catalog.model_profile_version v ON v.model_profile_id=m.id WHERE v.id=?)`, identity.ModelProfileVersionID).Error)
	dispatch, err := store.ClaimProviderDispatch(t.Context(), identity)
	require.ErrorIs(t, err, application.ErrWorkflowModelUnavailable)
	require.False(t, dispatch.Claimed)
}

func TestManagedBindingKeepsOriginalCredentialAfterRotation(t *testing.T) {
	database := operationStoreDB(t)
	fixture := seedManagedFixture(t, database)
	identity, store := fixture.identity, fixture.store
	provider := struct{ ID uuid.UUID }{fixture.provider}
	config, credential := fixture.config, fixture.credential
	dispatch, err := store.ClaimProviderDispatch(t.Context(), identity)
	require.NoError(t, err)
	require.True(t, dispatch.Claimed)
	require.NoError(t, database.Exec(`UPDATE catalog.provider_credential SET status='draining' WHERE id=?`, credential).Error)
	replacement := uuid.New()
	require.NoError(t, database.Exec(`INSERT INTO catalog.provider_credential(id,provider_id,label,last4,ciphertext,key_id,status) VALUES(?,?,'fixture','test',decode('abcd','hex'),'test','active')`, replacement, provider.ID).Error)
	saved, err := store.LoadManagedCredential(t.Context(), identity.OperationID)
	require.NoError(t, err)
	require.Equal(t, credential, saved.CredentialID)
	require.Equal(t, config, saved.ConfigVersionID)
	dispatch, err = store.ClaimProviderDispatch(t.Context(), identity)
	require.NoError(t, err)
	require.False(t, dispatch.Claimed)
	require.NoError(t, database.Exec(`UPDATE catalog.provider_credential SET status='disabled' WHERE id=?`, credential).Error)
	_, err = store.LoadManagedCredential(t.Context(), identity.OperationID)
	require.ErrorIs(t, err, application.ErrWorkflowModelUnavailable)
}

type managedFixture struct {
	identity                              application.ProviderDispatchIdentity
	store                                 *pgoperation.Store
	provider, config, release, credential uuid.UUID
}

func seedManagedFixture(t *testing.T, database *gorm.DB) managedFixture {
	t.Helper()
	f := seedUnverifiedManagedFixture(t, database)
	seedDispatchModeEvidence(t, database, f)
	return f
}

func seedUnverifiedManagedFixture(t *testing.T, database *gorm.DB) managedFixture {
	t.Helper()
	identity, store := seedDispatchCall(t, database, true)
	var provider struct{ ID uuid.UUID }
	require.NoError(t, database.Raw(`SELECT m.provider_id AS id FROM catalog.model_profile m JOIN catalog.model_profile_version v ON v.model_profile_id=m.id WHERE v.id=?`, identity.ModelProfileVersionID).Scan(&provider).Error)
	config, release, credential := uuid.New(), uuid.New(), uuid.New()
	require.NoError(t, database.Exec(`UPDATE catalog.provider SET adapter_key='openrouter',region='overseas',status='active' WHERE id=?`, provider.ID).Error)
	require.NoError(t, database.Exec(`INSERT INTO catalog.provider_config_version(id,provider_id,preset_version_id,version_no,config,content_hash) SELECT ?,?,'6da07a11-0000-4000-8000-000000000002',1,payload->'config',content_hash FROM catalog.provider_preset_version WHERE id='6da07a11-0000-4000-8000-000000000002'`, config, provider.ID).Error)
	require.NoError(t, database.Exec(`INSERT INTO catalog.model_release(id,model_id,model_version_id,price_version_id,config_version_id,upstream_cost,status) SELECT ?,model_profile_id,?,?,?,'{}','active' FROM catalog.model_profile_version WHERE id=?`, release, identity.ModelProfileVersionID, identity.PriceRuleVersionID, config, identity.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET active_release_id=?,status='active' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, release, identity.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.provider_credential SET status='disabled' WHERE provider_id=?`, provider.ID).Error)
	require.NoError(t, database.Exec(`INSERT INTO catalog.provider_credential(id,provider_id,label,last4,ciphertext,key_id,status) VALUES(?,?,'fixture','test',decode('abcd','hex'),'test','active')`, credential, provider.ID).Error)
	require.NoError(t, database.Exec(`INSERT INTO operation.provider_binding(operation_id,release_id) VALUES(?,?)`, identity.OperationID, release).Error)
	return managedFixture{identity: identity, store: store, provider: provider.ID, config: config, release: release, credential: credential}
}

func seedDispatchModeEvidence(t *testing.T, database *gorm.DB, f managedFixture) {
	t.Helper()
	identity, release, credential := f.identity, f.release, f.credential
	// A normal dispatch fixture represents a released mode with a completed
	// synthetic validation on this credential. It never calls a live supplier.
	evidence := uuid.New()
	require.NoError(t, database.Exec(`INSERT INTO operation.operation(id,project_id,target_type,capability,mode,input_hash,origin,status,region,model_profile_version_id,price_rule_version_id)
 SELECT ?,project_id,'free',capability,mode,?, 'system','completed',region,model_profile_version_id,price_rule_version_id FROM operation.operation WHERE id=?`, evidence, uuid.NewString(), identity.OperationID).Error)
	require.NoError(t, database.Exec(`INSERT INTO operation.provider_binding(operation_id,release_id) VALUES(?,?)`, evidence, release).Error)
	require.NoError(t, database.Exec(`INSERT INTO operation.provider_credential_binding(operation_id,credential_id) VALUES(?,?)`, evidence, credential).Error)
	require.NoError(t, database.Exec(`INSERT INTO catalog.model_validation(id,release_id,credential_id,adapter_version,mode,input_digest,operation_id,status,evidence,completed_at) SELECT ?,?,?,'ai-provider-v1',mode,input_hash,id,'succeeded','{"kind":"real_provider"}',now() FROM operation.operation WHERE id=?`, uuid.New(), release, credential, evidence).Error)
}

func TestManagedDispatchEmergencyStopPreventsCredentialBinding(t *testing.T) {
	db := operationStoreDB(t)
	f := seedManagedFixture(t, db)
	preset := uuid.New()
	// Use a dedicated preset so the global built-in remains available to other fixtures.
	require.NoError(t, db.Exec(`INSERT INTO catalog.provider_preset(id,slug,origin,emergency_disabled) VALUES(?,?,'custom',true)`, preset, "test-"+preset.String()).Error)
	version := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO catalog.provider_preset_version(id,preset_id,version_no,payload,content_hash) SELECT ?,?,1,payload,content_hash FROM catalog.provider_preset_version WHERE id='6da07a11-0000-4000-8000-000000000002'`, version, preset).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.provider_config_version SET preset_version_id=? WHERE id=?`, version, f.config).Error)
	result, err := f.store.ClaimProviderDispatch(t.Context(), f.identity)
	require.ErrorIs(t, err, application.ErrWorkflowModelUnavailable)
	require.False(t, result.Claimed)
	var count int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM operation.provider_credential_binding WHERE operation_id=?`, f.identity.OperationID).Scan(&count).Error)
	require.Zero(t, count)
}

func TestRotatedKeyNeedsOwnValidationBeforePaidDispatch(t *testing.T) {
	db := operationStoreDB(t)
	f := seedManagedFixture(t, db)
	require.NoError(t, db.Exec("UPDATE catalog.provider_credential SET status='draining' WHERE id=?", f.credential).Error)
	require.NoError(t, db.Exec("INSERT INTO catalog.provider_credential(id,provider_id,label,last4,ciphertext,key_id,status) VALUES(?,?,'next','test',decode('abcd','hex'),'test','active')", uuid.New(), f.provider).Error)
	result, err := f.store.ClaimProviderDispatch(t.Context(), f.identity)
	require.ErrorIs(t, err, application.ErrWorkflowModelUnavailable)
	require.False(t, result.Claimed)
	var n int
	require.NoError(t, db.Raw("SELECT count(*) FROM operation.provider_credential_binding WHERE operation_id=?", f.identity.OperationID).Scan(&n).Error)
	require.Zero(t, n)
}

func TestRoutePoolFreezesActualBackupCredentialAndTariff(t *testing.T) {
	db := operationStoreDB(t)
	a, b := seedManagedFixture(t, db), seedManagedFixture(t, db)
	var buyer uuid.UUID
	require.NoError(t, db.Raw(`SELECT u.id FROM identity."user" u JOIN workspace.project p ON p.org_id=u.org_id WHERE p.id=? LIMIT 1`, a.identity.ProjectID).Row().Scan(&buyer))
	baseCost := uuid.Nil
	for _, f := range []managedFixture{a, b} {
		tariff := uuid.New()
		if f.provider == b.provider {
			baseCost = tariff
		}
		require.NoError(t, db.Exec(`UPDATE catalog.provider SET current_config_version_id=? WHERE id=?`, f.config, f.provider).Error)
		require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET limits='{"provider_endpoint":"verified-fixture"}' WHERE id=?`, f.identity.ModelProfileVersionID).Error)
		require.NoError(t, db.Exec(`UPDATE catalog.model_release SET upstream_cost='{"currency":"CNY","max_micros":1000}' WHERE id=?`, f.release).Error)
		require.NoError(t, db.Exec(`INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,effective_from) SELECT ?,model_profile_id,2,unit,rule,now() FROM catalog.price_rule_version WHERE id=?`, tariff, f.identity.PriceRuleVersionID).Error)
		require.NoError(t, db.Exec(`UPDATE catalog.price_rule_version SET rule=rule||jsonb_build_object('procurement_price_version_id',?::text) WHERE id=?`, tariff, f.identity.PriceRuleVersionID).Error)
	}
	// Only the backup is pinned to this aggregator endpoint.
	require.NoError(t, db.Exec("UPDATE catalog.model_profile_version SET limits='{}' WHERE id=?", a.identity.ModelProfileVersionID).Error)
	ver := uuid.New()
	require.NoError(t, db.Exec("INSERT INTO catalog.route_pool_version(id,primary_release_id,revision,created_by) VALUES(?,?,1,?)", ver, a.release, buyer).Error)
	require.NoError(t, db.Exec("INSERT INTO catalog.route_pool_member(version_id,release_id,priority) VALUES(?,?,0),(?,?,1)", ver, a.release, ver, b.release).Error)
	require.NoError(t, db.Exec("INSERT INTO operation.route_pool_binding(operation_id,version_id) VALUES(?,?)", a.identity.OperationID, ver).Error)
	require.NoError(t, db.Exec("UPDATE catalog.provider_credential SET status='disabled' WHERE id=?", a.credential).Error)
	dispatch, err := a.store.ClaimProviderDispatch(t.Context(), a.identity)
	require.NoError(t, err)
	require.True(t, dispatch.Claimed)
	got, err := a.store.LoadManagedCredential(t.Context(), a.identity.OperationID)
	require.NoError(t, err)
	require.Equal(t, b.credential, got.CredentialID)
	var actual uuid.UUID
	require.NoError(t, db.Raw("SELECT procurement_price_id FROM operation.route_delivery WHERE operation_id=?", a.identity.OperationID).Row().Scan(&actual))
	require.Equal(t, baseCost, actual)
	again, err := a.store.ClaimProviderDispatch(t.Context(), a.identity)
	require.NoError(t, err)
	require.False(t, again.Claimed)

	require.NoError(t, db.Exec("UPDATE catalog.provider_credential SET status='active' WHERE id=?", a.credential).Error)
	replaced, err := a.store.ClaimUnsentBackup(t.Context(), a.identity, b.credential)
	require.NoError(t, err)
	require.True(t, replaced)
	recovered, err := a.store.LoadManagedCredential(t.Context(), a.identity.OperationID)
	require.NoError(t, err)
	require.Equal(t, a.credential, recovered.CredentialID)
	replaced, err = a.store.ClaimUnsentBackup(t.Context(), a.identity, b.credential)
	require.NoError(t, err)
	require.False(t, replaced)
}

func TestTwoConnectionsCompeteForOneSharedAccountSlot(t *testing.T) {
	db := operationStoreDB(t)
	a, b := seedManagedFixture(t, db), seedManagedFixture(t, db)
	require.NoError(t, db.Exec("UPDATE catalog.provider SET quota_group=?,concurrency_limit=1 WHERE id IN ?", uuid.NewString(), []uuid.UUID{a.provider, b.provider}).Error)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, f := range []managedFixture{a, b} {
		go func() { <-start; _, err := f.store.ClaimProviderDispatch(t.Context(), f.identity); results <- err }()
	}
	close(start)
	first, second := <-results, <-results
	if first == nil {
		require.ErrorIs(t, second, application.ErrManagedCapacity)
	} else {
		require.ErrorIs(t, first, application.ErrManagedCapacity)
		require.NoError(t, second)
	}
}

func TestSeedreamSharesImagesPerMinuteAcrossKeys(t *testing.T) {
	db := operationStoreDB(t)
	// Use a fixture SKU with a 3-image budget to exercise the last slot.
	sku := "seedream-ipm-test-" + uuid.NewString()
	group := uuid.NewString()
	require.NoError(t, db.Exec("INSERT INTO catalog.model_image_limit VALUES('volcengine_ark',?,3)", sku).Error)
	var official int
	require.NoError(t, db.Raw("SELECT images_per_minute FROM catalog.model_image_limit WHERE adapter_key='volcengine_ark' AND provider_model_id='doubao-seedream-5-0-flash-260915'").Row().Scan(&official))
	require.Equal(t, 500, official)
	a, b := seedManagedFixture(t, db), seedManagedFixture(t, db)
	for _, f := range []managedFixture{a, b} {
		require.NoError(t, db.Exec("UPDATE catalog.provider SET quota_group=?,adapter_key='volcengine_ark',concurrency_limit=1 WHERE id=?", group, f.provider).Error)
		require.NoError(t, db.Exec("UPDATE catalog.model_profile_version SET provider_model_id=? WHERE id=?", sku, f.identity.ModelProfileVersionID).Error)
		require.NoError(t, db.Exec("UPDATE operation.operation SET output_count=2 WHERE id=?", f.identity.OperationID).Error)
	}
	first, err := a.store.ClaimProviderDispatch(t.Context(), a.identity)
	require.NoError(t, err)
	require.True(t, first.Claimed)
	_, err = b.store.ClaimProviderDispatch(t.Context(), b.identity)
	require.ErrorIs(t, err, application.ErrManagedCapacity)
	require.NoError(t, db.Exec("UPDATE operation.operation SET output_count=1 WHERE id=?", b.identity.OperationID).Error)
	second, err := b.store.ClaimProviderDispatch(t.Context(), b.identity)
	require.NoError(t, err)
	require.True(t, second.Claimed)
}
