package prompt_test

import (
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

func TestBrandChangePreservesPublishedPromptTemplateIdentities(t *testing.T) {
	expected := map[string]string{
		"chapter_assets_extract": "ceb2eb5d-bbed-57e1-b3f5-556205fcb8af",
		"storyboard_plan":        "7980a0e2-9872-55a4-ba4a-f7b76eee3af0",
		"storyboard_repair":      "bb86b654-2ea4-5b5f-8fd2-d88a6b32b071",
		"storyboard_first_frame": "69eea5a0-4c56-5f86-97a7-c607d34d3fe3",
		"storyboard_video":       "edbaf5ed-84e4-5527-a8fb-008c8b540b47",
		"character_extract":      "eae3f8e7-280a-5d9a-a030-23534f1d74ca",
		"character_turnaround":   "fd7f7f3d-258e-5a6b-85c3-49d6265c6f10",
		"short_drama_outline":    "5518a988-e170-58aa-b14b-3d147226dcba",
		"skill_draft":            "7b335bb8-461b-5cef-ba14-f1a75826cd0e",
	}
	for _, d := range domain.Definitions() {
		if d.TemplateID.String() != expected[d.Operation] {
			t.Errorf("template %s changed identity: %s", d.Operation, d.TemplateID)
		}
	}
}
