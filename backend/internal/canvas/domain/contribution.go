package domain

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// ContributionBatch is trusted continuous accepted history, including the
// original causal request so a deletion transformed to noop remains explainable.
type ContributionBatch struct {
	Revision     int64
	ActorID      uuid.UUID
	OpID         string
	UndoOf       string
	OriginalBase int64
	Deletions    []Operation // Raw deletion intent, including known-removal normalization.
	Original     []Operation
	Operations   []Operation
	Effects      []Effect
	Touches      []FieldChange
}

func readOperationSequence(doc Document, op Operation) ([]Atom, error) {
	index := nodeIndex(&doc, op.NodeID)
	if index < 0 {
		return nil, operationError("target_deleted")
	}
	object, err := nodeObject(doc.Nodes[index])
	if err != nil {
		return nil, err
	}
	parent, key, err := fieldContainer(object, op.Path)
	if err != nil {
		return nil, err
	}
	rule, ok := policy(op.Path)
	if !ok {
		return nil, operationError("unsupported_field")
	}
	value, exists := fieldValue(parent, key, rule)
	value, exists = textValue(op.Path, value, exists)
	if !exists {
		return nil, operationError("target_deleted")
	}
	return operationSequence(value, key == "document")
}
func inverseField(doc Document, op Operation) (Operation, error) {
	index := nodeIndex(&doc, op.NodeID)
	if index < 0 {
		return Operation{}, operationError("undo_unavailable")
	}
	object, err := nodeObject(doc.Nodes[index])
	if err != nil {
		return Operation{}, err
	}
	if isBatchSlot(op.Path) {
		value, err := batchSlotValue(object, op.Path)
		if err != nil {
			return Operation{}, err
		}
		return Operation{Type: "set", NodeID: op.NodeID, Path: slices.Clone(op.Path), Value: value}, nil
	}
	parent, key, err := fieldContainer(object, op.Path)
	if err != nil {
		return Operation{}, err
	}
	rule, ok := policy(op.Path)
	if !ok {
		return Operation{}, operationError("undo_unavailable")
	}
	before, exists := fieldValue(parent, key, rule)
	out := Operation{Type: op.Type, NodeID: op.NodeID, Path: slices.Clone(op.Path)}
	if rule.context != "" {
		out.Context, err = fieldContext(object, op.Path, rule)
		if err != nil {
			return Operation{}, err
		}
	}
	if op.Type == "text" {
		textBefore, _ := textValue(op.Path, before, exists)
		sequence, err := operationSequence(textBefore, key == "document")
		if err != nil {
			return Operation{}, err
		}
		out.Text, err = InvertSequence(sequence, op.Text)
		return out, err
	}
	after, err := applyOperations(doc, []Operation{op}, false)
	if err != nil {
		return Operation{}, err
	}
	next, err := nodeObject(after.Document.Nodes[nodeIndex(&after.Document, op.NodeID)])
	if err != nil {
		return Operation{}, err
	}
	nextParent, nextKey, err := fieldContainer(next, op.Path)
	if err != nil {
		return Operation{}, err
	}
	value, afterExists := fieldValue(nextParent, nextKey, rule)
	out.Value = before
	out.Unset = !exists
	out.Expected = value
	out.ExpectedExists = &afterExists
	if rule.context != "" {
		out.Context, err = fieldContext(next, op.Path, rule)
	}
	return out, err
}

