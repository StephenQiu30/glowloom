package extract

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

const wordNamespace = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

type wordNode struct {
	name     xml.Name
	attrs    []xml.Attr
	children []*wordNode
	text     strings.Builder
}

func (n *wordNode) child(name string) *wordNode {
	for _, child := range n.children {
		if child.name.Space == wordNamespace && child.name.Local == name {
			return child
		}
	}
	return nil
}

func (n *wordNode) value(name string) string {
	for _, attr := range n.attrs {
		if attr.Name.Local == name && (attr.Name.Space == "" || attr.Name.Space == wordNamespace || name == "id" && attr.Name.Space == "http://schemas.openxmlformats.org/officeDocument/2006/relationships") {
			return attr.Value
		}
	}
	return ""
}

func parseWordXML(ctx context.Context, data []byte) (*wordNode, error) {
	decoder := xml.NewDecoder(contextReader{ctx: ctx, reader: bytes.NewReader(data)})
	var stack []*wordNode
	var root *wordNode
	nodes := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			nodes++
			if nodes > 200000 || len(stack) >= 64 {
				return nil, application.ErrExtractionFailed
			}
			seenAttributes := make(map[xml.Name]bool, len(value.Attr))
			for _, attribute := range value.Attr {
				if seenAttributes[attribute.Name] {
					return nil, application.ErrExtractionFailed
				}
				seenAttributes[attribute.Name] = true
			}
			node := &wordNode{name: value.Name, attrs: slices.Clone(value.Attr)}
			if len(stack) == 0 {
				if root != nil {
					return nil, application.ErrExtractionFailed
				}
				root = node
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, node)
			}
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, application.ErrExtractionFailed
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text.Write(value)
			} else if strings.TrimSpace(string(value)) != "" {
				return nil, application.ErrExtractionFailed
			}
		case xml.Directive:
			return nil, application.ErrExtractionFailed
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, application.ErrExtractionFailed
	}
	return root, nil
}

func wordParts(ctx context.Context, data []byte) (map[string][]byte, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(archive.File) > 4096 {
		return nil, application.ErrExtractionFailed
	}
	parts := make(map[string][]byte, len(archive.File))
	seen := make(map[string]bool, len(archive.File))
	var expanded int64
	for _, entry := range archive.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := entry.Name
		if name == "" || strings.ContainsAny(name, "\\\x00") || strings.HasPrefix(name, "/") || strings.TrimSuffix(name, "/") != path.Clean(name) || strings.HasPrefix(name, "../") || entry.Flags&1 != 0 || entry.UncompressedSize64 > 16<<20 || seen[name] || strings.Contains(strings.ToLower(name), "vbaproject") || entry.Mode()&fs.ModeType != 0 && !entry.FileInfo().IsDir() {
			return nil, application.ErrExtractionFailed
		}
		seen[name] = true
		if entry.FileInfo().IsDir() {
			continue
		}
		reader, err := entry.Open()
		if err != nil {
			return nil, err
		}
		content, readErr := io.ReadAll(io.LimitReader(contextReader{ctx: ctx, reader: reader}, (16<<20)+1))
		closeErr := reader.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			return nil, err
		}
		expanded += int64(len(content))
		if len(content) > 16<<20 || uint64(len(content)) != entry.UncompressedSize64 || expanded > 64<<20 {
			return nil, application.ErrExtractionFailed
		}
		if strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".rels") {
			if _, err := parseWordXML(ctx, content); err != nil {
				return nil, err
			}
		}
		parts[name] = content
	}
	if err := validateWordMainParts(ctx, parts); err != nil {
		return nil, err
	}
	return parts, nil
}

func warn(result *application.ExtractedDocument, code string) {
	for i := range result.Warnings {
		if result.Warnings[i].Code == code {
			result.Warnings[i].Count++
			return
		}
	}
	result.Warnings = append(result.Warnings, application.ExtractionWarning{Code: code, Count: 1})
}

