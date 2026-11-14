package catalog_test

import (
	"encoding/json"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	auditapp "github.com/StephenQiu30/glowloom-server/backend/internal/audit/application"
	pgcatalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	inbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func customProviderActor(t *testing.T, conn *gorm.DB) identity.Principal {
	t.Helper()
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"}
	require.NoError(t, conn.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password,platform_role) VALUES(?,?,?,'Custom Admin','producer','synthetic',false,'platform_admin')`, actor.ID, actor.OrgID, "custom-"+actor.ID.String()).Error)
	return identityfixture.Session(t, conn, actor)
}

func customProviderLogo(t *testing.T, conn *gorm.DB, actor identity.Principal, purpose string) uuid.UUID {
	t.Helper()
	require.NoError(t, conn.Exec(`INSERT INTO media.storage_profile(id,provider,endpoint,signing_endpoint,region,bucket,addressing_style,resource_identity) VALUES('catalog-logo-fixture','cos','http://127.0.0.1:57924','http://127.0.0.1:57924','us-east-1','catalog-logo-fixture','',repeat('b',64)) ON CONFLICT(id) DO NOTHING`).Error)
	id, location := uuid.New(), uuid.New()
	key := "personal/" + id.String() + ".png"
	require.NoError(t, conn.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state) VALUES(?,'catalog-logo-fixture','catalog-logo-fixture',?,'accepted')`, location, key).Error)
	detail, _ := json.Marshal(map[string]any{purpose: map[string]any{"version": 1, "method": "server_reencode_png"}})
	require.NoError(t, conn.Exec(`INSERT INTO media.media_asset(id,personal_org_id,personal_actor_id,kind,origin,status,object_key,object_location_id,mime_type,byte_size,width,height,codec,moderation_status,moderation_detail) VALUES(?,?,?,'image','upload','ready',?,?,'image/png',100,3,4,'png','passed',?::jsonb)`, id, actor.OrgID, actor.ID, key, location, string(detail)).Error)
	return id
}

