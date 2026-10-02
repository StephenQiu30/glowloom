// Package document validates bounded original TXT and non-macro DOCX containers.
// It does not extract, decode or normalize script content.
package document

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

const (
	maxParts               = 4096
	maxPartBytes     int64 = 16 << 20
	maxExpandedBytes int64 = 64 << 20
	contentTypesNS         = "http://schemas.openxmlformats.org/package/2006/content-types"
	relationshipsNS        = "http://schemas.openxmlformats.org/package/2006/relationships"
	wordNS                 = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	strictWordNS           = "http://purl.oclc.org/ooxml/wordprocessingml/main"
	mainContentType        = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
)

// Probe checks complete original bytes without invoking AV programs or network I/O.
func Probe(ctx context.Context, file *application.Downloaded) (application.ProbeResult, error) {
	if file == nil || file.File == nil || file.Size < 1 || file.Size > domain.MaxDocumentBytes {
		return application.ProbeResult{}, application.ErrUnsupportedUpload
	}
	info, err := file.File.Stat()
	if err != nil {
		return application.ProbeResult{}, err
	}
	if info.Size() != file.Size {
		return application.ProbeResult{}, application.ErrUnsupportedUpload
	}
	var extension string
	switch file.MIMEType {
	case domain.MIMEText:
		extension = "txt"
		err = validateText(ctx, io.NewSectionReader(file.File, 0, file.Size))
	case domain.MIMEDOCX:
		extension = "docx"
		err = validateDOCX(ctx, file)
	default:
		err = application.ErrUnsupportedUpload
	}
	if err != nil {
		return application.ProbeResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return application.ProbeResult{}, err
	}
	return application.ProbeResult{Kind: domain.KindDocument, Extension: extension, Codec: &extension}, nil
}

func validateText(ctx context.Context, reader io.Reader) error {
	var prefix [512]byte
	n, err := io.ReadFull(contextReader{ctx, reader}, prefix[:])
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	}
	if n == 0 {
		return application.ErrUnsupportedUpload
	}
	data := prefix[:n]
	// UTF16's BOM is an actual container marker. Its NUL bytes are expected;
	// decoding and invalid scalar/encoding errors remain the script owner's job.
	utf16 := bytes.HasPrefix(data, []byte{0xff, 0xfe}) || bytes.HasPrefix(data, []byte{0xfe, 0xff})
	if !utf16 && binaryTextBytes(data) {
		return application.ErrUnsupportedUpload
	}
	if !utf16 {
		mime := http.DetectContentType(data)
		if strings.HasPrefix(mime, "image/") || strings.HasPrefix(mime, "audio/") || strings.HasPrefix(mime, "video/") || strings.HasPrefix(mime, "application/") && mime != "application/octet-stream" {
			return application.ErrUnsupportedUpload
		}
	}
	var buffer [32 << 10]byte
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := reader.Read(buffer[:])
		if !utf16 && binaryTextBytes(buffer[:n]) {
			return application.ErrUnsupportedUpload
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func binaryTextBytes(data []byte) bool {
	for _, b := range data {
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' && b != '\f' || b == 0x7f {
			return true
		}
	}
	return false
}

