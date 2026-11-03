package application

import (
	"encoding/json"
	"strings"
)

// seedanceQuoteTokens estimates the documented video token quantity. Actual
// settlement always uses the supplier's completion_tokens, never this estimate.
func seedanceQuoteTokens(model, mode string, params map[string]any, media []FreeQuoteMediaFact) (int64, error) {
	if !strings.HasPrefix(model, "doubao-seedance-2-0-") && !strings.HasPrefix(model, "doubao-seedance-2-5-") {
		return 0, ErrFreeQuoteInputNotReady
	}
	for _, fact := range media {
		if fact.Kind == "video" {
			return 0, ErrFreeQuoteInputNotReady
		}
	}
	duration, ok := params["duration_ms"].(json.Number)
	if !ok {
		return 0, ErrFreeQuoteInputNotReady
	}
	ms, err := duration.Int64()
	if err != nil || ms < 4000 || ms > 30000 || ms%1000 != 0 {
		return 0, ErrFreeQuoteInputNotReady
	}
	resolution, _ := params["resolution"].(string)
	ratio, _ := params["aspect_ratio"].(string)
	if mode == "image2video" && strings.HasPrefix(model, "doubao-seedance-2-5-") {
		// The API requires adaptive ratio here. Reserve the largest documented area.
		ratio = "adaptive"
	}
	dimensions := map[string]map[string][2]int64{
		"480p":  {"16:9": {864, 496}, "9:16": {496, 864}, "1:1": {640, 640}, "4:3": {752, 560}, "3:4": {560, 752}, "21:9": {992, 432}},
		"720p":  {"16:9": {1280, 720}, "9:16": {720, 1280}, "1:1": {960, 960}, "4:3": {1112, 834}, "3:4": {834, 1112}, "21:9": {1470, 630}},
		"1080p": {"16:9": {1920, 1080}, "9:16": {1080, 1920}, "1:1": {1440, 1440}, "4:3": {1664, 1248}, "3:4": {1248, 1664}, "21:9": {2206, 946}},
	}
	if strings.HasPrefix(model, "doubao-seedance-2-5-") {
		dimensions["480p"]["16:9"] = [2]int64{854, 480}
		dimensions["480p"]["9:16"] = [2]int64{480, 854}
	}
	sizes, ok := dimensions[resolution]
	if !ok {
		return 0, ErrFreeQuoteInputNotReady
	}
	pixels := int64(0)
	if ratio == "adaptive" && mode == "image2video" {
		for _, size := range sizes {
			if area := size[0] * size[1]; area > pixels {
				pixels = area
			}
		}
	} else if size, exists := sizes[ratio]; exists {
		pixels = size[0] * size[1]
	} else {
		return 0, ErrFreeQuoteInputNotReady
	}
	numerator := pixels * 24 * (ms / 1000)
	return (numerator + 1023) / 1024, nil
}
