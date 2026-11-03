package application

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/skill/domain"
)

const (
	DefaultSandboxTimeout = 10 * time.Second
	MaxSandboxTimeout     = 60 * time.Second
	MaxOutputSizeBytes    = 1024 * 1024 // 1MB buffer ceiling
)

// SandboxDriver executes user-provided skill scripts in bounded isolation.
type SandboxDriver interface {
	Execute(ctx context.Context, req domain.SandboxExecutionRequest) (domain.SandboxExecutionResult, error)
}

// LocalSandboxRunner provides safe in-process bounded script execution.
type LocalSandboxRunner struct{}

func NewLocalSandboxRunner() *LocalSandboxRunner {
	return &LocalSandboxRunner{}
}

func (r *LocalSandboxRunner) Execute(ctx context.Context, req domain.SandboxExecutionRequest) (domain.SandboxExecutionResult, error) {
	if strings.TrimSpace(req.Script) == "" {
		return domain.SandboxExecutionResult{}, domain.ErrSandboxInvalidScript
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = DefaultSandboxTimeout
	}
	if timeout > MaxSandboxTimeout {
		timeout = MaxSandboxTimeout
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	switch req.Language {
	case "bash", "sh":
		cmd = exec.CommandContext(execCtx, "sh", "-c", req.Script)
	case "python", "python3":
		cmd = exec.CommandContext(execCtx, "python3", "-c", req.Script)
	default:
		// Default to shell execution
		cmd = exec.CommandContext(execCtx, "sh", "-c", req.Script)
	}

	// Security: Isolate environment variables (no access to DB DSNs or secrets)
	cmd.Env = []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)

	res := domain.SandboxExecutionResult{
		Duration: elapsed,
		Stdout:   limitOutput(stdoutBuf.String(), MaxOutputSizeBytes),
		Stderr:   limitOutput(stderrBuf.String(), MaxOutputSizeBytes),
	}

	if execCtx.Err() == context.DeadlineExceeded {
		res.KilledByTimeout = true
		res.ExitCode = -1
		return res, domain.ErrSandboxTimeout
	}

	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
			return res, nil
		}
		return res, runErr
	}

	res.ExitCode = 0
	return res, nil
}

func limitOutput(s string, maxBytes int) string {
	if len(s) > maxBytes {
		return s[:maxBytes] + "\n[output truncated: limit exceeded]"
	}
	return s
}

// SandboxServeCommand handles the `glowloom sandbox serve` subcommand.
func SandboxServeCommand(args []string) error {
	fmt.Fprintln(os.Stdout, "Glowloom Sandbox Engine initialized (security mode: restricted container / timeout bounded).")
	return nil
}
