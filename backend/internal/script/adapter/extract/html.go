// Package extract converts bounded original script inputs into the closed rich schema.
package extract

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// HTML preserves supported source-editor semantics and rejects meaningful loss.
func HTML(input string) (domain.RichDocument, error) {
	if len(input) > domain.MaxDocumentBytes || !utf8.ValidString(input) || strings.ContainsRune(input, 0) {
		return domain.RichDocument{}, domain.ErrInvalidDocument
	}
	contextNode := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(input), contextNode)
	if err != nil {
		return domain.RichDocument{}, fmt.Errorf("%w: html: %w", domain.ErrInvalidDocument, err)
	}
	doc := domain.RichDocument{Type: "doc"}
	for _, node := range nodes {
		items, err := richHTMLNode(node, nil, 1)
		if err != nil {
			return domain.RichDocument{}, err
		}
		doc.Content = append(doc.Content, items...)
	}
	if err := doc.Validate(); err != nil {
		return domain.RichDocument{}, err
	}
	encoded, err := doc.CanonicalJSON()
	if err != nil {
		return domain.RichDocument{}, err
	}
	return domain.DecodeRichDocument(encoded)
}

func richHTMLNode(n *html.Node, marks []domain.RichMark, depth int) ([]domain.RichDocument, error) {
	if depth > domain.MaxDocumentDepth {
		return nil, domain.ErrInvalidDocument
	}
	switch n.Type {
	case html.CommentNode:
		return nil, nil
	case html.TextNode:
		if n.Data == "" {
			return nil, nil
		}
		return []domain.RichDocument{{Type: "text", Text: domain.NormalizeText(n.Data), Marks: slices.Clone(marks)}}, nil
	case html.ElementNode:
	default:
		return nil, domain.ErrInvalidDocument
	}
	block := domain.RichDocument{}
	inline := true
	var mark *domain.RichMark
	switch n.Data {
	case "p":
		block.Type, inline = "paragraph", false
	case "h1", "h2", "h3":
		level, err := strconv.Atoi(strings.TrimPrefix(n.Data, "h"))
		if err != nil {
			return nil, err
		}
		block.Type, block.Attrs, inline = "heading", &domain.RichAttrs{Level: level}, false
	case "ul":
		block.Type, inline = "bulletList", false
	case "ol":
		block.Type, inline = "orderedList", false
	case "li":
		block.Type, inline = "listItem", false
	case "blockquote":
		block.Type, inline = "blockquote", false
	case "pre":
		block.Type, inline = "codeBlock", false
	case "hr":
		block.Type, inline = "horizontalRule", false
	case "br":
		block.Type, inline = "hardBreak", false
	case "strong", "b":
		mark = &domain.RichMark{Type: "bold"}
	case "em", "i":
		mark = &domain.RichMark{Type: "italic"}
	case "u":
		mark = &domain.RichMark{Type: "underline"}
	case "s", "del", "strike":
		mark = &domain.RichMark{Type: "strike"}
	case "a":
		mark = &domain.RichMark{Type: "link", Attrs: &domain.MarkAttrs{}}
	case "mark":
		mark = &domain.RichMark{Type: "highlight", Attrs: &domain.MarkAttrs{}}
	case "code":
		if n.Parent == nil || n.Parent.Data != "pre" {
			mark = &domain.RichMark{Type: "code"}
		}
	case "span":
	default:
		return nil, fmt.Errorf("%w: unsupported tag %s", domain.ErrInvalidDocument, n.Data)
	}
	styleMarks, err := richHTMLAttributes(n, &block, mark)
	if err != nil {
		return nil, err
	}
	childMarks := slices.Clone(marks)
	if mark != nil {
		childMarks = append(childMarks, *mark)
	}
	childMarks = append(childMarks, styleMarks...)
	var children []domain.RichDocument
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode && !inline && block.Type != "paragraph" && block.Type != "heading" && block.Type != "codeBlock" && strings.TrimSpace(child.Data) == "" {
			continue
		}
		items, err := richHTMLNode(child, childMarks, depth+1)
		if err != nil {
			return nil, err
		}
		children = append(children, items...)
		if block.Type == "codeBlock" && child.Type == html.ElementNode && child.Data == "code" {
			for _, attr := range child.Attr {
				if attr.Key == "class" {
					block.Attrs = &domain.RichAttrs{Language: strings.TrimPrefix(attr.Val, "language-")}
				}
			}
		}
	}
	if inline {
		return children, nil
	}
	block.Content = children
	return []domain.RichDocument{block}, nil
}