// UndoContribution derives and rebases a new operation from the target's actual
// contribution. It never restores a graph snapshot or trusts a client inverse.
func UndoContribution(floor Document, history []ContributionBatch, target string, actor uuid.UUID) ([]Operation, error) {
	targetIndex := slices.IndexFunc(history, func(b ContributionBatch) bool { return b.OpID == target && b.ActorID == actor })
	if targetIndex < 0 {
		return nil, operationError("undo_unavailable")
	}
	// Charge the work actually performed: replay, lineage per touched text field,
	// and inverse pairs against later operations, rather than squaring all typing.
	ownCount, later, replay := len(history[targetIndex].Operations), 0, 0
	textPaths := map[string]bool{}
	for _, op := range history[targetIndex].Operations {
		if op.Type == "text" {
			textPaths[op.NodeID.String()+strings.Join(op.Path, "/")] = true
		}
	}
	for i, row := range history {
		replay += len(row.Operations) + len(row.Original) + len(row.Deletions)
		if i > targetIndex {
			later += len(row.Operations)
		}
	}
	if err := ValidateTransformWork(floor, replay*(1+len(textPaths))+ownCount*(1+later)); err != nil {
		return nil, err
	}

	for _, later := range history[targetIndex+1:] {
		if later.UndoOf == target {
			return nil, operationError("undo_conflict")
		}
	}
	targetBase := floor
	var err error
	for _, row := range history[:targetIndex] {
		targetBase, err = ApplyEffects(targetBase, row.Effects)
		if err != nil {
			return nil, operationError("undo_unavailable")
		}
		targetBase.Revision = row.Revision
	}
	own := history[targetIndex]
	state := targetBase
	inverse := []Operation{}
	textFields := []Operation{}
	created := map[uuid.UUID]bool{}
	for _, op := range own.Operations {
		if (op.Type == "create_node" || op.Type == "restore_node") && op.Node != nil {
			created[op.Node.ID] = true
		}
	}
	for _, op := range own.Operations {
		switch op.Type {
		case "noop":
			continue
		case "migrate_composer":
			// Representation migration is retained when undoing the first user edit.
		case "text", "set":
			if created[op.NodeID] {
				break
			}
			inv, err := inverseField(state, op)
			if err != nil {
				return nil, operationError("undo_unavailable")
			}
			inverse = append([]Operation{inv}, inverse...)
			if op.Type == "text" && !slices.ContainsFunc(textFields, func(field Operation) bool { return field.NodeID == op.NodeID && slices.Equal(field.Path, op.Path) }) {
				textFields = append(textFields, op)
			}
		default:
			inv, err := inverseStructure(state, op)
			if err != nil {
				return nil, err
			}
			inverse = append(inv, inverse...)
			if referenceRemoval(op) {
				textFields = append(textFields, Operation{Type: "text", NodeID: op.NodeID, Path: []string{"config", "draft", "composer", "document"}})
			}
		}
		next, err := applyOperations(state, []Operation{op}, false)
		if err != nil {
			return nil, operationError("undo_unavailable")
		}
		state = next.Document
	}
	if len(inverse) == 0 {
		return nil, operationError("undo_unavailable")
	}
	for _, field := range textFields {
		if err := checkDeletedContribution(floor, history, targetIndex, field); err != nil {
			return nil, err
		}
	}
	// Equal-value writes still transfer ownership. Comparing current values alone
	// would miss ABA and let undo erase another editor's accepted choice.
	for _, row := range history[targetIndex+1:] {
		// Normalized/effective noops must not erase independent deletion intent.
		// Original is also inspected for trusted callers predating Deletions.
		for _, ops := range [][]Operation{row.Deletions, row.Original, row.Operations} {
			for _, deletion := range ops {
				for _, restore := range inverse {
					if deletion.Type == "delete_node" && restore.Type == "restore_node" && restore.Node != nil && deletion.NodeID == restore.Node.ID ||
						deletion.Type == "disconnect" && restore.Type == "restore_edge" && restore.Edge != nil && deletion.NodeID == restore.Edge.ID ||
						deletion.Type == "remove" && restore.Type == "restore_item" && deletion.NodeID == restore.NodeID && deletion.ItemID == restore.ItemID && slices.Equal(deletion.Path, restore.Path) {
						return nil, operationError("undo_conflict")
					}
				}
			}
		}
		for _, changed := range row.Operations {
			for _, op := range own.Operations {
				if (op.Type == "create_node" || op.Type == "restore_node" || op.Type == "delete_node") && operationUsesNode(changed, func() uuid.UUID {
					if op.Node != nil {
						return op.Node.ID
					}
					return op.NodeID
				}()) {
					return nil, operationError("undo_conflict")
				}
				if insertionOperation(op.Type) && isReferenceCollection(op.Path) && changed.NodeID == op.NodeID && insertedReference(changed.Text, op.ItemID) {
					return nil, operationError("undo_conflict")
				}
				if structureOperation(op.Type) && touchesSameItem(op, changed) {
					return nil, operationError("undo_conflict")
				}
			}
		}
	}
	for _, row := range history[targetIndex+1:] {
		for _, touch := range row.Touches {
			for _, op := range inverse {
				if touch.NodeID == op.NodeID && (op.Type == "set" && operationsOverlap(op, Operation{Type: "set", Path: touch.Path}) || op.Type == "replace_media" && slices.Equal(touch.Path, []string{"media_ref"})) {
					return nil, operationError("undo_conflict")
				}
			}
		}
	}
	state, err = ApplyEffects(targetBase, own.Effects)
	if err != nil {
		return nil, operationError("undo_unavailable")
	}
	for _, row := range history[targetIndex+1:] {
		_, inverse, err = TransformOperationPair(state, row.Operations, inverse)
		if err != nil {
			return nil, fmt.Errorf("transform undo contribution: %w: %w", operationError("undo_conflict"), err)
		}
		state, err = ApplyEffects(state, row.Effects)
		if err != nil {
			return nil, operationError("undo_unavailable")
		}
	}
	if _, err := ApplyOperations(state, inverse); err != nil {
		return nil, fmt.Errorf("validate undo contribution: %w: %w", operationError("undo_conflict"), err)
	}
	return inverse, nil
}

type contributionToken struct {
	Atom Atom
	ID   string
}

