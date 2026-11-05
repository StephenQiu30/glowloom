package config_test

import (
	"os"
	"strings"
	"testing"
)

// Configuration tests declare all their process inputs explicitly. Integration
// service variables inherited from a whole-repository run cannot supply defaults.
func TestMain(m *testing.M) {
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GLOWLOOM_") {
			_ = os.Unsetenv(key)
		}
	}
	os.Exit(m.Run())
}
