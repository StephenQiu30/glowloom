package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/google/uuid"

	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// Operation is the closed collaboration vocabulary. Paths address named fields
// and stable collection identities, never arbitrary JSON paths or array indices.
type Operation struct {
	unknown          map[string]json.RawMessage
	labelBase        *prompt.LabelState
	labelOrder       []string
	Type             string          `json:"type"`
	ExpectedMediaRef *MediaRef       `json:"expected_media_ref,omitempty"`
	NewMediaRef      *MediaRef       `json:"new_media_ref,omitempty"`
	NodeID           uuid.UUID       `json:"node_id,omitempty"`
	Path             []string        `json:"path,omitempty"`
	Value            json.RawMessage `json:"value,omitempty" swaggertype:"object"`
	Expected         json.RawMessage `json:"expected,omitempty" swaggertype:"object"`
	ExpectedExists   *bool           `json:"expected_exists,omitempty"`
	Context          json.RawMessage `json:"context,omitempty" swaggertype:"object"`
	Unset            bool            `json:"unset,omitempty"`
	Text             SequenceEdit    `json:"text,omitempty"`
	ItemID           string          `json:"item_id,omitempty"`
	BeforeID         string          `json:"before_id,omitempty"`
	Node             *Node           `json:"node,omitempty"`
	Edge             *Edge           `json:"edge,omitempty"`
}

// OperationError classifies a deterministic business rejection.
type OperationError struct{ Code string }

func (e *OperationError) Error() string { return e.Code }

func operationError(code string) error { return &OperationError{Code: code} }

// FieldChange stores only the touched field or group; equal writes still count.
type FieldChange struct {
	NodeID       uuid.UUID       `json:"node_id"`
	Path         []string        `json:"path"`
	Before       json.RawMessage `json:"before,omitempty"`
	After        json.RawMessage `json:"after,omitempty"`
	BeforeExists bool            `json:"before_exists"`
	AfterExists  bool            `json:"after_exists"`
}

// OperationOutcome is a fully evaluated candidate, not a persistence receipt.
type OperationOutcome struct {
	// IntermediateNodes are evaluated touched entities for source authorization and
	// immutable history retention, never snapshots persisted as graph authority.
	IntermediateNodes []Node
	Document          Document
	Changes           []FieldChange
	Effects           []Effect
	Inputs            []InputSnapshot
}

type fieldPolicy struct {
	kind     string
	group    []string
	cas      bool
	context  string
	optional bool
}

func cloneDocument(document Document) (Document, error) {
	body, err := json.Marshal(document)
	if err != nil {
		return Document{}, err
	}
	var clone Document
	if err := json.Unmarshal(body, &clone); err != nil {
		return Document{}, err
	}
	return clone, nil
}

func nodeObject(node Node) (map[string]any, error) {
	body, err := json.Marshal(node)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	err = decoder.Decode(&object)
	return object, err
}

func fieldContainer(object map[string]any, path []string) (map[string]any, string, error) {
	if len(path) == 0 || len(path) > 16 {
		return nil, "", operationError("unsupported_field")
	}
	var current any = object
	for _, part := range path[:len(path)-1] {
		switch value := current.(type) {
		case map[string]any:
			current = value[part]
		case []any:
			if !strings.HasPrefix(part, "@") {
				return nil, "", operationError("unsupported_field")
			}
			current = nil
			for _, item := range value {
				if object, ok := item.(map[string]any); ok && itemIdentity(object) == strings.TrimPrefix(part, "@") {
					current = object
					break
				}
			}
		default:
			return nil, "", operationError("target_not_found")
		}
	}
	parent, ok := current.(map[string]any)
	if !ok {
		return nil, "", operationError("target_not_found")
	}
	return parent, path[len(path)-1], nil
}

func fieldValue(parent map[string]any, key string, rule fieldPolicy) (json.RawMessage, bool) {
	value, exists := parent[key]
	if rule.group != nil {
		group := make(map[string]any)
		for _, member := range rule.group {
			if value, ok := parent[member]; ok {
				group[member] = value
			}
		}
		value, exists = group, !rule.optional || len(group) > 0
	}
	if !exists {
		return nil, false
	}
	body, _ := json.Marshal(value)
	return body, true
}

