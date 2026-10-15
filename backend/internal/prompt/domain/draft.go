package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"unicode/utf8"
)

var (
	// ErrInputStale means the saved generation intent changed after quoting.
	ErrInputStale = errors.New("quote_source_stale")
	// ErrInvalidDraft rejects malformed or inconsistent editor intent.
	ErrInvalidDraft = errors.New("invalid_prompt_draft")
	// ErrUnsupportedSchema prevents unknown versions from being overwritten.
	ErrUnsupportedSchema = errors.New("unsupported_schema")
	// ErrLabelConflict rejects reassignment or loss of existing display numbers.
	ErrLabelConflict = errors.New("label_allocation_conflict")
	// ErrReferenceUnavailable rejects unavailable or unauthorized source facts.
	ErrReferenceUnavailable = errors.New("reference_unavailable")
)

// Draft is the complete editable intent. It never contains resource URLs or readiness claims.
type Draft struct {
	SchemaVersion int            `json:"schemaVersion"`
	Document      Document       `json:"document"`
	References    []Reference    `json:"references"`
	ModelVersion  *string        `json:"modelVersion"`
	Mode          *string        `json:"mode"`
	Params        map[string]any `json:"params"`
	LabelState    LabelState     `json:"labelState"`
}

// Document is the versioned prompt tree root.
type Document struct {
	Type    string      `json:"type"`
	Content []Paragraph `json:"content"`
}

// Paragraph holds text, breaks and atomic references.
type Paragraph struct {
	Type    string   `json:"type"`
	Content []Inline `json:"content,omitempty"`
}

// Inline is one discriminated inline node.
type Inline struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	Attrs *ReferenceAttrs `json:"attrs,omitempty"`
}

// ReferenceAttrs links a mention to its stable binding.
type ReferenceAttrs struct {
	ReferenceID string `json:"referenceId"`
}

// Reference pins source facts, role and ordering in a draft.
type Reference struct {
	ID                string          `json:"id"`
	Source            ReferenceSource `json:"source"`
	BindingGeneration int64           `json:"bindingGeneration"`
	MediaKind         string          `json:"mediaKind"`
	Role              string          `json:"role"`
	Order             int             `json:"order"`
}

// ReferenceSource holds exactly one authorized source variant.
type ReferenceSource struct {
	Kind            string        `json:"kind"`
	ClientUploadID  string        `json:"clientUploadId,omitempty"`
	UploadAttemptID string        `json:"uploadAttemptId,omitempty"`
	UploadSessionID string        `json:"uploadSessionId,omitempty"`
	SourceID        string        `json:"sourceId,omitempty"`
	SourceVersion   string        `json:"sourceVersion,omitempty"`
	Description     string        `json:"description,omitempty"`
	Media           []SourceMedia `json:"media,omitempty"`
}

// SourceMedia pins a physical media version.
type SourceMedia struct {
	SourceID      string `json:"sourceId"`
	SourceVersion string `json:"sourceVersion"`
	MediaKind     string `json:"mediaKind"`
}

// LabelAllocation assigns a stable type-specific display number.
type LabelAllocation struct {
	LabelKind string `json:"labelKind"`
	Ordinal   int64  `json:"ordinal"`
}

// LabelState persists allocations and monotonic counters, including tombstones.
type LabelState struct {
	HighWater   map[string]int64           `json:"highWater"`
	Allocations map[string]LabelAllocation `json:"allocations"`
}

// DecodeDraft strictly validates the supported JSON schema.
func DecodeDraft(body []byte) (Draft, error) {
	var version struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if len(body) > 1<<20 || !utf8.Valid(body) || json.Unmarshal(body, &version) != nil {
		return Draft{}, ErrInvalidDraft
	}
	if version.SchemaVersion != 1 {
		return Draft{}, ErrUnsupportedSchema
	}
	var d Draft
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&d) != nil || decoder.Decode(new(any)) != io.EOF {
		return Draft{}, ErrInvalidDraft
	}
	return d, d.Validate()
}
func identifier(s string) bool {
	return len(s) > 0 && len(s) <= 256 && utf8.ValidString(s) && s != "__proto__" && s != "constructor" && s != "prototype"
}
func mediaKind(s string) bool { return s == "image" || s == "video" || s == "audio" }
func labelKind(s string) bool { return mediaKind(s) || s == "subject" }