func tokenAtoms(tokens []contributionToken) []Atom {
	atoms := make([]Atom, len(tokens))
	for i, t := range tokens {
		atoms[i] = t.Atom
	}
	return appendAtoms([]Atom{}, atoms...)
}
func contributionTokens(atoms []Atom, tag string) []contributionToken {
	result := []contributionToken{}
	add := func(a Atom) { result = append(result, contributionToken{a, fmt.Sprintf("%s/%d", tag, len(result))}) }
	for _, a := range atoms {
		if a.Type == "text" {
			for _, r := range a.Text {
				add(Atom{Type: "text", Text: string(r)})
			}
		} else {
			add(a)
		}
	}
	return result
}
func applyContributionTokens(before []contributionToken, edit SequenceEdit, tag string) ([]contributionToken, []contributionToken, error) {
	atoms := tokenAtoms(before)
	if _, err := ApplySequence(atoms, edit); err != nil {
		return nil, nil, err
	}
	complete, err := completeSequence(edit, SequenceLength(atoms))
	if err != nil {
		return nil, nil, err
	}
	result := []contributionToken{}
	removed := []contributionToken{}
	index, inserted := 0, 0
	for _, part := range complete {
		if part.Insert != nil {
			items := contributionTokens(part.Insert, tag)
			for i := range items {
				items[i].ID = fmt.Sprintf("%s/%d", tag, inserted)
				inserted++
			}
			result = append(result, items...)
			continue
		}
		units := part.Retain + part.Delete
		for units > 0 {
			token := before[index]
			units -= atomLength(token.Atom)
			if part.Delete > 0 {
				removed = append(removed, token)
			} else {
				result = append(result, token)
			}
			index++
		}
	}
	return result, removed, nil
}
func contributionStep(tokens []contributionToken, op, field Operation, tag string) ([]contributionToken, []contributionToken, error) {
	if op.NodeID != field.NodeID {
		return tokens, nil, nil
	}
	if op.Type == "migrate_composer" && referenceText(field) {
		var legacy Draft
		if json.Unmarshal(op.Expected, &legacy) != nil {
			return nil, nil, operationError("undo_unavailable")
		}
		atoms := []Atom{}
		for i, line := range strings.Split(legacy.Prompt, "\n") {
			if i > 0 {
				atoms = append(atoms, Atom{Type: "paragraphBreak"})
			}
			if line != "" {
				atoms = append(atoms, Atom{Type: "text", Text: line})
			}
		}
		return contributionTokens(atoms, tag+"/migration"), nil, nil
	}
	if op.Type == "text" && slices.Equal(op.Path, field.Path) {
		return applyContributionTokens(tokens, op.Text, tag)
	}
	if referenceRemoval(op) && referenceText(field) {
		kept, removed := []contributionToken{}, []contributionToken{}
		for _, token := range tokens {
			if token.Atom.Type == "reference" && token.Atom.ReferenceID == op.ItemID {
				removed = append(removed, token)
			} else {
				kept = append(kept, token)
			}
		}
		return kept, removed, nil
	}
	return tokens, nil, nil
}
func checkDeletedContribution(floor Document, history []ContributionBatch, targetIndex int, field Operation) error {
	initial, err := readOperationSequence(floor, field)
	if err != nil {
		initial = []Atom{}
	}
	tokens := contributionTokens(initial, "base")
	states := map[int64][]contributionToken{floor.Revision: tokens}
	owned := map[string]bool{}
	doc := floor
	for i, row := range history {
		original, found := states[row.OriginalBase]
		relevant := slices.ContainsFunc(row.Original, func(op Operation) bool {
			return op.NodeID == field.NodeID && (op.Type == "text" && slices.Equal(op.Path, field.Path) || referenceRemoval(op) && referenceText(field))
		})
		if relevant && !found {
			return operationError("undo_unavailable")
		}
		for step, op := range row.Original {
			var removed []contributionToken
			original, removed, err = contributionStep(original, op, field, fmt.Sprintf("%s/%d", row.OpID, step))
			if err != nil {
				return operationError("undo_unavailable")
			}
			if i > targetIndex {
				for _, token := range removed {
					if owned[token.ID] {
						return operationError("undo_conflict")
					}
				}
			}
		}
		for step, op := range row.Operations {
			var removed []contributionToken
			tokens, removed, err = contributionStep(tokens, op, field, fmt.Sprintf("%s/%d", row.OpID, step))
			if err != nil {
				return operationError("undo_unavailable")
			}
			if i == targetIndex {
				for _, token := range removed {
					owned[token.ID] = true
				}
			}
		}
		doc, err = ApplyEffects(doc, row.Effects)
		if err != nil {
			return operationError("undo_unavailable")
		}
		doc.Revision = row.Revision
		actual, err := readOperationSequence(doc, field)
		if err != nil {
			actual = []Atom{}
		}
		if !equalJSON(effectJSON(actual), effectJSON(tokenAtoms(tokens))) {
			if i >= targetIndex {
				return operationError("undo_conflict")
			}
			tokens = contributionTokens(actual, row.OpID+"/canonical")
		}
		states[row.Revision] = tokens
	}
	return nil
}