func operationSequence(value json.RawMessage, rich bool) ([]Atom, error) {
	if rich {
		var document struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(value, &document) != nil || document.Type != "doc" {
			return nil, ErrInvalidSequence
		}
		return sequenceFromPromptJSON(value)
	}
	var text string
	if json.Unmarshal(value, &text) != nil {
		return nil, ErrInvalidSequence
	}
	if text == "" {
		return []Atom{}, nil
	}
	return []Atom{{Type: "text", Text: text}}, nil
}

func applyField(document *Document, op Operation) (FieldChange, error) {
	i := nodeIndex(document, op.NodeID)
	if i < 0 {
		return FieldChange{}, operationError("target_not_found")
	}
	if !editable(document.Nodes[i]) || document.Nodes[i].LastOperationID != nil {
		return FieldChange{}, operationError("protected_target")
	}
	rule, ok := policy(op.Path)
	if !ok || rule.kind != op.Type {
		return FieldChange{}, operationError("unsupported_field")
	}
	object, err := nodeObject(document.Nodes[i])
	if err != nil {
		return FieldChange{}, err
	}
	if isBatchSlot(op.Path) {
		return applyBatchSlot(document, i, object, op)
	}
	parent, key, err := fieldContainer(object, op.Path)
	if err != nil {
		return FieldChange{}, err
	}
	before, exists := fieldValue(parent, key, rule)
	if err := timelineGate(object, op); err != nil {
		return FieldChange{}, err
	}
	if rule.context != "" {
		context, err := fieldContext(object, op.Path, rule)
		if err != nil {
			return FieldChange{}, err
		}
		if !equalJSON(context, op.Context) {
			return FieldChange{}, operationError("source_changed")
		}
	}
	if rule.cas || rule.optional && (op.Unset || !exists) || rule.group == nil && rule.context != "" && (compositeJSON(before) || compositeJSON(op.Value)) {
		if op.ExpectedExists == nil || *op.ExpectedExists != exists || exists && !equalJSON(before, op.Expected) {
			return FieldChange{}, operationError("field_conflict")
		}
	}
	if op.Type == "text" {
		textBefore, textExists := textValue(op.Path, before, exists)
		if !textExists || op.Value != nil || op.Unset {
			return FieldChange{}, operationError("invalid_operation")
		}
		rich := key == "document"
		sequence, err := operationSequence(textBefore, rich)
		if err != nil {
			return FieldChange{}, err
		}
		sequence, err = ApplySequence(sequence, op.Text)
		if err != nil {
			return FieldChange{}, err
		}
		if rich {
			value, err := SequencePrompt(sequence)
			if err != nil {
				return FieldChange{}, err
			}
			body, _ := json.Marshal(value)
			var decoded any
			if err := json.Unmarshal(body, &decoded); err != nil {
				return FieldChange{}, err
			}
			parent[key] = decoded
		} else {
			var text strings.Builder
			for _, atom := range sequence {
				if atom.Type != "text" {
					return FieldChange{}, ErrInvalidSequence
				}
				text.WriteString(atom.Text)
			}
			if negativePromptPath(op.Path) && text.Len() == 0 {
				delete(parent, key)
			} else {
				parent[key] = text.String()
			}
		}
	} else {
		if op.Text != nil || (op.Value == nil && !op.Unset) {
			return FieldChange{}, operationError("invalid_operation")
		}
		var value any
		if !op.Unset {
			decoder := json.NewDecoder(bytes.NewReader(op.Value))
			decoder.UseNumber()
			if decoder.Decode(&value) != nil {
				return FieldChange{}, operationError("invalid_operation")
			}
		}
		switch {
		case rule.group != nil:
			group, ok := value.(map[string]any)
			if op.Unset && rule.optional {
				group = map[string]any{}
				ok = true
			}
			if !ok || op.Unset && !rule.optional || len(group) > len(rule.group) {
				return FieldChange{}, operationError("invalid_operation")
			}
			for member := range group {
				if !slices.Contains(rule.group, member) {
					return FieldChange{}, operationError("invalid_operation")
				}
			}
			for _, member := range rule.group {
				v, ok := group[member]
				if !ok {
					if key == "position" || key == "size" {
						return FieldChange{}, operationError("invalid_operation")
					}
					delete(parent, member)
					continue
				}
				parent[member] = v
			}
		case op.Unset:
			delete(parent, key)
		default:
			parent[key] = value
		}
	}
	if err := normalizeBinding(object, op); err != nil {
		return FieldChange{}, err
	}
	after, afterExists := fieldValue(parent, key, rule)
	body, _ := json.Marshal(object)
	var node Node
	if err := json.Unmarshal(body, &node); err != nil {
		return FieldChange{}, operationError("invalid_operation")
	}
	document.Nodes[i] = node
	return FieldChange{NodeID: op.NodeID, Path: slices.Clone(op.Path), Before: before, After: after, BeforeExists: exists, AfterExists: afterExists}, nil
}

