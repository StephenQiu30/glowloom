package providerhttp

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

type serviceCheckTransport func(*http.Request) (*http.Response, error)

func (f serviceCheckTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func checkConfig(adapter string) catalog.ProviderConfig {
	for _, p := range catalog.BuiltinPresets() {
		if p.Config.AdapterKey == adapter {
			return p.Config
		}
	}
	panic("missing preset")
}
func TestCredentialCheckRejectsMalformedSuccessAndSeparatesQuota(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       string
	}{
		{"html", "<html>proxy</html>", 200, "invalid_response"},
		{"wrong schema", `{"ok":true}`, 200, "invalid_response"},
		{"empty directory", `{"data":[]}`, 200, "ok"},
		{"credits", `{"error":{"code":"credit_balance_exhausted","type":"insufficient_quota","message":"secret-raw-body"}}`, 429, "insufficient_balance"},
		{"project", `{"error":{"code":"project_spend_limit_exceeded","type":"insufficient_quota"}}`, 429, "project_limit_exceeded"},
		{"organization", `{"error":{"code":"organization_usage_limit_exceeded","type":"insufficient_quota"}}`, 429, "organization_limit_exceeded"},
		{"quota", `{"error":{"type":"insufficient_quota"}}`, 429, "insufficient_quota"},
		{"ip", `{"error":{"code":"ip_not_authorized"}}`, 401, "ip_restricted"},
		{"region", `{"error":{"code":"unsupported_country_region_territory"}}`, 403, "region_restricted"},
		{"permission", `{"error":{"code":"permission_denied"}}`, 403, "permission_denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewClient(&http.Client{Transport: serviceCheckTransport(func(r *http.Request) (*http.Response, error) {
				require.Equal(t, "GET", r.Method)
				require.Equal(t, "/v1/models", r.URL.Path)
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})})
			result := client.Probe(context.Background(), checkConfig("openai"), "fixture-key")
			require.Equal(t, tc.want, result.Auth)
		})
	}
}
func TestArkCredentialCheckRequiresTaskListSchema(t *testing.T) {
	client := NewClient(&http.Client{Transport: serviceCheckTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	})})
	require.Equal(t, "invalid_response", client.Probe(context.Background(), checkConfig("volcengine_ark"), "fixture-key").Auth)
}

func TestReachabilityHasNoCredentialAndAcceptsEveryHTTPStatus(t *testing.T) {
	for _, status := range []int{200, 301, 401, 403, 404, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			client := NewClient(&http.Client{Transport: serviceCheckTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "GET", r.Method)
				require.Empty(t, r.Header.Get("Authorization"))
				require.Nil(t, r.Body)
				return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://other.invalid/"}}, Body: io.NopCloser(strings.NewReader("not read"))}, nil
			})})
			got := client.ProbeReachability(context.Background(), checkConfig("openai"))
			require.Equal(t, "reachable", got.Result)
			require.Equal(t, status, got.HTTPStatus)
			require.Equal(t, 1, calls)
		})
	}
}
func TestReachabilityRetriesOnlyTimeoutAndRejectsUnapprovedURL(t *testing.T) {
	for _, tc := range []struct {
		err       error
		wantCalls int
	}{{context.DeadlineExceeded, 2}, {io.ErrUnexpectedEOF, 1}} {
		calls := 0
		client := NewClient(&http.Client{Transport: serviceCheckTransport(func(*http.Request) (*http.Response, error) { calls++; return nil, tc.err })})
		got := client.ProbeReachability(context.Background(), checkConfig("deepseek"))
		require.Equal(t, tc.wantCalls, calls)
		require.Equal(t, tc.wantCalls, got.Attempts)
		cfg := checkConfig("deepseek")
		cfg.BaseURL = "https://unapproved.invalid"
		got = client.ProbeReachability(context.Background(), cfg)
		require.Equal(t, "unsupported", got.Result)
		require.Equal(t, tc.wantCalls, calls)
	}
}
