package media_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	exportff "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/ffmpeg"
	tool "github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

func TestTimelineRendererCreatesActualCompositionAndBurnedChinese(t *testing.T) {
	renderer, err := exportff.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	imageID, audioID := uuid.New(), uuid.New()
	visual, audio, subtitle := uuid.New(), uuid.New(), uuid.New()
	config := canvasdomain.TimelineConfig{Version: 1, AspectRatio: "16:9", FPS: 30, SubtitleStyle: canvasdomain.SubtitleStyle{FontSize: 48, Color: "#00ff00", Position: "bottom"}, Tracks: []canvasdomain.TimelineTrack{
		{ID: visual, Kind: "image", Label: "图片", Visible: true}, {ID: audio, Kind: "audio", Label: "音频", Visible: true}, {ID: subtitle, Kind: "subtitle", Label: "字幕", Visible: true},
	}, Clips: []canvasdomain.TimelineClip{
		{ID: uuid.New(), TrackID: visual, Kind: "image", AssetID: &imageID, Title: "图片1", StartMS: 0, DurationMS: 200, Volume: 1},
		{ID: uuid.New(), TrackID: visual, Kind: "image", AssetID: &imageID, Title: "图片2", StartMS: 300, DurationMS: 200, Volume: 1},
		{ID: uuid.New(), TrackID: audio, Kind: "audio", AssetID: &audioID, Title: "声音", DurationMS: 500, Volume: 0.5, FadeInMS: 100, FadeOutMS: 100},
		{ID: uuid.New(), TrackID: subtitle, Kind: "subtitle", Title: "字幕", DurationMS: 500, Volume: 1, Text: "中文 AI\n混合文字"},
	}}
	root := t.TempDir()
	picture := filepath.Join(root, "image.png")
	var pngBytes bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	if err := png.Encode(&pngBytes, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(picture, pngBytes.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	sound := filepath.Join(root, "audio.wav")
	if data, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.5", sound).CombinedOutput(); err != nil {
		t.Fatalf("synthetic audio: %v: %s", err, data)
	}
	lastProgress := 0
	result, err := renderer.Render(t.Context(), tool.FrozenExport{Timeline: config}, map[uuid.UUID]string{imageID: picture, audioID: sound}, func(progress int, stage string) error {
		if progress < lastProgress || stage == "" {
			t.Fatalf("invalid actual export progress %d after%d", progress, lastProgress)
		}
		lastProgress = progress
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = result.Close() }()
	probe, err := (mediaflow.FFProber{}).Probe(t.Context(), result)
	if err != nil || probe.Width == nil || *probe.Width != 1920 || probe.Height == nil || *probe.Height != 1080 || probe.DurationMS == nil || *probe.DurationMS < 490 || *probe.DurationMS > 540 {
		t.Fatalf("actual export probe=%+v err=%v", probe, err)
	}
	output, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-ss", "0.25", "-i", result.File.Name(), "-frames:v", "1", "-f", "image2pipe", "-c:v", "png", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	frame, err := png.Decode(bytes.NewReader(output))
	if err != nil {
		t.Fatal(err)
	}
	green := 0
	for y := 800; y < 1080; y++ {
		for x := 400; x < 1520; x++ {
			r, g, b, _ := frame.At(x, y).RGBA()
			if g > 40000 && r < 20000 && b < 20000 {
				green++
			}
		}
	}
	if green < 100 {
		t.Fatalf("Chinese/mixed multiline captions were not actually burned: green pixels=%d", green)
	}
	data, err := exec.CommandContext(t.Context(), "ffprobe", "-v", "error", "-show_entries", "stream=codec_type", "-of", "json", result.File.Name()).Output()
	if err != nil {
		t.Fatal(err)
	}
	var streams struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
		} `json:"streams"`
	}
	if json.Unmarshal(data, &streams) != nil {
		t.Fatal("actual output streams")
	}
	hasAudio := false
	for _, s := range streams.Streams {
		hasAudio = hasAudio || s.CodecType == "audio"
	}
	if !hasAudio || lastProgress < 80 {
		t.Fatal("actual audio mix/progress missing")
	}
	pcm, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-i", result.File.Name(), "-map", "0:a", "-ac", "1", "-ar", "48000", "-f", "s16le", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	rms := func(from, to int) float64 {
		start, end := from*48, to*48
		if len(pcm) < end*2 {
			t.Fatal("actual audio samples truncated")
		}
		var energy float64
		for i := start; i < end; i++ {
			sample := float64(int16(binary.LittleEndian.Uint16(pcm[i*2 : i*2+2])))
			energy += sample * sample
		}
		return math.Sqrt(energy / float64(end-start))
	}
	middle := rms(210, 260)
	if middle < 100 || rms(0, 25) > middle*0.6 || rms(470, 495) > middle*0.6 {
		t.Fatalf("actual audio gain/fades missing: first=%f middle=%f last=%f", rms(0, 25), middle, rms(470, 495))
	}
}
