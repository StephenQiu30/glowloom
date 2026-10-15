package domain

import (
	"unicode/utf8"

	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// Draft holds editable intent; no provider execution or candidate state belongs here.
type Draft struct {
	Composer       *promptdomain.Draft `json:"composer,omitempty"`
	Prompt         string              `json:"prompt"`
	NegativePrompt string              `json:"negative_prompt"`
	AspectRatio    string              `json:"aspect_ratio"`
	CandidateCount int                 `json:"candidate_count"`
}

// IsDraft identifies the two locally editable generation intents.
func IsDraft(n Node) bool {
	return n.NodeType == "image_generation" || n.NodeType == "video_generation"
}

func validDraft(d Draft) bool {
	if d.Composer != nil {
		return d.Prompt == "" && d.NegativePrompt == "" && d.AspectRatio == "" && d.CandidateCount == 0 && d.Composer.Validate() == nil
	}
	return utf8.ValidString(d.Prompt) && utf8.RuneCountInString(d.Prompt) <= 10000 &&
		utf8.ValidString(d.NegativePrompt) && utf8.RuneCountInString(d.NegativePrompt) <= 10000 &&
		(d.AspectRatio == "1:1" || d.AspectRatio == "9:16" || d.AspectRatio == "16:9") && d.CandidateCount >= 1 && d.CandidateCount <= 8
}

// SameInput ignores inactive label tombstones, but includes every generation choice.
func SameInput(a, b Draft) bool {
	if a.Composer != nil && b.Composer != nil {
		_, ah := a.Composer.Hashes()
		_, bh := b.Composer.Hashes()
		return ah == bh
	}
	return a.Composer == nil && b.Composer == nil && a.Prompt == b.Prompt && a.NegativePrompt == b.NegativePrompt && a.AspectRatio == b.AspectRatio && a.CandidateCount == b.CandidateCount
}
