package catalog_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/pricevalidation"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestDeepSeekTimePricePublicationRequiresDeepSeekText(t *testing.T) {
	_, db := modelCatalogDB(t)
	org := insertModelCatalogOrganization(t.Context(), t, db)
	actor := insertModelCatalogActor(t.Context(), t, db, org)
	require.NoError(t, db.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, actor.ID).Error)
	actor = identityfixture.Session(t, db, actor)
	store := app.NewCatalogStore(db)
	provider := validProvider()
	provider.Key = "time-" + provider.ID.String()
	require.NoError(t, store.CreateProvider(t.Context(), provider))
	model := uuid.New()
	version := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO catalog.model_profile(id,model_key,provider_id,capability,display_name,status,revision) VALUES(?,?,?,'text.generate','Time Price','disabled',1)`, model, "time-"+model.String(), provider.ID).Error)
	require.NoError(t, db.Exec(`INSERT INTO catalog.model_profile_version(id,model_profile_id,version_no,provider_model_id,modes,limits,param_schema,queue,moderation,supports_query,supports_cancel,supports_callback,expected_max_ms,create_by) VALUES(?,?,1,'deepseek-flash',ARRAY['text_to_text'],'{}','[]','provider','provider',false,false,false,120000,?)`, version, model, actor.ID).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile SET current_version_id=? WHERE id=?`, version, model).Error)
	validator, err := pricevalidation.NewValidator()
	require.NoError(t, err)
	cmd := catalogapp.NewPublishPriceRuleCommand(store, validator, time.Now)
	now := time.Now().UTC()
	input := catalogapp.PublishPriceRuleInput{ModelID: model, ExpectedRevision: 1, VersionNo: 1, Unit: domain.PricePer1KTokens, Rule: json.RawMessage(`{"input_micros_per_1k":2000,"cached_input_micros_per_1k":40,"output_micros_per_1k":8000,"time_pricing":"deepseek_cn_2026"}`), Currency: "CNY", EffectiveFrom: now, SourceURL: "https://api-docs.deepseek.com/zh-cn/quick_start/pricing/", ReviewedAt: &now, RequestID: uuid.NewString()}
	_, err = cmd.Execute(t.Context(), actor, input)
	require.ErrorIs(t, err, domain.ErrInvalidPriceRule, "other providers must not inherit a DeepSeek discount")
	require.NoError(t, db.Exec(`UPDATE catalog.provider SET adapter_key='deepseek' WHERE id=?`, provider.ID).Error)
	input.RequestID = uuid.NewString()
	saved, err := cmd.Execute(t.Context(), actor, input)
	require.NoError(t, err)
	require.Equal(t, 1, saved.VersionNo)
	var count int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM catalog.price_rule_version WHERE model_profile_id=?`, model).Scan(&count).Error)
	require.Equal(t, int64(1), count)
}
