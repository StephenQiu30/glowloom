package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func uploadWebMFixture(t *testing.T, codec, duration string) []byte {
	t.Helper()
	name := filepath.Join(t.TempDir(), "recording.webm")
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=64x48:rate=10", "-t", duration,
		"-c:v", codec, "-threads", "1", "-deadline", "realtime", "-cpu-used", "8", "-an", "-f", "webm", "-live", "1", name}
	if output, err := exec.CommandContext(t.Context(), "ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("create real recording fixture: %v: %s", err, output)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func readUploadWebM(t *testing.T, data []byte) *mediaapp.Downloaded {
	t.Helper()
	file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(data), "白膜镜头.webm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func TestUploadNormalizerDecodesDurationlessVP8AndVP9ToRealMP4(t *testing.T) {
	for _, codec := range []string{"libvpx", "libvpx-vp9"} {
		t.Run(codec, func(t *testing.T) {
			source := readUploadWebM(t, uploadWebMFixture(t, codec, "0.6"))
			output, err := exec.CommandContext(t.Context(), "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "json", source.File.Name()).Output()
			if err != nil || bytes.Contains(output, []byte("duration")) {
				t.Fatalf("fixture must lack container duration: %s err=%v", output, err)
			}
			result, err := (mediaflow.FFUploadNormalizer{}).Normalize(t.Context(), source)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = result.File.Close() }()
			probe, err := (mediaflow.FFUploadProber{}).Probe(t.Context(), result.File)
			if err != nil || result.File == source || result.File.SHA256 == source.SHA256 || result.File.MIMEType != "video/mp4" ||
				probe.Kind != domain.KindVideo || probe.Extension != "mp4" || probe.Codec == nil || *probe.Codec != "h264" ||
				probe.Width == nil || *probe.Width != 64 || probe.Height == nil || *probe.Height != 48 ||
				probe.DurationMS == nil || *probe.DurationMS < 590 || *probe.DurationMS > 610 {
				t.Fatalf("canonical output=%+v probe=%+v err=%v", result, probe, err)
			}
			wantCodec := map[string]string{"libvpx": "vp8", "libvpx-vp9": "vp9"}[codec]
			if result.SourceCodec != wantCodec {
				t.Fatalf("actual source codec=%q want=%q", result.SourceCodec, wantCodec)
			}
			if source.File == nil {
				t.Fatal("normalizer closed caller-owned original")
			}
		})
	}
}

func TestUploadNormalizerRejectsLongCorruptAndCancelledRecordings(t *testing.T) {
	short := uploadWebMFixture(t, "libvpx-vp9", "0.3")
	long := uploadWebMFixture(t, "libvpx-vp9", "60.2")
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"long actual decoded duration", long},
		{"incomplete real container", short[:80]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := readUploadWebM(t, tc.data)
			result, err := (mediaflow.FFUploadNormalizer{}).Normalize(t.Context(), file)
			if !errors.Is(err, mediaapp.ErrUnsupportedUpload) || result.File != nil {
				t.Fatalf("bad recording returned canonical file=%+v err=%v", result, err)
			}
		})
	}
	file := readUploadWebM(t, short)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := (mediaflow.FFUploadNormalizer{}).Normalize(ctx, file)
	if !errors.Is(err, context.Canceled) || result.File != nil {
		t.Fatalf("cancelled normalization result=%+v err=%v", result, err)
	}
}

func TestUploadNormalizerEnforcesInputSizeAndContainerCodec(t *testing.T) {
	file := readUploadWebM(t, uploadWebMFixture(t, "libvpx", "0.3"))
	file.Size = mediaapp.MaxUploadVideoBytes + 1
	if _, err := (mediaflow.FFUploadNormalizer{}).Normalize(t.Context(), file); !errors.Is(err, mediaapp.ErrUploadTooLarge) {
		t.Fatalf("oversized input error=%v", err)
	}
	name := filepath.Join(t.TempDir(), "audio.webm")
	if output, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=duration=0.3", "-c:a", "libopus", name).CombinedOutput(); err != nil {
		t.Fatalf("create audio-only WebM: %v: %s", err, output)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (mediaflow.FFUploadNormalizer{}).Normalize(t.Context(), readUploadWebM(t, data)); !errors.Is(err, mediaapp.ErrUnsupportedUpload) {
		t.Fatalf("audio-only WebM accepted: %v", err)
	}
}

func TestNormalizedUploadPreservesInputReceiptAndCanonicalReview(t *testing.T) {
	repo, objects := &uploadRepoFake{}, &uploadObjectsFake{}
	in := uploadInput(t)
	in.File = readUploadWebM(t, uploadWebMFixture(t, "libvpx-vp9", "0.6"))
	in.Request.FileName = "白膜镜头.webm"
	service := mediaapp.NewUploadService(repo, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
	result, err := service.Upload(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	var review mediaapp.LocalUploadReview
	if err := json.Unmarshal(repo.asset.ModerationDetail, &review); err != nil {
		t.Fatal(err)
	}
	if result.Asset.Kind != "video" || repo.asset.FileName != "白膜镜头.mp4" || repo.asset.MimeType != "video/mp4" ||
		repo.asset.SHA256 == nil || *repo.asset.SHA256 == in.File.SHA256 || !strings.HasSuffix(repo.asset.ObjectKey, ".mp4") ||
		review.SHA256 != in.File.SHA256 || review.Normalization == nil || review.Normalization.Source.SHA256 != in.File.SHA256 ||
		review.Normalization.Canonical.SHA256 != *repo.asset.SHA256 || len(repo.rends) != 2 || len(objects.items) != 3 {
		t.Fatalf("normalized upload asset=%+v review=%+v", repo.asset, review)
	}
}