func TestCustomProviderTransactionsAndLogoMetadata(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("requires disposable catalog database")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	actor := customProviderActor(t, conn.DB)
	other := customProviderActor(t, conn.DB)
	store := app.NewCatalogStore(conn.DB)
	service := catalog.NewCustomProviders(store)
	logo := customProviderLogo(t, conn.DB, actor, "provider_logo")
	input := catalog.CustomProviderInput{RequestID: uuid.New(), Name: "自定义供应商", LogoAssetID: &logo, Config: domain.ProviderConfig{AdapterKey: "openai", BaseURL: "https://gateway.example/v1", Region: domain.RegionOverseas, Capabilities: []string{"text"}, TimeoutSeconds: 60, ConcurrencyLimit: 10, RateLimitPerMin: 60}}
	var wg sync.WaitGroup
	results := make([]catalog.PresetConnection, 2)
	errs := make([]error, 2)
	for i := range 2 {
		wg.Go(func() { results[i], errs[i] = service.Create(t.Context(), actor, input) })
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, results[0], results[1])
	created := results[0]
	var recorded []struct {
		ID                  uuid.UUID
		Topic, PartitionKey string
		Payload             []byte
	}
	require.NoError(t, conn.DB.Raw(`SELECT id,topic,partition_key,payload FROM infra.outbox WHERE topic='glowloom.audit.recorded.v1' AND payload->'data'->>'action'='provider.custom.connect' AND payload->'data'->>'request_id'=?`, input.RequestID.String()).Scan(&recorded).Error)
	require.Len(t, recorded, 1)
	event, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{Topic: recorded[0].Topic, Key: []byte(recorded[0].PartitionKey), Value: recorded[0].Payload})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, event.ID)
	require.Equal(t, recorded[0].ID, event.ID)
	require.Equal(t, "provider.custom.connect", event.Action)
	require.Equal(t, "provider", event.ObjectType)
	require.Equal(t, created.ProviderID.String(), event.ObjectID)
	require.Equal(t, input.RequestID.String(), event.RequestID)
	require.Equal(t, actor.OrgID, event.OrgID)
	require.Equal(t, &actor.ID, event.ActorID)
	require.Empty(t, event.Before)
	require.Empty(t, event.After)

	receipt, err := catalog.NewConfigReceipts(store).Read(t.Context(), actor, "provider.custom.connect", uuid.Nil, input.RequestID)
	require.NoError(t, err)
	require.NotContains(t, string(receipt.Result), "credential")
	var receiptConnection catalog.PresetConnection
	require.NoError(t, json.Unmarshal(receipt.Result, &receiptConnection))
	require.Equal(t, created, receiptConnection)
	_, err = catalog.NewConfigReceipts(store).Read(t.Context(), other, "provider.custom.connect", uuid.Nil, input.RequestID)
	require.ErrorIs(t, err, catalog.ErrProviderNotFound)

	configs := catalog.NewProviderConfigs(store)
	config, err := configs.Read(t.Context(), actor, created.ProviderID)
	require.NoError(t, err)
	require.Equal(t, input.Config, config.Config)
	require.NotNil(t, config.ModelCatalog)
	require.Empty(t, config.ModelCatalog.Models)
	require.Empty(t, config.ConfiguredModels)
	var status string
	require.NoError(t, conn.DB.Raw(`SELECT status FROM catalog.provider WHERE id=?`, created.ProviderID).Scan(&status).Error)
	require.Equal(t, "disabled", status)
	input.Name = "different"
	_, err = service.Create(t.Context(), actor, input)
	require.ErrorIs(t, err, catalog.ErrPresetConflict)
	require.NoError(t, conn.DB.Exec(`INSERT INTO catalog.provider_credential(id,provider_id,label,ciphertext,key_id,last4) VALUES(?,?,'synthetic',?,'isolated','test')`, uuid.New(), created.ProviderID, []byte("synthetic")).Error)
	model, err := configs.AddModel(t.Context(), actor, created.ProviderID, catalog.AddPresetModelInput{RequestID: uuid.New(), ConfigID: config.ID, CatalogVersion: config.ModelCatalog.Version, CustomModelName: "我的模型", CustomModelID: "gpt-fixture", Capability: "text.generate"})
	require.NoError(t, err)
	require.Equal(t, "我的模型", model.DisplayName)
	require.Equal(t, domain.ModelDisabled, model.Status)
	require.Equal(t, uuid.Nil, model.CurrentVersionID)
	var snapshot []byte
	require.NoError(t, conn.DB.Raw(`SELECT candidate_snapshot FROM catalog.model_profile WHERE id=?`, model.ID).Row().Scan(&snapshot))
	require.Contains(t, string(snapshot), "我的模型")
	require.Contains(t, string(snapshot), "gpt-fixture")
	var releases, prices int64
	require.NoError(t, conn.DB.Raw(`SELECT count(*) FROM catalog.model_release WHERE model_id=?`, model.ID).Scan(&releases).Error)
	require.NoError(t, conn.DB.Raw(`SELECT count(*) FROM catalog.price_rule_version WHERE model_profile_id=?`, model.ID).Scan(&prices).Error)
	require.Zero(t, releases)
	require.Zero(t, prices)
	adminModels := catalog.NewAdminModelsQuery(store)
	page, err := adminModels.List(t.Context(), actor, catalog.AdminModelListInput{ProviderID: created.ProviderID, Limit: 1})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, "openai", page.Items[0].ProviderAdapterKey)
	require.Equal(t, &logo, page.Items[0].ProviderLogoAssetID)
	detail, err := adminModels.Detail(t.Context(), actor, model.ID)
	require.NoError(t, err)
	require.Equal(t, "openai", detail.Model.ProviderAdapterKey)
	require.Equal(t, &logo, detail.Model.ProviderLogoAssetID)
	refs := pgcatalog.NewProviderLogoReferences(conn.DB)
	retained, err := refs.HasRetainedMediaReference(t.Context(), logo)
	require.NoError(t, err)
	require.True(t, retained)
	replacement := customProviderLogo(t, conn.DB, actor, "provider_logo")
	wrong := customProviderLogo(t, conn.DB, other, "provider_logo")
	avatar := customProviderLogo(t, conn.DB, actor, "avatar")
	for _, bad := range []uuid.UUID{wrong, avatar} {
		_, err = service.SetLogo(t.Context(), actor, created.ProviderID, catalog.ProviderLogoInput{RequestID: uuid.New(), ExpectedRevision: 1, LogoAssetID: &bad})
		require.ErrorIs(t, err, domain.ErrInvalidPreset)
	}
	patch := catalog.ProviderLogoInput{RequestID: uuid.New(), ExpectedRevision: 1, LogoAssetID: &replacement}
	updated, err := service.SetLogo(t.Context(), actor, created.ProviderID, patch)
	require.NoError(t, err)
	require.Equal(t, &replacement, updated.LogoAssetID)
	require.EqualValues(t, 2, updated.Revision)
	again, err := service.SetLogo(t.Context(), actor, created.ProviderID, patch)
	require.NoError(t, err)
	// Receipt replay must reproduce every wire field. time.Location is an
	// in-process representation detail and is not part of the JSON contract.
	updatedJSON, err := json.Marshal(updated)
	require.NoError(t, err)
	replayJSON, err := json.Marshal(again)
	require.NoError(t, err)
	require.Equal(t, updatedJSON, replayJSON)
	configAfter, err := configs.Read(t.Context(), actor, created.ProviderID)
	require.NoError(t, err)
	require.Equal(t, config.ID, configAfter.ID)
	retained, err = refs.HasRetainedMediaReference(t.Context(), logo)
	require.NoError(t, err)
	require.False(t, retained)
	updated, err = service.SetLogo(t.Context(), actor, created.ProviderID, catalog.ProviderLogoInput{RequestID: uuid.New(), ExpectedRevision: 2})
	require.NoError(t, err)
	require.Nil(t, updated.LogoAssetID)
	var count int64
	require.NoError(t, conn.DB.Raw(`SELECT count(*) FROM media.media_asset WHERE id IN (?,?)`, logo, replacement).Scan(&count).Error)
	require.EqualValues(t, 2, count)
	invalid := input
	invalid.RequestID = uuid.New()
	invalid.LogoAssetID = &wrong
	invalid.Name = "atomic-invalid-logo"
	_, err = service.Create(t.Context(), actor, invalid)
	require.ErrorIs(t, err, domain.ErrInvalidPreset)
	require.NoError(t, conn.DB.Raw(`SELECT count(*) FROM catalog.provider WHERE name=?`, invalid.Name).Scan(&count).Error)
	require.Zero(t, count)
	require.NoError(t, conn.DB.Exec(`UPDATE identity."user" SET platform_role='creator' WHERE id=?`, actor.ID).Error)
	_, err = service.Create(t.Context(), actor, input)
	require.ErrorIs(t, err, identity.ErrForbidden)
}
