package media_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	exportff "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/ffmpeg"
	exportdomain "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

func TestTimelineExplicitAspectsRenderAndProbeActualVideoAndProxy(t *testing.T) {
	renderer, err := exportff.NewRenderer(mediaflow.FFProber{})
	if err != nil {
		t.Fatal(err)
	}
	for _, aspect := range []string{"2.35:1", "21:9", "1.85:1", "16:9", "3:2", "4:3", "1:1", "3:4", "2:3", "9:16"} {
		t.Run(aspect, func(t *testing.T) {
			track := uuid.New()
			config := canvasdomain.TimelineConfig{
				Version: 1, AspectRatio: aspect, FPS: 25,
				Tracks:        []canvasdomain.TimelineTrack{{ID: track, Kind: "subtitle", Label: "字幕", Visible: true}},
				Clips:         []canvasdomain.TimelineClip{{ID: uuid.New(), TrackID: track, Kind: "subtitle", Title: "比例验收", Text: "比例", DurationMS: 120, Volume: 1}},
				SubtitleStyle: canvasdomain.SubtitleStyle{FontSize: 36, Color: "#ffffff", Position: "bottom"},
			}
			result, err := renderer.Render(t.Context(), exportdomain.FrozenExport{Timeline: config}, nil, nil)
			if err != nil {
				t.Fatal("actual MP4 render", err)
			}
			defer func() { _ = result.Close() }()
			probe, err := (mediaflow.FFProber{}).Probe(t.Context(), result)
			width, height, sizeErr := config.VideoDimensions()
			if err != nil || sizeErr != nil || probe.Width == nil || probe.Height == nil || int(*probe.Width) != width || int(*probe.Height) != height {
				t.Fatal("actual video has wrong aspect dimensions", probe, err, sizeErr)
			}
			renditions, err := (mediaflow.FFRenderer{}).Render(t.Context(), result, probe, aspect)
			if err != nil {
				t.Fatal("preview renderer rejected supported aspect", err)
			}
			defer closeRenditions(renditions)
			found := false
			for _, rendition := range renditions {
				if rendition.Kind != mediadomain.RenditionProxy720p {
					continue
				}
				found = true
				if int(rendition.Width)*height != int(rendition.Height)*width || min(rendition.Width, rendition.Height) != 720 {
					t.Fatal("proxy changed composition aspect", rendition.Width, rendition.Height)
				}
			}
			if !found {
				t.Fatal("video preview missing")
			}
		})
	}
	if _, err := renderer.Render(t.Context(), exportdomain.FrozenExport{Timeline: canvasdomain.TimelineConfig{AspectRatio: "auto"}}, nil, nil); !errors.Is(err, canvasdomain.ErrTimelineAspectUnresolved) {
		t.Fatal("unresolved automatic aspect silently rendered as landscape", err)
	}
}