func extractDOCX(ctx context.Context, data []byte) (application.ExtractedDocument, error) {
	result := application.ExtractedDocument{Document: domain.RichDocument{Type: "doc"}, Encoding: "docx-xml"}
	parts, err := wordParts(ctx, data)
	if err != nil {
		return result, err
	}
	root, err := parseWordXML(ctx, parts["word/document.xml"])
	if err != nil || root.name.Space != wordNamespace || root.name.Local != "document" || root.child("body") == nil {
		return result, application.ErrExtractionFailed
	}
	links, err := wordLinks(ctx, parts["word/_rels/document.xml.rels"])
	if err != nil {
		return result, err
	}
	numbering, err := wordNumbering(ctx, parts["word/numbering.xml"])
	if err != nil {
		return result, err
	}
	list := wordListState{rootIndex: -1}
	var visit func(*wordNode) error
	visit = func(node *wordNode) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if node.name.Space != wordNamespace {
			warn(&result, "unsupported_document_element")
			return nil
		}
		switch node.name.Local {
		case "p":
			paragraph, err := wordParagraph(node, links, &result)
			if err != nil {
				return err
			}
			props := node.child("pPr")
			if props != nil && props.child("numPr") != nil {
				if err := list.append(&result, paragraph, props.child("numPr"), numbering); err != nil {
					return err
				}
			} else {
				list.rootIndex = -1
				result.Document.Content = append(result.Document.Content, paragraph)
			}
		case "tbl":
			warn(&result, "table_layout_flattened")
			for _, child := range node.children {
				if err := visit(child); err != nil {
					return err
				}
			}
		case "body", "tr", "tc", "sdt", "sdtContent":
			for _, child := range node.children {
				if err := visit(child); err != nil {
					return err
				}
			}
		case "sectPr", "tblPr", "tblGrid", "trPr", "tcPr", "sdtPr":
			warn(&result, "document_layout_omitted")
		default:
			warn(&result, "unsupported_document_element")
		}
		return nil
	}
	if err := visit(root.child("body")); err != nil {
		return result, err
	}
	raw, err := json.Marshal(result.Document)
	if err != nil {
		return result, err
	}
	result.Document, err = domain.DecodeRichDocument(raw)
	if err != nil {
		return result, err
	}
	result.Mapping = wordMapping(result.Document)
	return result, nil
}

func wordLinks(ctx context.Context, data []byte) (map[string]string, error) {
	links := make(map[string]string)
	if len(data) == 0 {
		return links, nil
	}
	root, err := parseWordXML(ctx, data)
	if err != nil {
		return nil, err
	}
	for _, node := range root.children {
		if strings.HasSuffix(node.value("Type"), "/hyperlink") && node.value("TargetMode") == "External" {
			links[node.value("Id")] = node.value("Target")
		}
	}
	return links, nil
}

func wordParagraph(node *wordNode, links map[string]string, result *application.ExtractedDocument) (domain.RichDocument, error) {
	paragraph := domain.RichDocument{Type: "paragraph"}
	if props := node.child("pPr"); props != nil {
		for _, prop := range props.children {
			switch prop.name.Local {
			case "pStyle":
				style := strings.ToLower(strings.ReplaceAll(prop.value("val"), " ", ""))
				if strings.HasPrefix(style, "heading") {
					level, err := strconv.Atoi(strings.TrimPrefix(style, "heading"))
					if err == nil && level >= 1 && level <= 3 {
						paragraph.Type, paragraph.Attrs = "heading", &domain.RichAttrs{Level: level}
					} else {
						warn(result, "unsupported_heading_style")
					}
				} else if style != "normal" {
					warn(result, "paragraph_style_omitted")
				}
			case "jc":
				align := prop.value("val")
				if align == "both" {
					align = "justify"
				}
				if !slices.Contains([]string{"left", "center", "right", "justify"}, align) {
					warn(result, "unsupported_alignment")
					continue
				}
				if paragraph.Attrs == nil {
					paragraph.Attrs = &domain.RichAttrs{}
				}
				paragraph.Attrs.Align = align
			case "numPr":
			default:
				warn(result, "paragraph_layout_omitted")
			}
		}
	}
	var visit func(*wordNode, []domain.RichMark) error
	visit = func(child *wordNode, inherited []domain.RichMark) error {
		if child.name.Space != wordNamespace {
			warn(result, "unsupported_paragraph_content")
			return nil
		}
		switch child.name.Local {
		case "pPr", "bookmarkStart", "bookmarkEnd", "proofErr":
			return nil
		case "r":
			marks := slices.Clone(inherited)
			if props := child.child("rPr"); props != nil {
				marks = append(marks, wordRunMarks(props, result)...)
			}
			for _, part := range child.children {
				if part.name.Space != wordNamespace {
					warn(result, "unsupported_run_content")
					continue
				}
				switch part.name.Local {
				case "rPr":
				case "t":
					if len(part.children) != 0 {
						return application.ErrExtractionFailed
					}
					if part.text.Len() != 0 {
						paragraph.Content = append(paragraph.Content, domain.RichDocument{Type: "text", Text: domain.NormalizeText(part.text.String()), Marks: slices.Clone(marks)})
					}
				case "br", "cr":
					paragraph.Content = append(paragraph.Content, domain.RichDocument{Type: "hardBreak"})
				case "tab":
					paragraph.Content = append(paragraph.Content, domain.RichDocument{Type: "text", Text: "\t", Marks: slices.Clone(marks)})
				case "drawing", "pict", "object":
					warn(result, "embedded_media_omitted")
				default:
					warn(result, "unsupported_run_content")
				}
			}
		case "hyperlink":
			address := links[child.value("id")]
			if anchor := child.value("anchor"); anchor != "" {
				address = "#" + anchor
			}
			marks := slices.Clone(inherited)
			if domain.ValidLink(address) {
				marks = append(marks, domain.RichMark{Type: "link", Attrs: &domain.MarkAttrs{Href: address}})
			} else {
				warn(result, "unsupported_hyperlink")
			}
			for _, part := range child.children {
				if err := visit(part, marks); err != nil {
					return err
				}
			}
		case "ins", "sdt", "sdtContent":
			warn(result, "document_revision_omitted")
			for _, part := range child.children {
				if err := visit(part, inherited); err != nil {
					return err
				}
			}
		default:
			warn(result, "unsupported_paragraph_content")
		}
		return nil
	}
	for _, child := range node.children {
		if err := visit(child, nil); err != nil {
			return domain.RichDocument{}, err
		}
	}
	return paragraph, nil
}

