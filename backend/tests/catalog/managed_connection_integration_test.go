package catalog_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/paramvalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/pricevalidation"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestManagedConnectionKeyRotationAndReleaseGate(t *testing.T) {
	dsn := os.Getenv("LV_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("requires isolated catalog database")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	actor := adminPrincipal()
	require.NoError(t, conn.DB.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password,platform_role) VALUES(?,?,?,'Managed admin','producer','synthetic',false,'platform_admin')`, actor.ID, actor.OrgID, "managed-"+actor.ID.String()).Error)
	actor = identityfixture.Session(t, conn.DB, actor)
	store := app.NewCatalogStore(conn.DB)
	presets := catalogapp.NewProviderPresets(store)
	connection, err := presets.Connect(t.Context(), actor, uuid.MustParse("6da07a10-0000-4000-8000-000000000002"), catalogapp.ConnectPresetInput{VersionID: uuid.MustParse("6da07a11-0000-4000-8000-000000000002"), Key: "managed-" + uuid.NewString(), Name: "Managed", RequestID: uuid.New()})
	require.NoError(t, err)
	command := newCredentialCommand(t, store)
	firstInput := catalogapp.SetCredentialInput{ProviderID: connection.ProviderID, Label: "platform", Secret: json.RawMessage(`{"api_key":"fixture-only-first-key"}`), RequestID: uuid.NewString()}
	first, err := command.Execute(t.Context(), actor, firstInput)
	require.NoError(t, err)
	replay, err := command.Execute(t.Context(), actor, firstInput)
	require.NoError(t, err)
	sameReceipt(t, first, replay)
	changed := firstInput
	changed.Secret = json.RawMessage(`{"api_key":"fixture-only-other-key"}`)
	_, err = command.Execute(t.Context(), actor, changed)
	require.ErrorIs(t, err, catalogapp.ErrPresetConflict)
	changed.RequestID = uuid.NewString()
	second, err := command.Execute(t.Context(), actor, changed)
	require.NoError(t, err)
	replay, err = command.Execute(t.Context(), actor, firstInput)
	require.NoError(t, err)
	sameReceipt(t, first, replay)
	history, err := catalogapp.NewCredentialHistory(store).List(t.Context(), actor, connection.ProviderID)
	require.NoError(t, err)
	require.Len(t, history, 2)
	require.Equal(t, domain.CredentialDraining, history[1].Status)
	disable := catalogapp.DisableCredentialInput{ProviderID: connection.ProviderID, CredentialID: first.ID, RequestID: uuid.NewString()}
	_, err = catalogapp.NewDisableCredentialCommand(store, time.Now).Execute(t.Context(), actor, disable)
	require.NoError(t, err)
	_, err = catalogapp.NewDisableCredentialCommand(store, time.Now).Execute(t.Context(), actor, disable)
	require.NoError(t, err)
	var secretCount int64
	require.NoError(t, conn.DB.Raw(`SELECT count(*) FROM catalog.config_request WHERE actor_id=? AND (result::text LIKE '%fixture-only%' OR result::text LIKE '%ciphertext%')`, actor.ID).Scan(&secretCount).Error)
	require.Zero(t, secretCount)
	checks := catalogapp.NewRequestCredentialTestCommand(store)
	check, err := checks.Execute(t.Context(), actor, connection.ProviderID, second.ID, uuid.NewString())
	require.NoError(t, err)
	require.NoError(t, store.FailProviderCheck(t.Context(), check.CheckID, actor.ID, connection.ProviderID, second.ID))
	savedCheck, err := store.ReadProviderCheck(t.Context(), actor, check.CheckID)
	require.NoError(t, err)
	require.Equal(t, "failed", savedCheck.Status)
	create := catalogapp.NewCreateModelCommand(store, time.Now)
	createInput := catalogapp.CreateModelInput{Key: "managed-" + uuid.NewString(), ProviderID: connection.ProviderID, Capability: "video.generate", DisplayName: "Fixture video", RequestID: uuid.NewString()}
	model, err := create.Execute(t.Context(), actor, createInput)
	require.NoError(t, err)
	repeated, err := create.Execute(t.Context(), actor, createInput)
	require.NoError(t, err)
	sameReceipt(t, model, repeated)
	validator, err := paramvalidation.NewValidator()
	require.NoError(t, err)
	versionInput := catalogapp.PublishModelVersionInput{ModelID: model.ID, ExpectedRevision: model.Revision, VersionNo: 1, ProviderModelID: "fixture/video", Modes: []string{"text2video"}, Limits: json.RawMessage(`{"max_outputs":1}`), ParamSchema: json.RawMessage(`[]`), SupportsQuery: true, ExpectedMaxMS: 600000, Moderation: domain.ModerationProvider, Queue: "provider", RequestID: uuid.NewString()}
	versions := catalogapp.NewPublishModelVersionCommand(store, validator, time.Now)
	version, err := versions.Execute(t.Context(), actor, versionInput)
	require.NoError(t, err)
	repeatedVersion, err := versions.Execute(t.Context(), actor, versionInput)
	require.NoError(t, err)
	sameReceipt(t, version, repeatedVersion)
	priceValidator, err := pricevalidation.NewValidator()
	require.NoError(t, err)
	prices := catalogapp.NewPublishPriceRuleCommand(store, priceValidator, time.Now)
	priceInput := catalogapp.PublishPriceRuleInput{ModelID: model.ID, ExpectedRevision: version.Revision, VersionNo: 1, Unit: domain.PricePerRequest, Rule: json.RawMessage(`{"base_micros":1000}`), Currency: "CNY", EffectiveFrom: time.Now().Add(-time.Hour), RequestID: uuid.NewString()}
	price, err := prices.Execute(t.Context(), actor, priceInput)
	require.NoError(t, err)
	repeatedPrice, err := prices.Execute(t.Context(), actor, priceInput)
	require.NoError(t, err)
	sameReceipt(t, price, repeatedPrice)
	releases := catalogapp.NewModelReleases(store)
	release, err := releases.Create(t.Context(), actor, model.ID, catalogapp.CreateReleaseInput{ModelVersionID: version.ID, PriceVersionID: price.ID, ConfigVersionID: connection.Config.ID, RequestID: uuid.New(), UpstreamCost: catalogapp.UpstreamCost{Currency: "USD", MaxMicros: 100000, Source: "fixture only", ReviewedAt: time.Now()}})
	require.NoError(t, err)
	require.Equal(t, []string{"text2video"}, release.RequiredModes)
	require.Empty(t, release.ValidatedModes)
	_, err = releases.Activate(t.Context(), actor, release.ID, catalogapp.ActivateReleaseInput{ExpectedRevision: release.Revision, ExpectedModelRevision: price.Revision, EnableProvider: true, RequestID: uuid.New()})
	require.ErrorIs(t, err, catalogapp.ErrReleaseUnverified)
	_, err = catalogapp.NewSetModelStatusCommand(store, time.Now).Execute(t.Context(), actor, catalogapp.SetModelStatusInput{ModelID: model.ID, ExpectedRevision: price.Revision, Status: domain.ModelActive, RequestID: uuid.NewString()})
	require.ErrorIs(t, err, catalogapp.ErrReleaseUnverified)
	var providerStatus string
	require.NoError(t, conn.DB.Raw(`SELECT status FROM catalog.provider WHERE id=?`, connection.ProviderID).Scan(&providerStatus).Error)
	require.Equal(t, "disabled", providerStatus)
	// A database fixture pins a published release to test consumer isolation;
	// this is not supplier evidence and never goes through the activation API.
	require.NoError(t, conn.DB.Exec(`UPDATE catalog.provider SET status='active' WHERE id=?`, connection.ProviderID).Error)
	require.NoError(t, conn.DB.Exec(`UPDATE catalog.model_release SET status='active' WHERE id=?`, release.ID).Error)
	require.NoError(t, conn.DB.Exec(`UPDATE catalog.model_profile SET status='active',active_release_id=? WHERE id=?`, release.ID, model.ID).Error)
	versionInput.RequestID = uuid.NewString()
	versionInput.ExpectedRevision = price.Revision
	versionInput.VersionNo = 2
	versionInput.ProviderModelID = "fixture/next-video"
	draftVersion, err := versions.Execute(t.Context(), actor, versionInput)
	require.NoError(t, err)
	priceInput.RequestID = uuid.NewString()
	priceInput.ExpectedRevision = draftVersion.Revision
	priceInput.VersionNo = 2
	priceInput.Rule = json.RawMessage(`{"base_micros":9000}`)
	_, err = prices.Execute(t.Context(), actor, priceInput)
	require.NoError(t, err)
	org := insertModelCatalogOrganization(t.Context(), t, conn.DB)
	creator := insertModelCatalogActor(t.Context(), t, conn.DB, org)
	project := insertModelCatalogProject(t.Context(), t, conn.DB, creator, true)
	page, err := store.ListModelsForProject(t.Context(), creator, catalogapp.ListModelsInput{ProjectID: project, Capability: "video.generate", Limit: 200})
	require.NoError(t, err)
	found := false
	for _, item := range page.Models {
		if item.ID == model.ID {
			found = true
			require.NotNil(t, item.CurrentVersion)
			require.NotNil(t, item.CurrentPrice)
			require.Equal(t, version.ID, item.CurrentVersion.ID)
			require.Equal(t, price.ID, item.CurrentPrice.ID)
		}
	}
	require.True(t, found, "published model missing from creator catalog")
}

func sameReceipt(t *testing.T, a, b any) {
	t.Helper()
	left, err := json.Marshal(a)
	require.NoError(t, err)
	right, err := json.Marshal(b)
	require.NoError(t, err)
	require.JSONEq(t, string(left), string(right))
}
