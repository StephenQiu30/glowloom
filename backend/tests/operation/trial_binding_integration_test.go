package operation_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/trial"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestTrialBindingLoadsDraftWithoutProjectReleaseOrPriceAndHonorsRevocation(t *testing.T) {
	db := operationStoreDB(t)
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"}
	require.NoError(t, db.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password,platform_role) VALUES(?,?,?,'Trial Admin','producer','synthetic',false,'platform_admin')`, actor.ID, actor.OrgID, "trial-"+actor.ID.String()).Error)
	actor = identityfixture.Session(t, db, actor)
	catalog := app.NewCatalogStore(db)
	presets := catalogapp.NewProviderPresets(catalog)
	list, err := presets.List(t.Context(), actor)
	require.NoError(t, err)
	var base catalogdomain.Preset
	for _, p := range list {
		if p.Document.Config.AdapterKey == "volcengine_ark" && p.Origin == "builtin" {
			base = p
		}
	}
	require.NotEqual(t, uuid.Nil, base.ID)
	custom, err := presets.Save(t.Context(), actor, catalogapp.SavePresetInput{Slug: "trial-" + uuid.NewString(), ParentID: &base.ID, RequestID: uuid.New(), Document: base.Document, PreviewHash: base.Document.Digest()})
	require.NoError(t, err)
	connection, err := presets.Connect(t.Context(), actor, custom.ID, catalogapp.ConnectPresetInput{VersionID: custom.VersionID, Key: "trial-" + uuid.NewString(), Name: "Trial", RequestID: uuid.New()})
	require.NoError(t, err)
	keyID := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO catalog.provider_credential(id,provider_id,label,last4,ciphertext,key_id,status) VALUES(?,?,'fixture','test',decode('abcd','hex'),'fixture','active')`, keyID, connection.ProviderID).Error)
	in := application.TrialInput{Actor: actor, Request: application.TrialRequest{ProviderID: connection.ProviderID}}
	store := trial.NewStore(db, catalog)
	binding, err := store.Binding(t.Context(), in)
	require.NoError(t, err)
	require.Equal(t, connection.Config.ID, binding.ConfigID)
	require.Equal(t, keyID, binding.CredentialID)
	require.NoError(t, db.Exec(`UPDATE catalog.provider_credential SET status='draining' WHERE id=?`, keyID).Error)
	_, err = store.Binding(t.Context(), in)
	require.Error(t, err)
	in.ConfigVersionID = binding.ConfigID
	in.CredentialID = binding.CredentialID
	_, err = store.Binding(t.Context(), in)
	require.NoError(t, err)
	require.NoError(t, db.Exec(`UPDATE catalog.provider_preset SET emergency_disabled=true WHERE id=?`, custom.ID).Error)
	_, err = store.Binding(t.Context(), in)
	require.Error(t, err)
	require.NoError(t, db.Exec(`UPDATE identity."user" SET platform_role='creator' WHERE id=?`, actor.ID).Error)
	require.ErrorIs(t, store.Authorize(t.Context(), actor), identityapp.ErrForbidden)
}
