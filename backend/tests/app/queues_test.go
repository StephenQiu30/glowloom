package app_test

import (
	"errors"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func TestResolveWorkerQueuesPreservesCombinedRoleSelection(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
		cfg   config.Config
		want  string
	}{
		{name: "without signing capability", value: "auto", want: "flow,media"},
		{name: "with inline key", value: "auto", cfg: config.Config{CredentialPrivateKeyPEM: "configured-key"}, want: "flow,media,provider"},
		{name: "with key file", value: "auto", cfg: config.Config{CredentialPrivateKeyFile: "/operator/key.pem"}, want: "flow,media,provider"},
		{name: "blank key is not capability", value: "auto", cfg: config.Config{CredentialPrivateKeyPEM: "  "}, want: "flow,media"},
		{name: "old worker default", want: "flow"},
		{name: "explicit queues are canonical", value: "media,flow", want: "flow,media"},
		{name: "explicit provider is not silently removed", value: "provider", want: "provider"},
		{name: "codex remains opt in", value: "agent.codex", want: "agent.codex"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := app.ResolveWorkerQueues(test.value, test.cfg)
			if err != nil || got != test.want {
				t.Fatalf("ResolveWorkerQueues(%q) = %q, %v; want %q", test.value, got, err, test.want)
			}
		})
	}
}

func TestResolveWorkerQueuesRejectsMixedOrDuplicateSelection(t *testing.T) {
	for _, value := range []string{"auto,media", "media,auto", "auto,auto", "flow,flow", "unknown"} {
		if _, err := app.ResolveWorkerQueues(value, config.Config{}); !errors.Is(err, app.ErrRoleNotAvailable) {
			t.Errorf("ResolveWorkerQueues(%q) error = %v, want unavailable", value, err)
		}
	}
}
