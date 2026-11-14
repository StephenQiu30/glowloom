// Package native extracts bounded TXT and DOCX originals without remote execution.
package native

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

// Extractor performs bounded native TXT/Word extraction without remote execution.
type Extractor struct{}

// NewExtractor creates the stateless original-document extractor.
func NewExtractor() *Extractor { return &Extractor{} }

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// Extract verifies exact original bytes without changing the caller's file
// position, closing its handle, or removing its original.
func (e *Extractor) Extract(ctx context.Context, file *application.File) (application.Result, error) {
	if err := ctx.Err(); err != nil {
		return application.Result{}, err
	}
	if file == nil || file.File == nil || file.Size < 1 || file.Size > application.MaxBytes {
		return application.Result{}, application.ErrExtractionFailed
	}
	reader := io.NewSectionReader(file.File, 0, file.Size+1)
	data, err := io.ReadAll(contextReader{ctx: ctx, reader: reader})
	if err != nil {
		return application.Result{}, fmt.Errorf("read script original: %w", errors.Join(application.ErrExtractionFailed, err))
	}
	if int64(len(data)) != file.Size || domain.ContentSHA(data) != file.SHA256 {
		return application.Result{}, application.ErrExtractionFailed
	}
	var result application.Result
	switch file.MIMEType {
	case "text/plain":
		text, encoding, err := decodeText(data)
		if err != nil {
			return result, fmt.Errorf("decode text: %w", errors.Join(application.ErrExtractionFailed, err))
		}
		normalized := domain.NormalizeText(text)
		result.Document = textDocument(normalized)
		result.Encoding = encoding
		start, end := 0, utf8.RuneCountInString(text)
		result.Mapping = []domain.SourceOriginMapping{{Part: "original", DecodedStart: &start, DecodedEnd: &end, Start: 0, End: utf8.RuneCountInString(normalized)}}
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		result, err = extractDOCX(ctx, data)
		if err != nil {
			return application.Result{}, fmt.Errorf("extract Word source: %w", errors.Join(application.ErrExtractionFailed, err))
		}
	default:
		return result, application.ErrExtractionFailed
	}
	if err := result.Document.Validate(); err != nil {
		return application.Result{}, fmt.Errorf("validate extracted source: %w", errors.Join(application.ErrExtractionFailed, err))
	}
	return result, ctx.Err()
}

func decodeText(data []byte) (string, string, error) {
	if bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		data = data[3:]
		if !utf8.Valid(data) {
			return "", "", application.ErrExtractionFailed
		}
		return checkedDecoded(string(data), "utf-8-bom")
	}
	if bytes.HasPrefix(data, []byte{0xff, 0xfe}) || bytes.HasPrefix(data, []byte{0xfe, 0xff}) {
		var order binary.ByteOrder = binary.BigEndian
		encoding := "utf-16be"
		if data[0] == 0xff {
			order, encoding = binary.LittleEndian, "utf-16le"
		}
		data = data[2:]
		if len(data)%2 != 0 {
			return "", "", application.ErrExtractionFailed
		}
		words := make([]uint16, len(data)/2)
		for i := range words {
			words[i] = order.Uint16(data[i*2 : i*2+2])
		}
		for i := 0; i < len(words); i++ {
			word := words[i]
			if word >= 0xdc00 && word <= 0xdfff {
				return "", "", application.ErrExtractionFailed
			}
			if word >= 0xd800 && word <= 0xdbff {
				if i+1 >= len(words) || words[i+1] < 0xdc00 || words[i+1] > 0xdfff {
					return "", "", application.ErrExtractionFailed
				}
				i++
			}
		}
		return checkedDecoded(string(utf16.Decode(words)), encoding)
	}
	if utf8.Valid(data) {
		return checkedDecoded(string(data), "utf-8")
	}
	if !validGB18030(data) {
		return "", "", application.ErrExtractionFailed
	}
	decoded, _, err := transform.Bytes(simplifiedchinese.GB18030.NewDecoder(), data)
	if err != nil {
		return "", "", err
	}
	if bytes.Contains(decoded, []byte(string(utf8.RuneError))) {
		encoded, _, err := transform.Bytes(simplifiedchinese.GB18030.NewEncoder(), decoded)
		if err != nil || !bytes.Equal(encoded, data) {
			return "", "", application.ErrExtractionFailed
		}
	}
	return checkedDecoded(string(decoded), "gb18030")
}

func checkedDecoded(text, encoding string) (string, string, error) {
	if strings.ContainsRune(text, 0) || utf8.RuneCountInString(text) > domain.MaxScalarCount {
		return "", "", application.ErrExtractionFailed
	}
	return text, encoding, nil
}

func validGB18030(data []byte) bool {
	for i := 0; i < len(data); i++ {
		b := data[i]
		if b < 0x80 || b == 0x80 {
			continue
		}
		if b < 0x81 || b > 0xfe || i+1 >= len(data) {
			return false
		}
		second := data[i+1]
		if second >= 0x40 && second <= 0xfe && second != 0x7f {
			i++
			continue
		}
		if second < 0x30 || second > 0x39 || i+3 >= len(data) || data[i+2] < 0x81 || data[i+2] > 0xfe || data[i+3] < 0x30 || data[i+3] > 0x39 {
			return false
		}
		i += 3
	}
	return true
}

func textDocument(text string) domain.RichDocument {
	doc := domain.RichDocument{Type: "doc"}
	for _, block := range strings.Split(text, "\n\n") {
		paragraph := domain.RichDocument{Type: "paragraph"}
		for i, line := range strings.Split(block, "\n") {
			if i > 0 {
				paragraph.Content = append(paragraph.Content, domain.RichDocument{Type: "hardBreak"})
			}
			if line != "" {
				paragraph.Content = append(paragraph.Content, domain.RichDocument{Type: "text", Text: line})
			}
		}
		doc.Content = append(doc.Content, paragraph)
	}
	return doc
}