// Validate checks complete draft shape and cross-reference invariants.
func (d Draft) Validate() error {
	if d.SchemaVersion != 1 {
		return ErrUnsupportedSchema
	}
	if d.Document.Type != "doc" || len(d.Document.Content) == 0 || len(d.Document.Content) > 10000 || d.References == nil || len(d.References) > 255 || d.Params == nil || len(d.Params) > 100 || (d.ModelVersion != nil && !identifier(*d.ModelVersion)) || (d.Mode != nil && !identifier(*d.Mode)) {
		return ErrInvalidDraft
	}
	for k, v := range d.Params {
		if !identifier(k) {
			return ErrInvalidDraft
		}
		switch p := v.(type) {
		case nil, bool:
		case string:
			if len(p) > 10000 || !utf8.ValidString(p) {
				return ErrInvalidDraft
			}
		case float64:
			if math.IsInf(p, 0) || math.IsNaN(p) {
				return ErrInvalidDraft
			}
		default:
			return ErrInvalidDraft
		}
	}
	l := d.LabelState
	if len(l.HighWater) != 4 || l.Allocations == nil || len(l.Allocations) > 10000 {
		return ErrInvalidDraft
	}
	for _, k := range []string{"image", "video", "audio", "subject"} {
		n, ok := l.HighWater[k]
		if !ok || n < 0 || n > 9007199254740991 {
			return ErrInvalidDraft
		}
	}
	used := map[LabelAllocation]bool{}
	for id, a := range l.Allocations {
		if !identifier(id) || !labelKind(a.LabelKind) || a.Ordinal < 1 || a.Ordinal > l.HighWater[a.LabelKind] || used[a] {
			return ErrInvalidDraft
		}
		used[a] = true
	}
	refs := map[string]bool{}
	for i, r := range d.References {
		if !identifier(r.ID) || !identifier(r.Role) || !mediaKind(r.MediaKind) || r.Order != i || r.BindingGeneration < 1 || r.BindingGeneration > 9007199254740991 || refs[r.ID] {
			return ErrInvalidDraft
		}
		refs[r.ID] = true
		s := r.Source
		k := r.MediaKind
		switch s.Kind {
		case "pending_upload":
			if !identifier(s.ClientUploadID) || !identifier(s.UploadAttemptID) || (s.UploadSessionID != "" && !identifier(s.UploadSessionID)) || s.SourceID != "" || s.SourceVersion != "" || s.Description != "" || s.Media != nil {
				return ErrInvalidDraft
			}
		case "media", "subject":
			if !identifier(s.SourceID) || !identifier(s.SourceVersion) || s.ClientUploadID != "" || s.UploadAttemptID != "" || s.UploadSessionID != "" {
				return ErrInvalidDraft
			}
			if s.Kind == "media" && (s.Description != "" || s.Media != nil) {
				return ErrInvalidDraft
			}
			if s.Kind == "subject" {
				k = "subject"
				if len(s.Description) > 10000 || !utf8.ValidString(s.Description) || len(s.Media) > 255 {
					return ErrInvalidDraft
				}
				for _, m := range s.Media {
					if !identifier(m.SourceID) || !identifier(m.SourceVersion) || !mediaKind(m.MediaKind) {
						return ErrInvalidDraft
					}
				}
			}
		default:
			return ErrInvalidDraft
		}
		if a, ok := l.Allocations[r.ID]; !ok || a.LabelKind != k {
			return ErrInvalidDraft
		}
	}
	size := 0
	for _, p := range d.Document.Content {
		if p.Type != "paragraph" {
			return ErrInvalidDraft
		}
		for _, n := range p.Content {
			size += len(n.Text) + 1
			if size > 65536 {
				return ErrInvalidDraft
			}
			switch n.Type {
			case "text":
				if n.Text == "" || !utf8.ValidString(n.Text) || n.Attrs != nil {
					return ErrInvalidDraft
				}
			case "hardBreak":
				if n.Text != "" || n.Attrs != nil {
					return ErrInvalidDraft
				}
			case "reference":
				if n.Text != "" || n.Attrs == nil || !refs[n.Attrs.ReferenceID] {
					return ErrInvalidDraft
				}
			default:
				return ErrInvalidDraft
			}
		}
	}
	return nil
}

