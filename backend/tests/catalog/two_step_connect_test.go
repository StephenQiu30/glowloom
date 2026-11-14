package catalog_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

type directoryTransport func(*http.Request) (*http.Response, error)

func (transport directoryTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return transport(r)
}

func TestTwoStepModelDirectoryUsesOnlyRegisteredReadEndpoints(t *testing.T) {
	spec, ok := domain.GetAdapterSpec("openai")
	require.True(t, ok)
	config := domain.ProviderConfig{
		AdapterKey: "openai", BaseURL: spec.AllowedBaseURL, Region: spec.AllowedRegion,
		Capabilities: []string{"text"}, TimeoutSeconds: 10, ConcurrencyLimit: 1, RateLimitPerMin: 60,
	}
	calls := 0
	client := providerhttp.NewClient(&http.Client{Transport: directoryTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "https://api.openai.com/v1/models", r.URL.String())
		require.Equal(t, "Bearer directory-test-value", r.Header.Get("Authorization"))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(
			"{\"data\":[{\"id\":\"b\",\"model_type\":\"video\"},{\"id\":\"a\"},{\"id\":\"b\"},{\"id\":\"invalid id\"}]}",
		))}, nil
	})})
	ids, err := client.ReadModelIDs(t.Context(), config, "directory-test-value")
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, ids)

	config.BaseURL = "https://127.0.0.1/v1"
	_, err = client.ReadModelIDs(t.Context(), config, "directory-test-value")
	require.ErrorIs(t, err, catalogapp.ErrModelDirectoryUnsupported)
	require.Equal(t, 1, calls)
}

func TestTwoStepConnectPersistsOnlyTheConfirmedDrafts(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("requires disposable catalog database")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"}
	require.NoError(t, conn.DB.Exec(
		"INSERT INTO identity.\"user\"(id,org_id,login_name,display_name,role,password_hash,must_change_password,platform_role) VALUES(?,?,?,'Directory Test','producer','synthetic',false,'platform_admin')",
		actor.ID, actor.OrgID, "directory-"+actor.ID.String(),
	).Error)
	actor = identityfixture.Session(t, conn.DB, actor)

	openAI, ok := domain.GetAdapterSpec("openai")
	require.True(t, ok)
	first := presetTextTemplate()
	first.Key, first.Name, first.ProviderModelID = "fixture-first", "Fixture first", "fixture-first"
	second := first
	second.Key, second.Name, second.ProviderModelID = "fixture-second", "Fixture second", "fixture-second"
	document := domain.PresetDocument{
		SchemaVersion: 1, Name: "Directory Test",
		Config: domain.ProviderConfig{
			AdapterKey: "openai", BaseURL: openAI.AllowedBaseURL, Region: openAI.AllowedRegion,
			Capabilities: []string{"text"}, TimeoutSeconds: 10, ConcurrencyLimit: 1, RateLimitPerMin: 60,
		},
		ModelTemplates: []domain.PresetModelTemplate{first, second},
	}
	service := catalogapp.NewProviderPresets(app.NewCatalogStore(conn.DB))
	preset, err := service.Save(t.Context(), actor, catalogapp.SavePresetInput{
		Slug: "directory-" + uuid.NewString(), RequestID: uuid.New(), Document: document, PreviewHash: document.Digest(),
	})
	require.NoError(t, err)

	discovery := catalogapp.PresetDirectoryInput{VersionID: preset.VersionID, APIKey: "test-only"}
	readCalls := 0
	read := func(_ context.Context, config domain.ProviderConfig, key string) ([]string, error) {
		readCalls++
		require.Equal(t, openAI.AllowedBaseURL, config.BaseURL)
		require.Equal(t, discovery.APIKey, key)
		return []string{"fixture-first"}, nil
	}
	_, err = service.ReadDirectory(t.Context(), identityapp.Principal{}, preset.ID, discovery, read)
	require.ErrorIs(t, err, identityapp.ErrForbidden)
	require.Zero(t, readCalls)
	result, err := service.ReadDirectory(t.Context(), actor, preset.ID, discovery, read)
	require.NoError(t, err)
	require.Equal(t, []string{"fixture-first"}, result.ModelIDs)
	require.Equal(t, 1, readCalls)

	input := catalogapp.ConnectPresetInput{
		SelectedModelsOnly: true,
		CatalogVersion:     preset.ModelCatalog.Version,
		CandidateKeys:      []string{first.Key},
		VersionID:          preset.VersionID, RequestID: uuid.New(),
	}
	created, err := service.Connect(t.Context(), actor, preset.ID, input)
	require.NoError(t, err)
	require.Len(t, created.ModelIDs, 1)
	again, err := service.Connect(t.Context(), actor, preset.ID, input)
	require.NoError(t, err)
	require.Equal(t, created, again)

	var modelKeys []string
	require.NoError(t, conn.DB.Raw(
		"SELECT v.provider_model_id FROM catalog.model_profile m JOIN catalog.model_profile_version v ON v.id=m.current_version_id WHERE m.provider_id=? AND m.status='disabled'",
		created.ProviderID,
	).Scan(&modelKeys).Error)
	require.Equal(t, []string{first.ProviderModelID}, modelKeys)

	var priceCount, releaseCount int64
	require.NoError(t, conn.DB.Table("catalog.price_rule_version").Where("model_profile_id = ?", created.ModelIDs[0]).Count(&priceCount).Error)
	require.NoError(t, conn.DB.Table("catalog.model_release").Where("model_id = ?", created.ModelIDs[0]).Count(&releaseCount).Error)
	require.Zero(t, priceCount)
	require.Zero(t, releaseCount)

	input.RequestID, input.CandidateKeys = uuid.New(), nil
	connectionOnly, err := service.Connect(t.Context(), actor, preset.ID, input)
	require.NoError(t, err)
	require.Empty(t, connectionOnly.ModelIDs)
}