// ApplyOperations checks the entire candidate and advances each final input once.
func ApplyOperations(document Document, operations []Operation) (OperationOutcome, error) {
	if err := ValidateGraph(document); err != nil {
		return OperationOutcome{}, fmt.Errorf("%w: %w", ErrGraphReadOnly, err)
	}
	return applyOperations(document, operations, true)
}

func applyOperations(document Document, operations []Operation, final bool) (OperationOutcome, error) {
	if len(operations) > 100 {
		return OperationOutcome{}, operationError("operation_limit")
	}
	doc, err := cloneDocument(document)
	if err != nil {
		return OperationOutcome{}, err
	}
	out := OperationOutcome{}
	created := map[uuid.UUID]bool{}
	removedItems := map[string]bool{}
	for index, op := range operations {
		if err := validOperationShape(op); err != nil {
			return OperationOutcome{}, fmt.Errorf("operation %d: %w", index, err)
		}
		if op.Type == "noop" {
			continue
		}
		var newID uuid.UUID
		if op.Type == "create_node" && op.Node != nil {
			newID = op.Node.ID
		}
		if op.Type == "connect" && op.Edge != nil {
			newID = op.Edge.ID
		}
		if newID != uuid.Nil {
			if created[newID] {
				return OperationOutcome{}, operationError("id_conflict")
			}
			created[newID] = true
		}
		itemKey := op.NodeID.String() + "/" + strings.Join(op.Path, "/") + "/" + op.ItemID
		if op.Type == "insert" && removedItems[itemKey] {
			return OperationOutcome{}, operationError("id_conflict")
		}
		if op.Type == "remove" {
			removedItems[itemKey] = true
		}
		var change FieldChange
		var err error
		switch op.Type {
		case "replace_media":
			change, err = replaceMedia(&doc, op)
		case "migrate_composer":
			change, err = migrateComposer(&doc, op)
		case "set", "text":
			change, err = applyField(&doc, op)
		case "insert", "restore_item", "remove", "move":
			change, err = applyCollection(&doc, op)
		case "create_node", "restore_node", "delete_node", "connect", "restore_edge", "disconnect":
			err = applyStructure(&doc, op)
		default:
			err = operationError("unsupported_operation")
		}
		if err != nil {
			return OperationOutcome{}, fmt.Errorf("operation %d: %w", index, err)
		}
		if change.Path != nil {
			out.Changes = append(out.Changes, change)
		}
		if final {
			target := op.NodeID
			if op.Node != nil {
				target = op.Node.ID
			}
			if i := nodeIndex(&doc, target); i >= 0 {
				var touched Node
				if err := json.Unmarshal(effectJSON(doc.Nodes[i]), &touched); err != nil {
					return OperationOutcome{}, err
				}
				out.IntermediateNodes = append(out.IntermediateNodes, touched)
			}
		}
	}
	if final {
		for i := range doc.Nodes {
			n := &doc.Nodes[i]
			if !IsDraft(*n) {
				continue
			}
			old := nodeIndex(&document, n.ID)
			switch {
			case old < 0:
				n.InputRevision = 1
				for _, op := range operations {
					if op.Type == "restore_node" && op.Node != nil && op.Node.ID == n.ID {
						if op.Node.InputRevision >= math.MaxInt32 {
							return OperationOutcome{}, operationError("input_revision_limit")
						}
						n.InputRevision = op.Node.InputRevision + 1
					}
				}
			case !SameInput(*document.Nodes[old].Config.Draft, *n.Config.Draft):
				if document.Nodes[old].InputRevision >= math.MaxInt32 {
					return OperationOutcome{}, operationError("input_revision_limit")
				}
				n.InputRevision = document.Nodes[old].InputRevision + 1
			default:
				continue
			}
			out.Inputs = append(out.Inputs, InputSnapshot{NodeID: n.ID, Revision: n.InputRevision, Draft: *n.Config.Draft})
		}
		for _, node := range doc.Nodes {
			if !validTitle(node.Title) || !coordinate(node.X) || !coordinate(node.Y) || !dimension(node.NodeType, node.Width) || !dimension(node.NodeType, node.Height) || !validZIndex(node.ZIndex) {
				return OperationOutcome{}, operationError("invalid_node")
			}
		}
		if err := ValidateGraph(doc); err != nil {
			return OperationOutcome{}, err
		}
	}
	out.Effects = DocumentEffects(document, doc)
	out.Document = doc
	return out, nil
}

