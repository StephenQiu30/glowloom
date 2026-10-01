package videodepth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

const maxNativeLog = 64 << 10
const maxProcessRSS int64 = 8 << 30

// nativeOutput consumes bounded diagnostics without returning private stderr.
type nativeOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	line   []byte
	onLine func([]byte) error
	cancel context.CancelFunc
	err    error
	stream bool
}

func (w *nativeOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return len(p), nil
	}
	if !w.stream && w.buffer.Len()+len(p) > maxNativeLog {
		w.err = application.ErrDepthBudgetExceeded
		w.cancel()
		return len(p), nil
	}
	if !w.stream {
		_, _ = w.buffer.Write(p)
	}
	if w.onLine != nil {
		w.line = append(w.line, p...)
		for {
			i := bytes.IndexByte(w.line, '\n')
			if i < 0 {
				if len(w.line) > maxNativeLog {
					w.err = application.ErrDepthBudgetExceeded
					w.cancel()
				}
				break
			}
			if i > maxNativeLog {
				w.err = application.ErrDepthBudgetExceeded
				w.cancel()
				break
			}
			if err := w.onLine(w.line[:i]); err != nil {
				w.err = err
				w.cancel()
				break
			}
			w.line = w.line[i+1:]
		}
	}
	return len(p), nil
}

func runCommand(ctx context.Context, exe string, args []string, rss int64) ([]byte, error) {
	data, _, err := runProcess(ctx, exe, args, rss, nil)
	return data, err
}

func runProcess(ctx context.Context, exe string, args []string, rss int64, onLine func([]byte) error) ([]byte, application.DepthExecutionFacts, error) {
	return runNative(ctx, exe, args, rss, onLine, false)
}

func runNative(ctx context.Context, exe string, args []string, rss int64, onLine func([]byte) error, stream bool) ([]byte, application.DepthExecutionFacts, error) {
	facts := application.DepthExecutionFacts{RSSLimitBytes: rss}
	started := time.Now()
	dir, err := os.MkdirTemp("", "lanverse-depth-runtime-*")
	if err != nil {
		return nil, facts, errors.Join(application.ErrDepthRuntimeUnavailable, err)
	}
	ceased := true
	defer func() {
		if ceased {
			_ = os.RemoveAll(dir)
		}
	}()
	owned, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(owned, exe, args...)
	// The fixed ML script receives every path explicitly. Do not pass secrets,
	// proxy settings, PYTHONPATH or an unrelated user's package configuration.
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=/usr/bin:/bin", "LANG=C.UTF-8", "PYTORCH_ENABLE_MPS_FALLBACK=0",
		"HOME=" + dir, "XDG_CACHE_HOME=" + filepath.Join(dir, "cache"),
		"MPLCONFIGDIR=" + filepath.Join(dir, "matplotlib"),
		"TORCH_HOME=" + filepath.Join(dir, "torch"),
		"TORCHINDUCTOR_CACHE_DIR=" + filepath.Join(dir, "inductor"),
		"HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1",
	}
	if err := configureProcess(cmd); err != nil {
		return nil, facts, err
	}
	stdout := &nativeOutput{onLine: onLine, cancel: cancel, stream: stream}
	stderr := &nativeOutput{cancel: cancel}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Start(); err != nil {
		return nil, facts, errors.Join(application.ErrDepthRuntimeUnavailable, err)
	}
	finished := make(chan struct{})
	var monitor sync.WaitGroup
	var memoryErr error
	if rss > 0 {
		monitor.Add(1)
		go func() {
			defer monitor.Done()
			ticker := time.NewTicker(200 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-finished:
					return
				case <-owned.Done():
					return
				case <-ticker.C:
					value, err := processGroupRSS(owned, cmd.Process.Pid)
					if err != nil {
						memoryErr = application.ErrDepthRuntimeUnavailable
						cancel()
						return
					}
					if value > rss {
						facts.PeakRSSBytes = max(facts.PeakRSSBytes, value)
						memoryErr = application.ErrDepthBudgetExceeded
						cancel()
						return
					}
					facts.PeakRSSBytes = max(facts.PeakRSSBytes, value)
				}
			}
		}()
	}
	err = cmd.Wait()
	close(finished)
	monitor.Wait()
	// Wait on the parent does not prove that a Python-owned FFmpeg child exited.
	// Always stop any surviving group and check actual non-zombie cessation.
	stopErr := stopProcessGroup(ctx, cmd.Process.Pid)
	facts.ElapsedMS = time.Since(started).Milliseconds()
	if stopErr != nil {
		ceased = false
		return nil, facts, errors.Join(application.ErrDepthCessationUncertain, ctx.Err(), stopErr)
	}
	facts.ProcessGroupJoined = true
	if memoryErr != nil {
		return nil, facts, memoryErr
	}
	if stdout.err != nil {
		return nil, facts, stdout.err
	}
	if stderr.err != nil {
		return nil, facts, stderr.err
	}
	if ctx.Err() != nil {
		return nil, facts, ctx.Err()
	}
	if err != nil {
		return stdout.buffer.Bytes(), facts, err
	}
	return stdout.buffer.Bytes(), facts, nil
}

var _ io.Writer = (*nativeOutput)(nil)
