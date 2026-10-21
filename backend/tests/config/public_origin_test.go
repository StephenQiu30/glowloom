package config_test

import (
	"errors"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func TestBrowserOriginIsExactAndLocalHTTPIsLoopbackOnly(t *testing.T) {
	for _, test := range []struct {
		origin, env string
		valid       bool
	}{
		{"http://127.0.0.1:3140", "local", true},
		{"http://[::1]:3140", "local", true},
		{"http://localhost:3000", "local", true},
		{"https://glowloom.example", "prod", true},
		{"https://glowloom.example", "staging", true},
		{"http://glowloom.example", "local", false},
		{"http://localhost:3000", "prod", false},
		{"http://127.0.0.1:3140", "staging", false},
		{"https://user@glowloom.example", "prod", false},
		{"https://glowloom.example/", "prod", false},
		{"https://glowloom.example?", "prod", false},
		{"https://glowloom.example?token=foo", "prod", false},
		{"https://glowloom.example#fragment", "prod", false},
		{"https://:3140", "prod", false},
		{"", "local", false},
	} {
		t.Run(test.origin+"/"+test.env, func(t *testing.T) {
			err := config.ValidatePublicOrigin(test.origin, test.env)
			if test.valid && err != nil || !test.valid && !errors.Is(err, config.ErrInvalid) {
				t.Fatalf("origin validity=%v error=%v", test.valid, err)
			}
		})
	}
}

func TestProductionWorkerConfigurationDoesNotRequireBrowserOrigin(t *testing.T) {
	t.Setenv("GLOWLOOM_ENV_FILE", "")
	t.Setenv("GLOWLOOM_ENV", "prod")
	t.Setenv("GLOWLOOM_PUBLIC_ORIGIN", "")
	if _, err := config.Load(); err != nil {
		t.Fatalf("non-HTTP role configuration rejected: %v", err)
	}
}