func validateDOCX(ctx context.Context, file *application.Downloaded) error {
	archive, err := zip.NewReader(file.File, file.Size)
	if err != nil || len(archive.File) == 0 || len(archive.File) > maxParts {
		return application.ErrUnsupportedUpload
	}
	seen := make(map[string]bool, len(archive.File))
	var expanded uint64
	mainDeclared, officeRelationship, bodyValid := false, false, false
	for _, part := range archive.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := part.Name
		if !validPartName(name) || seen[name] || part.Flags&1 != 0 || part.UncompressedSize64 > uint64(maxPartBytes) || part.Method != zip.Store && part.Method != zip.Deflate || strings.Contains(strings.ToLower(name), "vbaproject") {
			return application.ErrUnsupportedUpload
		}
		seen[name] = true
		expanded += part.UncompressedSize64
		if expanded > uint64(maxExpandedBytes) {
			return application.ErrUnsupportedUpload
		}
		reader, err := part.Open()
		if err != nil {
			return application.ErrUnsupportedUpload
		}
		// Reading through EOF checks actual ZIP CRC and declared expanded size.
		data, readErr := io.ReadAll(io.LimitReader(contextReader{ctx, reader}, maxPartBytes+1))
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil || int64(len(data)) > maxPartBytes || uint64(len(data)) != part.UncompressedSize64 {
			if err := ctx.Err(); err != nil {
				return err
			}
			return application.ErrUnsupportedUpload
		}
		if part.FileInfo().IsDir() {
			continue
		}
		if path.Ext(name) == ".xml" || path.Ext(name) == ".rels" {
			root, attrs, err := validateXML(ctx, data)
			if err != nil {
				return err
			}
			switch name {
			case "[Content_Types].xml":
				if root != (xml.Name{Space: contentTypesNS, Local: "Types"}) {
					return application.ErrUnsupportedUpload
				}
				for _, a := range attrs {
					if a.Name.Local == "ContentType" && (strings.Contains(strings.ToLower(a.Value), "macroenabled") || strings.Contains(strings.ToLower(a.Value), "vbaproject")) {
						return application.ErrUnsupportedUpload
					}
				}
				mainDeclared = hasAttributes(data, "Override", map[string]string{"PartName": "/word/document.xml", "ContentType": mainContentType})
			case "_rels/.rels":
				if root != (xml.Name{Space: relationshipsNS, Local: "Relationships"}) {
					return application.ErrUnsupportedUpload
				}
				officeRelationship = hasAttributes(data, "Relationship", map[string]string{"Target": "word/document.xml", "Type": "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"}) || hasAttributes(data, "Relationship", map[string]string{"Target": "word/document.xml", "Type": "http://purl.oclc.org/ooxml/officeDocument/relationships/officeDocument"})
			case "word/document.xml":
				bodyValid = root.Local == "document" && (root.Space == wordNS || root.Space == strictWordNS)
			}
		}
	}
	if !mainDeclared || !officeRelationship || !bodyValid {
		return application.ErrUnsupportedUpload
	}
	return nil
}

func validPartName(name string) bool {
	base := strings.TrimSuffix(name, "/")
	return base != "" && len(name) <= 512 && path.Clean(base) == base && !strings.HasPrefix(base, "/") && base != ".." && !strings.HasPrefix(base, "../") && !strings.ContainsAny(base, "\\\x00")
}

func validateXML(ctx context.Context, data []byte) (xml.Name, []xml.Attr, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var root xml.Name
	var attrs []xml.Attr
	depth, nodes, roots := 0, 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return root, nil, err
		}
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return root, nil, application.ErrUnsupportedUpload
		}
		switch t := token.(type) {
		case xml.StartElement:
			seenAttrs := make(map[xml.Name]bool, len(t.Attr))
			for _, attr := range t.Attr {
				if seenAttrs[attr.Name] {
					return root, nil, application.ErrUnsupportedUpload
				}
				seenAttrs[attr.Name] = true
			}
			if depth == 0 {
				root, roots = t.Name, roots+1
			}
			depth++
			nodes++
			if depth > 64 || nodes > 200000 || roots > 1 {
				return root, nil, application.ErrUnsupportedUpload
			}
			attrs = append(attrs, t.Attr...)
		case xml.EndElement:
			depth--
		case xml.Directive:
			return root, nil, application.ErrUnsupportedUpload
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(t)) != "" {
				return root, nil, application.ErrUnsupportedUpload
			}
		}
	}
	if roots != 1 || depth != 0 {
		return root, nil, application.ErrUnsupportedUpload
	}
	return root, attrs, nil
}

func hasAttributes(data []byte, element string, required map[string]string) bool {
	d := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := d.Token()
		if err != nil {
			return false
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != element {
			continue
		}
		matches := 0
		for _, attr := range start.Attr {
			if wanted, ok := required[attr.Name.Local]; ok && attr.Value == wanted {
				matches++
			}
		}
		if matches == len(required) {
			return true
		}
	}
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, fmt.Errorf("read document: %w", err)
	}
	return r.reader.Read(p)
}
