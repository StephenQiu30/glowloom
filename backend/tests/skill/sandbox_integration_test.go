package skill_test

import (
	"errors"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/skill/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/skill/domain"
)

func TestScriptsRequireConfiguredIsolation(t *testing.T) {
	runner := application.NewSandboxRunner(nil)
	if _, err := runner.Execute(t.Context(), domain.SandboxExecutionRequest{Language: "bash", Script: "echo should-not-run"}); !errors.Is(err, application.ErrIsolationUnavailable) {
		t.Fatalf("unisolated script admitted: %v", err)
	}
}