func pairOperations(base Document, a, b Operation) (Operation, Operation, error) {
	if a.Type == "delete_node" && operationUsesNode(b, a.NodeID) {
		if b.Type == "delete_node" && b.NodeID == a.NodeID {
			return a, Operation{Type: "noop"}, nil
		}
		return a, b, operationError("target_deleted")
	}
	if b.Type == "delete_node" && operationUsesNode(a, b.NodeID) {
		return Operation{Type: "noop"}, b, nil
	}
	if a.NodeID == b.NodeID && (a.Type == "migrate_composer" && draftOperation(b) || b.Type == "migrate_composer" && draftOperation(a)) {
		return a, b, operationError("field_conflict")
	}
	if a.Type == "migrate_composer" || b.Type == "migrate_composer" {
		return a, b, nil
	}
	if (a.Type == "create_node" || a.Type == "restore_node") && (b.Type == "create_node" || b.Type == "restore_node") && a.Node != nil && b.Node != nil && a.Node.ID == b.Node.ID {
		return a, b, operationError("id_conflict")
	}
	if a.Type == "connect" && b.Type == "connect" && a.Edge != nil && b.Edge != nil && a.Edge.ID == b.Edge.ID {
		return a, b, operationError("id_conflict")
	}
	if a.Type == "disconnect" && b.Type == "disconnect" && a.NodeID == b.NodeID {
		return a, Operation{Type: "noop"}, nil
	}
	if a.Type == "create_node" || b.Type == "create_node" || a.Type == "restore_node" || b.Type == "restore_node" || a.Type == "restore_edge" || b.Type == "restore_edge" || a.Type == "connect" || b.Type == "connect" || a.Type == "disconnect" || b.Type == "disconnect" {
		return a, b, nil
	}
	if a.Type == "noop" || b.Type == "noop" || a.NodeID != b.NodeID {
		return a, b, nil
	}
	if a.Type == "replace_media" || b.Type == "replace_media" {
		if a.Type == b.Type {
			return a, b, operationError("media_conflict")
		}
		return a, b, nil
	}
	if structureOperation(a.Type) && b.Type == "restore_item" {
		var err error
		b, err = transformRestoredRow(base, a, b)
		if err != nil {
			return a, b, err
		}
	}
	if a.Type == "restore_item" && structureOperation(b.Type) {
		var err error
		a, err = transformRestoredRow(base, b, a)
		if err != nil {
			return a, b, err
		}
	}
	if referenceRemoval(a) && referenceText(b) || referenceText(a) && referenceRemoval(b) {
		return transformReferenceCleanup(base, a, b)
	}
	if structureOperation(a.Type) && structureOperation(b.Type) && slices.Equal(a.Path, b.Path) {
		if a.Type == "move" || b.Type == "move" {
			return a, b, operationError("order_conflict")
		}
		if insertionOperation(a.Type) && insertionOperation(b.Type) {
			if a.ItemID == b.ItemID {
				return a, b, operationError("id_conflict")
			}
			if isReferenceCollection(a.Path) {
				i := nodeIndex(&base, a.NodeID)
				if i < 0 {
					return a, b, operationError("target_not_found")
				}
				if a.labelBase == nil {
					labels := base.Nodes[i].Config.Draft.Composer.LabelState
					a.labelBase = &labels
				}
				if len(a.labelOrder) == 0 {
					a.labelOrder = []string{a.ItemID}
				}
				if !slices.Contains(a.labelOrder, b.ItemID) {
					a.labelOrder = append(slices.Clone(a.labelOrder), b.ItemID)
				}
			}
			if a.BeforeID == b.BeforeID {
				a.BeforeID = b.ItemID
			}
			return a, b, nil
		}
		if a.Type == "remove" && b.Type == "remove" && a.ItemID == b.ItemID {
			return a, Operation{Type: "noop"}, nil
		}
		if a.Type == "remove" && (b.BeforeID == a.ItemID || operationTargetsItem(b, a)) {
			return a, b, operationError("target_deleted")
		}
		return a, b, nil
	}
	if a.Type == "remove" && operationTargetsItem(b, a) {
		return a, b, operationError("target_deleted")
	}
	if b.Type == "remove" && operationTargetsItem(a, b) {
		return Operation{Type: "noop"}, b, nil
	}
	if !slices.Equal(a.Path, b.Path) {
		ar, _ := policy(a.Path)
		br, _ := policy(b.Path)
		if ar.cas && operationsOverlap(a, b) || br.cas && operationsOverlap(a, b) {
			return a, b, operationError("field_conflict")
		}
		return a, b, nil
	}
	if a.Type == "text" && b.Type == "text" {
		i := nodeIndex(&base, a.NodeID)
		if i < 0 {
			return a, b, operationError("target_not_found")
		}
		object, err := nodeObject(base.Nodes[i])
		if err != nil {
			return a, b, err
		}
		parent, key, err := fieldContainer(object, a.Path)
		if err != nil {
			return a, b, err
		}
		rule, _ := policy(a.Path)
		value, exists := fieldValue(parent, key, rule)
		value, _ = textValue(a.Path, value, exists)
		sequence, err := operationSequence(value, key == "document")
		if err != nil {
			return a, b, err
		}
		a.Text, b.Text, err = TransformSequencePair(a.Text, b.Text, SequenceLength(sequence))
		return a, b, err
	}
	if a.Type == "set" && b.Type == "set" {
		rule, _ := policy(a.Path)
		if rule.cas || rule.optional && (a.Unset || b.Unset || a.ExpectedExists != nil && !*a.ExpectedExists || b.ExpectedExists != nil && !*b.ExpectedExists) || rule.group == nil && (compositeJSON(a.Value) || compositeJSON(b.Value)) {
			return a, b, operationError("field_conflict")
		}
		return Operation{Type: "noop"}, b, nil
	}
	return a, b, operationError("field_conflict")
}

