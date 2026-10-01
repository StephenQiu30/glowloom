package media_test

import (
	"errors"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/videodepth"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

func TestDepthCanonicalOutputRequiresPlayableSilentFaststart420MP4(t *testing.T) {
	p, err := videodepth.NewPreprocessor("ffmpeg", "ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, pixel string
		fast, audio bool
		valid       bool
	}{
		{name: "canonical", pixel: "yuv420p", fast: true, valid: true},
		{name: "moov-after-data", pixel: "yuv420p"},
		{name: "retained-audio", pixel: "yuv420p", fast: true, audio: true},
		{name: "444-output", pixel: "yuv444p", fast: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "output.mp4")
			args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=64x36:rate=30:duration=1"}
			if tc.audio {
				args = append(args, "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=1", "-c:a", "aac")
			}
			args = append(args, "-c:v", "libx264", "-pix_fmt", tc.pixel)
			if tc.fast {
				args = append(args, "-movflags", "+faststart")
			}
			args = append(args, file)
			if data, err := exec.CommandContext(t.Context(), "ffmpeg", args...).CombinedOutput(); err != nil {
				t.Fatalf("actual MP4: %v %s", err, data)
			}
			err := p.VerifyCanonicalOutput(t.Context(), file)
			if tc.valid && err != nil {
				t.Fatal(err)
			}
			if !tc.valid && !errors.Is(err, toolapp.ErrDepthOutputInvalid) {
				t.Fatalf("accepted noncanonical real MP4: %v", err)
			}
		})
	}
}
