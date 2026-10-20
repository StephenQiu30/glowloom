package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ErrInvalidSequence rejects malformed edits, including split UTF-16 pairs.
var ErrInvalidSequence = errors.New("invalid_sequence")

// Atom is text or one typed editor token. Tokens never share a text sentinel.
type Atom struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	ReferenceID string `json:"reference_id,omitempty"`
}

// SequenceComponent consumes base units or inserts a typed sequence.
type SequenceComponent struct {
	Retain int    `json:"retain,omitempty"`
	Insert []Atom `json:"insert,omitempty"`
	Delete int    `json:"delete,omitempty"`
}

// SequenceEdit is ordered; an omitted base suffix is retained.
type SequenceEdit []SequenceComponent

const maxSequenceUnits = 1 << 20

// UnmarshalJSON rejects unknown atom fields and malformed Unicode escapes.
func (a *Atom) UnmarshalJSON(data []byte) error {
	type wire Atom
	return strictSequenceJSON(data, (*wire)(a))
}

// UnmarshalJSON keeps unsupported component attributes from being lost.
func (c *SequenceComponent) UnmarshalJSON(data []byte) error {
	type wire SequenceComponent
	return strictSequenceJSON(data, (*wire)(c))
}

func strictSequenceJSON(data []byte, target any) error {
	if len(data) > 4<<20 || !utf8.Valid(data) {
		return ErrInvalidSequence
	}
	for i := 0; i < len(data); i++ {
		if data[i] != '\\' {
			continue
		}
		i++
		if i >= len(data) {
			return ErrInvalidSequence
		}
		if data[i] != 'u' {
			continue
		}
		if i+4 >= len(data) {
			return ErrInvalidSequence
		}
		n, err := strconv.ParseUint(string(data[i+1:i+5]), 16, 16)
		if err != nil {
			return ErrInvalidSequence
		}
		i += 4
		if n >= 0xdc00 && n <= 0xdfff {
			return ErrInvalidSequence
		}
		if n >= 0xd800 && n <= 0xdbff {
			if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
				return ErrInvalidSequence
			}
			low, err := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return ErrInvalidSequence
			}
			i += 6
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil || decoder.Decode(new(any)) != io.EOF {
		return ErrInvalidSequence
	}
	return nil
}

func atomLength(a Atom) int {
	switch a.Type {
	case "text":
		if a.Text == "" || !utf8.ValidString(a.Text) || a.ReferenceID != "" {
			return -1
		}
		n := 0
		for _, r := range a.Text {
			n++
			if r > 0xffff {
				n++
			}
		}
		return n
	case "paragraphBreak", "hardBreak":
		if a.Text == "" && a.ReferenceID == "" {
			return 1
		}
	case "reference":
		if a.Text == "" && a.ReferenceID != "" && len(a.ReferenceID) <= 256 && utf8.ValidString(a.ReferenceID) {
			return 1
		}
	}
	return -1
}

// SequenceLength counts UTF-16 text units and typed tokens, or -1 if invalid.
func SequenceLength(sequence []Atom) int {
	n := 0
	for _, a := range sequence {
		length := atomLength(a)
		if length < 0 || n > maxSequenceUnits-length {
			return -1
		}
		n += length
	}
	return n
}

func appendAtoms(out []Atom, atoms ...Atom) []Atom {
	var text strings.Builder
	if len(out) > 0 && out[len(out)-1].Type == "text" {
		text.WriteString(out[len(out)-1].Text)
		out = out[:len(out)-1]
	}
	flush := func() {
		if text.Len() > 0 {
			out = append(out, Atom{Type: "text", Text: text.String()})
			text.Reset()
		}
	}
	for _, a := range atoms {
		if a.Type == "text" {
			text.WriteString(a.Text)
		} else {
			flush()
			out = append(out, a)
		}
	}
	flush()
	return out
}

func appendComponent(out SequenceEdit, c SequenceComponent) SequenceEdit {
	if c.Retain == 0 && c.Delete == 0 && len(c.Insert) == 0 {
		return out
	}
	if len(out) > 0 {
		last := &out[len(out)-1]
		switch {
		case last.Retain > 0 && c.Retain > 0:
			last.Retain += c.Retain
			return out
		case last.Delete > 0 && c.Delete > 0:
			last.Delete += c.Delete
			return out
		case len(last.Insert) > 0 && len(c.Insert) > 0:
			last.Insert = appendAtoms(last.Insert, c.Insert...)
			return out
		}
	}
	if c.Insert != nil {
		c.Insert = appendAtoms([]Atom{}, c.Insert...)
	}
	return append(out, c)
}

func completeSequence(edit SequenceEdit, baseLength int) (SequenceEdit, error) {
	if baseLength < 0 || baseLength > maxSequenceUnits || len(edit) > 10000 {
		return nil, ErrInvalidSequence
	}
	var out SequenceEdit
	consumed := 0
	for _, c := range edit {
		kinds := 0
		for _, set := range []bool{c.Retain != 0, c.Delete != 0, c.Insert != nil} {
			if set {
				kinds++
			}
		}
		if kinds != 1 || c.Retain < 0 || c.Delete < 0 || (c.Insert != nil && (len(c.Insert) == 0 || SequenceLength(c.Insert) < 0)) {
			return nil, ErrInvalidSequence
		}
		if c.Retain > baseLength-consumed || c.Delete > baseLength-consumed {
			return nil, ErrInvalidSequence
		}
		consumed += c.Retain + c.Delete
		out = appendComponent(out, c)
	}
	out = appendComponent(out, SequenceComponent{Retain: baseLength - consumed})
	return out, nil
}

