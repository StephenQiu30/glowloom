package domain

import (
	"encoding/json"
	"slices"

	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

func isReferenceCollection(path []string) bool {
	return slices.Equal(path, []string{"config", "draft", "composer", "references"})
}
func normalizeReferenceCollection(object map[string]any, op Operation) error {
	parent, _, err := fieldContainer(object, op.Path)
	if err != nil {
		return err
	}
	refs, _ := parent["references"].([]any)
	body, _ := json.Marshal(parent["labelState"])
	var labels prompt.LabelState
	if json.Unmarshal(body, &labels) != nil || labels.HighWater == nil || labels.Allocations == nil {
		return operationError("invalid_labels")
	}
	if op.Type == "insert" && op.labelBase == nil {
		if _, used := labels.Allocations[op.ItemID]; used {
			return operationError("reference_id_reused")
		}
	}
	if op.labelBase != nil {
		labels = prompt.LabelState{}
		body, _ = json.Marshal(op.labelBase)
		if json.Unmarshal(body, &labels) != nil {
			return operationError("invalid_labels")
		}
	}
	ordered := slices.Clone(op.labelOrder)
	for i, value := range refs {
		ref := value.(map[string]any)
		ref["order"] = i
		id := itemIdentity(ref)
		if !slices.Contains(ordered, id) {
			ordered = append(ordered, id)
		}
	}
	for _, id := range ordered {
		index := collectionIndex(refs, id)
		if index < 0 {
			continue
		}
		ref := refs[index].(map[string]any)
		kind, _ := ref["mediaKind"].(string)
		source, _ := ref["source"].(map[string]any)
		if source["kind"] == "subject" {
			kind = "subject"
		}
		if allocation, ok := labels.Allocations[id]; ok {
			if allocation.LabelKind != kind {
				return operationError("label_kind_changed")
			}
			continue
		}
		if len(labels.Allocations) >= 10000 || labels.HighWater[kind] >= 9007199254740991 {
			return operationError("label_limit")
		}
		labels.HighWater[kind]++
		labels.Allocations[id] = prompt.LabelAllocation{LabelKind: kind, Ordinal: labels.HighWater[kind]}
	}
	if op.Type == "insert" {
		if index := collectionIndex(refs, op.ItemID); index >= 0 {
			refs[index].(map[string]any)["bindingGeneration"] = 1
		}
	}
	if op.Type == "restore_item" {
		index := collectionIndex(refs, op.ItemID)
		if index < 0 {
			return operationError("undo_conflict")
		}
		ref := refs[index].(map[string]any)
		var generation int64
		if json.Unmarshal(effectJSON(ref["bindingGeneration"]), &generation) != nil || generation < 1 || generation >= 9007199254740991 {
			return operationError("binding_generation_limit")
		}
		ref["bindingGeneration"] = generation + 1
	}
	if op.Type == "remove" {
		document, _ := parent["document"].(map[string]any)
		paragraphs, _ := document["content"].([]any)
		for _, value := range paragraphs {
			paragraph := value.(map[string]any)
			content, _ := paragraph["content"].([]any)
			content = slices.DeleteFunc(content, func(item any) bool {
				inline, ok := item.(map[string]any)
				if !ok || inline["type"] != "reference" {
					return false
				}
				attrs, _ := inline["attrs"].(map[string]any)
				return attrs["referenceId"] == op.ItemID
			})
			if len(content) == 0 {
				delete(paragraph, "content")
			} else {
				paragraph["content"] = content
			}
		}
	}
	body, _ = json.Marshal(labels)
	var value any
	if json.Unmarshal(body, &value) != nil {
		return operationError("invalid_labels")
	}
	parent["labelState"] = value
	return nil
}
func normalizeBinding(object map[string]any, op Operation) error {
	if len(op.Path) != 6 || !isReferenceCollection(op.Path[:4]) || op.Path[5] != "binding" {
		return nil
	}
	parent, _, err := fieldContainer(object, op.Path)
	if err != nil {
		return err
	}
	var previous map[string]any
	if json.Unmarshal(op.Expected, &previous) != nil {
		return operationError("field_conflict")
	}
	generation, ok := previous["bindingGeneration"].(float64)
	if !ok || generation < 1 || generation >= 9007199254740991 {
		return operationError("binding_generation_limit")
	}
	parent["bindingGeneration"] = generation + 1
	return nil
}