func richHTMLAttributes(n *html.Node, block *domain.RichDocument, mark *domain.RichMark) ([]domain.RichMark, error) {
	var marks []domain.RichMark
	for _, attr := range n.Attr {
		if attr.Namespace != "" {
			return nil, domain.ErrInvalidDocument
		}
		switch attr.Key {
		case "href":
			if n.Data != "a" || mark == nil || !domain.ValidLink(attr.Val) {
				return nil, domain.ErrInvalidDocument
			}
			mark.Attrs.Href = attr.Val
		case "target":
			if n.Data != "a" || attr.Val != "_blank" {
				return nil, domain.ErrInvalidDocument
			}
		case "rel":
			if n.Data != "a" {
				return nil, domain.ErrInvalidDocument
			}
			for _, token := range strings.Fields(attr.Val) {
				if !slices.Contains([]string{"noopener", "noreferrer", "nofollow"}, token) {
					return nil, domain.ErrInvalidDocument
				}
			}
		case "start":
			value, err := strconv.Atoi(attr.Val)
			if n.Data != "ol" || err != nil || value < 1 {
				return nil, domain.ErrInvalidDocument
			}
			block.Attrs = &domain.RichAttrs{Start: &value}
		case "class":
			if n.Data != "code" || n.Parent == nil || n.Parent.Data != "pre" || !strings.HasPrefix(attr.Val, "language-") || strings.ContainsAny(attr.Val, " \t\n") {
				return nil, domain.ErrInvalidDocument
			}
		case "data-color":
			if n.Data != "mark" || mark == nil || !domain.ValidColor(attr.Val) {
				return nil, domain.ErrInvalidDocument
			}
			color := domain.CanonicalColor(attr.Val)
			if mark.Attrs.Color != "" && mark.Attrs.Color != color {
				return nil, domain.ErrInvalidDocument
			}
			mark.Attrs.Color = color
		case "style":
			for _, entry := range strings.Split(attr.Val, ";") {
				if strings.TrimSpace(entry) == "" {
					continue
				}
				name, value, ok := strings.Cut(entry, ":")
				name, value = strings.TrimSpace(name), strings.TrimSpace(value)
				if !ok {
					return nil, domain.ErrInvalidDocument
				}
				switch name {
				case "text-align":
					if block.Type != "paragraph" && block.Type != "heading" || !slices.Contains([]string{"left", "center", "right", "justify"}, value) {
						return nil, domain.ErrInvalidDocument
					}
					if block.Attrs == nil {
						block.Attrs = &domain.RichAttrs{}
					}
					block.Attrs.Align = value
				case "color":
					if n.Data == "mark" && value == "inherit" {
						continue
					}
					if !domain.ValidColor(value) {
						return nil, domain.ErrInvalidDocument
					}
					marks = append(marks, domain.RichMark{Type: "textColor", Attrs: &domain.MarkAttrs{Color: value}})
				case "background-color":
					if n.Data != "mark" || mark == nil || !domain.ValidColor(value) || mark.Attrs.Color != "" && mark.Attrs.Color != domain.CanonicalColor(value) {
						return nil, domain.ErrInvalidDocument
					}
					mark.Attrs.Color = domain.CanonicalColor(value)
				default:
					return nil, fmt.Errorf("%w: unsupported style %s", domain.ErrInvalidDocument, name)
				}
			}
		default:
			return nil, fmt.Errorf("%w: unsupported attribute %s", domain.ErrInvalidDocument, attr.Key)
		}
	}
	if mark != nil && (mark.Type == "link" && mark.Attrs.Href == "" || mark.Type == "highlight" && mark.Attrs.Color == "") {
		return nil, domain.ErrInvalidDocument
	}
	return marks, nil
}
