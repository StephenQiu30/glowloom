package skill_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/skill/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/skill/domain"
)

func TestLocalSandboxRunnerExecution(t *testing.T) {
	runner := application.NewLocalSandboxRunner()
	ctx := context.Background()

	t.Run("SuccessfulScriptOutput", func(t *testing.T) {
		req := domain.SandboxExecutionRequest{
			Language: "bash",
			Script:   `echo "Hello Sandbox Script"`,
			Timeout:  5 * time.Second,
		}
		res, err := runner.Execute(ctx, req)
		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}
		if res.ExitCode != 0 {
			t.Errorf("expected exit code 0, got %d", res.ExitCode)
		}
		if !strings.Contains(res.Stdout, "Hello Sandbox Script") {
			t.Errorf("expected output to contain greeting, got: %s", res.Stdout)
		}
		if res.KilledByTimeout {
			t.Errorf("expected killed by timeout to be false")
		}
	})

	t.Run("TimeoutTermination", func(t *testing.T) {
		req := domain.SandboxExecutionRequest{
			Language: "bash",
			Script:   `sleep 2`,
			Timeout:  100 * time.Millisecond, // very short timeout
		}
		res, err := runner.Execute(ctx, req)
		if !errors.Is(err, domain.ErrSandboxTimeout) {
			t.Fatalf("expected ErrSandboxTimeout, got %v", err)
		}
		if !res.KilledByTimeout {
			t.Errorf("expected res.KilledByTimeout to be true")
		}
	})

	t.Run("NonZeroExitCode", func(t *testing.T) {
		req := domain.SandboxExecutionRequest{
			Language: "bash",
			Script:   `echo "failing" >&2; exit 42`,
			Timeout:  5 * time.Second,
		}
		res, err := runner.Execute(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error wrapping non-zero exit: %v", err)
		}
		if res.ExitCode != 42 {
			t.Errorf("expected exit code 42, got %d", res.ExitCode)
		}
		if !strings.Contains(res.Stderr, "failing") {
			t.Errorf("expected stderr to contain 'failing', got: %s", res.Stderr)
		}
	})

	t.Run("EnvironmentIsolation", func(t *testing.T) {
		req := domain.SandboxExecutionRequest{
			Language: "bash",
			Script:   `env`,
			Timeout:  5 * time.Second,
		}
		res, err := runner.Execute(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Must NOT contain sensitive env keys like DSN or SECRETS
		if strings.Contains(res.Stdout, "GLOWLOOM_TEST_AGENT_CONTROL_DB_DSN") {
			t.Errorf("security breach: sensitive database DSN leaked into sandbox environment!")
		}
	})
}
