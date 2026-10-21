package config

import (
	"os"
	"strings"

	"github.com/spf13/viper"
)

// environmentFile prefers an explicitly configured canonical key, even if empty.
func environmentFile() string {
	if value, ok := os.LookupEnv("GLOWLOOM_ENV_FILE"); ok {
		return value
	}
	return os.Getenv("LV_ENV_FILE")
}

// applyLegacyEnvironment keeps existing deployments usable while canonical keys
// migrate. Canonical environment and file keys are authoritative; explicit empty
// canonical values never revive an old credential or a required setting.
func applyLegacyEnvironment(v *viper.Viper) {
	for _, key := range v.AllKeys() {
		legacy := strings.ToUpper(key)
		if !strings.HasPrefix(legacy, "LV_") {
			continue
		}
		canonical := "GLOWLOOM_" + strings.TrimPrefix(legacy, "LV_")
		if !v.InConfig(canonical) {
			v.SetDefault(canonical, v.Get(key))
		}
	}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(key, "LV_") || value == "" {
			continue
		}
		canonical := "GLOWLOOM_" + strings.TrimPrefix(key, "LV_")
		if _, explicit := os.LookupEnv(canonical); !explicit && !v.InConfig(canonical) {
			v.Set(canonical, value)
		}
	}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok && strings.HasPrefix(key, "GLOWLOOM_") {
			v.Set(key, value)
		}
	}
}