// ApplySequence validates against the original sequence before applying an edit.
func ApplySequence(before []Atom, edit SequenceEdit) ([]Atom, error) {
	complete, err := completeSequence(edit, SequenceLength(before))
	if err != nil {
		return nil, err
	}
	// A rune is indivisible. Consuming one unit of a surrogate pair fails below.
	units := make([]Atom, 0, len(before))
	for _, a := range before {
		if a.Type != "text" {
			units = append(units, a)
			continue
		}
		for _, r := range a.Text {
			units = append(units, Atom{Type: "text", Text: string(r)})
		}
	}
	out := []Atom{}
	index := 0
	for _, c := range complete {
		if c.Insert != nil {
			out = append(out, c.Insert...)
			continue
		}
		n := c.Retain + c.Delete
		for n > 0 {
			if index >= len(units) || atomLength(units[index]) > n {
				return nil, ErrInvalidSequence
			}
			n -= atomLength(units[index])
			if c.Retain > 0 {
				out = append(out, units[index])
			}
			index++
		}
	}
	if SequenceLength(out) < 0 {
		return nil, ErrInvalidSequence
	}
	return appendAtoms([]Atom{}, out...), nil
}

// TransformSequencePair returns A across B and B across A. A has authoritative
// priority at a common insertion boundary; neither input is mutated.
func TransformSequencePair(a, b SequenceEdit, baseLength int) (SequenceEdit, SequenceEdit, error) {
	left, err := completeSequence(a, baseLength)
	if err != nil {
		return nil, nil, err
	}
	right, err := completeSequence(b, baseLength)
	if err != nil {
		return nil, nil, err
	}
	ap, bp := SequenceEdit{}, SequenceEdit{}
	i, j := 0, 0
	for i < len(left) || j < len(right) {
		if i < len(left) && left[i].Insert != nil {
			ap = appendComponent(ap, left[i])
			bp = appendComponent(bp, SequenceComponent{Retain: SequenceLength(left[i].Insert)})
			i++
			continue
		}
		if j < len(right) && right[j].Insert != nil {
			bp = appendComponent(bp, right[j])
			ap = appendComponent(ap, SequenceComponent{Retain: SequenceLength(right[j].Insert)})
			j++
			continue
		}
		if i == len(left) || j == len(right) {
			return nil, nil, ErrInvalidSequence
		}
		x, y := &left[i], &right[j]
		n := min(x.Retain+x.Delete, y.Retain+y.Delete)
		switch {
		case x.Retain > 0 && y.Retain > 0:
			ap = appendComponent(ap, SequenceComponent{Retain: n})
			bp = appendComponent(bp, SequenceComponent{Retain: n})
		case x.Delete > 0 && y.Retain > 0:
			ap = appendComponent(ap, SequenceComponent{Delete: n})
		case x.Retain > 0 && y.Delete > 0:
			bp = appendComponent(bp, SequenceComponent{Delete: n})
		}
		if x.Retain > 0 {
			x.Retain -= n
		} else {
			x.Delete -= n
		}
		if y.Retain > 0 {
			y.Retain -= n
		} else {
			y.Delete -= n
		}
		if x.Retain+x.Delete == 0 {
			i++
		}
		if y.Retain+y.Delete == 0 {
			j++
		}
	}
	return ap, bp, nil
}

// InvertSequence records the actual inserted/deleted contribution, not a diff of
// the final string. The inverse still requires history transformation before use.
func InvertSequence(before []Atom, edit SequenceEdit) (SequenceEdit, error) {
	if _, err := ApplySequence(before, edit); err != nil {
		return nil, err
	}
	complete, err := completeSequence(edit, SequenceLength(before))
	if err != nil {
		return nil, err
	}
	units := []Atom{}
	for _, a := range before {
		if a.Type == "text" {
			for _, r := range a.Text {
				units = append(units, Atom{Type: "text", Text: string(r)})
			}
		} else {
			units = append(units, a)
		}
	}
	inverse := SequenceEdit{}
	index := 0
	for _, c := range complete {
		if c.Insert != nil {
			inverse = appendComponent(inverse, SequenceComponent{Delete: SequenceLength(c.Insert)})
			continue
		}
		start := index
		remaining := c.Retain + c.Delete
		for remaining > 0 {
			remaining -= atomLength(units[index])
			index++
		}
		if c.Retain > 0 {
			inverse = appendComponent(inverse, SequenceComponent{Retain: c.Retain})
		} else {
			inverse = appendComponent(inverse, SequenceComponent{Insert: appendAtoms([]Atom{}, units[start:index]...)})
		}
	}
	return inverse, nil
}
