package domain

import (
	"bytes"
	"encoding/json"
	"net/url"
	"unicode/utf8"

	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

type richTextNode struct {
	Type    string                     `json:"type"`
	Text    string                     `json:"text,omitempty"`
	Attrs   map[string]json.RawMessage `json:"attrs,omitempty"`
	Marks   []richTextMark             `json:"marks,omitempty"`
	Content []richTextNode             `json:"content,omitempty"`
}
type richTextMark struct {
	Type  string                     `json:"type"`
	Attrs map[string]json.RawMessage `json:"attrs,omitempty"`
}

// Rich text and its reference bindings are accepted in one atomic content field.
// The editor projection keeps formatting; the composer owns authorization and labels.
func validRichText(body json.RawMessage, composer *prompt.Draft) bool {
	if len(body) > 1<<20 || !utf8.Valid(body) {
		return false
	}
	var doc struct {
		SchemaVersion int          `json:"schemaVersion"`
		Content       richTextNode `json:"content"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&doc) != nil || doc.SchemaVersion != 1 || doc.Content.Type != "doc" {
		return false
	}
	bindings := map[string]bool{}
	for _, ref := range composer.References {
		bindings[ref.ID] = true
	}
	count, chars := 0, 0
	var walk func(richTextNode, int) bool
	walk = func(node richTextNode, depth int) bool {
		count++
		chars += utf8.RuneCountInString(node.Text)
		if count > 10000 || chars > 10000 || depth > 32 {
			return false
		}
		switch node.Type {
		case "doc", "paragraph", "heading", "bulletList", "orderedList", "listItem", "blockquote", "codeBlock", "horizontalRule", "hardBreak", "text", "mention", "reference":
		default:
			return false
		}
		if node.Type == "doc" && depth != 0 {
			return false
		}
		if node.Type == "reference" {
			var id string
			if json.Unmarshal(node.Attrs["referenceId"], &id) != nil || !bindings[id] || len(node.Content) > 0 {
				return false
			}
		}
		if node.Type == "mention" {
			var id string
			if json.Unmarshal(node.Attrs["id"], &id) != nil || id == "" || len(id) > 256 {
				return false
			}
		}
		for key := range node.Attrs {
			switch key {
			case "level", "start", "language", "id", "label", "token", "referenceId", "textAlign":
			default:
				return false
			}
		}
		for _, mark := range node.Marks {
			switch mark.Type {
			case "bold", "italic", "underline", "strike", "code":
				if len(mark.Attrs) > 0 {
					return false
				}
			case "link":
				var href string
				if json.Unmarshal(mark.Attrs["href"], &href) != nil {
					return false
				}
				u, err := url.Parse(href)
				if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
					return false
				}
				for key := range mark.Attrs {
					switch key {
					case "href", "target", "rel", "class", "title":
					default:
						return false
					}
				}
			default:
				return false
			}
		}
		for _, child := range node.Content {
			if !walk(child, depth+1) {
				return false
			}
		}
		return true
	}
	return walk(doc.Content, 0)
}
