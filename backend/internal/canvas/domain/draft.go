package domain

import (
	"unicode/utf8"
)

// Draft holds editable intent; no provider execution or candidate state belongs here.
type Draft struct {
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negative_prompt"`
	AspectRatio    string `json:"aspect_ratio"`
	CandidateCount int    `json:"candidate_count"`
}

// IsDraft identifies the two locally editable generation intents.
func IsDraft(n Node) bool {
	return n.NodeType == "image_generation" || n.NodeType == "video_generation"
}

func validDraft(d Draft) bool {
	return utf8.ValidString(d.Prompt) && utf8.RuneCountInString(d.Prompt) <= 10000 &&
		utf8.ValidString(d.NegativePrompt) && utf8.RuneCountInString(d.NegativePrompt) <= 10000 &&
		(d.AspectRatio == "1:1" || d.AspectRatio == "9:16" || d.AspectRatio == "16:9") && d.CandidateCount >= 1 && d.CandidateCount <= 8
}
