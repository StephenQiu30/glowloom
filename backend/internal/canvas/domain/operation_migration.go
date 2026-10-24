package domain

import (
	"encoding/json"
	"strings"

	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

func migrateComposer(document *Document, op Operation) (FieldChange, error) {
	index := nodeIndex(document, op.NodeID)
	if index < 0 {
		return FieldChange{}, operationError("target_not_found")
	}
	n := &document.Nodes[index]
	if !IsDraft(*n) || !editable(*n) || n.Config.Draft == nil {
		return FieldChange{}, operationError("protected_target")
	}
	before, _ := json.Marshal(n.Config.Draft)
	if n.Config.Draft.Composer != nil || !equalJSON(before, op.Expected) {
		return FieldChange{}, operationError("field_conflict")
	}
	old := n.Config.Draft
	paragraphs := make([]prompt.Paragraph, 0, strings.Count(old.Prompt, "\n")+1)
	for _, line := range strings.Split(old.Prompt, "\n") {
		p := prompt.Paragraph{Type: "paragraph"}
		if line != "" {
			p.Content = []prompt.Inline{{Type: "text", Text: line}}
		}
		paragraphs = append(paragraphs, p)
	}
	params := map[string]any{"aspect_ratio": old.AspectRatio, "output_count": float64(old.CandidateCount)}
	if old.NegativePrompt != "" {
		params["negative_prompt"] = old.NegativePrompt
	}
	n.Config.Draft = &Draft{Composer: &prompt.Draft{SchemaVersion: 1, Document: prompt.Document{Type: "doc", Content: paragraphs}, References: []prompt.Reference{}, Params: params, LabelState: prompt.LabelState{HighWater: map[string]int64{"image": 0, "video": 0, "audio": 0, "subject": 0}, Allocations: map[string]prompt.LabelAllocation{}}}}
	after, _ := json.Marshal(n.Config.Draft)
	return FieldChange{NodeID: n.ID, Path: []string{"config", "draft"}, Before: before, After: after, BeforeExists: true, AfterExists: true}, nil
}
