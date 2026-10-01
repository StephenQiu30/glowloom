package media_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/videodepth"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

func depthTestVideo(t *testing.T, rate, duration string) *mediaapp.Downloaded {
	t.Helper()
	name := filepath.Join(t.TempDir(), "source.mp4")
	cmd := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate="+rate+":duration="+duration, "-c:v", "libx264", "-pix_fmt", "yuv420p", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("actual source: %v %s", err, out)
	}
	return depthOpenTestFile(t, name)
}

func depthOpenTestFile(t *testing.T, name string) *mediaapp.Downloaded {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(data)
	input := &mediaapp.Downloaded{File: f, Size: int64(len(data)), SHA256: hex.EncodeToString(h[:]), MIMEType: "video/mp4"}
	t.Cleanup(func() { _ = input.Close() })
	return input
}

func TestDepthPreprocessPreservesVariableTimestampsAndFullBoundary(t *testing.T) {
	p, err := videodepth.NewPreprocessor("ffmpeg", "ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("variable-timestamps", func(t *testing.T) {
		name := filepath.Join(t.TempDir(), "variable.mp4")
		args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=60:duration=2", "-vf", "select='if(lt(t,1),not(mod(n,2)),not(mod(n,3)))'", "-fps_mode", "vfr", "-c:v", "libx264", "-pix_fmt", "yuv420p", name}
		if data, err := exec.CommandContext(t.Context(), "ffmpeg", args...).CombinedOutput(); err != nil {
			t.Fatalf("actual VFR: %v %s", err, data)
		}
		input := depthOpenTestFile(t, name)
		before := depthFrameTimestamps(t, name)
		firstDelta := before[1] - before[0]
		variable := false
		for i := 2; i < len(before); i++ {
			variable = variable || math.Abs((before[i]-before[i-1])-firstDelta) > 0.001
		}
		if !variable {
			t.Fatal("test source did not have actual variable timestamps")
		}
		output, err := p.Prepare(t.Context(), input, input.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = output.File.Close() }()
		after := depthFrameTimestamps(t, output.File.File.Name())
		for i := 1; i < len(after); i++ {
			if math.Abs(after[i]-after[i-1]-1/output.Prepared.FPS) > 0.0001 {
				t.Fatalf("not actual CFR: frame %d timestamps %v", i, after)
			}
		}
		if math.Abs(float64(output.Prepared.DurationMS-output.Source.DurationMS)) > math.Ceil(1000/output.Prepared.FPS)+2 || output.Prepared.FrameCount != len(after) {
			t.Fatalf("VFR duration/frame count not preserved: %+v", output)
		}
	})
	t.Run("fifteen-point-one-seconds", func(t *testing.T) {
		input := depthTestVideo(t, "30", "15.1")
		output, err := p.Prepare(t.Context(), input, input.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = output.File.Close() }()
		if output.Prepared.FrameCount != 453 || output.Prepared.DurationMS != 15100 {
			t.Fatalf("boundary truncated: %+v", output.Prepared)
		}
	})
}

func depthFrameTimestamps(t *testing.T, file string) []float64 {
	t.Helper()
	data, err := exec.CommandContext(t.Context(), "ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "frame=best_effort_timestamp_time", "-of", "json", file).Output()
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Frames []struct {
			Time string `json:"best_effort_timestamp_time"`
		} `json:"frames"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Frames) < 2 {
		t.Fatal("missing actual frame timestamps")
	}
	values := make([]float64, len(result.Frames))
	for i, frame := range result.Frames {
		value, err := strconv.ParseFloat(frame.Time, 64)
		if err != nil {
			t.Fatal(err)
		}
		values[i] = value
	}
	return values
}

func TestDepthPreprocessRejectsCancelledAndOversizedSourceBeforeAV(t *testing.T) {
	p, err := videodepth.NewPreprocessor("ffmpeg", "ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	input := depthTestVideo(t, "30", "1")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if output, err := p.Prepare(ctx, input, input.SHA256); output != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled input produced output: %v %+v", err, output)
	}
	oversized := *input
	oversized.Size = 500<<20 + 1
	if output, err := p.Prepare(t.Context(), &oversized, input.SHA256); output != nil || !errors.Is(err, toolapp.ErrDepthBudgetExceeded) {
		t.Fatalf("size budget produced output: %v %+v", err, output)
	}
}

func TestDepthPreprocessPreservesCompleteFiftyFPSVideo(t *testing.T) {
	input := depthTestVideo(t, "50", "2")
	p, err := videodepth.NewPreprocessor("ffmpeg", "ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Prepare(t.Context(), input, input.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = got.File.Close() }()
	if got.Source.FrameCount != 100 || got.Prepared.FrameCount != 60 || got.Prepared.DurationMS != 2000 || got.Prepared.FPS != 30 {
		t.Fatalf("CFR lost duration or frames: %+v", got)
	}
	if got.File.SHA256 == "" || got.File.Size < 1 {
		t.Fatal("unverified CFR bytes")
	}
}

func TestDepthPreprocessPreservesThirtyFPSVideo(t *testing.T) {
	input := depthTestVideo(t, "30", "2")
	p, err := videodepth.NewPreprocessor("ffmpeg", "ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Prepare(t.Context(), input, input.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = got.File.Close() }()
	if got.Prepared.FrameCount != 60 || got.Prepared.DurationMS != 2000 {
		t.Fatalf("ordinary CFR changed: %+v", got)
	}
}

func TestDepthPreprocessFractionalFPSAndBudgetRejection(t *testing.T) {
	p, err := videodepth.NewPreprocessor("ffmpeg", "ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	input := depthTestVideo(t, "60000/1001", "2")
	got, err := p.Prepare(t.Context(), input, input.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = got.File.Close() }()
	if got.Prepared.FrameCount < 60 || got.Prepared.FrameCount > 61 || got.Prepared.DurationMS < 2000 || got.Prepared.DurationMS > 2040 {
		t.Fatalf("fractional source truncated: %+v", got)
	}
	if _, err = p.Prepare(t.Context(), input, "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"); !errors.Is(err, toolapp.ErrInvalidDepthInput) {
		t.Fatalf("unchecked source SHA: %v", err)
	}
	long := depthTestVideo(t, "30", "15.2")
	if _, err = p.Prepare(t.Context(), long, long.SHA256); !errors.Is(err, toolapp.ErrDepthBudgetExceeded) {
		t.Fatalf("accepted long clip: %v", err)
	}
}
