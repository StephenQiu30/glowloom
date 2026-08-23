package scripts

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/stephenqiu30/lanverse/backend/src/platform/toolkit"
)

const (
	maxSourceBytes       = 32 << 20
	maxExtractedXMLBytes = 8 << 20
)

type SourceDocument struct {
	Format         string `json:"format"`
	MediaType      string `json:"media_type"`
	OriginalHash   string `json:"original_hash"`
	OriginalLength int    `json:"original_length"`
	Text           string `json:"-"`
	TextHash       string `json:"text_hash"`
	ParagraphCount int    `json:"paragraph_count"`
}

type SourceUpload struct {
	FileName  string
	MediaType string
	Original  []byte
}

func ParseSourceDocument(name, declaredMediaType string, original []byte) (SourceDocument, error) {
	if len(original) == 0 {
		return SourceDocument{}, errors.New("source document must not be empty")
	}
	if len(original) > maxSourceBytes {
		return SourceDocument{}, fmt.Errorf("source document exceeds the %d byte limit", maxSourceBytes)
	}

	format, mediaType, err := sourceFormat(name, declaredMediaType)
	if err != nil {
		return SourceDocument{}, err
	}
	document := SourceDocument{
		Format:         format,
		MediaType:      mediaType,
		OriginalHash:   toolkit.SHA256Hex(original),
		OriginalLength: len(original),
	}

	switch format {
	case "txt", "markdown":
		if !utf8.Valid(original) {
			return SourceDocument{}, errors.New("text source must be valid UTF-8")
		}
		document.Text = normalizeSourceText(string(original))
		document.ParagraphCount = countTextParagraphs(document.Text)
	case "docx":
		document.Text, document.ParagraphCount, err = extractDOCXText(original)
		if err != nil {
			return SourceDocument{}, err
		}
	default:
		return SourceDocument{}, fmt.Errorf("unsupported source format %q", format)
	}

	if err := ValidateSource(document.Text); err != nil {
		return SourceDocument{}, err
	}
	document.TextHash = toolkit.SHA256String(document.Text)
	return document, nil
}

func sourceFormat(name, declaredMediaType string) (string, string, error) {
	mediaType := strings.TrimSpace(declaredMediaType)
	if mediaType != "" {
		parsed, _, err := mime.ParseMediaType(mediaType)
		if err != nil {
			return "", "", fmt.Errorf("parse source media type: %w", err)
		}
		mediaType = strings.ToLower(parsed)
	}

	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(name)))
	var format string
	switch extension {
	case ".txt":
		format = "txt"
	case ".md", ".markdown":
		format = "markdown"
	case ".docx":
		format = "docx"
	default:
		return "", "", fmt.Errorf("unsupported source extension %q", extension)
	}

	allowedMediaTypes := map[string]map[string]bool{
		"txt": {
			"":                         true,
			"application/octet-stream": true,
			"text/plain":               true,
		},
		"markdown": {
			"":                         true,
			"application/octet-stream": true,
			"text/markdown":            true,
			"text/plain":               true,
		},
		"docx": {
			"":                         true,
			"application/octet-stream": true,
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
		},
	}
	if !allowedMediaTypes[format][mediaType] {
		return "", "", fmt.Errorf("media type %q does not match %s source", mediaType, format)
	}
	if mediaType == "" || mediaType == "application/octet-stream" {
		switch format {
		case "txt":
			mediaType = "text/plain"
		case "markdown":
			mediaType = "text/markdown"
		case "docx":
			mediaType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		}
	}
	return format, mediaType, nil
}

func mediaTypeForSourceType(sourceType string) string {
	switch sourceType {
	case "txt":
		return "text/plain"
	case "markdown":
		return "text/markdown"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return "application/octet-stream"
	}
}

func normalizeSourceText(text string) string {
	text = strings.TrimPrefix(text, "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

func countTextParagraphs(text string) int {
	count := 0
	for _, paragraph := range strings.Split(text, "\n") {
		if strings.TrimSpace(paragraph) != "" {
			count++
		}
	}
	return count
}

func extractDOCXText(original []byte) (string, int, error) {
	archive, err := zip.NewReader(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		return "", 0, fmt.Errorf("open DOCX package: %w", err)
	}

	var documentPart *zip.File
	for _, part := range archive.File {
		if part.Name == "word/document.xml" {
			documentPart = part
			break
		}
	}
	if documentPart == nil {
		return "", 0, errors.New("DOCX package is missing word/document.xml")
	}
	if documentPart.UncompressedSize64 > maxExtractedXMLBytes {
		return "", 0, fmt.Errorf("DOCX document.xml exceeds the %d byte limit", maxExtractedXMLBytes)
	}

	partReader, err := documentPart.Open()
	if err != nil {
		return "", 0, fmt.Errorf("open DOCX document.xml: %w", err)
	}
	defer partReader.Close()
	limited := io.LimitReader(partReader, maxExtractedXMLBytes+1)
	return decodeWordprocessingML(limited)
}

func decodeWordprocessingML(reader io.Reader) (string, int, error) {
	decoder := xml.NewDecoder(reader)
	var output strings.Builder
	var paragraph strings.Builder
	inParagraph := false
	inText := false
	paragraphCount := 0

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", 0, fmt.Errorf("decode DOCX document.xml: %w", err)
		}
		switch typed := token.(type) {
		case xml.StartElement:
			switch typed.Name.Local {
			case "p":
				inParagraph = true
				paragraph.Reset()
			case "t":
				if inParagraph {
					inText = true
				}
			case "tab":
				if inParagraph {
					paragraph.WriteByte('\t')
				}
			case "br", "cr":
				if inParagraph {
					paragraph.WriteByte('\n')
				}
			}
		case xml.CharData:
			if inParagraph && inText {
				paragraph.Write([]byte(typed))
			}
		case xml.EndElement:
			switch typed.Name.Local {
			case "t":
				inText = false
			case "p":
				text := normalizeSourceText(paragraph.String())
				output.WriteString(text)
				output.WriteByte('\n')
				if strings.TrimSpace(text) != "" {
					paragraphCount++
				}
				inParagraph = false
				inText = false
			}
		}
	}
	return output.String(), paragraphCount, nil
}
