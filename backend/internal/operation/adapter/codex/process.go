package codex

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sync"
)

// ProcessLauncher starts the explicitly configured Codex executable without a
// shell. It does not read, copy, or log the client's account credentials.
type ProcessLauncher struct{ binary string }

// NewProcessLauncher fixes the executable path; version and real account/model
// eligibility must be verified before the Worker registers this adapter.
func NewProcessLauncher(binary string) (*ProcessLauncher, error) {
	if !filepath.IsAbs(binary) {
		return nil, fmt.Errorf("codex executable must be absolute")
	}
	return &ProcessLauncher{binary: binary}, nil
}

// Open starts one app-server child for a single task. Stderr is deliberately
// discarded because it may contain provider or account diagnostics.
func (l *ProcessLauncher) Open(ctx context.Context, dir string) (io.ReadWriteCloser, error) {
	childCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(childCtx, l.binary, "app-server", "--stdio")
	cmd.Dir = dir
	cmd.Stderr = io.Discard
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open codex stdin: %w", err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		_ = input.Close()
		return nil, fmt.Errorf("open codex stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		_ = input.Close()
		_ = output.Close()
		return nil, fmt.Errorf("start codex process: %w", err)
	}
	return &processConnection{input: input, output: output, cmd: cmd, cancel: cancel}, nil
}

type processConnection struct {
	input  io.WriteCloser
	output io.ReadCloser
	cmd    *exec.Cmd
	cancel context.CancelFunc
	once   sync.Once
}

func (p *processConnection) Read(data []byte) (int, error)  { return p.output.Read(data) }
func (p *processConnection) Write(data []byte) (int, error) { return p.input.Write(data) }
func (p *processConnection) Close() error {
	p.once.Do(func() { p.cancel(); _ = p.input.Close(); _ = p.output.Close(); _ = p.cmd.Wait() })
	return nil
}
