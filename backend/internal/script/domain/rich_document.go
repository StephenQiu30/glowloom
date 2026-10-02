// Package domain defines immutable script content and Unicode-scalar coordinates.
package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Script budgets apply before normalization and never silently truncate content.
const (
	MaxDocumentBytes = 8 << 20
	MaxVersionBytes  = 32 << 20
	MaxHTTPBytes     = 36 << 20
	MaxDocumentNodes = 200000
	MaxDocumentDepth = 64
	MaxScalarCount   = 1500000
)

var (
	// ErrInvalidDocument rejects content outside the closed editable rich schema.
	ErrInvalidDocument = errors.New("invalid script rich document")
	// ErrInvalidSpan rejects a coordinate outside the canonical Unicode-scalar text.
	ErrInvalidSpan = errors.New("invalid script scalar span")
)

// RichAttrs contains only the attributes of supported block nodes.
type RichAttrs struct {
	Level    int    `json:"level,omitempty"`
	Align    string `json:"text_align,omitempty"`
	Start    *int   `json:"start,omitempty"`
	Language string `json:"language,omitempty"`
}

// MarkAttrs binds a safe link or an explicit color to an inline mark.
type MarkAttrs struct {
	Href  string `json:"href,omitempty"`
	Color string `json:"color,omitempty"`
}

// RichMark preserves supported inline editing semantics.
type RichMark struct {
	Type  string     `json:"type"`
	Attrs *MarkAttrs `json:"attrs,omitempty"`
}

// RichDocument is the recursive closed document schema, including zero-width nodes.
type RichDocument struct {
	Type    string         `json:"type"`
	Attrs   *RichAttrs     `json:"attrs,omitempty"`
	Text    string         `json:"text,omitempty"`
	Marks   []RichMark     `json:"marks,omitempty"`
	Content []RichDocument `json:"content,omitempty"`
}

