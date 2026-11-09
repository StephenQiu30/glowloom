package operation_test

import (
	"net/http"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/tests/catalogfixture"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestBillingErrorClassificationRequiresStructuredEvidence(t *testing.T) {
	config := catalogfixture.BuiltinPresets()[0].Config
	for _, tc := range []struct {
		status     int
		body, code string
	}{
		{402, `{"error":{"message":"private invoice"}}`, "payment_required"},
		{402, `{"error":{"code":"credit_balance_exhausted","message":"private invoice"}}`, "insufficient_balance"},
		{429, `{"error":{"code":"project_spend_limit_exceeded"}}`, "project_limit_exceeded"},
		{429, `{"error":{"code":"rate_limit_exceeded"}}`, "rate_limited"},
		{429, `{"error":{}}`, "request_limited"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			client := providerhttp.NewClient(&http.Client{Transport: adapterTransport(func(*http.Request) (*http.Response, error) { return adapterResponse(tc.status, tc.body), nil })})
			result, err := client.Submit(t.Context(), application.GenerationRequest{Config: config, Capability: "text.generate", Mode: "text_to_text", ModelID: "fixture", Prompt: "hello", OutputCount: 1}, "fixture-key")
			require.NoError(t, err)
			require.Equal(t, tc.code, result.ErrorCode)
			require.NotContains(t, result.ErrorCode, "private")
		})
	}
}
