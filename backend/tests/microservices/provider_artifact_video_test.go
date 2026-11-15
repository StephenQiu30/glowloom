package microservices_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/fetch"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/media/adapter/workflow"
)

func TestProviderArtifactMediaVideoProbeKeepsExactOriginalBytes(t *testing.T) {
	name := filepath.Join(t.TempDir(), "original.mp4")
	cmd := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "color=c=black:s=32x32:r=10", "-t", "1", "-an", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-threads", "1", "-y", name)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	source := &fetch.Downloaded{File: file, Size: int64(len(raw)), SHA256: hex.EncodeToString(sum[:]), MIMEType: "video/mp4"}
	defer func() { _ = source.Close() }()
	req := artifactRequest(t, raw)
	req.MIME = "video/mp4"
	req.Processing = "video_probe"
	req.Command.SHA256 = req.Digest()
	result, duration, err := (mediaflow.ProviderArtifactProcessor{}).ProcessProviderArtifact(t.Context(), req, source)
	if err != nil || result != source || duration < 1000 || duration > 1100 {
		t.Fatalf("original video probe failed: same=%v duration=%d err=%v", result == source, duration, err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	after := sha256.Sum256(actual)
	if after != sum {
		t.Fatal("probe altered original supplier video")
	}
}