// DecodeRichDocument rejects unknown fields, trailing input and surrogate repair.
func DecodeRichDocument(input []byte) (RichDocument, error) {
	var doc RichDocument
	if len(input) > MaxDocumentBytes || !validJSONScalars(input) || !uniqueJSONKeys(input) {
		return doc, ErrInvalidDocument
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&doc); err != nil {
		return RichDocument{}, fmt.Errorf("%w: decode: %w", ErrInvalidDocument, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return RichDocument{}, ErrInvalidDocument
	}
	if err := doc.Validate(); err != nil {
		return RichDocument{}, err
	}
	doc.normalize()
	return doc, nil
}

func (d *RichDocument) normalize() {
	d.Text = NormalizeText(d.Text)
	if d.Attrs != nil && d.Attrs.Level == 0 && d.Attrs.Align == "" && d.Attrs.Start == nil && d.Attrs.Language == "" {
		d.Attrs = nil
	}
	for i := range d.Marks {
		if d.Marks[i].Attrs != nil && d.Marks[i].Attrs.Href == "" && d.Marks[i].Attrs.Color == "" {
			d.Marks[i].Attrs = nil
		}
		if d.Marks[i].Attrs != nil && d.Marks[i].Attrs.Color != "" {
			d.Marks[i].Attrs.Color = CanonicalColor(d.Marks[i].Attrs.Color)
		}
	}
	slices.SortFunc(d.Marks, func(a, b RichMark) int { return strings.Compare(a.Type, b.Type) })
	for i := range d.Content {
		d.Content[i].normalize()
	}
}

// NormalizeText preserves whitespace and Unicode composition while normalizing EOL.
func NormalizeText(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}

// Validate enforces placement, attributes, marks and complete document budgets.
func (d RichDocument) Validate() error {
	if d.Type != "doc" {
		return ErrInvalidDocument
	}
	count := 0
	var check func(RichDocument, string, int) error
	check = func(node RichDocument, parent string, depth int) error {
		count++
		if depth > MaxDocumentDepth || count > MaxDocumentNodes || !utf8.ValidString(node.Text) || strings.ContainsRune(node.Text, 0) {
			return ErrInvalidDocument
		}
		if !validRichPlacement(node.Type, parent) || !validRichAttributes(node) {
			return fmt.Errorf("%w: node %s", ErrInvalidDocument, node.Type)
		}
		if node.Type != "text" && (node.Text != "" || len(node.Marks) != 0) || node.Type == "text" && (node.Text == "" || len(node.Content) != 0) {
			return ErrInvalidDocument
		}
		seen := make(map[string]bool, len(node.Marks))
		for _, mark := range node.Marks {
			if seen[mark.Type] || !validRichMark(mark) || parent == "codeBlock" {
				return ErrInvalidDocument
			}
			seen[mark.Type] = true
		}
		if (node.Type == "hardBreak" || node.Type == "horizontalRule") && len(node.Content) != 0 {
			return ErrInvalidDocument
		}
		if node.Type == "listItem" && (len(node.Content) == 0 || node.Content[0].Type != "paragraph") || (node.Type == "bulletList" || node.Type == "orderedList") && len(node.Content) == 0 {
			return ErrInvalidDocument
		}
		for _, child := range node.Content {
			if err := check(child, node.Type, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := check(d, "", 1); err != nil {
		return err
	}
	text := d.plainText()
	if utf8.RuneCountInString(text) > MaxScalarCount {
		return ErrInvalidDocument
	}
	encoded, err := json.Marshal(d)
	if err != nil || len(encoded) > MaxDocumentBytes {
		return ErrInvalidDocument
	}
	return nil
}

func validRichPlacement(node, parent string) bool {
	switch parent {
	case "":
		return node == "doc"
	case "doc", "blockquote", "listItem":
		return slices.Contains([]string{"paragraph", "heading", "bulletList", "orderedList", "blockquote", "codeBlock", "horizontalRule"}, node)
	case "paragraph", "heading":
		return node == "text" || node == "hardBreak"
	case "codeBlock":
		return node == "text"
	case "bulletList", "orderedList":
		return node == "listItem"
	default:
		return false
	}
}

func validRichAttributes(node RichDocument) bool {
	a := node.Attrs
	if a == nil {
		return node.Type != "heading"
	}
	if a.Level != 0 && node.Type != "heading" || a.Start != nil && node.Type != "orderedList" || a.Language != "" && node.Type != "codeBlock" || a.Align != "" && node.Type != "paragraph" && node.Type != "heading" {
		return false
	}
	if a.Align != "" && !slices.Contains([]string{"left", "center", "right", "justify"}, a.Align) {
		return false
	}
	return (node.Type != "heading" || a.Level >= 1 && a.Level <= 3) && (a.Start == nil || *a.Start >= 0 && *a.Start <= 1000000) && utf8.ValidString(a.Language) && len(a.Language) <= 64 && !strings.ContainsAny(a.Language, "\x00\r\n")
}

func validRichMark(mark RichMark) bool {
	a := mark.Attrs
	switch mark.Type {
	case "bold", "italic", "underline", "strike", "code":
		return a == nil || a.Href == "" && a.Color == ""
	case "link":
		return a != nil && a.Color == "" && ValidLink(a.Href)
	case "textColor", "highlight":
		return a != nil && a.Href == "" && ValidColor(a.Color)
	default:
		return false
	}
}

// ValidLink excludes executable schemes and whitespace-obfuscated addresses.
func ValidLink(value string) bool {
	if value == "" || len(value) > 2048 || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n\t ") || strings.HasPrefix(value, "\\") {
		return false
	}
	u, err := url.Parse(value)
	if err != nil || u.User != nil {
		return false
	}
	return u.Scheme == "" && !strings.HasPrefix(value, "//") || (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" || u.Scheme == "mailto" && u.Opaque != ""
}

var colorPattern = regexp.MustCompile(`^(#[0-9a-fA-F]{3}|#[0-9a-fA-F]{4}|#[0-9a-fA-F]{6}|#[0-9a-fA-F]{8})$`)

// CanonicalColor equates browser RGB serialization and exact hexadecimal colors.
// Fractional alpha remains explicit rather than rounding it to a different value.
func CanonicalColor(value string) string {
	value = strings.ToLower(value)
	if strings.HasPrefix(value, "#") {
		if len(value) == 4 || len(value) == 5 {
			var out strings.Builder
			out.WriteByte('#')
			for _, r := range value[1:] {
				out.WriteRune(r)
				out.WriteRune(r)
			}
			return out.String()
		}
		return value
	}
	body, ok := strings.CutPrefix(value, "rgb(")
	if !ok || !strings.HasSuffix(body, ")") {
		return value
	}
	parts := strings.Split(strings.TrimSuffix(body, ")"), ",")
	if len(parts) != 3 {
		return value
	}
	var channels [3]int
	for i, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 || n > 255 {
			return value
		}
		channels[i] = n
	}
	return fmt.Sprintf("#%02x%02x%02x", channels[0], channels[1], channels[2])
}

// ValidColor supports explicit hex, browser rgb/rgba and the named source colors.
func ValidColor(value string) bool {
	if colorPattern.MatchString(value) {
		return true
	}
	if slices.Contains(strings.Fields("aliceblue antiquewhite aqua aquamarine azure beige bisque black blanchedalmond blue blueviolet brown burlywood cadetblue chartreuse chocolate coral cornflowerblue cornsilk crimson cyan darkblue darkcyan darkgoldenrod darkgray darkgreen darkgrey darkkhaki darkmagenta darkolivegreen darkorange darkorchid darkred darksalmon darkseagreen darkslateblue darkslategray darkslategrey darkturquoise darkviolet deeppink deepskyblue dimgray dimgrey dodgerblue firebrick floralwhite forestgreen fuchsia gainsboro ghostwhite gold goldenrod gray green greenyellow grey honeydew hotpink indianred indigo ivory khaki lavender lavenderblush lawngreen lemonchiffon lightblue lightcoral lightcyan lightgoldenrodyellow lightgray lightgreen lightgrey lightpink lightsalmon lightseagreen lightskyblue lightslategray lightslategrey lightsteelblue lightyellow lime limegreen linen magenta maroon mediumaquamarine mediumblue mediumorchid mediumpurple mediumseagreen mediumslateblue mediumspringgreen mediumturquoise mediumvioletred midnightblue mintcream mistyrose moccasin navajowhite navy oldlace olive olivedrab orange orangered orchid palegoldenrod palegreen paleturquoise palevioletred papayawhip peachpuff peru pink plum powderblue purple rebeccapurple red rosybrown royalblue saddlebrown salmon sandybrown seagreen seashell sienna silver skyblue slateblue slategray slategrey snow springgreen steelblue tan teal thistle tomato turquoise violet wheat white whitesmoke yellow yellowgreen transparent"), strings.ToLower(value)) {
		return true
	}
	for _, name := range []string{"rgb", "rgba"} {
		body, ok := strings.CutPrefix(value, name+"(")
		if !ok || !strings.HasSuffix(body, ")") {
			continue
		}
		parts := strings.Split(strings.TrimSuffix(body, ")"), ",")
		want := 3
		if name == "rgba" {
			want = 4
		}
		if len(parts) != want {
			return false
		}
		for i, part := range parts {
			n, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
			maxValue := 255.0
			if i == 3 {
				maxValue = 1
			}
			if err != nil || !(n >= 0 && n <= maxValue) {
				return false
			}
		}
		return true
	}
	return false
}

// PlainText returns the sole canonical text from which scalar coordinates derive.
func (d RichDocument) PlainText() (string, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}
	return d.plainText(), nil
}

func (d RichDocument) plainText() string {
	if d.Type == "text" {
		return NormalizeText(d.Text)
	}
	if d.Type == "hardBreak" {
		return "\n"
	}
	parts := make([]string, len(d.Content))
	for i, child := range d.Content {
		parts[i] = child.plainText()
	}
	separator := "\n\n"
	switch d.Type {
	case "paragraph", "heading", "codeBlock":
		separator = ""
	case "bulletList", "orderedList":
		separator = "\n"
	}
	return strings.Join(parts, separator)
}

// CanonicalJSON normalizes EOL and mark ordering without erasing rich structure.
func (d RichDocument) CanonicalJSON() ([]byte, error) {
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	copyDoc, err := DecodeRichDocument(raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(copyDoc)
}

// Hashes returns distinct plain-text and canonical rich-semantic SHA256 values.
func (d RichDocument) Hashes() (string, string, error) {
	plain, err := d.PlainText()
	if err != nil {
		return "", "", err
	}
	rich, err := d.CanonicalJSON()
	if err != nil {
		return "", "", err
	}
	return ContentSHA([]byte(plain)), ContentSHA(rich), nil
}

// ContentSHA hashes exact immutable bytes using lowercase hex.
func ContentSHA(value []byte) string {
	sha := sha256.Sum256(value)
	return hex.EncodeToString(sha[:])
}

// ScalarSlice validates and slices canonical Unicode-scalar coordinates.
func ScalarSlice(text string, start, end int) (string, error) {
	if !utf8.ValidString(text) || strings.ContainsRune(text, 0) || start < 0 || end < start || end > MaxScalarCount {
		return "", ErrInvalidSpan
	}
	runes := []rune(text)
	if end > len(runes) {
		return "", ErrInvalidSpan
	}
	return string(runes[start:end]), nil
}

func validJSONScalars(input []byte) bool {
	if !utf8.Valid(input) {
		return false
	}
	quoted := false
	for i := 0; i < len(input); i++ {
		if input[i] == '"' {
			quoted = !quoted
			continue
		}
		if !quoted || input[i] != '\\' {
			continue
		}
		i++
		if i >= len(input) {
			return false
		}
		if input[i] != 'u' {
			continue
		}
		if i+4 >= len(input) {
			return false
		}
		n, err := strconv.ParseUint(string(input[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if n >= 0xDC00 && n <= 0xDFFF {
			return false
		}
		if n < 0xD800 || n > 0xDBFF {
			continue
		}
		if i+6 >= len(input) || string(input[i+1:i+3]) != "\\u" {
			return false
		}
		low, err := strconv.ParseUint(string(input[i+3:i+7]), 16, 16)
		if err != nil || low < 0xDC00 || low > 0xDFFF {
			return false
		}
		i += 6
	}
	return !quoted
}

func uniqueJSONKeys(input []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(input))
	var read func(int) bool
	read = func(depth int) bool {
		if depth > MaxDocumentDepth*3 {
			return false
		}
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return true
		}
		switch delim {
		case '{':
			seen := make(map[string]bool)
			for decoder.More() {
				key, err := decoder.Token()
				name, ok := key.(string)
				if err != nil || !ok || seen[name] || !read(depth+1) {
					return false
				}
				seen[name] = true
			}
		case '[':
			for decoder.More() {
				if !read(depth + 1) {
					return false
				}
			}
		default:
			return false
		}
		_, err = decoder.Token()
		return err == nil
	}
	if !read(0) {
		return false
	}
	_, err := decoder.Token()
	return errors.Is(err, io.EOF)
}

// ValidateJSON rejects malformed Unicode, duplicate fields and unbounded JSON nesting.
// Typed boundary decoders additionally reject unknown fields and trailing values.
func ValidateJSON(data []byte) error {
	if len(data) > MaxHTTPBytes || !utf8.Valid(data) || !validJSONScalars(data) || !uniqueJSONKeys(data) {
		return ErrInvalidDocument
	}
	return nil
}
