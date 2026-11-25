package media_test

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func TestReferenceVisualsDecodeActualImageAndBoundOutput(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "source-*.png")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	im := image.NewNRGBA(image.Rect(0, 0, 1200, 800))
	im.Set(0, 0, color.NRGBA{R: 255, A: 255})
	if err := png.Encode(file, im); err != nil {
		t.Fatal(err)
	}
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	frames, err := mediaflow.InspectReferenceVisuals(t.Context(), &application.Downloaded{File: file, Size: info.Size(), MIMEType: "image/png"})
	if err != nil || len(frames) != 1 || len(frames[0].Data) == 0 || len(frames[0].Data) > 2<<20 || frames[0].Width > 640 || frames[0].Height > 640 {
		t.Fatalf("frames=%+v error=%v", frames, err)
	}
}

func TestReferenceVisualsExtractThreeActualVideoFramesAndCancel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.mp4")
	cmd := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "testsrc=size=160x90:rate=10", "-t", "3", "-pix_fmt", "yuv420p", "-y", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create real video: %s: %v", output, err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	source := &application.Downloaded{File: file, Size: info.Size(), MIMEType: "video/mp4"}
	frames, err := mediaflow.InspectReferenceVisuals(t.Context(), source)
	if err != nil || len(frames) != 3 {
		t.Fatalf("frames=%+v error=%v", frames, err)
	}
	for i, frame := range frames {
		if len(frame.Data) == 0 || len(frame.Data) > 2<<20 || i > 0 && frame.TimestampMS <= frames[i-1].TimestampMS {
			t.Fatalf("invalid frame %d: %+v", i, frame)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := mediaflow.InspectReferenceVisuals(ctx, source); err == nil {
		t.Fatal("cancelled inspection succeeded")
	}
}