func wordRunMarks(props *wordNode, result *application.ExtractedDocument) []domain.RichMark {
	var marks []domain.RichMark
	for _, prop := range props.children {
		if slices.Contains([]string{"0", "false", "off", "none"}, prop.value("val")) {
			continue
		}
		mark := domain.RichMark{}
		switch prop.name.Local {
		case "b":
			mark.Type = "bold"
		case "i":
			mark.Type = "italic"
		case "u":
			mark.Type = "underline"
			if prop.value("val") != "" && prop.value("val") != "single" {
				warn(result, "underline_style_omitted")
			}
		case "strike", "dstrike":
			mark.Type = "strike"
		case "color":
			value := prop.value("val")
			if value == "auto" {
				continue
			}
			if !domain.ValidColor("#" + value) {
				warn(result, "unsupported_text_color")
				continue
			}
			mark = domain.RichMark{Type: "textColor", Attrs: &domain.MarkAttrs{Color: "#" + value}}
		case "highlight":
			color := prop.value("val")
			if !domain.ValidColor(color) {
				warn(result, "unsupported_highlight_color")
				continue
			}
			mark = domain.RichMark{Type: "highlight", Attrs: &domain.MarkAttrs{Color: color}}
		default:
			warn(result, "run_format_omitted")
			continue
		}
		if slices.ContainsFunc(marks, func(existing domain.RichMark) bool { return existing.Type == mark.Type }) {
			warn(result, "duplicate_run_format")
			continue
		}
		marks = append(marks, mark)
	}
	return marks
}

func wordMapping(doc domain.RichDocument) []application.SourceMapping {
	var mappings []application.SourceMapping
	var walk func(domain.RichDocument, int) int
	walk = func(node domain.RichDocument, start int) int {
		if node.Type == "text" {
			return start + utf8.RuneCountInString(node.Text)
		}
		if node.Type == "hardBreak" {
			return start + 1
		}
		next := start
		for i, child := range node.Content {
			if i > 0 {
				switch node.Type {
				case "paragraph", "heading", "codeBlock":
				case "orderedList", "bulletList":
					next++
				default:
					next += 2
				}
			}
			next = walk(child, next)
		}
		if node.Type == "paragraph" || node.Type == "heading" {
			paragraph := len(mappings)
			mappings = append(mappings, application.SourceMapping{Part: "word/document.xml", Paragraph: &paragraph, Start: start, End: next})
		}
		return next
	}
	walk(doc, 0)
	return mappings
}

type wordNumberFormat struct {
	kind  string
	start int
}
type wordListState struct {
	rootIndex int
	numberID  string
}

