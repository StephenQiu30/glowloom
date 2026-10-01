package media_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	toolff "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/ffmpeg"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

func TestTranscriptionPreprocessActualPCMAndRejectSilentVideo(t *testing.T) {
	p := filepath.Join(t.TempDir(), "source.mp4")
	if data, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=blue:s=32x32:r=25:d=1", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-t", "1", p).CombinedOutput(); err != nil {
		t.Fatalf("real source: %v %s", err, data)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := f.Stat()
	source := &mediaapp.Downloaded{File: f, Size: info.Size(), MIMEType: "video/mp4"}
	defer func() { _ = source.Close() }()
	prep, err := toolff.NewAudioPreprocessor(mediaflow.FFProber{})
	if err != nil {
		t.Fatal(err)
	}
	pcm, err := prep.Prepare(t.Context(), source, 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pcm.File.Close() }()
	output, err := exec.CommandContext(t.Context(), "ffprobe", "-v", "error", "-show_entries", "stream=codec_name,channels,sample_rate", "-of", "json", pcm.File.File.Name()).Output()
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Streams []struct {
			Codec    string `json:"codec_name"`
			Channels int    `json:"channels"`
			Rate     string `json:"sample_rate"`
		} `json:"streams"`
	}
	if json.Unmarshal(output, &parsed) != nil || len(parsed.Streams) != 1 || parsed.Streams[0].Codec != "pcm_s16le" || parsed.Streams[0].Channels != 1 || parsed.Streams[0].Rate != "16000" || pcm.DurationMS < 1000 || pcm.DurationMS > 1050 || pcm.File.MIMEType != "audio/wav" {
		t.Fatalf("not actual native PCM: %s %+v", output, pcm)
	}
	silent := filepath.Join(t.TempDir(), "silent.mp4")
	if data, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=blue:s=32x32:r=25:d=1", "-c:v", "libx264", silent).CombinedOutput(); err != nil {
		t.Fatalf("silent source: %v %s", err, data)
	}
	sf, err := os.Open(silent)
	if err != nil {
		t.Fatal(err)
	}
	si, _ := sf.Stat()
	input := &mediaapp.Downloaded{File: sf, Size: si.Size(), MIMEType: "video/mp4"}
	defer func() { _ = input.Close() }()
	if _, err := prep.Prepare(t.Context(), input, 1000); !errors.Is(err, toolapp.ErrNoAudio) {
		t.Fatalf("silent video fabricated PCM extraction: %v", err)
	}
}
