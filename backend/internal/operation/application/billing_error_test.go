package application

import "testing"

func TestInvalidProviderRequestKeepsSafeClassification(t *testing.T) {
	if got := SafeProviderFailureCode("invalid_provider_request", "not_submitted"); got != "provider_invalid_parameters" {
		t.Fatalf("failure code = %q, want provider_invalid_parameters", got)
	}
	if got := SafeProviderFailureCode("unreviewed private message", "not_submitted"); got != "provider_not_submitted" {
		t.Fatalf("unreviewed code escaped: %q", got)
	}
}