func wordNumbering(ctx context.Context, data []byte) (map[string]map[int]wordNumberFormat, error) {
	result := make(map[string]map[int]wordNumberFormat)
	if len(data) == 0 {
		return result, nil
	}
	root, err := parseWordXML(ctx, data)
	if err != nil {
		return nil, err
	}
	abstract := make(map[string]map[int]wordNumberFormat)
	for _, node := range root.children {
		if node.name.Local != "abstractNum" {
			continue
		}
		levels := make(map[int]wordNumberFormat)
		for _, level := range node.children {
			if level.name.Local != "lvl" || level.child("numFmt") == nil {
				continue
			}
			position, err := strconv.Atoi(level.value("ilvl"))
			if err != nil || position < 0 || position > 8 {
				return nil, application.ErrExtractionFailed
			}
			start := 1
			if node := level.child("start"); node != nil {
				start, err = strconv.Atoi(node.value("val"))
				if err != nil || start < 1 || start > 1000000 {
					return nil, application.ErrExtractionFailed
				}
			}
			levels[position] = wordNumberFormat{kind: level.child("numFmt").value("val"), start: start}
		}
		abstract[node.value("abstractNumId")] = levels
	}
	for _, node := range root.children {
		if node.name.Local == "num" && node.child("abstractNumId") != nil {
			result[node.value("numId")] = abstract[node.child("abstractNumId").value("val")]
		}
	}
	return result, nil
}

func (s *wordListState) append(result *application.ExtractedDocument, paragraph domain.RichDocument, props *wordNode, formats map[string]map[int]wordNumberFormat) error {
	number, level := props.child("numId"), props.child("ilvl")
	if number == nil {
		return application.ErrExtractionFailed
	}
	depth := 0
	if level != nil {
		parsed, err := strconv.Atoi(level.value("val"))
		if err != nil || parsed < 0 || parsed > 8 {
			return application.ErrExtractionFailed
		}
		depth = parsed
	}
	listNode := func(at int) domain.RichDocument {
		format := formats[number.value("val")][at]
		if format.kind == "bullet" {
			return domain.RichDocument{Type: "bulletList"}
		}
		if format.kind != "decimal" {
			warn(result, "unsupported_numbering_format")
		}
		start := format.start
		if start < 1 {
			start = 1
		}
		return domain.RichDocument{Type: "orderedList", Attrs: &domain.RichAttrs{Start: &start}}
	}
	if s.rootIndex < 0 || s.numberID != number.value("val") {
		s.rootIndex, s.numberID = len(result.Document.Content), number.value("val")
		result.Document.Content = append(result.Document.Content, listNode(0))
	}
	container := &result.Document.Content[s.rootIndex]
	for i := 1; i <= depth; i++ {
		if len(container.Content) == 0 {
			warn(result, "numbering_missing_parent")
			break
		}
		parent := &container.Content[len(container.Content)-1]
		if len(parent.Content) == 0 || !slices.Contains([]string{"orderedList", "bulletList"}, parent.Content[len(parent.Content)-1].Type) {
			parent.Content = append(parent.Content, listNode(i))
		}
		container = &parent.Content[len(parent.Content)-1]
	}
	if paragraph.Type != "paragraph" {
		warn(result, "numbered_heading_as_paragraph")
		paragraph.Type = "paragraph"
		if paragraph.Attrs != nil {
			paragraph.Attrs.Level = 0
		}
	}
	container.Content = append(container.Content, domain.RichDocument{Type: "listItem", Content: []domain.RichDocument{paragraph}})
	return nil
}

func validateWordMainParts(ctx context.Context, parts map[string][]byte) error {
	types, err := parseWordXML(ctx, parts["[Content_Types].xml"])
	if err != nil {
		return err
	}
	const contentTypes = "http://schemas.openxmlformats.org/package/2006/content-types"
	if types.name.Space != contentTypes || types.name.Local != "Types" {
		return application.ErrExtractionFailed
	}
	main := false
	for _, part := range types.children {
		mime := part.value("ContentType")
		if strings.Contains(strings.ToLower(mime), "macroenabled") || strings.Contains(strings.ToLower(mime), "vbaproject") {
			return application.ErrExtractionFailed
		}
		if part.name.Space == contentTypes && part.name.Local == "Override" && part.value("PartName") == "/word/document.xml" && mime == "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml" {
			if main {
				return application.ErrExtractionFailed
			}
			main = true
		}
	}
	relationships, err := parseWordXML(ctx, parts["_rels/.rels"])
	if err != nil {
		return err
	}
	const relationshipNS = "http://schemas.openxmlformats.org/package/2006/relationships"
	if relationships.name.Space != relationshipNS || relationships.name.Local != "Relationships" {
		return application.ErrExtractionFailed
	}
	linked := false
	for _, rel := range relationships.children {
		if rel.name.Space == relationshipNS && rel.name.Local == "Relationship" && rel.value("Type") == "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" {
			if linked || rel.value("TargetMode") == "External" || strings.TrimPrefix(rel.value("Target"), "/") != "word/document.xml" {
				return application.ErrExtractionFailed
			}
			linked = true
		}
	}
	if !main || !linked || len(parts["word/document.xml"]) == 0 {
		return application.ErrExtractionFailed
	}
	return nil
}
