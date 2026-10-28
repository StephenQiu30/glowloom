package application

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// IncludeReviewedText exposes a bounded, reviewed text result, never the provider payload.
func (o *TaskOutput) IncludeReviewedText(capability string, status domain.Status, payload json.RawMessage) {
	o.Text = nil
	if (!strings.HasPrefix(capability, "text.") && !strings.HasPrefix(capability, "document.")) || status != domain.StatusCompleted || o.Kind != "json" || o.ModerationStatus != "passed" || len(payload) > 1<<20 {
		return
	}
	var result struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(payload, &result) != nil || !utf8.ValidString(result.Text) || len(result.Text) > 65536 || strings.TrimSpace(result.Text) == "" {
		return
	}
	o.Text = &result.Text
}
