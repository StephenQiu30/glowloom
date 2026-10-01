package media_test

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"

	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	exportff "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/ffmpeg"
	exportdomain "github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

func TestExportCaptionPixelsPreserveChineseStyleOutlineAndBackground(t *testing.T) {
	renderer, err := exportff.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	picture := filepath.Join(t.TempDir(), "gray.png")
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			img.SetRGBA(x, y, color.RGBA{R: 180, G: 180, B: 180, A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(picture, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	for _, position := range []string{"top", "center", "bottom"} {
		t.Run(position, func(t *testing.T) {
			frame := exportedCaptionFrame(t, renderer, picture, position, "中文 AI\n混合文字")
			var bounds image.Rectangle
			green, stroke, background := 0, 0, 0
			for y := range 1080 {
				for x := range 1080 {
					r, g, b, _ := frame.At(x, y).RGBA()
					r, g, b = r>>8, g>>8, b>>8
					if r < 60 && g > 150 && b < 60 {
						green++
						bounds = bounds.Union(image.Rect(x, y, x+1, y+1))
					}
					if r < 70 && g < 70 && b < 70 {
						stroke++
					}
					if r >= 168 && r <= 190 && g >= 168 && g <= 190 && b >= 168 && b <= 190 {
						background++
					}
				}
			}
			if green < 200 || stroke < 100 || background < 1000000 || bounds.Empty() {
				t.Fatalf("actual rendered Chinese/fill/stroke/transparent overlay: green=%d stroke=%d background=%d", green, stroke, background)
			}
			if middle := (bounds.Min.X + bounds.Max.X) / 2; middle < 510 || middle > 570 {
				t.Fatalf("actual caption is not centered: %v", bounds)
			}
			switch position {
			case "top":
				if bounds.Max.Y > 250 {
					t.Fatal(bounds)
				}
			case "center":
				if bounds.Min.Y < 400 || bounds.Max.Y > 680 {
					t.Fatal(bounds)
				}
			case "bottom":
				if bounds.Min.Y < 800 {
					t.Fatal(bounds)
				}
			}
		})
	}
	wrapped := exportedCaptionFrame(t, renderer, picture, "top", strings.Repeat("中文 mixed ", 25))
	greenBelowFirstLines := 0
	for y := 160; y < 600; y++ {
		for x := 48; x < 1032; x++ {
			r, g, b, _ := wrapped.At(x, y).RGBA()
			if r < 15000 && g > 40000 && b < 15000 {
				greenBelowFirstLines++
			}
		}
	}
	if greenBelowFirstLines < 100 {
		t.Fatal("long mixed Chinese/Latin caption was not actually wrapped")
	}
}

func exportedCaptionFrame(t *testing.T, renderer *exportff.Renderer, picture, position, text string) image.Image {
	t.Helper()
	asset, visual, captions := uuid.New(), uuid.New(), uuid.New()
	config := canvasdomain.TimelineConfig{Version: 1, AspectRatio: "1:1", FPS: 25, SubtitleStyle: canvasdomain.SubtitleStyle{FontSize: 36, Color: "#00ff00", Position: position}, Tracks: []canvasdomain.TimelineTrack{{ID: visual, Kind: "image", Label: "Background", Visible: true}, {ID: captions, Kind: "subtitle", Label: "Captions", Visible: true}}, Clips: []canvasdomain.TimelineClip{{ID: uuid.New(), TrackID: visual, Kind: "image", AssetID: &asset, Title: "Background", DurationMS: 120, Volume: 1}, {ID: uuid.New(), TrackID: captions, Kind: "subtitle", Title: "Captions", Text: text, DurationMS: 120, Volume: 1}}}
	result, err := renderer.Render(t.Context(), exportdomain.FrozenExport{Timeline: config}, map[uuid.UUID]string{asset: picture}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = result.Close() }()
	data, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-i", result.File.Name(), "-frames:v", "1", "-f", "image2pipe", "-c:v", "png", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	frame, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

func TestExportLongCaptionTimelinePlansWithoutDurationSizedAllocation(t *testing.T) {
	renderer, err := exportff.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	lane := uuid.New()
	config := canvasdomain.TimelineConfig{Version: 1, AspectRatio: "1:1", FPS: 60, Tracks: []canvasdomain.TimelineTrack{{ID: lane, Kind: "subtitle", Label: "Captions", Visible: true}}, Clips: []canvasdomain.TimelineClip{{ID: uuid.New(), TrackID: lane, Kind: "subtitle", Title: "Captions", Text: "中文 AI", DurationMS: 86400000, Volume: 1}}, SubtitleStyle: canvasdomain.SubtitleStyle{FontSize: 24, Color: "#ffffff", Position: "bottom"}}
	stop := errors.New("stop after bounded export planning")
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	calls := 0
	result, err := renderer.Render(t.Context(), exportdomain.FrozenExport{Timeline: config}, nil, func(progress int, stage string) error {
		calls++
		runtime.ReadMemStats(&after)
		if progress != 20 || stage != "rendering" {
			t.Fatalf("unexpected first real render boundary %d %s", progress, stage)
		}
		return stop
	})
	if result != nil {
		_ = result.Close()
	}
	if !errors.Is(err, stop) || calls != 1 || after.TotalAlloc-before.TotalAlloc > 16<<20 {
		t.Fatalf("24 hour export planning expanded duration-sized state: calls=%d allocated=%d err=%v", calls, after.TotalAlloc-before.TotalAlloc, err)
	}
}
