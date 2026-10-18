package operation_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

func TestPublicTaskTextOnlyExposesReviewedCompletedText(t *testing.T) {
	for _, tc := range []struct {
		name, capability, kind, moderation string
		status                             domain.Status
		payload                            string
		want                               bool
	}{
		{"approved", "text.generate", "json", "passed", domain.StatusCompleted, `{"text":"真实回复","provider_secret":"private"}`, true},
		{"pending", "text.generate", "json", "pending", domain.StatusCompleted, `{"text":"未审核"}`, false},
		{"not complete", "text.generate", "json", "passed", domain.StatusIngesting, `{"text":"处理中"}`, false},
		{"other capability", "image.generate", "json", "passed", domain.StatusCompleted, `{"text":"provider receipt"}`, false},
		{"invalid shape", "text.generate", "json", "passed", domain.StatusCompleted, `{"text":{"secret":1}}`, false},
		{"oversized", "text.generate", "json", "passed", domain.StatusCompleted, `{"text":"` + strings.Repeat("a", 65537) + `"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := application.TaskOutput{Kind: tc.kind, ModerationStatus: tc.moderation}
			output.IncludeReviewedText(tc.capability, tc.status, json.RawMessage(tc.payload))
			if (output.Text != nil) != tc.want {
				t.Fatalf("text exposure %v, want %v", output.Text, tc.want)
			}
			encoded, err := json.Marshal(output)
			if err != nil || strings.Contains(string(encoded), "private") {
				t.Fatal("raw provider payload exposed", err)
			}
		})
	}
}
