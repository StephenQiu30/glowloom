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
