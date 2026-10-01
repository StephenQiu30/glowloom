package media_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/videodepth"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

func TestDepthRunnerRejectsOfflineModelBeforeOutput(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "video_depth_anything_vits.pth")
	if err := os.WriteFile(model, []byte("wrong model"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := videodepth.NewRunner(videodepth.Config{PythonPath: "/usr/bin/python3", SourceDir: dir, ModelPath: model, Device: "cpu", FFmpegPath: "ffmpeg", FFprobePath: "ffprobe"}, mediaflow.FFProber{})
	if !errors.Is(err, toolapp.ErrDepthModelMismatch) {
		t.Fatalf("missing fixed source/weights was accepted: %v", err)
	}
}

func TestDepthRunnerRejectsSameSizeWrongHashBeforeAVOrModel(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "video_depth_anything_vits.pth")
	file, err := os.Create(model)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(116440756); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source")
	if err := os.MkdirAll(filepath.Join(source, "video_depth_anything"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "video_depth_anything", "video_depth.py"), []byte("untrusted fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	runner, err := videodepth.NewRunner(videodepth.Config{PythonPath: "/usr/bin/python3", SourceDir: source, ModelPath: model, Device: "cpu", FFmpegPath: "ffmpeg", FFprobePath: "ffprobe"}, mediaflow.FFProber{})
	if err != nil {
		t.Fatal(err)
	}
	var phases []toolapp.DepthPhase
	output, err := runner.Process(t.Context(), nil, "", func(p toolapp.DepthPhase) error { phases = append(phases, p); return nil })
	if output != nil || !errors.Is(err, toolapp.ErrDepthModelMismatch) || len(phases) != 1 || phases[0] != toolapp.DepthChecking {
		t.Fatalf("fixed-hash gate executed AV/model: %v %+v %v", err, output, phases)
	}
}

func TestDepthRunnerSerialSlotWaitingCancellationAndCallbackRelease(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "video_depth_anything_vits.pth")
	file, err := os.Create(model)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(116440756); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "video_depth_anything"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "video_depth_anything", "video_depth.py"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	runner, err := videodepth.NewRunner(videodepth.Config{PythonPath: "/usr/bin/python3", SourceDir: dir, ModelPath: model, Device: "cpu", FFmpegPath: "ffmpeg", FFprobePath: "ffprobe"}, mediaflow.FFProber{})
	if err != nil {
		t.Fatal(err)
	}
	owner, cancel := context.WithCancel(t.Context())
	defer cancel()
	entered := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := runner.Process(owner, nil, "", func(toolapp.DepthPhase) error {
			close(entered)
			<-owner.Done()
			return owner.Err()
		})
		done <- err
	}()
	<-entered
	waiter, stop := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer stop()
	phaseCalled := false
	if out, err := runner.Process(waiter, nil, "", func(toolapp.DepthPhase) error { phaseCalled = true; return nil }); out != nil || !errors.Is(err, context.DeadlineExceeded) || phaseCalled {
		t.Fatalf("waiting caller started work or lost cancellation: %v %+v %v", err, out, phaseCalled)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	stopCallback := errors.New("stop before validation")
	if out, err := runner.Process(t.Context(), nil, "", func(toolapp.DepthPhase) error { return stopCallback }); out != nil || !errors.Is(err, stopCallback) {
		t.Fatalf("callback left the sole slot unavailable: %v %+v", err, out)
	}
}

func TestDepthRunnerRejectsUnconfiguredOrUnknownDevice(t *testing.T) {
	for _, cfg := range []videodepth.Config{{}, {PythonPath: "/usr/bin/python3", Device: "auto"}, {PythonPath: "/usr/bin/python3", Device: "cuda"}} {
		if _, err := videodepth.NewRunner(cfg, mediaflow.FFProber{}); !errors.Is(err, toolapp.ErrDepthRuntimeUnavailable) {
			t.Fatalf("unsafe or unverified runtime accepted: %+v %v", cfg, err)
		}
	}
}