// TransformOperationPair preserves causal step order inside both batches. A is
// earlier in authoritative server order; inputs must share the original base.
func TransformOperationPair(base Document, a, b []Operation) ([]Operation, []Operation, error) {
	if err := ValidateTransformWork(base, len(a)*len(b)+len(a)+len(b)); err != nil {
		return nil, nil, err
	}
	if _, err := ApplyOperations(base, a); err != nil {
		return nil, nil, err
	}
	if _, err := ApplyOperations(base, b); err != nil {
		return nil, nil, err
	}
	ap := slices.Clone(a)
	bp := make([]Operation, 0, len(b))
	prefix := base
	for _, late := range b {
		state := prefix
		next := make([]Operation, 0, len(ap))
		transformed := late
		for _, early := range ap {
			x, y, err := pairOperations(state, early, transformed)
			if err != nil {
				return nil, nil, err
			}
			next = append(next, x)
			transformed = y
			step, err := applyOperations(state, []Operation{early}, false)
			if err != nil {
				return nil, nil, err
			}
			state = step.Document
		}
		ap = next
		bp = append(bp, transformed)
		step, err := applyOperations(prefix, []Operation{late}, false)
		if err != nil {
			return nil, nil, err
		}
		prefix = step.Document
	}
	return ap, bp, nil
}

// ValidateTransformWork bounds in-memory work before cloning or entering the
// pair grid. Storage/request byte bounds alone do not bound transform CPU cost.
func ValidateTransformWork(document Document, cells int) error {
	if cells < 0 || cells > 4096 {
		return operationError("history_unavailable")
	}
	body, err := json.Marshal(document)
	if err != nil {
		return err
	}
	if int64(len(body))*int64(cells) > 32<<20 {
		return operationError("history_unavailable")
	}
	return nil
}