// ExtendsLabels rejects reclamation of any previously committed allocation.
func (d Draft) ExtendsLabels(previous LabelState) error {
	for k, n := range previous.HighWater {
		if d.LabelState.HighWater[k] < n {
			return ErrLabelConflict
		}
	}
	for id, a := range previous.Allocations {
		if d.LabelState.Allocations[id] != a {
			return ErrLabelConflict
		}
	}
	return nil
}
func hashValue(value any) string {
	body, _ := json.Marshal(value)
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}

// Hashes separates persisted metadata from generation intent. Inactive tombstones never invalidate canvas input revisions.
func (d Draft) Hashes() (draftHash, inputHash string) {
	draftHash = hashValue(d)
	active := map[string]LabelAllocation{}
	for _, r := range d.References {
		active[r.ID] = d.LabelState.Allocations[r.ID]
	}
	d.LabelState = LabelState{Allocations: active}
	return draftHash, hashValue(d)
}

// CompiledMedia is one deduplicated physical input and role.
type CompiledMedia struct {
	SourceMedia
	Role string `json:"role"`
}

// ReferenceMapping maps a display label to one-based physical input positions.
type ReferenceMapping struct {
	ReferenceID string `json:"reference_id"`
	Label       string `json:"label"`
	MediaSeqNos []int  `json:"media_seq_nos"`
	Role        string `json:"role"`
}

// CompilerVersion identifies the persisted mapping and text rules.
const CompilerVersion = "prompt-reference-v1"

// CompiledDraft is the deterministic text and ordered media projection.
type CompiledDraft struct {
	Mapping []ReferenceMapping `json:"mapping"`
	Prompt  string             `json:"prompt"`
	Media   []CompiledMedia    `json:"media"`
}

// Compile uses authorized frozen facts supplied by the storage boundary; repeated mentions do not duplicate physical inputs.
func (d Draft) Compile() (CompiledDraft, error) {
	if err := d.Validate(); err != nil {
		return CompiledDraft{}, err
	}
	out := CompiledDraft{Media: []CompiledMedia{}}
	seen := map[CompiledMedia]int{}
	subjects := []string{}
	add := func(m SourceMedia, role string) int {
		item := CompiledMedia{m, role}
		if seen[item] == 0 {
			out.Media = append(out.Media, item)
			seen[item] = len(out.Media)
		}
		return seen[item]
	}
	for _, r := range d.References {
		s := r.Source
		mapping := ReferenceMapping{ReferenceID: r.ID, Label: d.Label(r.ID), Role: r.Role, MediaSeqNos: []int{}}
		switch s.Kind {
		case "pending_upload":
			return CompiledDraft{}, ErrReferenceUnavailable
		case "media":
			mapping.MediaSeqNos = append(mapping.MediaSeqNos, add(SourceMedia{s.SourceID, s.SourceVersion, r.MediaKind}, r.Role))
		case "subject":
			subjects = append(subjects, "["+d.Label(r.ID)+"] "+s.Description)
			for _, m := range s.Media {
				mapping.MediaSeqNos = append(mapping.MediaSeqNos, add(m, r.Role))
			}
		}
		out.Mapping = append(out.Mapping, mapping)
	}
	var text strings.Builder
	for i, p := range d.Document.Content {
		if i > 0 {
			text.WriteByte('\n')
		}
		for _, n := range p.Content {
			switch n.Type {
			case "text":
				text.WriteString(n.Text)
			case "hardBreak":
				text.WriteByte('\n')
			case "reference":
				text.WriteString("[" + d.Label(n.Attrs.ReferenceID) + "]")
			}
		}
	}
	out.Prompt = text.String()
	if len(subjects) > 0 {
		out.Prompt += "\n" + strings.Join(subjects, "\n")
	}
	return out, nil
}

