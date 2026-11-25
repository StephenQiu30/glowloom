package domain

import (
	"bytes"
	"encoding/json"
	"slices"
	"sort"

	"github.com/google/uuid"
)

// Effect is a bounded preimage of an actual touched field. Entity creation and
// deletion carry that entity only; unchanged nodes and the graph are never saved.
// Before is private history data; public feeds expose After only.
type Effect struct {
	Entity       string          `json:"entity"`
	ID           uuid.UUID       `json:"id"`
	Path         []string        `json:"path"`
	Index        int             `json:"index"`
	Before       json.RawMessage `json:"before,omitempty"`
	After        json.RawMessage `json:"after,omitempty"`
	BeforeExists bool            `json:"before_exists"`
	AfterExists  bool            `json:"after_exists"`
}

func effectJSON(value any) json.RawMessage { body, _ := json.Marshal(value); return body }

func diffEffectFields(e Effect, before, after any) []Effect {
	x, xok := before.(map[string]any)
	y, yok := after.(map[string]any)
	if xok && yok {
		keys := make([]string, 0, len(x)+len(y))
		seen := map[string]bool{}
		for k := range x {
			seen[k] = true
			keys = append(keys, k)
		}
		for k := range y {
			if !seen[k] {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		var result []Effect
		for _, k := range keys {
			bv, bok := x[k]
			av, aok := y[k]
			next := e
			next.Path = append(slices.Clone(e.Path), k)
			next.BeforeExists = bok
			next.AfterExists = aok
			if bok && aok {
				result = append(result, diffEffectFields(next, bv, av)...)
				continue
			}
			if bok {
				next.Before = effectJSON(bv)
			}
			if aok {
				next.After = effectJSON(av)
			}
			result = append(result, next)
		}
		return result
	}
	e.Before = effectJSON(before)
	e.After = effectJSON(after)
	if bytes.Equal(e.Before, e.After) {
		return nil
	}
	return []Effect{e}
}

// DocumentEffects derives canonical changes, including server normalization and
// cleanup effects, without mistaking a same-value write for a structural change.
func DocumentEffects(before, after Document) []Effect {
	var result []Effect
	for _, entity := range []string{"node", "edge"} {
		old := map[uuid.UUID]any{}
		next := map[uuid.UUID]any{}
		indexes := map[uuid.UUID]int{}
		var ids []uuid.UUID
		if entity == "node" {
			for i, n := range before.Nodes {
				old[n.ID] = n
				indexes[n.ID] = i
				ids = append(ids, n.ID)
			}
			for i, n := range after.Nodes {
				next[n.ID] = n
				if _, ok := old[n.ID]; !ok {
					indexes[n.ID] = i
					ids = append(ids, n.ID)
				}
			}
		} else {
			for i, e := range before.Edges {
				old[e.ID] = e
				indexes[e.ID] = i
				ids = append(ids, e.ID)
			}
			for i, e := range after.Edges {
				next[e.ID] = e
				if _, ok := old[e.ID]; !ok {
					indexes[e.ID] = i
					ids = append(ids, e.ID)
				}
			}
		}
		for _, id := range ids {
			b, bok := old[id]
			a, aok := next[id]
			e := Effect{Entity: entity, ID: id, Index: indexes[id], Path: []string{}, BeforeExists: bok, AfterExists: aok}
			if !bok || !aok {
				if bok {
					e.Before = effectJSON(b)
				}
				if aok {
					e.After = effectJSON(a)
				}
				result = append(result, e)
				continue
			}
			if entity == "edge" {
				e.Before, e.After = effectJSON(b), effectJSON(a)
				if !equalJSON(e.Before, e.After) {
					result = append(result, e)
				}
				continue
			}
			var bo, ao any
			decode := func(value any, target *any) {
				d := json.NewDecoder(bytes.NewReader(effectJSON(value)))
				d.UseNumber()
				_ = d.Decode(target)
			}
			decode(b, &bo)
			decode(a, &ao)
			result = append(result, diffEffectFields(e, bo, ao)...)
		}
	}
	return result
}

// RewindEffects reconstructs an operation's base from checked private preimages.
// It is only used with contiguous trusted database history, never client patches.
func RewindEffects(document Document, effects []Effect) (Document, error) {
	doc, err := cloneDocument(document)
	if err != nil {
		return Document{}, err
	}
	for i := len(effects) - 1; i >= 0; i-- {
		e := effects[i]
		if e.Index < 0 {
			return Document{}, operationError("history_unavailable")
		}
		switch {
		case e.Entity == "node":
			index := nodeIndex(&doc, e.ID)
			if len(e.Path) == 0 {
				if (index >= 0) != e.AfterExists || index >= 0 && !equalJSON(effectJSON(doc.Nodes[index]), e.After) {
					return Document{}, operationError("history_unavailable")
				}
			}
			if len(e.Path) == 0 {
				if e.BeforeExists {
					var n Node
					if json.Unmarshal(e.Before, &n) != nil || n.ID != e.ID {
						return Document{}, operationError("history_unavailable")
					}
					if index >= 0 {
						doc.Nodes[index] = n
					} else {
						doc.Nodes = slices.Insert(doc.Nodes, min(e.Index, len(doc.Nodes)), n)
					}
				} else if index >= 0 {
					doc.Nodes = slices.Delete(doc.Nodes, index, index+1)
				}
				continue
			}
			if index < 0 {
				return Document{}, operationError("history_unavailable")
			}
			object, err := nodeObject(doc.Nodes[index])
			if err != nil {
				return Document{}, err
			}
			parent, key, err := fieldContainer(object, e.Path)
			if err != nil {
				return Document{}, operationError("history_unavailable")
			}
			current, exists := parent[key]
			if exists != e.AfterExists || exists && !equalJSON(effectJSON(current), e.After) {
				return Document{}, operationError("history_unavailable")
			}
			if e.BeforeExists {
				var value any
				d := json.NewDecoder(bytes.NewReader(e.Before))
				d.UseNumber()
				if d.Decode(&value) != nil {
					return Document{}, operationError("history_unavailable")
				}
				parent[key] = value
			} else {
				delete(parent, key)
			}
			var restored Node
			if json.Unmarshal(effectJSON(object), &restored) != nil {
				return Document{}, operationError("history_unavailable")
			}
			doc.Nodes[index] = restored
		case e.Entity == "edge" && len(e.Path) == 0:
			index := slices.IndexFunc(doc.Edges, func(edge Edge) bool { return edge.ID == e.ID })
			if (index >= 0) != e.AfterExists || index >= 0 && !equalJSON(effectJSON(doc.Edges[index]), e.After) {
				return Document{}, operationError("history_unavailable")
			}
			if e.BeforeExists {
				var edge Edge
				if json.Unmarshal(e.Before, &edge) != nil || edge.ID != e.ID {
					return Document{}, operationError("history_unavailable")
				}
				if index >= 0 {
					doc.Edges[index] = edge
				} else {
					doc.Edges = slices.Insert(doc.Edges, min(e.Index, len(doc.Edges)), edge)
				}
			} else if index >= 0 {
				doc.Edges = slices.Delete(doc.Edges, index, index+1)
			}
		default:
			return Document{}, operationError("history_unavailable")
		}
	}
	return doc, nil
}

// ApplyEffects advances only trusted canonical effects with checked preconditions.
func ApplyEffects(document Document, effects []Effect) (Document, error) {
	inverse := make([]Effect, len(effects))
	for i, e := range effects {
		e.Before, e.After = e.After, e.Before
		e.BeforeExists, e.AfterExists = e.AfterExists, e.BeforeExists
		inverse[len(effects)-1-i] = e
	}
	return RewindEffects(document, inverse)
}
