package media_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func TestVideoRendererIsDeterministicAndUsesPortraitProjectAspect(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("FFmpeg is required for the media worker renderer")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is required for the media worker renderer")
	}
	fixture := filepath.Join(t.TempDir(), "provider.mp4")
	cmd := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error",
		"-y", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=25", "-t", "1",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", fixture)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create video fixture: %v: %s", err, output)
	}
	source, err := os.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }()
	info, err := source.Stat()
	if err != nil {
		t.Fatal(err)
	}
	media := &mediaapp.Downloaded{File: source, Size: info.Size(), MIMEType: "video/mp4"}
	probe, err := (mediaflow.FFProber{}).Probe(t.Context(), media)
	if err != nil {
		t.Fatal(err)
	}
	first, err := (mediaflow.FFRenderer{}).Render(t.Context(), media, probe, "9:16")
	if err != nil {
		t.Fatal(err)
	}
	defer closeRenditions(first)
	second, err := (mediaflow.FFRenderer{}).Render(t.Context(), media, probe, "9:16")
	if err != nil {
		t.Fatal(err)
	}
	defer closeRenditions(second)
	if len(first) != 2 || len(second) != 2 ||
		first[1].Kind != domain.RenditionProxy720p || first[1].Width != 720 || first[1].Height != 1280 {
		t.Fatalf("portrait previews = %+v", first)
	}
	for i := range first {
		if first[i].Kind != second[i].Kind || first[i].Result.SHA256 != second[i].Result.SHA256 {
			t.Fatalf("render %d differs across retry", i)
		}
	}
}

func closeRenditions(renditions []mediaapp.RenditionFile) {
	for _, rendition := range renditions {
		_ = rendition.Result.Close()
	}
}
