package application

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func jsonObject(v any) map[string]any {
	body, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	return m
}
func jsonValue(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func pickFields(m map[string]any, keys ...string) map[string]any {
	r := map[string]any{}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			r[k] = v
		}
	}
	return r
}

// YjsProjectionOperations translates only the accepted editing surface. Applying
// the result must reconstruct every candidate field; unsupported changes fail closed.
func YjsProjectionOperations(before domain.Document, after DocumentDTO, client uuid.UUID) ([]domain.Operation, error) {
	if after.ID != before.ID || after.ProjectID != before.ProjectID || after.SchemaVersion != SchemaVersion || after.OperationVersion != OperationVersion || after.Revision != Revision(before.Revision) {
		return nil, fmt.Errorf("invalid projection identity")
	}
	if after.Name == "" || strings.TrimSpace(after.Name) != after.Name || !utf8.ValidString(after.Name) || utf8.RuneCountInString(after.Name) > 128 || strings.ContainsRune(after.Name, 0) {
		return nil, domain.ErrInvalidCommand
	}
	previous := Snapshot(before)
	nodes := map[uuid.UUID]NodeDTO{}
	next := map[uuid.UUID]NodeDTO{}
	edges := map[uuid.UUID]domain.Edge{}
	nextEdges := map[uuid.UUID]domain.Edge{}
	for _, n := range previous.Nodes {
		nodes[n.ID] = n
	}
	for _, n := range after.Nodes {
		if _, exists := next[n.ID]; exists {
			return nil, domain.ErrInvalidCommand
		}
		next[n.ID] = n
	}
	for _, e := range before.Edges {
		edges[e.ID] = e
	}
	for _, e := range after.Edges {
		if _, exists := nextEdges[e.ID]; exists {
			return nil, domain.ErrInvalidCommand
		}
		nextEdges[e.ID] = e
	}
	raw := []json.RawMessage{}
	add := func(op any) { raw = append(raw, jsonValue(op)) }
	for _, e := range before.Edges {
		if _, ok := nextEdges[e.ID]; !ok {
			_, source := next[e.SourceNodeID]
			_, target := next[e.TargetNodeID]
			if source && target {
				add(map[string]any{"type": "disconnect", "node_id": e.ID})
			}
		}
	}
	ordered := slices.Clone(after.Nodes)
	slices.SortStableFunc(ordered, func(a, b NodeDTO) int {
		if a.NodeType == "group" && b.NodeType != "group" {
			return -1
		}
		if a.NodeType != "group" && b.NodeType == "group" {
			return 1
		}
		return 0
	})
	for _, n := range ordered {
		old, exists := nodes[n.ID]
		if !exists {
			m := jsonObject(n)
			for _, key := range []string{"ref_type", "ref_id", "content_version", "input_revision", "last_operation_id"} {
				delete(m, key)
			}
			add(map[string]any{"type": "create_node", "node": m})
			continue
		}
		a, b := jsonObject(old), jsonObject(n)
		set := func(path []string, old, nextValue any, cas bool, present bool) {
			if reflect.DeepEqual(old, nextValue) {
				return
			}
			op := map[string]any{"type": "set", "node_id": n.ID, "path": path, "value": nextValue}
			if cas {
				op["expected_exists"] = present
				if present {
					op["expected"] = old
				}
			}
			add(op)
		}
		for _, key := range []string{"title", "z_index", "parent_id"} {
			set([]string{key}, a[key], b[key], false, true)
		}
		set([]string{"position"}, pickFields(a, "x", "y"), pickFields(b, "x", "y"), false, true)
		set([]string{"size"}, pickFields(a, "width", "height"), pickFields(b, "width", "height"), false, true)
		ac, bc := jsonObject(old.Config), jsonObject(n.Config)
		set([]string{"config", "collapsed"}, ac["collapsed"], bc["collapsed"], false, true)
		_, present := ac["editor"]
		set([]string{"config", "editor"}, ac["editor"], bc["editor"], true, present)
		set([]string{"config", "text_content"}, pickFields(ac, "text", "composer", "format", "format_version", "rich_text"), pickFields(bc, "text", "composer", "format", "format_version", "rich_text"), true, true)
		if !reflect.DeepEqual(old.MediaRef, n.MediaRef) {
			from, to := any(old.MediaRef), any(n.MediaRef)
			if old.MediaRef == nil {
				from = map[string]any{"empty": true}
			}
			if n.MediaRef == nil {
				to = map[string]any{"empty": true}
			}
			add(map[string]any{"type": "replace_media", "node_id": n.ID, "expected_media_ref": from, "new_media_ref": to})
		}
	}
	for _, e := range after.Edges {
		old, exists := edges[e.ID]
		if !exists {
			add(map[string]any{"type": "connect", "edge": e})
		} else if !reflect.DeepEqual(jsonObject(old), jsonObject(e)) {
			add(map[string]any{"type": "set_edge", "node_id": e.ID, "edge": e, "expected": old})
		}
	}
	removed := slices.Clone(previous.Nodes)
	slices.SortStableFunc(removed, func(a, b NodeDTO) int {
		if a.NodeType == "group" && b.NodeType != "group" {
			return 1
		}
		if a.NodeType != "group" && b.NodeType == "group" {
			return -1
		}
		return 0
	})
	for _, n := range removed {
		if _, exists := next[n.ID]; !exists {
			add(map[string]any{"type": "delete_node", "node_id": n.ID})
		}
	}
	if len(raw) == 0 {
		if !sameProjection(previous, after) {
			return nil, domain.ErrUnsupportedCommand
		}
		return []domain.Operation{}, nil
	}
	body, _ := json.Marshal(SubmissionDTO{ClientID: client, OperationVersion: 1, BaseRevision: Revision(before.Revision), Operations: raw})
	input, err := DecodeSubmission(body)
	if err != nil {
		return nil, err
	}
	applied, err := domain.ApplyOperations(before, input.Operations)
	if err != nil {
		return nil, err
	}
	if !sameProjection(Snapshot(applied.Document), after) {
		return nil, domain.ErrUnsupportedCommand
	}
	return input.Operations, nil
}
func sameProjection(a, b DocumentDTO) bool {
	if len(a.Nodes) != len(b.Nodes) || len(a.Edges) != len(b.Edges) {
		return false
	}
	nodes := map[uuid.UUID]NodeDTO{}
	for _, n := range a.Nodes {
		nodes[n.ID] = n
	}
	for _, n := range b.Nodes {
		old, ok := nodes[n.ID]
		if !ok || !reflect.DeepEqual(jsonObject(old), jsonObject(n)) {
			return false
		}
	}
	edges := map[uuid.UUID]domain.Edge{}
	for _, e := range a.Edges {
		edges[e.ID] = e
	}
	for _, e := range b.Edges {
		old, ok := edges[e.ID]
		if !ok || !reflect.DeepEqual(jsonObject(old), jsonObject(e)) {
			return false
		}
	}
	return true
}
