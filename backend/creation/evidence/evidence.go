// Package evidence defines bounded, project-scoped retrieval and stable citations.
package evidence

import (
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

var ErrInvalid = errors.New("invalid project evidence request")

const (
	SearchTool  = "search_project_evidence"
	ReadTool    = "read_project_evidence"
	MaxReceipts = 64
)

var kinds = []string{"brief", "decision", "canvas_node", "script_source", "scene", "script_line", "character", "location", "prop", "media", "transcript"}

// Query searches published sources or locates an exact object by stable ID.
type Query struct {
	Text  string   `json:"query"`
	Kinds []string `json:"kinds,omitempty"`
	Limit int      `json:"limit,omitempty"`
}

// Normalize bounds retrieval work before accessing the project corpus.
func (q Query) Normalize() (Query, error) {
	if !utf8.ValidString(q.Text) || strings.IndexFunc(q.Text, func(r rune) bool { return unicode.IsControl(r) && !unicode.IsSpace(r) }) >= 0 {
		return Query{}, ErrInvalid
	}
	q.Text = strings.Join(strings.Fields(q.Text), " ")
	if q.Limit == 0 {
		q.Limit = 6
	}
	if q.Text == "" || utf8.RuneCountInString(q.Text) > 160 || q.Limit < 1 || q.Limit > 12 || len(q.Kinds) > len(kinds) {
		return Query{}, ErrInvalid
	}
	for _, k := range q.Kinds {
		if !slices.Contains(kinds, k) {
			return Query{}, ErrInvalid
		}
	}
	return q, nil
}

// Reference pins the exact authorized excerpt. Part is one-based.
type Reference struct {
	Kind    string    `json:"kind"`
	ID      uuid.UUID `json:"id"`
	Part    int       `json:"part,omitempty"`
	Version string    `json:"version,omitempty"`
}

// Normalize accepts an unpinned ID only for a direct request.
func (ref Reference) Normalize() (Reference, error) {
	if ref.Part == 0 {
		ref.Part = 1
	}
	if !slices.Contains(kinds, ref.Kind) || ref.ID == uuid.Nil || ref.Part < 1 || ref.Part > 10000 ||
		(ref.Version != "" && (len(ref.Version) != 64 || strings.Trim(ref.Version, "0123456789abcdef") != "")) {
		return Reference{}, ErrInvalid
	}
	return ref, nil
}

// Item is a source excerpt with a stable object, version and optional position.
type Item struct {
	Reference
	Revision     int64      `json:"revision"`
	Title        string     `json:"title"`
	Excerpt      string     `json:"excerpt"`
	ParentID     *uuid.UUID `json:"parent_id,omitempty"`
	Start        *int64     `json:"start,omitempty"`
	End          *int64     `json:"end,omitempty"`
	PositionUnit string     `json:"position_unit,omitempty"`
	Draft        bool       `json:"draft,omitempty"`
}

// Result distinguishes an empty search, ambiguous names and an outdated locator.
type Result struct {
	Status             string `json:"status"`
	Items              []Item `json:"items"`
	ScriptIndexPending bool   `json:"script_index_pending,omitempty"`
}