// Label derives the display name from persistent allocation metadata.
func (d Draft) Label(id string) string {
	a := d.LabelState.Allocations[id]
	name := map[string]string{"image": "图片", "video": "视频", "audio": "音频", "subject": "主体"}[a.LabelKind]
	n, _ := json.Marshal(a.Ordinal)
	return name + string(n)
}

// MarshalJSON preserves the required empty fields of a subject variant.
func (s ReferenceSource) MarshalJSON() ([]byte, error) {
	switch s.Kind {
	case "pending_upload":
		return json.Marshal(struct {
			Kind            string `json:"kind"`
			ClientUploadID  string `json:"clientUploadId"`
			UploadAttemptID string `json:"uploadAttemptId"`
			UploadSessionID string `json:"uploadSessionId,omitempty"`
		}{s.Kind, s.ClientUploadID, s.UploadAttemptID, s.UploadSessionID})
	case "subject":
		media := s.Media
		if media == nil {
			media = []SourceMedia{}
		}
		return json.Marshal(struct {
			Kind          string        `json:"kind"`
			SourceID      string        `json:"sourceId"`
			SourceVersion string        `json:"sourceVersion"`
			Description   string        `json:"description"`
			Media         []SourceMedia `json:"media"`
		}{s.Kind, s.SourceID, s.SourceVersion, s.Description, media})
	default:
		return json.Marshal(struct {
			Kind          string `json:"kind"`
			SourceID      string `json:"sourceId"`
			SourceVersion string `json:"sourceVersion"`
		}{s.Kind, s.SourceID, s.SourceVersion})
	}
}

// UnmarshalJSON rejects fields belonging to another source variant, including empty values.
func (s *ReferenceSource) UnmarshalJSON(body []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return ErrInvalidDraft
	}
	var kind string
	if json.Unmarshal(fields["kind"], &kind) != nil {
		return ErrInvalidDraft
	}
	allowed := map[string]bool{"kind": true}
	var required []string
	switch kind {
	case "pending_upload":
		required = []string{"clientUploadId", "uploadAttemptId"}
		allowed["uploadSessionId"] = true
	case "media":
		required = []string{"sourceId", "sourceVersion"}
	case "subject":
		required = []string{"sourceId", "sourceVersion", "description", "media"}
	default:
		return ErrInvalidDraft
	}
	for _, key := range required {
		allowed[key] = true
		if raw, ok := fields[key]; !ok || bytes.Equal(raw, []byte("null")) {
			return ErrInvalidDraft
		}
	}
	for key := range fields {
		if !allowed[key] {
			return ErrInvalidDraft
		}
	}
	type source ReferenceSource
	var value source
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil {
		return ErrInvalidDraft
	}
	*s = ReferenceSource(value)
	return nil
}

// UnmarshalJSON rejects attributes that do not belong to the inline node variant.
func (n *Inline) UnmarshalJSON(body []byte) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return ErrInvalidDraft
	}
	var kind string
	if json.Unmarshal(fields["type"], &kind) != nil {
		return ErrInvalidDraft
	}
	allowed := map[string]bool{"type": true}
	switch kind {
	case "text":
		allowed["text"] = true
	case "hardBreak":
	case "reference":
		allowed["attrs"] = true
	default:
		return ErrInvalidDraft
	}
	for key := range fields {
		if !allowed[key] {
			return ErrInvalidDraft
		}
	}
	type inline Inline
	var value inline
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil {
		return ErrInvalidDraft
	}
	*n = Inline(value)
	return nil
}
