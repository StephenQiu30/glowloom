package domain

import "errors"

// ErrTimelineAspectUnresolved prevents an automatic selection from silently
// rendering as a fixed ratio before its source-derived dimensions are resolved.
var ErrTimelineAspectUnresolved = errors.New("timeline automatic aspect ratio is unresolved")

func validTimelineAspect(aspect string) bool {
	if aspect == "auto" {
		return true
	}
	_, _, err := timelineVideoDimensions(aspect)
	return err == nil
}

// VideoDimensions returns the existing 1080-short-edge encoding contract for
// explicit ratios. Every pair has an exact ratio and even codec dimensions.
// Automatic intent is persistable but cannot be guessed at rendering time.
func (c TimelineConfig) VideoDimensions() (int, int, error) {
	return timelineVideoDimensions(c.AspectRatio)
}

func timelineVideoDimensions(aspect string) (int, int, error) {
	switch aspect {
	case "2.35:1":
		return 2538, 1080, nil
	case "21:9":
		return 2520, 1080, nil
	case "1.85:1":
		return 1998, 1080, nil
	case "16:9":
		return 1920, 1080, nil
	case "3:2":
		return 1620, 1080, nil
	case "4:3":
		return 1440, 1080, nil
	case "1:1":
		return 1080, 1080, nil
	case "3:4":
		return 1080, 1440, nil
	case "2:3":
		return 1080, 1620, nil
	case "9:16":
		return 1080, 1920, nil
	case "auto":
		return 0, 0, ErrTimelineAspectUnresolved
	default:
		return 0, 0, ErrInvalidCommand
	}
}
