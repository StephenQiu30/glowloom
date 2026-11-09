package catalog_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func arkDirectoryConfig(t *testing.T) domain.ProviderConfig {
	t.Helper()
	spec, ok := domain.GetAdapterSpec("volcengine_ark")
	require.True(t, ok)
	return domain.ProviderConfig{AdapterKey: spec.AdapterKey, BaseURL: spec.AllowedBaseURL, Region: spec.AllowedRegion, Capabilities: []string{"text"}, TimeoutSeconds: 10, ConcurrencyLimit: 1, RateLimitPerMin: 60}
}

func TestArkModelDirectoryReadsOnlyExactIDsThroughExistingParser(t *testing.T) {
	config := arkDirectoryConfig(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "GET", r.Method)
		require.Equal(t, "/api/v3/models", r.URL.Path)
		require.Equal(t, "Bearer synthetic-directory-key", r.Header.Get("Authorization"))
		require.Empty(t, r.URL.RawQuery)
		_, e := io.WriteString(w, `{"data":[{"id":"doubao-seed-2-1-lite-260915","domain":"LLM","status":"Published","api_supported":{"chat_completions":true}},{"id":"doubao-seed-evolving"},{"id":"doubao-seed-2-1-lite-260915"},{"id":"invalid id"},{"id":""},{"id":"line\nbreak"}]}`)
		require.NoError(t, e)
	}))
	defer server.Close()
	target, e := url.Parse(server.URL)
	require.NoError(t, e)
	client := providerhttp.NewClient(&http.Client{Transport: directoryTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "https://ark.cn-beijing.volces.com/api/v3/models", r.URL.String())
		local := r.Clone(r.Context())
		u := *r.URL
		local.URL = &u
		local.URL.Scheme = target.Scheme
		local.URL.Host = target.Host
		return http.DefaultTransport.RoundTrip(local)
	})})
	ids, e := client.ReadModelIDs(t.Context(), config, "synthetic-directory-key")
	require.NoError(t, e)
	require.Equal(t, []string{"doubao-seed-2-1-lite-260915", "doubao-seed-evolving"}, ids)
	require.Equal(t, 1, calls)
	config.BaseURL = "https://untrusted.invalid"
	_, e = client.ReadModelIDs(t.Context(), config, "synthetic-directory-key")
	require.ErrorIs(t, e, catalogapp.ErrModelDirectoryUnsupported)
	require.Equal(t, 1, calls)
}

type arkDirectoryPresetStore struct {
	catalogapp.PresetStore
	presets []domain.Preset
	calls   int
}

func (s *arkDirectoryPresetStore) ListPresets(context.Context, identityapp.Principal) ([]domain.Preset, error) {
	s.calls++
	return s.presets, nil
}

func TestArkModelDirectoryRetainsAdminPresetAndVersionGates(t *testing.T) {
	presetID, version := uuid.New(), uuid.New()
	store := &arkDirectoryPresetStore{presets: []domain.Preset{{ID: presetID, VersionID: version, Document: domain.PresetDocument{Config: arkDirectoryConfig(t)}}}}
	service := catalogapp.NewProviderPresets(store)
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"}
	calls := 0
	read := func(_ context.Context, config domain.ProviderConfig, key string) ([]string, error) {
		calls++
		require.Equal(t, "volcengine_ark", config.AdapterKey)
		require.Equal(t, "synthetic-key", key)
		return []string{"doubao-seed-2-1-lite-260915"}, nil
	}
	input := catalogapp.PresetDirectoryInput{VersionID: version, APIKey: "synthetic-key"}
	_, e := service.ReadDirectory(t.Context(), identityapp.Principal{}, presetID, input, read)
	require.ErrorIs(t, e, identityapp.ErrForbidden)
	require.Zero(t, store.calls)
	require.Zero(t, calls)
	found, e := service.ReadDirectory(t.Context(), actor, presetID, input, read)
	require.NoError(t, e)
	require.Equal(t, []string{"doubao-seed-2-1-lite-260915"}, found.ModelIDs)
	require.Equal(t, 1, calls)
	input.VersionID = uuid.New()
	_, e = service.ReadDirectory(t.Context(), actor, presetID, input, read)
	require.ErrorIs(t, e, catalogapp.ErrPresetUnavailable)
	require.Equal(t, 1, calls)
	input.VersionID = version
	store.presets[0].EmergencyDisabled = true
	_, e = service.ReadDirectory(t.Context(), actor, presetID, input, read)
	require.ErrorIs(t, e, catalogapp.ErrPresetUnavailable)
	require.Equal(t, 1, calls)
}

func TestArkModelDirectoryPreservesBoundedFailuresAndNoRedirect(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"unauthorized", 401, `{"error":{"message":"secret must not escape"}}`, catalogapp.ErrModelDirectoryCredential},
		{"forbidden", 403, `{}`, catalogapp.ErrModelDirectoryCredential},
		{"rate_limited", 429, `{}`, catalogapp.ErrModelDirectoryRateLimited},
		{"not_found", 404, `{}`, catalogapp.ErrModelDirectoryUnavailable},
		{"redirect", 302, `{}`, catalogapp.ErrModelDirectoryUnavailable},
		{"malformed", 200, `{"data":`, catalogapp.ErrModelDirectoryUnavailable},
		{"missing_data", 200, `{}`, catalogapp.ErrModelDirectoryUnavailable},
		{"oversize", 200, strings.Repeat(" ", (1<<20)+1), catalogapp.ErrModelDirectoryUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := providerhttp.NewClient(&http.Client{Transport: directoryTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "GET", r.Method)
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Location": []string{"https://untrusted.invalid/steal"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})})
			_, e := client.ReadModelIDs(t.Context(), arkDirectoryConfig(t), "synthetic-directory-key")
			require.ErrorIs(t, e, tc.want)
			require.NotContains(t, e.Error(), "secret must not escape")
			require.Equal(t, 1, calls)
		})
	}
}
