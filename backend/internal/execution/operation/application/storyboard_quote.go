package application

import (
	"encoding/json"
	"strings"
)

// StoryboardShotPrompt derives the video input from the saved shot.
func StoryboardShotPrompt(title, action, dialogue, camera, sound string) string {
	lines := []string{"镜头：" + strings.TrimSpace(title), "动作：" + strings.TrimSpace(action)}
	for _, optional := range []struct{ label, value string }{
		{"对白：", dialogue},
		{"运镜：", camera},
		{"声音：", sound},
	} {
		if value := strings.TrimSpace(optional.value); value != "" {
			lines = append(lines, optional.label+value)
		}
	}
	return strings.Join(lines, "\n")
}

// StoryboardShotDurationMatches checks the request's exact saved duration.
func StoryboardShotDurationMatches(durationMS int64, params json.RawMessage) bool {
	var requested struct {
		DurationMS int64 `json:"duration_ms"`
	}
	return json.Unmarshal(params, &requested) == nil && requested.DurationMS > 0 && requested.DurationMS == durationMS
}
