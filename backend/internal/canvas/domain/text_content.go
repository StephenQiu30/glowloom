package domain

import (
	"encoding/json"
	"strings"

	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// retainTextLabels keeps tombstones even when undo restores pre-Tiptap text.
// A full content save may remove bindings, but cannot reassign their numbers.
func retainTextLabels(parent map[string]any, before json.RawMessage) error {
	var old, next NodeConfig
	body, err := json.Marshal(parent)
	if err != nil || json.Unmarshal(before, &old) != nil || json.Unmarshal(body, &next) != nil {
		return operationError("invalid_operation")
	}
	if old.Composer == nil {
		return nil
	}
	if next.Composer == nil {
		if !validConfig("text", next) {
			return operationError("invalid_operation")
		}
		paragraphs := make([]prompt.Paragraph, 0)
		for _, line := range strings.Split(*next.Text, "\n") {
			paragraph := prompt.Paragraph{Type: "paragraph"}
			if line != "" {
				paragraph.Content = []prompt.Inline{{Type: "text", Text: line}}
			}
			paragraphs = append(paragraphs, paragraph)
		}
		next.Composer = &prompt.Draft{SchemaVersion: 1, Document: prompt.Document{Type: "doc", Content: paragraphs}, References: []prompt.Reference{}, Params: map[string]any{}, LabelState: prompt.LabelState{HighWater: map[string]int64{"image": 0, "video": 0, "audio": 0, "subject": 0}, Allocations: map[string]prompt.LabelAllocation{}}}
		next.Text = nil
		next.Format, next.FormatVersion = "tiptap", 1
	}
	if !validConfig("text", next) {
		return operationError("invalid_operation")
	}
	for id, allocation := range old.Composer.LabelState.Allocations {
		if current, ok := next.Composer.LabelState.Allocations[id]; ok && current != allocation {
			return operationError("label_allocation_conflict")
		}
		next.Composer.LabelState.Allocations[id] = allocation
	}
	for kind, high := range old.Composer.LabelState.HighWater {
		next.Composer.LabelState.HighWater[kind] = max(high, next.Composer.LabelState.HighWater[kind])
	}
	if !validConfig("text", next) {
		return operationError("label_allocation_conflict")
	}
	body, err = json.Marshal(next)
	if err != nil {
		return err
	}
	clear(parent)
	return json.Unmarshal(body, &parent)
}
