package media_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/videodepth"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

func depthNativeRunner(t *testing.T) *videodepth.Runner {
	return depthNativeRunnerBudget(t, 0)
}

func depthNativeRunnerBudget(t *testing.T, rss int64) *videodepth.Runner {
	t.Helper()
	base := os.Getenv("LV_DEPTH_TEST_ROOT")
	if base == "" {
		t.Skip("actual fixed VDA/PyTorch offline bundle was not configured")
	}
	runner, err := videodepth.NewRunner(videodepth.Config{PythonPath: filepath.Join(base, "venv", "bin", "python"), SourceDir: filepath.Join(base, "vda-runtime"), ModelPath: filepath.Join(base, "video_depth_anything_vits.pth"), Device: "mps", FFmpegPath: "ffmpeg", FFprobePath: "ffprobe", RSSLimitBytes: rss}, mediaflow.FFProber{})
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func TestDepthNativeActualSmallMPSCompleteVideos(t *testing.T) {
	runner := depthNativeRunner(t)
	for _, duration := range []string{"2", "15"} {
		t.Run(duration+"s", func(t *testing.T) {
			input := depthTestVideo(t, "30", duration)
			var phases []toolapp.DepthPhase
			output, err := runner.Process(t.Context(), input, input.SHA256, func(phase toolapp.DepthPhase) error { phases = append(phases, phase); return nil })
			if err != nil {
				t.Fatalf("actual native failure after phases %v: %v", phases, err)
			}
			defer func() { _ = output.File.Close() }()
			want := 60
			if duration == "15" {
				want = 450
			}
			if output.Receipt.Device != "mps" || output.Receipt.Output.FrameCount != want || output.Receipt.Output.DurationMS != int64(want)*1000/30 || output.Receipt.Output.Width != 1920 || output.Receipt.Output.Height != 1080 || output.Receipt.Output.FPS != 30 || output.Receipt.Prepared.FrameCount != want || output.File.SHA256 == input.SHA256 || !output.Receipt.Native.ProcessGroupJoined || output.Receipt.Native.PeakRSSBytes < 1 || output.Receipt.Native.PeakRSSBytes > output.Receipt.Native.RSSLimitBytes || output.Receipt.Native.ElapsedMS < 1 {
				t.Fatalf("not complete actual fixed Small output: %+v", output)
			}
			if len(phases) != 6 || phases[0] != toolapp.DepthChecking || phases[2] != toolapp.DepthLoading || phases[3] != toolapp.DepthInferring || phases[4] != toolapp.DepthEncoding || phases[5] != toolapp.DepthVerifying {
				t.Fatalf("fabricated/missing phases: %v", phases)
			}
			t.Logf("actual fixed Small MPS %ss SHA=%s bytes=%d receipt=%+v", duration, output.File.SHA256, output.File.Size, output.Receipt)
			if base := os.Getenv("LV_DEPTH_TEST_EVIDENCE"); base != "" {
				if err := os.MkdirAll(base, 0700); err != nil {
					t.Fatal(err)
				}
				f, err := os.Create(filepath.Join(base, "depth-"+duration+"s.mp4"))
				if err != nil {
					t.Fatal(err)
				}
				_, copyErr := io.Copy(f, io.NewSectionReader(output.File.File, 0, output.File.Size))
				closeErr := f.Close()
				if copyErr != nil || closeErr != nil {
					t.Fatal(errors.Join(copyErr, closeErr))
				}
				data, err := json.MarshalIndent(output.Receipt, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(base, "receipt-"+duration+"s.json"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestDepthNativeCallbackFailureStopsRealEncoding(t *testing.T) {
	runner := depthNativeRunner(t)
	input := depthTestVideo(t, "30", "2")
	stop := errors.New("owner withdrew encoding")
	var stopped []int
	output, err := runner.Process(t.Context(), input, input.SHA256, func(phase toolapp.DepthPhase) error {
		if phase == toolapp.DepthEncoding {
			parents := depthOwnedChildren(t, os.Getpid())
			if len(parents) != 1 || !strings.Contains(strings.ToLower(parents[0].program), "python") {
				t.Errorf("actual controlled Python was not observed: %+v", parents)
				return stop
			}
			children := depthOwnedChildren(t, parents[0].pid)
			foundFFmpeg := false
			stopped = []int{parents[0].pid}
			for _, child := range children {
				if child.group != parents[0].group {
					t.Errorf("native child escaped its owned process group: %+v", child)
				}
				foundFFmpeg = foundFFmpeg || strings.Contains(strings.ToLower(child.program), "ffmpeg")
				stopped = append(stopped, child.pid)
			}
			if !foundFFmpeg {
				t.Errorf("actual controlled FFmpeg child was not observed: %+v", children)
				return stop
			}
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || output != nil {
		t.Fatalf("phase failure still published output: %v %+v", err, output)
	}
	if len(stopped) < 2 {
		t.Fatal("no real parent-and-child cessation evidence")
	}
	depthAssertStopped(t, stopped)
	// A second caller must observe a released slot only after native cessation.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := runner.Process(ctx, input, input.SHA256, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancelable slot: %v", err)
	}
}

type depthChild struct {
	pid, group int
	program    string
}

// Inspect only this test's immediate children and their program names. Command
// arguments and unrelated processes are never read.
func depthOwnedChildren(t *testing.T, parent int) []depthChild {
	t.Helper()
	data, err := exec.CommandContext(t.Context(), "pgrep", "-P", strconv.Itoa(parent)).Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return nil
		}
		t.Error(err)
		return nil
	}
	var result []depthChild
	for _, field := range strings.Fields(string(data)) {
		pid, err := strconv.Atoi(field)
		if err != nil {
			t.Error(err)
			continue
		}
		status, err := exec.CommandContext(t.Context(), "ps", "-p", field, "-o", "pgid=,stat=,comm=").Output()
		if err != nil {
			continue // The pgrep helper itself may have already exited.
		}
		fields := strings.Fields(string(status))
		if len(fields) < 3 || strings.HasPrefix(fields[1], "Z") {
			continue
		}
		group, err := strconv.Atoi(fields[0])
		if err != nil {
			t.Error(err)
			continue
		}
		if parent == os.Getpid() && group != pid {
			continue // Ignore the status helper in the test's original group.
		}
		result = append(result, depthChild{pid: pid, group: group, program: filepath.Base(strings.Join(fields[2:], " "))})
	}
	return result
}

func depthAssertStopped(t *testing.T, pids []int) {
	t.Helper()
	for _, pid := range pids {
		data, err := exec.CommandContext(t.Context(), "ps", "-p", strconv.Itoa(pid), "-o", "stat=").Output()
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if state := strings.TrimSpace(string(data)); state != "" && !strings.HasPrefix(state, "Z") {
			t.Fatalf("runner returned while own PID %d remained %q", pid, state)
		}
	}
	t.Logf("actual controlled process IDs %v have no running process after return", pids)
}

func TestDepthNativeMemoryBudgetKillsAndJoinsActualPython(t *testing.T) {
	runner := depthNativeRunnerBudget(t, 1<<20)
	input := depthTestVideo(t, "30", "2")
	done := make(chan error, 1)
	go func() {
		out, err := runner.Process(t.Context(), input, input.SHA256, nil)
		if out != nil {
			_ = out.File.Close()
			done <- errors.New("memory-bound run returned output")
			return
		}
		done <- err
	}()
	var observed []int
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case err := <-done:
			if !errors.Is(err, toolapp.ErrDepthBudgetExceeded) || len(observed) == 0 {
				t.Fatalf("no real memory-limit stop: %v, observed=%v", err, observed)
			}
			depthAssertStopped(t, observed)
			return
		case <-ticker.C:
			for _, child := range depthOwnedChildren(t, os.Getpid()) {
				if strings.Contains(strings.ToLower(child.program), "python") {
					observed = []int{child.pid}
				}
			}
		case <-deadline.C:
			t.Fatal("native memory stop did not converge")
		}
	}
}

func TestDepthNativeActualSmallMPSMaximumFramesAndPixels(t *testing.T) {
	runner := depthNativeRunner(t)
	name := filepath.Join(t.TempDir(), "square.mp4")
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=960x960:rate=30:duration=15.1", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", name}
	if data, err := exec.CommandContext(t.Context(), "ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("actual bounded square source: %v %s", err, data)
	}
	input := depthOpenTestFile(t, name)
	out, err := runner.Process(t.Context(), input, input.SHA256, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = out.File.Close() }()
	if out.Receipt.Prepared.Width != 960 || out.Receipt.Prepared.Height != 960 || out.Receipt.Prepared.FrameCount != 453 || out.Receipt.Output.FrameCount != 453 || out.Receipt.Output.DurationMS != 15100 || out.Receipt.Native.PeakRSSBytes > out.Receipt.Native.RSSLimitBytes || !out.Receipt.Native.ProcessGroupJoined {
		t.Fatalf("maximum full input was reduced or truncated: %+v", out.Receipt)
	}
	t.Logf("actual 960x960/453-frame MPS source output SHA=%s bytes=%d receipt=%+v", out.File.SHA256, out.File.Size, out.Receipt)
	if base := os.Getenv("LV_DEPTH_TEST_EVIDENCE"); base != "" {
		if err := os.MkdirAll(base, 0700); err != nil {
			t.Fatal(err)
		}
		file, err := os.Create(filepath.Join(base, "depth-max-square.mp4"))
		if err != nil {
			t.Fatal(err)
		}
		_, copyErr := io.Copy(file, io.NewSectionReader(out.File.File, 0, out.File.Size))
		if err := errors.Join(copyErr, file.Close()); err != nil {
			t.Fatal(err)
		}
		data, err := json.MarshalIndent(out.Receipt, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(base, "receipt-max-square.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
