package deployment_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestBackendCIEnforcesFullGoQuality(t *testing.T) {
	config := viper.New()
	config.SetConfigFile(filepath.Join("..", "..", "..", ".github", "workflows", "ci.yml"))
	if err := config.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	if got := config.GetString("jobs.backend.defaults.run.working-directory"); got != "backend" {
		t.Fatalf("Go quality checks run outside the canonical module: %q", got)
	}
	var steps []struct{ Run string }
	if err := config.UnmarshalKey("jobs.backend.steps", &steps); err != nil {
		t.Fatal(err)
	}
	var commands strings.Builder
	for _, step := range steps {
		commands.WriteString(step.Run)
		commands.WriteByte('\n')
		if !strings.Contains(step.Run, "golangci-lint") {
			continue
		}
		for _, bypass := range []string{"--new", "--issues-exit-code", "|| true"} {
			if strings.Contains(step.Run, bypass) {
				t.Errorf("Go quality gate bypasses full failures with %q", bypass)
			}
		}
	}
	run := commands.String()
	pinned := regexp.MustCompile(`go install github\.com/golangci/golangci-lint/v2/cmd/golangci-lint@v\d+\.\d+\.\d+(\s|$)`)
	if !pinned.MatchString(run) {
		t.Error("CI does not pin the Go quality tool to a reproducible version")
	}
	for _, command := range []string{"golangci-lint\" run ./...", "go vet ./...", "go build ./..."} {
		if !strings.Contains(run, command) {
			t.Errorf("CI is missing full module check %q", command)
		}
	}
}
