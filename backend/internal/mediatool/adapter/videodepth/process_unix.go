//go:build darwin || linux

package videodepth

import (
	"context"
	"errors"
	"io"
	"math"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func configureProcess(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	cmd.WaitDelay = 3 * time.Second
	return nil
}

func groupStatus(ctx context.Context, pid int) ([]byte, error) {
	probe, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	args := []string{"-o", "stat=,rss=", "-g", strconv.Itoa(pid)}
	if runtime.GOOS == "linux" {
		// GNU ps -g selects sessions, unlike BSD ps. Observe only group IDs,
		// states and RSS, then retain the exact owned process group.
		args = []string{"-e", "-o", "pgid=,stat=,rss="}
	}
	cmd := exec.CommandContext(probe, "/bin/ps", args...)
	output := &nativeOutput{cancel: cancel}
	cmd.Stdout, cmd.Stderr = output, io.Discard
	err := cmd.Run()
	if output.err != nil {
		return nil, output.err
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	data := output.buffer.Bytes()
	if runtime.GOOS == "linux" {
		var rows strings.Builder
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 3 && fields[0] == strconv.Itoa(pid) {
				rows.WriteString(fields[1] + " " + fields[2] + "\n")
			}
		}
		return []byte(rows.String()), nil
	}
	return data, nil
}

func processGroupRSS(ctx context.Context, pid int) (int64, error) {
	data, err := groupStatus(ctx, pid)
	if err != nil {
		return 0, err
	}
	var size int64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "Z") {
			continue
		}
		if len(fields) != 2 {
			return 0, errors.New("native process memory observation invalid")
		}
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || value < 0 || value > math.MaxInt64/1024 || size > math.MaxInt64-value*1024 {
			return 0, errors.New("native process memory observation invalid")
		}
		size += value * 1024
	}
	return size, nil
}

func stopProcessGroup(parent context.Context, pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	// Cessation remains owned cleanup after the caller's cancellation.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer cancel()
	for {
		data, err := groupStatus(ctx, pid)
		if err != nil {
			return err
		}
		alive := false
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) > 0 && !strings.HasPrefix(fields[0], "Z") {
				alive = true
			}
		}
		if !alive {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}
