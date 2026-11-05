// Package application executes skill requests through an explicitly injected isolation driver.
package application

import (
	"context"
	"errors"
	"strings"

	"github.com/StephenQiu30/glowloom-server/backend/internal/skill/domain"
)

// ErrIsolationUnavailable reports that no isolation driver is configured.
var ErrIsolationUnavailable = errors.New("skill isolation capability unavailable")

// SandboxDriver must be a configured isolation facility; host shell is not a sandbox.
type SandboxDriver interface {
	Execute(context.Context, domain.SandboxExecutionRequest) (domain.SandboxExecutionResult, error)
}

// SandboxRunner validates script requests before delegating to the configured isolation driver.
type SandboxRunner struct{ driver SandboxDriver }

// NewSandboxRunner injects the isolation facility used for script execution.
func NewSandboxRunner(driver SandboxDriver) *SandboxRunner { return &SandboxRunner{driver: driver} }

// Execute rejects empty scripts and runs valid requests through the isolation driver.
func (r *SandboxRunner) Execute(ctx context.Context, in domain.SandboxExecutionRequest) (domain.SandboxExecutionResult, error) {
	if strings.TrimSpace(in.Script) == "" {
		return domain.SandboxExecutionResult{}, domain.ErrSandboxInvalidScript
	}
	if r.driver == nil {
		return domain.SandboxExecutionResult{}, ErrIsolationUnavailable
	}
	return r.driver.Execute(ctx, in)
}
