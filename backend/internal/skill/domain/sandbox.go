// Package domain defines immutable skill packages and isolated execution contracts.
package domain

import (
	"errors"
	"time"
)

// Sandbox errors classify execution limits, denied network access and invalid input.
var (
	ErrSandboxTimeout       = errors.New("sandbox execution timed out")
	ErrSandboxMemoryLimit   = errors.New("sandbox memory limit exceeded")
	ErrSandboxNetworkDenied = errors.New("sandbox network access denied")
	ErrSandboxInvalidScript = errors.New("sandbox script cannot be empty")
)

// SandboxExecutionRequest describes a guest script invocation inside an isolated container.
type SandboxExecutionRequest struct {
	Language        string        `json:"language"` // "python", "bash", "javascript"
	Script          string        `json:"script"`
	Timeout         time.Duration `json:"timeout"`
	MemoryLimitMB   int           `json:"memory_limit_mb"`
	NetworkDisabled bool          `json:"network_disabled"`
	ReadOnlyRoot    bool          `json:"read_only_root"`
}

// SandboxExecutionResult captures the captured output and exit metrics of a sandbox run.
type SandboxExecutionResult struct {
	ExitCode        int           `json:"exit_code"`
	Stdout          string        `json:"stdout"`
	Stderr          string        `json:"stderr"`
	Duration        time.Duration `json:"duration"`
	KilledByTimeout bool          `json:"killed_by_timeout"`
}
