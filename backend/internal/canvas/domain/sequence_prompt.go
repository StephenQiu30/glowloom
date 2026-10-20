package domain

import prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"

func sequenceFromPromptJSON(body []byte) ([]Atom, error) {
	var document prompt.Document
	if err := strictSequenceJSON(body, &document); err != nil {
		return nil, err
	}
	return PromptSequence(document)
}

// PromptSequence projects the actual restricted editor schema without sentinels.
func PromptSequence(document prompt.Document) ([]Atom, error) {
	if document.Type != "doc" || len(document.Content) == 0 {
		return nil, ErrInvalidSequence
	}
	out := []Atom{}
	for index, paragraph := range document.Content {
		if paragraph.Type != "paragraph" {
			return nil, ErrInvalidSequence
		}
		if index > 0 {
			out = append(out, Atom{Type: "paragraphBreak"})
		}
		for _, inline := range paragraph.Content {
			switch inline.Type {
			case "text":
				if inline.Attrs != nil {
					return nil, ErrInvalidSequence
				}
				out = append(out, Atom{Type: "text", Text: inline.Text})
			case "hardBreak":
				if inline.Attrs != nil || inline.Text != "" {
					return nil, ErrInvalidSequence
				}
				out = append(out, Atom{Type: "hardBreak"})
			case "reference":
				if inline.Attrs == nil || inline.Text != "" {
					return nil, ErrInvalidSequence
				}
				out = append(out, Atom{Type: "reference", ReferenceID: inline.Attrs.ReferenceID})
			default:
				return nil, ErrInvalidSequence
			}
		}
	}
	if SequenceLength(out) < 0 {
		return nil, ErrInvalidSequence
	}
	return appendAtoms([]Atom{}, out...), nil
}

// SequencePrompt reconstructs paragraphs, soft breaks and stable reference IDs.
func SequencePrompt(sequence []Atom) (prompt.Document, error) {
	if SequenceLength(sequence) < 0 {
		return prompt.Document{}, ErrInvalidSequence
	}
	doc := prompt.Document{Type: "doc", Content: []prompt.Paragraph{{Type: "paragraph"}}}
	for _, atom := range appendAtoms([]Atom{}, sequence...) {
		paragraph := &doc.Content[len(doc.Content)-1]
		switch atom.Type {
		case "paragraphBreak":
			doc.Content = append(doc.Content, prompt.Paragraph{Type: "paragraph"})
		case "text":
			paragraph.Content = append(paragraph.Content, prompt.Inline{Type: "text", Text: atom.Text})
		case "hardBreak":
			paragraph.Content = append(paragraph.Content, prompt.Inline{Type: "hardBreak"})
		case "reference":
			paragraph.Content = append(paragraph.Content, prompt.Inline{Type: "reference", Attrs: &prompt.ReferenceAttrs{ReferenceID: atom.ReferenceID}})
		}
	}
	return doc, nil
}
