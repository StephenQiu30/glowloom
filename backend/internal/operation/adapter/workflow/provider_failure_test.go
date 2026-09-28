package workflow

import "testing"

func TestProviderFailureCodeNormalizesAndNamespaces(t *testing.T) {
	for _, test := range []struct {
		name  string
		input *ProviderError
		want  string
	}{
		{name: "missing", want: "provider:failed"},
		{name: "normalized", input: &ProviderError{Code: " Rate Limited / 429 "}, want: "provider:rate_limited___429"},
		{name: "stable", input: &ProviderError{Code: "mock_failure"}, want: "provider:mock_failure"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := providerFailureCode(test.input); got != test.want {
				t.Fatalf("provider failure code = %q, want %q", got, test.want)
			}
		})
	}
}
