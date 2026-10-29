package canvas_test

import (
	"errors"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

var timelineAspectRatios = []string{"auto", "2.35:1", "21:9", "1.85:1", "16:9", "3:2", "4:3", "1:1", "3:4", "2:3", "9:16"}

func TestTimelineAcceptsSourceAspectRatiosAndRejectsUnknown(t *testing.T) {
	for _, aspect := range timelineAspectRatios {
		t.Run(aspect, func(t *testing.T) {
			config := timelineNode().Config.Timeline
			config.AspectRatio = aspect
			if err := config.Validate(); err != nil {
				t.Fatalf("source aspect ratio rejected: %v", err)
			}
		})
	}
	for _, aspect := range []string{"", "adaptive", "2:1", "16/9", "AUTO", "16:0", "-1:1"} {
		config := timelineNode().Config.Timeline
		config.AspectRatio = aspect
		if config.Validate() == nil {
			t.Fatalf("unknown ratio %q accepted", aspect)
		}
	}
}

func TestTimelineExplicitRatioDimensionsAreExactAndAutoUnresolved(t *testing.T) {
	for _, tc := range []struct {
		aspect        string
		width, height int
	}{
		{"2.35:1", 2538, 1080}, {"21:9", 2520, 1080}, {"1.85:1", 1998, 1080},
		{"16:9", 1920, 1080}, {"3:2", 1620, 1080}, {"4:3", 1440, 1080},
		{"1:1", 1080, 1080}, {"3:4", 1080, 1440}, {"2:3", 1080, 1620}, {"9:16", 1080, 1920},
	} {
		config := timelineNode().Config.Timeline
		config.AspectRatio = tc.aspect
		width, height, err := config.VideoDimensions()
		if err != nil || width != tc.width || height != tc.height || width%2 != 0 || height%2 != 0 {
			t.Fatalf("%s: got %dx%d: %v", tc.aspect, width, height, err)
		}
	}
	config := timelineNode().Config.Timeline
	config.AspectRatio = "auto"
	if width, height, err := config.VideoDimensions(); !errors.Is(err, domain.ErrTimelineAspectUnresolved) || width != 0 || height != 0 {
		t.Fatal("automatic ratio silently rendered at a fixed size", width, height, err)
	}
}
